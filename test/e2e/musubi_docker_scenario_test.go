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
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/agent"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// buildMockDockerBinary compiles a lightweight mock Docker CLI that simulates container port binding.
func buildMockDockerBinary(t *testing.T, targetDir string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(targetDir, 0750))

	dockerSrc := `package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Docker CLI Mock v24.0.0")
		os.Exit(0)
	}

	cmd := os.Args[1]
	if cmd == "version" || cmd == "--version" {
		fmt.Println("Docker version 24.0.0, build mock")
		return
	}

	if cmd == "run" {
		var portMap string
		var image string
		for i := 2; i < len(os.Args); i++ {
			if os.Args[i] == "-p" && i+1 < len(os.Args) {
				portMap = os.Args[i+1]
				i++
			} else if !strings.HasPrefix(os.Args[i], "-") {
				image = os.Args[i]
			}
		}

		hostPort := "0"
		if portMap != "" {
			parts := strings.Split(portMap, ":")
			if len(parts) >= 1 {
				hostPort = parts[0]
			}
		}

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

		// Start simulated service based on image or port
		if strings.Contains(image, "grafana") || hostPort == "3000" {
			mux := http.NewServeMux()
			mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(` + "`" + `{"database":"ok","version":"10.4.0-docker"}` + "`" + `))
			})
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(` + "`" + `<!DOCTYPE html><html><head><title>Grafana Container</title></head><body><h1>Grafana Dockerized</h1></body></html>` + "`" + `))
			})
			srv := &http.Server{
				Addr: fmt.Sprintf(":%s", hostPort),
				Handler: mux,
				ReadHeaderTimeout: 5 * time.Second,
			}
			go func() {
				_ = srv.ListenAndServe()
			}()
			fmt.Printf("[mock-docker] container grafana started on :%s\n", hostPort)
			<-sig
			_ = srv.Close()
			return
		}

		// Default TCP listener for PostgreSQL or other services
		l, err := net.Listen("tcp", fmt.Sprintf(":%s", hostPort))
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mock-docker] listen failed: %v\n", err)
			os.Exit(1)
		}
		defer l.Close()
		fmt.Printf("[mock-docker] container service started on :%s\n", hostPort)
		go func() {
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				_ = c.Close()
			}
		}()
		<-sig
		fmt.Println("[mock-docker] container stopped")
		return
	}
}
`
	dockerFile := filepath.Join(targetDir, "mock_docker_main.go")
	dockerBin := filepath.Join(targetDir, "docker")
	require.NoError(t, os.WriteFile(dockerFile, []byte(dockerSrc), 0600))

	//nolint:gosec // Compile test mock docker CLI
	cmd := exec.Command("go", "build", "-o", dockerBin, dockerFile)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "failed to compile mock docker binary: %s", string(out))

	return dockerBin
}

type fakeTestRunner struct {
	commands []string
}

func (f *fakeTestRunner) Run(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	full := name + " " + strings.Join(args, " ")
	f.commands = append(f.commands, full)
	return []byte("success"), nil
}

// TestMusubi_Docker_RepositoryConfigurationPipeline verifies APT repo addition for Docker CE.
func TestMusubi_Docker_RepositoryConfigurationPipeline(t *testing.T) {
	t.Parallel()

	runner := &fakeTestRunner{}
	aptMgr := agent.NewAptPackageManager(runner).WithSudo(true)

	// Docker official repository target specification
	dockerTarget := manifest.PackageTarget{
		Manager:    "apt",
		RepoURL:    "https://download.docker.com/linux/ubuntu",
		RepoName:   "docker",
		Suite:      "noble",
		Components: []string{"stable"},
		Order:      1,
		Items: []manifest.PackageItem{
			{Name: "docker-ce"},
			{Name: "containerd.io"},
		},
	}

	ctx := context.Background()

	// 1. Verify dynamic repository configuration
	err := aptMgr.ConfigureNamedRepository(ctx, dockerTarget, "docker")
	require.NoError(t, err)

	require.NotEmpty(t, runner.commands)
	lastCmd := runner.commands[len(runner.commands)-1]
	assert.Contains(t, lastCmd, "sudo -n sh -c")
	assert.Contains(t, lastCmd, "/etc/apt/sources.list.d/docker.list")
	assert.Contains(t, lastCmd, "deb [trusted=yes] https://download.docker.com/linux/ubuntu noble stable")

	// 2. Verify repository cache update
	err = aptMgr.UpdateRepositories(ctx)
	require.NoError(t, err)
	updateCmd := runner.commands[len(runner.commands)-1]
	assert.Contains(t, updateCmd, "apt-get update")

	// 3. Verify package installation
	err = aptMgr.InstallPackages(ctx, dockerTarget.Items)
	require.NoError(t, err)
	installCmd := runner.commands[len(runner.commands)-1]
	assert.Contains(t, installCmd, "apt-get install -y --no-install-recommends")
	assert.Contains(t, installCmd, "docker-ce containerd.io")
}

// TestMusubi_Docker_HybridContainerSupervisionScenario verifies ytagarasu supervision of Docker containers + Musubi.
func TestMusubi_Docker_HybridContainerSupervisionScenario(t *testing.T) {
	t.Parallel()

	serverBin, webBin := resolveMusubiBinaries(t)

	workDir := t.TempDir()
	logDir := filepath.Join(workDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0750))

	dockerBin := buildMockDockerBinary(t, filepath.Join(workDir, "mock_docker"))

	pgPort := getFreePort(t)
	grafanaPort := getFreePort(t)
	serverPort := getFreePort(t)
	webPort := getFreePort(t)
	trapPort := getFreePort(t)

	pgAddr := fmt.Sprintf("127.0.0.1:%d", pgPort)
	serverHealthURL := fmt.Sprintf("http://127.0.0.1:%d/v1/system/healthz", serverPort)
	webHealthURL := fmt.Sprintf("http://127.0.0.1:%d/healthz", webPort)
	webDashboardURL := fmt.Sprintf("http://127.0.0.1:%d/", webPort)
	grafanaHealthURL := fmt.Sprintf("http://127.0.0.1:%d/api/health", grafanaPort)
	grafanaDashboardURL := fmt.Sprintf("http://127.0.0.1:%d/", grafanaPort)

	// 1. Construct Hybrid Manifest (Docker containers for PG/Grafana + Native Musubi)
	hybridManifest := struct {
		Version   string                            `yaml:"version"`
		Metadata  map[string]string                 `yaml:"metadata"`
		Packages  map[string]manifest.PackageTarget `yaml:"packages"`
		Processes []supervisor.ProcessConfig        `yaml:"processes"`
	}{
		Version: "1.0",
		Metadata: map[string]string{
			"name":        "musubi-hybrid-docker",
			"environment": "airgap-docker-hybrid",
		},
		Packages: map[string]manifest.PackageTarget{
			"docker-apt": {
				Manager:    "apt",
				Order:      1,
				RepoURL:    "https://download.docker.com/linux/ubuntu",
				RepoName:   "docker",
				Suite:      "noble",
				Components: []string{"stable"},
				Items: []manifest.PackageItem{
					{Name: "docker-ce"},
					{Name: "containerd.io"},
				},
			},
		},
		Processes: []supervisor.ProcessConfig{
			{
				Name:        "postgres-container",
				Binary:      dockerBin,
				Args:        []string{"run", "-p", fmt.Sprintf("%d:5432", pgPort), "postgres:16-alpine"},
				AutoRestart: true,
				MaxRetries:  3,
				ListenPort:  pgPort,
				LogPath:     filepath.Join(logDir, "postgres-docker.log"),
			},
			{
				Name:           "grafana-container",
				Binary:         dockerBin,
				Args:           []string{"run", "-p", fmt.Sprintf("%d:3000", grafanaPort), "grafana/grafana:10.4.0"},
				AutoRestart:    true,
				MaxRetries:     3,
				ListenPort:     grafanaPort,
				HealthEndpoint: grafanaHealthURL,
				LogPath:        filepath.Join(logDir, "grafana-docker.log"),
			},
			{
				Name:    "musubi-server",
				Binary:  serverBin,
				WorkDir: workDir,
				Env: map[string]string{
					"PORT":            fmt.Sprintf("%d", serverPort),
					"SNMP_TRAP_PORT":  fmt.Sprintf("%d", trapPort),
					"DATABASE_DRIVER": "sqlite3",
					"DATABASE_DSN":    filepath.Join(workDir, "musubi-hybrid.db") + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)",
					"BACKUP_ENABLED":  "false",
				},
				AutoRestart:    true,
				MaxRetries:     3,
				ListenPort:     serverPort,
				HealthEndpoint: serverHealthURL,
				LogPath:        filepath.Join(logDir, "musubi-server.log"),
			},
			{
				Name:    "musubi-web",
				Binary:  webBin,
				WorkDir: workDir,
				Env: map[string]string{
					"PORT":           fmt.Sprintf("%d", webPort),
					"MUSUBI_API_URL": fmt.Sprintf("http://127.0.0.1:%d", serverPort),
				},
				AutoRestart:    true,
				MaxRetries:     3,
				ListenPort:     webPort,
				HealthEndpoint: webHealthURL,
				LogPath:        filepath.Join(logDir, "musubi-web.log"),
			},
		},
	}

	manifestBytes, err := yaml.Marshal(hybridManifest)
	require.NoError(t, err)

	manifestPath := filepath.Join(workDir, "manifest-docker.yaml")
	require.NoError(t, os.WriteFile(manifestPath, manifestBytes, 0600))

	// 2. Cryptographic Integrity Signature (Ed25519)
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	manifestHash := sha256.Sum256(manifestBytes)
	signature := ed25519.Sign(privKey, manifestHash[:])
	require.True(t, ed25519.Verify(pubKey, manifestHash[:], signature), "Signature must verify")

	// 3. Initialize and Start Supervisor
	ms, err := supervisor.NewManifestSupervisor(manifestPath)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = ms.StopAll(3 * time.Second)
	})

	err = ms.StartAll(ctx)
	require.NoError(t, err)

	// 4. Verify all 4 processes (2 Docker containers + 2 Native processes) are RUNNING
	statuses := ms.Status()
	assert.Len(t, statuses, 4)
	for name, st := range statuses {
		assert.Equal(t, supervisor.StateRunning, st.State, "Process %s must be RUNNING", name)
		assert.Positive(t, st.PID, "Process %s must have valid PID", name)
	}

	// 5. Poll Endpoints for Readiness
	waitForTCPPort(t, pgAddr, 10*time.Second)
	waitForEndpoint(t, serverHealthURL, 15*time.Second)
	waitForEndpoint(t, webHealthURL, 15*time.Second)
	waitForEndpoint(t, grafanaHealthURL, 10*time.Second)

	// 6. Verify Musubi Dashboard Rendering (HTTP 200 & HTML Contents)
	client := &http.Client{Timeout: 5 * time.Second}
	respWeb, err := client.Get(webDashboardURL)
	require.NoError(t, err)
	defer func() {
		_ = respWeb.Body.Close()
	}()
	assert.Equal(t, http.StatusOK, respWeb.StatusCode)
	bodyWeb, err := io.ReadAll(respWeb.Body)
	require.NoError(t, err)
	assert.Contains(t, string(bodyWeb), "Musubi Dashboard")
	assert.Contains(t, string(bodyWeb), "統合監視ダッシュボード")

	// 7. Verify Containerized Grafana Dashboard Rendering (HTTP 200)
	respGrafana, err := client.Get(grafanaDashboardURL)
	require.NoError(t, err)
	defer func() {
		_ = respGrafana.Body.Close()
	}()
	assert.Equal(t, http.StatusOK, respGrafana.StatusCode)
	bodyGrafana, err := io.ReadAll(respGrafana.Body)
	require.NoError(t, err)
	assert.Contains(t, string(bodyGrafana), "Grafana Container")
	assert.Contains(t, string(bodyGrafana), "Grafana Dockerized")

	t.Logf("Hybrid Docker + Native Musubi supervision scenario successfully verified!")

	// 8. Graceful Shutdown
	require.NoError(t, ms.StopAll(3*time.Second))
	finalStatuses := ms.Status()
	for name, st := range finalStatuses {
		assert.Equal(t, supervisor.StateStopped, st.State, "Process %s must be STOPPED", name)
	}
}
