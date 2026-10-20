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

package delta_test

import (
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/delta"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDelta_DriftDetection_FullLifecycle(t *testing.T) {
	t.Parallel()

	// 1. Desired Manifest specification
	desired := &manifest.Manifest{
		Release: "v2.0-reconcile",
		Containers: []manifest.ContainerSpec{
			{
				Name:  "web-api",
				Image: "api:v2.0",
				Ports: []string{"8080:8080"},
			},
			{
				Name:  "db-postgres",
				Image: "postgres:16",
				Ports: []string{"5433:5432"}, // Port changed from 5432 to 5433
			},
			{
				Name:  "cache-redis",
				Image: "redis:7",
				Ports: []string{"6379:6379"}, // Newly added container
			},
		},
		Applications: []manifest.Application{
			{
				Name:        "billing-agent",
				Type:        manifest.AppTypeGolang,
				Destination: "/opt/billing",
			},
		},
	}

	// 2. Actual Runtime Supervisor State
	actual := &supervisor.SupervisorState{
		ManifestPath: "/etc/ytagarasu/manifest.yaml",
		UpdatedAt:    time.Now().Add(-1 * time.Hour),
		Processes: map[string]supervisor.ProcessRuntimeStatus{
			"web-api": {
				Name:       "web-api",
				PID:        1001,
				State:      supervisor.StateRunning,
				ListenPort: 8080,
			},
			"db-postgres": {
				Name:       "db-postgres",
				PID:        1002,
				State:      supervisor.StateRunning,
				ListenPort: 5432, // Port in actual is 5432, desired is 5433 -> UPDATE
			},
			"billing-agent": {
				Name:  "billing-agent",
				PID:   1003,
				State: supervisor.StateRunning,
			},
			"legacy-worker": {
				Name:  "legacy-worker",
				PID:   999,
				State: supervisor.StateRunning, // Missing in desired -> DELETE
			},
		},
	}

	// 3. Compute drift
	plan := delta.DetectDrift(desired, actual)
	require.NotNil(t, plan)
	assert.Equal(t, "v2.0-reconcile", plan.ReleaseName)
	assert.True(t, plan.RequiresApproval, "Drift plan must require manual approval when changes exist")

	driftsMap := make(map[string]delta.ServiceDrift)
	for _, d := range plan.Drifts {
		driftsMap[d.Name] = d
	}

	// Verify CREATE for cache-redis
	redisDrift, hasRedis := driftsMap["cache-redis"]
	require.True(t, hasRedis)
	assert.Equal(t, delta.ActionCreate, redisDrift.Action)

	// Verify UPDATE for db-postgres
	dbDrift, hasDB := driftsMap["db-postgres"]
	require.True(t, hasDB)
	assert.Equal(t, delta.ActionUpdate, dbDrift.Action)
	assert.Contains(t, dbDrift.Differences[0], "port changed: 5432 -> 5433")

	// Verify RETAIN for web-api and billing-agent
	webDrift, hasWeb := driftsMap["web-api"]
	require.True(t, hasWeb)
	assert.Equal(t, delta.ActionRetain, webDrift.Action)

	billingDrift, hasBilling := driftsMap["billing-agent"]
	require.True(t, hasBilling)
	assert.Equal(t, delta.ActionRetain, billingDrift.Action)

	// Verify DELETE for legacy-worker
	legacyDrift, hasLegacy := driftsMap["legacy-worker"]
	require.True(t, hasLegacy)
	assert.Equal(t, delta.ActionDelete, legacyDrift.Action)

	// 4. Verify CUD output format
	report := plan.FormatDriftReport()
	assert.Contains(t, report, "=== RECONCILIATION PLAN: RELEASE V2.0-RECONCILE ===")
	assert.Contains(t, report, "+ [◆ CREATE] cache-redis")
	assert.Contains(t, report, "~ [▲ UPDATE] db-postgres")
	assert.Contains(t, report, "- [✖ DELETE] legacy-worker")
	assert.Contains(t, report, "= [■ RETAIN] web-api")
	assert.Contains(t, report, "DRIFT DETECTED. Manual approval required")
}

func TestDelta_DriftDetection_InSyncNoApproval(t *testing.T) {
	t.Parallel()

	desired := &manifest.Manifest{
		Release: "v1.0-synced",
		Containers: []manifest.ContainerSpec{
			{
				Name:  "nginx",
				Ports: []string{"80:80"},
			},
		},
	}

	actual := &supervisor.SupervisorState{
		Processes: map[string]supervisor.ProcessRuntimeStatus{
			"nginx": {
				Name:       "nginx",
				PID:        2001,
				State:      supervisor.StateRunning,
				ListenPort: 80,
			},
		},
	}

	plan := delta.DetectDrift(desired, actual)
	require.NotNil(t, plan)
	assert.False(t, plan.RequiresApproval, "In-sync plan should not require approval")

	report := plan.FormatDriftReport()
	assert.Contains(t, report, "= [■ RETAIN] nginx")
	assert.Contains(t, report, "Status: IN SYNC")
}
