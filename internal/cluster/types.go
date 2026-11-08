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
	"time"
)

// SyncStatus represents the synchronization state between desired release and active host runtime.
type SyncStatus string

const (
	SyncStatusSynced    SyncStatus = "Synced"
	SyncStatusOutOfSync SyncStatus = "OutOfSync"
	SyncStatusDegraded  SyncStatus = "Degraded"
	SyncStatusDrifted   SyncStatus = "Drifted"
)

// CUD Triple-Encoded representation (Symbol + Text + Hex Color) conforming to Tactical Cyberpunk UI standard.
func (s SyncStatus) Symbol() string {
	switch s {
	case SyncStatusSynced:
		return "◆"
	case SyncStatusOutOfSync:
		return "▲"
	case SyncStatusDegraded:
		return "✖"
	case SyncStatusDrifted:
		return "■"
	default:
		return "●"
	}
}

func (s SyncStatus) BadgeClass() string {
	switch s {
	case SyncStatusSynced:
		return "badge-synced"
	case SyncStatusOutOfSync:
		return "badge-outofsync"
	case SyncStatusDegraded:
		return "badge-degraded"
	case SyncStatusDrifted:
		return "badge-drifted"
	default:
		return "badge-unknown"
	}
}

// NodeInfo represents runtime telemetry, desired release drift, and health of a deployed agent node.
type NodeInfo struct {
	ID                 string     `json:"id"`
	Hostname           string     `json:"hostname"`
	IPAddress          string     `json:"ip_address"`
	Role               string     `json:"role"`
	CurrentReleaseID   string     `json:"current_release_id"`
	DesiredReleaseID   string     `json:"desired_release_id"`
	SyncStatus         SyncStatus `json:"sync_status"`
	ActiveContainers   int        `json:"active_containers"`
	ActiveProcesses    int        `json:"active_processes"`
	RestartCount       int        `json:"restart_count"`
	UptimeSeconds      int64      `json:"uptime_seconds"`
	LastHeartbeat      time.Time  `json:"last_heartbeat"`
	HeartbeatLatencyMs int64      `json:"heartbeat_latency_ms"`
}

// ClusterSummary represents an aggregated cluster overview of all active distributed nodes.
type ClusterSummary struct {
	TotalNodes     int        `json:"total_nodes"`
	SyncedNodes    int        `json:"synced_nodes"`
	OutOfSyncNodes int        `json:"out_of_sync_nodes"`
	DegradedNodes  int        `json:"degraded_nodes"`
	DriftedNodes   int        `json:"drifted_nodes"`
	Nodes          []NodeInfo `json:"nodes"`
	GeneratedAt    time.Time  `json:"generated_at"`
}
