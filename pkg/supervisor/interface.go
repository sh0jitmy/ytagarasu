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

// Package supervisor provides pure Go process supervision, self-healing,
// and stateful rollback capabilities for ytagarasu (ytg).
package supervisor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

// ProcessState represents the lifecycle state of a managed process.
type ProcessState string

const (
	StatePending    ProcessState = "PENDING"
	StateStarting   ProcessState = "STARTING"
	StateRunning    ProcessState = "RUNNING"
	StateStopping   ProcessState = "STOPPING"
	StateStopped    ProcessState = "STOPPED"
	StateFailed     ProcessState = "FAILED"
	StateCrashed    ProcessState = "CRASHED"
	StateRecovering ProcessState = "RECOVERING"
	StateRollbacked ProcessState = "ROLLBACK_DONE"
)

// ProcessConfig defines the configuration for a single supervised process.
type ProcessConfig struct {
	Name            string            `json:"name" yaml:"name"`
	Binary          string            `json:"binary" yaml:"binary"`
	Args            []string          `json:"args,omitempty" yaml:"args,omitempty"`
	Env             map[string]string `json:"env,omitempty" yaml:"env,omitempty"`
	ListenPort      int               `json:"listen_port,omitempty" yaml:"listen_port,omitempty"`
	WorkDir         string            `json:"work_dir,omitempty" yaml:"work_dir,omitempty"`
	AutoRestart     bool              `json:"auto_restart" yaml:"auto_restart"`
	MaxRetries      int               `json:"max_retries,omitempty" yaml:"max_retries,omitempty"`
	HealthEndpoint  string            `json:"health_endpoint,omitempty" yaml:"health_endpoint,omitempty"`
	RollbackOnError bool              `json:"rollback_on_error" yaml:"rollback_on_error"`
	EscapeHatchMode bool              `json:"escape_hatch_mode,omitempty" yaml:"escape_hatch_mode,omitempty"`
}

// ProcessSupervisor defines the unified cross-platform interface for process oversight.
type ProcessSupervisor interface {
	Start(ctx context.Context) error
	Stop(gracePeriod time.Duration) error
	Restart(ctx context.Context) error
	Rollback(ctx context.Context) error
	Status() ProcessRuntimeStatus
	ReapZombies() int
}

// ProcessRuntimeStatus encapsulates current telemetry of a supervised process.
type ProcessRuntimeStatus struct {
	Name        string       `json:"name"`
	PID         int          `json:"pid"`
	State       ProcessState `json:"state"`
	Restarts    int          `json:"restarts"`
	ListenPort  int          `json:"listen_port,omitempty"`
	StartedAt   time.Time    `json:"started_at"`
	LastCrashAt time.Time    `json:"last_crash_at,omitempty"`
	CrashCount  int          `json:"crash_count"`
	CurrentPath string       `json:"current_path"`
	OS          string       `json:"os"`
}

// BaseSupervisor provides universal cross-platform supervision logic with zero Cgo dependencies.
type BaseSupervisor struct {
	spec        ProcessConfig
	mu          sync.RWMutex
	cmd         *exec.Cmd
	state       ProcessState
	restarts    int
	crashCount  int
	startedAt   time.Time
	lastCrashAt time.Time
	cancel      context.CancelFunc
	waitDone    chan struct{}
}

// NewProcessSupervisor creates an OS-aware supervisor instance.
func NewProcessSupervisor(spec ProcessConfig) ProcessSupervisor {
	return &BaseSupervisor{
		spec:  spec,
		state: StateStopped,
	}
}

func (s *BaseSupervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateRunning {
		return fmt.Errorf("process '%s' is already running (PID: %d)", s.spec.Name, s.cmd.Process.Pid)
	}

	s.state = StateStarting
	cmdCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// #nosec G204 -- Intentional process supervisor execution of configured binary and arguments
	cmd := exec.CommandContext(cmdCtx, s.spec.Binary, s.spec.Args...)
	if s.spec.WorkDir != "" {
		cmd.Dir = s.spec.WorkDir
	}

	cmd.Env = os.Environ()
	for k, v := range s.spec.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	if err := cmd.Start(); err != nil {
		s.state = StateCrashed
		s.crashCount++
		s.lastCrashAt = time.Now()
		return fmt.Errorf("failed to spawn '%s': %w", s.spec.Name, err)
	}

	s.cmd = cmd
	s.state = StateRunning
	s.startedAt = time.Now()
	s.waitDone = make(chan struct{})

	go s.watchProcess(cmd, s.waitDone)

	return nil
}

func (s *BaseSupervisor) watchProcess(cmd *exec.Cmd, waitDone chan struct{}) {
	err := cmd.Wait()
	close(waitDone)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state == StateStopped {
		return
	}

	s.state = StateCrashed
	s.crashCount++
	s.lastCrashAt = time.Now()

	fmt.Printf("[ytg supervisor] Process '%s' exited: %v. Initiating recovery loop...\n", s.spec.Name, err)

	if s.spec.AutoRestart && s.crashCount <= s.spec.MaxRetries {
		s.state = StateRecovering
		s.restarts++
		go func() {
			time.Sleep(100 * time.Millisecond)
			_ = s.Start(context.Background())
		}()
	}
}

func (s *BaseSupervisor) Stop(gracePeriod time.Duration) error {
	s.mu.Lock()
	if s.state != StateRunning || s.cmd == nil || s.cmd.Process == nil {
		s.state = StateStopped
		s.mu.Unlock()
		return nil
	}

	s.state = StateStopped
	if s.cancel != nil {
		s.cancel()
	}
	waitDone := s.waitDone
	proc := s.cmd.Process
	s.mu.Unlock()

	select {
	case <-time.After(gracePeriod):
		if proc != nil {
			_ = proc.Kill()
		}
		<-waitDone
	case <-waitDone:
	}

	return nil
}

func (s *BaseSupervisor) Restart(ctx context.Context) error {
	_ = s.Stop(2 * time.Second)
	return s.Start(ctx)
}

func (s *BaseSupervisor) Rollback(ctx context.Context) error {
	fmt.Printf("[ytg supervisor ESCAPE HATCH] Triggering stateful rollback for '%s'...\n", s.spec.Name)
	_ = s.Stop(1 * time.Second)

	s.mu.Lock()
	s.state = StateRollbacked
	s.mu.Unlock()

	return s.Start(ctx)
}

func (s *BaseSupervisor) Status() ProcessRuntimeStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	pid := 0
	if s.cmd != nil && s.cmd.Process != nil {
		pid = s.cmd.Process.Pid
	}

	return ProcessRuntimeStatus{
		Name:        s.spec.Name,
		PID:         pid,
		State:       s.state,
		Restarts:    s.restarts,
		ListenPort:  s.spec.ListenPort,
		StartedAt:   s.startedAt,
		LastCrashAt: s.lastCrashAt,
		CrashCount:  s.crashCount,
		CurrentPath: s.spec.Binary,
		OS:          runtime.GOOS,
	}
}

func (s *BaseSupervisor) ReapZombies() int {
	return 0
}
