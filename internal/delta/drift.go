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

package delta

import (
	"fmt"
	"sort"
	"strings"

	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
)

// DriftAction categorizes the required reconciliation action for a workload.
type DriftAction string

const (
	ActionCreate DriftAction = "CREATE"
	ActionUpdate DriftAction = "UPDATE"
	ActionDelete DriftAction = "DELETE"
	ActionRetain DriftAction = "RETAIN"
)

// ServiceDrift records the diff between desired and actual running state.
type ServiceDrift struct {
	Name        string      `json:"name"`
	Kind        string      `json:"kind"`
	Action      DriftAction `json:"action"`
	Differences []string    `json:"differences,omitempty"`
}

// ReconcilePlan encapsulates all detected drifts and approval requirements.
type ReconcilePlan struct {
	ReleaseName      string         `json:"release_name"`
	Drifts           []ServiceDrift `json:"drifts"`
	RequiresApproval bool           `json:"requires_approval"`
}

// DetectDrift computes state drift between desired manifest and actual runtime supervisor state.
func DetectDrift(desired *manifest.Manifest, actual *supervisor.SupervisorState) *ReconcilePlan {
	plan := &ReconcilePlan{
		ReleaseName:      "unknown",
		Drifts:           make([]ServiceDrift, 0),
		RequiresApproval: false,
	}

	if desired != nil {
		plan.ReleaseName = desired.Release
	}

	actualProcs := make(map[string]supervisor.ProcessRuntimeStatus)
	if actual != nil && actual.Processes != nil {
		actualProcs = actual.Processes
	}

	desiredItems := make(map[string]struct {
		kind       string
		listenPort int
		binary     string
		image      string
	})

	if desired != nil {
		// Collect containers
		for _, ctr := range desired.Containers {
			primaryPort := 0
			if len(ctr.Ports) > 0 {
				parts := strings.Split(ctr.Ports[0], ":")
				if len(parts) >= 1 {
					_, _ = fmt.Sscanf(parts[0], "%d", &primaryPort)
				}
			}
			desiredItems[ctr.Name] = struct {
				kind       string
				listenPort int
				binary     string
				image      string
			}{
				kind:       "container",
				listenPort: primaryPort,
				image:      ctr.Image,
			}
		}

		// Collect applications
		for _, app := range desired.Applications {
			bin := app.Destination
			if bin == "" {
				bin = app.Artifact
			}
			if bin == "" {
				bin = app.Name
			}
			desiredItems[app.Name] = struct {
				kind       string
				listenPort int
				binary     string
				image      string
			}{
				kind:   "application",
				binary: bin,
			}
		}
	}

	// 1. Identify Created & Updated items from desired
	for name, des := range desiredItems {
		act, exists := actualProcs[name]
		if !exists {
			plan.Drifts = append(plan.Drifts, ServiceDrift{
				Name:        name,
				Kind:        des.kind,
				Action:      ActionCreate,
				Differences: []string{"workload not currently running (new deployment)"},
			})
			plan.RequiresApproval = true
			continue
		}

		// Item exists in actual state: check for property differences
		diffs := make([]string, 0)
		if des.listenPort > 0 && act.ListenPort > 0 && des.listenPort != act.ListenPort {
			diffs = append(diffs, fmt.Sprintf("port changed: %d -> %d", act.ListenPort, des.listenPort))
		}

		if len(diffs) > 0 {
			plan.Drifts = append(plan.Drifts, ServiceDrift{
				Name:        name,
				Kind:        des.kind,
				Action:      ActionUpdate,
				Differences: diffs,
			})
			plan.RequiresApproval = true
		} else {
			plan.Drifts = append(plan.Drifts, ServiceDrift{
				Name:        name,
				Kind:        des.kind,
				Action:      ActionRetain,
				Differences: []string{"in sync with actual state"},
			})
		}
	}

	// 2. Identify Deleted items from actual that are missing in desired
	for name, act := range actualProcs {
		if _, exists := desiredItems[name]; !exists {
			plan.Drifts = append(plan.Drifts, ServiceDrift{
				Name:        name,
				Kind:        "workload",
				Action:      ActionDelete,
				Differences: []string{fmt.Sprintf("running PID %d absent in desired manifest (requires decommission)", act.PID)},
			})
			plan.RequiresApproval = true
		}
	}

	// Sort drifts deterministically by Name
	sort.Slice(plan.Drifts, func(i, j int) bool {
		return plan.Drifts[i].Name < plan.Drifts[j].Name
	})

	return plan
}

// FormatDriftReport formats ReconcilePlan using Tactical Cyberpunk × Enterprise CUD symbols.
func (p *ReconcilePlan) FormatDriftReport() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "=== RECONCILIATION PLAN: RELEASE %s ===\n", strings.ToUpper(p.ReleaseName))

	for _, d := range p.Drifts {
		switch d.Action {
		case ActionCreate:
			fmt.Fprintf(&sb, "  + [◆ CREATE] %s (%s): %s\n", d.Name, d.Kind, strings.Join(d.Differences, ", "))
		case ActionUpdate:
			fmt.Fprintf(&sb, "  ~ [▲ UPDATE] %s (%s): %s\n", d.Name, d.Kind, strings.Join(d.Differences, ", "))
		case ActionDelete:
			fmt.Fprintf(&sb, "  - [✖ DELETE] %s (%s): %s\n", d.Name, d.Kind, strings.Join(d.Differences, ", "))
		case ActionRetain:
			fmt.Fprintf(&sb, "  = [■ RETAIN] %s (%s): %s\n", d.Name, d.Kind, strings.Join(d.Differences, ", "))
		}
	}

	if p.RequiresApproval {
		sb.WriteString(">>> Status: DRIFT DETECTED. Manual approval required before applying changes.\n")
	} else {
		sb.WriteString(">>> Status: IN SYNC. Desired state matches actual running state.\n")
	}

	return sb.String()
}
