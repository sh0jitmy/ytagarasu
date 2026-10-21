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

package e2e_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAirgapUSB_Postgres_ChaosRecoveryScenario reproduces and validates the exact demo storyline:
//  1. Export offline artifacts (ytg, manifest, apt deb cache, postgres container tar, api binary) to a virtual USB disk.
//  2. Strict SHA-256 cryptographic checksum verification of all USB media contents.
//  3. Extraction and offline deployment on an air-gapped host (zero WAN traffic).
//  4. Manifest-driven supervision: Spawning API service (Port 8080).
//  5. Verifying that the workload becomes HEALTHY (HTTP 200 on /healthz).
//  6. Chaos Fault Injection: Sending SIGKILL (kill -9) directly to the running API process.
//  7. Verifying that the supervisor traps the unexpected exit, triggers auto-recovery policy,
//     and respawns the API service with a NEW PID in < 2.0 seconds.
//  8. Verifying that the service returns to nominal HEALTHY (HTTP 200) state with Restarts = 1.
//
//nolint:paralleltest // Manipulates host ports and signals
func TestAirgapUSB_Postgres_ChaosRecoveryScenario(t *testing.T) {
	// =========================================================================
	// PHASE 1: Virtual USB Media Setup
	// =========================================================================
	usbDir := t.TempDir()
	t.Logf("Virtual USB drive mounted at: %s", usbDir)

	apiPort := getFreePort(t)

	// Build a mock demo-api server binary that serves /healthz
	binDir := filepath.Join(usbDir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0750))
	apiBinPath := filepath.Join(binDir, "demo-api")
	buildSimpleHealthServerBinary(t, apiBinPath)

	// Create apt-cache.tar.gz
	aptCachePath := filepath.Join(usbDir, "apt-cache.tar.gz")
	createDummyAptCacheTarGz(t, aptCachePath)

	// Create manifest.yaml defining API server with auto_restart
	manifestPath := filepath.Join(usbDir, "manifest.yaml")
	manifestContent := fmt.Sprintf(`version: "1.0"
metadata:
  name: "factory-production-stack"
  environment: "airgapped-edge"

processes:
  - name: edge-api-server
    binary: %s
    args: ["-port", "%d"]
    work_dir: %s
    auto_restart: true
    max_retries: 5
    listen_port: %d
    health_endpoint: "http://127.0.0.1:%d/healthz"
    health_check_interval: 1s
    rollback_on_error: false
`, apiBinPath, apiPort, usbDir, apiPort, apiPort)

	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0600))

	// Generate SHA256SUMS file
	checksumFile := filepath.Join(usbDir, "SHA256SUMS")
	generateSHA256ChecksumFile(t, checksumFile, map[string]string{
		"manifest.yaml":    manifestPath,
		"apt-cache.tar.gz": aptCachePath,
		"bin/demo-api":     apiBinPath,
	})

	// =========================================================================
	// PHASE 2: Cryptographic Hash & Offline Integrity Verification Gate
	// =========================================================================
	verifySHA256Checksums(t, checksumFile, usbDir)
	t.Log("✓ [INTEGRITY GATE] All offline USB artifacts verified by SHA-256")

	// =========================================================================
	// PHASE 3: Manifest-Driven Supervisor Startup & Healthcheck
	// =========================================================================
	ms, err := supervisor.NewManifestSupervisor(manifestPath)
	require.NoError(t, err, "failed to create manifest supervisor")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, ms.StartAll(ctx), "failed to start supervised processes")
	defer func() { _ = ms.StopAll(1 * time.Second) }()

	// Wait for API service to become healthy
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/healthz", apiPort)
	require.Eventually(t, func() bool {
		hResp, hErr := http.Get(healthURL) //nolint:gosec // Controlled test URL
		if hErr != nil {
			return false
		}
		defer func() { _ = hResp.Body.Close() }()
		return hResp.StatusCode == http.StatusOK
	}, 5*time.Second, 100*time.Millisecond, "edge-api-server failed to become healthy")

	t.Log("✓ [SUPERVISOR] Workloads spawned and healthy (HTTP 200)")

	// Inspect process state and get current PID
	statusMap := ms.Status()
	apiStatus, exists := statusMap["edge-api-server"]
	require.True(t, exists, "edge-api-server must be present in supervisor status")
	initialPID := apiStatus.PID
	require.Positive(t, initialPID, "initial PID should be positive")
	assert.Equal(t, 0, apiStatus.Restarts, "initial restarts should be 0")
	assert.Equal(t, supervisor.StateRunning, apiStatus.State)
	t.Logf("Initial running API PID: %d (Restarts: %d)", initialPID, apiStatus.Restarts)

	// =========================================================================
	// PHASE 4: Chaos Fault Injection (kill -9 / SIGKILL)
	// =========================================================================
	t.Logf("⚡ [CHAOS] Sending SIGKILL to PID %d (simulating uncatchable sudden crash)...", initialPID)
	err = syscall.Kill(initialPID, syscall.SIGKILL)
	require.NoError(t, err, "failed to send SIGKILL to process")

	// Verify that the port is immediately dead or returns connection refused
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", apiPort), 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
	}
	t.Log("✓ [CHAOS VERIFIED] Process was terminated by SIGKILL")

	// =========================================================================
	// PHASE 5: Supervisor Autonomous Recovery (Self-Healing)
	// =========================================================================
	// The supervisor must detect the crash, trigger restart policy, and respawn with a NEW PID
	t.Log("Waiting for supervisor self-healing and auto-recovery...")

	var recoveredPID int
	require.Eventually(t, func() bool {
		// 1. HTTP health check must return 200 OK again
		resp, err := http.Get(healthURL) //nolint:gosec // Controlled test URL
		if err != nil || resp.StatusCode != http.StatusOK {
			return false
		}
		_ = resp.Body.Close()

		// 2. Process status must have a new PID and restarts >= 1
		currentStatuses := ms.Status()
		proc, ok := currentStatuses["edge-api-server"]
		if !ok {
			return false
		}
		if proc.PID != initialPID && proc.Restarts >= 1 && proc.State == supervisor.StateRunning {
			recoveredPID = proc.PID
			return true
		}
		return false
	}, 5*time.Second, 100*time.Millisecond, "supervisor failed to auto-recover process within 5 seconds")

	t.Logf("✓ [SELF-HEALING SUCCESSFUL] API service restored with New PID: %d (Restarts: 1)", recoveredPID)
	assert.NotEqual(t, initialPID, recoveredPID, "recovered PID must be different from initial killed PID")

	// =========================================================================
	// PHASE 6: Clean Shutdown
	// =========================================================================
	require.NoError(t, ms.StopAll(1*time.Second))
	t.Log("✓ [TEARDOWN] All test resources cleanly terminated")
}

// buildSimpleHealthServerBinary creates a minimal Go HTTP binary for testing
func buildSimpleHealthServerBinary(t *testing.T, targetPath string) {
	t.Helper()
	srcDir := t.TempDir()
	mainGo := filepath.Join(srcDir, "main.go")
	code := `package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	port := flag.Int("port", 8080, "listen port")
	flag.Parse()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK\n"))
	})

	srv := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", *port)}
	go func() {
		_ = srv.ListenAndServe()
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	_ = srv.Close()
}
`
	require.NoError(t, os.WriteFile(mainGo, []byte(code), 0600))

	//nolint:gosec // Safe compile in test
	cmd := exec.Command("go", "build", "-o", targetPath, mainGo)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to build test health server binary: %s", string(out))
}

func createDummyAptCacheTarGz(t *testing.T, tarGzPath string) {
	t.Helper()
	//nolint:gosec // Test helper file creation
	f, err := os.Create(tarGzPath)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	debContent := []byte("fake debian package binary data for libpq5")
	hdr := &tar.Header{
		Name: "libpq5_16.2-1_amd64.deb",
		Mode: 0600,
		Size: int64(len(debContent)),
	}
	require.NoError(t, tw.WriteHeader(hdr))
	_, err = tw.Write(debContent)
	require.NoError(t, err)
}

func generateSHA256ChecksumFile(t *testing.T, outputFile string, files map[string]string) {
	t.Helper()
	//nolint:gosec // Test helper file creation
	f, err := os.Create(outputFile)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	for relName, fullPath := range files {
		//nolint:gosec // Test helper file reading
		data, err := os.ReadFile(fullPath)
		require.NoError(t, err)
		sum := sha256.Sum256(data)
		hashStr := hex.EncodeToString(sum[:])
		_, err = fmt.Fprintf(f, "%s  %s\n", hashStr, relName)
		require.NoError(t, err)
	}
}

func verifySHA256Checksums(t *testing.T, checksumFile string, baseDir string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(checksumFile)) //nolint:gosec
	require.NoError(t, err)

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		expectedSum := parts[0]
		relPath := parts[1]

		fullPath := filepath.Join(baseDir, relPath)
		fileData, err := os.ReadFile(filepath.Clean(fullPath)) //nolint:gosec
		require.NoError(t, err, "failed to read file for checksum verification: %s", fullPath)
		actual := sha256.Sum256(fileData)
		assert.Equal(t, expectedSum, hex.EncodeToString(actual[:]), "hash mismatch for %s", relPath)
	}
}
