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
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildYtgBinary(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ytg")

	cmd := exec.Command("go", "build", "-o", binPath, ".") //nolint:gosec // Test helper
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to build ytg: %s", string(out))

	return binPath
}

func TestManifestSigning_CLI_Workflow(t *testing.T) {
	t.Parallel()
	binPath := buildYtgBinary(t)
	tmpDir := t.TempDir()

	// 1. Generate keypair via ytg manifest keygen --json
	keyPrefix := filepath.Join(tmpDir, "airgap-key")
	cmd := exec.Command(binPath, "manifest", "keygen", "-o", keyPrefix, "--json") //nolint:gosec // Test helper
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	require.NoError(t, cmd.Run())

	var keygenReport struct {
		PublicKeyPath  string `json:"public_key_path"`
		PrivateKeyPath string `json:"private_key_path"`
		PublicKey      string `json:"public_key"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &keygenReport))
	assert.FileExists(t, keygenReport.PublicKeyPath)
	assert.FileExists(t, keygenReport.PrivateKeyPath)
	assert.NotEmpty(t, keygenReport.PublicKey)

	// 2. Create manifest and sign via ytg manifest sign
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	sigPath := filepath.Join(tmpDir, "manifest.yaml.sig")
	require.NoError(t, os.WriteFile(manifestPath, []byte("applications:\n  core:\n    version: 1.0.0\n"), 0600))

	cmd = exec.Command(binPath, "manifest", "sign", manifestPath, "--key", keygenReport.PrivateKeyPath, "-o", sigPath, "--json") //nolint:gosec // Test helper
	stdout.Reset()
	cmd.Stdout = &stdout
	require.NoError(t, cmd.Run())

	var signReport struct {
		ManifestPath string `json:"manifest_path"`
		Signature    string `json:"signature"`
		OutputPath   string `json:"output_path"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &signReport))
	assert.Equal(t, manifestPath, signReport.ManifestPath)
	assert.NotEmpty(t, signReport.Signature)
	assert.FileExists(t, sigPath)

	// 3. Verify signature via ytg manifest verify (Success)
	cmd = exec.Command(binPath, "manifest", "verify", manifestPath, "--key", keygenReport.PublicKeyPath, "--sig", sigPath, "--json") //nolint:gosec // Test helper
	stdout.Reset()
	cmd.Stdout = &stdout
	require.NoError(t, cmd.Run())

	var verifyReport struct {
		ManifestPath string `json:"manifest_path"`
		Verified     bool   `json:"verified"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &verifyReport))
	assert.True(t, verifyReport.Verified)

	// 4. Verify tampering detection: alter manifest content and expect verification failure
	require.NoError(t, os.WriteFile(manifestPath, []byte("applications:\n  core:\n    version: 1.0.0\n    tampered: true\n"), 0600))

	cmd = exec.Command(binPath, "manifest", "verify", manifestPath, "--key", keygenReport.PublicKeyPath, "--sig", sigPath, "--json") //nolint:gosec // Test helper
	stdout.Reset()
	cmd.Stdout = &stdout
	err := cmd.Run()
	require.Error(t, err, "verification must exit with non-zero on tampered manifest")

	var failedReport struct {
		ManifestPath string `json:"manifest_path"`
		Verified     bool   `json:"verified"`
		Error        string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &failedReport))
	assert.False(t, failedReport.Verified)
	assert.Contains(t, failedReport.Error, "tampering detected")
}
