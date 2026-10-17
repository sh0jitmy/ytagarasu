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
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CgroupMemoryEvents records metrics parsed from cgroup v2 memory.events.
type CgroupMemoryEvents struct {
	Low          int64 `json:"low"`
	High         int64 `json:"high"`
	Max          int64 `json:"max"`
	OOM          int64 `json:"oom"`
	OOMKill      int64 `json:"oom_kill"`
	OOMGroupKill int64 `json:"oom_group_kill"`
}

// CgroupInspector inspects process cgroup assignment and memory events.
type CgroupInspector struct {
	CgroupRoot string
	ProcRoot   string
}

// NewCgroupInspector creates an inspector targeting standard Linux system paths.
func NewCgroupInspector() *CgroupInspector {
	return &CgroupInspector{
		CgroupRoot: "/sys/fs/cgroup",
		ProcRoot:   "/proc",
	}
}

// GetProcessCgroupPath parses /proc/<pid>/cgroup to identify the cgroup v2 path.
func (c *CgroupInspector) GetProcessCgroupPath(pid int) (string, error) {
	if pid <= 0 {
		return "", fmt.Errorf("invalid PID %d", pid)
	}

	procFile := filepath.Join(c.ProcRoot, strconv.Itoa(pid), "cgroup")
	cleanPath := filepath.Clean(procFile)

	// #nosec G304 -- Path constructed from validated proc root and numeric PID
	file, err := os.Open(cleanPath)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = file.Close()
	}()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// cgroup v2 format: 0::<cgroup-path>
		if strings.HasPrefix(line, "0::") {
			relPath := strings.TrimPrefix(line, "0::")
			relPath = strings.TrimPrefix(relPath, "/")
			return relPath, nil
		}
	}

	if scanErr := scanner.Err(); scanErr != nil {
		return "", scanErr
	}

	return "", fmt.Errorf("cgroup v2 entry not found for PID %d", pid)
}

// ReadMemoryEvents parses a memory.events file at the specified absolute or relative path.
func (c *CgroupInspector) ReadMemoryEvents(path string) (*CgroupMemoryEvents, error) {
	cleanPath := filepath.Clean(path)

	// #nosec G304 -- Sanitized memory.events path
	file, err := os.Open(cleanPath)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()

	events := &CgroupMemoryEvents{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		val, parseErr := strconv.ParseInt(fields[1], 10, 64)
		if parseErr != nil {
			continue
		}
		switch fields[0] {
		case "low":
			events.Low = val
		case "high":
			events.High = val
		case "max":
			events.Max = val
		case "oom":
			events.OOM = val
		case "oom_kill":
			events.OOMKill = val
		case "oom_group_kill":
			events.OOMGroupKill = val
		}
	}

	if scanErr := scanner.Err(); scanErr != nil {
		return nil, scanErr
	}

	return events, nil
}

// InspectPID inspects the memory events for a given process PID.
func (c *CgroupInspector) InspectPID(pid int) (*CgroupMemoryEvents, error) {
	relPath, err := c.GetProcessCgroupPath(pid)
	if err != nil {
		return nil, err
	}

	eventsPath := filepath.Join(c.CgroupRoot, relPath, "memory.events")
	return c.ReadMemoryEvents(eventsPath)
}

// IsOOMKilled determines whether the process was terminated by kernel OOM Killer.
func (c *CgroupInspector) IsOOMKilled(pid int, baseline *CgroupMemoryEvents) (bool, *CgroupMemoryEvents) {
	current, err := c.InspectPID(pid)
	if err != nil {
		return false, nil
	}

	baseKill := int64(0)
	if baseline != nil {
		baseKill = baseline.OOMKill
	}

	if current.OOMKill > baseKill {
		return true, current
	}

	return false, current
}
