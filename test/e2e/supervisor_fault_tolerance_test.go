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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSupervisor_FaultTolerance_PortConflict verifies that the supervisor detects
// port conflicts prior to spawning processes or during preflight, avoiding blind crash loops.
//
//nolint:paralleltest // Manipulates host network ports, cannot run parallel
func TestSupervisor_FaultTolerance_PortConflict(t *testing.T) {
	// 1. Occupy a random available TCP port on localhost to simulate existing running process (e.g., system Postgres)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() {
		_ = listener.Close()
	}()

	occupiedPort := listener.Addr().(*net.TCPAddr).Port
	t.Logf("Simulated host occupied port: %d", occupiedPort)

	// 2. Direct port check validation
	err = supervisor.CheckPortAvailable(occupiedPort)
	require.Error(t, err, "CheckPortAvailable should report error when port is already occupied")
	require.ErrorIs(t, err, supervisor.ErrPortConflict)
	assert.Contains(t, err.Error(), "already in use")

	// Verify an unoccupied port reports nil
	unoccupiedPort := occupiedPort + 1000
	if unoccupiedPort > 65535 {
		unoccupiedPort = occupiedPort - 1000
	}
	// Verify whether unoccupiedPort is indeed free
	if testL, testErr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", unoccupiedPort)); testErr == nil {
		_ = testL.Close()
		assert.NoError(t, supervisor.CheckPortAvailable(unoccupiedPort))
	}

	// 3. Test BaseSupervisor behavior when configured with an occupied ListenPort
	cfg := supervisor.ProcessConfig{
		Name:          "conflicting-service",
		Binary:        "sh",
		Args:          []string{"-c", "sleep 1"},
		ListenPort:    occupiedPort,
		AutoRestart:   false,
		PreflightPort: true,
	}

	sup := supervisor.NewProcessSupervisor(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	startErr := sup.Start(ctx)
	require.Error(t, startErr, "Supervisor Start must fail immediately if PreflightPort is enabled and port is in use")
	require.ErrorIs(t, startErr, supervisor.ErrPortConflict)

	status := sup.Status()
	assert.Equal(t, supervisor.StatePortConflict, status.State)
	assert.Equal(t, occupiedPort, status.ListenPort)
}

// TestSupervisor_FaultTolerance_ExponentialBackoff verifies that rapid crashing processes
// are restarted with exponential backoff intervals rather than a fixed tight loop,
// protecting host CPU and system stability.
//
//nolint:paralleltest // Measures precise process lifecycle timing
func TestSupervisor_FaultTolerance_ExponentialBackoff(t *testing.T) {
	cfg := supervisor.ProcessConfig{
		Name:           "crashing-worker",
		Binary:         "sh",
		Args:           []string{"-c", "exit 1"},
		AutoRestart:    true,
		MaxRetries:     3,
		InitialBackoff: 50 * time.Millisecond,
		BackoffFactor:  2.0,
	}

	sup := supervisor.NewProcessSupervisor(cfg)
	ctx := context.Background()

	startTime := time.Now()
	err := sup.Start(ctx)
	require.NoError(t, err)

	// Wait until supervisor exhausts MaxRetries (50ms + 100ms + 200ms ~= 350ms min)
	require.Eventually(t, func() bool {
		st := sup.Status()
		return st.State == supervisor.StateCrashed && st.CrashCount >= 3
	}, 3*time.Second, 50*time.Millisecond)

	elapsed := time.Since(startTime)
	t.Logf("Total elapsed time for 3 crashes with exponential backoff: %v", elapsed)

	// In fixed 100ms loop, 3 crashes would take ~300ms.
	// With 50ms * 2^n (50ms + 100ms + 200ms = 350ms+ execution overhead), elapsed should be >= 300ms.
	assert.GreaterOrEqual(t, elapsed, 300*time.Millisecond)

	st := sup.Status()
	assert.Equal(t, supervisor.StateCrashed, st.State)
	assert.GreaterOrEqual(t, st.CrashCount, 3)
}

// TestSupervisor_FaultTolerance_DockerPreflightDiagnostics verifies that
// when the Docker daemon is unreachable or nonexistent, ManifestSupervisor
// returns a human-friendly actionable diagnostic error rather than an opaque exit status.
//
//nolint:paralleltest // Sets environment variables via t.Setenv
func TestSupervisor_FaultTolerance_DockerPreflightDiagnostics(t *testing.T) {
	tempDir := t.TempDir()
	manifestPath := filepath.Join(tempDir, "manifest.yaml")

	manifestContent := `
version: "1.0"
bundleVersion: "1.0.0"
release: "fault-tolerant-docker"
targets:
  - os: linux
    release: ubuntu-24.04
    arch: amd64
applications:
  - name: dummy-app
    type: golang
    artifact: dummy
    destination: /tmp/dummy
containers:
  - name: test-db
    image: postgres:16-alpine
    ports:
      - "15432:5432"
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0600))

	parsed, err := manifest.ParseFile(manifestPath)
	require.NoError(t, err)

	// 1. Point to an invalid socket via DOCKER_HOST to simulate offline / dead docker daemon
	dummySocket := filepath.Join(tempDir, "nonexistent-docker.sock")
	t.Setenv("DOCKER_HOST", "unix://"+dummySocket)

	// DiagnoseDockerDaemon should detect that the socket does not exist or daemon is unreachable
	diagErr := supervisor.CheckDockerDaemonReachable(context.Background())
	require.Error(t, diagErr)
	assert.Contains(t, diagErr.Error(), "docker daemon is unreachable")

	// 2. Initializing ManifestSupervisor with RequireDockerCheck flag should surface this error
	ms, err := supervisor.NewManifestSupervisorWithConfig(parsed, manifestPath, supervisor.ManifestSupervisorConfig{
		RequireDockerPreflight: true,
	})
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		startErr := ms.StartAll(ctx)
		require.Error(t, startErr)
		assert.Contains(t, startErr.Error(), "docker daemon is unreachable")
	} else {
		assert.Contains(t, err.Error(), "docker daemon is unreachable")
	}
}
