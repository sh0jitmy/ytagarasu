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

package supervisor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// SupervisorState records running process states for CLI telemetry and management.
type SupervisorState struct {
	ManifestPath string                          `json:"manifest_path"`
	UpdatedAt    time.Time                       `json:"updated_at"`
	Processes    map[string]ProcessRuntimeStatus `json:"processes"`
}

// ManifestSpec represents deployment manifest structure for process supervision.
type ManifestSpec struct {
	Version   string          `yaml:"version"`
	Processes []ProcessConfig `yaml:"processes,omitempty"`
	Services  []struct {
		Name        string `yaml:"name"`
		Binary      string `yaml:"binary,omitempty"`
		Action      string `yaml:"action,omitempty"`
		AutoRestart *bool  `yaml:"auto_restart,omitempty"`
	} `yaml:"services,omitempty"`
	Applications []struct {
		Name        string `yaml:"name"`
		Artifact    string `yaml:"artifact,omitempty"`
		Destination string `yaml:"destination,omitempty"`
	} `yaml:"applications,omitempty"`
}

// ManifestSupervisor coordinates multiple supervised processes defined in a manifest.
type ManifestSupervisor struct {
	manifestPath string
	mu           sync.RWMutex
	supervisors  map[string]ProcessSupervisor
	configs      map[string]ProcessConfig
}

// NewManifestSupervisor loads and initializes a manifest-driven supervisor.
func NewManifestSupervisor(manifestPath string) (*ManifestSupervisor, error) {
	cleanPath := filepath.Clean(manifestPath)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file %s: %w", manifestPath, err)
	}

	ms, err := NewManifestSupervisorFromYAML(data)
	if err != nil {
		return nil, err
	}
	ms.manifestPath = manifestPath
	return ms, nil
}

// NewManifestSupervisorFromYAML constructs ManifestSupervisor from raw YAML bytes.
func NewManifestSupervisorFromYAML(data []byte) (*ManifestSupervisor, error) {
	var spec ManifestSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse manifest YAML: %w", err)
	}

	configs := make(map[string]ProcessConfig)

	// 1. Explicit processes section
	for _, proc := range spec.Processes {
		configs[proc.Name] = proc
	}

	// 2. Services section mapping
	for _, svc := range spec.Services {
		if _, exists := configs[svc.Name]; !exists {
			bin := svc.Binary
			if bin == "" {
				bin = svc.Name
			}
			autoRestart := true
			if svc.AutoRestart != nil {
				autoRestart = *svc.AutoRestart
			}
			configs[svc.Name] = ProcessConfig{
				Name:        svc.Name,
				Binary:      bin,
				AutoRestart: autoRestart,
				MaxRetries:  3,
			}
		}
	}

	// 3. Applications section mapping fallback
	for _, app := range spec.Applications {
		if _, exists := configs[app.Name]; !exists {
			bin := app.Destination
			if bin == "" {
				bin = app.Artifact
			}
			if bin == "" {
				bin = app.Name
			}
			configs[app.Name] = ProcessConfig{
				Name:        app.Name,
				Binary:      bin,
				AutoRestart: true,
				MaxRetries:  3,
			}
		}
	}

	if len(configs) == 0 {
		return nil, errors.New("manifest does not contain any runnable processes, services, or applications")
	}

	supervisors := make(map[string]ProcessSupervisor)
	for name, cfg := range configs {
		supervisors[name] = NewProcessSupervisor(cfg)
	}

	return &ManifestSupervisor{
		supervisors: supervisors,
		configs:     configs,
	}, nil
}

// StartAll starts all managed processes defined in the manifest.
func (m *ManifestSupervisor) StartAll(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, s := range m.supervisors {
		if err := s.Start(ctx); err != nil {
			return fmt.Errorf("failed to start process %s: %w", name, err)
		}
	}
	return nil
}

// StopAll stops all managed processes with grace period.
func (m *ManifestSupervisor) StopAll(gracePeriod time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for name, s := range m.supervisors {
		if err := s.Stop(gracePeriod); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("failed to stop process %s: %w", name, err)
		}
	}
	return firstErr
}

// RestartProcess restarts a single specific process by name.
func (m *ManifestSupervisor) RestartProcess(ctx context.Context, name string) error {
	m.mu.RLock()
	s, exists := m.supervisors[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("process %s not found in manifest", name)
	}
	return s.Restart(ctx)
}

// RestartAll restarts all processes managed by the manifest.
func (m *ManifestSupervisor) RestartAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, s := range m.supervisors {
		if err := s.Restart(ctx); err != nil {
			return fmt.Errorf("failed to restart process %s: %w", name, err)
		}
	}
	return nil
}

// RollbackProcess executes stateful rollback for a single process.
func (m *ManifestSupervisor) RollbackProcess(ctx context.Context, name string) error {
	m.mu.RLock()
	s, exists := m.supervisors[name]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("process %s not found in manifest", name)
	}
	return s.Rollback(ctx)
}

// RollbackAll rolls back all processes managed by the manifest.
func (m *ManifestSupervisor) RollbackAll(ctx context.Context) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for name, s := range m.supervisors {
		if err := s.Rollback(ctx); err != nil {
			return fmt.Errorf("failed to rollback process %s: %w", name, err)
		}
	}
	return nil
}

// Status returns runtime status for all processes in the manifest.
func (m *ManifestSupervisor) Status() map[string]ProcessRuntimeStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make(map[string]ProcessRuntimeStatus, len(m.supervisors))
	for name, s := range m.supervisors {
		res[name] = s.Status()
	}
	return res
}

// SaveState writes current status to a state file atomically.
func (m *ManifestSupervisor) SaveState(statePath string) error {
	cleanPath := filepath.Clean(statePath)
	tmpPath := cleanPath + ".tmp"

	state := SupervisorState{
		ManifestPath: m.manifestPath,
		UpdatedAt:    time.Now(),
		Processes:    m.Status(),
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal supervisor state: %w", err)
	}

	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write temp state: %w", err)
	}

	if err := os.Rename(tmpPath, cleanPath); err != nil {
		return fmt.Errorf("failed to atomically commit state: %w", err)
	}

	return nil
}

// LoadState reads supervisor state from a state file with transient retries.
func LoadState(statePath string) (*SupervisorState, error) {
	cleanPath := filepath.Clean(statePath)

	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		data, err := os.ReadFile(cleanPath)
		if err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}

		if len(data) == 0 {
			lastErr = errors.New("state file is currently empty")
			time.Sleep(50 * time.Millisecond)
			continue
		}

		var state SupervisorState
		if err := json.Unmarshal(data, &state); err != nil {
			lastErr = err
			time.Sleep(50 * time.Millisecond)
			continue
		}

		return &state, nil
	}

	return nil, fmt.Errorf("failed to read state file after retries: %w", lastErr)
}
