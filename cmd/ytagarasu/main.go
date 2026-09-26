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
	"fmt"
	"os"

	"github.com/shjtmy/go_sh0jitmy_template/internal/manifest"
	"github.com/shjtmy/go_sh0jitmy_template/internal/version"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

func main() {
	app := &cli.App{
		Name:    "ytagarasu",
		Usage:   "Offline Air-Gapped Deployment Platform CLI",
		Version: version.Version,
		Commands: []*cli.Command{
			{
				Name:  "manifest",
				Usage: "Manage deployment bundle manifests (init, generate, validate, lint)",
				Subcommands: []*cli.Command{
					{
						Name:  "init",
						Usage: "Interactive wizard to create a new manifest.yaml",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "output",
								Aliases: []string{"o"},
								Value:   "manifest.yaml",
								Usage:   "Output path for generated manifest",
							},
						},
						Action: runManifestInit,
					},
					{
						Name:  "generate",
						Usage: "Automatically scan project directory and generate draft manifest.yaml",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "dir",
								Aliases: []string{"d"},
								Value:   ".",
								Usage:   "Source project directory to scan",
							},
							&cli.StringFlag{
								Name:    "output",
								Aliases: []string{"o"},
								Value:   "manifest.yaml",
								Usage:   "Output path for generated manifest",
							},
							&cli.BoolFlag{
								Name:  "auto",
								Value: true,
								Usage: "Perform automated static scanning",
							},
						},
						Action: runManifestGenerate,
					},
					{
						Name:      "validate",
						Usage:     "Validate manifest syntax and schema conformity",
						ArgsUsage: "<path/to/manifest.yaml>",
						Action:    runManifestValidate,
					},
					{
						Name:      "lint",
						Usage:     "Run static analysis and security lint on manifest",
						ArgsUsage: "<path/to/manifest.yaml>",
						Action:    runManifestLint,
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runManifestInit(c *cli.Context) error {
	m, err := manifest.InitInteractive(os.Stdin, os.Stdout)
	if err != nil {
		return fmt.Errorf("wizard failed: %w", err)
	}

	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to encode YAML: %w", err)
	}

	outputPath := c.String("output")
	if err := os.WriteFile(outputPath, data, 0640); err != nil { //nolint:gosec // user readable manifest file
		return fmt.Errorf("failed to write %s: %w", outputPath, err)
	}

	fmt.Printf("✅ Successfully created manifest at: %s\n", outputPath)
	return nil
}

func runManifestGenerate(c *cli.Context) error {
	dir := c.String("dir")
	fmt.Printf("==> Scanning directory %s for application targets...\n", dir)

	m, err := manifest.GenerateAuto(dir)
	if err != nil {
		return fmt.Errorf("auto-generation failed: %w", err)
	}

	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to encode YAML: %w", err)
	}

	outputPath := c.String("output")
	if err := os.WriteFile(outputPath, data, 0640); err != nil { //nolint:gosec // user readable manifest file
		return fmt.Errorf("failed to write %s: %w", outputPath, err)
	}

	fmt.Printf("✅ Automatically generated manifest with %d application(s) at: %s\n", len(m.Applications), outputPath)
	return nil
}

func runManifestValidate(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("please provide path to manifest.yaml")
	}
	path := c.Args().First()
	fmt.Printf("==> Validating manifest: %s\n", path)

	m, err := manifest.ParseFile(path)
	if err != nil {
		return fmt.Errorf("❌ Validation failed: %w", err)
	}

	fmt.Printf("✅ Manifest is valid! (release: %s, targets: %d, applications: %d)\n",
		m.Release, len(m.Targets), len(m.Applications))
	return nil
}

func runManifestLint(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("please provide path to manifest.yaml")
	}
	path := c.Args().First()
	fmt.Printf("==> Linting manifest: %s\n", path)

	m, err := manifest.ParseFile(path)
	if err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	issues := manifest.Lint(m)
	if len(issues) == 0 {
		fmt.Println("✅ No issues found. Manifest passed all lint checks!")
		return nil
	}

	hasError := false
	for _, issue := range issues {
		fmt.Printf("  %s\n", issue)
		if issue.Severity == manifest.LintError {
			hasError = true
		}
	}

	if hasError {
		return fmt.Errorf("lint inspection failed with errors")
	}
	fmt.Printf("⚠️ Completed with %d warning(s)\n", len(issues))
	return nil
}
