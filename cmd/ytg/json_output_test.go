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

package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
)

func TestHasJSONFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantJSON   bool
		wantRemain []string
	}{
		{
			name:       "no json flag",
			args:       []string{"status", "backend"},
			wantJSON:   false,
			wantRemain: []string{"status", "backend"},
		},
		{
			name:       "json flag at end",
			args:       []string{"status", "--json"},
			wantJSON:   true,
			wantRemain: []string{"status"},
		},
		{
			name:       "json flag at start",
			args:       []string{"--json", "backend"},
			wantJSON:   true,
			wantRemain: []string{"backend"},
		},
		{
			name:       "json flag only",
			args:       []string{"--json"},
			wantJSON:   true,
			wantRemain: nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotJSON, gotRemain := hasJSONFlag(tc.args)
			if gotJSON != tc.wantJSON {
				t.Errorf("hasJSONFlag(%v) gotJSON = %v, want %v", tc.args, gotJSON, tc.wantJSON)
			}
			if len(gotRemain) != len(tc.wantRemain) {
				t.Fatalf("hasJSONFlag(%v) gotRemain len = %d, want %d", tc.args, len(gotRemain), len(tc.wantRemain))
			}
			for i := range gotRemain {
				if gotRemain[i] != tc.wantRemain[i] {
					t.Errorf("hasJSONFlag(%v) gotRemain[%d] = %q, want %q", tc.args, i, gotRemain[i], tc.wantRemain[i])
				}
			}
		})
	}
}

func TestStatusReport_JSONEncoding(t *testing.T) {
	t.Parallel()

	report := StatusReport{
		ManifestPath: "manifest.yaml",
		UpdatedAt:    time.Now().Format(time.RFC3339),
		TotalCount:   2,
		Services: []ServiceStatusInfo{
			{
				Name:        "api-server",
				PID:         1234,
				State:       string(supervisor.StateRunning),
				Restarts:    0,
				StartedAt:   "2026-10-18T10:00:00Z",
				CurrentPath: "/bin/api-server",
				Uptime:      "1h0m0s",
			},
			{
				Name:        "worker",
				PID:         1235,
				State:       string(supervisor.StateRunning),
				Restarts:    1,
				StartedAt:   "2026-10-18T10:05:00Z",
				CurrentPath: "/bin/worker",
				Uptime:      "55m0s",
			},
		},
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal(StatusReport) failed: %v", err)
	}

	var parsed StatusReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal(StatusReport) failed: %v", err)
	}

	if parsed.TotalCount != 2 || len(parsed.Services) != 2 {
		t.Errorf("unexpected parsed report: %+v", parsed)
	}
	if parsed.Services[0].Name != "api-server" || parsed.Services[0].PID != 1234 {
		t.Errorf("unexpected parsed service[0]: %+v", parsed.Services[0])
	}
}

func TestDoctorReport_JSONEncoding(t *testing.T) {
	t.Parallel()

	report := DoctorReport{
		OS:        "linux",
		Arch:      "amd64",
		GoVersion: "go1.26.9",
		Checks: []DoctorCheckItem{
			{Name: "Platform", Status: "INFO", Message: "linux/amd64"},
			{Name: "Manifest File", Status: "PASS", Message: "manifest.yaml found"},
		},
		Passed: true,
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal(DoctorReport) failed: %v", err)
	}

	var parsed DoctorReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal(DoctorReport) failed: %v", err)
	}

	if !parsed.Passed || len(parsed.Checks) != 2 {
		t.Errorf("unexpected parsed DoctorReport: %+v", parsed)
	}
}
