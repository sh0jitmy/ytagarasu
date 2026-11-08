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

package secret_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/secret"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecret_KeygenEncryptDecrypt(t *testing.T) {
	t.Parallel()

	// 1. Generate Keypair
	pubKey, privKey, err := secret.GenerateKeyPair()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(pubKey, "age1"))
	assert.True(t, strings.HasPrefix(privKey, "AGE-SECRET-KEY-1"))

	// 2. Encrypt plaintext
	plaintext := "SuperSecretPassword123!"
	encString, err := secret.Encrypt(pubKey, []byte(plaintext))
	require.NoError(t, err)
	assert.True(t, secret.IsEncrypted(encString))
	assert.True(t, strings.HasPrefix(encString, "ENC[age,x25519:"))
	assert.True(t, strings.HasSuffix(encString, "]"))

	// 3. Decrypt ciphertext
	decrypted, err := secret.Decrypt(privKey, encString)
	require.NoError(t, err)
	assert.Equal(t, plaintext, string(decrypted))
}

func TestSecret_KeyFromFile(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	pubFile := filepath.Join(tmpDir, "deploy.pub")
	privFile := filepath.Join(tmpDir, "deploy.key")

	pubKey, privKey, err := secret.GenerateKeyPair()
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(pubFile, []byte(pubKey+"\n"), 0o600))
	require.NoError(t, os.WriteFile(privFile, []byte(privKey+"\n"), 0o600))

	// Encrypt using public key file path
	raw := "DatabasePassword_Secure_456"
	encString, err := secret.Encrypt(pubFile, []byte(raw))
	require.NoError(t, err)

	// Decrypt using private key file path
	decrypted, err := secret.Decrypt(privFile, encString)
	require.NoError(t, err)
	assert.Equal(t, raw, string(decrypted))
}

func TestSecret_DecryptSecretMapAndInterpolate(t *testing.T) {
	t.Parallel()

	pubKey, privKey, err := secret.GenerateKeyPair()
	require.NoError(t, err)

	rawPass := "ProductionPostgresP@ss!"
	encPass, err := secret.Encrypt(pubKey, []byte(rawPass))
	require.NoError(t, err)

	secrets := map[string]string{
		"db_password": encPass,
		"plain_key":   "unencrypted-value",
	}

	decrypted, err := secret.DecryptSecretMap(secrets, privKey)
	require.NoError(t, err)
	assert.Equal(t, rawPass, decrypted["db_password"])
	assert.Equal(t, "unencrypted-value", decrypted["plain_key"])

	// Test template interpolation
	inputTemplate := "postgres://admin:{{.Secrets.db_password}}@localhost:5432/mydb?auth={{.Secrets.plain_key}}"
	resolved := secret.InterpolateSecrets(inputTemplate, decrypted)
	assert.Equal(t, "postgres://admin:ProductionPostgresP@ss!@localhost:5432/mydb?auth=unencrypted-value", resolved)
}

func TestSecret_Decrypt_CorruptedOrWrongKey(t *testing.T) {
	t.Parallel()

	pubKey1, _, err := secret.GenerateKeyPair()
	require.NoError(t, err)
	_, privKey2, err := secret.GenerateKeyPair()
	require.NoError(t, err)

	encString, err := secret.Encrypt(pubKey1, []byte("TestPayload"))
	require.NoError(t, err)

	// Attempt decrypt with different private key
	_, err = secret.Decrypt(privKey2, encString)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decrypt age payload")

	// Attempt decrypt with invalid formatted string
	_, err = secret.Decrypt(privKey2, "not-encrypted")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match ENC[")
}
