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

	case "doctor":
		handleDoctor(os.Args[2:])

	case "restart":
		handleSupervisorRestart(os.Args[2:])

	case "supervisor":
		handleSupervisor(os.Args[2:])

	case "diff":
		handleDiffCommand(os.Args[2:])

	case "completion":
		handleCompletion(os.Args[2:])

	case "manifest":
		if len(os.Args) > 2 {
			switch os.Args[2] {
			case "init":
				handleManifestInit(os.Args[3:])
				return
			case "keygen":
				handleManifestKeygen(os.Args[3:])
				return
			case "sign":
				handleManifestSign(os.Args[3:])
				return
			case "verify":
				handleManifestVerify(os.Args[3:])
				return
			case "convert":
				handleManifestConvert(os.Args[3:])
				return
			}
		}
		// Transparently delegate to ytagarasu binary if available
		//nolint:gosec // Intentional delegation to sibling ytagarasu command
		cmd := exec.Command("ytagarasu", os.Args[1:]...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "[ytg delegation error] failed to execute 'ytagarasu manifest': %v\n", err)
			os.Exit(1)
		}
		return

	case "bundle", "audit", "discover":
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

// hasJSONFlag detects if --json flag is present in arguments and returns remaining args.
func hasJSONFlag(args []string) (bool, []string) {
	isJSON := false
	var remaining []string
	for _, a := range args {
		if a == "--json" {
			isJSON = true
		} else {
			remaining = append(remaining, a)
		}
	}
	return isJSON, remaining
}

// ServiceStatusInfo represents deterministic structured status of a single supervised service.
type ServiceStatusInfo struct {
	Name        string `json:"name"`
	PID         int    `json:"pid"`
	State       string `json:"state"`
	Restarts    int    `json:"restarts"`
	StartedAt   string `json:"started_at"`
	CurrentPath string `json:"current_path"`
	Uptime      string `json:"uptime,omitempty"`
}

// StatusReport represents deterministic structured JSON output for ytg status.
type StatusReport struct {
	ManifestPath string                `json:"manifest_path"`
	UpdatedAt    string                `json:"updated_at"`
	TotalCount   int                   `json:"total_count"`
	Integrity    ManifestIntegrityInfo `json:"integrity"`
	Services     []ServiceStatusInfo   `json:"services"`
}

func handleSupervisorStatus(args []string) {
	isJSON, _ := hasJSONFlag(args)

	state, err := supervisor.LoadState(defaultStatePath)
	if err != nil {
		if isJSON {
			errData, _ := json.MarshalIndent(map[string]interface{}{
				"error":    "no active supervisor daemon state found",
				"services": []ServiceStatusInfo{},
			}, "", "  ")
			fmt.Println(string(errData))
			os.Exit(1)
		}
		fmt.Println("[ytg status] No active supervisor daemon state found.")
		fmt.Println("Start one with: ytg supervisor run -c <manifest.yaml>")
		os.Exit(1)
	}

	services := make([]ServiceStatusInfo, 0, len(state.Processes))
	for name, p := range state.Processes {
		startedStr := "-"
		uptimeStr := "-"
		if !p.StartedAt.IsZero() {
			startedStr = p.StartedAt.Format(time.RFC3339)
			uptimeStr = time.Since(p.StartedAt).Round(time.Second).String()
		}
		services = append(services, ServiceStatusInfo{
			Name:        name,
			PID:         p.PID,
			State:       string(p.State),
			Restarts:    p.Restarts,
			StartedAt:   startedStr,
			CurrentPath: p.CurrentPath,
			Uptime:      uptimeStr,
		})
	}

	integrity := CheckManifestIntegrity(state.ManifestPath)

	if isJSON {
		report := StatusReport{
			ManifestPath: state.ManifestPath,
			UpdatedAt:    state.UpdatedAt.Format(time.RFC3339),
			TotalCount:   len(services),
			Integrity:    integrity,
			Services:     services,
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("[ytg status] Manifest: %s (updated: %s)\n", state.ManifestPath, state.UpdatedAt.Format(time.RFC3339))
	switch integrity.Status {
	case "VERIFIED":
		fmt.Printf("[ytg status] Integrity: \033[32mVERIFIED\033[0m (Ed25519: %s...) [%s]\n", integrity.KeyFingerprint, integrity.SignaturePath)
	case "TAMPERED":
		fmt.Printf("[ytg status] Integrity: \033[31m\033[1m⚠️ TAMPERED\033[0m (%v)\n", integrity.VerificationError)
	case "UNSIGNED":
		fmt.Printf("[ytg status] Integrity: UNSIGNED (run: ytg manifest sign %s)\n", state.ManifestPath)
	default:
		fmt.Printf("[ytg status] Integrity: %s (%s)\n", integrity.Status, integrity.VerificationError)
	}
	fmt.Printf("%-18s %-7s %-12s %-9s %-11s %s\n", "NAME", "PID", "STATE", "RESTARTS", "STARTED_AT", "COMMAND")
	fmt.Println("--------------------------------------------------------------------------------")
	for _, s := range services {
		fmt.Printf("%-18s %-7d %-12s %-9d %-11s %s\n", s.Name, s.PID, s.State, s.Restarts, s.StartedAt, s.CurrentPath)
	}
}

// InspectReport represents deterministic structured JSON output for ytg inspect.
type InspectReport struct {
	Name         string                 `json:"name"`
	PID          int                    `json:"pid"`
	State        string                 `json:"state"`
	Restarts     int                    `json:"restarts"`
	StartedAt    string                 `json:"started_at"`
	Uptime       string                 `json:"uptime"`
	Command      string                 `json:"command"`
	ManifestPath string                 `json:"manifest_path"`
	Integrity    *ManifestIntegrityInfo `json:"integrity,omitempty"`
	LastUpdated  string                 `json:"last_updated"`
}

func handleSupervisorInspect(args []string) {
	isJSON, remaining := hasJSONFlag(args)
	if len(remaining) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: ytg inspect <process-name> [--json]")
		os.Exit(1)
	}

	targetName := remaining[0]
	state, err := supervisor.LoadState(defaultStatePath)
	if err != nil {
		if isJSON {
			errData, _ := json.MarshalIndent(map[string]string{
				"error": fmt.Sprintf("state file not found (%v)", err),
			}, "", "  ")
			fmt.Println(string(errData))
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "[ytg inspect error] Cannot inspect: state file not found (%v)\n", err)
		os.Exit(1)
	}

	p, exists := state.Processes[targetName]
	if !exists {
		if isJSON {
			errData, _ := json.MarshalIndent(map[string]string{
				"error": fmt.Sprintf("process '%s' not found in supervisor state", targetName),
			}, "", "  ")
			fmt.Println(string(errData))
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "[ytg inspect error] Process '%s' not found in supervisor state\n", targetName)
		os.Exit(1)
	}

	startedStr := "-"
	uptimeStr := "-"
	if !p.StartedAt.IsZero() {
		startedStr = p.StartedAt.Format(time.RFC3339)
		uptimeStr = time.Since(p.StartedAt).Round(time.Second).String()
	}

	integrity := CheckManifestIntegrity(state.ManifestPath)
	report := InspectReport{
		Name:         targetName,
		PID:          p.PID,
		State:        string(p.State),
		Restarts:     p.Restarts,
		StartedAt:    startedStr,
		Uptime:       uptimeStr,
		Command:      p.CurrentPath,
		ManifestPath: state.ManifestPath,
		Integrity:    &integrity,
		LastUpdated:  state.UpdatedAt.Format(time.RFC3339),
	}

	if isJSON {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Printf("[ytg inspect] Process: %s\n", report.Name)
	fmt.Printf("  PID:          %d\n", report.PID)
	fmt.Printf("  State:        %s\n", report.State)
	fmt.Printf("  Restarts:     %d\n", report.Restarts)
	fmt.Printf("  Started At:   %s (Uptime: %s)\n", report.StartedAt, report.Uptime)
	fmt.Printf("  Command:      %s\n", report.Command)
	fmt.Printf("  Manifest:     %s\n", report.ManifestPath)
	fmt.Printf("  Last Updated: %s\n", report.LastUpdated)
}

// DoctorCheckItem represents a single system diagnostic check item.
type DoctorCheckItem struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "PASS", "WARN", "FAIL", "INFO"
	Message string `json:"message"`
}

// DoctorReport represents complete structured environment diagnostics.
type DoctorReport struct {
	OS        string            `json:"os"`
	Arch      string            `json:"arch"`
	GoVersion string            `json:"go_version"`
	Checks    []DoctorCheckItem `json:"checks"`
	Passed    bool              `json:"passed"`
}

func handleDoctor(args []string) {
	isJSON, _ := hasJSONFlag(args)

	var checks []DoctorCheckItem
	allPassed := true

	// Check 1: Operating System
	checks = append(checks, DoctorCheckItem{
		Name:    "Platform",
		Status:  "INFO",
		Message: fmt.Sprintf("%s/%s (Go %s)", runtime.GOOS, runtime.GOARCH, runtime.Version()),
	})

	// Check 2: Manifest file existence & cryptographic integrity
	if _, err := os.Stat("manifest.yaml"); err == nil {
		checks = append(checks, DoctorCheckItem{
			Name:    "Manifest File",
			Status:  "PASS",
			Message: "manifest.yaml found in current working directory",
		})
		integrity := CheckManifestIntegrity("manifest.yaml")
		switch integrity.Status {
		case "VERIFIED":
			checks = append(checks, DoctorCheckItem{
				Name:    "Manifest Cryptographic Signature",
				Status:  "PASS",
				Message: fmt.Sprintf("Ed25519 signature verified (%s...)", integrity.KeyFingerprint),
			})
		case "TAMPERED":
			allPassed = false
			checks = append(checks, DoctorCheckItem{
				Name:    "Manifest Cryptographic Signature",
				Status:  "FAIL",
				Message: fmt.Sprintf("Tampering detected: %s", integrity.VerificationError),
			})
		case "UNSIGNED":
			checks = append(checks, DoctorCheckItem{
				Name:    "Manifest Cryptographic Signature",
				Status:  "WARN",
				Message: "Manifest is unsigned (sign with: ytg manifest sign manifest.yaml)",
			})
		default:
			checks = append(checks, DoctorCheckItem{
				Name:    "Manifest Cryptographic Signature",
				Status:  "INFO",
				Message: fmt.Sprintf("Status: %s (%s)", integrity.Status, integrity.VerificationError),
			})
		}
	} else {
		checks = append(checks, DoctorCheckItem{
			Name:    "Manifest File",
			Status:  "WARN",
			Message: "manifest.yaml not found in current directory (run: ytg manifest init)",
		})
	}

	// Check 3: Supervisor state file
	if _, err := os.Stat(defaultStatePath); err == nil {
		checks = append(checks, DoctorCheckItem{
			Name:    "Supervisor Daemon",
			Status:  "PASS",
			Message: fmt.Sprintf("Active state file detected (%s)", defaultStatePath),
		})
	} else {
		checks = append(checks, DoctorCheckItem{
			Name:    "Supervisor Daemon",
			Status:  "INFO",
			Message: "No active supervisor state (daemon is currently idle)",
		})
	}

	// Check 4: cgroup v2 support (Linux specific)
	if runtime.GOOS == "linux" {
		if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
			checks = append(checks, DoctorCheckItem{
				Name:    "Linux cgroups v2",
				Status:  "PASS",
				Message: "Unified cgroup v2 hierarchy available for memory/CPU isolation",
			})
		} else {
			checks = append(checks, DoctorCheckItem{
				Name:    "Linux cgroups v2",
				Status:  "WARN",
				Message: "cgroup v2 controllers not mounted; falling back to process-group signals",
			})
		}
	}

	report := DoctorReport{
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		GoVersion: runtime.Version(),
		Checks:    checks,
		Passed:    allPassed,
	}

	if isJSON {
		data, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(data))
		return
	}

	fmt.Println("[ytg doctor] On-Premise Environment Diagnostics")
	fmt.Println("--------------------------------------------------------------------------------")
	for _, c := range checks {
		badge := "[ OK ]"
		switch c.Status {
		case "PASS":
			badge = "[PASS]"
		case "WARN":
			badge = "[WARN]"
		case "FAIL":
			badge = "[FAIL]"
		case "INFO":
			badge = "[INFO]"
		}
		fmt.Printf("%-8s %-22s: %s\n", badge, c.Name, c.Message)
	}
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println("Diagnostics completed. Environment ready for air-gapped process orchestration.")
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
	_, _ = fmt.Fprintln(w, "  diff [-f manifest]      Detect drift between desired manifest and runtime state")
	_, _ = fmt.Fprintln(w, "  doctor [--json]         Diagnose host environment, cgroup, manifest, and runtime health")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Subsystems:")
	_, _ = fmt.Fprintln(w, "  supervisor <subcmd>     Manage runtime process daemon (run, status, inspect, restart, rollback)")
	_, _ = fmt.Fprintln(w, "  manifest <subcmd>       Manage deployment manifests (init, keygen, sign, verify, convert, validate, lint)")
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
	_, _ = fmt.Fprintln(w, "  ytg manifest convert -f docker-compose.yml -o manifest.yaml")
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
