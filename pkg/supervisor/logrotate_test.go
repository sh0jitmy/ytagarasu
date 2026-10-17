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

package supervisor

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRotatingWriter_BasicAndRotation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	// MaxSize: 500 bytes, MaxBackups: 2
	cfg := RotatingLogConfig{
		Filename:   logFile,
		MaxSizeMB:  1, // We will manually trigger or set a small maxSize for test
		MaxBackups: 2,
		Compress:   true,
	}

	w, err := NewRotatingWriter(cfg)
	if err != nil {
		t.Fatalf("unexpected error creating writer: %v", err)
	}
	defer func() {
		_ = w.Close()
	}()

	// Override maxSize directly for testing small byte increments
	w.maxSize = 200

	payload := []byte("12345678901234567890\n") // 21 bytes

	// Write 15 times = 315 bytes (should trigger rotation)
	for i := 0; i < 15; i++ {
		if _, writeErr := w.Write(payload); writeErr != nil {
			t.Fatalf("write failed at iteration %d: %v", i, writeErr)
		}
	}

	if syncErr := w.Sync(); syncErr != nil {
		t.Fatalf("sync failed: %v", syncErr)
	}

	// Verify active file exists
	if _, statErr := os.Stat(logFile); statErr != nil {
		t.Fatalf("active log file missing: %v", statErr)
	}

	// Allow goroutine compression to settle
	time.Sleep(100 * time.Millisecond)

	// List directory files
	entries, readErr := os.ReadDir(tmpDir)
	if readErr != nil {
		t.Fatalf("failed to read dir: %v", readErr)
	}

	var foundBackup bool
	for _, e := range entries {
		if e.Name() != "test.log" {
			foundBackup = true
			break
		}
	}

	if !foundBackup {
		t.Errorf("expected rotated backup file in %s, got: %v", tmpDir, entries)
	}
}

func TestRotatingWriter_Compression(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "compress.log")

	cfg := RotatingLogConfig{
		Filename:   logFile,
		MaxSizeMB:  1,
		MaxBackups: 3,
		Compress:   true,
	}

	w, err := NewRotatingWriter(cfg)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	defer func() {
		_ = w.Close()
	}()

	w.maxSize = 100

	data := bytes.Repeat([]byte("hello onpremise zero-dependency logging!\n"), 5)
	if _, writeErr := w.Write(data); writeErr != nil {
		t.Fatalf("write failed: %v", writeErr)
	}

	// Trigger explicit rotation
	if rotErr := w.Rotate(); rotErr != nil {
		t.Fatalf("rotate failed: %v", rotErr)
	}

	// Allow compression goroutine to complete
	time.Sleep(200 * time.Millisecond)

	entries, readErr := os.ReadDir(tmpDir)
	if readErr != nil {
		t.Fatalf("readdir failed: %v", readErr)
	}

	var gzFound string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".gz" {
			gzFound = filepath.Join(tmpDir, e.Name())
			break
		}
	}

	if gzFound == "" {
		t.Fatalf("expected .gz compressed file, found: %v", entries)
	}

	// Verify decompression
	// #nosec G304 -- Test reads temporary file created in test dir
	gzF, openErr := os.Open(filepath.Clean(gzFound))
	if openErr != nil {
		t.Fatalf("failed to open gz: %v", openErr)
	}
	defer func() {
		_ = gzF.Close()
	}()

	zr, zrErr := gzip.NewReader(gzF)
	if zrErr != nil {
		t.Fatalf("failed to create gzip reader: %v", zrErr)
	}
	defer func() {
		_ = zr.Close()
	}()

	decompressed, copyErr := io.ReadAll(zr)
	if copyErr != nil {
		t.Fatalf("failed to read gzip: %v", copyErr)
	}

	if !bytes.Equal(decompressed, data) {
		t.Errorf("decompressed content mismatch: got %d bytes, want %d bytes", len(decompressed), len(data))
	}
}

func TestRotatingWriter_ConcurrentWrites(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "concurrent.log")

	cfg := RotatingLogConfig{
		Filename:   logFile,
		MaxSizeMB:  1,
		MaxBackups: 5,
		Compress:   false,
	}

	w, err := NewRotatingWriter(cfg)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	defer func() {
		_ = w.Close()
	}()

	w.maxSize = 1000

	var wg sync.WaitGroup
	workers := 10
	iterations := 50

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				msg := fmt.Sprintf("worker-%02d: message-%04d\n", workerID, j)
				if _, writeErr := w.Write([]byte(msg)); writeErr != nil {
					t.Errorf("write error: %v", writeErr)
					return
				}
			}
		}(i)
	}

	wg.Wait()

	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("close failed: %v", closeErr)
	}
}
