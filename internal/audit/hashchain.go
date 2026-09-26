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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// GenesisHash defines the initial previous-hash for sequence 1 (64 hex zeros).
const GenesisHash = "0000000000000000000000000000000000000000000000000000000000000000"

// Standard audit event types.
const (
	EventReleaseImport   = "release.import"
	EventDeployStart     = "agent.deploy.start"
	EventDeploySuccess   = "agent.deploy.success"
	EventDeployFailure   = "agent.deploy.failure"
	EventDeployRollback  = "agent.rollback"
	EventConfigValidated = "config.validated"
)

// Record represents a cryptographically chained, immutable audit record.
type Record struct {
	Sequence       int64     `json:"sequence"`
	Timestamp      time.Time `json:"timestamp"`
	EventType      string    `json:"event_type"`
	EntityID       string    `json:"entity_id"`
	Actor          string    `json:"actor"`
	PayloadDigest  string    `json:"payload_digest"`
	PrevRecordHash string    `json:"prev_record_hash"`
	RecordHash     string    `json:"record_hash"`
}

// VerificationReport details the integrity evaluation of an audit hash chain.
type VerificationReport struct {
	Valid          bool     `json:"valid"`
	TotalRecords   int      `json:"total_records"`
	LastSequence   int64    `json:"last_sequence"`
	LastRecordHash string   `json:"last_record_hash"`
	Errors         []string `json:"errors,omitempty"`
}

// ComputePayloadDigest returns the hex-encoded SHA-256 hash of arbitrary payload data.
func ComputePayloadDigest(payload []byte) string {
	h := sha256.Sum256(payload)
	return hex.EncodeToString(h[:])
}

// ComputeRecordHash computes the SHA-256 hash for an audit record based on its fields.
func ComputeRecordHash(prevHash string, seq int64, ts time.Time, eventType, entityID, actor, payloadDigest string) string {
	// Canonical hash preimage format: PrevHash|Sequence|TimestampRFC3339|EventType|EntityID|Actor|PayloadDigest
	preimage := fmt.Sprintf("%s|%d|%s|%s|%s|%s|%s",
		strings.ToLower(strings.TrimSpace(prevHash)),
		seq,
		ts.UTC().Format(time.RFC3339),
		strings.TrimSpace(eventType),
		strings.TrimSpace(entityID),
		strings.TrimSpace(actor),
		strings.ToLower(strings.TrimSpace(payloadDigest)),
	)
	h := sha256.Sum256([]byte(preimage))
	return hex.EncodeToString(h[:])
}

// NewRecord creates and seals a new Record with calculated hashes.
func NewRecord(prevHash string, seq int64, ts time.Time, eventType, entityID, actor string, payload []byte) Record {
	cleanPrev := strings.TrimSpace(prevHash)
	if cleanPrev == "" {
		cleanPrev = GenesisHash
	}
	tsUTC := ts.UTC()
	digest := ComputePayloadDigest(payload)
	recHash := ComputeRecordHash(cleanPrev, seq, tsUTC, eventType, entityID, actor, digest)

	return Record{
		Sequence:       seq,
		Timestamp:      tsUTC,
		EventType:      eventType,
		EntityID:       entityID,
		Actor:          actor,
		PayloadDigest:  digest,
		PrevRecordHash: cleanPrev,
		RecordHash:     recHash,
	}
}

// VerifyChain checks that records forms an unbroken, mathematically sound hash chain.
func VerifyChain(records []Record) *VerificationReport {
	report := &VerificationReport{
		Valid:        true,
		TotalRecords: len(records),
		Errors:       make([]string, 0),
	}

	if len(records) == 0 {
		return report
	}

	for i, rec := range records {
		expectedSeq := int64(i + 1)
		if rec.Sequence != expectedSeq {
			report.Valid = false
			report.Errors = append(report.Errors,
				fmt.Sprintf("broken sequence at index %d: expected sequence %d, got %d", i, expectedSeq, rec.Sequence),
			)
		}

		if i == 0 {
			if rec.PrevRecordHash != GenesisHash {
				report.Valid = false
				report.Errors = append(report.Errors,
					fmt.Sprintf("genesis record has invalid prev_record_hash: expected %s, got %s", GenesisHash, rec.PrevRecordHash),
				)
			}
		} else {
			prevRec := records[i-1]
			if rec.PrevRecordHash != prevRec.RecordHash {
				report.Valid = false
				report.Errors = append(report.Errors,
					fmt.Sprintf("hash link broken at sequence %d: prev_record_hash %s != previous record_hash %s",
						rec.Sequence, rec.PrevRecordHash, prevRec.RecordHash),
				)
			}
		}

		expectedHash := ComputeRecordHash(
			rec.PrevRecordHash,
			rec.Sequence,
			rec.Timestamp,
			rec.EventType,
			rec.EntityID,
			rec.Actor,
			rec.PayloadDigest,
		)

		if rec.RecordHash != expectedHash {
			report.Valid = false
			report.Errors = append(report.Errors,
				fmt.Sprintf("tampered record hash at sequence %d: stored %s != recomputed %s",
					rec.Sequence, rec.RecordHash, expectedHash),
			)
		}
	}

	lastRec := records[len(records)-1]
	report.LastSequence = lastRec.Sequence
	report.LastRecordHash = lastRec.RecordHash

	return report
}
