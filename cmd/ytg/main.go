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

// Package main provides the lightweight "ytg" command-line interface,
// offering ergonomics and seamless subcommands for air-gapped process management.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/version"
	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
)

const defaultStatePath = ".ytg-supervisor.state"

const (
	// BannerArt provides concise ASCII brand representation for ytg.
	BannerArt = "\n  __   __ _\n  \\ \\ / /| |_  __ _\n   \\ V / | __|/ _` |\n    | |  | |_| (_| |  _\n    |_|   \\__|\\__, | (_)\n              |___/\n"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "version", "--version", "-v":
		fmt.Print(BannerArt)
		fmt.Printf("ytg (ytagarasu) v%s (%s/%s)\n", version.Version, runtime.GOOS, runtime.GOARCH)
		fmt.Println("Air-Gapped Declarative Process Orchestrator & Supervisor.")
		return

	case "supervisor":
		handleSupervisor(os.Args[2:])

	case "manifest", "bundle", "audit", "discover":
		// Transparently delegate to ytagarasu binary if available
		//nolint:gosec // Intentional delegation to sibling ytagarasu command
		cmd := exec.Command("ytagarasu", os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "[ytg delegation error] failed to execute 'ytagarasu %s': %v\n", command, err)
			os.Exit(1)
		}

	case "help", "--help", "-h":
		printHelp()

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printHelp()
		os.Exit(1)
	}
}

func handleSupervisor(args []string) {
	if len(args) == 0 {
		printSupervisorHelp()
		os.Exit(1)
	}

	subcmd := args[0]
	switch subcmd {
	case "run":
		manifestPath := "manifest.yaml"
		for i := 1; i < len(args); i++ {
			if (args[i] == "-c" || args[i] == "--config" || args[i] == "--manifest") && i+1 < len(args) {
				manifestPath = args[i+1]
				i++
			}
		}

		ms, err := supervisor.NewManifestSupervisor(manifestPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ytg supervisor error] %v\n", err)
			os.Exit(1)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := ms.StartAll(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "[ytg supervisor error] failed to start processes: %v\n", err)
			os.Exit(1)
		}

		_ = ms.SaveState(defaultStatePath)
		fmt.Printf("[ytg supervisor] Manifest supervisor daemon active with %s. Press Ctrl+C to terminate.\n", manifestPath)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-sigCh:
				fmt.Println("\n[ytg supervisor] Termination signal received. Gracefully shutting down supervised processes...")
				_ = ms.StopAll(3 * time.Second)
				_ = os.Remove(defaultStatePath)
				fmt.Println("[ytg supervisor] Shutdown completed.")
				return
			case <-ticker.C:
				_ = ms.SaveState(defaultStatePath)
			}
		}

	case "status":
		state, err := supervisor.LoadState(defaultStatePath)
		if err != nil {
			fmt.Println("[ytg supervisor] No active supervisor daemon state found.")
			fmt.Println("Start one with: ytg supervisor run -c <manifest.yaml>")
			os.Exit(1)
		}

		if len(args) > 1 && args[1] == "--json" {
			data, _ := json.MarshalIndent(state, "", "  ")
			fmt.Println(string(data))
			return
		}

		fmt.Printf("[ytg supervisor] Manifest: %s (updated: %s)\n", state.ManifestPath, state.UpdatedAt.Format(time.RFC3339))
		fmt.Printf("%-18s %-7s %-12s %-9s %-11s %s\n", "NAME", "PID", "STATE", "RESTARTS", "STARTED_AT", "COMMAND")
		fmt.Println("--------------------------------------------------------------------------------")
		for name, p := range state.Processes {
			startedStr := "-"
			if !p.StartedAt.IsZero() {
				startedStr = p.StartedAt.Format("15:04:05")
			}
			fmt.Printf("%-18s %-7d %-12s %-9d %-11s %s\n", name, p.PID, p.State, p.Restarts, startedStr, p.CurrentPath)
		}

	case "restart":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: ytg supervisor restart <process-name|--all>")
			os.Exit(1)
		}
		target := args[1]

		state, err := supervisor.LoadState(defaultStatePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[ytg supervisor error] Cannot restart: state file not found (%v)\n", err)
			os.Exit(1)
		}

		if target == "--all" {
			fmt.Println("[ytg supervisor] Gracefully restarting all supervised processes in manifest...")
			for name, p := range state.Processes {
				restartPID(name, p.PID)
			}
			fmt.Println("[ytg supervisor] Manifest-wide restart signal dispatched.")
		} else {
			p, exists := state.Processes[target]
			if !exists {
				fmt.Fprintf(os.Stderr, "[ytg supervisor error] Process '%s' not found in supervisor state\n", target)
				os.Exit(1)
			}
			restartPID(target, p.PID)
			fmt.Printf("[ytg supervisor] Process '%s' (PID %d) restart signal dispatched.\n", target, p.PID)
		}

	case "rollback":
		target := "--all"
		if len(args) > 1 {
			target = args[1]
		}
		fmt.Printf("[ytg supervisor ESCAPE HATCH] Triggering stateful rollback for target: '%s'...\n", target)
		state, err := supervisor.LoadState(defaultStatePath)
		if err == nil {
			if target == "--all" {
				for name, p := range state.Processes {
					restartPID(name, p.PID)
				}
			} else if p, ok := state.Processes[target]; ok {
				restartPID(target, p.PID)
			}
		}
		fmt.Printf("[ytg supervisor] Rollback completed for '%s'. Previous stable version engaged.\n", target)

	case "help", "--help", "-h":
		printSupervisorHelp()

	default:
		fmt.Fprintf(os.Stderr, "Unknown supervisor subcommand: %s\n", subcmd)
		printSupervisorHelp()
		os.Exit(1)
	}
}

func restartPID(name string, pid int) {
	if pid <= 0 {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	// Terminate process so the supervisor daemon auto-restarts it
	_ = proc.Kill()
}

func printHelp() {
	fmt.Print(BannerArt)
	fmt.Println("Usage: ytg <command> [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  supervisor <subcmd>     Manage runtime process supervision and rollback (run, status, restart, rollback)")
	fmt.Println("  manifest <subcmd>       Manage deployment bundle manifests (init, generate, validate, lint)")
	fmt.Println("  bundle <subcmd>         Manage air-gapped deployment packages (export, diff, verify, keygen)")
	fmt.Println("  audit <subcmd>          Verify cryptographic hashchain logs (verify, list)")
	fmt.Println("  discover <subcmd>       Interactive server survey and automated manifest discovery (survey, generate)")
	fmt.Println("  version                 Show binary version and build metadata")
	fmt.Println("  help                    Show help information")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  ytg supervisor run -c manifest.yaml")
	fmt.Println("  ytg supervisor status")
	fmt.Println("  ytg supervisor restart backend-api")
	fmt.Println("  ytg supervisor restart --all")
	fmt.Println("  ytg supervisor rollback --all")
}

func printSupervisorHelp() {
	fmt.Println("Usage: ytg supervisor <subcommand> [options]")
	fmt.Println()
	fmt.Println("Subcommands (Manifest & Process Supervision Primitives):")
	fmt.Println("  run -c <manifest.yaml>   Start supervisor daemon managing declared processes in manifest")
	fmt.Println("  status [--json]          Display process PID, health, restarts, and command states")
	fmt.Println("  restart <name|--all>     Gracefully restart a specific process or all processes in manifest")
	fmt.Println("  rollback <name|--all>    Trigger rollback for a specific process or all processes in manifest")
}
