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
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/bundle"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestManifestAndArchiveBuildPipeline verifies the complete Day 0 / Day 1 pipeline:
// 1. Converting existing docker-compose.yml to ytagarasu manifest.yaml
// 2. Generating Ed25519 signing keys and cryptographically signing the manifest
// 3. Packaging all offline assets (binaries, packages, configs) into a distributable archive
// 4. Verifying the final bundle archive against cryptographic checksums and signatures
//
//nolint:paralleltest // Manipulates filesystem pipeline
func TestManifestAndArchiveBuildPipeline(t *testing.T) {
	workDir := t.TempDir()
	t.Logf("Build workstation workspace: %s", workDir)

	// Step 1: Create a realistic hospital / factory compose file
	composePath := filepath.Join(workDir, "docker-compose.yml")
	composeYAML := `version: "3.8"
services:
  postgres-db:
    image: postgres:16-alpine
    ports:
      - "5432:5432"
    environment:
      POSTGRES_DB: hospital_records
  redis-cache:
    image: redis:7-alpine
    ports:
      - "6379:6379"
`
	require.NoError(t, os.WriteFile(composePath, []byte(composeYAML), 0600))

	// Step 2: Convert compose to ytagarasu manifest
	composeData, err := os.ReadFile(filepath.Clean(composePath)) //nolint:gosec
	require.NoError(t, err)

	converted, err := manifest.ConvertComposeToManifest(composeData, "his-production-release")
	require.NoError(t, err)
	require.Len(t, converted.Containers, 2)
	t.Log("✓ [CONVERT] Successfully converted docker-compose.yml to manifest struct")

	// Append native supervised processes (Bulkhead hospital gateway & audit daemon)
	artifactsDir := filepath.Join(workDir, "artifacts")
	require.NoError(t, os.MkdirAll(artifactsDir, 0750))

	dummyGatewayPath := filepath.Join(artifactsDir, "his-gateway")
	require.NoError(t, os.WriteFile(dummyGatewayPath, []byte("#!/bin/sh\necho his-gateway"), 0600))

	dummyAuditPath := filepath.Join(artifactsDir, "audit-backup-daemon")
	require.NoError(t, os.WriteFile(dummyAuditPath, []byte("#!/bin/sh\necho audit-backup-daemon"), 0600))

	converted.Applications = []manifest.Application{
		{
			Name:        "his-gateway",
			Type:        "golang",
			Artifact:    "artifacts/his-gateway",
			Destination: "/opt/his/bin/his-gateway",
		},
		{
			Name:        "audit-backup-daemon",
			Type:        "golang",
			Artifact:    "artifacts/audit-backup-daemon",
			Destination: "/opt/his/bin/audit-backup-daemon",
		},
	}

	manifestPath := filepath.Join(workDir, "manifest.yaml")
	manifestBytes, err := yaml.Marshal(converted)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Clean(manifestPath), manifestBytes, 0600)) //nolint:gosec // Safe temporary path
	t.Log("✓ [MANIFEST] Saved production manifest.yaml with 2 containers and 2 native applications")

	// Step 3: Cryptographic Ed25519 key generation and manifest signing
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	signature := ed25519.Sign(privKey, manifestBytes)
	sigPath := manifestPath + ".sig"
	require.NoError(t, os.WriteFile(filepath.Clean(sigPath), signature, 0600)) //nolint:gosec // Safe temporary path

	// Verify signature immediately
	assert.True(t, ed25519.Verify(pubKey, manifestBytes, signature), "Ed25519 signature must verify")
	t.Logf("✓ [SIGN] Manifest signed with Ed25519 (Fingerprint: %s...)", hex.EncodeToString(pubKey[:8]))

	// Prepare offline container image archive mocks (images/*.tar)
	imagesDir := filepath.Join(workDir, "images")
	require.NoError(t, os.MkdirAll(imagesDir, 0750))
	require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "postgres-db.tar"), []byte("dummy tar content for postgres"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(imagesDir, "redis-cache.tar"), []byte("dummy tar content for redis"), 0600))

	// Step 4: Build offline deployment bundle archive (.tar.gz)
	outputBundle := filepath.Join(workDir, "hospital-his-bundle-2026.10.tar.gz")
	reportPath := filepath.Join(workDir, "preparation-result.yaml")

	prepResult, err := bundle.BuildBundle(bundle.BuildOptions{
		ManifestPath:       manifestPath,
		SourceDir:          workDir,
		OutputFile:         outputBundle,
		ReportOutputFile:   reportPath,
		PrivateKey:         privKey,
		AllowWarningStatus: true,
	})
	require.NoError(t, err, "failed to build offline bundle archive")
	require.NotNil(t, prepResult)

	fi, err := os.Stat(outputBundle)
	require.NoError(t, err)
	assert.Positive(t, fi.Size(), "bundle archive must not be empty")
	t.Logf("✓ [BUNDLE] Generated offline distribution archive: %s (%d bytes)", filepath.Base(outputBundle), fi.Size())

	// Step 5: Verify archive contents & SHA-256 integrity
	bundleData, err := os.ReadFile(filepath.Clean(outputBundle)) //nolint:gosec
	require.NoError(t, err)
	hasher := sha256.New()
	hasher.Write(bundleData)
	bundleSHA := hex.EncodeToString(hasher.Sum(nil))
	t.Logf("✓ [VERIFY] Bundle archive SHA-256 digest: %s", bundleSHA)

	vResult, err := bundle.VerifyBundle(outputBundle, pubKey)
	require.NoError(t, err)
	assert.True(t, vResult.Valid, "VerifyBundle must succeed for signed bundle archive")
	t.Log("✓ [INTEGRITY GATE] All cryptographic signatures, manifest schemas, and tar payloads verified")
}
