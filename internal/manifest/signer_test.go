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

package manifest_test

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEd25519Signer_GenerateAndSave(t *testing.T) {
	t.Parallel()

	pubHex, privHex, err := manifest.GenerateEd25519KeyPair()
	require.NoError(t, err)

	pubBytes, err := hex.DecodeString(pubHex)
	require.NoError(t, err)
	assert.Len(t, pubBytes, 32)

	privBytes, err := hex.DecodeString(privHex)
	require.NoError(t, err)
	assert.Len(t, privBytes, 64)

	tmpDir := t.TempDir()
	prefix := filepath.Join(tmpDir, "keys", "release-signing")

	pubPath, privPath, err := manifest.SaveKeyPair(prefix)
	require.NoError(t, err)
	assert.Equal(t, prefix+".pub", pubPath)
	assert.Equal(t, prefix+".key", privPath)

	privInfo, err := os.Stat(privPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), privInfo.Mode().Perm())
}

func TestEd25519Signer_SignAndVerifyData(t *testing.T) {
	t.Parallel()

	pubHex, privHex, err := manifest.GenerateEd25519KeyPair()
	require.NoError(t, err)

	data := []byte("version: 1.0.0\nservice: payment-gateway\n")

	sigHex, err := manifest.SignData(privHex, data)
	require.NoError(t, err)

	valid, err := manifest.VerifySignature(pubHex, data, sigHex)
	require.NoError(t, err)
	assert.True(t, valid)

	// Tampered data should fail verification
	tamperedData := []byte("version: 1.0.0\nservice: payment-gateway-hacked\n")
	valid, err = manifest.VerifySignature(pubHex, tamperedData, sigHex)
	require.NoError(t, err)
	assert.False(t, valid)

	// Corrupted signature format
	_, err = manifest.VerifySignature(pubHex, data, "invalid-hex")
	require.Error(t, err)

	// Signature wrong length
	_, err = manifest.VerifySignature(pubHex, data, "001122")
	require.Error(t, err)

	// Invalid public key length
	_, err = manifest.VerifySignature("aabb", data, sigHex)
	require.Error(t, err)
	require.ErrorIs(t, err, manifest.ErrInvalidKey)
}

func TestEd25519Signer_SignAndVerifyFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	prefix := filepath.Join(tmpDir, "manifest-key")
	pubPath, privPath, err := manifest.SaveKeyPair(prefix)
	require.NoError(t, err)

	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	manifestContent := []byte("applications:\n  payment-api:\n    version: 1.2.0\n")
	require.NoError(t, os.WriteFile(manifestPath, manifestContent, 0600))

	// Sign file
	sigHex, err := manifest.SignFile(privPath, manifestPath)
	require.NoError(t, err)
	require.NotEmpty(t, sigHex)

	// Save detached signature file
	sigPath := manifestPath + ".sig"
	require.NoError(t, os.WriteFile(sigPath, []byte(sigHex+"\n"), 0600))

	// Verify using file paths
	err = manifest.VerifyFile(pubPath, manifestPath, sigPath)
	require.NoError(t, err)

	// Verify using direct hex strings
	pubHexBytes, err := os.ReadFile(filepath.Clean(pubPath)) //nolint:gosec // Reading test key file
	require.NoError(t, err)
	err = manifest.VerifyFile(string(pubHexBytes), manifestPath, sigHex)
	require.NoError(t, err)

	// Tampering detection: modify manifest
	tamperedContent := []byte("applications:\n  payment-api:\n    version: 1.2.0\n    malicious: true\n")
	require.NoError(t, os.WriteFile(manifestPath, tamperedContent, 0600))

	err = manifest.VerifyFile(pubPath, manifestPath, sigPath)
	require.Error(t, err)
	require.ErrorIs(t, err, manifest.ErrSignatureMismatch)
}
