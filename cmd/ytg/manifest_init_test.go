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
	"testing"

	"gopkg.in/yaml.v3"
)

func TestGenerateManifestTemplate_Presets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		opts       ManifestInitOptions
		wantSubstr string
	}{
		{
			name:       "generic preset",
			opts:       ManifestInitOptions{AppType: "generic"},
			wantSubstr: "web-api",
		},
		{
			name:       "golang preset",
			opts:       ManifestInitOptions{AppType: "golang"},
			wantSubstr: "golang-service",
		},
		{
			name:       "python preset",
			opts:       ManifestInitOptions{AppType: "python"},
			wantSubstr: "python-worker",
		},
		{
			name:       "minimal generic preset",
			opts:       ManifestInitOptions{AppType: "generic", Minimal: true},
			wantSubstr: "main-service",
		},
		{
			name:       "minimal python preset",
			opts:       ManifestInitOptions{AppType: "python", Minimal: true},
			wantSubstr: "python3",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			content, err := GenerateManifestTemplate(tc.opts)
			if err != nil {
				t.Fatalf("GenerateManifestTemplate failed: %v", err)
			}
			if !strings.Contains(content, tc.wantSubstr) {
				t.Errorf("content missing expected substring %q, got:\n%s", tc.wantSubstr, content)
			}

			// Validate YAML syntax
			var node yaml.Node
			if err := yaml.Unmarshal([]byte(content), &node); err != nil {
				t.Fatalf("generated YAML is syntactically invalid: %v\nContent:\n%s", err, content)
			}
		})
	}
}

func TestGenerateManifestTemplate_Unsupported(t *testing.T) {
	t.Parallel()

	_, err := GenerateManifestTemplate(ManifestInitOptions{AppType: "unknown_stack"})
	if err == nil {
		t.Fatal("expected error for unsupported app type, got nil")
	}
}

func TestParseManifestInitArgs(t *testing.T) {
	t.Parallel()

	args := []string{"-o", "custom.yaml", "--type", "golang", "--minimal", "-f"}
	opts, err := parseManifestInitArgs(args)
	if err != nil {
		t.Fatalf("parseManifestInitArgs failed: %v", err)
	}

	if opts.OutputPath != "custom.yaml" {
		t.Errorf("OutputPath = %q, want 'custom.yaml'", opts.OutputPath)
	}
	if opts.AppType != "golang" {
		t.Errorf("AppType = %q, want 'golang'", opts.AppType)
	}
	if !opts.Minimal {
		t.Errorf("Minimal = false, want true")
	}
	if !opts.Force {
		t.Errorf("Force = false, want true")
	}
}

func TestGenerateManifest_FilePersistence(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "manifest.yaml")

	opts := ManifestInitOptions{
		OutputPath: targetFile,
		AppType:    "golang",
	}

	content, err := GenerateManifestTemplate(opts)
	if err != nil {
		t.Fatalf("GenerateManifestTemplate failed: %v", err)
	}

	cleanFile := filepath.Clean(targetFile)
	if writeErr := os.WriteFile(cleanFile, []byte(content), 0600); writeErr != nil {
		t.Fatalf("failed to write manifest: %v", writeErr)
	}

	// Verify file existence and contents
	data, err := os.ReadFile(cleanFile)
	if err != nil {
		t.Fatalf("failed to read back written manifest: %v", err)
	}

	if !strings.Contains(string(data), "golang-service") {
		t.Errorf("written manifest missing golang-service content")
	}
}
