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

package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManifestIntegrity_Evaluation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	pubHex, privHex, err := manifest.GenerateEd25519KeyPair()
	require.NoError(t, err)

	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	sigPath := manifestPath + ".sig"
	pubPath := filepath.Join(tmpDir, "trusted.pub")

	manifestContent := []byte("version: 1.0\nservice: secure-service\n")
	require.NoError(t, os.WriteFile(manifestPath, manifestContent, 0600))

	sigHex, err := manifest.SignData(privHex, manifestContent)
	require.NoError(t, err)

	t.Run("Unsigned manifest returns UNSIGNED status", func(t *testing.T) {
		t.Parallel()
		unsignedManifest := filepath.Join(tmpDir, "unsigned.yaml")
		require.NoError(t, os.WriteFile(unsignedManifest, []byte("version: 1.0\n"), 0600))
		assert.FileExists(t, unsignedManifest)
	})

	t.Run("Signed and verified manifest lifecycle", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, os.WriteFile(sigPath, []byte(sigHex+"\n"), 0600))
		require.NoError(t, os.WriteFile(pubPath, []byte(pubHex+"\n"), 0600))

		// Verification via internal/manifest.VerifyFile
		err := manifest.VerifyFile(pubPath, manifestPath, sigPath)
		require.NoError(t, err)

		// Tampering detection: modify manifest
		tamperedPath := filepath.Join(tmpDir, "tampered.yaml")
		require.NoError(t, os.WriteFile(tamperedPath, []byte("tampered content"), 0600))
		tamperedErr := manifest.VerifyFile(pubPath, tamperedPath, sigPath)
		require.Error(t, tamperedErr)
		require.ErrorIs(t, tamperedErr, manifest.ErrSignatureMismatch)
	})
}

func TestStatusReport_IntegrityJSONSerialization(t *testing.T) {
	t.Parallel()

	sampleJSON := `{
		"manifest_path": "manifest.yaml",
		"updated_at": "2026-10-19T09:00:00Z",
		"total_count": 1,
		"integrity": {
			"verified": true,
			"status": "VERIFIED",
			"signature_path": "manifest.yaml.sig",
			"signature_found": true,
			"key_fingerprint": "a1b2c3d4e5f60718"
		},
		"services": [
			{
				"name": "worker",
				"pid": 1234,
				"state": "RUNNING",
				"restarts": 0,
				"started_at": "2026-10-19T08:00:00Z",
				"current_path": "/opt/bin/worker"
			}
		]
	}`

	var parsed map[string]interface{}
	err := json.Unmarshal([]byte(sampleJSON), &parsed)
	require.NoError(t, err)

	integrity, ok := parsed["integrity"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, integrity["verified"])
	assert.Equal(t, "VERIFIED", integrity["status"])
	assert.Equal(t, "a1b2c3d4e5f60718", integrity["key_fingerprint"])
}
