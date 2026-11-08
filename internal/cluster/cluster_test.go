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

package cluster_test

import (
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/cluster"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCluster_SyncStatusEvaluation(t *testing.T) {
	t.Parallel()

	// 1. Fully Synced Node
	node1 := cluster.NodeInfo{
		ID:               "node-1",
		CurrentReleaseID: "rel-1.0.0",
		DesiredReleaseID: "rel-1.0.0",
		LastHeartbeat:    time.Now().UTC(),
		RestartCount:     0,
	}
	cluster.EvaluateSyncStatus(&node1)
	assert.Equal(t, cluster.SyncStatusSynced, node1.SyncStatus)
	assert.Equal(t, "◆", node1.SyncStatus.Symbol())

	// 2. OutOfSync Node (Pending target update)
	node2 := cluster.NodeInfo{
		ID:               "node-2",
		CurrentReleaseID: "rel-0.9.0",
		DesiredReleaseID: "rel-1.0.0",
		LastHeartbeat:    time.Now().UTC(),
		RestartCount:     0,
	}
	cluster.EvaluateSyncStatus(&node2)
	assert.Equal(t, cluster.SyncStatusOutOfSync, node2.SyncStatus)
	assert.Equal(t, "▲", node2.SyncStatus.Symbol())

	// 3. Degraded Node (Frequent Restarts)
	node3 := cluster.NodeInfo{
		ID:               "node-3",
		CurrentReleaseID: "rel-1.0.0",
		DesiredReleaseID: "rel-1.0.0",
		LastHeartbeat:    time.Now().UTC(),
		RestartCount:     4,
	}
	cluster.EvaluateSyncStatus(&node3)
	assert.Equal(t, cluster.SyncStatusDegraded, node3.SyncStatus)
	assert.Equal(t, "✖", node3.SyncStatus.Symbol())

	// 4. Degraded Node (Heartbeat Silence)
	node4 := cluster.NodeInfo{
		ID:               "node-4",
		CurrentReleaseID: "rel-1.0.0",
		DesiredReleaseID: "rel-1.0.0",
		LastHeartbeat:    time.Now().UTC().Add(-2 * time.Minute),
		RestartCount:     0,
	}
	cluster.EvaluateSyncStatus(&node4)
	assert.Equal(t, cluster.SyncStatusDegraded, node4.SyncStatus)
}

func TestCluster_SummaryAggregation(t *testing.T) {
	t.Parallel()

	mgr := cluster.NewManager()

	mgr.UpsertNode(cluster.NodeInfo{
		ID:               "node-a",
		Hostname:         "edge-gateway-1",
		Role:             "gateway",
		CurrentReleaseID: "v1.0",
		DesiredReleaseID: "v1.0",
		LastHeartbeat:    time.Now().UTC(),
	})

	mgr.UpsertNode(cluster.NodeInfo{
		ID:               "node-b",
		Hostname:         "hospital-his-1",
		Role:             "worker",
		CurrentReleaseID: "v0.9",
		DesiredReleaseID: "v1.0",
		LastHeartbeat:    time.Now().UTC(),
	})

	mgr.UpsertNode(cluster.NodeInfo{
		ID:               "node-c",
		Hostname:         "ot-plc-bridge",
		Role:             "worker",
		CurrentReleaseID: "v1.0",
		DesiredReleaseID: "v1.0",
		RestartCount:     5,
		LastHeartbeat:    time.Now().UTC(),
	})

	summary := mgr.GetSummary()
	assert.Equal(t, 3, summary.TotalNodes)
	assert.Equal(t, 1, summary.SyncedNodes)
	assert.Equal(t, 1, summary.OutOfSyncNodes)
	assert.Equal(t, 1, summary.DegradedNodes)
	assert.Len(t, summary.Nodes, 3)

	// Verify Heartbeat update
	err := mgr.UpdateHeartbeat("node-a", 12)
	require.NoError(t, err)

	updatedNode, err := mgr.GetNode("node-a")
	require.NoError(t, err)
	assert.Equal(t, int64(12), updatedNode.HeartbeatLatencyMs)
}
