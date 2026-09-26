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
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sh0jitmy/ytagarasu/internal/manifest"
)

// VerificationResult contains the inspection findings of a bundle archive.
type VerificationResult struct {
	Valid          bool               `json:"valid"`
	Manifest       *manifest.Manifest `json:"manifest,omitempty"`
	SignatureValid bool               `json:"signatureValid"`
	ChecksumsValid bool               `json:"checksumsValid"`
	FileCount      int                `json:"fileCount"`
	Files          map[string][]byte  `json:"-"`
	Errors         []string           `json:"errors,omitempty"`
}

// VerifyBundle unpacks in-memory, checks all SHA-256 checksums, and verifies the Ed25519 signature.
func VerifyBundle(bundlePath string, expectedPubKey ed25519.PublicKey) (*VerificationResult, error) {
	cleanPath := filepath.Clean(bundlePath)
	f, err := os.Open(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open bundle: %w", err)
	}
	defer func() { _ = f.Close() }()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("failed to open gzip stream: %w", err)
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)

	files := make(map[string][]byte)
	for {
		header, nextErr := tr.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, fmt.Errorf("error reading tar stream: %w", nextErr)
		}

		if header.Typeflag == tar.TypeReg {
			data, readErr := io.ReadAll(tr)
			if readErr != nil {
				return nil, fmt.Errorf("failed to read tar entry %s: %w", header.Name, readErr)
			}
			files[filepath.ToSlash(header.Name)] = data
		}
	}

	res := &VerificationResult{
		Valid:     true,
		FileCount: len(files),
		Files:     files,
		Errors:    make([]string, 0),
	}

	// 1. Verify manifest.yaml
	manifestBytes, ok := files["manifest.yaml"]
	if !ok {
		res.Valid = false
		res.Errors = append(res.Errors, "manifest.yaml missing in bundle root")
		return res, nil
	}

	m, err := manifest.Parse(manifestBytes)
	if err != nil {
		res.Valid = false
		res.Errors = append(res.Errors, fmt.Sprintf("invalid manifest.yaml: %v", err))
	} else {
		res.Manifest = m
	}

	// 2. Verify signature
	sigBytes, hasSig := files["signatures/manifest.sig"]
	if !hasSig {
		res.Valid = false
		res.Errors = append(res.Errors, "signatures/manifest.sig is missing")
	} else {
		pubKey := expectedPubKey
		if pubKey == nil {
			// Fallback: check if public key is packaged inside signatures/public.key
			if pubKeyData, hasPub := files["signatures/public.key"]; hasPub {
				decodedPub, err := hex.DecodeString(strings.TrimSpace(string(pubKeyData)))
				if err == nil && len(decodedPub) == ed25519.PublicKeySize {
					pubKey = ed25519.PublicKey(decodedPub)
				}
			}
		}

		if pubKey == nil {
			res.Valid = false
			res.Errors = append(res.Errors, "no public key available to verify signature")
		} else if VerifySignature(pubKey, manifestBytes, sigBytes) {
			res.SignatureValid = true
		} else {
			res.Valid = false
			res.Errors = append(res.Errors, "Ed25519 digital signature mismatch (tampering detected)")
		}
	}

	// 3. Verify checksums file
	checksumsData, hasChecksums := files["checksums"]
	if !hasChecksums {
		res.Valid = false
		res.Errors = append(res.Errors, "checksums file missing in bundle")
	} else {
		res.ChecksumsValid = true
		scanner := bufio.NewScanner(strings.NewReader(string(checksumsData)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			expectedHash := parts[0]
			relPath := filepath.ToSlash(parts[1])

			data, exists := files[relPath]
			if !exists {
				res.Valid = false
				res.ChecksumsValid = false
				res.Errors = append(res.Errors, fmt.Sprintf("file %s listed in checksums but missing from archive", relPath))
				continue
			}

			h := sha256.Sum256(data)
			actualHash := hex.EncodeToString(h[:])
			if actualHash != expectedHash {
				res.Valid = false
				res.ChecksumsValid = false
				res.Errors = append(res.Errors, fmt.Sprintf("checksum mismatch for %s: expected %s, got %s", relPath, expectedHash, actualHash))
			}
		}
	}

	if len(res.Errors) > 0 {
		res.Valid = false
	}

	return res, nil
}
