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
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/delta"
	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_DesiredStateDiff_ReconciliationSafety verifies that ytg diff accurately detects
// workload drift between desired manifest and runtime state, enforcing manual approval safety.
//
//nolint:paralleltest // CLI execution and temporary filesystem interaction
func TestE2E_DesiredStateDiff_ReconciliationSafety(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Create Desired Manifest (Release v2.0-secure)
	manifestPath := filepath.Join(tempDir, "desired-manifest.yaml")
	manifestContent := `
version: "1.0"
bundleVersion: "2.0.0"
release: "v2.0-secure"
targets:
  - os: linux
    release: ubuntu-24.04
    arch: amd64
containers:
  - name: frontend-proxy
    image: nginx:alpine
    ports:
      - "80:80"
  - name: backend-api
    image: mycorp/api:v2.0
    ports:
      - "8443:8443"
applications:
  - name: telemetry-agent
    type: golang
    artifact: agent
    destination: /opt/telemetry
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0600))

	// 2. Create Actual Runtime State (v1.0 running workloads)
	statePath := filepath.Join(tempDir, "actual_state.json")
	state := supervisor.SupervisorState{
		ManifestPath: manifestPath,
		UpdatedAt:    time.Now().Add(-2 * time.Hour),
		Processes: map[string]supervisor.ProcessRuntimeStatus{
			"frontend-proxy": {
				Name:       "frontend-proxy",
				PID:        3101,
				State:      supervisor.StateRunning,
				ListenPort: 80, // In sync -> RETAIN
			},
			"backend-api": {
				Name:       "backend-api",
				PID:        3102,
				State:      supervisor.StateRunning,
				ListenPort: 8080, // Desired is 8443 -> UPDATE
			},
			"deprecated-worker": {
				Name:  "deprecated-worker",
				PID:   2999,
				State: supervisor.StateRunning, // Missing in v2.0 -> DELETE
			},
		},
	}
	stateBytes, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(statePath, stateBytes, 0600))

	// 3. Build ytg binary to test real CLI interaction
	ytgBin := filepath.Join(tempDir, "ytg")
	// #nosec G204 -- test execution of go build
	buildCmd := exec.Command("go", "build", "-o", ytgBin, "github.com/sh0jitmy/ytagarasu/cmd/ytg")
	buildOut, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "Failed to compile ytg binary for E2E: %s", string(buildOut))

	// 4. Execute `ytg diff` without --approve: must detect drift and exit with safety guard (exit 2)
	// #nosec G204 -- test execution of compiled binary
	diffCmd := exec.Command(ytgBin, "diff", "-f", manifestPath, "-s", statePath)
	diffOut, diffErr := diffCmd.CombinedOutput()
	require.Error(t, diffErr, "ytg diff without --approve must exit with non-zero safety status")
	diffStr := string(diffOut)

	t.Logf("CLI Output:\n%s", diffStr)

	// Verify Tactical Cyberpunk × Enterprise CUD symbols in stdout
	assert.Contains(t, diffStr, "=== RECONCILIATION PLAN: RELEASE V2.0-SECURE ===")
	assert.Contains(t, diffStr, "= [■ RETAIN] frontend-proxy")
	assert.Contains(t, diffStr, "~ [▲ UPDATE] backend-api")
	assert.Contains(t, diffStr, "+ [◆ CREATE] telemetry-agent")
	assert.Contains(t, diffStr, "- [✖ DELETE] deprecated-worker")
	assert.Contains(t, diffStr, "SAFETY GUARD: Changes require approval")

	// 5. Execute `ytg diff` with `--approve`: must succeed with exit code 0
	// #nosec G204 -- test execution of compiled binary
	approveCmd := exec.Command(ytgBin, "diff", "-f", manifestPath, "-s", statePath, "--approve")
	approveOut, approveErr := approveCmd.CombinedOutput()
	require.NoError(t, approveErr, "ytg diff with --approve must exit with code 0: %s", string(approveOut))
	assert.Contains(t, string(approveOut), "DRIFT DETECTED")

	// 6. Execute `ytg diff` with `--json`: verify machine-readable telemetry output
	// #nosec G204 -- test execution of compiled binary
	jsonCmd := exec.Command(ytgBin, "diff", "-f", manifestPath, "-s", statePath, "--json")
	jsonOut, jsonErr := jsonCmd.CombinedOutput()
	require.NoError(t, jsonErr, "ytg diff --json must exit cleanly")

	var plan delta.ReconcilePlan
	require.NoError(t, json.Unmarshal(jsonOut, &plan))
	assert.Equal(t, "v2.0-secure", plan.ReleaseName)
	assert.True(t, plan.RequiresApproval)
	assert.Len(t, plan.Drifts, 4)
}
