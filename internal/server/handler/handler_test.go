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

package handler_test

import (
	"bytes"
	"encoding/json"
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

func setupTestServer(t *testing.T) (*handler.Server, string, []byte) {
	t.Helper()

	workDir := t.TempDir()

	cas, err := store.NewCAS(filepath.Join(workDir, "cas"))
	require.NoError(t, err)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := store.NewDB(dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	nsDir := filepath.Join(workDir, "namespaces")
	imp, err := importer.NewImporter(cas, db, nsDir)
	require.NoError(t, err)

	srv := handler.NewServer(db, imp, nsDir)

	// Create a valid bundle
	pubKey, privKey, err := bundle.GenerateKeyPair()
	require.NoError(t, err)

	srcDir := filepath.Join(workDir, "bundle-src")
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "artifacts", "golang"), 0750))
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "configs"), 0750))
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "apt"), 0750))

	require.NoError(t, os.WriteFile(filepath.Clean(filepath.Join(srcDir, "artifacts", "golang", "demo-app")), []byte("binary-payload"), 0700)) //nolint:gosec
	require.NoError(t, os.WriteFile(filepath.Clean(filepath.Join(srcDir, "configs", "demo.conf.tmpl")), []byte("env: test"), 0600))
	require.NoError(t, os.WriteFile(filepath.Clean(filepath.Join(srcDir, "apt", "demo.deb")), []byte("deb-package-bytes"), 0600))

	manifestContent := `
version: "1.0"
bundleVersion: "2026.09.26.1"
release: "3.0.0"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
applications:
  - name: "demo-service"
    type: "golang"
    artifact: "artifacts/golang/demo-app"
    destination: "/usr/local/bin/demo-app"
configs:
  - template: "configs/demo.conf.tmpl"
    destination: "/etc/demo.conf"
`
	manifestFile := filepath.Join(srcDir, "manifest.yaml")
	require.NoError(t, os.WriteFile(filepath.Clean(manifestFile), []byte(manifestContent), 0600))

	bundlePath := filepath.Join(workDir, "test-bundle.tar.gz")
	_, err = bundle.BuildBundle(bundle.BuildOptions{
		ManifestPath: manifestFile,
		SourceDir:    srcDir,
		OutputFile:   bundlePath,
		PrivateKey:   privKey,
	})
	require.NoError(t, err)

	bundleBytes, err := os.ReadFile(filepath.Clean(bundlePath)) //nolint:gosec
	require.NoError(t, err)

	_ = pubKey
	return srv, workDir, bundleBytes
}

func TestHandler_Healthz(t *testing.T) {
	t.Parallel()

	srv, _, _ := setupTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp map[string]any
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "ok", resp["status"])
}

func TestHandler_BundleImport_And_VirtualRepos(t *testing.T) {
	t.Parallel()

	srv, _, bundleBytes := setupTestServer(t)

	// 1. Initial services check -> empty
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var services []store.Service
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &services))
	assert.Empty(t, services)

	// 2. Import bundle via multipart/form-data
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("bundle", "bundle.tar.gz")
	require.NoError(t, err)
	_, err = io.Copy(part, bytes.NewReader(bundleBytes))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req = httptest.NewRequest(http.MethodPost, "/api/v1/bundles/import", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	var importRes importer.Result
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &importRes))
	assert.Equal(t, "demo-service", importRes.ServiceID)
	assert.Equal(t, "3.0.0", importRes.ReleaseVersion)

	// 3. Query desired manifest for demo-service
	req = httptest.NewRequest(http.MethodGet, "/api/v1/services/demo-service/desired", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/x-yaml", w.Header().Get("Content-Type"))
	assert.Equal(t, "3.0.0", w.Header().Get("X-Release-Version"))
	assert.Contains(t, w.Body.String(), "release: \"3.0.0\"")

	// 4. Access virtual repository files (apt debian package)
	req = httptest.NewRequest(http.MethodGet, "/repos/demo-service/apt/demo.deb", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/vnd.debian.binary-package", w.Header().Get("Content-Type"))
	assert.Equal(t, "deb-package-bytes", w.Body.String())

	// 5. Test Directory Traversal Defense
	req = httptest.NewRequest(http.MethodGet, "/repos/demo-service/..%2fsecret.txt", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	t.Logf("traversal status code: %d", w.Code)
	assert.True(t, w.Code == http.StatusForbidden || w.Code == http.StatusBadRequest || w.Code == http.StatusMovedPermanently)

	// 6. Test Non-existent file
	req = httptest.NewRequest(http.MethodGet, "/repos/demo-service/apt/not-found.deb", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
