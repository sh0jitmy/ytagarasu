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
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"gopkg.in/yaml.v3"
)

func handleManifestConvert(args []string) {
	fs := flag.NewFlagSet("manifest convert", flag.ExitOnError)
	composePath := fs.String("f", "docker-compose.yml", "Path to docker-compose.yml file")
	outputPath := fs.String("o", "manifest.yaml", "Output path for generated ytagarasu manifest")
	releaseName := fs.String("release", "compose-app", "Release identifier for converted manifest")

	_ = fs.Parse(args)

	cleanComposePath := filepath.Clean(*composePath)
	m, err := manifest.ConvertComposeFile(cleanComposePath, *releaseName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ytg manifest convert error] %v\n", err)
		os.Exit(1)
	}

	data, err := yaml.Marshal(m)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ytg manifest convert error] failed to serialize manifest: %v\n", err)
		os.Exit(1)
	}

	cleanOutPath := filepath.Clean(*outputPath)
	if err := os.WriteFile(cleanOutPath, data, 0640); err != nil { //nolint:gosec // user CLI output
		fmt.Fprintf(os.Stderr, "[ytg manifest convert error] failed writing manifest file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Successfully converted '%s' to ytagarasu manifest '%s'!\n", cleanComposePath, cleanOutPath)
	fmt.Printf("  • Release:    %s\n", m.Release)
	fmt.Printf("  • Containers: %d\n", len(m.Containers))
	for _, ctr := range m.Containers {
		portsStr := "none"
		if len(ctr.Ports) > 0 {
			portsStr = fmt.Sprintf("%v", ctr.Ports)
		}
		fmt.Printf("    - %-18s (Image: %s, Ports: %s)\n", ctr.Name, ctr.Image, portsStr)
	}
	fmt.Println("👉 Next steps:")
	fmt.Println("   1. Sign manifest: ytg manifest sign", cleanOutPath)
	fmt.Println("   2. Supervised run: ytg supervisor run -c", cleanOutPath)
}
