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

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/delta"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCLI_ManifestDiff_ExecutionAndFormat(t *testing.T) {
	t.Parallel()
	tempDir := t.TempDir()

	manifestPath := filepath.Join(tempDir, "manifest.yaml")
	manifestContent := `
version: "1.0"
bundleVersion: "1.0.0"
release: "diff-test"
targets:
  - os: linux
    release: ubuntu-24.04
    arch: amd64
containers:
  - name: web
    image: nginx:alpine
    ports:
      - "8080:80"
applications:
  - name: dummy
    type: golang
    artifact: dummy
    destination: /tmp/dummy
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0600))

	statePath := filepath.Join(tempDir, "supervisor_state.json")
	state := supervisor.SupervisorState{
		ManifestPath: manifestPath,
		UpdatedAt:    time.Now(),
		Processes: map[string]supervisor.ProcessRuntimeStatus{
			"web": {
				Name:       "web",
				PID:        5001,
				State:      supervisor.StateRunning,
				ListenPort: 8080,
			},
		},
	}
	stateBytes, err := json.Marshal(state)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(statePath, stateBytes, 0600))

	parsed, err := manifest.ParseFile(manifestPath)
	require.NoError(t, err)

	loadedState, err := supervisor.LoadState(statePath)
	require.NoError(t, err)

	plan := delta.DetectDrift(parsed, loadedState)
	require.NotNil(t, plan)

	// Since dummy application is in manifest but not in state, it should be marked CREATE
	drifts := make(map[string]delta.DriftAction)
	for _, d := range plan.Drifts {
		drifts[d.Name] = d.Action
	}
	assert.Equal(t, delta.ActionRetain, drifts["web"])
	assert.Equal(t, delta.ActionCreate, drifts["dummy"])

	report := plan.FormatDriftReport()
	assert.Contains(t, report, "= [■ RETAIN] web")
	assert.Contains(t, report, "+ [◆ CREATE] dummy")

	var jsonBuf bytes.Buffer
	enc := json.NewEncoder(&jsonBuf)
	require.NoError(t, enc.Encode(plan))
	assert.Contains(t, jsonBuf.String(), `"release_name":"diff-test"`)
}
