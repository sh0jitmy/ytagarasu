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
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/bundle"
	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestAirgap_MultiPart_EmailAndTransferSimulationScenario verifies the end-to-end lifecycle
// of multi-part chunked bundle deployment across secure email and file-transfer gateways.
//
//nolint:paralleltest,funlen // Comprehensive integration scenario test
func TestAirgap_MultiPart_EmailAndTransferSimulationScenario(t *testing.T) {
	testDir := t.TempDir()
	t.Logf("Airgap Multi-Part Workspace: %s", testDir)

	// ==========================================
	// PHASE 1: Generate Mock Application Stack & Signing Keys
	// ==========================================
	t.Log("PHASE 1: Setting up Hospital HIS application stack and Ed25519 signing keys")
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	sourceDir := filepath.Join(testDir, "source")
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "artifacts"), 0750))
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "configs"), 0750))

	// Mock Gateway Binary (200 KB, valid shell script with uncompressible random padding)
	gatewayBin := filepath.Join(sourceDir, "artifacts", "his-gateway")
	header := []byte("#!/bin/sh\nsleep 5\n# PADDING: ")
	dummyBin := make([]byte, 200*1024)
	copy(dummyBin, header)
	randPadding := make([]byte, len(dummyBin)-len(header)-1)
	_, _ = rand.Read(randPadding)
	copy(dummyBin[len(header):], randPadding)
	dummyBin[len(dummyBin)-1] = '\n'
	require.NoError(t, os.WriteFile(gatewayBin, dummyBin, 0755)) //nolint:gosec

	// Mock Config
	configPath := filepath.Join(sourceDir, "configs", "gateway.conf")
	require.NoError(t, os.WriteFile(configPath, []byte("port: 8080\nmax_connections: 500\n"), 0600))

	// Create manifest
	manifestPath := filepath.Join(sourceDir, "manifest.yaml")
	manifestContent := `version: "1.0"
bundleVersion: "1.0.0"
release: "hospital-his-multipart-v1.0.0"
targets:
  - os: "linux"
    release: "ubuntu-22.04"
    arch: "amd64"
applications:
  - name: "his-gateway"
    type: "golang"
    artifact: "artifacts/his-gateway"
    destination: "/usr/local/bin/his-gateway"
    permissions: "0755"
services:
  - name: "his-gateway"
    action: "restart"
    command: "artifacts/his-gateway"
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0600))

	// ==========================================
	// PHASE 2: Build & Split Bundle with 50 KB Chunks (Simulating Email / Portal Limit)
	// ==========================================
	t.Log("PHASE 2: Packaging bundle with --split 50KB chunking")
	chunkOutputDir := filepath.Join(testDir, "split-chunks")
	require.NoError(t, os.MkdirAll(chunkOutputDir, 0750))

	archivePath := filepath.Join(chunkOutputDir, "bundle.tar.gz")
	buildOpts := bundle.BuildOptions{
		ManifestPath:       manifestPath,
		SourceDir:          sourceDir,
		OutputFile:         archivePath,
		PrivateKey:         privKey,
		AllowWarningStatus: true,
		SplitBytes:         50 * 1024, // 50 KB chunks
		SplitOutputDir:     chunkOutputDir,
	}

	evalRes, err := bundle.BuildBundle(buildOpts)
	require.NoError(t, err)
	require.NotNil(t, evalRes)

	// ==========================================
	// PHASE 3: Verify Split Manifest & Generated Parts
	// ==========================================
	t.Log("PHASE 3: Verifying multi-part directory structure and bundle.manifest.yaml")
	manifestFile := filepath.Join(chunkOutputDir, "bundle.manifest.yaml")
	require.FileExists(t, manifestFile)

	manifestData, err := os.ReadFile(filepath.Clean(manifestFile)) //nolint:gosec
	require.NoError(t, err)

	var splitManifest bundle.MultiPartManifest
	require.NoError(t, yaml.Unmarshal(manifestData, &splitManifest))

	assert.Equal(t, "MultiPartBundleManifest", splitManifest.Kind)
	assert.GreaterOrEqual(t, splitManifest.Parts.TotalParts, int64(2))
	t.Logf("Generated %d parts for archive (chunk size: %d bytes)",
		splitManifest.Parts.TotalParts, splitManifest.Parts.ChunkSize)

	for _, part := range splitManifest.Parts.Items {
		partPath := filepath.Join(chunkOutputDir, part.FileName)
		require.FileExists(t, partPath)
		info, sErr := os.Stat(partPath)
		require.NoError(t, sErr)
		assert.Equal(t, part.SizeBytes, info.Size())
		assert.NotEmpty(t, part.SHA256)
	}

	// ==========================================
	// PHASE 4: Fault Tolerance: Simulate Email Missing Part (Part 2 dropped)
	// ==========================================
	t.Log("PHASE 4: Simulating email transfer error: missing Part 2")
	missingPartDir := filepath.Join(testDir, "transferred-with-missing-part")
	require.NoError(t, copyDir(chunkOutputDir, missingPartDir))

	// Remove Part 2
	require.NoError(t, os.Remove(filepath.Join(missingPartDir, "bundle.tar.gz.part002")))

	// Preflight validation must block with ErrMissingPart and pinpoint Part 2
	_, _, err = bundle.ValidateMultiPartManifest(missingPartDir)
	require.Error(t, err)
	require.ErrorIs(t, err, bundle.ErrMissingPart)
	assert.Contains(t, err.Error(), "Part 2", "error must specifically identify missing Part 2")
	t.Logf("Expected failure verified: %v", err)

	// ==========================================
	// PHASE 5: Fault Tolerance: Simulate 1-Bit Corruption in Transit
	// ==========================================
	t.Log("PHASE 5: Simulating transfer corruption: 1-bit tampering in Part 1")
	corruptPartDir := filepath.Join(testDir, "transferred-with-corruption")
	require.NoError(t, copyDir(chunkOutputDir, corruptPartDir))

	part1Path := filepath.Join(corruptPartDir, "bundle.tar.gz.part001")
	part1Bytes, err := os.ReadFile(filepath.Clean(part1Path)) //nolint:gosec
	require.NoError(t, err)
	part1Bytes[len(part1Bytes)-1] ^= 0x01                                         // flip 1 bit
	require.NoError(t, os.WriteFile(filepath.Clean(part1Path), part1Bytes, 0600)) //nolint:gosec

	// Preflight validation must block with ErrChecksumMismatch
	_, _, err = bundle.ValidateMultiPartManifest(corruptPartDir)
	require.Error(t, err)
	require.ErrorIs(t, err, bundle.ErrChecksumMismatch)
	assert.Contains(t, err.Error(), "Part 1", "error must specifically identify corrupted Part 1")
	t.Logf("Expected corruption failure verified: %v", err)

	// ==========================================
	// PHASE 6: Zero-Disk-Waste Streaming Deployment
	// ==========================================
	t.Log("PHASE 6: Executing zero-disk-waste streaming unpack directly from chunks")
	deployTargetDir := filepath.Join(testDir, "deployed-hospital-his")
	require.NoError(t, os.MkdirAll(deployTargetDir, 0750))

	// Open streaming multi-part reader without creating intermediate disk archive
	stream, loadedManifest, err := bundle.OpenMultiPartReader(chunkOutputDir)
	require.NoError(t, err)
	defer func() { _ = stream.Close() }()

	assert.Equal(t, splitManifest.Metadata.ArchiveSHA256, loadedManifest.Metadata.ArchiveSHA256)

	// Directly stream through gzip.Reader and tar.Reader into deploy target
	gzReader, err := gzip.NewReader(stream)
	require.NoError(t, err)
	defer func() { _ = gzReader.Close() }()

	tarReader := tar.NewReader(gzReader)
	unpackedFiles := 0

	for {
		header, tErr := tarReader.Next()
		if errors.Is(tErr, io.EOF) {
			break
		}
		require.NoError(t, tErr)

		cleanName := filepath.Clean(header.Name)
		if strings.HasPrefix(cleanName, "..") {
			continue
		}
		targetFile := filepath.Join(deployTargetDir, cleanName)

		if header.Typeflag == tar.TypeDir {
			require.NoError(t, os.MkdirAll(targetFile, 0750))
			continue
		}

		require.NoError(t, os.MkdirAll(filepath.Dir(targetFile), 0750))
		f, oErr := os.OpenFile(filepath.Clean(targetFile), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode)) //nolint:gosec
		require.NoError(t, oErr)
		_, cErr := io.Copy(f, tarReader) //nolint:gosec // Controlled test tar archive
		_ = f.Close()
		require.NoError(t, cErr)
		unpackedFiles++
	}

	t.Logf("Successfully unpacked %d files via streaming reader directly into %s",
		unpackedFiles, deployTargetDir)

	// Verify essential artifacts exist in deployed location
	require.FileExists(t, filepath.Join(deployTargetDir, "manifest.yaml"))
	require.FileExists(t, filepath.Join(deployTargetDir, "artifacts", "his-gateway"))
	require.FileExists(t, filepath.Join(deployTargetDir, "configs", "gateway.conf"))

	// ==========================================
	// PHASE 7: Reassembly Utility Verification (ytg bundle assemble)
	// ==========================================
	t.Log("PHASE 7: Testing bundle reassembly utility (ytg bundle assemble)")
	assembledArchive := filepath.Join(testDir, "assembled-bundle.tar.gz")
	require.NoError(t, bundle.AssembleMultiPart(chunkOutputDir, assembledArchive))

	// Reassembled archive must be valid via standard VerifyBundle
	verifyRes, err := bundle.VerifyBundle(assembledArchive, pubKey)
	require.NoError(t, err)
	assert.True(t, verifyRes.Valid)
	assert.True(t, verifyRes.SignatureValid)
	assert.True(t, verifyRes.ChecksumsValid)

	// Multi-part directory must also verify directly via VerifyMultiPartBundle
	multiVerifyRes, err := bundle.VerifyMultiPartBundle(chunkOutputDir, pubKey)
	require.NoError(t, err)
	assert.True(t, multiVerifyRes.Valid)
	assert.True(t, multiVerifyRes.SignatureValid)

	// ==========================================
	// PHASE 8: Launch Supervisor from Deployed Manifest & Verify Health
	// ==========================================
	t.Log("PHASE 8: Launching process supervisor from streamed deployment")
	deployedManifestFile := filepath.Join(deployTargetDir, "manifest.yaml")
	require.FileExists(t, deployedManifestFile)

	deployedBinPath := filepath.Join(deployTargetDir, "artifacts", "his-gateway")
	require.FileExists(t, deployedBinPath)

	supYAML := fmt.Sprintf(`version: "1.0"
processes:
  - name: his-gateway
    binary: %s
    work_dir: %s
    auto_restart: true
`, deployedBinPath, deployTargetDir)

	ms, err := supervisor.NewManifestSupervisorFromYAML([]byte(supYAML))
	require.NoError(t, err, "failed to create manifest supervisor from deployed multi-part files")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, ms.StartAll(ctx), "failed starting supervisor")
	defer func() { _ = ms.StopAll(1 * time.Second) }()
	time.Sleep(300 * time.Millisecond)

	procStatus := ms.Status()
	assert.NotEmpty(t, procStatus)
	t.Logf("Supervisor successfully supervised services (%d active)", len(procStatus))
	if status, ok := procStatus["his-gateway"]; ok {
		assert.Equal(t, supervisor.StateRunning, status.State)
		t.Logf("Service 'his-gateway' running (PID: %d, State: %s, Restarts: %d)",
			status.PID, status.State, status.Restarts)
	}
	t.Log("PHASE 8 COMPLETE: Multi-part airgap bundle deployment verified successfully with zero disk waste!")
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(filepath.Clean(path)) //nolint:gosec
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Clean(target), data, info.Mode()) //nolint:gosec // Controlled test directory copy utility
	})
}
