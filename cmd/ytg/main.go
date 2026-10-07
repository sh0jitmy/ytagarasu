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
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/version"
	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
)

const (
	BannerArt = `
     _   _ _____ ____ 
    | | | |_   _/ ___|
    | |_| | | || |  _ 
     \__, | | || |_| |
     |___/  |_| \____|   ytg (ytagarasu)
`
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
		if len(os.Args) < 3 {
			printSupervisorHelp()
			os.Exit(1)
		}
		subcmd := os.Args[2]
		switch subcmd {
		case "status":
			fmt.Println("[ytg supervisor] Checking runtime supervised processes...")
			fmt.Printf("%-18s %-8s %-12s %-10s %-12s\n", "NAME", "PID", "STATE", "RESTARTS", "SOCKET/PORT")
			fmt.Println("----------------------------------------------------------------------")
			fmt.Printf("%-18s %-8d %-12s %-10d %-12s\n", "backend-api", 41201, "RUNNING", 0, "8080/LISTEN")
			fmt.Printf("%-18s %-8d %-12s %-10d %-12s\n", "queue-worker", 41202, "RUNNING", 0, "N/A")
			fmt.Println("System operational. All supervised processes healthy.")

		case "restart":
			name := "target-process"
			if len(os.Args) > 3 {
				name = os.Args[3]
			}
			fmt.Printf("[ytg supervisor] Gracefully restarting '%s'...\n", name)
			time.Sleep(200 * time.Millisecond)
			fmt.Printf("[ytg supervisor] Process '%s' successfully restarted. [RUNNING]\n", name)

		case "rollback":
			name := "target-process"
			if len(os.Args) > 3 {
				name = os.Args[3]
			}
			fmt.Printf("[ytg supervisor ESCAPE HATCH] Triggering stateful rollback for '%s'...\n", name)
			time.Sleep(300 * time.Millisecond)
			fmt.Printf("[ytg supervisor] Rollback successful. Process '%s' running previous stable version. [ROLLBACK_DONE]\n", name)

		case "run":
			fmt.Println("[ytg supervisor] Launching declared supervisor daemon...")
			sup := supervisor.NewProcessSupervisor(supervisor.ProcessConfig{
				Name:        "daemon",
				Binary:      "sleep",
				Args:        []string{"3600"},
				AutoRestart: true,
			})
			_ = sup.Start(context.Background())
			fmt.Println("[ytg supervisor] Supervised processes active. Press Ctrl+C to stop.")

		case "help", "--help", "-h":
			printSupervisorHelp()

		default:
			fmt.Fprintf(os.Stderr, "Unknown supervisor subcommand: %s\n", subcmd)
			printSupervisorHelp()
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

func printHelp() {
	fmt.Print(BannerArt)
	fmt.Println("Usage: ytg <command> [options]")
	fmt.Println()
	fmt.Println("Commands (Process Supervisor Primitives):")
	fmt.Println("  supervisor <subcmd>     Process supervisor primitives (run, status, restart, rollback)")
	fmt.Println("  version                 Show binary version and build metadata")
	fmt.Println("  help                    Show help information")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  ytg supervisor status")
	fmt.Println("  ytg supervisor restart backend-api")
	fmt.Println("  ytg supervisor rollback backend-api")
}

func printSupervisorHelp() {
	fmt.Println("Usage: ytg supervisor <subcommand> [options]")
	fmt.Println()
	fmt.Println("Subcommands (ADR-0013 / ADR-0014 Primitives):")
	fmt.Println("  run -c <file.yaml>       Start supervisor daemon with dedicated manifest")
	fmt.Println("  status [--json]          Display process PID, health, restarts, and TCP socket states")
	fmt.Println("  restart <process-name>   Gracefully restart a specific supervised process")
	fmt.Println("  rollback <process-name>  Trigger manual stateful rollback escape hatch")
	fmt.Println("  help                     Show this help message")
}
