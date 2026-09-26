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

package pkgengine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Downloader handles concurrent, checksum-verified downloads of package artifacts.
type Downloader struct {
	BaseURL     string
	OutputDir   string
	Concurrency int
	HTTPClient  *http.Client
}

// DownloadProgress reports progress for a single file download.
type DownloadProgress struct {
	PackageName string
	BytesCopied int64
	TotalBytes  int64
	Done        bool
	Err         error
}

// NewDownloader creates an initialized Downloader.
func NewDownloader(baseURL, outputDir string, concurrency int) *Downloader {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &Downloader{
		BaseURL:     baseURL,
		OutputDir:   outputDir,
		Concurrency: concurrency,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// DownloadAll concurrently downloads and verifies all packages in the resolution result.
func (d *Downloader) DownloadAll(ctx context.Context, pkgs []PackageMetadata) error {
	if err := os.MkdirAll(d.OutputDir, 0750); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	taskCh := make(chan PackageMetadata, len(pkgs))
	for _, p := range pkgs {
		taskCh <- p
	}
	close(taskCh)

	errCh := make(chan error, len(pkgs))
	var wg sync.WaitGroup

	for i := 0; i < d.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for pkg := range taskCh {
				select {
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				default:
					if err := d.downloadAndVerify(ctx, pkg); err != nil {
						errCh <- fmt.Errorf("failed to download package %s: %w", pkg.Name, err)
						return
					}
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *Downloader) downloadAndVerify(ctx context.Context, pkg PackageMetadata) error {
	destPath := filepath.Join(d.OutputDir, filepath.Base(pkg.Filename))

	// If file already exists with identical SHA-256, skip downloading (idempotent / CAS)
	if fi, err := os.Stat(destPath); err == nil && !fi.IsDir() && pkg.SHA256 != "" {
		if verified, _ := verifyFileChecksum(destPath, pkg.SHA256); verified {
			return nil
		}
	}

	reqURL := pkg.Filename
	if d.BaseURL != "" && !filepath.IsAbs(pkg.Filename) {
		reqURL = fmt.Sprintf("%s/%s", stringsTrimRight(d.BaseURL, "/"), stringsTrimLeft(pkg.Filename, "/"))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("network request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status %d for %s", resp.StatusCode, reqURL)
	}

	tmpFile := destPath + ".tmp"
	f, err := os.OpenFile(filepath.Clean(tmpFile), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}

	hasher := sha256.New()
	multiWriter := io.MultiWriter(f, hasher)

	_, err = io.Copy(multiWriter, resp.Body)
	_ = f.Close()
	if err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed while writing body: %w", err)
	}

	// Verify SHA-256
	if pkg.SHA256 != "" {
		calculatedHash := hex.EncodeToString(hasher.Sum(nil))
		if calculatedHash != pkg.SHA256 {
			_ = os.Remove(tmpFile)
			return fmt.Errorf("SHA-256 checksum mismatch for %s: expected %s, got %s",
				pkg.Name, pkg.SHA256, calculatedHash)
		}
	}

	// Atomic rename to final destination
	if err := os.Rename(tmpFile, destPath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to rename temp file to %s: %w", destPath, err)
	}

	return nil
}

func verifyFileChecksum(path, expectedHash string) (bool, error) {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}

	return hex.EncodeToString(h.Sum(nil)) == expectedHash, nil
}

func stringsTrimLeft(s, cutset string) string {
	for len(s) > 0 && len(cutset) > 0 && s[:1] == cutset {
		s = s[1:]
	}
	return s
}

func stringsTrimRight(s, cutset string) string {
	for len(s) > 0 && len(cutset) > 0 && s[len(s)-1:] == cutset {
		s = s[:len(s)-1]
	}
	return s
}
