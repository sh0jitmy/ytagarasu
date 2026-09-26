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

package configmgr_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/configmgr"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderer_RenderWithHelpers(t *testing.T) {
	t.Parallel()

	tmplStr := `
service: {{ .ServiceID | upper }}
host: {{ .Hostname | lower }}
port: {{ default "8080" .Port }}
`
	renderer, err := configmgr.NewRenderer("test.yaml", tmplStr)
	require.NoError(t, err)

	data := map[string]string{
		"ServiceID": "payment-api",
		"Hostname":  "NODE-01.INTERNAL",
		"Port":      "",
	}

	rendered, err := renderer.Render(data)
	require.NoError(t, err)

	assert.Contains(t, string(rendered), "service: PAYMENT-API")
	assert.Contains(t, string(rendered), "host: node-01.internal")
	assert.Contains(t, string(rendered), "port: 8080")
}

func TestTransaction_AtomicCommitAndRollback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	workDir := t.TempDir()
	backupDir := filepath.Join(workDir, "backups")

	targetFile := filepath.Join(workDir, "etc", "app.conf")
	require.NoError(t, os.MkdirAll(filepath.Dir(targetFile), 0750))
	require.NoError(t, os.WriteFile(filepath.Clean(targetFile), []byte("version: 1.0"), 0600))

	// 1. Transaction with validation error -> Original file must be completely preserved (DoD 1)
	txFail, err := configmgr.NewTransaction(backupDir)
	require.NoError(t, err)

	// Validate command fails using exit 1
	err = txFail.StageFile(ctx, targetFile, []byte("version: 2.0-invalid"), 0600, "false")
	require.Error(t, err)
	require.ErrorIs(t, err, configmgr.ErrValidationFailed)

	// Verify original file is unchanged
	origContent, err := os.ReadFile(filepath.Clean(targetFile)) //nolint:gosec
	require.NoError(t, err)
	assert.Equal(t, "version: 1.0", string(origContent))

	// 2. Successful Transaction with Commit
	txSuccess, err := configmgr.NewTransaction(backupDir)
	require.NoError(t, err)

	// Validate command succeeds using true (exit 0)
	err = txSuccess.StageFile(ctx, targetFile, []byte("version: 2.0-valid"), 0600, "true")
	require.NoError(t, err)

	// Still old content before commit
	midContent, err := os.ReadFile(filepath.Clean(targetFile)) //nolint:gosec
	require.NoError(t, err)
	assert.Equal(t, "version: 1.0", string(midContent))

	// Commit atomically updates the file
	err = txSuccess.Commit()
	require.NoError(t, err)

	newContent, err := os.ReadFile(filepath.Clean(targetFile)) //nolint:gosec
	require.NoError(t, err)
	assert.Equal(t, "version: 2.0-valid", string(newContent))

	// 3. Rollback after commit -> Restores original file (DoD 2)
	err = txSuccess.Rollback()
	require.NoError(t, err)

	restoredContent, err := os.ReadFile(filepath.Clean(targetFile)) //nolint:gosec
	require.NoError(t, err)
	assert.Equal(t, "version: 1.0", string(restoredContent))
}

func TestHealthChecker_HTTPAndCommand(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// 1. HTTP Probe Success
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	checker := configmgr.NewHealthChecker(server.Client())

	hcHTTP := manifest.HealthCheck{
		Type:            manifest.HealthCheckHTTP,
		Endpoint:        server.URL + "/healthz",
		ExpectedStatus:  http.StatusOK,
		IntervalSeconds: 0,
		MaxRetries:      2,
	}
	require.NoError(t, checker.Check(ctx, hcHTTP))

	// 2. HTTP Probe Failure
	hcHTTPFail := manifest.HealthCheck{
		Type:            manifest.HealthCheckHTTP,
		Endpoint:        server.URL + "/bad-path",
		ExpectedStatus:  http.StatusOK,
		IntervalSeconds: 0,
		MaxRetries:      1,
	}
	require.Error(t, checker.Check(ctx, hcHTTPFail))

	// 3. Command Probe Success
	hcCmd := manifest.HealthCheck{
		Type:       manifest.HealthCheckCommand,
		Command:    "true",
		MaxRetries: 1,
	}
	require.NoError(t, checker.Check(ctx, hcCmd))

	// 4. Command Probe Failure
	hcCmdFail := manifest.HealthCheck{
		Type:       manifest.HealthCheckCommand,
		Command:    "false",
		MaxRetries: 1,
	}
	require.Error(t, checker.Check(ctx, hcCmdFail))
}
