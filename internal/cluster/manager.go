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

package cluster

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// DefaultHeartbeatTimeout after which a node is marked as Degraded if silent.
const DefaultHeartbeatTimeout = 30 * time.Second

// Manager maintains distributed agent nodes state in-memory with thread-safety.
type Manager struct {
	mu    sync.RWMutex
	nodes map[string]NodeInfo
}

// NewManager initializes a cluster manager.
func NewManager() *Manager {
	return &Manager{
		nodes: make(map[string]NodeInfo),
	}
}

// UpsertNode registers or updates a cluster node's state.
func (m *Manager) UpsertNode(node NodeInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if node.LastHeartbeat.IsZero() {
		node.LastHeartbeat = time.Now().UTC()
	}

	EvaluateSyncStatus(&node)
	m.nodes[node.ID] = node
}

// UpdateHeartbeat records a pulse from the agent with network round-trip latency.
func (m *Manager) UpdateHeartbeat(nodeID string, latencyMs int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	node, exists := m.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node with id '%s' not registered", nodeID)
	}

	node.LastHeartbeat = time.Now().UTC()
	node.HeartbeatLatencyMs = latencyMs
	EvaluateSyncStatus(&node)
	m.nodes[nodeID] = node
	return nil
}

// GetNode retrieves a specific node by its identifier.
func (m *Manager) GetNode(nodeID string) (*NodeInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	node, exists := m.nodes[nodeID]
	if !exists {
		return nil, fmt.Errorf("node '%s' not found", nodeID)
	}
	EvaluateSyncStatus(&node)
	return &node, nil
}

// GetSummary aggregates states across all known nodes.
func (m *Manager) GetSummary() ClusterSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	summary := ClusterSummary{
		TotalNodes:  len(m.nodes),
		Nodes:       make([]NodeInfo, 0, len(m.nodes)),
		GeneratedAt: time.Now().UTC(),
	}

	for _, n := range m.nodes {
		nodeCopy := n
		EvaluateSyncStatus(&nodeCopy)

		switch nodeCopy.SyncStatus {
		case SyncStatusSynced:
			summary.SyncedNodes++
		case SyncStatusOutOfSync:
			summary.OutOfSyncNodes++
		case SyncStatusDegraded:
			summary.DegradedNodes++
		case SyncStatusDrifted:
			summary.DriftedNodes++
		}

		summary.Nodes = append(summary.Nodes, nodeCopy)
	}

	sort.Slice(summary.Nodes, func(i, j int) bool {
		return summary.Nodes[i].ID < summary.Nodes[j].ID
	})

	return summary
}

// EvaluateSyncStatus recalculates the SyncStatus of a node based on target release, restarts, and heartbeat freshness.
func EvaluateSyncStatus(n *NodeInfo) {
	// 1. Degraded check: Heartbeat silence or crash-loop restarts
	if !n.LastHeartbeat.IsZero() && time.Since(n.LastHeartbeat) > DefaultHeartbeatTimeout {
		n.SyncStatus = SyncStatusDegraded
		return
	}
	if n.RestartCount >= 3 {
		n.SyncStatus = SyncStatusDegraded
		return
	}

	// 2. OutOfSync check: Current release does not match desired release
	if n.DesiredReleaseID != "" && n.CurrentReleaseID != n.DesiredReleaseID {
		n.SyncStatus = SyncStatusOutOfSync
		return
	}

	// 3. Otherwise healthy & synchronized
	if n.SyncStatus == "" || n.SyncStatus == SyncStatusOutOfSync || n.SyncStatus == SyncStatusDegraded {
		n.SyncStatus = SyncStatusSynced
	}
}
