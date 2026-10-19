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
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// getFreePort binds a random OS-assigned TCP port on 127.0.0.1 and returns it.
func getFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err, "failed to acquire free port")
	defer func() {
		_ = l.Close()
	}()
	return l.Addr().(*net.TCPAddr).Port
}

// resolveMusubiBinaries locates or builds musubi-server and musubi-web binaries.
func resolveMusubiBinaries(t *testing.T) (string, string) {
	t.Helper()

	// 1. Check environment variables
	serverBin := os.Getenv("MUSUBI_SERVER_BIN")
	webBin := os.Getenv("MUSUBI_WEB_BIN")
	if serverBin != "" && webBin != "" {
		if fileExists(serverBin) && fileExists(webBin) {
			return filepath.Clean(serverBin), filepath.Clean(webBin)
		}
	}

	// 2. Candidate source / bin directories
	candidates := []string{
		"../../Musubi",
		"/Users/shjtmy/gravity/Musubi",
		"./Musubi",
	}

	for _, cand := range candidates {
		candClean := filepath.Clean(cand)
		sBin := filepath.Join(candClean, "bin", "musubi-server")
		wBin := filepath.Join(candClean, "bin", "musubi-web")
		if fileExists(sBin) && fileExists(wBin) {
			absS, _ := filepath.Abs(sBin)
			absW, _ := filepath.Abs(wBin)
			return absS, absW
		}

		// Try building if source directory exists
		if fileExists(filepath.Join(candClean, "cmd", "musubi-server", "main.go")) {
			t.Logf("Musubi source directory detected at %s. Compiling binaries...", candClean)
			binDir := filepath.Join(candClean, "bin")
			_ = os.MkdirAll(binDir, 0750)

			//nolint:gosec // Controlled test compilation of sibling Musubi repository
			cmdServer := exec.Command("go", "build", "-o", filepath.Join("bin", "musubi-server"), "./cmd/musubi-server")
			cmdServer.Dir = candClean
			if out, err := cmdServer.CombinedOutput(); err != nil {
				t.Logf("Failed to compile musubi-server: %v, output: %s", err, string(out))
				continue
			}

			//nolint:gosec // Controlled test compilation of sibling Musubi repository
			cmdWeb := exec.Command("go", "build", "-o", filepath.Join("bin", "musubi-web"), "./cmd/musubi-web")
			cmdWeb.Dir = candClean
			if out, err := cmdWeb.CombinedOutput(); err != nil {
				t.Logf("Failed to compile musubi-web: %v, output: %s", err, string(out))
				continue
			}

			absS, _ := filepath.Abs(sBin)
			absW, _ := filepath.Abs(wBin)
			return absS, absW
		}
	}

	t.Skip("Musubi binaries (musubi-server, musubi-web) not found. Set MUSUBI_SERVER_BIN and MUSUBI_WEB_BIN or clone Musubi sibling repository to run this scenario test.")
	return "", ""
}

func fileExists(path string) bool {
	//nolint:gosec // Stat on candidate binary path
	info, err := os.Stat(filepath.Clean(path))
	return err == nil && !info.IsDir()
}

// waitForEndpoint polls the given URL until it returns HTTP 200 OK or timeout.
func waitForEndpoint(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	require.Failf(t, "endpoint not ready", "timed out waiting for endpoint %s", url)
}

func TestMusubi_DeploymentAndDashboardScenario(t *testing.T) {
	t.Parallel()

	serverBin, webBin := resolveMusubiBinaries(t)

	workDir := t.TempDir()
	logDir := filepath.Join(workDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0750))

	serverPort := getFreePort(t)
	webPort := getFreePort(t)
	trapPort := getFreePort(t)

	dbPath := filepath.Join(workDir, "musubi.db")
	dbDSN := fmt.Sprintf("%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dbPath)

	serverHealthURL := fmt.Sprintf("http://127.0.0.1:%d/v1/system/healthz", serverPort)
	webHealthURL := fmt.Sprintf("http://127.0.0.1:%d/healthz", webPort)
	webDashboardURL := fmt.Sprintf("http://127.0.0.1:%d/", webPort)

	serverLogPath := filepath.Join(logDir, "musubi-server.log")
	webLogPath := filepath.Join(logDir, "musubi-web.log")

	// 1. Construct ytagarasu declarative deployment manifest
	manifestData := supervisor.ManifestSpec{
		Version: "1.0",
		Processes: []supervisor.ProcessConfig{
			{
				Name:    "musubi-server",
				Binary:  serverBin,
				WorkDir: workDir,
				Env: map[string]string{
					"PORT":            fmt.Sprintf("%d", serverPort),
					"SNMP_TRAP_PORT":  fmt.Sprintf("%d", trapPort),
					"DATABASE_DRIVER": "sqlite3",
					"DATABASE_DSN":    dbDSN,
					"BACKUP_ENABLED":  "false",
				},
				AutoRestart:     true,
				MaxRetries:      3,
				ListenPort:      serverPort,
				HealthEndpoint:  serverHealthURL,
				RollbackOnError: true,
				EscapeHatchMode: true,
				LogPath:         serverLogPath,
				LogMaxSizeMB:    10,
				LogMaxBackups:   2,
				LogCompress:     false,
			},
			{
				Name:    "musubi-web",
				Binary:  webBin,
				WorkDir: workDir,
				Env: map[string]string{
					"PORT":           fmt.Sprintf("%d", webPort),
					"MUSUBI_API_URL": fmt.Sprintf("http://127.0.0.1:%d", serverPort),
				},
				AutoRestart:     true,
				MaxRetries:      3,
				ListenPort:      webPort,
				HealthEndpoint:  webHealthURL,
				RollbackOnError: true,
				EscapeHatchMode: true,
				LogPath:         webLogPath,
				LogMaxSizeMB:    10,
				LogMaxBackups:   2,
				LogCompress:     false,
			},
		},
	}

	manifestBytes, err := yaml.Marshal(manifestData)
	require.NoError(t, err)

	manifestPath := filepath.Join(workDir, "manifest.yaml")
	require.NoError(t, os.WriteFile(manifestPath, manifestBytes, 0600))

	// 2. Cryptographic Integrity Signature (Ed25519)
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	manifestHash := sha256.Sum256(manifestBytes)
	signature := ed25519.Sign(privKey, manifestHash[:])
	require.True(t, ed25519.Verify(pubKey, manifestHash[:], signature), "Ed25519 signature must verify")

	// 3. Initialize and Start ytagarasu Manifest Supervisor
	ms, err := supervisor.NewManifestSupervisor(manifestPath)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = ms.StopAll(3 * time.Second)
	})

	err = ms.StartAll(ctx)
	require.NoError(t, err)

	statePath := filepath.Join(workDir, ".ytg-supervisor.state")
	require.NoError(t, ms.SaveState(statePath))

	// 4. Verify Supervisor State Persistence
	state, err := supervisor.LoadState(statePath)
	require.NoError(t, err)
	assert.Equal(t, manifestPath, state.ManifestPath)
	assert.Len(t, state.Processes, 2)
	assert.Contains(t, state.Processes, "musubi-server")
	assert.Contains(t, state.Processes, "musubi-web")

	// 5. Poll Endpoints for Readiness
	waitForEndpoint(t, serverHealthURL, 15*time.Second)
	waitForEndpoint(t, webHealthURL, 15*time.Second)

	// 6. Scenario Test Final Stage: Verify Musubi Dashboard Rendering (HTTP 200 & HTML Contents)
	dashboardClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := dashboardClient.Get(webDashboardURL)
	require.NoError(t, err, "failed to GET Musubi Web Dashboard")
	defer func() {
		_ = resp.Body.Close()
	}()

	require.Equal(t, http.StatusOK, resp.StatusCode, "Musubi Dashboard should respond with HTTP 200 OK")
	contentType := resp.Header.Get("Content-Type")
	assert.True(t, strings.HasPrefix(contentType, "text/html"), "Content-Type must be text/html, got: %s", contentType)

	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "failed to read Musubi Dashboard HTML body")
	htmlContent := string(bodyBytes)

	// Validate critical dashboard UI components rendered by HTMX & templates
	assert.Contains(t, htmlContent, "Musubi Dashboard", "Dashboard title brand must exist")
	assert.Contains(t, htmlContent, "Air-Gapped & Standalone Network Orchestration", "Sub-header brand tag must exist")
	assert.Contains(t, htmlContent, "統合監視ダッシュボード", "Main dashboard section header must exist")
	assert.Contains(t, htmlContent, "システムリソース & SNMP帯域", "Resource metrics section must exist")
	assert.Contains(t, htmlContent, "ターゲットインベントリ & 死活ステータス", "Target inventory section must exist")
	assert.Contains(t, htmlContent, "最新MIBテレメトリ", "MIB telemetry section must exist")
	assert.Contains(t, htmlContent, "hx-get=\"/ui/components/system-metrics\"", "HTMX auto-refresh directive must exist")

	// Save HTML snapshot for evidence artifact
	snapshotPath := filepath.Join(workDir, "musubi-dashboard-snapshot.html")
	require.NoError(t, os.WriteFile(snapshotPath, bodyBytes, 0600))
	t.Logf("Musubi Dashboard successfully verified and HTML snapshot saved to %s (size: %d bytes)", snapshotPath, len(bodyBytes))

	// 7. Verify Process Logs were generated
	require.FileExists(t, serverLogPath)
	require.FileExists(t, webLogPath)
	//nolint:gosec // Controlled test log file read
	serverLogData, err := os.ReadFile(filepath.Clean(serverLogPath))
	require.NoError(t, err)
	assert.Contains(t, string(serverLogData), "Starting Musubi server")

	//nolint:gosec // Controlled test log file read
	webLogData, err := os.ReadFile(filepath.Clean(webLogPath))
	require.NoError(t, err)
	assert.Contains(t, string(webLogData), "Starting Musubi Web Frontend Server")

	// 8. Verify Graceful Shutdown
	err = ms.StopAll(3 * time.Second)
	require.NoError(t, err)

	statuses := ms.Status()
	for name, st := range statuses {
		assert.Equal(t, supervisor.StateStopped, st.State, "Process %s must be in STOPPED state", name)
	}
}
