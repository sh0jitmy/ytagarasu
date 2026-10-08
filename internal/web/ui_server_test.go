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

package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIServer_RoutesAndHTMX(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// In-memory sqlite for clean testing
	dbClient, err := database.NewClient(ctx, "sqlite3", "file:test_web.db?mode=memory&cache=shared&_pragma=foreign_keys(1)")
	require.NoError(t, err)
	defer func() { _ = dbClient.Close() }()

	require.NoError(t, database.SeedAdminUser(ctx, dbClient))

	server, err := NewUIServer(dbClient, t.TempDir(), "3001")
	require.NoError(t, err)

	// 1. Health check endpoints
	{
		// /healthz
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
		server.Engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "OK")

		// /v1/system/healthz (Navigation link)
		wHealth := httptest.NewRecorder()
		reqHealth, _ := http.NewRequest(http.MethodGet, "/v1/system/healthz", nil)
		server.Engine.ServeHTTP(wHealth, reqHealth)
		assert.Equal(t, http.StatusOK, wHealth.Code)
		assert.Contains(t, wHealth.Body.String(), "UP")

		// /metrics (Prometheus endpoint)
		wMetrics := httptest.NewRecorder()
		reqMetrics, _ := http.NewRequest(http.MethodGet, "/metrics", nil)
		server.Engine.ServeHTTP(wMetrics, reqMetrics)
		assert.Equal(t, http.StatusOK, wMetrics.Code)
		assert.Contains(t, wMetrics.Body.String(), "go_goroutines")
	}

	// 2. Full Dashboard Page
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		server.Engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "ytagarasu Tactical Dashboard")
		assert.Contains(t, w.Body.String(), "hx-get=\"/ui/components/supervision-panel\"")
		assert.Contains(t, w.Body.String(), "hx-trigger=\"every 1s\"", "Expected high-frequency 1s polling")
		assert.Contains(t, w.Body.String(), "hx-get=\"/ui/components/system-metrics\"")
	}

	// 3. HTMX Supervision Panel Initial Rendering
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/ui/components/supervision-panel", nil)
		server.Engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "常駐プロセス看取り・運用制御盤")
		assert.Contains(t, w.Body.String(), "HAL:")
		assert.Contains(t, w.Body.String(), "CUD Triple-Coding")
		assert.Contains(t, w.Body.String(), "demo-api")
		assert.Contains(t, w.Body.String(), "RUNNING")
		assert.Contains(t, w.Body.String(), "◆")
	}

	// 4. HTMX Supervision Actions (Stop -> State Persists on Next GET Polling)
	{
		// Action: Stop process
		wStop := httptest.NewRecorder()
		reqStop, _ := http.NewRequest(http.MethodPost, "/ui/actions/stop-process?name=demo-api", nil)
		server.Engine.ServeHTTP(wStop, reqStop)
		assert.Equal(t, http.StatusOK, wStop.Code)
		assert.Contains(t, wStop.Body.String(), "STOPPED")

		// Subsequent Polling GET MUST maintain STOPPED state!
		wPoll := httptest.NewRecorder()
		reqPoll, _ := http.NewRequest(http.MethodGet, "/ui/components/supervision-panel", nil)
		server.Engine.ServeHTTP(wPoll, reqPoll)
		assert.Equal(t, http.StatusOK, wPoll.Code)
		assert.Contains(t, wPoll.Body.String(), "STOPPED", "Stop status must persist across subsequent polling calls")

		// Action: Restart process
		wRestart := httptest.NewRecorder()
		reqRestart, _ := http.NewRequest(http.MethodPost, "/ui/actions/restart-process?name=demo-api", nil)
		server.Engine.ServeHTTP(wRestart, reqRestart)
		assert.Equal(t, http.StatusOK, wRestart.Code)
		assert.Contains(t, wRestart.Body.String(), "RUNNING")

		// Action: Start process
		wStart := httptest.NewRecorder()
		reqStart, _ := http.NewRequest(http.MethodPost, "/ui/actions/start-process?name=demo-api", nil)
		server.Engine.ServeHTTP(wStart, reqStart)
		assert.Equal(t, http.StatusOK, wStart.Code)
		assert.Contains(t, wStart.Body.String(), "RUNNING")
	}

	// 5. Dynamic Time Progression & CPU Oscillation Verification
	{
		time.Sleep(10 * time.Millisecond)
		status1 := server.SupervisionManager.CollectStatus()
		time.Sleep(20 * time.Millisecond)
		status2 := server.SupervisionManager.CollectStatus()

		assert.NotEmpty(t, status1.SystemUptime)
		assert.Equal(t, 2, status1.TotalProcs)
		assert.Greater(t, status1.Processes[0].CPUPercent, 0.0)
		assert.Greater(t, status2.Processes[0].CPUPercent, 0.0)
	}

	// 6. HTMX System Metrics Partial
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/ui/components/system-metrics", nil)
		server.Engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "Active Goroutines")
	}

	// 7. HTMX Users Table Partial
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/ui/components/users-table", nil)
		server.Engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "登録ユーザー一覧")
	}

	// 8. HTMX Backups Panel Partial
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/ui/components/backup-panel", nil)
		server.Engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "データベースバックアップ")
	}

	// 9. Static CSS Asset
	{
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/static/css/dashboard.css", nil)
		server.Engine.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "--bg-base")
		assert.Contains(t, w.Body.String(), "supervision-card")
		assert.Contains(t, w.Body.String(), "navbar-brand")
	}
}
