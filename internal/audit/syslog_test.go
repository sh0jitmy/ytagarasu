// Copyright 2026 [Copyright Holder]
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: [YOUR_NAME]

package audit_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/audit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyslog_FormatRFC5424(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 11, 8, 1, 30, 0, 0, time.UTC)
	event := audit.AuditEvent{
		Timestamp:  ts,
		EventType:  audit.EventSecurityTamperingDetected,
		Severity:   audit.SeverityCritical,
		Facility:   audit.FacilityAuthPriv,
		AppName:    "ytg-supervisor",
		ProcessID:  "1234",
		MessageID:  "TAMPER-01",
		Message:    "Unauthorized artifact signature mismatch detected",
		EntityID:   "bundle-ot-99",
		Actor:      "system-integrity-scanner",
		RecordHash: "abcdef1234567890",
		StructuredData: map[string]string{
			"policy": "strict-tamper-guard",
		},
	}

	frame := audit.FormatRFC5424(event, "hospital-node-01", "ytagarasu")

	// PRI: FacilityAuthPriv (10) * 8 + SeverityCritical (2) = 82
	expectedPri := "<82>1"
	assert.True(t, strings.HasPrefix(frame, expectedPri), "must start with expected RFC 5424 PRI and version")
	assert.Contains(t, frame, "2026-11-08T01:30:00Z")
	assert.Contains(t, frame, "hospital-node-01")
	assert.Contains(t, frame, "ytg-supervisor")
	assert.Contains(t, frame, "1234")
	assert.Contains(t, frame, "TAMPER-01")
	assert.Contains(t, frame, `[audit@47450 actor="system-integrity-scanner" entityId="bundle-ot-99" eventType="SECURITY_TAMPERING_DETECTED" policy="strict-tamper-guard" recordHash="abcdef1234567890"]`)
	assert.Contains(t, frame, "Unauthorized artifact signature mismatch detected")
}

func TestSyslog_FormatRFC3164(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 11, 8, 1, 30, 0, 0, time.UTC)
	event := audit.AuditEvent{
		Timestamp: ts,
		EventType: audit.EventProcessCrashed,
		Severity:  audit.SeverityError,
		Facility:  audit.FacilityDaemon,
		AppName:   "ytg-agent",
		ProcessID: "5678",
		Message:   "Child worker terminated with uncatchable SIGKILL",
	}

	frame := audit.FormatRFC3164(event, "factory-gateway", "ytagarasu")

	// PRI: FacilityDaemon (3) * 8 + SeverityError (3) = 27
	assert.True(t, strings.HasPrefix(frame, "<27>"))
	assert.Contains(t, frame, "factory-gateway ytg-agent[5678]: Child worker terminated with uncatchable SIGKILL")
}

func TestSyslog_WriterForwarder_EndToEnd(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	cfg := audit.Config{
		AppName:         "ytg-test",
		Hostname:        "test-host",
		DefaultFacility: audit.FacilityAuthPriv,
		Format:          "rfc5424",
	}

	forwarder := audit.NewWriterForwarder(&buf, cfg)
	defer func() {
		_ = forwarder.Close()
	}()

	ctx := context.Background()

	events := []audit.AuditEvent{
		{
			EventType: audit.EventSecurityTamperingDetected,
			Message:   "Tampering detected in payload checksum",
		},
		{
			EventType: audit.EventSecuritySignatureVerified,
			Message:   "Ed25519 payload signature successfully validated",
		},
		{
			EventType: audit.EventProcessCrashed,
			Message:   "Supervisor detected unexpected child exit",
		},
		{
			EventType: audit.EventRollbackTriggered,
			Message:   "Healthcheck failed; auto rollback initiated",
		},
	}

	for _, ev := range events {
		err := forwarder.Forward(ctx, ev)
		require.NoError(t, err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.Len(t, lines, 4)

	// Check line 1: SECURITY_TAMPERING_DETECTED (Critical: 10*8 + 2 = 82)
	assert.True(t, strings.HasPrefix(lines[0], "<82>1"))
	assert.Contains(t, lines[0], "Tampering detected in payload checksum")

	// Check line 2: SECURITY_SIGNATURE_VERIFIED (Notice: 10*8 + 5 = 85)
	assert.True(t, strings.HasPrefix(lines[1], "<85>1"))
	assert.Contains(t, lines[1], "Ed25519 payload signature successfully validated")

	// Check line 3: PROCESS_CRASHED (Error: 10*8 + 3 = 83)
	assert.True(t, strings.HasPrefix(lines[2], "<83>1"))
	assert.Contains(t, lines[2], "Supervisor detected unexpected child exit")

	// Check line 4: ROLLBACK_TRIGGERED (Warning: 10*8 + 4 = 84)
	assert.True(t, strings.HasPrefix(lines[3], "<84>1"))
	assert.Contains(t, lines[3], "Healthcheck failed; auto rollback initiated")
}

//nolint:paralleltest // Manipulates filesystem unix socket
func TestSyslog_UnixSocket_LiveTransfer(t *testing.T) {
	// Keep socket path short to stay within 104-byte limit on macOS / BSD
	socketPath := filepath.Join("/tmp", fmt.Sprintf("ytg_syslog_%d.sock", time.Now().UnixNano()))
	defer func() {
		_ = os.Remove(socketPath)
	}()

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer func() {
		_ = listener.Close()
	}()

	receivedFrames := make(chan string, 1)

	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		buf := make([]byte, 2048)
		n, readErr := conn.Read(buf)
		if readErr == nil || readErr == io.EOF {
			receivedFrames <- string(buf[:n])
		}
	}()

	cfg := audit.Config{
		Network:  "unix",
		Address:  socketPath,
		AppName:  "ytg-agent",
		Hostname: "airgap-node",
	}

	forwarder, err := audit.NewSyslogForwarder(cfg)
	require.NoError(t, err)
	defer func() {
		_ = forwarder.Close()
	}()

	event := audit.AuditEvent{
		EventType: audit.EventSecurityTamperingDetected,
		Message:   "Live socket audit delivery test",
		EntityID:  "manifest-v1",
	}

	err = forwarder.Forward(context.Background(), event)
	require.NoError(t, err)

	select {
	case frame := <-receivedFrames:
		assert.Contains(t, frame, "airgap-node")
		assert.Contains(t, frame, "ytg-agent")
		assert.Contains(t, frame, "Live socket audit delivery test")
		assert.Contains(t, frame, "manifest-v1")
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for live unix socket syslog delivery")
	}
}

func TestSyslog_DefaultSeverity(t *testing.T) {
	t.Parallel()

	assert.Equal(t, audit.SeverityCritical, audit.DefaultSeverityForEvent(audit.EventSecurityTamperingDetected))
	assert.Equal(t, audit.SeverityError, audit.DefaultSeverityForEvent(audit.EventProcessCrashed))
	assert.Equal(t, audit.SeverityWarning, audit.DefaultSeverityForEvent(audit.EventRollbackTriggered))
	assert.Equal(t, audit.SeverityNotice, audit.DefaultSeverityForEvent(audit.EventSecuritySignatureVerified))
	assert.Equal(t, audit.SeverityError, audit.DefaultSeverityForEvent(audit.EventDeployFailure))
	assert.Equal(t, audit.SeverityInformational, audit.DefaultSeverityForEvent(audit.EventDeploySuccess))
	assert.Equal(t, audit.SeverityInformational, audit.DefaultSeverityForEvent("UNKNOWN_EVENT"))
}
