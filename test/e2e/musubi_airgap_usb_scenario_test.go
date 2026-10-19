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
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/agent"
	"github.com/sh0jitmy/ytagarasu/internal/bundle"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestMusubi_Airgap_USBSimulationScenario verifies end-to-end air-gapped deployment:
// 1. Exporting assets (binaries, container tar images, manifest, Ed25519 signature) to USB media.
// 2. Strict cryptographic integrity verification of USB media contents.
// 3. Offline deployment on an isolated air-gapped host (network fully severed).
// 4. Offline loading of container images via `docker load -i` (zero external registry access).
// 5. Manifest-driven process supervision of hybrid Docker containers and native Musubi processes.
// 6. Complete verification of Musubi Web Dashboard and Grafana HTML rendering.
func TestMusubi_Airgap_USBSimulationScenario(t *testing.T) {

	// =========================================================================
	// PHASE 1: Online / Staging Phase - Export Data & Manifest to USB Media
	// =========================================================================
	usbMountDir := t.TempDir()
	usbArtifactsDir := filepath.Join(usbMountDir, "artifacts")
	usbImagesDir := filepath.Join(usbMountDir, "images")
	usbSignaturesDir := filepath.Join(usbMountDir, "signatures")
	require.NoError(t, os.MkdirAll(usbArtifactsDir, 0750))
	require.NoError(t, os.MkdirAll(usbImagesDir, 0750))
	require.NoError(t, os.MkdirAll(usbSignaturesDir, 0750))

	// 1-1. Copy Musubi Binaries to USB media
	srcServerBin, srcWebBin := resolveMusubiBinaries(t)
	usbServerBin := filepath.Join(usbArtifactsDir, "musubi-server")
	usbWebBin := filepath.Join(usbArtifactsDir, "musubi-web")
	copyFile(t, srcServerBin, usbServerBin, 0755)
	copyFile(t, srcWebBin, usbWebBin, 0755)

	// 1-2. Export Docker Container Tar Images to USB media (simulating `docker save`)
	pgTarPath := filepath.Join(usbImagesDir, "postgres.tar")
	grafanaTarPath := filepath.Join(usbImagesDir, "grafana.tar")
	require.NoError(t, os.WriteFile(pgTarPath, []byte("tar-archive-postgres:16-alpine-simulated"), 0600))
	require.NoError(t, os.WriteFile(grafanaTarPath, []byte("tar-archive-grafana:10.4.0-simulated"), 0600))

	// 1-3. Generate Air-Gapped Manifest for USB Media
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

	// Host-side mock docker CLI that supports both `docker load -i` and `docker run`
	hostDockerDir := filepath.Join(t.TempDir(), "bin")
	dockerBin := buildMockDockerBinary(t, hostDockerDir)

	hostWorkDir := t.TempDir()
	logDir := filepath.Join(hostWorkDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0750))

	airgapManifest := struct {
		Version       string                            `yaml:"version"`
		BundleVersion string                            `yaml:"bundleVersion"`
		Release       string                            `yaml:"release"`
		Metadata      map[string]string                 `yaml:"metadata"`
		Targets       []manifest.Target                 `yaml:"targets"`
		Applications  []manifest.Application            `yaml:"applications"`
		Packages      map[string]manifest.PackageTarget `yaml:"packages"`
		Processes     []supervisor.ProcessConfig        `yaml:"processes"`
	}{
		Version:       "1.0",
		BundleVersion: "1.0.0",
		Release:       "1.0.0",
		Metadata: map[string]string{
			"name":        "musubi-airgap-docker-hybrid",
			"environment": "airgap-usb",
		},
		Targets: []manifest.Target{
			{OS: "linux", Release: "ubuntu-24.04", Arch: "amd64"},
		},
		Applications: []manifest.Application{
			{
				Name:        "musubi-server",
				Type:        manifest.AppTypeGolang,
				Destination: "/opt/musubi/bin/musubi-server",
			},
			{
				Name:        "musubi-web",
				Type:        manifest.AppTypeHTMX,
				Destination: "/opt/musubi/bin/musubi-web",
			},
		},
		Packages: map[string]manifest.PackageTarget{
			"container-images": {
				Manager: "docker",
				Order:   1,
				Items: []manifest.PackageItem{
					{Name: pgTarPath, Version: "16-alpine"},
					{Name: grafanaTarPath, Version: "10.4.0"},
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
				Binary:  usbServerBin,
				WorkDir: hostWorkDir,
				Env: map[string]string{
					"PORT":            fmt.Sprintf("%d", serverPort),
					"SNMP_TRAP_PORT":  fmt.Sprintf("%d", trapPort),
					"DATABASE_DRIVER": "sqlite3",
					"DATABASE_DSN":    filepath.Join(hostWorkDir, "musubi-airgap.db") + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)",
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
				Binary:  usbWebBin,
				WorkDir: hostWorkDir,
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

	manifestBytes, err := yaml.Marshal(airgapManifest)
	require.NoError(t, err)

	usbManifestPath := filepath.Join(usbMountDir, "manifest.yaml")
	require.NoError(t, os.WriteFile(usbManifestPath, manifestBytes, 0600))

	// 1-4. Generate Ed25519 Cryptographic Signature & Public Key
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	manifestSig := ed25519.Sign(privKey, manifestBytes)
	sigPath := filepath.Join(usbSignaturesDir, "manifest.sig")
	pubKeyPath := filepath.Join(usbSignaturesDir, "public.key")
	require.NoError(t, os.WriteFile(sigPath, manifestSig, 0600))
	require.NoError(t, os.WriteFile(pubKeyPath, []byte(hex.EncodeToString(pubKey)), 0600))

	// 1-5. Compute SHA-256 Checksums on USB Media
	checksumsContent := computeChecksums(t, usbMountDir)
	checksumsPath := filepath.Join(usbMountDir, "checksums")
	require.NoError(t, os.WriteFile(checksumsPath, []byte(checksumsContent), 0600))

	// 1-6. Package into Portable Bundle Archive on USB media (bundle-musubi-hybrid-airgap-1.0.tar.gz)
	usbBundleArchive := filepath.Join(usbMountDir, "bundle-musubi-hybrid-airgap-1.0.tar.gz")
	createTarGz(t, usbMountDir, usbBundleArchive)

	// Verify all USB media assets exist
	assert.FileExists(t, usbManifestPath)
	assert.FileExists(t, usbServerBin)
	assert.FileExists(t, usbWebBin)
	assert.FileExists(t, pgTarPath)
	assert.FileExists(t, grafanaTarPath)
	assert.FileExists(t, sigPath)
	assert.FileExists(t, pubKeyPath)
	assert.FileExists(t, checksumsPath)
	assert.FileExists(t, usbBundleArchive)
	t.Logf("PHASE 1: USB media export completed. Assets successfully written to: %s", usbMountDir)

	// =========================================================================
	// PHASE 2: Complete Air-Gap Environment Simulation (External Network Severed)
	// =========================================================================
	// Route all outbound HTTP/HTTPS proxy to an invalid local port to ensure zero internet access
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("ALL_PROXY", "http://127.0.0.1:1")
	t.Setenv("GOPROXY", "off")
	t.Logf("PHASE 2: Air-gapped network isolation active. Outbound external connectivity severed.")

	// =========================================================================
	// PHASE 3: Cryptographic Integrity & Anti-Tampering Verification on USB Media
	// =========================================================================
	verifyRes, err := bundle.VerifyBundle(usbBundleArchive, pubKey)
	require.NoError(t, err)
	if !verifyRes.Valid {
		t.Fatalf("bundle verification failed: errors=%v, sigValid=%v, checksumValid=%v", verifyRes.Errors, verifyRes.SignatureValid, verifyRes.ChecksumsValid)
	}
	require.True(t, verifyRes.Valid, "USB bundle archive must pass cryptographic verification")
	require.True(t, verifyRes.SignatureValid, "Ed25519 digital signature must be valid")
	require.True(t, verifyRes.ChecksumsValid, "SHA-256 checksums must all match")
	assert.NotNil(t, verifyRes.Manifest)
	assert.Equal(t, "1.0.0", verifyRes.Manifest.Release)

	// Anti-Tampering Test: Verify that modifying 1 byte in bundle triggers verification failure
	tamperedArchive := filepath.Join(t.TempDir(), "tampered-bundle.tar.gz")
	archiveData, err := os.ReadFile(filepath.Clean(usbBundleArchive)) //nolint:gosec
	require.NoError(t, err)
	if len(archiveData) > 50 {
		archiveData[45] ^= 0xFF
	}
	require.NoError(t, os.WriteFile(filepath.Clean(tamperedArchive), archiveData, 0600)) //nolint:gosec
	tamperedRes, verifyErr := bundle.VerifyBundle(tamperedArchive, pubKey)
	assert.True(t, verifyErr != nil || (tamperedRes != nil && !tamperedRes.Valid), "Tampered bundle archive must be rejected by verification")
	t.Logf("PHASE 3: Cryptographic verification passed. Tampering detection confirmed.")

	// =========================================================================
	// PHASE 4: Offline Container Images Loading via `docker load -i`
	// =========================================================================
	runner := &fakeTestRunner{}
	dockerPkgMgr := agent.NewDockerPackageManager(runner)

	ctx := context.Background()
	offlineItems := []manifest.PackageItem{
		{Name: pgTarPath},
		{Name: grafanaTarPath},
	}
	err = dockerPkgMgr.InstallPackages(ctx, offlineItems)
	require.NoError(t, err)

	require.Len(t, runner.commands, 2)
	assert.Contains(t, runner.commands[0], "docker load -i")
	assert.Contains(t, runner.commands[0], pgTarPath)
	assert.Contains(t, runner.commands[1], "docker load -i")
	assert.Contains(t, runner.commands[1], grafanaTarPath)
	t.Logf("PHASE 4: Offline container images loaded via docker load -i. Zero registry pulls occurred.")

	// =========================================================================
	// PHASE 5: Launch Manifest-Driven Process Supervisor from USB Media
	// =========================================================================
	ms, err := supervisor.NewManifestSupervisor(usbManifestPath)
	require.NoError(t, err)

	supCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		_ = ms.StopAll(3 * time.Second)
	})

	err = ms.StartAll(supCtx)
	require.NoError(t, err)

	// Verify all 4 processes (2 Docker containers + 2 Native processes) are RUNNING
	statuses := ms.Status()
	assert.Len(t, statuses, 4)
	for name, st := range statuses {
		assert.Equal(t, supervisor.StateRunning, st.State, "Process %s must be RUNNING", name)
		assert.Positive(t, st.PID, "Process %s must have valid PID", name)
	}

	// Poll endpoints for readiness in air-gapped environment
	waitForTCPPort(t, pgAddr, 10*time.Second)
	waitForEndpoint(t, serverHealthURL, 15*time.Second)
	waitForEndpoint(t, webHealthURL, 15*time.Second)
	waitForEndpoint(t, grafanaHealthURL, 10*time.Second)

	// =========================================================================
	// PHASE 6: End-to-End Verification of Musubi & Grafana Web Dashboards
	// =========================================================================
	client := &http.Client{Timeout: 5 * time.Second}

	// 6-1. Musubi HTMX Web Dashboard
	respWeb, err := client.Get(webDashboardURL)
	require.NoError(t, err)
	defer func() { _ = respWeb.Body.Close() }()
	assert.Equal(t, http.StatusOK, respWeb.StatusCode)
	bodyWeb, err := io.ReadAll(respWeb.Body)
	require.NoError(t, err)
	assert.Contains(t, string(bodyWeb), "Musubi Dashboard")
	assert.Contains(t, string(bodyWeb), "統合監視ダッシュボード")

	// 6-2. Grafana Telemetry Dashboard
	respGrafana, err := client.Get(grafanaDashboardURL)
	require.NoError(t, err)
	defer func() { _ = respGrafana.Body.Close() }()
	assert.Equal(t, http.StatusOK, respGrafana.StatusCode)
	bodyGrafana, err := io.ReadAll(respGrafana.Body)
	require.NoError(t, err)
	assert.Contains(t, string(bodyGrafana), "Grafana Container")
	assert.Contains(t, string(bodyGrafana), "Grafana Dockerized")

	t.Logf("PHASE 6: Both Musubi Dashboard and Grafana successfully rendered in full air-gap environment!")

	// =========================================================================
	// PHASE 7: Graceful Shutdown & Post-Run Cleanup
	// =========================================================================
	require.NoError(t, ms.StopAll(3*time.Second))
	finalStatuses := ms.Status()
	for name, st := range finalStatuses {
		assert.Equal(t, supervisor.StateStopped, st.State, "Process %s must be gracefully STOPPED", name)
	}
	t.Logf("PHASE 7: Air-gapped deployment scenario successfully concluded.")
}

func copyFile(t *testing.T, src, dst string, mode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(src)) //nolint:gosec
	require.NoError(t, err, "failed to read source file: %s", src)
	require.NoError(t, os.WriteFile(filepath.Clean(dst), data, mode), "failed to write destination file: %s", dst) //nolint:gosec
}

func computeChecksums(t *testing.T, rootDir string) string {
	t.Helper()
	var lines string
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(rootDir, path)
		if relErr != nil {
			return relErr
		}
		if rel == "checksums" || rel == "signatures/manifest.sig" || filepath.Ext(rel) == ".gz" {
			return nil
		}
		data, readErr := os.ReadFile(filepath.Clean(path)) //nolint:gosec
		if readErr != nil {
			return readErr
		}
		h := sha256.Sum256(data)
		lines += fmt.Sprintf("%x  %s\n", h, filepath.ToSlash(rel))
		return nil
	})
	require.NoError(t, err)
	return lines
}

func createTarGz(t *testing.T, sourceDir, archivePath string) {
	t.Helper()
	f, err := os.Create(filepath.Clean(archivePath)) //nolint:gosec
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == archivePath {
			return nil
		}
		rel, relErr := filepath.Rel(sourceDir, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		header, hdrErr := tar.FileInfoHeader(info, info.Name())
		if hdrErr != nil {
			return hdrErr
		}
		header.Name = filepath.ToSlash(rel)
		if writeHdrErr := tw.WriteHeader(header); writeHdrErr != nil {
			return writeHdrErr
		}
		if info.IsDir() {
			return nil
		}
		fileData, openErr := os.Open(filepath.Clean(path)) //nolint:gosec
		if openErr != nil {
			return openErr
		}
		defer func() { _ = fileData.Close() }()
		_, copyErr := io.Copy(tw, fileData)
		return copyErr
	})
	require.NoError(t, err)
}
