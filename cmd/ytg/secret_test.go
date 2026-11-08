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

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecretCLI_KeygenEncryptDecryptFlow(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "deploy.key")
	pubFile := filepath.Join(tmpDir, "deploy.pub")

	// 1. Generate Keypair
	pubKey, privKey, err := secret.GenerateKeyPair()
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(keyFile, []byte(privKey+"\n"), 0o600))
	require.NoError(t, os.WriteFile(pubFile, []byte(pubKey+"\n"), 0o600))

	// 2. Encrypt value
	plaintext := "DatabaseAdminPassw0rd!"
	encString, err := secret.Encrypt(pubFile, []byte(plaintext))
	require.NoError(t, err)
	assert.True(t, secret.IsEncrypted(encString))

	// 3. Decrypt value
	decrypted, err := secret.Decrypt(keyFile, encString)
	require.NoError(t, err)
	assert.Equal(t, plaintext, string(decrypted))
}

func TestSecretCLI_ViewManifestSecrets(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	keyFile := filepath.Join(tmpDir, "deploy.key")
	pubFile := filepath.Join(tmpDir, "deploy.pub")

	pubKey, privKey, err := secret.GenerateKeyPair()
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(keyFile, []byte(privKey+"\n"), 0o600))
	require.NoError(t, os.WriteFile(pubFile, []byte(pubKey+"\n"), 0o600))

	encDb, err := secret.Encrypt(pubFile, []byte("SuperSecretDBPass"))
	require.NoError(t, err)
	encJwt, err := secret.Encrypt(pubFile, []byte("JWT_SIGNING_KEY_XYZ"))
	require.NoError(t, err)

	manifestYaml := strings.ReplaceAll(`version: "1.0"
bundleVersion: "1.0.0"
release: "rel-1"
author: "admin"
targets:
  - os: "linux"
    arch: "amd64"
applications:
  - name: "sample-app"
    version: "1.0.0"
secrets:
  db_password: "ENC_DB_VAL"
  jwt_secret: "ENC_JWT_VAL"
rollback:
  autoOnFailure: true
  strategy: "immediate"
`, "ENC_DB_VAL", encDb)
	manifestYaml = strings.ReplaceAll(manifestYaml, "ENC_JWT_VAL", encJwt)

	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestYaml), 0o600))

	// Test secret map decryption directly
	secrets := map[string]string{
		"db_password": encDb,
		"jwt_secret":  encJwt,
	}

	decryptedMap, err := secret.DecryptSecretMap(secrets, keyFile)
	require.NoError(t, err)
	assert.Equal(t, "SuperSecretDBPass", decryptedMap["db_password"])
	assert.Equal(t, "JWT_SIGNING_KEY_XYZ", decryptedMap["jwt_secret"])
}
