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

package bundle

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// GenerateKeyPair generates a new Ed25519 keypair.
func GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate ed25519 keypair: %w", err)
	}
	return pub, priv, nil
}

// SignData creates an Ed25519 signature of the given data.
func SignData(priv ed25519.PrivateKey, data []byte) []byte {
	return ed25519.Sign(priv, data)
}

// VerifySignature verifies the signature of the data using the public key.
func VerifySignature(pub ed25519.PublicKey, data, sig []byte) bool {
	if len(pub) != ed25519.PublicKeySize {
		return false
	}
	return ed25519.Verify(pub, data, sig)
}

// SaveKeyToFile writes raw or hex-encoded key bytes to a protected file.
func SaveKeyToFile(path string, keyBytes []byte, perm os.FileMode) error {
	cleanPath := filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0750); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	}
	return os.WriteFile(cleanPath, []byte(hex.EncodeToString(keyBytes)), perm) //nolint:gosec
}

// LoadKeyFromFile reads a hex-encoded key from disk.
func LoadKeyFromFile(path string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}
	trimmed := string(data)
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '\n' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	decoded, err := hex.DecodeString(trimmed)
	if err != nil {
		// If not hex, return raw bytes
		return data, nil
	}
	return decoded, nil
}

// LoadPrivateKeyFromFile loads an Ed25519 private key from disk.
func LoadPrivateKeyFromFile(path string) (ed25519.PrivateKey, error) {
	bytes, err := LoadKeyFromFile(path)
	if err != nil {
		return nil, err
	}
	if len(bytes) == ed25519.PrivateKeySize {
		return ed25519.PrivateKey(bytes), nil
	}
	if len(bytes) == ed25519.SeedSize {
		return ed25519.NewKeyFromSeed(bytes), nil
	}
	return nil, errors.New("invalid Ed25519 private key size")
}

// LoadPublicKeyFromFile loads an Ed25519 public key from disk.
func LoadPublicKeyFromFile(path string) (ed25519.PublicKey, error) {
	bytes, err := LoadKeyFromFile(path)
	if err != nil {
		return nil, err
	}
	if len(bytes) != ed25519.PublicKeySize {
		return nil, errors.New("invalid Ed25519 public key size")
	}
	return ed25519.PublicKey(bytes), nil
}
