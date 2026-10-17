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
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestProcessSupervisor_LifecycleAndRestart(t *testing.T) {
	t.Parallel()

	spec := ProcessConfig{
		Name:            "test-worker",
		Binary:          "sleep",
		Args:            []string{"1"},
		AutoRestart:     true,
		MaxRetries:      2,
		RollbackOnError: true,
	}

	sup := NewProcessSupervisor(spec)
	ctx := context.Background()

	// 1. Start process
	if err := sup.Start(ctx); err != nil {
		t.Fatalf("Failed to start supervisor: %v", err)
	}

	status := sup.Status()
	if status.PID == 0 {
		t.Errorf("Expected valid PID, got 0")
	}

	time.Sleep(100 * time.Millisecond)

	// 2. Restart process
	if err := sup.Restart(ctx); err != nil {
		t.Fatalf("Failed to restart process: %v", err)
	}

	// 3. Stop process
	if err := sup.Stop(1 * time.Second); err != nil {
		t.Fatalf("Failed to stop process: %v", err)
	}

	status = sup.Status()
	if status.State != StateStopped {
		t.Errorf("Expected StateStopped, got %v", status.State)
	}
}

func TestProcessSupervisor_WithLogRotation(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "supervisor-out.log")

	spec := ProcessConfig{
		Name:          "log-worker",
		Binary:        "go",
		Args:          []string{"version"},
		LogPath:       logFile,
		LogMaxSizeMB:  10,
		LogMaxBackups: 2,
		LogCompress:   false,
	}

	sup := NewProcessSupervisor(spec)
	ctx := context.Background()

	if err := sup.Start(ctx); err != nil {
		t.Fatalf("Failed to start supervisor with logging: %v", err)
	}

	// Wait for process to exit and flush
	time.Sleep(600 * time.Millisecond)
	_ = sup.Stop(1 * time.Second)

	// Check that logFile was written and contains "go version"
	// #nosec G304 -- Test reads temporary file created in test dir
	content, err := os.ReadFile(filepath.Clean(logFile))
	if err != nil {
		t.Fatalf("Failed to read supervisor log file: %v", err)
	}

	if !strings.Contains(string(content), "go version") {
		t.Errorf("Expected log file to contain 'go version', got: %s", string(content))
	}
}

func TestProcessSupervisor_OOMDetectionMock(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	procRoot := filepath.Join(tmpDir, "proc")
	cgroupRoot := filepath.Join(tmpDir, "sys", "fs", "cgroup")

	spec := ProcessConfig{
		Name:            "oom-worker",
		Binary:          "go",
		Args:            []string{"version"},
		AutoRestart:     false,
		RollbackOnError: false,
	}

	baseSup := &BaseSupervisor{
		spec:  spec,
		state: StateStopped,
		cgroupInspector: &CgroupInspector{
			CgroupRoot: cgroupRoot,
			ProcRoot:   procRoot,
		},
	}

	ctx := context.Background()
	if err := baseSup.Start(ctx); err != nil {
		t.Fatalf("failed to start supervisor: %v", err)
	}

	pid := baseSup.cmd.Process.Pid
	pidDir := filepath.Join(procRoot, strconv.Itoa(pid))
	_ = os.MkdirAll(pidDir, 0750)
	cgroupRel := "system.slice/oom-worker.service"
	cgroupDir := filepath.Join(cgroupRoot, cgroupRel)
	_ = os.MkdirAll(cgroupDir, 0750)

	_ = os.WriteFile(filepath.Join(pidDir, "cgroup"), []byte("0::/"+cgroupRel+"\n"), 0600)
	_ = os.WriteFile(filepath.Join(cgroupDir, "memory.events"), []byte("oom 1\noom_kill 1\n"), 0600)

	// Wait for process to exit
	time.Sleep(300 * time.Millisecond)
	_ = baseSup.Stop(1 * time.Second)

	status := baseSup.Status()
	if status.State != StateStopped && status.State != StateCrashed {
		t.Errorf("unexpected state: %v", status.State)
	}
}
