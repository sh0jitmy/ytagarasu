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

package store_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCAS_PutGetLink(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	cas, err := store.NewCAS(tmpDir)
	require.NoError(t, err)

	content := []byte("hello cas storage offline deployment")
	expectedHasher := sha256.New()
	expectedHasher.Write(content)
	expectedHash := hex.EncodeToString(expectedHasher.Sum(nil))

	// 1. Put
	hash, size, err := cas.Put(bytes.NewReader(content))
	require.NoError(t, err)
	assert.Equal(t, expectedHash, hash)
	assert.Equal(t, int64(len(content)), size)

	// 2. Has
	assert.True(t, cas.Has(expectedHash))
	assert.False(t, cas.Has("non-existent-hash"))

	// 3. Deduplication: Put identical content again
	hash2, size2, err := cas.Put(bytes.NewReader(content))
	require.NoError(t, err)
	assert.Equal(t, expectedHash, hash2)
	assert.Equal(t, int64(len(content)), size2)

	// 4. Get
	rc, err := cas.Get(expectedHash)
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()

	readBytes, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, content, readBytes)

	// 5. Link
	destPath := filepath.Join(tmpDir, "namespaces", "demo", "file.txt")
	err = cas.Link(expectedHash, destPath)
	require.NoError(t, err)

	destBytes, err := os.ReadFile(filepath.Clean(destPath)) //nolint:gosec
	require.NoError(t, err)
	assert.Equal(t, content, destBytes)
}

func TestDB_RegisterAndQuery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := store.NewDB(dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// Register Release 1
	rel1 := &store.Release{
		ID:             "rel-1",
		ServiceID:      "core-api",
		ReleaseVersion: "1.0.0",
		ManifestYAML:   "service: core-api\nrelease: 1.0.0",
		Signature:      "sig1",
		PublicKey:      "pub1",
		CreatedAt:      time.Now().UTC(),
	}
	artifacts1 := []store.ReleaseArtifact{
		{
			ID:         "art-1",
			ReleaseID:  "rel-1",
			RelPath:    "artifacts/bin/core-api",
			SHA256Hash: "hash1",
			SizeBytes:  1024,
		},
	}
	err = db.RegisterRelease(ctx, rel1, artifacts1)
	require.NoError(t, err)

	// Query active release
	active1, err := db.GetActiveRelease(ctx, "core-api")
	require.NoError(t, err)
	assert.Equal(t, "rel-1", active1.ID)
	assert.Equal(t, "1.0.0", active1.ReleaseVersion)
	assert.Equal(t, "active", active1.Status)

	// Register Release 2 for same service -> should deactivate rel-1
	rel2 := &store.Release{
		ID:             "rel-2",
		ServiceID:      "core-api",
		ReleaseVersion: "1.0.1",
		ManifestYAML:   "service: core-api\nrelease: 1.0.1",
		Signature:      "sig2",
		PublicKey:      "pub2",
		CreatedAt:      time.Now().UTC(),
	}
	artifacts2 := []store.ReleaseArtifact{
		{
			ID:         "art-2",
			ReleaseID:  "rel-2",
			RelPath:    "artifacts/bin/core-api",
			SHA256Hash: "hash2",
			SizeBytes:  2048,
		},
	}
	err = db.RegisterRelease(ctx, rel2, artifacts2)
	require.NoError(t, err)

	active2, err := db.GetActiveRelease(ctx, "core-api")
	require.NoError(t, err)
	assert.Equal(t, "rel-2", active2.ID)
	assert.Equal(t, "1.0.1", active2.ReleaseVersion)

	// Check services list
	services, err := db.ListServices(ctx)
	require.NoError(t, err)
	require.Len(t, services, 1)
	assert.Equal(t, "core-api", services[0].ID)
	assert.Equal(t, "rel-2", services[0].ActiveReleaseID)

	// Check artifacts list for rel-2
	artifacts, err := db.GetReleaseArtifacts(ctx, "rel-2")
	require.NoError(t, err)
	require.Len(t, artifacts, 1)
	assert.Equal(t, "art-2", artifacts[0].ID)
	assert.Equal(t, "hash2", artifacts[0].SHA256Hash)
}

func TestDB_AuditLogAndVerification(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := store.NewDB(dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	// 1. Initial report on empty audit log
	report, err := db.VerifyAuditTrail(ctx)
	require.NoError(t, err)
	assert.True(t, report.Valid)
	assert.Equal(t, 0, report.TotalRecords)

	// 2. Append records
	rec1, err := db.AppendAuditLog(ctx, "release.import", "rel-1", "importer", []byte("bundle 1 payload"))
	require.NoError(t, err)
	assert.Equal(t, int64(1), rec1.Sequence)

	rec2, err := db.AppendAuditLog(ctx, "agent.deploy.start", "rel-1", "agent:srv-1", []byte("deploy started"))
	require.NoError(t, err)
	assert.Equal(t, int64(2), rec2.Sequence)
	assert.Equal(t, rec1.RecordHash, rec2.PrevRecordHash)

	rec3, err := db.AppendAuditLog(ctx, "agent.deploy.success", "rel-1", "agent:srv-1", []byte("deploy completed"))
	require.NoError(t, err)
	assert.Equal(t, int64(3), rec3.Sequence)
	assert.Equal(t, rec2.RecordHash, rec3.PrevRecordHash)

	// 3. List records
	list, err := db.ListAuditLogs(ctx, 10, 0)
	require.NoError(t, err)
	require.Len(t, list, 3)
	assert.Equal(t, int64(1), list[0].Sequence)
	assert.Equal(t, int64(2), list[1].Sequence)
	assert.Equal(t, int64(3), list[2].Sequence)

	// 4. Verify valid chain
	report, err = db.VerifyAuditTrail(ctx)
	require.NoError(t, err)
	assert.True(t, report.Valid)
	assert.Equal(t, 3, report.TotalRecords)
	assert.Equal(t, int64(3), report.LastSequence)
	assert.Empty(t, report.Errors)

	// 5. Simulate unauthorized SQL tampering (e.g., malicious update to payload_digest)
	_, err = db.DB().ExecContext(ctx, "UPDATE audit_log SET payload_digest = 'malicious_digest' WHERE sequence = 2;")
	require.NoError(t, err)

	report, err = db.VerifyAuditTrail(ctx)
	require.NoError(t, err)
	assert.False(t, report.Valid)
	assert.NotEmpty(t, report.Errors)
	assert.Contains(t, report.Errors[0], "tampered record hash at sequence 2")
}
