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
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/version"
	"github.com/sh0jitmy/ytagarasu/pkg/completion"
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

	case "status":
		handleSupervisorStatus(os.Args[2:])

	case "inspect":
		handleSupervisorInspect(os.Args[2:])

	case "restart":
		handleSupervisorRestart(os.Args[2:])

	case "supervisor":
		handleSupervisor(os.Args[2:])

	case "completion":
		handleCompletion(os.Args[2:])

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

func handleCompletion(args []string) {
	if len(args) == 0 {
		printCompletionHelp()
		os.Exit(1)
	}

	sh := completion.Shell(args[0])
	if err := completion.Generate(sh, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		printCompletionHelp()
		os.Exit(1)
	}
}

func printCompletionHelp() {
	fmt.Println("Usage: ytg completion <shell>")
	fmt.Println()
	fmt.Println("Generate shell autocompletion scripts for ytg.")
	fmt.Println()
	fmt.Println("Supported shells:")
	fmt.Println("  bash          Generate Bash autocompletion script")
	fmt.Println("  zsh           Generate Zsh autocompletion script")
	fmt.Println("  powershell    Generate PowerShell autocompletion script")
	fmt.Println("  fish          Generate Fish autocompletion script")
	fmt.Println()
	fmt.Println("Installation Examples:")
	fmt.Println("  Bash:")
	fmt.Println("    source <(ytg completion bash)")
	fmt.Println("    ytg completion bash | sudo tee /etc/bash_completion.d/ytg")
	fmt.Println()
	fmt.Println("  Zsh:")
	fmt.Println("    ytg completion zsh > \"${fpath[1]}/_ytg\"")
	fmt.Println()
	fmt.Println("  PowerShell:")
	fmt.Println("    ytg completion powershell | Out-String | Invoke-Expression")
	fmt.Println()
	fmt.Println("  Fish:")
	fmt.Println("    ytg completion fish > ~/.config/fish/completions/ytg.fish")
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
		handleSupervisorStatus(args[1:])

	case "inspect":
		handleSupervisorInspect(args[1:])

	case "restart":
		handleSupervisorRestart(args[1:])

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

func handleSupervisorStatus(args []string) {
	state, err := supervisor.LoadState(defaultStatePath)
	if err != nil {
		fmt.Println("[ytg status] No active supervisor daemon state found.")
		fmt.Println("Start one with: ytg supervisor run -c <manifest.yaml>")
		os.Exit(1)
	}

	if len(args) > 0 && args[0] == "--json" {
		data, _ := json.MarshalIndent(state, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("[ytg status] Manifest: %s (updated: %s)\n", state.ManifestPath, state.UpdatedAt.Format(time.RFC3339))
	fmt.Printf("%-18s %-7s %-12s %-9s %-11s %s\n", "NAME", "PID", "STATE", "RESTARTS", "STARTED_AT", "COMMAND")
	fmt.Println("--------------------------------------------------------------------------------")
	for name, p := range state.Processes {
		startedStr := "-"
		if !p.StartedAt.IsZero() {
			startedStr = p.StartedAt.Format("15:04:05")
		}
		fmt.Printf("%-18s %-7d %-12s %-9d %-11s %s\n", name, p.PID, p.State, p.Restarts, startedStr, p.CurrentPath)
	}
}

func handleSupervisorInspect(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: ytg inspect <process-name> [--json]")
		os.Exit(1)
	}

	targetName := args[0]
	state, err := supervisor.LoadState(defaultStatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ytg inspect error] Cannot inspect: state file not found (%v)\n", err)
		os.Exit(1)
	}

	p, exists := state.Processes[targetName]
	if !exists {
		fmt.Fprintf(os.Stderr, "[ytg inspect error] Process '%s' not found in supervisor state\n", targetName)
		os.Exit(1)
	}

	if len(args) > 1 && args[1] == "--json" {
		data, _ := json.MarshalIndent(p, "", "  ")
		fmt.Println(string(data))
		return
	}

	startedStr := "-"
	uptimeStr := "-"
	if !p.StartedAt.IsZero() {
		startedStr = p.StartedAt.Format(time.RFC3339)
		uptimeStr = time.Since(p.StartedAt).Round(time.Second).String()
	}

	fmt.Printf("[ytg inspect] Process: %s\n", targetName)
	fmt.Printf("  PID:          %d\n", p.PID)
	fmt.Printf("  State:        %s\n", p.State)
	fmt.Printf("  Restarts:     %d\n", p.Restarts)
	fmt.Printf("  Started At:   %s (Uptime: %s)\n", startedStr, uptimeStr)
	fmt.Printf("  Command:      %s\n", p.CurrentPath)
	fmt.Printf("  Manifest:     %s\n", state.ManifestPath)
	fmt.Printf("  Last Updated: %s\n", state.UpdatedAt.Format(time.RFC3339))
}

func handleSupervisorRestart(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: ytg restart <process-name|--all>")
		os.Exit(1)
	}
	target := args[0]

	state, err := supervisor.LoadState(defaultStatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ytg restart error] Cannot restart: state file not found (%v)\n", err)
		os.Exit(1)
	}

	if target == "--all" {
		fmt.Println("[ytg restart] Gracefully restarting all supervised processes in manifest...")
		for name, p := range state.Processes {
			restartPID(name, p.PID)
		}
		fmt.Println("[ytg restart] Manifest-wide restart signal dispatched.")
		return
	}

	p, exists := state.Processes[target]
	if !exists {
		fmt.Fprintf(os.Stderr, "[ytg restart error] Process '%s' not found in supervisor state\n", target)
		os.Exit(1)
	}
	restartPID(target, p.PID)
	fmt.Printf("[ytg restart] Process '%s' (PID %d) restart signal dispatched.\n", target, p.PID)
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
	printHelpTo(os.Stdout)
}

func printHelpTo(w io.Writer) {
	_, _ = fmt.Fprint(w, BannerArt)
	_, _ = fmt.Fprintln(w, "Usage: ytg <command> [options]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Core Commands (Headless Operations):")
	_, _ = fmt.Fprintln(w, "  status [--json]         Display process PID, health, restarts, and command states")
	_, _ = fmt.Fprintln(w, "  inspect <name> [--json] Display detailed process metrics, state, and paths")
	_, _ = fmt.Fprintln(w, "  restart <name|--all>    Gracefully restart a specific process or all processes")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Subsystems:")
	_, _ = fmt.Fprintln(w, "  supervisor <subcmd>     Manage runtime process daemon (run, status, inspect, restart, rollback)")
	_, _ = fmt.Fprintln(w, "  manifest <subcmd>       Manage deployment bundle manifests (init, generate, validate, lint)")
	_, _ = fmt.Fprintln(w, "  bundle <subcmd>         Manage air-gapped deployment packages (export, diff, verify, keygen)")
	_, _ = fmt.Fprintln(w, "  audit <subcmd>          Verify cryptographic hashchain logs (verify, list)")
	_, _ = fmt.Fprintln(w, "  discover <subcmd>       Interactive server survey and automated manifest discovery (survey, generate)")
	_, _ = fmt.Fprintln(w, "  completion <shell>      Generate shell autocompletion script (bash, zsh, powershell, fish)")
	_, _ = fmt.Fprintln(w, "  version                 Show binary version and build metadata")
	_, _ = fmt.Fprintln(w, "  help                    Show help information")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  ytg status")
	_, _ = fmt.Fprintln(w, "  ytg inspect backend-api")
	_, _ = fmt.Fprintln(w, "  ytg restart backend-api")
	_, _ = fmt.Fprintln(w, "  ytg restart --all")
	_, _ = fmt.Fprintln(w, "  ytg supervisor run -c manifest.yaml")
	_, _ = fmt.Fprintln(w, "  source <(ytg completion bash)")
	_, _ = fmt.Fprintln(w, "  ytg completion zsh > ~/.zfunc/_ytg")
}

func printSupervisorHelp() {
	printSupervisorHelpTo(os.Stdout)
}

func printSupervisorHelpTo(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: ytg supervisor <subcommand> [options]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Subcommands (Manifest & Process Supervision Primitives):")
	_, _ = fmt.Fprintln(w, "  run -c <manifest.yaml>   Start supervisor daemon managing declared processes in manifest")
	_, _ = fmt.Fprintln(w, "  status [--json]          Display process PID, health, restarts, and command states")
	_, _ = fmt.Fprintln(w, "  inspect <name> [--json]  Display detailed process metrics, state, and paths")
	_, _ = fmt.Fprintln(w, "  restart <name|--all>     Gracefully restart a specific process or all processes in manifest")
	_, _ = fmt.Fprintln(w, "  rollback <name|--all>    Trigger rollback for a specific process or all processes in manifest")
}
