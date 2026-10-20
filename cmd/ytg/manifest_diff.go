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
	"path/filepath"

	"github.com/sh0jitmy/ytagarasu/internal/delta"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
)

func handleDiffCommand(args []string) {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	manifestPath := fs.String("f", "manifest.yaml", "Path to desired state manifest file")
	statePath := fs.String("s", ".ytg/supervisor_state.json", "Path to actual supervisor runtime state file")
	jsonOutput := fs.Bool("json", false, "Output reconciliation plan in JSON format")
	approve := fs.Bool("approve", false, "Explicitly approve detected changes (escape hatch)")

	_ = fs.Parse(args)

	cleanManifest := filepath.Clean(*manifestPath)
	desired, err := manifest.ParseFile(cleanManifest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ytg diff error] failed to parse desired manifest %s: %v\n", cleanManifest, err)
		os.Exit(1)
	}

	cleanState := filepath.Clean(*statePath)
	actual, _ := supervisor.LoadState(cleanState) // If missing, treated as nil (all workloads new)

	plan := delta.DetectDrift(desired, actual)

	if *jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(plan); err != nil {
			fmt.Fprintf(os.Stderr, "[ytg diff error] failed to encode JSON plan: %v\n", err)
			os.Exit(1)
		}
		return
	}

	fmt.Print(plan.FormatDriftReport())

	if plan.RequiresApproval && !*approve {
		fmt.Println("⚠️  SAFETY GUARD: Changes require approval. Run with '--approve' to apply.")
		os.Exit(2)
	}
}
