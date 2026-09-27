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
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/server/handler"
	"github.com/sh0jitmy/ytagarasu/internal/server/importer"
	"github.com/sh0jitmy/ytagarasu/internal/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIEndpoints(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	dbDSN := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := store.NewDB(dbDSN)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
	})

	cas, err := store.NewCAS(filepath.Join(tmpDir, "cas"))
	require.NoError(t, err)

	nsDir := filepath.Join(tmpDir, "namespaces")
	imp, err := importer.NewImporter(cas, db, nsDir)
	require.NoError(t, err)

	ctx := t.Context()
	// Insert dummy audit log
	_, err = db.AppendAuditLog(ctx, "system.init", "server", "test", []byte("unit testing ui"))
	require.NoError(t, err)

	srv := handler.NewServer(db, imp, nsDir)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.Close()
	})

	client := ts.Client()

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		containsBody   string
	}{
		{
			name:           "Root redirect to /ui",
			path:           "/",
			expectedStatus: http.StatusOK, // follows redirect
			containsBody:   "オフライン配信ダッシュボード",
		},
		{
			name:           "Dashboard HTML",
			path:           "/ui",
			expectedStatus: http.StatusOK,
			containsBody:   "ytagarasu",
		},
		{
			name:           "Audit HTML",
			path:           "/ui/audit",
			expectedStatus: http.StatusOK,
			containsBody:   "改ざん耐性 SHA-256 監査チェーン",
		},
		{
			name:           "Static CSS",
			path:           "/ui/static/css/dashboard.css",
			expectedStatus: http.StatusOK,
			containsBody:   "--bg-base",
		},
		{
			name:           "Static JS HTMX",
			path:           "/ui/static/js/htmx.min.js",
			expectedStatus: http.StatusOK,
			containsBody:   "htmx",
		},
		{
			name:           "Component Services",
			path:           "/ui/components/services",
			expectedStatus: http.StatusOK,
			containsBody:   "services-table-container",
		},
		{
			name:           "Component System Metrics",
			path:           "/ui/components/system-metrics",
			expectedStatus: http.StatusOK,
			containsBody:   "system-metrics-container",
		},
		{
			name:           "Component Audit Table",
			path:           "/ui/components/audit-table",
			expectedStatus: http.StatusOK,
			containsBody:   "audit-table-container",
		},
		{
			name:           "Action Verify Audit",
			path:           "/ui/actions/verify-audit",
			expectedStatus: http.StatusOK,
			containsBody:   "チェーン整合性確認済み",
		},
		{
			name:           "Component Service Detail empty",
			path:           "/ui/components/service-detail?service_id=none",
			expectedStatus: http.StatusOK,
			containsBody:   "アクティブリリース詳細インスペクト",
		},
		{
			name:           "Component Audit Table Filtered match",
			path:           "/ui/components/audit-table?event_type=system.init&q=server",
			expectedStatus: http.StatusOK,
			containsBody:   "system.init",
		},
		{
			name:           "Component Audit Table Filtered no match",
			path:           "/ui/components/audit-table?event_type=nonexistent",
			expectedStatus: http.StatusOK,
			containsBody:   "一致する監査ログレコードがありません",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp, err := client.Get(ts.URL + tc.path)
			require.NoError(t, err)
			defer func() {
				_ = resp.Body.Close()
			}()

			assert.Equal(t, tc.expectedStatus, resp.StatusCode)
			body := make([]byte, 10240)
			n, _ := resp.Body.Read(body)
			bodyStr := string(body[:n])
			assert.Contains(t, bodyStr, tc.containsBody)
		})
	}
}
