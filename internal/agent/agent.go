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

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/manifest"
)

// Config defines the deployment agent settings.
type Config struct {
	ServerURL    string
	ServiceID    string
	PollInterval time.Duration
	StateFile    string
	LockFile     string
	InstallDir   string
	Roles        []string
	Hostname     string
	HTTPClient   *http.Client
}

// DeployReport contains the outcome of a deployment attempt sent to the server audit API.
type DeployReport struct {
	ServiceID    string    `json:"service_id"`
	ReleaseID    string    `json:"release_id"`
	Version      string    `json:"version"`
	Success      bool      `json:"success"`
	ErrorMessage string    `json:"error_message,omitempty"`
	Host         string    `json:"host"`
	ExecutedAt   time.Time `json:"executed_at"`
	DurationMs   int64     `json:"duration_ms"`
}

// Agent orchestrates autonomous polling, artifact installation, and reporting.
type Agent struct {
	cfg        Config
	stateStore *StateStore
	pkgMgr     PackageManager
	client     *http.Client
}

// NewAgent constructs a new Agent instance.
func NewAgent(cfg Config, pkgMgr PackageManager) *Agent {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 10 * time.Second
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	if cfg.Hostname == "" {
		cfg.Hostname, _ = os.Hostname()
	}
	if cfg.StateFile == "" {
		cfg.StateFile = "./data/ytagarasu-agent/state.json"
	}
	if cfg.LockFile == "" {
		cfg.LockFile = "./data/ytagarasu-agent/agent.lock"
	}
	if cfg.InstallDir == "" {
		cfg.InstallDir = "."
	}

	return &Agent{
		cfg:        cfg,
		stateStore: NewStateStore(cfg.StateFile),
		pkgMgr:     pkgMgr,
		client:     cfg.HTTPClient,
	}
}

// StepOnce performs a single deployment check and application cycle.
func (a *Agent) StepOnce(ctx context.Context) (*DeployReport, error) {
	// 1. Acquire exclusive lock
	lock, err := AcquireLock(a.cfg.LockFile)
	if err != nil {
		if errors.Is(err, ErrLocked) {
			slog.Debug("agent lock held by another process, skipping cycle")
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("failed acquiring file lock: %w", err)
	}
	defer func() {
		_ = lock.Release()
	}()

	startTime := time.Now().UTC()

	// 2. Fetch desired manifest from ytagarasu-server
	reqURL := fmt.Sprintf("%s/api/v1/services/%s/desired", strings.TrimRight(a.cfg.ServerURL, "/"), a.cfg.ServiceID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed creating desired request: %w", err)
	}

	state, err := a.stateStore.Load()
	if err != nil {
		return nil, fmt.Errorf("failed loading current state: %w", err)
	}

	if state.CurrentReleaseID != "" {
		req.Header.Set("If-None-Match", state.CurrentReleaseID)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed connecting to ytagarasu-server: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotModified {
		slog.Debug("desired release unchanged (304 Not Modified)")
		state.LastCheckedAt = time.Now().UTC()
		_ = a.stateStore.Save(state)
		return nil, nil
	}

	if resp.StatusCode == http.StatusNotFound {
		slog.Debug("no active release for service on server", "service", a.cfg.ServiceID)
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned unexpected status: %s", resp.Status)
	}

	releaseID := resp.Header.Get("X-Release-ID")
	if releaseID == "" {
		releaseID = resp.Header.Get("ETag")
	}

	manifestBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading manifest body: %w", err)
	}

	m, err := manifest.Parse(manifestBytes)
	if err != nil {
		return nil, fmt.Errorf("failed parsing server manifest: %w", err)
	}

	if releaseID == "" {
		releaseID = m.Release
	}

	// If already running this exact release, skip
	if state.CurrentReleaseID == releaseID && state.Status == "synced" {
		slog.Debug("host already synchronized with release", "release", releaseID)
		state.LastCheckedAt = time.Now().UTC()
		_ = a.stateStore.Save(state)
		return nil, nil
	}

	slog.Info("new release detected, beginning deployment",
		"service", a.cfg.ServiceID,
		"new_release", releaseID,
		"prev_release", state.CurrentReleaseID,
	)

	// 3. Apply OS Packages if specified and manager is configured
	if a.pkgMgr != nil {
		for _, pkgTarget := range m.Packages {
			if len(pkgTarget.Items) > 0 {
				slog.Info("applying OS packages non-interactively", "manager", pkgTarget.Manager, "count", len(pkgTarget.Items))
				if pkgErr := a.pkgMgr.InstallPackages(ctx, pkgTarget.Items); pkgErr != nil {
					report := a.createReport(releaseID, m.Release, false, fmt.Sprintf("package install failed: %v", pkgErr), startTime)
					a.sendReport(ctx, report)
					state.Status = "failed"
					_ = a.stateStore.Save(state)
					return report, pkgErr
				}
			}
		}
	}

	// 4. Download and apply application artifacts
	for _, app := range m.Applications {
		if !app.Selector.Matches(a.cfg.Roles, a.cfg.Hostname) {
			continue
		}
		if app.Artifact == "" {
			continue
		}

		slog.Info("downloading application artifact", "app", app.Name, "artifact", app.Artifact)
		artifactURL := fmt.Sprintf("%s/repos/%s/%s", strings.TrimRight(a.cfg.ServerURL, "/"), a.cfg.ServiceID, app.Artifact)
		if downloadErr := a.downloadArtifact(ctx, artifactURL, app); downloadErr != nil {
			report := a.createReport(releaseID, m.Release, false, fmt.Sprintf("artifact download failed: %v", downloadErr), startTime)
			a.sendReport(ctx, report)
			state.Status = "failed"
			_ = a.stateStore.Save(state)
			return report, downloadErr
		}
	}

	// 5. Update State
	state.ServiceID = a.cfg.ServiceID
	state.CurrentReleaseID = releaseID
	state.CurrentVersion = m.Release
	state.LastDeployedAt = time.Now().UTC()
	state.LastCheckedAt = time.Now().UTC()
	state.Status = "synced"
	if err := a.stateStore.Save(state); err != nil {
		slog.Error("failed saving agent state", "error", err)
	}

	// 6. Submit Success Report
	report := a.createReport(releaseID, m.Release, true, "", startTime)
	a.sendReport(ctx, report)

	slog.Info("autonomous deployment succeeded",
		"service", a.cfg.ServiceID,
		"release", releaseID,
		"duration_ms", report.DurationMs,
	)

	return report, nil
}

func (a *Agent) downloadArtifact(ctx context.Context, artifactURL string, app manifest.Application) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, artifactURL, nil)
	if err != nil {
		return err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("artifact server returned HTTP %s for %s", resp.Status, artifactURL)
	}

	destPath := app.Destination
	if destPath == "" {
		destPath = filepath.Join(a.cfg.InstallDir, "bin", app.Name)
	} else if !filepath.IsAbs(destPath) {
		destPath = filepath.Join(a.cfg.InstallDir, destPath)
	}
	cleanDest := filepath.Clean(destPath)

	if dirErr := os.MkdirAll(filepath.Dir(cleanDest), 0750); dirErr != nil {
		return fmt.Errorf("failed creating artifact destination directory: %w", dirErr)
	}

	mode := os.FileMode(0750)
	if app.Permissions != "" {
		if parsed, parseErr := strconv.ParseUint(app.Permissions, 8, 32); parseErr == nil {
			mode = os.FileMode(parsed)
		}
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(cleanDest), "download-*")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Chmod(mode); err != nil {
		_ = tmpFile.Close()
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	return os.Rename(tmpName, cleanDest)
}

func (a *Agent) createReport(releaseID, version string, success bool, errMsg string, startTime time.Time) *DeployReport {
	now := time.Now().UTC()
	return &DeployReport{
		ServiceID:    a.cfg.ServiceID,
		ReleaseID:    releaseID,
		Version:      version,
		Success:      success,
		ErrorMessage: errMsg,
		Host:         a.cfg.Hostname,
		ExecutedAt:   now,
		DurationMs:   now.Sub(startTime).Milliseconds(),
	}
}

func (a *Agent) sendReport(ctx context.Context, report *DeployReport) {
	reportURL := fmt.Sprintf("%s/api/v1/audit/logs", strings.TrimRight(a.cfg.ServerURL, "/"))
	data, err := json.Marshal(report)
	if err != nil {
		return
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reportURL, bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
}

// Run starts the agent polling ticker loop until context cancellation.
func (a *Agent) Run(ctx context.Context) error {
	slog.Info("starting autonomous agent polling loop",
		"service", a.cfg.ServiceID,
		"server", a.cfg.ServerURL,
		"interval", a.cfg.PollInterval,
	)

	// Run initial step immediately
	if _, err := a.StepOnce(ctx); err != nil && !errors.Is(err, ErrLocked) {
		slog.Warn("initial deployment step reported error", "error", err)
	}

	ticker := time.NewTicker(a.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("agent polling loop shutting down")
			return ctx.Err()
		case <-ticker.C:
			if _, err := a.StepOnce(ctx); err != nil && !errors.Is(err, ErrLocked) {
				slog.Warn("deployment step reported error", "error", err)
			}
		}
	}
}
