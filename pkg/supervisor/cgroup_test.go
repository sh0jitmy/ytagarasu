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
	"os"
	"path/filepath"
	"testing"
)

func TestCgroupInspector_ParseAndDetectOOM(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	procRoot := filepath.Join(tmpDir, "proc")
	cgroupRoot := filepath.Join(tmpDir, "sys", "fs", "cgroup")

	pid := 12345
	pidDir := filepath.Join(procRoot, "12345")
	if err := os.MkdirAll(pidDir, 0750); err != nil {
		t.Fatalf("failed to create mock proc dir: %v", err)
	}

	cgroupRel := "system.slice/worker.service"
	cgroupDir := filepath.Join(cgroupRoot, cgroupRel)
	if err := os.MkdirAll(cgroupDir, 0750); err != nil {
		t.Fatalf("failed to create mock cgroup dir: %v", err)
	}

	// 1. Create mock /proc/<pid>/cgroup
	procCgroupContent := "0::/system.slice/worker.service\n"
	if err := os.WriteFile(filepath.Join(pidDir, "cgroup"), []byte(procCgroupContent), 0600); err != nil {
		t.Fatalf("failed to write mock /proc/cgroup: %v", err)
	}

	// 2. Create mock memory.events (baseline)
	baselineContent := `low 0
high 12
max 5
oom 1
oom_kill 1
oom_group_kill 0
`
	eventsFile := filepath.Join(cgroupDir, "memory.events")
	if err := os.WriteFile(eventsFile, []byte(baselineContent), 0600); err != nil {
		t.Fatalf("failed to write mock memory.events: %v", err)
	}

	inspector := &CgroupInspector{
		CgroupRoot: cgroupRoot,
		ProcRoot:   procRoot,
	}

	// 3. Verify path extraction
	relPath, err := inspector.GetProcessCgroupPath(pid)
	if err != nil {
		t.Fatalf("unexpected error finding cgroup path: %v", err)
	}
	if relPath != cgroupRel {
		t.Errorf("expected relPath %q, got %q", cgroupRel, relPath)
	}

	// 4. Inspect baseline events
	baseline, err := inspector.InspectPID(pid)
	if err != nil {
		t.Fatalf("unexpected error inspecting PID: %v", err)
	}
	if baseline.OOMKill != 1 || baseline.High != 12 {
		t.Errorf("unexpected baseline values: %+v", baseline)
	}

	// Baseline comparison should be false (no new OOM)
	killed, _ := inspector.IsOOMKilled(pid, baseline)
	if killed {
		t.Errorf("expected IsOOMKilled false when counter equals baseline")
	}

	// 5. Simulate Kernel OOM Killer event incrementing oom_kill
	oomContent := `low 0
high 15
max 7
oom 2
oom_kill 2
oom_group_kill 0
`
	if err := os.WriteFile(eventsFile, []byte(oomContent), 0600); err != nil {
		t.Fatalf("failed to update mock memory.events: %v", err)
	}

	killedAfter, current := inspector.IsOOMKilled(pid, baseline)
	if !killedAfter {
		t.Errorf("expected IsOOMKilled true after OOM counter increment")
	}
	if current == nil || current.OOMKill != 2 {
		t.Errorf("expected current OOMKill = 2, got: %+v", current)
	}
}

func TestCgroupInspector_MissingOrNonLinuxFallback(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	inspector := &CgroupInspector{
		CgroupRoot: filepath.Join(tmpDir, "cgroup"),
		ProcRoot:   filepath.Join(tmpDir, "proc"),
	}

	// Non-existent PID should gracefully return false
	killed, current := inspector.IsOOMKilled(99999, nil)
	if killed {
		t.Errorf("expected IsOOMKilled false for non-existent PID")
	}
	if current != nil {
		t.Errorf("expected nil current events for non-existent PID")
	}
}
