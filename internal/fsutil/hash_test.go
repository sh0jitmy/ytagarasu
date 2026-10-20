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

package fsutil_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/fsutil"
)

func TestFileSHA256(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	t.Run("ValidFile", func(t *testing.T) {
		t.Parallel()

		filePath := filepath.Join(tmpDir, "test.txt")
		content := []byte("hello air-gap world")
		if err := os.WriteFile(filePath, content, 0600); err != nil {
			t.Fatalf("failed to write test file: %v", err)
		}

		expectedHash := sha256.Sum256(content)
		expectedHex := hex.EncodeToString(expectedHash[:])

		actualHex, err := fsutil.FileSHA256(filePath)
		if err != nil {
			t.Fatalf("unexpected error from FileSHA256: %v", err)
		}

		if actualHex != expectedHex {
			t.Errorf("expected hash %s, got %s", expectedHex, actualHex)
		}
	})

	t.Run("EmptyFile", func(t *testing.T) {
		t.Parallel()

		filePath := filepath.Join(tmpDir, "empty.txt")
		if err := os.WriteFile(filePath, []byte(""), 0600); err != nil {
			t.Fatalf("failed to write empty file: %v", err)
		}

		expectedHash := sha256.Sum256([]byte(""))
		expectedHex := hex.EncodeToString(expectedHash[:])

		actualHex, err := fsutil.FileSHA256(filePath)
		if err != nil {
			t.Fatalf("unexpected error for empty file: %v", err)
		}

		if actualHex != expectedHex {
			t.Errorf("expected hash %s, got %s", expectedHex, actualHex)
		}
	})

	t.Run("NonExistentFile", func(t *testing.T) {
		t.Parallel()

		_, err := fsutil.FileSHA256(filepath.Join(tmpDir, "does-not-exist.bin"))
		if err == nil {
			t.Error("expected error for non-existent file, got nil")
		}
	})
}

func TestVerifyFileSHA256(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "sample.pkg")
	content := []byte("binary package content 12345")
	if err := os.WriteFile(filePath, content, 0600); err != nil {
		t.Fatalf("failed to write sample package: %v", err)
	}

	h := sha256.Sum256(content)
	correctHash := hex.EncodeToString(h[:])

	tests := []struct {
		name         string
		expectedHash string
		wantMatch    bool
		wantErr      bool
	}{
		{
			name:         "ExactMatch",
			expectedHash: correctHash,
			wantMatch:    true,
			wantErr:      false,
		},
		{
			name:         "UppercaseHash",
			expectedHash: strings.ToUpper(correctHash),
			wantMatch:    true,
			wantErr:      false,
		},
		{
			name:         "WhitespacePadded",
			expectedHash: "  " + correctHash + "\n",
			wantMatch:    true,
			wantErr:      false,
		},
		{
			name:         "Mismatch",
			expectedHash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			wantMatch:    false,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			matched, err := fsutil.VerifyFileSHA256(filePath, tt.expectedHash)
			if (err != nil) != tt.wantErr {
				t.Fatalf("VerifyFileSHA256() error = %v, wantErr %v", err, tt.wantErr)
			}
			if matched != tt.wantMatch {
				t.Errorf("VerifyFileSHA256() = %v, want %v", matched, tt.wantMatch)
			}
		})
	}
}
