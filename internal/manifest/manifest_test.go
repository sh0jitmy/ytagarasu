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

package manifest_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shjtmy/go_sh0jitmy_template/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const sampleFullManifest = `
version: "1.0"
bundleVersion: "2026.09.26.1"
release: "1.4.2"
createdAt: "2026-09-26T10:00:00Z"
expiresAt: "2027-09-26T10:00:00Z"
author: "Release Engineering Team <release@example.internal>"

targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
  - os: "rhel"
    release: "9"
    arch: "amd64"

applications:
  - name: "core-api"
    type: "golang"
    selector:
      roles: ["api"]
    artifact: "artifacts/golang/api-server"
    destination: "/usr/local/bin/api-server"
    permissions: "0755"
    owner: "app"
    group: "app"

  - name: "ml-worker"
    type: "python"
    selector:
      roles: ["worker"]
    sourceDir: "artifacts/python/app"
    destination: "/opt/ml-worker/app"
    venvPath: "/opt/ml-worker/venv"
    wheelsDir: "artifacts/python/wheels"
    requirementsFile: "artifacts/python/requirements.lock"
    pythonBinary: "/usr/bin/python3"
    owner: "app"
    group: "app"

  - name: "admin-portal"
    type: "react"
    selector:
      roles: ["web"]
    distDir: "artifacts/react/dist"
    destination: "/var/www/admin-portal"
    owner: "www-data"
    group: "www-data"

  - name: "htmx-dashboard"
    type: "htmx"
    selector:
      roles: ["api"]
    embeddedIn: "core-api"

packages:
  ubuntu-24.04-amd64:
    manager: "apt"
    items:
      - name: "libssl3"
        version: "3.0.13-0ubuntu3.4"
        selector:
          roles: ["api", "worker"]
      - name: "python3"
        version: "3.12.3-0ubuntu1"
        selector:
          roles: ["worker"]
      - name: "nginx"
        version: "1.24.0-2ubuntu7"
        selector:
          roles: ["web"]

certificates:
  - name: "core-api-tls"
    selector:
      roles: ["api"]
    certPath: "/etc/ssl/certs/core-api.crt"
    keyPath: "/etc/ssl/private/core-api.key"
    permissions: "0600"
    owner: "app"
    group: "app"
    provisioning: "acme"
    acmeServer: "https://ytagarasu-server:9000/acme/acme/directory"
    commonName: "api.example.internal"
    reloadService: "core-api"
  - name: "web-tls"
    selector:
      roles: ["web"]
    certPath: "/etc/ssl/certs/web.crt"
    keyPath: "/etc/ssl/private/web.key"
    permissions: "0600"
    owner: "root"
    group: "root"
    provisioning: "encrypted_sops"
    encryptedKeyArtifact: "configs/certs/web.key.enc"
    certArtifact: "configs/certs/web.crt"

configs:
  - template: "configs/api.yaml.tmpl"
    destination: "/etc/ytagarasu/api.yaml"
    permissions: "0640"
    owner: "app"
    group: "app"
    sopsEncrypted: true
    selector:
      roles: ["api"]

services:
  - name: "core-api"
    action: "restart"
    unitFile: "configs/core-api.service"
    timeoutSeconds: 30
    selector:
      roles: ["api"]

healthChecks:
  - type: "http"
    endpoint: "http://127.0.0.1:8080/v1/system/healthz"
    expectedStatus: 200
    intervalSeconds: 2
    maxRetries: 10
    selector:
      roles: ["api"]

rollback:
  autoOnFailure: true
  strategy: "immediate"
  preserveBackupGenerations: 3
`

func TestParse_FullManifestSuccess(t *testing.T) {
	t.Parallel()

	m, err := manifest.Parse([]byte(sampleFullManifest))
	require.NoError(t, err)
	assert.Equal(t, "1.0", m.Version)
	assert.Equal(t, "2026.09.26.1", m.BundleVersion)
	assert.Equal(t, "1.4.2", m.Release)
	assert.Len(t, m.Targets, 2)
	assert.Len(t, m.Applications, 4)
	assert.Len(t, m.Certificates, 2)
	assert.Len(t, m.Configs, 1)
	assert.Len(t, m.Services, 1)
	assert.Len(t, m.HealthChecks, 1)
	assert.True(t, m.Rollback.AutoOnFailure)
	assert.Equal(t, manifest.RollbackImmediate, m.Rollback.Strategy)
}

func TestValidate_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		modify  func(m *manifest.Manifest)
		wantErr string
	}{
		{
			name: "missing version",
			modify: func(m *manifest.Manifest) {
				m.Version = ""
			},
			wantErr: "manifest version is required",
		},
		{
			name: "missing bundleVersion",
			modify: func(m *manifest.Manifest) {
				m.BundleVersion = ""
			},
			wantErr: "bundleVersion is required",
		},
		{
			name: "missing targets",
			modify: func(m *manifest.Manifest) {
				m.Targets = nil
			},
			wantErr: "at least one target platform must be defined",
		},
		{
			name: "missing applications",
			modify: func(m *manifest.Manifest) {
				m.Applications = nil
			},
			wantErr: "at least one application must be defined",
		},
		{
			name: "invalid application type",
			modify: func(m *manifest.Manifest) {
				m.Applications[0].Type = "ruby"
			},
			wantErr: "invalid application type",
		},
		{
			name: "missing golang destination",
			modify: func(m *manifest.Manifest) {
				m.Applications[0].Destination = ""
			},
			wantErr: "requires destination path",
		},
		{
			name: "invalid service action",
			modify: func(m *manifest.Manifest) {
				m.Services[0].Action = "destroy"
			},
			wantErr: "invalid service action",
		},
		{
			name: "acme cert missing server",
			modify: func(m *manifest.Manifest) {
				m.Certificates[0].ACMEServer = ""
			},
			wantErr: "acme provisioning requires acmeServer",
		},
		{
			name: "encrypted_sops cert missing artifact",
			modify: func(m *manifest.Manifest) {
				m.Certificates[1].EncryptedKeyArtifact = ""
			},
			wantErr: "encrypted_sops provisioning requires encryptedKeyArtifact",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, err := manifest.Parse([]byte(sampleFullManifest))
			require.NoError(t, err)

			tt.modify(m)
			err = manifest.Validate(m)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLint_PermissionsAndExpiration(t *testing.T) {
	t.Parallel()

	m, err := manifest.Parse([]byte(sampleFullManifest))
	require.NoError(t, err)

	// Normal manifest should have 0 errors
	issues := manifest.Lint(m)
	assert.Empty(t, issues)

	// Test insecure permission on private key
	m.Certificates[0].Permissions = "0777"
	issues = manifest.Lint(m)
	require.NotEmpty(t, issues)
	assert.Contains(t, issues[0].Message, "must not allow group or world access")

	// Test expired bundle
	past := time.Now().Add(-24 * time.Hour)
	m.ExpiresAt = &past
	issues = manifest.Lint(m)
	foundExpired := false
	for _, issue := range issues {
		if strings.Contains(issue.Message, "bundle expired") {
			foundExpired = true
			break
		}
	}
	assert.True(t, foundExpired)
}

func TestFilterByNode(t *testing.T) {
	t.Parallel()

	m, err := manifest.Parse([]byte(sampleFullManifest))
	require.NoError(t, err)

	// Filter for API role
	apiNode := manifest.FilterByNode(m, []string{"api"}, "api-host-01")
	require.NotNil(t, apiNode)
	// Applications matching "api": core-api, htmx-dashboard
	assert.Len(t, apiNode.Applications, 2)
	assert.Equal(t, "core-api", apiNode.Applications[0].Name)
	assert.Equal(t, "htmx-dashboard", apiNode.Applications[1].Name)
	// Certificate matching "api": core-api-tls
	assert.Len(t, apiNode.Certificates, 1)
	assert.Equal(t, "core-api-tls", apiNode.Certificates[0].Name)
	// Service matching "api": core-api
	assert.Len(t, apiNode.Services, 1)

	// Filter for Worker role
	workerNode := manifest.FilterByNode(m, []string{"worker"}, "worker-host-01")
	require.NotNil(t, workerNode)
	assert.Len(t, workerNode.Applications, 1)
	assert.Equal(t, "ml-worker", workerNode.Applications[0].Name)
	assert.Empty(t, workerNode.Certificates)

	// Filter for Web role
	webNode := manifest.FilterByNode(m, []string{"web"}, "web-host-01")
	require.NotNil(t, webNode)
	assert.Len(t, webNode.Applications, 1)
	assert.Equal(t, "admin-portal", webNode.Applications[0].Name)
	assert.Len(t, webNode.Certificates, 1)
}

func TestGenerateAuto(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Simulate Go and Python and React files
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module example.com/demo"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "requirements.txt"), []byte("fastapi==0.110.0"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(`{"name":"demo-web"}`), 0600))

	m, err := manifest.GenerateAuto(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, m)

	assert.Equal(t, "1.0", m.Version)
	assert.Len(t, m.Applications, 3)

	types := []manifest.ApplicationType{
		m.Applications[0].Type,
		m.Applications[1].Type,
		m.Applications[2].Type,
	}
	assert.Contains(t, types, manifest.AppTypeGolang)
	assert.Contains(t, types, manifest.AppTypePython)
	assert.Contains(t, types, manifest.AppTypeReact)

	// Validate generated manifest directly
	require.NoError(t, manifest.Validate(m))
}

func TestInitInteractive(t *testing.T) {
	t.Parallel()

	input := "custom-api\ngolang\napi\n/usr/local/bin/custom-api\ny\n"
	r := strings.NewReader(input)
	var w bytes.Buffer

	m, err := manifest.InitInteractive(r, &w)
	require.NoError(t, err)
	require.NotNil(t, m)

	assert.Len(t, m.Applications, 1)
	assert.Equal(t, "custom-api", m.Applications[0].Name)
	assert.Equal(t, manifest.AppTypeGolang, m.Applications[0].Type)
	assert.Len(t, m.Certificates, 1)
	assert.Equal(t, "custom-api-tls", m.Certificates[0].Name)

	require.NoError(t, manifest.Validate(m))
}

func TestParseFile(t *testing.T) {
	t.Parallel()

	tmpFile := filepath.Join(t.TempDir(), "manifest.yaml")
	require.NoError(t, os.WriteFile(tmpFile, []byte(sampleFullManifest), 0600))

	m, err := manifest.ParseFile(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "1.0", m.Version)

	// Roundtrip check
	outBytes, err := yaml.Marshal(m)
	require.NoError(t, err)
	m2, err := manifest.Parse(outBytes)
	require.NoError(t, err)
	assert.Equal(t, m.Release, m2.Release)
}

func TestEvaluateBundle(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	m, err := manifest.Parse([]byte(sampleFullManifest))
	require.NoError(t, err)

	target := manifest.Target{OS: "ubuntu", Release: "24.04", Arch: "amd64"}

	// 1. Missing artifacts -> should be blocked
	res, err := manifest.EvaluateBundle(m, tmpDir, target)
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, manifest.StatusBlocked, res.Status)
	assert.NotEmpty(t, res.Missing)
	assert.NotEmpty(t, res.Risks)

	// 2. Prepare required artifact files
	binaryPath := filepath.Join(tmpDir, "artifacts/golang/api-server")
	require.NoError(t, os.MkdirAll(filepath.Dir(binaryPath), 0750))
	require.NoError(t, os.WriteFile(binaryPath, []byte("dummy-binary"), 0750)) //nolint:gosec // testing executable

	reqPath := filepath.Join(tmpDir, "artifacts/python/requirements.lock")
	require.NoError(t, os.MkdirAll(filepath.Dir(reqPath), 0750))
	require.NoError(t, os.WriteFile(reqPath, []byte("dep==1.0"), 0600))

	reactDir := filepath.Join(tmpDir, "artifacts/react/dist")
	require.NoError(t, os.MkdirAll(reactDir, 0750))
	require.NoError(t, os.WriteFile(filepath.Join(reactDir, "index.html"), []byte("<html></html>"), 0600))

	tmplPath := filepath.Join(tmpDir, "configs/api.yaml.tmpl")
	require.NoError(t, os.MkdirAll(filepath.Dir(tmplPath), 0750))
	require.NoError(t, os.WriteFile(tmplPath, []byte("key: value"), 0600))

	// Re-evaluate -> should be ready
	res2, err := manifest.EvaluateBundle(m, tmpDir, target)
	require.NoError(t, err)
	require.NotNil(t, res2)
	assert.Equal(t, manifest.StatusReady, res2.Status)
	assert.Empty(t, res2.Missing)
	assert.Empty(t, res2.Risks)
	assert.Positive(t, res2.ResourceEstimates.RequiredDiskFreeBytes)

	// Save preparation result to disk
	outResultPath := filepath.Join(tmpDir, "preparation-result.yaml")
	require.NoError(t, manifest.SavePreparationResult(res2, outResultPath))
	assert.FileExists(t, outResultPath)
}
