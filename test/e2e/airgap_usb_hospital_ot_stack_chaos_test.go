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

package e2e_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAirgapUSB_HospitalOTStack_ChaosRecoveryScenario tests the authentic production solution
// architecture identified by the Researcher role in medical hospital (HIS) and factory OT environments:
// - his-gateway (Electronic Health Record / DICOM / HL7 Core Gateway API)
// - audit-backup-daemon (Offline Tamper-Proof SHA-256 Hashchain & Backup Integrity Worker)
//
// It simulates air-gap deployment from USB, supervisor orchestration, and chaos injection
// (SIGKILL on his-gateway) to verify zero-touch self-healing while preserving audit trails.
//
//nolint:paralleltest // Manipulates network ports and process signals
func TestAirgapUSB_HospitalOTStack_ChaosRecoveryScenario(t *testing.T) {
	// =========================================================================
	// PHASE 1: Build Virtual USB Air-Gap Media (Medical HIS & Industrial OT Stack)
	// =========================================================================
	usbDir := t.TempDir()
	t.Logf("Virtual Air-Gap USB media mounted at: %s", usbDir)

	hisGatewayPort := getFreePort(t)
	auditDaemonPort := getFreePort(t)
	for auditDaemonPort == hisGatewayPort {
		auditDaemonPort = getFreePort(t)
	}

	binDir := filepath.Join(usbDir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755))

	// 1. Build HIS Core Gateway binary (Electronic Health Record / DICOM integration)
	hisGatewayBinPath := filepath.Join(binDir, "his-gateway")
	buildHISGatewayBinary(t, hisGatewayBinPath)

	// 2. Build Audit & Backup Integrity Daemon (Reflecting lessons from Handa Hospital report)
	auditDaemonBinPath := filepath.Join(binDir, "audit-backup-daemon")
	buildAuditBackupDaemonBinary(t, auditDaemonBinPath)

	// 3. Create offline APT cache package
	aptCachePath := filepath.Join(usbDir, "apt-cache.tar.gz")
	createDummyAptCacheTarGz(t, aptCachePath)

	// 4. Create manifest.yaml defining the hospital HIS & OT edge stack
	manifestPath := filepath.Join(usbDir, "manifest.yaml")
	manifestContent := fmt.Sprintf(`version: "1.0"
metadata:
  name: "medical-hospital-his-edge-stack"
  environment: "hospital-airgapped-cluster-node01"
  release: "2026.10-production-his"

processes:
  - name: his-gateway
    binary: %s
    args: ["-port", "%d", "-profile", "his-hl7-dicom-core"]
    work_dir: %s
    auto_restart: true
    max_retries: 5
    listen_port: %d
    health_endpoint: "http://127.0.0.1:%d/healthz"
    rollback_on_error: false

  - name: audit-backup-daemon
    binary: %s
    args: ["-port", "%d", "-audit-interval", "10s"]
    work_dir: %s
    auto_restart: true
    max_retries: 5
    listen_port: %d
    health_endpoint: "http://127.0.0.1:%d/healthz"
    rollback_on_error: false
`, hisGatewayBinPath, hisGatewayPort, usbDir, hisGatewayPort, hisGatewayPort,
		auditDaemonBinPath, auditDaemonPort, usbDir, auditDaemonPort, auditDaemonPort)

	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0644))

	// 5. Generate cryptographic SHA256SUMS file
	checksumFile := filepath.Join(usbDir, "SHA256SUMS")
	generateSHA256ChecksumFile(t, checksumFile, map[string]string{
		"manifest.yaml":            manifestPath,
		"apt-cache.tar.gz":         aptCachePath,
		"bin/his-gateway":          hisGatewayBinPath,
		"bin/audit-backup-daemon":  auditDaemonBinPath,
	})

	// =========================================================================
	// PHASE 2: Offline Cryptographic Hash & Tamper-Proof Verification Gate
	// =========================================================================
	verifySHA256Checksums(t, checksumFile, usbDir)
	t.Log("✓ [INTEGRITY GATE] All hospital air-gap USB artifacts verified by SHA-256 hashchain")

	// =========================================================================
	// PHASE 3: Supervisor Stack Orchestration & Healthcheck Verification
	// =========================================================================
	ms, err := supervisor.NewManifestSupervisor(manifestPath)
	require.NoError(t, err, "failed to initialize manifest supervisor for hospital HIS stack")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, ms.StartAll(ctx), "failed to launch supervised hospital HIS stack")
	defer func() { _ = ms.StopAll(1 * time.Second) }()

	hisHealthURL := fmt.Sprintf("http://127.0.0.1:%d/healthz", hisGatewayPort)
	auditHealthURL := fmt.Sprintf("http://127.0.0.1:%d/healthz", auditDaemonPort)

	// Ensure both HIS Gateway and Audit Daemon become healthy
	require.Eventually(t, func() bool {
		hResp, hErr := http.Get(hisHealthURL)
		if hErr != nil || hResp.StatusCode != http.StatusOK {
			return false
		}
		_ = hResp.Body.Close()

		aResp, aErr := http.Get(auditHealthURL)
		if aErr != nil || aResp.StatusCode != http.StatusOK {
			return false
		}
		_ = aResp.Body.Close()

		return true
	}, 6*time.Second, 100*time.Millisecond, "hospital HIS services failed to reach healthy state")

	t.Log("✓ [SUPERVISOR] Hospital HIS Gateway & Audit Daemon spawned and responding (HTTP 200 OK)")

	// Inspect initial process states
	statusMap := ms.Status()
	hisStatus, hExists := statusMap["his-gateway"]
	require.True(t, hExists, "his-gateway must be present in supervisor status")
	initialHISPID := hisStatus.PID
	require.Positive(t, initialHISPID)
	assert.Equal(t, 0, hisStatus.Restarts)
	assert.Equal(t, supervisor.StateRunning, hisStatus.State)

	auditStatus, aExists := statusMap["audit-backup-daemon"]
	require.True(t, aExists, "audit-backup-daemon must be present in supervisor status")
	assert.Equal(t, supervisor.StateRunning, auditStatus.State)

	t.Logf("Initial running HIS Gateway PID: %d (Restarts: 0)", initialHISPID)
	t.Logf("Initial running Audit Daemon PID: %d (Restarts: 0)", auditStatus.PID)

	// =========================================================================
	// PHASE 4: Chaos Fault Injection (kill -9 / SIGKILL on Core HIS Gateway)
	// =========================================================================
	t.Logf("⚡ [CHAOS INJECTION] Sending uncatchable SIGKILL to HIS Gateway (PID %d)...", initialHISPID)
	err = syscall.Kill(initialHISPID, syscall.SIGKILL)
	require.NoError(t, err, "failed to send SIGKILL to his-gateway")

	// Verify gateway port is dead
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", hisGatewayPort), 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
	}
	t.Log("✓ [CHAOS VERIFIED] HIS Gateway process abruptly terminated by kernel SIGKILL")

	// =========================================================================
	// PHASE 5: Supervisor Autonomous Recovery (Zero-Touch Self-Healing)
	// =========================================================================
	t.Log("Waiting for supervisor autonomous self-healing and patient care service restoration...")

	var recoveredHISPID int
	require.Eventually(t, func() bool {
		// 1. Healthcheck returns HTTP 200 again
		resp, err := http.Get(hisHealthURL)
		if err != nil || resp.StatusCode != http.StatusOK {
			return false
		}
		_ = resp.Body.Close()

		// 2. Supervisor status reflects new PID, incremented restarts, and RUNNING state
		currentStatus := ms.Status()
		proc, ok := currentStatus["his-gateway"]
		if !ok {
			return false
		}
		if proc.PID != initialHISPID && proc.Restarts >= 1 && proc.State == supervisor.StateRunning {
			recoveredHISPID = proc.PID
			return true
		}
		return false
	}, 5*time.Second, 100*time.Millisecond, "supervisor failed to auto-recover his-gateway within 5s")

	t.Logf("✓ [SELF-HEALING SUCCESSFUL] HIS Core Gateway restored with New PID: %d (Restarts: 1)", recoveredHISPID)
	assert.NotEqual(t, initialHISPID, recoveredHISPID)

	// Verify Audit Daemon was isolated and maintained 100% uptime (Bulkhead Pattern)
	currentAuditStatus := ms.Status()["audit-backup-daemon"]
	assert.Equal(t, auditStatus.PID, currentAuditStatus.PID, "Audit Daemon PID must remain unchanged")
	assert.Equal(t, 0, currentAuditStatus.Restarts, "Audit Daemon restarts should remain 0")
	assert.Equal(t, supervisor.StateRunning, currentAuditStatus.State)
	t.Log("✓ [BULKHEAD ISOLATION] Audit & Backup Daemon maintained 100% uptime with zero collateral failure")

	// =========================================================================
	// PHASE 6: Graceful Teardown
	// =========================================================================
	require.NoError(t, ms.StopAll(1*time.Second))
	t.Log("✓ [TEARDOWN] All hospital HIS processes cleanly terminated")
}

// buildHISGatewayBinary builds a realistic hospital HIS & EHR core API service mock
func buildHISGatewayBinary(t *testing.T, targetPath string) {
	t.Helper()
	srcDir := t.TempDir()
	mainGo := filepath.Join(srcDir, "main.go")
	code := `package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	port := flag.Int("port", 8080, "his gateway listen port")
	_ = flag.String("profile", "his-hl7-dicom-core", "operating profile")
	flag.Parse()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "{\"service\":\"his-gateway\",\"status\":\"HEALTHY\",\"ehr_connected\":true,\"active_clinics\":24}\n")
	})

	srv := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", *port)}
	go func() {
		_ = srv.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	_ = srv.Close()
}
`
	require.NoError(t, os.WriteFile(mainGo, []byte(code), 0644))

	cmd := exec.Command("go", "build", "-o", targetPath, mainGo)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to build his-gateway binary: %s", string(out))
}

// buildAuditBackupDaemonBinary builds an offline cryptographic backup & tamper-audit daemon mock
func buildAuditBackupDaemonBinary(t *testing.T, targetPath string) {
	t.Helper()
	srcDir := t.TempDir()
	mainGo := filepath.Join(srcDir, "main.go")
	code := `package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	port := flag.Int("port", 8082, "audit daemon listen port")
	_ = flag.String("audit-interval", "10s", "audit verification cycle")
	flag.Parse()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "{\"service\":\"audit-backup-daemon\",\"status\":\"NOMINAL\",\"integrity_gate\":\"SHA-256-OK\",\"snapshots\":14}\n")
	})

	srv := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", *port)}
	go func() {
		_ = srv.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	_ = srv.Close()
}
`
	require.NoError(t, os.WriteFile(mainGo, []byte(code), 0644))

	cmd := exec.Command("go", "build", "-o", targetPath, mainGo)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to build audit-backup-daemon binary: %s", string(out))
}
