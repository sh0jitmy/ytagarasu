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

//go:build matrix_test

package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/agent"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// matrixRunner tracks package command execution during matrix E2E.
type matrixRunner struct {
	executedCommands []string
}

func (m *matrixRunner) Run(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmdStr := fmt.Sprintf("%s %v", name, args)
	m.executedCommands = append(m.executedCommands, cmdStr)
	return []byte("simulated success output"), nil
}

// TestPackageMatrix_AllEcosystems verifies offline deployment for all supported package managers:
// APT (Debian/Ubuntu), DNF (RHEL/Rocky), Pip (Python wheels), and Docker (Container tarballs).
func TestPackageMatrix_AllEcosystems(t *testing.T) {
	t.Parallel()

	ecosystems := []struct {
		managerName     string
		packages        []manifest.PackageItem
		expectedCommand string
	}{
		{
			managerName: "apt",
			packages: []manifest.PackageItem{
				{Name: "libssl3", Version: "3.0.2"},
				{Name: "ca-certificates"},
			},
			expectedCommand: "apt-get",
		},
		{
			managerName: "dnf",
			packages: []manifest.PackageItem{
				{Name: "openssl-libs", Version: "3.0.7"},
				{Name: "curl"},
			},
			expectedCommand: "dnf",
		},
		{
			managerName: "pip",
			packages: []manifest.PackageItem{
				{Name: "pydantic", Version: "2.6.4"},
				{Name: "uvicorn"},
			},
			expectedCommand: "python3",
		},
		{
			managerName: "docker",
			packages: []manifest.PackageItem{
				{Name: "/opt/bundles/containers/app-engine.tar"},
			},
			expectedCommand: "docker",
		},
	}

	for _, tc := range ecosystems {
		tc := tc
		t.Run("Ecosystem_"+tc.managerName, func(t *testing.T) {
			t.Parallel()

			runner := &matrixRunner{}
			multiMgr := agent.NewMultiPackageManager(nil)
			multiMgr.Register("apt", agent.NewAptPackageManager(runner))
			multiMgr.Register("dnf", agent.NewRpmPackageManager(runner))
			multiMgr.Register("pip", agent.NewPipPackageManager(runner, "python3", "/opt/wheels"))
			multiMgr.Register("docker", agent.NewDockerPackageManager(runner))

			// Mock server providing a manifest targeting this ecosystem
			manifestYAML := fmt.Sprintf(`
version: "1.0"
bundleVersion: "2026.09.27.matrix"
release: "v1.0.0"
targets:
  - os: "linux"
    release: "generic"
    arch: "amd64"
applications:
  - name: "sample-app"
    type: "golang"
    destination: "bin/sample-app"
packages:
  dep_set:
    manager: "%s"
    items:
`, tc.managerName)
			for _, pkg := range tc.packages {
				if pkg.Version != "" {
					manifestYAML += fmt.Sprintf("      - name: %q\n        version: %q\n", pkg.Name, pkg.Version)
				} else {
					manifestYAML += fmt.Sprintf("      - name: %q\n", pkg.Name)
				}
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/x-yaml")
				w.Header().Set("X-Release-ID", "matrix-"+tc.managerName+"-v1.0.0")
				w.Header().Set("X-Release-Version", "v1.0.0")
				_, _ = w.Write([]byte(manifestYAML))
			}))
			defer server.Close()

			workDir := t.TempDir()
			stateFile := filepath.Join(workDir, "state.json")
			lockFile := filepath.Join(workDir, "agent.lock")
			installDir := filepath.Join(workDir, "install")
			require.NoError(t, os.MkdirAll(installDir, 0750))

			ag := agent.NewAgent(agent.Config{
				ServerURL:    server.URL,
				ServiceID:    "matrix-" + tc.managerName,
				StateFile:    stateFile,
				LockFile:     lockFile,
				InstallDir:   installDir,
				PollInterval: 10 * time.Second,
				Roles:        []string{"all"},
			}, multiMgr)

			report, err := ag.StepOnce(context.Background())
			require.NoError(t, err)
			require.NotNil(t, report)
			assert.True(t, report.Success)

			// Assert that the expected manager tool was invoked
			require.NotEmpty(t, runner.executedCommands)
			assert.Contains(t, runner.executedCommands[0], tc.expectedCommand)
		})
	}
}
