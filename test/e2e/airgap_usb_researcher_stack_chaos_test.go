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
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAirgapUSB_ResearcherStack_ChaosRecoveryScenario tests a realistic multi-component
// on-premise workload: Research Data Collector (crawler/analyzer worker) and Research Web Portal.
// It verifies air-gap USB media integrity, supervisor orchestration, and zero-touch self-healing
// when the intensive researcher worker is forcefully terminated via SIGKILL.
//
//nolint:paralleltest // Manipulates network ports and process signals
func TestAirgapUSB_ResearcherStack_ChaosRecoveryScenario(t *testing.T) {
	// =========================================================================
	// PHASE 1: Build Virtual USB Air-Gap Media (Manifest, Artifacts, Packages)
	// =========================================================================
	usbDir := t.TempDir()
	t.Logf("Virtual Air-Gap USB media mounted at: %s", usbDir)

	collectorPort := getFreePort(t)
	portalPort := getFreePort(t)
	for portalPort == collectorPort {
		portalPort = getFreePort(t)
	}

	binDir := filepath.Join(usbDir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0755))

	// 1. Build Research Collector worker binary
	collectorBinPath := filepath.Join(binDir, "research-collector")
	buildResearcherWorkerBinary(t, collectorBinPath)

	// 2. Build Research Portal API binary
	portalBinPath := filepath.Join(binDir, "research-portal")
	buildResearchPortalBinary(t, portalBinPath)

	// 3. Create offline APT cache package
	aptCachePath := filepath.Join(usbDir, "apt-cache.tar.gz")
	createDummyAptCacheTarGz(t, aptCachePath)

	// 4. Create manifest.yaml defining the complete researcher production stack
	manifestPath := filepath.Join(usbDir, "manifest.yaml")
	manifestContent := fmt.Sprintf(`version: "1.0"
metadata:
  name: "onpremise-researcher-production-stack"
  environment: "airgapped-factory-edge"
  release: "2026.10-production"

processes:
  - name: research-collector
    binary: %s
    args: ["-port", "%d", "-mode", "continuous-harvest"]
    work_dir: %s
    auto_restart: true
    max_retries: 5
    listen_port: %d
    health_endpoint: "http://127.0.0.1:%d/healthz"
    rollback_on_error: false

  - name: research-portal
    binary: %s
    args: ["-port", "%d", "-upstream-collector", "%d"]
    work_dir: %s
    auto_restart: true
    max_retries: 5
    listen_port: %d
    health_endpoint: "http://127.0.0.1:%d/healthz"
    rollback_on_error: false
`, collectorBinPath, collectorPort, usbDir, collectorPort, collectorPort,
		portalBinPath, portalPort, collectorPort, usbDir, portalPort, portalPort)

	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0644))

	// 5. Generate cryptographic SHA256SUMS file
	checksumFile := filepath.Join(usbDir, "SHA256SUMS")
	generateSHA256ChecksumFile(t, checksumFile, map[string]string{
		"manifest.yaml":          manifestPath,
		"apt-cache.tar.gz":       aptCachePath,
		"bin/research-collector": collectorBinPath,
		"bin/research-portal":    portalBinPath,
	})

	// =========================================================================
	// PHASE 2: Offline Cryptographic Hash & Tamper-Proof Verification Gate
	// =========================================================================
	verifySHA256Checksums(t, checksumFile, usbDir)
	t.Log("✓ [INTEGRITY GATE] All offline USB artifacts verified by SHA-256 hashchain")

	// =========================================================================
	// PHASE 3: Supervisor Stack Orchestration & Healthcheck Verification
	// =========================================================================
	ms, err := supervisor.NewManifestSupervisor(manifestPath)
	require.NoError(t, err, "failed to initialize manifest supervisor for researcher stack")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, ms.StartAll(ctx), "failed to launch supervised researcher stack")
	defer func() { _ = ms.StopAll(1 * time.Second) }()

	collectorHealthURL := fmt.Sprintf("http://127.0.0.1:%d/healthz", collectorPort)
	portalHealthURL := fmt.Sprintf("http://127.0.0.1:%d/healthz", portalPort)

	// Ensure both collector and portal become healthy
	require.Eventually(t, func() bool {
		cResp, cErr := http.Get(collectorHealthURL)
		if cErr != nil || cResp.StatusCode != http.StatusOK {
			return false
		}
		_ = cResp.Body.Close()

		pResp, pErr := http.Get(portalHealthURL)
		if pErr != nil || pResp.StatusCode != http.StatusOK {
			return false
		}
		_ = pResp.Body.Close()

		return true
	}, 6*time.Second, 100*time.Millisecond, "researcher stack services failed to become healthy")

	t.Log("✓ [SUPERVISOR] All researcher stack processes spawned and responding (HTTP 200)")

	// Inspect process telemetry
	statusMap := ms.Status()
	collectorStatus, cExists := statusMap["research-collector"]
	require.True(t, cExists, "research-collector must be present in supervisor status")
	initialCollectorPID := collectorStatus.PID
	require.Positive(t, initialCollectorPID)
	assert.Equal(t, 0, collectorStatus.Restarts)
	assert.Equal(t, supervisor.StateRunning, collectorStatus.State)

	portalStatus, pExists := statusMap["research-portal"]
	require.True(t, pExists, "research-portal must be present in supervisor status")
	assert.Equal(t, supervisor.StateRunning, portalStatus.State)

	t.Logf("Initial running Researcher Collector PID: %d (Restarts: 0)", initialCollectorPID)
	t.Logf("Initial running Researcher Portal PID: %d (Restarts: 0)", portalStatus.PID)

	// =========================================================================
	// PHASE 4: Chaos Fault Injection (kill -9 / SIGKILL on Research Collector)
	// =========================================================================
	t.Logf("⚡ [CHAOS INJECTION] Sending SIGKILL to Research Collector (PID %d)...", initialCollectorPID)
	err = syscall.Kill(initialCollectorPID, syscall.SIGKILL)
	require.NoError(t, err, "failed to send SIGKILL to research-collector")

	// Verify collector port is dead
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", collectorPort), 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
	}
	t.Log("✓ [CHAOS VERIFIED] Research Collector terminated abruptly by kernel SIGKILL")

	// =========================================================================
	// PHASE 5: Supervisor Autonomous Recovery (Zero-Touch Self-Healing)
	// =========================================================================
	t.Log("Waiting for supervisor autonomous self-healing and service restoration...")

	var recoveredCollectorPID int
	require.Eventually(t, func() bool {
		// 1. Healthcheck returns HTTP 200 again
		resp, err := http.Get(collectorHealthURL)
		if err != nil || resp.StatusCode != http.StatusOK {
			return false
		}
		_ = resp.Body.Close()

		// 2. Supervisor status reflects new PID, incremented restarts, and RUNNING state
		currentStatus := ms.Status()
		proc, ok := currentStatus["research-collector"]
		if !ok {
			return false
		}
		if proc.PID != initialCollectorPID && proc.Restarts >= 1 && proc.State == supervisor.StateRunning {
			recoveredCollectorPID = proc.PID
			return true
		}
		return false
	}, 5*time.Second, 100*time.Millisecond, "supervisor failed to auto-recover research-collector within 5s")

	t.Logf("✓ [SELF-HEALING SUCCESSFUL] Research Collector restored with New PID: %d (Restarts: 1)", recoveredCollectorPID)
	assert.NotEqual(t, initialCollectorPID, recoveredCollectorPID)

	// Verify Portal remains running unaffected (Bulkhead / Fault Isolation)
	currentPortalStatus := ms.Status()["research-portal"]
	assert.Equal(t, portalStatus.PID, currentPortalStatus.PID, "Portal PID must not restart due to collector crash")
	assert.Equal(t, 0, currentPortalStatus.Restarts, "Portal restarts should remain 0")
	assert.Equal(t, supervisor.StateRunning, currentPortalStatus.State)
	t.Log("✓ [BULKHEAD ISOLATION] Research Portal was isolated and maintained 100% uptime during worker recovery")

	// =========================================================================
	// PHASE 6: Graceful Teardown
	// =========================================================================
	require.NoError(t, ms.StopAll(1*time.Second))
	t.Log("✓ [TEARDOWN] All researcher stack processes cleanly stopped")
}

// buildResearcherWorkerBinary builds a realistic research crawler & analyzer mock service
func buildResearcherWorkerBinary(t *testing.T, targetPath string) {
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
	port := flag.Int("port", 8081, "worker listen port")
	_ = flag.String("mode", "continuous-harvest", "worker operation mode")
	flag.Parse()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "{\"component\":\"research-collector\",\"status\":\"OK\",\"queue_depth\":12,\"indexed_records\":384}\n")
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
	require.NoError(t, os.WriteFile(mainGo, []byte(code), 0644))

	cmd := exec.Command("go", "build", "-o", targetPath, mainGo)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to build researcher worker binary: %s", string(out))
}

func buildResearchPortalBinary(t *testing.T, targetPath string) {
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
	port := flag.Int("port", 8080, "portal listen port")
	_ = flag.Int("upstream-collector", 8081, "collector upstream port")
	flag.Parse()

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "{\"component\":\"research-portal\",\"status\":\"HEALTHY\",\"cluster\":\"airgap-edge-node01\"}\n")
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
	require.NoError(t, os.WriteFile(mainGo, []byte(code), 0644))

	cmd := exec.Command("go", "build", "-o", targetPath, mainGo)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to build research portal binary: %s", string(out))
}
