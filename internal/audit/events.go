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
	"time"
)

// Security Audit Event Types required by Issue #94 & air-gap compliance.
const (
	EventSecurityTamperingDetected = "SECURITY_TAMPERING_DETECTED"
	EventSecuritySignatureVerified = "SECURITY_SIGNATURE_VERIFIED"
	EventProcessCrashed            = "PROCESS_CRASHED"
	EventRollbackTriggered         = "ROLLBACK_TRIGGERED"
)

// Severity represents RFC 5424 Syslog severity levels.
type Severity int

const (
	SeverityEmergency     Severity = 0 // System is unusable
	SeverityAlert         Severity = 1 // Action must be taken immediately
	SeverityCritical      Severity = 2 // Critical conditions (e.g. tampering detected)
	SeverityError         Severity = 3 // Error conditions (e.g. process crashed)
	SeverityWarning       Severity = 4 // Warning conditions (e.g. rollback triggered)
	SeverityNotice        Severity = 5 // Normal but significant condition (e.g. signature verified)
	SeverityInformational Severity = 6 // Informational messages
	SeverityDebug         Severity = 7 // Debug-level messages
)

// Facility represents RFC 5424 Syslog facility codes.
type Facility int

const (
	FacilityKern     Facility = 0
	FacilityUser     Facility = 1
	FacilityMail     Facility = 2
	FacilityDaemon   Facility = 3
	FacilityAuth     Facility = 4
	FacilitySyslog   Facility = 5
	FacilityLpr      Facility = 6
	FacilityNews     Facility = 7
	FacilityUucp     Facility = 8
	FacilityCron     Facility = 9
	FacilityAuthPriv Facility = 10
	FacilityFtp      Facility = 11
	FacilityLocal0   Facility = 16
	FacilityLocal1   Facility = 17
	FacilityLocal2   Facility = 18
	FacilityLocal3   Facility = 19
	FacilityLocal4   Facility = 20
	FacilityLocal5   Facility = 21
	FacilityLocal6   Facility = 22
	FacilityLocal7   Facility = 23
)

// Priority computes RFC 5424 PRI value = (Facility * 8) + Severity.
func Priority(f Facility, s Severity) int {
	return int(f)*8 + int(s)
}

// AuditEvent represents a high-integrity security audit event to be forwarded.
type AuditEvent struct {
	Timestamp      time.Time         `json:"timestamp"`
	EventType      string            `json:"event_type"`
	Severity       Severity          `json:"severity"`
	Facility       Facility          `json:"facility"`
	AppName        string            `json:"app_name,omitempty"`
	ProcessID      string            `json:"process_id,omitempty"`
	MessageID      string            `json:"message_id,omitempty"`
	Message        string            `json:"message"`
	EntityID       string            `json:"entity_id,omitempty"`
	Actor          string            `json:"actor,omitempty"`
	RecordHash     string            `json:"record_hash,omitempty"`
	StructuredData map[string]string `json:"structured_data,omitempty"`
}

// DefaultSeverityForEvent maps event types to sensible security defaults.
func DefaultSeverityForEvent(eventType string) Severity {
	switch eventType {
	case EventSecurityTamperingDetected:
		return SeverityCritical
	case EventProcessCrashed:
		return SeverityError
	case EventRollbackTriggered:
		return SeverityWarning
	case EventSecuritySignatureVerified:
		return SeverityNotice
	case EventDeployFailure:
		return SeverityError
	case EventDeploySuccess, EventReleaseImport, EventConfigValidated:
		return SeverityInformational
	default:
		return SeverityInformational
	}
}
