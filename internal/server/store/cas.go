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

package store

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CAS provides Content-Addressable Storage for deduplicating artifacts using SHA-256.
type CAS struct {
	rootDir string
}

// NewCAS initializes a CAS repository under rootDir.
func NewCAS(rootDir string) (*CAS, error) {
	cleanDir := filepath.Clean(rootDir)
	if err := os.MkdirAll(cleanDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create CAS root directory: %w", err)
	}
	return &CAS{rootDir: cleanDir}, nil
}

// BlobPath returns the absolute filesystem path for a given SHA-256 hash.
func (c *CAS) BlobPath(hash string) string {
	if len(hash) < 2 {
		return filepath.Join(c.rootDir, "blobs", "sha256", hash)
	}
	return filepath.Join(c.rootDir, "blobs", "sha256", hash[:2], hash)
}

// Has checks whether a blob with the specified hash exists in the storage.
func (c *CAS) Has(hash string) bool {
	p := c.BlobPath(hash)
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// Put writes contents from r into the CAS, calculating the SHA-256 hash.
// If an identical blob already exists, it is deduplicated without overwriting.
func (c *CAS) Put(r io.Reader) (string, int64, error) {
	tmpDir := filepath.Join(c.rootDir, "tmp")
	if err := os.MkdirAll(tmpDir, 0750); err != nil {
		return "", 0, fmt.Errorf("failed to create temporary CAS directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(tmpDir, "blob-*")
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	hasher := sha256.New()
	mw := io.MultiWriter(tmpFile, hasher)

	size, copyErr := io.Copy(mw, r)
	if copyErr != nil {
		_ = tmpFile.Close()
		return "", 0, fmt.Errorf("failed writing data to temp CAS file: %w", copyErr)
	}

	if syncErr := tmpFile.Sync(); syncErr != nil {
		_ = tmpFile.Close()
		return "", 0, fmt.Errorf("failed to sync temp CAS file: %w", syncErr)
	}
	if closeErr := tmpFile.Close(); closeErr != nil {
		return "", 0, fmt.Errorf("failed to close temp CAS file: %w", closeErr)
	}

	hash := hex.EncodeToString(hasher.Sum(nil))
	targetPath := c.BlobPath(hash)

	// Deduplication: if target already exists, skip rename
	if c.Has(hash) {
		return hash, size, nil
	}

	if dirErr := os.MkdirAll(filepath.Dir(targetPath), 0750); dirErr != nil {
		return "", 0, fmt.Errorf("failed to create blob directory: %w", dirErr)
	}

	if renErr := os.Rename(tmpPath, targetPath); renErr != nil {
		// If another concurrent goroutine already renamed it, return success
		if c.Has(hash) {
			return hash, size, nil
		}
		return "", 0, fmt.Errorf("failed to move blob to target path: %w", renErr)
	}

	return hash, size, nil
}

// Get opens a reader for the blob identified by hash.
func (c *CAS) Get(hash string) (io.ReadCloser, error) {
	targetPath := c.BlobPath(hash)
	f, err := os.Open(filepath.Clean(targetPath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("blob not found: %s", hash)
		}
		return nil, fmt.Errorf("failed to open blob: %w", err)
	}
	return f, nil
}

// Link creates a hardlink from the CAS blob to destPath.
// If hardlinking fails (e.g. cross-device link), it falls back to a file copy.
func (c *CAS) Link(hash string, destPath string) error {
	srcPath := c.BlobPath(hash)
	if !c.Has(hash) {
		return fmt.Errorf("cannot link non-existent blob: %s", hash)
	}

	cleanDest := filepath.Clean(destPath)
	if err := os.MkdirAll(filepath.Dir(cleanDest), 0750); err != nil {
		return fmt.Errorf("failed to create dest parent directory: %w", err)
	}

	// Remove target if it already exists to allow atomic refresh
	_ = os.Remove(cleanDest)

	// Try hardlink first
	if linkErr := os.Link(srcPath, cleanDest); linkErr == nil {
		return nil
	}

	// Fallback to copy
	srcFile, err := os.Open(filepath.Clean(srcPath)) //nolint:gosec // Path is controlled by CAS BlobPath
	if err != nil {
		return fmt.Errorf("failed to open source blob for copy: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	destFile, err := os.OpenFile(cleanDest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed to create dest file: %w", err)
	}
	defer func() { _ = destFile.Close() }()

	if _, err := io.Copy(destFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy blob to dest: %w", err)
	}

	return nil
}
