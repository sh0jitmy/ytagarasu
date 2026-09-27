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

package e2e_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/agent"
	"github.com/sh0jitmy/ytagarasu/internal/bundle"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/sh0jitmy/ytagarasu/internal/server/handler"
	"github.com/sh0jitmy/ytagarasu/internal/server/importer"
	"github.com/sh0jitmy/ytagarasu/internal/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type mockPkgMgr struct{}

func (m *mockPkgMgr) UpdateRepositories(_ context.Context) error {
	return nil
}

func (m *mockPkgMgr) InstallPackages(_ context.Context, _ []manifest.PackageItem) error {
	return nil
}

func TestAgentServerE2E_FullLifecycleAndRollback(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	serverDataDir := filepath.Join(workDir, "server_data")
	agentWorkDir := filepath.Join(workDir, "agent_work")
	configsTargetDir := filepath.Join(workDir, "etc_app")

	require.NoError(t, os.MkdirAll(serverDataDir, 0750))
	require.NoError(t, os.MkdirAll(agentWorkDir, 0750))
	require.NoError(t, os.MkdirAll(configsTargetDir, 0750))

	// 1. Key generation (Ed25519)
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	// 2. Setup Server
	dbDSN := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := store.NewDB(dbDSN)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	cas, err := store.NewCAS(filepath.Join(serverDataDir, "cas"))
	require.NoError(t, err)

	nsDir := filepath.Join(serverDataDir, "namespaces")
	imp, err := importer.NewImporter(cas, db, nsDir)
	require.NoError(t, err)

	srv := handler.NewServer(db, imp, nsDir)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.Close()
	})

	// 3. Create Valid Release Bundle v1.0.0
	serviceID := "payment-gw"
	v1Archive := filepath.Join(workDir, "bundle-v1.0.0.tar.gz")
	targetConfigFile := filepath.Join(configsTargetDir, "payment.conf")
	v1SrcDir := filepath.Join(workDir, "src_v1")
	require.NoError(t, os.MkdirAll(filepath.Join(v1SrcDir, "configs"), 0750))

	v1ManifestYAML := fmt.Sprintf(`
version: "1.0"
bundleVersion: "2026.09.27.1"
release: "v1.0.0"
createdAt: "2026-09-27T00:00:00Z"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
applications:
  - name: "%s"
    type: "golang"
    artifact: "artifacts/payment-gw"
    destination: "/tmp/payment-gw"
    selector:
      roles: ["payment"]
configs:
  - template: "configs/payment.conf.tmpl"
    destination: "%s"
    permissions: "0644"
    validateCommand: "cat {{.TempFile}}"
healthChecks:
  - type: "command"
    command: "test -f %s"
    intervalSeconds: 1
    maxRetries: 2
rollback:
  autoOnFailure: true
  strategy: "immediate"
`, serviceID, targetConfigFile, targetConfigFile)

	require.NoError(t, os.MkdirAll(filepath.Join(v1SrcDir, "artifacts"), 0750))
	require.NoError(t, os.WriteFile(filepath.Join(v1SrcDir, "artifacts", "payment-gw"), []byte("#!/bin/sh\necho ok\n"), 0755)) //nolint:gosec
	require.NoError(t, os.WriteFile(filepath.Join(v1SrcDir, "manifest.yaml"), []byte(v1ManifestYAML), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(v1SrcDir, "configs", "payment.conf.tmpl"), []byte("env=production-offline\nstatus=active\n"), 0644)) //nolint:gosec

	_, err = bundle.BuildBundle(bundle.BuildOptions{
		SourceDir:          v1SrcDir,
		OutputFile:         v1Archive,
		PrivateKey:         privKey,
		AllowWarningStatus: true,
	})
	require.NoError(t, err)

	// 4. Import v1.0.0 Bundle to Server via HTTP API
	uploadBundle(t, ts.URL+"/api/v1/bundles/import", v1Archive)

	// Verify server desired state endpoint
	desiredResp, err := ts.Client().Get(fmt.Sprintf("%s/api/v1/services/%s/desired", ts.URL, serviceID))
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = desiredResp.Body.Close()
	})
	assert.Equal(t, http.StatusOK, desiredResp.StatusCode)

	var fetchedManifest manifest.Manifest
	require.NoError(t, yaml.NewDecoder(desiredResp.Body).Decode(&fetchedManifest))
	assert.Equal(t, "v1.0.0", fetchedManifest.Release)

	// 5. Initialize Agent and Sync v1.0.0
	ag := agent.NewAgent(agent.Config{
		ServerURL:  ts.URL,
		ServiceID:  serviceID,
		StateFile:  filepath.Join(agentWorkDir, "state.json"),
		LockFile:   filepath.Join(agentWorkDir, "agent.lock"),
		InstallDir: agentWorkDir,
		Roles:      []string{"payment"},
		Hostname:   "node-e2e-01",
		HTTPClient: ts.Client(),
	}, &mockPkgMgr{})

	// Run agent sync
	ctx := t.Context()
	report, err := ag.StepOnce(ctx)
	require.NoError(t, err)
	assert.True(t, report.Success)
	assert.Equal(t, "v1.0.0", report.Version)

	// Verify target config content
	renderedBytes, err := os.ReadFile(targetConfigFile) //nolint:gosec
	require.NoError(t, err)
	assert.Contains(t, string(renderedBytes), "env=production-offline")
	assert.Contains(t, string(renderedBytes), "status=active")

	// 6. Test Rollback Scenario: Create Bad Bundle v1.1.0 with failing validateCommand
	v11Archive := filepath.Join(workDir, "bundle-v1.1.0-bad.tar.gz")
	v11SrcDir := filepath.Join(workDir, "src_v11")
	require.NoError(t, os.MkdirAll(filepath.Join(v11SrcDir, "configs"), 0750))

	v11ManifestYAML := fmt.Sprintf(`
version: "1.0"
bundleVersion: "2026.09.27.2"
release: "v1.1.0"
createdAt: "2026-09-27T01:00:00Z"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
applications:
  - name: "%s"
    type: "golang"
    artifact: "artifacts/payment-gw"
    destination: "/tmp/payment-gw"
    selector:
      roles: ["payment"]
configs:
  - template: "configs/payment.conf.tmpl"
    destination: "%s"
    permissions: "0644"
    validateCommand: "sh -c 'exit 1'"
healthChecks:
  - type: "command"
    command: "test -f %s"
    intervalSeconds: 1
    maxRetries: 1
rollback:
  autoOnFailure: true
  strategy: "immediate"
`, serviceID, targetConfigFile, targetConfigFile)

	require.NoError(t, os.MkdirAll(filepath.Join(v11SrcDir, "artifacts"), 0750))
	require.NoError(t, os.WriteFile(filepath.Join(v11SrcDir, "artifacts", "payment-gw"), []byte("#!/bin/sh\necho ok\n"), 0755)) //nolint:gosec
	require.NoError(t, os.WriteFile(filepath.Join(v11SrcDir, "manifest.yaml"), []byte(v11ManifestYAML), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(v11SrcDir, "configs", "payment.conf.tmpl"), []byte("broken=true\n"), 0644)) //nolint:gosec

	_, err = bundle.BuildBundle(bundle.BuildOptions{
		SourceDir:          v11SrcDir,
		OutputFile:         v11Archive,
		PrivateKey:         privKey,
		AllowWarningStatus: true,
	})
	require.NoError(t, err)

	// Import bad bundle
	uploadBundle(t, ts.URL+"/api/v1/bundles/import", v11Archive)

	// Run agent sync again -> should fail and rollback
	report2, err := ag.StepOnce(ctx)
	require.Error(t, err, "expected agent sync to fail due to validateCommand failure")
	if report2 != nil {
		assert.False(t, report2.Success)
	}

	// Verify file was restored to v1.0.0 snapshot
	restoredBytes, err := os.ReadFile(targetConfigFile) //nolint:gosec
	require.NoError(t, err)
	assert.Contains(t, string(restoredBytes), "status=active", "file should be restored to v1.0.0 snapshot")
	assert.NotContains(t, string(restoredBytes), "broken=true")

	// 7. Verify Audit Hash Chain via Server API
	auditResp, err := ts.Client().Get(ts.URL + "/api/v1/audit/verify")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = auditResp.Body.Close()
	})
	assert.Equal(t, http.StatusOK, auditResp.StatusCode)

	var verifyReport map[string]interface{}
	require.NoError(t, json.NewDecoder(auditResp.Body).Decode(&verifyReport))
	assert.Equal(t, true, verifyReport["valid"], "audit chain must be mathematically intact")
}

func uploadBundle(t *testing.T, targetURL, filePath string) {
	t.Helper()

	fileBytes, err := os.ReadFile(filePath) //nolint:gosec
	require.NoError(t, err)

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	part, err := w.CreateFormFile("bundle", filepath.Base(filePath))
	require.NoError(t, err)
	_, err = part.Write(fileBytes)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	req, err := http.NewRequest(http.MethodPost, targetURL, &b)
	require.NoError(t, err)
	req.Header.Set("Content-Type", w.FormDataContentType())

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()

	require.Equal(t, http.StatusCreated, resp.StatusCode, "bundle upload failed with code: %d", resp.StatusCode)
}
