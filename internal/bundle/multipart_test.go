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
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

//nolint:paralleltest
func TestParseSize(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		hasErr   bool
	}{
		{"25MB", 25 * 1024 * 1024, false},
		{"25M", 25 * 1024 * 1024, false},
		{"100KB", 100 * 1024, false},
		{"100K", 100 * 1024, false},
		{"1GB", 1024 * 1024 * 1024, false},
		{"1G", 1024 * 1024 * 1024, false},
		{"512B", 512, false},
		{"1024", 1024, false},
		{"", 0, true},
		{"-5MB", 0, true},
		{"invalid", 0, true},
	}

	for _, tc := range tests {
		val, err := ParseSize(tc.input)
		if tc.hasErr {
			if err == nil {
				t.Errorf("ParseSize(%q) expected error, got nil", tc.input)
			}
		} else {
			if err != nil {
				t.Errorf("ParseSize(%q) unexpected error: %v", tc.input, err)
			}
			if val != tc.expected {
				t.Errorf("ParseSize(%q) expected %d, got %d", tc.input, tc.expected, val)
			}
		}
	}
}

//nolint:paralleltest
func TestMultiPart_SplitAndStreamingReader(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Create a dummy file of 2.5 MB
	dummyData := make([]byte, int(2.5*1024*1024))
	for i := range dummyData {
		dummyData[i] = byte(i % 256)
	}
	srcPath := filepath.Join(tmpDir, "source.bin")
	if err := os.WriteFile(srcPath, dummyData, 0600); err != nil {
		t.Fatalf("failed writing source file: %v", err)
	}

	// 2. Split with chunk size of 1 MB (should create 3 parts: 1MB, 1MB, 0.5MB)
	outDir := filepath.Join(tmpDir, "chunks")
	chunkSize := int64(1024 * 1024)
	manifest, err := SplitFile(srcPath, chunkSize, outDir, "bundle.tar.gz")
	if err != nil {
		t.Fatalf("SplitFile failed: %v", err)
	}

	if manifest.Parts.TotalParts != 3 {
		t.Fatalf("expected 3 parts, got %d", manifest.Parts.TotalParts)
	}
	if len(manifest.Parts.Items) != 3 {
		t.Fatalf("expected 3 items in manifest, got %d", len(manifest.Parts.Items))
	}

	expectedHash := fmt.Sprintf("%x", sha256.Sum256(dummyData))
	if !strings.EqualFold(manifest.Metadata.ArchiveSHA256, expectedHash) {
		t.Fatalf("overall hash mismatch: expected %s, got %s", expectedHash, manifest.Metadata.ArchiveSHA256)
	}

	// 3. Test OpenMultiPartReader (Streaming without temp file)
	stream, mLoaded, err := OpenMultiPartReader(outDir)
	if err != nil {
		t.Fatalf("OpenMultiPartReader failed: %v", err)
	}
	defer func() { _ = stream.Close() }()

	if mLoaded.Metadata.TotalBytes != int64(len(dummyData)) {
		t.Fatalf("metadata total bytes mismatch: expected %d, got %d", len(dummyData), mLoaded.Metadata.TotalBytes)
	}

	streamData, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("io.ReadAll from stream failed: %v", err)
	}

	if !bytes.Equal(streamData, dummyData) {
		t.Fatalf("streamed data does not match original data! length: got %d, expected %d", len(streamData), len(dummyData))
	}

	// 4. Test AssembleMultiPart
	assembledPath := filepath.Join(tmpDir, "assembled.bin")
	if aErr := AssembleMultiPart(outDir, assembledPath); aErr != nil {
		t.Fatalf("AssembleMultiPart failed: %v", aErr)
		t.Fatalf("AssembleMultiPart failed: %v", err)
	}

	assembledData, err := os.ReadFile(filepath.Clean(assembledPath)) //nolint:gosec
	if err != nil {
		t.Fatalf("reading assembled file failed: %v", err)
	}

	if !bytes.Equal(assembledData, dummyData) {
		t.Fatalf("assembled file does not match original data")
	}
}

//nolint:paralleltest
func TestMultiPart_MissingPartDetection(t *testing.T) {
	tmpDir := t.TempDir()

	dummyData := make([]byte, 1024*1024)
	srcPath := filepath.Join(tmpDir, "source.bin")
	if err := os.WriteFile(srcPath, dummyData, 0600); err != nil {
		t.Fatalf("failed writing source file: %v", err)
	}

	outDir := filepath.Join(tmpDir, "chunks")
	_, err := SplitFile(srcPath, 400*1024, outDir, "bundle.tar.gz")
	if err != nil {
		t.Fatalf("SplitFile failed: %v", err)
	}

	// Delete Part 2
	part2Path := filepath.Join(outDir, "bundle.tar.gz.part002")
	if rmErr := os.Remove(part2Path); rmErr != nil {
		t.Fatalf("failed removing part 2: %v", rmErr)
		t.Fatalf("failed removing part 2: %v", err)
	}

	// Validate should fail with ErrMissingPart
	_, _, err = ValidateMultiPartManifest(outDir)
	if err == nil {
		t.Fatalf("expected error on missing part, got nil")
	}
	if !errors.Is(err, ErrMissingPart) {
		t.Fatalf("expected ErrMissingPart, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Part 2") {
		t.Fatalf("expected error to mention 'Part 2', got: %v", err)
	}

	// OpenMultiPartReader should also fail
	_, _, err = OpenMultiPartReader(outDir)
	if err == nil {
		t.Fatalf("OpenMultiPartReader expected error on missing part, got nil")
	}
	if !errors.Is(err, ErrMissingPart) {
		t.Fatalf("expected ErrMissingPart, got: %v", err)
	}
}

//nolint:paralleltest
func TestMultiPart_CorruptedPartDetection(t *testing.T) {
	tmpDir := t.TempDir()

	dummyData := make([]byte, 1024*1024)
	srcPath := filepath.Join(tmpDir, "source.bin")
	if err := os.WriteFile(srcPath, dummyData, 0600); err != nil {
		t.Fatalf("failed writing source file: %v", err)
	}

	outDir := filepath.Join(tmpDir, "chunks")
	_, err := SplitFile(srcPath, 400*1024, outDir, "bundle.tar.gz")
	if err != nil {
		t.Fatalf("SplitFile failed: %v", err)
	}

	// Tamper Part 2 by flipping last byte
	part2Path := filepath.Join(outDir, "bundle.tar.gz.part002")
	part2Bytes, err := os.ReadFile(filepath.Clean(part2Path)) //nolint:gosec
	if err != nil {
		t.Fatalf("failed reading part 2: %v", err)
	}
	part2Bytes[len(part2Bytes)-1] ^= 0xFF
	if wErr := os.WriteFile(filepath.Clean(part2Path), part2Bytes, 0600); wErr != nil { //nolint:gosec
		t.Fatalf("failed writing tampered part 2: %v", wErr)
	}

	// Validate should fail with ErrChecksumMismatch
	_, _, err = ValidateMultiPartManifest(outDir)
	if err == nil {
		t.Fatalf("expected error on tampered part, got nil")
	}
	if !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("expected ErrChecksumMismatch, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Part 2") {
		t.Fatalf("expected error to mention 'Part 2', got: %v", err)
	}
}

//nolint:paralleltest
func TestMultiPart_VerifyMultiPartBundle(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Create a dummy tar.gz archive with manifest.yaml, signatures, and payload
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating key: %v", err)
	}

	manifestContent := []byte("version: '1.0'\nrelease: 'test-v1.0.0'\nservices:\n  - name: test-app\n")
	sig := ed25519.Sign(privKey, manifestContent)

	payloadFile := filepath.Join(tmpDir, "bundle.tar.gz")
	f, err := os.OpenFile(filepath.Clean(payloadFile), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600) //nolint:gosec
	if err != nil {
		t.Fatalf("failed creating bundle archive: %v", err)
	}

	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	// Add manifest.yaml
	_ = tw.WriteHeader(&tar.Header{
		Name: "manifest.yaml",
		Mode: 0640,
		Size: int64(len(manifestContent)),
	})
	_, _ = tw.Write(manifestContent)

	// Add signatures/manifest.sig
	_ = tw.WriteHeader(&tar.Header{
		Name: "signatures/manifest.sig",
		Mode: 0640,
		Size: int64(len(sig)),
	})
	_, _ = tw.Write(sig)

	// Add signatures/public.key
	pubHex := []byte(hex.EncodeToString(pubKey))
	_ = tw.WriteHeader(&tar.Header{
		Name: "signatures/public.key",
		Mode: 0640,
		Size: int64(len(pubHex)),
	})
	_, _ = tw.Write(pubHex)

	// Add dummy payload (500 KB)
	dummyPayload := make([]byte, 500*1024)
	for i := range dummyPayload {
		dummyPayload[i] = byte(i % 127)
	}
	_ = tw.WriteHeader(&tar.Header{
		Name: "artifacts/bin/test-app",
		Mode: 0750,
		Size: int64(len(dummyPayload)),
	})
	_, _ = tw.Write(dummyPayload)

	_ = tw.Close()
	_ = gw.Close()
	_ = f.Close()

	// 2. Split into 150KB chunks
	outDir := filepath.Join(tmpDir, "split-bundle")
	_, err = SplitFile(payloadFile, 150*1024, outDir, "bundle.tar.gz")
	if err != nil {
		t.Fatalf("SplitFile failed: %v", err)
	}

	// 3. Verify multi-part bundle
	res, err := VerifyMultiPartBundle(outDir, pubKey)
	if err != nil {
		t.Fatalf("VerifyMultiPartBundle failed: %v", err)
	}

	if !res.Valid {
		t.Fatalf("expected bundle to be valid, errors: %v", res.Errors)
	}
	if !res.SignatureValid {
		t.Fatalf("expected signature to be valid")
	}
	if res.FileCount != 4 {
		t.Fatalf("expected 4 files in bundle, got %d", res.FileCount)
	}
}
