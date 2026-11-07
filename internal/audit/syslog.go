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

package audit

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Standard constants for Syslog framing.
const (
	RFC5424Version   = 1
	NilValue         = "-"
	EnterpriseID     = "47450" // Enterprise Number or Private ID for ytagarasu
	DefaultSyslogApp = "ytagarasu"
)

// Forwarder defines the contract for audit event destination forwarders.
type Forwarder interface {
	Forward(ctx context.Context, event AuditEvent) error
	Close() error
}

// Config specifies the runtime settings for Syslog forwarding.
type Config struct {
	// Network is "unixgram", "unix", "udp", or "tcp". Empty defaults to platform default.
	Network string
	// Address is socket path (/dev/log) or host:port. Empty defaults to platform default.
	Address string
	// DefaultFacility defaults to FacilityAuthPriv (10).
	DefaultFacility Facility
	// Hostname defaults to os.Hostname().
	Hostname string
	// AppName defaults to "ytagarasu".
	AppName string
	// Format is "rfc5424" (default) or "rfc3164".
	Format string
	// CustomWriter allows routing syslog frames to custom io.Writer (e.g. for testing/mocking).
	CustomWriter io.Writer
}

// WriterForwarder is a generic Forwarder that writes formatted frames to an io.Writer.
type WriterForwarder struct {
	mu       sync.Mutex
	writer   io.Writer
	closer   io.Closer
	hostname string
	appName  string
	facility Facility
	format   string
}

// NewWriterForwarder constructs a Forwarder outputting to an arbitrary io.Writer.
func NewWriterForwarder(w io.Writer, cfg Config) *WriterForwarder {
	hostname := cfg.Hostname
	if hostname == "" {
		if h, err := os.Hostname(); err == nil && h != "" {
			hostname = h
		} else {
			hostname = "localhost"
		}
	}

	appName := cfg.AppName
	if appName == "" {
		appName = DefaultSyslogApp
	}

	facility := cfg.DefaultFacility
	if facility == 0 {
		facility = FacilityAuthPriv
	}

	format := strings.ToLower(cfg.Format)
	if format == "" {
		format = "rfc5424"
	}

	var closer io.Closer
	if c, ok := w.(io.Closer); ok {
		closer = c
	}

	return &WriterForwarder{
		writer:   w,
		closer:   closer,
		hostname: hostname,
		appName:  appName,
		facility: facility,
		format:   format,
	}
}

// Forward sends an AuditEvent formatted according to the configured standard.
func (f *WriterForwarder) Forward(ctx context.Context, event AuditEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.writer == nil {
		return fmt.Errorf("audit syslog writer is closed or nil")
	}

	// Apply defaults
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Facility == 0 {
		event.Facility = f.facility
	}
	if event.Severity == 0 && event.EventType != "" {
		event.Severity = DefaultSeverityForEvent(event.EventType)
	}

	var frame string
	if f.format == "rfc3164" {
		frame = FormatRFC3164(event, f.hostname, f.appName)
	} else {
		frame = FormatRFC5424(event, f.hostname, f.appName)
	}

	data := []byte(frame + "\n")
	_, err := f.writer.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write audit syslog frame: %w", err)
	}

	return nil
}

// Close closes the underlying writer if it implements io.Closer.
func (f *WriterForwarder) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	var err error
	if f.closer != nil {
		err = f.closer.Close()
		f.closer = nil
	}
	f.writer = nil
	return err
}

// FormatRFC5424 formats an AuditEvent into an RFC 5424 syslog message:
// <PRI>1 TIMESTAMP HOSTNAME APP-NAME PROCID MSGID STRUCTURED-DATA MSG
func FormatRFC5424(event AuditEvent, hostname string, fallbackAppName string) string {
	pri := Priority(event.Facility, event.Severity)

	ts := event.Timestamp.Format(time.RFC3339Nano)

	h := sanitizeField(hostname, NilValue)

	app := event.AppName
	if app == "" {
		app = fallbackAppName
	}
	app = sanitizeField(app, NilValue)

	procID := event.ProcessID
	if procID == "" {
		procID = strconv.Itoa(os.Getpid())
	}
	procID = sanitizeField(procID, NilValue)

	msgID := sanitizeField(event.MessageID, NilValue)
	if msgID == NilValue && event.EventType != "" {
		msgID = sanitizeField(event.EventType, NilValue)
	}

	sd := buildStructuredData(event)

	msg := event.Message
	if msg == "" && event.EventType != "" {
		msg = fmt.Sprintf("Security audit event %s occurred", event.EventType)
	}

	return fmt.Sprintf("<%d>%d %s %s %s %s %s %s %s", pri, RFC5424Version, ts, h, app, procID, msgID, sd, msg)
}

// FormatRFC3164 formats an AuditEvent into legacy BSD RFC 3164 syslog format:
// <PRI>Mmm dd hh:mm:ss HOSTNAME TAG[PID]: MSG
func FormatRFC3164(event AuditEvent, hostname string, fallbackAppName string) string {
	pri := Priority(event.Facility, event.Severity)

	ts := event.Timestamp.Format("Jan _2 15:04:05")

	h := sanitizeField(hostname, "localhost")

	app := event.AppName
	if app == "" {
		app = fallbackAppName
	}
	app = sanitizeField(app, "ytagarasu")

	procID := event.ProcessID
	if procID == "" {
		procID = strconv.Itoa(os.Getpid())
	}

	msg := event.Message
	if msg == "" && event.EventType != "" {
		msg = fmt.Sprintf("event=%s", event.EventType)
	}

	return fmt.Sprintf("<%d>%s %s %s[%s]: %s", pri, ts, h, app, procID, msg)
}

func sanitizeField(val string, fallback string) string {
	val = strings.TrimSpace(val)
	if val == "" {
		return fallback
	}
	// RFC 5424 allows printable ASCII except space, =, ], ", etc. in header fields
	return strings.ReplaceAll(val, " ", "_")
}

func buildStructuredData(event AuditEvent) string {
	params := make(map[string]string)
	if event.EventType != "" {
		params["eventType"] = event.EventType
	}
	if event.EntityID != "" {
		params["entityId"] = event.EntityID
	}
	if event.Actor != "" {
		params["actor"] = event.Actor
	}
	if event.RecordHash != "" {
		params["recordHash"] = event.RecordHash
	}
	for k, v := range event.StructuredData {
		params[k] = v
	}

	if len(params) == 0 {
		return NilValue
	}

	// Stable order for deterministic output and hashing
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString("[audit@" + EnterpriseID)
	for _, k := range keys {
		v := params[k]
		// Escape backslash, quote, and closing bracket
		escaped := strings.ReplaceAll(v, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		escaped = strings.ReplaceAll(escaped, `]`, `\]`)
		fmt.Fprintf(&sb, ` %s="%s"`, k, escaped)
	}
	sb.WriteString("]")

	return sb.String()
}
