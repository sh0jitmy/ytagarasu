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
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/bundle"
	"github.com/sh0jitmy/ytagarasu/internal/server/handler"
	"github.com/sh0jitmy/ytagarasu/internal/server/importer"
	"github.com/sh0jitmy/ytagarasu/internal/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUI_E2E_FullBrowserWorkflow(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	serverDataDir := filepath.Join(workDir, "server_data")
	nsDir := filepath.Join(serverDataDir, "namespaces")
	require.NoError(t, os.MkdirAll(nsDir, 0750))

	// 1. Setup Server with SQLite WAL
	dbDSN := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := store.NewDB(dbDSN)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	cas, err := store.NewCAS(filepath.Join(serverDataDir, "cas"))
	require.NoError(t, err)

	imp, err := importer.NewImporter(cas, db, nsDir)
	require.NoError(t, err)

	srv := handler.NewServer(db, imp, nsDir)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.Close()
	})

	client := ts.Client()

	// 2. Initial UI Check: Empty dashboard and zero registered services
	resp, err := client.Get(ts.URL + "/ui")
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	initialBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(initialBody), "オフライン配信ダッシュボード")
	assert.Contains(t, string(initialBody), "登録されているサービスはまだありません")
	assert.Contains(t, string(initialBody), "hx-get=\"/ui/components/system-metrics\"")

	// 3. Build a Valid Bundle for UI Upload Test
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	bundleSrcDir := filepath.Join(workDir, "bundle_src")
	require.NoError(t, os.MkdirAll(filepath.Join(bundleSrcDir, "artifacts"), 0750))
	require.NoError(t, os.WriteFile(filepath.Join(bundleSrcDir, "artifacts", "web-portal"), []byte("#!/bin/sh\n"), 0755)) //nolint:gosec

	manifestYAML := `
version: "1.0"
bundleVersion: "2026.09.27.1"
release: "v2.0.0"
createdAt: "2026-09-27T00:00:00Z"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
applications:
  - name: "web-portal"
    type: "golang"
    artifact: "artifacts/web-portal"
    destination: "/tmp/web-portal"
rollback:
  autoOnFailure: true
  strategy: "immediate"
`
	require.NoError(t, os.WriteFile(filepath.Join(bundleSrcDir, "manifest.yaml"), []byte(manifestYAML), 0600))
	bundleTarPath := filepath.Join(workDir, "upload-test-v2.0.0.tar.gz")
	_, err = bundle.BuildBundle(bundle.BuildOptions{
		SourceDir:          bundleSrcDir,
		OutputFile:         bundleTarPath,
		PrivateKey:         privKey,
		AllowWarningStatus: true,
	})
	require.NoError(t, err)

	// 4. Test UI Action: POST /ui/actions/import-bundle (HTMX upload form simulation)
	fileBytes, err := os.ReadFile(bundleTarPath) //nolint:gosec
	require.NoError(t, err)

	var formBuf bytes.Buffer
	mw := multipart.NewWriter(&formBuf)
	part, err := mw.CreateFormFile("bundle", "upload-test-v2.0.0.tar.gz")
	require.NoError(t, err)
	_, err = part.Write(fileBytes)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	importReq, err := http.NewRequest(http.MethodPost, ts.URL+"/ui/actions/import-bundle", &formBuf)
	require.NoError(t, err)
	importReq.Header.Set("Content-Type", mw.FormDataContentType())

	importResp, err := client.Do(importReq)
	require.NoError(t, err)
	defer func() {
		_ = importResp.Body.Close()
	}()
	assert.Equal(t, http.StatusOK, importResp.StatusCode)
	importBody, err := io.ReadAll(importResp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(importBody), "バンドルインポート成功", "HTMX import response should indicate success")
	assert.Contains(t, string(importBody), "web-portal")
	assert.Contains(t, string(importBody), "v2.0.0")

	// 5. Verify UI Components after Upload: Services table partial
	svcResp, err := client.Get(ts.URL + "/ui/components/services")
	require.NoError(t, err)
	defer func() {
		_ = svcResp.Body.Close()
	}()
	assert.Equal(t, http.StatusOK, svcResp.StatusCode)
	svcBody, err := io.ReadAll(svcResp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(svcBody), "web-portal")
	assert.Contains(t, string(svcBody), "v2.0.0")
	assert.NotContains(t, string(svcBody), "登録されているサービスはまだありません")

	// 6. Verify Audit Log UI & HTMX Verification Action
	auditResp, err := client.Get(ts.URL + "/ui/audit")
	require.NoError(t, err)
	defer func() {
		_ = auditResp.Body.Close()
	}()
	assert.Equal(t, http.StatusOK, auditResp.StatusCode)
	auditBody, err := io.ReadAll(auditResp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(auditBody), "改ざん耐性 SHA-256 監査チェーン")
	assert.Contains(t, string(auditBody), "release.import")
	assert.Contains(t, string(auditBody), "web-portal:v2.0.0")

	// Trigger verify-audit HTMX action
	verifyResp, err := client.Get(ts.URL + "/ui/actions/verify-audit")
	require.NoError(t, err)
	defer func() {
		_ = verifyResp.Body.Close()
	}()
	assert.Equal(t, http.StatusOK, verifyResp.StatusCode)
	verifyBody, err := io.ReadAll(verifyResp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(verifyBody), "チェーン整合性確認済み")
	assert.Contains(t, string(verifyBody), "badge-success")
}
