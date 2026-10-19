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

	"github.com/sh0jitmy/ytagarasu/internal/manifest"
)

// ManifestIntegrityInfo represents cryptographic authenticity and verification status.
type ManifestIntegrityInfo struct {
	Verified          bool   `json:"verified"`
	Status            string `json:"status"` // VERIFIED, TAMPERED, UNSIGNED, KEY_NOT_CONFIGURED, MANIFEST_NOT_FOUND
	SignaturePath     string `json:"signature_path,omitempty"`
	SignatureFound    bool   `json:"signature_found"`
	KeyFingerprint    string `json:"key_fingerprint,omitempty"`
	VerificationError string `json:"verification_error,omitempty"`
}

// CheckManifestIntegrity evaluates the cryptographic Ed25519 signature of a manifest file.
func CheckManifestIntegrity(manifestPath string) ManifestIntegrityInfo {
	info := ManifestIntegrityInfo{
		Status: "UNSIGNED",
	}

	if manifestPath == "" {
		manifestPath = "manifest.yaml"
	}

	cleanManifest := filepath.Clean(manifestPath)
	if _, err := os.Stat(cleanManifest); err != nil {
		info.Status = "MANIFEST_NOT_FOUND"
		info.VerificationError = err.Error()
		return info
	}

	// Look for detached signature (.sig)
	sigPath := cleanManifest + ".sig"
	if _, err := os.Stat(sigPath); err != nil {
		info.SignatureFound = false
		info.Status = "UNSIGNED"
		return info
	}

	info.SignatureFound = true
	info.SignaturePath = sigPath

	// Look for public key
	pubKeyPath := findPublicKey(cleanManifest)
	if pubKeyPath == "" {
		info.Status = "KEY_NOT_CONFIGURED"
		info.VerificationError = "detached signature found but no trusted public key (.pub) configured"
		return info
	}

	pubContent, err := os.ReadFile(filepath.Clean(pubKeyPath)) //nolint:gosec // Trusted key reading
	if err != nil {
		info.Status = "KEY_READ_ERROR"
		info.VerificationError = err.Error()
		return info
	}

	pubHex := strings.TrimSpace(string(pubContent))
	if len(pubHex) >= 16 {
		info.KeyFingerprint = pubHex[:16]
	} else {
		info.KeyFingerprint = pubHex
	}

	if verifyErr := manifest.VerifyFile(pubHex, cleanManifest, sigPath); verifyErr != nil {
		info.Verified = false
		info.Status = "TAMPERED"
		info.VerificationError = verifyErr.Error()
		return info
	}

	info.Verified = true
	info.Status = "VERIFIED"
	return info
}

// findPublicKey searches for candidate public key locations.
func findPublicKey(manifestPath string) string {
	if envKey := os.Getenv("YTAGARASU_PUBLIC_KEY"); envKey != "" {
		cleanEnv := filepath.Clean(envKey)
		if _, err := os.Stat(cleanEnv); err == nil { //nolint:gosec // Trusted key lookup
			return cleanEnv
		}

	}

	dir := filepath.Dir(manifestPath)
	candidates := []string{
		manifestPath + ".pub",
		filepath.Join(dir, "ytagarasu-manifest.pub"),
		filepath.Join(dir, "trusted.pub"),
		"ytagarasu-manifest.pub",
		"trusted.pub",
	}

	for _, c := range candidates {
		if _, err := os.Stat(filepath.Clean(c)); err == nil {
			return c
		}
	}
	return ""
}
