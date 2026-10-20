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

package fsutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileSHA256 computes the hexadecimal SHA-256 checksum of the file at the given path.
// It uses stream reading via io.Copy to maintain minimal memory footprint regardless of file size.
func FileSHA256(path string) (string, error) {
	cleanPath := filepath.Clean(path)
	f, err := os.Open(cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file for SHA-256 calculation (%s): %w", path, err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to compute SHA-256 checksum for %s: %w", path, err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// VerifyFileSHA256 compares the computed SHA-256 checksum of a file with an expected hash.
// The comparison is case-insensitive and trims surrounding whitespace.
func VerifyFileSHA256(path, expectedHash string) (bool, error) {
	actual, err := FileSHA256(path)
	if err != nil {
		return false, err
	}

	cleanedExpected := strings.TrimSpace(expectedHash)
	return strings.EqualFold(actual, cleanedExpected), nil
}
