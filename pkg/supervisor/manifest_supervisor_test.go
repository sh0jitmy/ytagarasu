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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestManifestSupervisor_FromYAML_Lifecycle(t *testing.T) {
	t.Parallel()

	binary := "sleep"
	args := `["10"]`
	if runtime.GOOS == "windows" {
		binary = "powershell"
		args = `["-Command", "Start-Sleep -Seconds 10"]`
	}

	manifestYAML := []byte(fmt.Sprintf(`
version: "1.0"
processes:
  - name: "worker-a"
    binary: "%s"
    args: %s
    auto_restart: true
    max_retries: 2
  - name: "worker-b"
    binary: "%s"
    args: %s
    auto_restart: false
`, binary, args, binary, args))

	ms, err := NewManifestSupervisorFromYAML(manifestYAML)
	if err != nil {
		t.Fatalf("unexpected error initializing manifest supervisor: %v", err)
	}

	ctx := context.Background()
	if err := ms.StartAll(ctx); err != nil {
		t.Fatalf("StartAll failed: %v", err)
	}
	defer func() {
		_ = ms.StopAll(1 * time.Second)
	}()

	time.Sleep(50 * time.Millisecond)

	status := ms.Status()
	if len(status) != 2 {
		t.Fatalf("expected 2 processes in status, got %d", len(status))
	}

	if status["worker-a"].State != StateRunning {
		t.Errorf("worker-a should be RUNNING, got %s", status["worker-a"].State)
	}

	// Test single process restart
	if err := ms.RestartProcess(ctx, "worker-a"); err != nil {
		t.Errorf("failed to restart worker-a: %v", err)
	}

	// Test non-existent process restart error
	if err := ms.RestartProcess(ctx, "non-existent"); err == nil {
		t.Errorf("expected error restarting non-existent process, got nil")
	}

	// Test manifest-wide restart
	if err := ms.RestartAll(ctx); err != nil {
		t.Errorf("failed to restart all processes: %v", err)
	}

	// Test manifest-wide rollback
	if err := ms.RollbackAll(ctx); err != nil {
		t.Errorf("failed to rollback all processes: %v", err)
	}

	// Stop all
	if err := ms.StopAll(1 * time.Second); err != nil {
		t.Errorf("StopAll returned error: %v", err)
	}
}

func TestManifestSupervisor_StatePersistence(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "supervisor-state.json")

	manifestYAML := []byte(`
version: "1.0"
services:
  - name: "mock-daemon"
    binary: "mock-daemon"
`)

	ms, err := NewManifestSupervisorFromYAML(manifestYAML)
	if err != nil {
		t.Fatalf("failed to init: %v", err)
	}

	if saveErr := ms.SaveState(statePath); saveErr != nil {
		t.Fatalf("SaveState failed: %v", saveErr)
	}

	if _, statErr := os.Stat(statePath); os.IsNotExist(statErr) {
		t.Fatalf("state file was not created: %s", statePath)
	}

	loaded, err := LoadState(statePath)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}

	if _, exists := loaded.Processes["mock-daemon"]; !exists {
		t.Errorf("mock-daemon not found in loaded state")
	}
}
