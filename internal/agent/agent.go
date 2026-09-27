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

	"github.com/sh0jitmy/ytagarasu/internal/configmgr"
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

// SetPackageManager updates the agent's package manager implementation.
func (a *Agent) SetPackageManager(pkgMgr PackageManager) {
	a.pkgMgr = pkgMgr
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

	// 3. Apply OS & Application Packages if specified and manager is configured
	if a.pkgMgr != nil {
		orderedTargets := SortPackageTargets(m.Packages)
		for _, opt := range orderedTargets {
			pkgTarget := opt.Target
			if len(pkgTarget.Items) > 0 {
				slog.Info("applying packages non-interactively",
					"target", opt.Key,
					"manager", pkgTarget.Manager,
					"order", pkgTarget.Order,
					"count", len(pkgTarget.Items),
				)
				var pkgErr error
				if multiMgr, ok := a.pkgMgr.(*MultiPackageManager); ok && pkgTarget.Manager != "" {
					pkgErr = multiMgr.InstallForManager(ctx, pkgTarget.Manager, pkgTarget.Items)
				} else {
					pkgErr = a.pkgMgr.InstallPackages(ctx, pkgTarget.Items)
				}

				if pkgErr != nil {
					report := a.createReport(releaseID, m.Release, false, fmt.Sprintf("package install failed for '%s': %v", opt.Key, pkgErr), startTime)
					a.sendReport(ctx, report)
					state.Status = "failed"
					_ = a.stateStore.Save(state)
					return report, pkgErr
				}
			}
		}
	}

	// 4. Initialize Atomic Configuration & Artifact Transaction
	backupDir := filepath.Join(filepath.Dir(a.cfg.StateFile), "backups", releaseID)
	tx, err := configmgr.NewTransaction(backupDir)
	if err != nil {
		return nil, fmt.Errorf("failed initializing rollback transaction: %w", err)
	}

	// Stage application binaries
	for _, app := range m.Applications {
		if !app.Selector.Matches(a.cfg.Roles, a.cfg.Hostname) {
			continue
		}
		if app.Artifact == "" {
			continue
		}

		slog.Info("downloading application artifact", "app", app.Name, "artifact", app.Artifact)
		artifactURL := fmt.Sprintf("%s/repos/%s/%s", strings.TrimRight(a.cfg.ServerURL, "/"), a.cfg.ServiceID, app.Artifact)
		binData, downloadErr := a.fetchRemoteBytes(ctx, artifactURL)
		if downloadErr != nil {
			_ = tx.Rollback()
			report := a.createReport(releaseID, m.Release, false, fmt.Sprintf("artifact download failed: %v", downloadErr), startTime)
			a.sendReport(ctx, report)
			state.Status = "failed"
			_ = a.stateStore.Save(state)
			return report, downloadErr
		}

		destPath := app.Destination
		if destPath == "" {
			destPath = filepath.Join(a.cfg.InstallDir, "bin", app.Name)
		} else if !filepath.IsAbs(destPath) {
			destPath = filepath.Join(a.cfg.InstallDir, destPath)
		}

		mode := os.FileMode(0750)
		if app.Permissions != "" {
			if parsed, parseErr := strconv.ParseUint(app.Permissions, 8, 32); parseErr == nil {
				mode = os.FileMode(parsed)
			}
		}

		if stageErr := tx.StageFile(ctx, destPath, binData, mode, ""); stageErr != nil {
			_ = tx.Rollback()
			report := a.createReport(releaseID, m.Release, false, fmt.Sprintf("staging binary failed: %v", stageErr), startTime)
			a.sendReport(ctx, report)
			state.Status = "failed"
			_ = a.stateStore.Save(state)
			return report, stageErr
		}
	}

	// Stage configuration templates
	for _, cfg := range m.Configs {
		if !cfg.Selector.Matches(a.cfg.Roles, a.cfg.Hostname) {
			continue
		}
		if cfg.Template == "" {
			continue
		}

		tmplURL := fmt.Sprintf("%s/repos/%s/%s", strings.TrimRight(a.cfg.ServerURL, "/"), a.cfg.ServiceID, cfg.Template)
		tmplBytes, fetchErr := a.fetchRemoteBytes(ctx, tmplURL)
		if fetchErr != nil {
			_ = tx.Rollback()
			report := a.createReport(releaseID, m.Release, false, fmt.Sprintf("config template fetch failed: %v", fetchErr), startTime)
			a.sendReport(ctx, report)
			state.Status = "failed"
			_ = a.stateStore.Save(state)
			return report, fetchErr
		}

		renderer, rendErr := configmgr.NewRenderer(filepath.Base(cfg.Template), string(tmplBytes))
		if rendErr != nil {
			_ = tx.Rollback()
			report := a.createReport(releaseID, m.Release, false, fmt.Sprintf("template compile failed: %v", rendErr), startTime)
			a.sendReport(ctx, report)
			state.Status = "failed"
			_ = a.stateStore.Save(state)
			return report, rendErr
		}

		templateContext := map[string]any{
			"Hostname":  a.cfg.Hostname,
			"ServiceID": a.cfg.ServiceID,
			"Release":   m.Release,
			"Roles":     a.cfg.Roles,
		}

		rendered, execErr := renderer.Render(templateContext)
		if execErr != nil {
			_ = tx.Rollback()
			report := a.createReport(releaseID, m.Release, false, fmt.Sprintf("template render failed: %v", execErr), startTime)
			a.sendReport(ctx, report)
			state.Status = "failed"
			_ = a.stateStore.Save(state)
			return report, execErr
		}

		destPath := cfg.Destination
		if !filepath.IsAbs(destPath) {
			destPath = filepath.Join(a.cfg.InstallDir, destPath)
		}

		mode := os.FileMode(0640)
		if cfg.Permissions != "" {
			if parsed, parseErr := strconv.ParseUint(cfg.Permissions, 8, 32); parseErr == nil {
				mode = os.FileMode(parsed)
			}
		}

		// Stage with validation command
		if stageErr := tx.StageFile(ctx, destPath, rendered, mode, cfg.ValidateCommand); stageErr != nil {
			_ = tx.Rollback()
			report := a.createReport(releaseID, m.Release, false, fmt.Sprintf("config validation rejected: %v", stageErr), startTime)
			a.sendReport(ctx, report)
			state.Status = "failed"
			_ = a.stateStore.Save(state)
			return report, stageErr
		}
	}

	// 5. Commit Transaction via Atomic Rename (renameat)
	if commitErr := tx.Commit(); commitErr != nil {
		_ = tx.Rollback()
		report := a.createReport(releaseID, m.Release, false, fmt.Sprintf("atomic commit failed: %v", commitErr), startTime)
		a.sendReport(ctx, report)
		state.Status = "failed"
		_ = a.stateStore.Save(state)
		return report, commitErr
	}

	// 6. Execute Health Checks & Automatic Rollback on Failure
	if len(m.HealthChecks) > 0 {
		checker := configmgr.NewHealthChecker(a.client)
		for _, hc := range m.HealthChecks {
			if !hc.Selector.Matches(a.cfg.Roles, a.cfg.Hostname) {
				continue
			}

			slog.Info("executing health check probe", "type", hc.Type, "target", hc.Endpoint+hc.Command)
			if checkErr := checker.Check(ctx, hc); checkErr != nil {
				slog.Error("health check probe failed, triggering automatic rollback", "error", checkErr)
				if rbErr := tx.Rollback(); rbErr != nil {
					slog.Error("error during rollback execution", "rollback_error", rbErr)
				}
				report := a.createReport(releaseID, m.Release, false, fmt.Sprintf("health check failed (rolled back): %v", checkErr), startTime)
				a.sendReport(ctx, report)
				state.Status = "failed"
				_ = a.stateStore.Save(state)
				return report, checkErr
			}
		}
	}

	// 7. Update State on Success
	state.ServiceID = a.cfg.ServiceID
	state.CurrentReleaseID = releaseID
	state.CurrentVersion = m.Release
	state.LastDeployedAt = time.Now().UTC()
	state.LastCheckedAt = time.Now().UTC()
	state.Status = "synced"
	if err := a.stateStore.Save(state); err != nil {
		slog.Error("failed saving agent state", "error", err)
	}

	// 8. Submit Success Report
	report := a.createReport(releaseID, m.Release, true, "", startTime)
	a.sendReport(ctx, report)

	slog.Info("autonomous deployment succeeded and committed",
		"service", a.cfg.ServiceID,
		"release", releaseID,
		"duration_ms", report.DurationMs,
	)

	return report, nil
}

func (a *Agent) fetchRemoteBytes(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("remote server returned HTTP %s for %s", resp.Status, url)
	}

	return io.ReadAll(resp.Body)
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
