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

package agent_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/agent"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockPackageManager struct {
	mu            sync.Mutex
	installedPkgs []manifest.PackageItem
}

func (m *mockPackageManager) UpdateRepositories(ctx context.Context) error {
	return nil
}

func (m *mockPackageManager) InstallPackages(ctx context.Context, pkgs []manifest.PackageItem) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.installedPkgs = append(m.installedPkgs, pkgs...)
	return nil
}

func TestAgent_DeployLifecycleAndAutonomousUpdate(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()

	currentManifest := `
version: "1.0"
bundleVersion: "2026.09.26.1"
release: "1.0.0"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
packages:
  system:
    manager: "apt"
    items:
      - name: "curl"
applications:
  - name: "payment-api"
    type: "golang"
    artifact: "artifacts/golang/payment-api"
    destination: "bin/payment-api"
    permissions: "0755"
`
	currentReleaseID := "payment-api-1.0.0-100"

	var mu sync.Mutex
	var reportedLogs []agent.DeployReport

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		switch r.URL.Path {
		case "/api/v1/services/payment-api/desired":
			if r.Header.Get("If-None-Match") == currentReleaseID {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("Content-Type", "application/x-yaml")
			w.Header().Set("X-Release-ID", currentReleaseID)
			w.Header().Set("X-Release-Version", "1.0.0")
			_, _ = w.Write([]byte(currentManifest))

		case "/repos/payment-api/artifacts/golang/payment-api":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("#!/bin/sh\necho payment-api running\n"))

		case "/api/v1/audit/logs":
			var rep agent.DeployReport
			if err := json.NewDecoder(r.Body).Decode(&rep); err == nil {
				reportedLogs = append(reportedLogs, rep)
			}
			w.WriteHeader(http.StatusOK)

		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	mockPkg := &mockPackageManager{}
	cfg := agent.Config{
		ServerURL:    server.URL,
		ServiceID:    "payment-api",
		PollInterval: 100 * time.Millisecond,
		StateFile:    filepath.Join(workDir, "state.json"),
		LockFile:     filepath.Join(workDir, "agent.lock"),
		InstallDir:   workDir,
		Roles:        []string{"api"},
		Hostname:     "host-node-01",
	}

	ag := agent.NewAgent(cfg, mockPkg)
	ctx := context.Background()

	// 1. Initial Step: Should detect 1.0.0, install curl, download binary, send report
	rep, err := ag.StepOnce(ctx)
	require.NoError(t, err)
	require.NotNil(t, rep)
	assert.Equal(t, "payment-api", rep.ServiceID)
	assert.Equal(t, currentReleaseID, rep.ReleaseID)
	assert.True(t, rep.Success)

	// Verify packages installed
	mockPkg.mu.Lock()
	require.Len(t, mockPkg.installedPkgs, 1)
	assert.Equal(t, "curl", mockPkg.installedPkgs[0].Name)
	mockPkg.mu.Unlock()

	// Verify downloaded artifact binary
	binPath := filepath.Join(workDir, "bin", "payment-api")
	binData, err := os.ReadFile(filepath.Clean(binPath)) //nolint:gosec
	require.NoError(t, err)
	assert.Contains(t, string(binData), "payment-api running")

	info, err := os.Stat(binPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0755), info.Mode().Perm())

	// 2. Second Step: No changes (304 Not Modified / already synced)
	rep2, err := ag.StepOnce(ctx)
	require.NoError(t, err)
	assert.Nil(t, rep2)

	// 3. Update server release to 1.1.0 -> Agent autonomously deploys new release
	mu.Lock()
	currentReleaseID = "payment-api-1.1.0-200"
	currentManifest = `
version: "1.0"
bundleVersion: "2026.09.26.2"
release: "1.1.0"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
packages:
  system:
    manager: "apt"
    items:
      - name: "jq"
applications:
  - name: "payment-api"
    type: "golang"
    artifact: "artifacts/golang/payment-api"
    destination: "bin/payment-api"
    permissions: "0755"
`
	mu.Unlock()

	rep3, err := ag.StepOnce(ctx)
	require.NoError(t, err)
	require.NotNil(t, rep3)
	assert.Equal(t, currentReleaseID, rep3.ReleaseID)
	assert.Equal(t, "1.1.0", rep3.Version)
	assert.True(t, rep3.Success)

	// Verify new package jq installed
	mockPkg.mu.Lock()
	require.Len(t, mockPkg.installedPkgs, 2)
	assert.Equal(t, "jq", mockPkg.installedPkgs[1].Name)
	mockPkg.mu.Unlock()

	// Verify reports sent to audit API
	mu.Lock()
	require.Len(t, reportedLogs, 2)
	assert.Equal(t, "payment-api-1.0.0-100", reportedLogs[0].ReleaseID)
	assert.Equal(t, "payment-api-1.1.0-200", reportedLogs[1].ReleaseID)
	mu.Unlock()
}
