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

package secret

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"filippo.io/age"
)

// Prefix and suffix markers for inline encrypted strings in manifests.
const (
	EncPrefix = "ENC[age,x25519:"
	EncSuffix = "]"
)

var (
	encRegex      = regexp.MustCompile(`ENC\[age,x25519:([a-zA-Z0-9+/=_-]+)\]`)
	templateRegex = regexp.MustCompile(`\{\{\.Secrets\.([a-zA-Z0-9_-]+)\}\}`)
)

// GenerateKeyPair generates a new age X25519 identity and its corresponding public recipient key.
func GenerateKeyPair() (publicKey string, privateKey string, err error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate age x25519 identity: %w", err)
	}

	return identity.Recipient().String(), identity.String(), nil
}

// Encrypt encrypts raw plaintext using an age X25519 recipient public key and returns the inline ENC[...] string.
func Encrypt(recipientKey string, plaintext []byte) (string, error) {
	recipientKey = strings.TrimSpace(recipientKey)
	// Support reading from key file if recipientKey looks like a path
	if _, err := os.Stat(recipientKey); err == nil {
		//nolint:gosec // Key file path provided via explicit key reference
		data, readErr := os.ReadFile(recipientKey)
		if readErr == nil {
			recipientKey = strings.TrimSpace(string(data))
		}
	}

	recipient, err := age.ParseX25519Recipient(recipientKey)
	if err != nil {
		return "", fmt.Errorf("invalid age recipient key: %w", err)
	}

	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return "", fmt.Errorf("failed to initialize age encryption: %w", err)
	}

	if _, err := w.Write(plaintext); err != nil {
		return "", fmt.Errorf("failed to write age payload: %w", err)
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("failed to finalize age payload: %w", err)
	}

	encoded := base64.RawURLEncoding.EncodeToString(buf.Bytes())
	return fmt.Sprintf("%s%s%s", EncPrefix, encoded, EncSuffix), nil
}

// Decrypt decrypts an inline ENC[...] string using an age X25519 private key.
func Decrypt(identityKey string, encString string) ([]byte, error) {
	identityKey = strings.TrimSpace(identityKey)
	// Support reading from key file if identityKey looks like a path
	if _, err := os.Stat(identityKey); err == nil {
		//nolint:gosec // Key file path provided via explicit key reference
		data, readErr := os.ReadFile(identityKey)
		if readErr == nil {
			identityKey = strings.TrimSpace(string(data))
		}
	}

	identity, err := age.ParseX25519Identity(identityKey)
	if err != nil {
		return nil, fmt.Errorf("invalid age identity private key: %w", err)
	}

	matches := encRegex.FindStringSubmatch(strings.TrimSpace(encString))
	if len(matches) < 2 {
		return nil, fmt.Errorf("input string does not match ENC[age,x25519:<base64>] format")
	}

	rawCiphertext, err := base64.RawURLEncoding.DecodeString(matches[1])
	if err != nil {
		// Fallback to standard base64 if url-safe decode fails
		var stdErr error
		rawCiphertext, stdErr = base64.StdEncoding.DecodeString(matches[1])
		if stdErr != nil {
			return nil, fmt.Errorf("failed to base64 decode age payload: %w", err)
		}
	}

	r, err := age.Decrypt(bytes.NewReader(rawCiphertext), identity)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt age payload (wrong key or corrupted data): %w", err)
	}

	var out bytes.Buffer
	if _, err := io.Copy(&out, r); err != nil {
		return nil, fmt.Errorf("failed to read decrypted age payload: %w", err)
	}

	return out.Bytes(), nil
}

// IsEncrypted returns true if the input string contains an inline age encryption marker.
func IsEncrypted(val string) bool {
	return encRegex.MatchString(val)
}

// DecryptSecretMap decrypts all encrypted values in a key-value secret map into memory.
func DecryptSecretMap(secrets map[string]string, privateKey string) (map[string]string, error) {
	decrypted := make(map[string]string, len(secrets))
	for k, v := range secrets {
		if IsEncrypted(v) {
			val, err := Decrypt(privateKey, v)
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt secret key '%s': %w", k, err)
			}
			decrypted[k] = string(val)
		} else {
			decrypted[k] = v
		}
	}
	return decrypted, nil
}

// InterpolateSecrets substitutes template placeholders {{.Secrets.<name>}} with decrypted values.
func InterpolateSecrets(input string, decrypted map[string]string) string {
	return templateRegex.ReplaceAllStringFunc(input, func(m string) string {
		submatches := templateRegex.FindStringSubmatch(m)
		if len(submatches) >= 2 {
			secName := submatches[1]
			if val, ok := decrypted[secName]; ok {
				return val
			}
		}
		return m
	})
}
