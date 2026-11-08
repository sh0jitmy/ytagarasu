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
	"flag"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/cluster"
)

// DefaultGlobalClusterManager for standalone simulation / CLI reporting.
var DefaultGlobalClusterManager = cluster.NewManager()

func init() {
	// Pre-populate mock nodes if in demo/local environment so cluster commands are immediately useful
	DefaultGlobalClusterManager.UpsertNode(cluster.NodeInfo{
		ID:                 "node-tokyo-edge-01",
		Hostname:           "edge-gateway-1",
		IPAddress:          "192.168.10.11",
		Role:               "gateway",
		CurrentReleaseID:   "rel-2026.11-v1",
		DesiredReleaseID:   "rel-2026.11-v1",
		SyncStatus:         cluster.SyncStatusSynced,
		ActiveContainers:   4,
		ActiveProcesses:    2,
		RestartCount:       0,
		UptimeSeconds:      86400 * 5,
		LastHeartbeat:      time.Now().UTC(),
		HeartbeatLatencyMs: 3,
	})
	DefaultGlobalClusterManager.UpsertNode(cluster.NodeInfo{
		ID:                 "node-osaka-his-02",
		Hostname:           "his-workload-2",
		IPAddress:          "192.168.20.15",
		Role:               "hospital-his",
		CurrentReleaseID:   "rel-2026.10-v4",
		DesiredReleaseID:   "rel-2026.11-v1",
		SyncStatus:         cluster.SyncStatusOutOfSync,
		ActiveContainers:   2,
		ActiveProcesses:    1,
		RestartCount:       1,
		UptimeSeconds:      86400 * 2,
		LastHeartbeat:      time.Now().UTC(),
		HeartbeatLatencyMs: 14,
	})
}

func handleClusterCommand(args []string) {
	if len(args) == 0 {
		printClusterHelp()
		os.Exit(1)
	}

	subcommand := args[0]
	switch subcommand {
	case "status":
		handleClusterStatus(args[1:])
	case "inspect":
		handleClusterInspect(args[1:])
	case "help", "--help", "-h":
		printClusterHelp()
	default:
		fmt.Fprintf(os.Stderr, "Unknown cluster subcommand: %s\n", subcommand)
		printClusterHelp()
		os.Exit(1)
	}
}

func printClusterHelp() {
	fmt.Println("Usage: ytg cluster <subcommand> [options] [arguments]")
	fmt.Println("\nMulti-Node Deployment State Aggregation & Topology Observability.")
	fmt.Println("\nAvailable Subcommands:")
	fmt.Println("  status     Display topology status across all distributed nodes")
	fmt.Println("  inspect    Display telemetry and drift diagnostics for a specific node")
	fmt.Println("\nExamples:")
	fmt.Println("  ytg cluster status")
	fmt.Println("  ytg cluster status --json")
	fmt.Println("  ytg cluster inspect node-tokyo-edge-01")
}

func handleClusterStatus(args []string) {
	fs := flag.NewFlagSet("cluster status", flag.ExitOnError)
	jsonOutput := fs.Bool("json", false, "Output cluster summary in JSON format")
	_ = fs.Parse(args)

	summary := DefaultGlobalClusterManager.GetSummary()

	if *jsonOutput {
		data, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "JSON encoding error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}

	fmt.Println("=== ytagarasu Distributed Cluster Status ===")
	fmt.Printf("Total Nodes: %d | Synced: %d (◆) | OutOfSync: %d (▲) | Degraded: %d (✖) | Drifted: %d (■)\n\n",
		summary.TotalNodes, summary.SyncedNodes, summary.OutOfSyncNodes, summary.DegradedNodes, summary.DriftedNodes)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "NODE ID\tROLE\tIP ADDRESS\tRELEASE (ACTIVE/DESIRED)\tSYNC STATUS\tHEALTH\tLATENCY")
	for _, n := range summary.Nodes {
		healthStr := fmt.Sprintf("%d cont / %d proc", n.ActiveContainers, n.ActiveProcesses)
		if n.RestartCount > 0 {
			healthStr += fmt.Sprintf(" (%d restarts)", n.RestartCount)
		}
		releaseStr := fmt.Sprintf("%s / %s", n.CurrentReleaseID, n.DesiredReleaseID)
		statusStr := fmt.Sprintf("%s %s", n.SyncStatus.Symbol(), n.SyncStatus)
		latencyStr := fmt.Sprintf("%dms", n.HeartbeatLatencyMs)

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			n.ID, n.Role, n.IPAddress, releaseStr, statusStr, healthStr, latencyStr)
	}
	_ = w.Flush()
}

func handleClusterInspect(args []string) {
	fs := flag.NewFlagSet("cluster inspect", flag.ExitOnError)
	jsonOutput := fs.Bool("json", false, "Output detailed node info in JSON format")
	_ = fs.Parse(args)

	remaining := fs.Args()
	if len(remaining) == 0 {
		fmt.Fprintln(os.Stderr, "Error: node-id argument is required")
		os.Exit(1)
	}

	nodeID := remaining[0]
	node, err := DefaultGlobalClusterManager.GetNode(nodeID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if *jsonOutput {
		data, err := json.MarshalIndent(node, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "JSON encoding error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(string(data))
		return
	}

	fmt.Printf("=== Node Telemetry: %s ===\n", node.ID)
	fmt.Printf("Hostname             : %s\n", node.Hostname)
	fmt.Printf("IP Address           : %s\n", node.IPAddress)
	fmt.Printf("Role                 : %s\n", node.Role)
	fmt.Printf("Active Release       : %s\n", node.CurrentReleaseID)
	fmt.Printf("Desired Release      : %s\n", node.DesiredReleaseID)
	fmt.Printf("Sync Status          : %s %s\n", node.SyncStatus.Symbol(), node.SyncStatus)
	fmt.Printf("Active Workloads     : %d containers, %d processes\n", node.ActiveContainers, node.ActiveProcesses)
	fmt.Printf("Restart Count        : %d\n", node.RestartCount)
	fmt.Printf("Uptime               : %d seconds\n", node.UptimeSeconds)
	fmt.Printf("Heartbeat Latency    : %d ms\n", node.HeartbeatLatencyMs)
	fmt.Printf("Last Heartbeat       : %s\n", node.LastHeartbeat.Format(time.RFC3339))
}
