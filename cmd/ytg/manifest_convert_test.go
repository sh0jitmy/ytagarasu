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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleManifestConvert(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	outputPath := filepath.Join(tempDir, "manifest.yaml")

	composeContent := `
version: "3"
services:
  web:
    image: nginx:alpine
    ports:
      - "8080:80"
    environment:
      - ENV=production
`
	require.NoError(t, os.WriteFile(composePath, []byte(composeContent), 0600))

	// Run conversion
	handleManifestConvert([]string{"-f", composePath, "-o", outputPath, "-release", "test-release"})

	assert.FileExists(t, outputPath)
	data, err := os.ReadFile(filepath.Clean(outputPath)) //nolint:gosec
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "name: web")
	assert.Contains(t, content, "image: nginx:alpine")
	assert.Contains(t, content, "8080:80")
	assert.Contains(t, content, "release: test-release")
}
