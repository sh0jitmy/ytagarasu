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
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/audit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashChain_ValidChain(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)

	// Build a valid chain of 3 records
	r1 := audit.NewRecord(audit.GenesisHash, 1, now, audit.EventReleaseImport, "rel-1", "admin", []byte("file1"))
	r2 := audit.NewRecord(r1.RecordHash, 2, now.Add(time.Minute), audit.EventDeployStart, "rel-1", "agent:web-01", []byte("starting"))
	r3 := audit.NewRecord(r2.RecordHash, 3, now.Add(2*time.Minute), audit.EventDeploySuccess, "rel-1", "agent:web-01", []byte("ok"))

	records := []audit.Record{r1, r2, r3}
	report := audit.VerifyChain(records)

	require.NotNil(t, report)
	assert.True(t, report.Valid)
	assert.Equal(t, 3, report.TotalRecords)
	assert.Equal(t, int64(3), report.LastSequence)
	assert.Equal(t, r3.RecordHash, report.LastRecordHash)
	assert.Empty(t, report.Errors)
}

func TestHashChain_EmptyChain(t *testing.T) {
	t.Parallel()

	report := audit.VerifyChain([]audit.Record{})
	require.NotNil(t, report)
	assert.True(t, report.Valid)
	assert.Equal(t, 0, report.TotalRecords)
	assert.Empty(t, report.Errors)
}

func TestHashChain_TamperedContent(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)

	r1 := audit.NewRecord(audit.GenesisHash, 1, now, audit.EventReleaseImport, "rel-1", "admin", []byte("file1"))
	r2 := audit.NewRecord(r1.RecordHash, 2, now.Add(time.Minute), audit.EventDeployStart, "rel-1", "agent:web-01", []byte("starting"))

	// Tamper payload digest of r1 without recomputing RecordHash
	r1.PayloadDigest = "deadbeefcafebabe"

	records := []audit.Record{r1, r2}
	report := audit.VerifyChain(records)

	require.NotNil(t, report)
	assert.False(t, report.Valid)
	assert.NotEmpty(t, report.Errors)
	assert.Contains(t, report.Errors[0], "tampered record hash at sequence 1")
}

func TestHashChain_BrokenLinkBetweenBlocks(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)

	r1 := audit.NewRecord(audit.GenesisHash, 1, now, audit.EventReleaseImport, "rel-1", "admin", []byte("file1"))
	// Provide wrong previous hash for r2
	r2 := audit.NewRecord("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 2, now.Add(time.Minute), audit.EventDeployStart, "rel-1", "agent:web-01", []byte("starting"))

	records := []audit.Record{r1, r2}
	report := audit.VerifyChain(records)

	require.NotNil(t, report)
	assert.False(t, report.Valid)
	assert.NotEmpty(t, report.Errors)
	assert.Contains(t, report.Errors[0], "hash link broken at sequence 2")
}

func TestHashChain_SequenceGap(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)

	r1 := audit.NewRecord(audit.GenesisHash, 1, now, audit.EventReleaseImport, "rel-1", "admin", []byte("file1"))
	r3 := audit.NewRecord(r1.RecordHash, 3, now.Add(2*time.Minute), audit.EventDeploySuccess, "rel-1", "agent:web-01", []byte("ok"))

	records := []audit.Record{r1, r3}
	report := audit.VerifyChain(records)

	require.NotNil(t, report)
	assert.False(t, report.Valid)
	assert.NotEmpty(t, report.Errors)
	assert.Contains(t, report.Errors[0], "broken sequence at index 1")
}

func TestHashChain_InvalidGenesis(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)

	r1 := audit.NewRecord("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1, now, audit.EventReleaseImport, "rel-1", "admin", []byte("file1"))

	records := []audit.Record{r1}
	report := audit.VerifyChain(records)

	require.NotNil(t, report)
	assert.False(t, report.Valid)
	assert.NotEmpty(t, report.Errors)
	assert.Contains(t, report.Errors[0], "genesis record has invalid prev_record_hash")
}
