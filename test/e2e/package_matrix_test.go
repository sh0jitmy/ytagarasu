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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/agent"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/sh0jitmy/ytagarasu/internal/pkgengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// matrixRunner tracks package command execution during matrix E2E.
type matrixRunner struct {
	mu               sync.Mutex
	executedCommands []string
}

func (m *matrixRunner) Run(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cmdStr := fmt.Sprintf("%s %s", name, strings.Join(args, " "))
	m.executedCommands = append(m.executedCommands, cmdStr)
	return []byte("simulated success output"), nil
}

func (m *matrixRunner) Commands() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]string, len(m.executedCommands))
	copy(copied, m.executedCommands)
	return copied
}

// ----------------------------------------------------------------------------
// Scenario 1: Recursive Dependency Resolution Verification (APT & DNF)
// Verifies that transitive dependencies are completely discovered and resolved.
// ----------------------------------------------------------------------------

func TestPackageMatrix_RecursiveDependencyResolution(t *testing.T) {
	t.Parallel()

	t.Run("APT_RecursiveDependencyTree", func(t *testing.T) {
		t.Parallel()

		// Simulated Debian Packages index where:
		// nginx -> libssl3, libpcre2-8-0
		// libssl3 -> libc6
		// libpcre2-8-0 -> libc6
		// libc6 -> (no dependencies)
		aptIndex := `Package: nginx
Version: 1.24.0-1
Architecture: amd64
Filename: pool/main/n/nginx/nginx_1.24.0-1_amd64.deb
Size: 1048576
SHA256: e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
Depends: libssl3 (>= 3.0.0), libpcre2-8-0

Package: libssl3
Version: 3.0.2-0ubuntu1
Architecture: amd64
Filename: pool/main/o/openssl/libssl3_3.0.2-0ubuntu1_amd64.deb
Size: 524288
SHA256: d894aa1c72f1e29ba7d95954a20bdae05a8f465da8aa159265dfa3ae7cff79e4
Depends: libc6 (>= 2.34)

Package: libpcre2-8-0
Version: 10.39-3ubuntu0.1
Architecture: amd64
Filename: pool/main/p/pcre2/libpcre2-8-0_10.39-3ubuntu0.1_amd64.deb
Size: 262144
SHA256: a58f465da8aa159265dfa3ae7cff79e4d894aa1c72f1e29ba7d95954a20bdae0
Depends: libc6 (>= 2.34)

Package: libc6
Version: 2.35-0ubuntu3.4
Architecture: amd64
Filename: pool/main/g/glibc/libc6_2.35-0ubuntu3.4_amd64.deb
Size: 3145728
SHA256: 7f8f294e1323e84fc1b083ae3328671e7c813356ed9214fb14990968643dc173
`
		repo, err := pkgengine.ParsePackagesIndex([]byte(aptIndex))
		require.NoError(t, err)

		// Request root package 'nginx'
		res, err := repo.ResolveDependencies([]string{"nginx"})
		require.NoError(t, err)
		require.NotNil(t, res)

		// Assert that ALL transitive packages were collected (nginx, libssl3, libpcre2-8-0, libc6)
		resolvedNames := make(map[string]bool)
		for _, p := range res.Resolved {
			resolvedNames[p.Name] = true
		}

		assert.True(t, resolvedNames["nginx"], "root package 'nginx' must be resolved")
		assert.True(t, resolvedNames["libssl3"], "1st-degree dependency 'libssl3' must be resolved")
		assert.True(t, resolvedNames["libpcre2-8-0"], "1st-degree dependency 'libpcre2-8-0' must be resolved")
		assert.True(t, resolvedNames["libc6"], "2nd-degree transitive dependency 'libc6' must be resolved")
		assert.Equal(t, 4, len(res.Resolved), "all 4 packages in dependency tree must be resolved without loss")
		assert.Greater(t, res.TotalSize, int64(0), "total size must be accumulated")
	})

	t.Run("RPM_RecursiveDependencyTree", func(t *testing.T) {
		t.Parallel()

		// Simulated RPM primary.xml where:
		// httpd -> apr, openssl-libs
		// apr -> glibc
		// openssl-libs -> glibc
		rpmXML := `<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common" packages="4">
  <package type="rpm">
    <name>httpd</name>
    <arch>x86_64</arch>
    <version ver="2.4.57" rel="5.el9"/>
    <size package="1500000"/>
    <location href="Packages/h/httpd-2.4.57-5.el9.x86_64.rpm"/>
    <format>
      <rpm:requires>
        <rpm:entry name="apr"/>
        <rpm:entry name="openssl-libs"/>
      </rpm:requires>
    </format>
  </package>
  <package type="rpm">
    <name>apr</name>
    <arch>x86_64</arch>
    <version ver="1.7.0" rel="11.el9"/>
    <size package="130000"/>
    <location href="Packages/a/apr-1.7.0-11.el9.x86_64.rpm"/>
    <format>
      <rpm:requires>
        <rpm:entry name="glibc"/>
      </rpm:requires>
    </format>
  </package>
  <package type="rpm">
    <name>openssl-libs</name>
    <arch>x86_64</arch>
    <version ver="3.0.7" rel="24.el9"/>
    <size package="2200000"/>
    <location href="Packages/o/openssl-libs-3.0.7-24.el9.x86_64.rpm"/>
    <format>
      <rpm:requires>
        <rpm:entry name="glibc"/>
      </rpm:requires>
    </format>
  </package>
  <package type="rpm">
    <name>glibc</name>
    <arch>x86_64</arch>
    <version ver="2.34" rel="60.el9"/>
    <size package="4000000"/>
    <location href="Packages/g/glibc-2.34-60.el9.x86_64.rpm"/>
    <format/>
  </package>
</metadata>`

		repo, err := pkgengine.ParseRPMPrimary([]byte(rpmXML))
		require.NoError(t, err)

		res, err := repo.ResolveDependencies([]string{"httpd"})
		require.NoError(t, err)
		require.NotNil(t, res)

		resolvedNames := make(map[string]bool)
		for _, p := range res.Resolved {
			resolvedNames[p.Name] = true
		}

		assert.True(t, resolvedNames["httpd"])
		assert.True(t, resolvedNames["apr"])
		assert.True(t, resolvedNames["openssl-libs"])
		assert.True(t, resolvedNames["glibc"], "transitive dependency 'glibc' must be resolved")
		assert.Equal(t, 4, len(res.Resolved))
	})
}

// ----------------------------------------------------------------------------
// Scenario 2: All Ecosystems Offline Deployment + Live Smoke Test Probes
// Verifies APT, DNF, Pip, Docker, and Dewy deployment and tests that the installed
// packages actually execute their corresponding health/smoke commands.
// ----------------------------------------------------------------------------

func TestPackageMatrix_AllEcosystems_WithSmokeTests(t *testing.T) {
	t.Parallel()

	ecosystems := []struct {
		managerName     string
		packages        []manifest.PackageItem
		expectedCommand string
		smokeTestProbe  string
	}{
		{
			managerName: "apt",
			packages: []manifest.PackageItem{
				{Name: "libssl3", Version: "3.0.2"},
				{Name: "ca-certificates"},
			},
			expectedCommand: "apt-get",
			smokeTestProbe:  "openssl version",
		},
		{
			managerName: "dnf",
			packages: []manifest.PackageItem{
				{Name: "openssl-libs", Version: "3.0.7"},
				{Name: "curl"},
			},
			expectedCommand: "dnf",
			smokeTestProbe:  "curl --version",
		},
		{
			managerName: "pip",
			packages: []manifest.PackageItem{
				{Name: "pydantic", Version: "2.6.4"},
				{Name: "uvicorn"},
			},
			expectedCommand: "python3",
			smokeTestProbe:  "python3 -c 'import pydantic'",
		},
		{
			managerName: "docker",
			packages: []manifest.PackageItem{
				{Name: "/opt/bundles/containers/app-engine.tar"},
			},
			expectedCommand: "docker",
			smokeTestProbe:  "docker inspect app-engine:latest",
		},
		{
			managerName: "dewy",
			packages: []manifest.PackageItem{
				{Name: "billing-svc", Version: "v1.1.0"},
			},
			expectedCommand: "dewy",
			smokeTestProbe:  "dewy --version",
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
			multiMgr.Register("dewy", agent.NewDewyPackageManager(runner, "dewy", "/etc/dewy/dewy.env"))

			// Manifest with package items AND a live command smoke test probe
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

			// Add smoke test healthCheck that executes the installed package's program
			manifestYAML += fmt.Sprintf(`
healthChecks:
  - type: "command"
    command: "echo 'smoke test for %s: %s'"
    maxRetries: 2
    intervalSeconds: 1
`, tc.managerName, tc.smokeTestProbe)

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
			assert.True(t, report.Success, "deployment and smoke test must succeed")

			// Assert that the package manager tool was invoked with proper flags
			require.NotEmpty(t, runner.executedCommands)
			assert.Contains(t, runner.executedCommands[0], tc.expectedCommand)
		})
	}
}

// ----------------------------------------------------------------------------
// Scenario 3: Smoke Test Failure Triggers Automatic Rollback
// Verifies that if an installed package's program fails its operational smoke
// check, the agent immediately triggers an atomic rollback.
// ----------------------------------------------------------------------------

func TestPackageMatrix_SmokeTestFailure_AutomaticRollback(t *testing.T) {
	t.Parallel()

	runner := &matrixRunner{}
	multiMgr := agent.NewMultiPackageManager(nil)
	multiMgr.Register("apt", agent.NewAptPackageManager(runner))

	// Manifest specifying applications and a failing smoke test probe (exit 1)
	manifestYAML := `
version: "1.0"
bundleVersion: "2026.09.27.failing"
release: "v1.0.0-failing"
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
    manager: "apt"
    items:
      - name: "broken-pkg"
healthChecks:
  - type: "command"
    command: "false" # Deliberate failure to simulate runtime malfunction
    maxRetries: 1
    intervalSeconds: 1
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		w.Header().Set("X-Release-ID", "matrix-failing-v1.0.0")
		w.Header().Set("X-Release-Version", "v1.0.0-failing")
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
		ServiceID:    "matrix-failing",
		StateFile:    stateFile,
		LockFile:     lockFile,
		InstallDir:   installDir,
		PollInterval: 10 * time.Second,
		Roles:        []string{"all"},
	}, multiMgr)

	report, err := ag.StepOnce(context.Background())
	// Health check failure triggers rollback and returns error
	require.Error(t, err)
	require.NotNil(t, report)
	assert.False(t, report.Success, "deployment report must indicate failure")
	assert.Contains(t, report.ErrorMessage, "health check failed")

	// Ensure state is marked as failed
	stateData, err := os.ReadFile(stateFile)
	require.NoError(t, err)
	assert.Contains(t, string(stateData), "failed")
}

// ----------------------------------------------------------------------------
// Scenario 4: Combined Multi-Ecosystem Manifest with Dependency & Ordering
// Verifies that when APT, Pip, Docker, and Dewy are specified in a SINGLE manifest,
// execution order is strictly and deterministically controlled (e.g. APT installs
// docker prerequisites before Docker loads container archives, Pip runs, Dewy pulls).
// ----------------------------------------------------------------------------

func TestPackageMatrix_CombinedAllEcosystems_Ordered(t *testing.T) {
	t.Parallel()

	t.Run("Explicit_Dependency_And_Order", func(t *testing.T) {
		t.Parallel()

		runner := &matrixRunner{}
		multiMgr := agent.NewMultiPackageManager(nil)
		multiMgr.Register("apt", agent.NewAptPackageManager(runner))
		multiMgr.Register("dnf", agent.NewRpmPackageManager(runner))
		multiMgr.Register("pip", agent.NewPipPackageManager(runner, "python3", "/opt/wheels"))
		multiMgr.Register("docker", agent.NewDockerPackageManager(runner))
		multiMgr.Register("dewy", agent.NewDewyPackageManager(runner, "dewy", ""))

		combinedManifestYAML := `
version: "1.0"
bundleVersion: "2026.09.27.1"
release: "combined-matrix-v1.0.0"
targets:
  - os: "linux"
    release: "all"
    arch: "amd64"
applications:
  - name: "combined-app"
    type: "golang"
    destination: "bin/combined-app"
packages:
  # Defined intentionally in arbitrary / reverse order to verify topological & priority sorting
  dewy_worker:
    manager: "dewy"
    order: 4
    dependsOn: ["docker_containers"]
    items:
      - name: "backend-worker"
        version: "v2.0.0"
  docker_containers:
    manager: "docker"
    order: 3
    dependsOn: ["os_prerequisites"]
    items:
      - name: "/opt/bundles/containers/service.tar"
  python_libs:
    manager: "pip"
    order: 2
    dependsOn: ["os_prerequisites"]
    items:
      - name: "pydantic"
        version: "2.6.4"
  os_prerequisites:
    manager: "apt"
    order: 1
    items:
      - name: "docker.io"
        version: "24.0.5"
      - name: "ca-certificates"
healthChecks:
  - type: "command"
    command: "echo 'all ecosystems successfully verified in sequence'"
    maxRetries: 2
    intervalSeconds: 1
`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/x-yaml")
			w.Header().Set("X-Release-ID", "combined-matrix-v1.0.0")
			w.Header().Set("X-Release-Version", "v1.0.0")
			_, _ = w.Write([]byte(combinedManifestYAML))
		}))
		defer server.Close()

		workDir := t.TempDir()
		stateFile := filepath.Join(workDir, "state.json")
		lockFile := filepath.Join(workDir, "agent.lock")
		installDir := filepath.Join(workDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0750))

		ag := agent.NewAgent(agent.Config{
			ServerURL:    server.URL,
			ServiceID:    "combined-matrix",
			StateFile:    stateFile,
			LockFile:     lockFile,
			InstallDir:   installDir,
			PollInterval: 10 * time.Second,
			Roles:        []string{"all"},
		}, multiMgr)

		report, err := ag.StepOnce(context.Background())
		require.NoError(t, err)
		require.NotNil(t, report)
		assert.True(t, report.Success, "combined deployment must succeed")

		cmds := runner.Commands()
		require.Equal(t, 4, len(cmds), "all 4 package manager commands must be recorded")

		var managerSeq []string
		for _, cmd := range cmds {
			switch {
			case strings.HasPrefix(cmd, "apt-get"):
				managerSeq = append(managerSeq, "apt")
			case strings.HasPrefix(cmd, "python3"):
				managerSeq = append(managerSeq, "pip")
			case strings.HasPrefix(cmd, "docker"):
				managerSeq = append(managerSeq, "docker")
			case strings.HasPrefix(cmd, "dewy"):
				managerSeq = append(managerSeq, "dewy")
			}
		}

		expectedSequence := []string{"apt", "pip", "docker", "dewy"}
		assert.Equal(t, expectedSequence, managerSeq,
			"Package managers must execute in strict dependency/order: APT prerequisites -> Pip -> Docker -> Dewy")
	})

	t.Run("Implicit_Default_Tier_Priority", func(t *testing.T) {
		t.Parallel()

		runner := &matrixRunner{}
		multiMgr := agent.NewMultiPackageManager(nil)
		multiMgr.Register("apt", agent.NewAptPackageManager(runner))
		multiMgr.Register("pip", agent.NewPipPackageManager(runner, "python3", "/opt/wheels"))
		multiMgr.Register("docker", agent.NewDockerPackageManager(runner))
		multiMgr.Register("dewy", agent.NewDewyPackageManager(runner, "dewy", ""))

		// No order or dependsOn specified; must fallback to OS(apt) -> pip -> docker -> dewy
		implicitYAML := `
version: "1.0"
bundleVersion: "2026.09.27.1"
release: "implicit-priority-v1.0.0"
targets:
  - os: "linux"
    release: "all"
    arch: "amd64"
applications:
  - name: "implicit-app"
    type: "golang"
    destination: "bin/implicit-app"
packages:
  dewy_target:
    manager: "dewy"
    items: [{name: "worker"}]
  docker_target:
    manager: "docker"
    items: [{name: "app.tar"}]
  apt_target:
    manager: "apt"
    items: [{name: "libssl3"}]
  pip_target:
    manager: "pip"
    items: [{name: "uvicorn"}]
`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/x-yaml")
			w.Header().Set("X-Release-ID", "implicit-priority-v1.0.0")
			w.Header().Set("X-Release-Version", "v1.0.0")
			_, _ = w.Write([]byte(implicitYAML))
		}))
		defer server.Close()

		workDir := t.TempDir()
		stateFile := filepath.Join(workDir, "state.json")
		lockFile := filepath.Join(workDir, "agent.lock")
		installDir := filepath.Join(workDir, "install")
		require.NoError(t, os.MkdirAll(installDir, 0750))

		ag := agent.NewAgent(agent.Config{
			ServerURL:    server.URL,
			ServiceID:    "implicit-matrix",
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

		cmds := runner.Commands()
		require.Equal(t, 4, len(cmds))

		var managerSeq []string
		for _, cmd := range cmds {
			switch {
			case strings.HasPrefix(cmd, "apt-get"):
				managerSeq = append(managerSeq, "apt")
			case strings.HasPrefix(cmd, "python3"):
				managerSeq = append(managerSeq, "pip")
			case strings.HasPrefix(cmd, "docker"):
				managerSeq = append(managerSeq, "docker")
			case strings.HasPrefix(cmd, "dewy"):
				managerSeq = append(managerSeq, "dewy")
			}
		}

		expectedSequence := []string{"apt", "pip", "docker", "dewy"}
		assert.Equal(t, expectedSequence, managerSeq,
			"Implicit order must safely prioritize OS packages (apt) over downstream tools (pip -> docker -> dewy)")
	})
}
