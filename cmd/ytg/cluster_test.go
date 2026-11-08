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
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/cluster"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClusterCLI_SummaryAndInspect(t *testing.T) {
	t.Parallel()

	mgr := cluster.NewManager()
	testNode := cluster.NodeInfo{
		ID:                 "node-factory-his-01",
		Hostname:           "factory-his-1",
		IPAddress:          "10.0.1.50",
		Role:               "factory-his",
		CurrentReleaseID:   "rel-2026.11",
		DesiredReleaseID:   "rel-2026.11",
		ActiveContainers:   3,
		ActiveProcesses:    1,
		RestartCount:       0,
		UptimeSeconds:      3600,
		LastHeartbeat:      time.Now().UTC(),
		HeartbeatLatencyMs: 5,
	}

	mgr.UpsertNode(testNode)

	summary := mgr.GetSummary()
	assert.Equal(t, 1, summary.TotalNodes)
	assert.Equal(t, 1, summary.SyncedNodes)
	assert.Equal(t, "◆", summary.Nodes[0].SyncStatus.Symbol())

	retrieved, err := mgr.GetNode("node-factory-his-01")
	require.NoError(t, err)
	assert.Equal(t, "10.0.1.50", retrieved.IPAddress)
	assert.Equal(t, cluster.SyncStatusSynced, retrieved.SyncStatus)
}
