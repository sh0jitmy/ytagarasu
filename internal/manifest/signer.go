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

package manifest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	// ErrSignatureMismatch is returned when Ed25519 cryptographic signature verification fails.
	ErrSignatureMismatch = errors.New("cryptographic signature mismatch (tampering detected)")

	// ErrInvalidKey is returned when the key format or length does not meet Ed25519 specifications.
	ErrInvalidKey = errors.New("invalid cryptographic key specification")
)

// GenerateEd25519KeyPair generates a new cryptographically secure Ed25519 public and private key pair encoded in hexadecimal.
func GenerateEd25519KeyPair() (pubHex string, privHex string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate ed25519 keypair: %w", err)
	}
	return hex.EncodeToString(pub), hex.EncodeToString(priv), nil
}

// SaveKeyPair writes the Ed25519 public and private keys to disk using the given prefix.
// The private key is saved with strict 0600 permissions.
func SaveKeyPair(prefix string) (pubPath string, privPath string, err error) {
	pubHex, privHex, err := GenerateEd25519KeyPair()
	if err != nil {
		return "", "", err
	}

	cleanPrefix := filepath.Clean(prefix)
	dir := filepath.Dir(cleanPrefix)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return "", "", fmt.Errorf("failed to create directory for keypair: %w", err)
		}
	}

	pubPath = cleanPrefix + ".pub"
	privPath = cleanPrefix + ".key"

	if err := os.WriteFile(pubPath, []byte(pubHex+"\n"), 0644); err != nil { //nolint:gosec // Public key is world-readable
		return "", "", fmt.Errorf("failed to write public key to '%s': %w", pubPath, err)
	}

	if err := os.WriteFile(privPath, []byte(privHex+"\n"), 0600); err != nil {
		return "", "", fmt.Errorf("failed to write private key to '%s': %w", privPath, err)
	}

	return pubPath, privPath, nil
}

// SignData computes an Ed25519 signature over arbitrary bytes using a hexadecimal private key.
func SignData(privateKeyHex string, data []byte) (string, error) {
	privBytes, err := hex.DecodeString(strings.TrimSpace(privateKeyHex))
	if err != nil {
		return "", fmt.Errorf("%w: failed to decode private key hex: %v", ErrInvalidKey, err)
	}

	var privKey ed25519.PrivateKey
	switch len(privBytes) {
	case ed25519.SeedSize: // 32 bytes seed
		privKey = ed25519.NewKeyFromSeed(privBytes)
	case ed25519.PrivateKeySize: // 64 bytes private key
		privKey = ed25519.PrivateKey(privBytes)
	default:
		return "", fmt.Errorf("%w: expected 32 or 64 bytes private key, got %d", ErrInvalidKey, len(privBytes))
	}

	sig := ed25519.Sign(privKey, data)
	return hex.EncodeToString(sig), nil
}

// VerifySignature validates an Ed25519 signature over arbitrary bytes using a hexadecimal public key.
func VerifySignature(publicKeyHex string, data []byte, signatureHex string) (bool, error) {
	pubBytes, err := hex.DecodeString(strings.TrimSpace(publicKeyHex))
	if err != nil {
		return false, fmt.Errorf("%w: failed to decode public key hex: %v", ErrInvalidKey, err)
	}
	if len(pubBytes) != ed25519.PublicKeySize {
		return false, fmt.Errorf("%w: expected 32 bytes public key, got %d", ErrInvalidKey, len(pubBytes))
	}

	sigBytes, err := hex.DecodeString(strings.TrimSpace(signatureHex))
	if err != nil {
		return false, fmt.Errorf("invalid signature hex encoding: %w", err)
	}
	if len(sigBytes) != ed25519.SignatureSize {
		return false, fmt.Errorf("invalid signature length: expected 64 bytes, got %d", len(sigBytes))
	}

	return ed25519.Verify(ed25519.PublicKey(pubBytes), data, sigBytes), nil
}

// readKeyOrHex reads key content from a file path if the file exists, or returns the string trimmed as hex.
func readKeyOrHex(pathOrHex string) (string, error) {
	pathOrHex = strings.TrimSpace(pathOrHex)
	if pathOrHex == "" {
		return "", errors.New("empty key provided")
	}

	cleanPath := filepath.Clean(pathOrHex)
	if data, err := os.ReadFile(cleanPath); err == nil {
		return strings.TrimSpace(string(data)), nil
	}

	return pathOrHex, nil
}

// SignFile computes an Ed25519 signature of the given target file using a private key file or hex string.
func SignFile(privateKeyPathOrHex string, targetFilePath string) (string, error) {
	privHex, err := readKeyOrHex(privateKeyPathOrHex)
	if err != nil {
		return "", fmt.Errorf("failed to read private key: %w", err)
	}

	cleanTarget := filepath.Clean(targetFilePath)
	data, err := os.ReadFile(cleanTarget)
	if err != nil {
		return "", fmt.Errorf("failed to read target file for signing: %w", err)
	}

	return SignData(privHex, data)
}

// VerifyFile verifies the cryptographic Ed25519 signature of the given target file.
// Both public key and signature can be supplied as a file path or direct hexadecimal string.
func VerifyFile(publicKeyPathOrHex string, targetFilePath string, signaturePathOrHex string) error {
	pubHex, err := readKeyOrHex(publicKeyPathOrHex)
	if err != nil {
		return fmt.Errorf("failed to read public key: %w", err)
	}

	sigHex, err := readKeyOrHex(signaturePathOrHex)
	if err != nil {
		return fmt.Errorf("failed to read signature: %w", err)
	}

	cleanTarget := filepath.Clean(targetFilePath)
	data, err := os.ReadFile(cleanTarget)
	if err != nil {
		return fmt.Errorf("failed to read target file for verification: %w", err)
	}

	valid, err := VerifySignature(pubHex, data, sigHex)
	if err != nil {
		return err
	}
	if !valid {
		return ErrSignatureMismatch
	}

	return nil
}
