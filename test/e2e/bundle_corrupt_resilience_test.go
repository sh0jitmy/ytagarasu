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
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/bundle"
	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBundle_CorruptResilience_EmptyAndTruncated verifies that empty, invalid magic,
// or truncated bundles return distinct, actionable errors and never cause panics.
//
//nolint:paralleltest // Creates temporary files for resilience inspection
func TestBundle_CorruptResilience_EmptyAndTruncated(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Zero-byte bundle (simulating power failure during file write)
	emptyBundlePath := filepath.Join(tempDir, "empty-bundle.tar.gz")
	require.NoError(t, os.WriteFile(emptyBundlePath, []byte{}, 0600))

	_, err := bundle.VerifyBundle(emptyBundlePath, nil)
	require.Error(t, err, "VerifyBundle must return an error for 0-byte file")
	require.ErrorIs(t, err, bundle.ErrEmptyBundle)
	assert.Contains(t, err.Error(), "bundle file is empty")

	// 2. Corrupted magic header (simulating invalid file extension or non-gzip file)
	corruptMagicPath := filepath.Join(tempDir, "corrupt-magic.tar.gz")
	require.NoError(t, os.WriteFile(corruptMagicPath, []byte("NOT_A_GZIP_FILE_JUST_RANDOM_TEXT_FOR_TESTING"), 0600))

	_, err = bundle.VerifyBundle(corruptMagicPath, nil)
	require.Error(t, err, "VerifyBundle must return an error for invalid gzip magic")
	require.ErrorIs(t, err, bundle.ErrCorruptBundle)
	assert.Contains(t, err.Error(), "invalid gzip or corrupt header")

	// 3. Truncated bundle archive (simulating USB disconnected midway)
	// Create a valid gzip stream then truncate half of it
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	content := []byte("manifest: dummy content for truncation test")
	hdr := &tar.Header{
		Name: "manifest.yaml",
		Mode: 0600,
		Size: int64(len(content)),
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, _ = tw.Write(content)
	require.NoError(t, tw.Close())
	require.NoError(t, gw.Close())

	validBytes := buf.Bytes()
	require.NotEmpty(t, validBytes)

	// Truncate the file at 40% of its content
	truncatedLength := len(validBytes) * 4 / 10
	truncatedPath := filepath.Join(tempDir, "truncated-bundle.tar.gz")
	require.NoError(t, os.WriteFile(truncatedPath, validBytes[:truncatedLength], 0600))

	_, err = bundle.VerifyBundle(truncatedPath, nil)
	require.Error(t, err, "VerifyBundle must return an error for truncated stream")
	require.ErrorIs(t, err, bundle.ErrCorruptBundle)
	assert.Contains(t, err.Error(), "corrupt or truncated")
}

// TestSupervisor_CorruptResilience_TarValidation verifies that supervisor.ValidateTarArchive
// detects empty, invalid, and healthy container image tar files prior to passing them to docker load.
//
//nolint:paralleltest // File-based resilience validation
func TestSupervisor_CorruptResilience_TarValidation(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Zero-byte tar archive
	emptyTar := filepath.Join(tempDir, "empty.tar")
	require.NoError(t, os.WriteFile(emptyTar, []byte{}, 0600))

	err := supervisor.ValidateTarArchive(emptyTar)
	require.Error(t, err)
	require.ErrorIs(t, err, supervisor.ErrEmptyArchive)
	assert.Contains(t, err.Error(), "archive is empty")

	// 2. Corrupt / non-tar data
	corruptTar := filepath.Join(tempDir, "corrupt.tar")
	require.NoError(t, os.WriteFile(corruptTar, []byte("THIS IS NOT A VALID TAR FILE AT ALL"), 0600))

	err = supervisor.ValidateTarArchive(corruptTar)
	require.Error(t, err)
	require.ErrorIs(t, err, supervisor.ErrCorruptArchive)
	assert.Contains(t, err.Error(), "corrupt or invalid tar header")

	// 3. Valid tar archive with 1 file entry
	validTar := filepath.Join(tempDir, "valid.tar")
	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)
	entryContent := []byte(`{"schemaVersion": 2}`)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "manifest.json",
		Mode: 0600,
		Size: int64(len(entryContent)),
	}))
	_, _ = tw.Write(entryContent)
	require.NoError(t, tw.Close())
	require.NoError(t, os.WriteFile(validTar, tarBuf.Bytes(), 0600))

	err = supervisor.ValidateTarArchive(validTar)
	require.NoError(t, err, "ValidateTarArchive must succeed for valid tar archive")
}

// TestBundle_CorruptResilience_TamperedPayload ensures 1-bit signature mismatch is detected.
//
//nolint:paralleltest
func TestBundle_CorruptResilience_TamperedPayload(t *testing.T) {
	tempDir := t.TempDir()

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	manifestData := []byte("version: \"1.0\"\nrelease: \"resilience-v1\"\n")
	sig := ed25519.Sign(privKey, manifestData)

	// Tamper 1 bit in manifest
	tamperedManifest := make([]byte, len(manifestData))
	copy(tamperedManifest, manifestData)
	tamperedManifest[0] ^= 0x01 // Flip 1 bit

	isValid := bundle.VerifySignature(pubKey, tamperedManifest, sig)
	assert.False(t, isValid, "VerifySignature must reject tampered content with 1 bit difference")
	_ = tempDir
}
