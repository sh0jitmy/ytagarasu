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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sh0jitmy/ytagarasu/internal/audit"
	"github.com/sh0jitmy/ytagarasu/internal/bundle"
	"github.com/sh0jitmy/ytagarasu/internal/delta"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/sh0jitmy/ytagarasu/internal/pkgengine"
	"github.com/sh0jitmy/ytagarasu/internal/server/store"
	"github.com/sh0jitmy/ytagarasu/internal/version"
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
			{
				Name:    "package",
				Aliases: []string{"pkg"},
				Usage:   "Resolve and inspect OS package dependencies (APT, RPM)",
				Subcommands: []*cli.Command{
					{
						Name:      "resolve",
						Usage:     "Resolve recursive package dependencies from repository index",
						ArgsUsage: "<pkg1> [pkg2...]",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "apt",
								Usage: "Path to Debian/Ubuntu Packages or Packages.gz/xz index",
							},
							&cli.StringFlag{
								Name:  "rpm",
								Usage: "Path to RHEL/Rocky primary.xml or primary.xml.gz index",
							},
						},
						Action: runPackageResolve,
					},
				},
			},
			{
				Name:  "bundle",
				Usage: "Manage, export, and verify offline deployment bundles",
				Subcommands: []*cli.Command{
					{
						Name:  "export",
						Usage: "Evaluate, sign, and build an offline deployment bundle archive",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "manifest",
								Aliases: []string{"m"},
								Value:   "manifest.yaml",
								Usage:   "Path to manifest.yaml",
							},
							&cli.StringFlag{
								Name:    "source",
								Aliases: []string{"s"},
								Value:   ".",
								Usage:   "Root directory containing bundle assets",
							},
							&cli.StringFlag{
								Name:    "output",
								Aliases: []string{"o"},
								Value:   "",
								Usage:   "Output bundle archive path (.tar.gz)",
							},
							&cli.StringFlag{
								Name:  "report",
								Value: "preparation-result.yaml",
								Usage: "Path to save preparation result evaluation report",
							},
							&cli.StringFlag{
								Name:  "key",
								Usage: "Path to Ed25519 private key for signing (generated if omitted)",
							},
							&cli.BoolFlag{
								Name:  "force",
								Usage: "Allow building bundle even if evaluation status is warning",
							},
							&cli.StringFlag{
								Name:  "delta-from",
								Usage: "Path to previous release bundle (.tar.gz) to build a delta bundle",
							},
						},
						Action: runBundleExport,
					},
					{
						Name:      "diff",
						Usage:     "Compare two deployment bundles and display differential asset metrics",
						ArgsUsage: "<base-bundle.tar.gz> <target-bundle.tar.gz>",
						Action:    runBundleDiff,
					},
					{
						Name:      "verify",
						Usage:     "Verify integrity, checksums, and Ed25519 signature of a bundle",
						ArgsUsage: "<path/to/bundle.tar.gz>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "key",
								Usage: "Path to Ed25519 public key (uses packaged key if omitted)",
							},
						},
						Action: runBundleVerify,
					},
					{
						Name:  "keygen",
						Usage: "Generate a new Ed25519 keypair for bundle signing and verification",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "dir",
								Aliases: []string{"d"},
								Value:   ".",
								Usage:   "Directory to save private.key and public.key",
							},
						},
						Action: runBundleKeygen,
					},
				},
			},
			{
				Name:  "keygen",
				Usage: "Generate a new Ed25519 keypair for bundle signing and verification",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "dir",
						Aliases: []string{"d"},
						Value:   ".",
						Usage:   "Directory to save private.key and public.key",
					},
				},
				Action: runBundleKeygen,
			},
			{
				Name:  "audit",
				Usage: "Inspect and verify tamper-proof audit trail hash chain",
				Subcommands: []*cli.Command{
					{
						Name:  "verify",
						Usage: "Verify cryptographic integrity of the audit hash chain",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "server",
								Aliases: []string{"s"},
								Value:   "http://127.0.0.1:8080",
								Usage:   "Artifact server URL to query",
							},
							&cli.StringFlag{
								Name:  "db",
								Usage: "Path to local SQLite release DB file (bypasses HTTP server if provided)",
							},
						},
						Action: runAuditVerify,
					},
					{
						Name:  "list",
						Usage: "List recorded audit log events",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "server",
								Aliases: []string{"s"},
								Value:   "http://127.0.0.1:8080",
								Usage:   "Artifact server URL to query",
							},
							&cli.StringFlag{
								Name:  "db",
								Usage: "Path to local SQLite release DB file",
							},
							&cli.IntFlag{
								Name:    "limit",
								Aliases: []string{"n"},
								Value:   20,
								Usage:   "Maximum number of audit events to display",
							},
						},
						Action: runAuditList,
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

func runPackageResolve(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("please specify at least one root package to resolve")
	}
	targets := c.Args().Slice()

	aptPath := c.String("apt")
	rpmPath := c.String("rpm")

	if aptPath == "" && rpmPath == "" {
		return fmt.Errorf("please specify repository index via --apt or --rpm")
	}

	if aptPath != "" {
		cleanApt := filepath.Clean(aptPath)
		fmt.Printf("==> Reading APT index: %s\n", cleanApt)
		data, err := os.ReadFile(cleanApt)
		if err != nil {
			return fmt.Errorf("failed to read APT index file: %w", err)
		}
		repo, err := pkgengine.ParsePackagesIndex(data)
		if err != nil {
			return fmt.Errorf("failed to parse APT Packages index: %w", err)
		}
		fmt.Printf("==> Loaded %d packages from index. Resolving dependencies for: %v\n",
			len(repo.AllPackages), targets)

		res, err := repo.ResolveDependencies(targets)
		if err != nil {
			return fmt.Errorf("❌ Resolution failed: %w", err)
		}

		fmt.Printf("✅ Successfully resolved %d package(s) (total download size: %.2f MB):\n",
			len(res.Resolved), float64(res.TotalSize)/(1024*1024))
		for _, p := range res.Resolved {
			fmt.Printf("  • %-24s %-20s (%s)\n", p.Name, p.Version, p.Filename)
		}
		return nil
	}

	if rpmPath != "" {
		cleanRPM := filepath.Clean(rpmPath)
		fmt.Printf("==> Reading RPM index: %s\n", cleanRPM)
		data, err := os.ReadFile(cleanRPM)
		if err != nil {
			return fmt.Errorf("failed to read RPM index file: %w", err)
		}
		repo, err := pkgengine.ParseRPMPrimary(data)
		if err != nil {
			return fmt.Errorf("failed to parse RPM primary index: %w", err)
		}
		fmt.Printf("==> Loaded %d packages from index. Resolving dependencies for: %v\n",
			len(repo.AllPackages), targets)

		res, err := repo.ResolveDependencies(targets)
		if err != nil {
			return fmt.Errorf("❌ Resolution failed: %w", err)
		}

		fmt.Printf("✅ Successfully resolved %d package(s) (total download size: %.2f MB):\n",
			len(res.Resolved), float64(res.TotalSize)/(1024*1024))
		for _, p := range res.Resolved {
			fmt.Printf("  • %-24s %-20s (%s)\n", p.Name, p.Version, p.Filename)
		}
		return nil
	}

	return nil
}

func runBundleExport(c *cli.Context) error {
	manifestPath := c.String("manifest")
	sourceDir := c.String("source")
	outputPath := c.String("output")
	reportPath := c.String("report")
	keyPath := c.String("key")
	force := c.Bool("force")

	fmt.Printf("==> Evaluating and building offline deployment bundle...\n")
	fmt.Printf("  Manifest: %s\n", manifestPath)
	fmt.Printf("  Source:   %s\n", sourceDir)

	var privKey []byte
	if keyPath != "" {
		var err error
		privKey, err = bundle.LoadPrivateKeyFromFile(keyPath)
		if err != nil {
			return fmt.Errorf("failed to load private key %s: %w", keyPath, err)
		}
	}

	deltaFrom := c.String("delta-from")
	if deltaFrom != "" {
		fmt.Printf("==> Building DELTA deployment bundle against base: %s\n", deltaFrom)
		if outputPath == "" {
			outputPath = "bundle-delta.tar.gz"
		}
		diffReport, err := delta.BuildDeltaBundle(delta.BuildDeltaOptions{
			BaseBundlePath: deltaFrom,
			ManifestPath:   manifestPath,
			SourceDir:      sourceDir,
			OutputFile:     outputPath,
			ReportPath:     reportPath,
			PrivateKey:     privKey,
		})
		if err != nil {
			return fmt.Errorf("failed building delta bundle: %w", err)
		}
		baseChecksumPrefix := diffReport.BaseChecksum
		if len(baseChecksumPrefix) > 12 {
			baseChecksumPrefix = baseChecksumPrefix[:12]
		}
		fmt.Printf("✅ Successfully built and signed DELTA bundle!\n")
		fmt.Printf("  • Base Release:     %s (checksum: %s...)\n", diffReport.BaseRelease, baseChecksumPrefix)
		fmt.Printf("  • Target Release:   %s\n", diffReport.TargetRelease)
		fmt.Printf("  • Output Archive:   %s (%d bytes)\n", outputPath, diffReport.DeltaSizeBytes)
		fmt.Printf("  • Size Reduction:   %.2f%% (from %d bytes)\n", diffReport.ReductionPercent, diffReport.FullSizeBytes)
		fmt.Printf("  • Files Packaged:   %d added, %d modified (omitted %d unchanged)\n",
			diffReport.AddedCount, diffReport.ModifiedCount, diffReport.UnchangedCount)
		return nil
	}

	opts := bundle.BuildOptions{
		ManifestPath:       manifestPath,
		SourceDir:          sourceDir,
		OutputFile:         outputPath,
		ReportOutputFile:   reportPath,
		PrivateKey:         privKey,
		AllowWarningStatus: force,
	}

	evalRes, err := bundle.BuildBundle(opts)
	if err != nil {
		if evalRes != nil {
			fmt.Printf("❌ Pre-export evaluation: status=%s\n", evalRes.Status)
			for _, m := range evalRes.Missing {
				fmt.Printf("  • Missing %s: %s (%s)\n", m.Type, m.Name, m.Reason)
			}
			for _, r := range evalRes.Risks {
				fmt.Printf("  • Risk [%s] %s: %s\n", r.Severity, r.ID, r.Description)
			}
		}
		return fmt.Errorf("bundle build failed: %w", err)
	}

	fmt.Printf("✅ Pre-export evaluation passed (status=%s)\n", evalRes.Status)
	fmt.Printf("✅ Successfully built and signed deployment bundle archive!\n")
	if reportPath != "" {
		fmt.Printf("  Evaluation Report: %s\n", reportPath)
	}
	return nil
}

func runBundleDiff(c *cli.Context) error {
	if c.NArg() < 2 {
		return fmt.Errorf("usage: ytagarasu bundle diff <base-bundle.tar.gz> <target-bundle.tar.gz>")
	}
	baseBundle := c.Args().Get(0)
	targetBundle := c.Args().Get(1)

	fmt.Printf("==> Computing differential metrics between:\n")
	fmt.Printf("  Base:   %s\n", baseBundle)
	fmt.Printf("  Target: %s\n\n", targetBundle)

	report, err := delta.ComputeDiff(baseBundle, targetBundle)
	if err != nil {
		return fmt.Errorf("diff computation failed: %w", err)
	}

	fmt.Printf("Differential Summary (%s -> %s):\n", report.BaseRelease, report.TargetRelease)
	fmt.Printf("  • Added files:     %d\n", report.AddedCount)
	fmt.Printf("  • Modified files:  %d\n", report.ModifiedCount)
	fmt.Printf("  • Deleted files:   %d\n", report.DeletedCount)
	fmt.Printf("  • Unchanged files: %d\n", report.UnchangedCount)
	fmt.Printf("  • Full size:       %d bytes\n", report.FullSizeBytes)
	fmt.Printf("  • Delta est. size: %d bytes\n", report.DeltaSizeBytes)
	fmt.Printf("  • Transfer saving: %.2f%%\n\n", report.ReductionPercent)

	fmt.Printf("%-10s | %-40s | %-12s | %s\n", "CHANGE", "FILE PATH", "SIZE", "HASH")
	fmt.Println("-----------+------------------------------------------+--------------+------------------")
	for _, f := range report.Files {
		h := f.NewHash
		if h == "" {
			h = f.OldHash
		}
		if len(h) > 12 {
			h = h[:12]
		}
		fmt.Printf("%-10s | %-40s | %-12d | %s\n", f.ChangeType, f.Path, f.NewSize, h)
	}

	return nil
}

func runBundleVerify(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("please specify bundle archive file (.tar.gz)")
	}
	bundlePath := c.Args().First()
	keyPath := c.String("key")

	fmt.Printf("==> Verifying deployment bundle: %s\n", bundlePath)

	var pubKey []byte
	if keyPath != "" {
		var err error
		pubKey, err = bundle.LoadPublicKeyFromFile(keyPath)
		if err != nil {
			return fmt.Errorf("failed to load public key %s: %w", keyPath, err)
		}
	}

	res, err := bundle.VerifyBundle(bundlePath, pubKey)
	if err != nil {
		return fmt.Errorf("bundle verification failed: %w", err)
	}

	if !res.Valid {
		fmt.Printf("❌ Bundle verification FAILED (%d error(s)):\n", len(res.Errors))
		for _, e := range res.Errors {
			fmt.Printf("  • %s\n", e)
		}
		return fmt.Errorf("bundle is invalid or tampered")
	}

	fmt.Printf("✅ Bundle is VALID and cryptographically verified!\n")
	fmt.Printf("  • Release:         %s (v%s)\n", res.Manifest.Release, res.Manifest.BundleVersion)
	fmt.Printf("  • Signature:       Valid (Ed25519)\n")
	fmt.Printf("  • Checksums:       Valid (%d files verified)\n", res.FileCount)
	return nil
}

func runBundleKeygen(c *cli.Context) error {
	dir := c.String("dir")
	cleanDir := filepath.Clean(dir)

	pub, priv, err := bundle.GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("keygen failed: %w", err)
	}

	privPath := filepath.Join(cleanDir, "private.key")
	pubPath := filepath.Join(cleanDir, "public.key")

	if err := bundle.SaveKeyToFile(privPath, priv, 0600); err != nil {
		return fmt.Errorf("failed to save private key: %w", err)
	}
	if err := bundle.SaveKeyToFile(pubPath, pub, 0640); err != nil {
		return fmt.Errorf("failed to save public key: %w", err)
	}

	fmt.Printf("✅ Generated new Ed25519 signing keypair:\n")
	fmt.Printf("  Private Key: %s (permissions: 0600)\n", privPath)
	fmt.Printf("  Public Key:  %s\n", pubPath)
	return nil
}

func runAuditVerify(c *cli.Context) error {
	dbPath := c.String("db")
	if dbPath != "" {
		cleanDB := filepath.Clean(dbPath)
		db, err := store.NewDB(fmt.Sprintf("file:%s", cleanDB))
		if err != nil {
			return fmt.Errorf("failed opening database: %w", err)
		}
		defer func() { _ = db.Close() }()

		report, err := db.VerifyAuditTrail(context.Background())
		if err != nil {
			return fmt.Errorf("failed verifying audit trail: %w", err)
		}
		printAuditReport(report)
		if !report.Valid {
			return errors.New("audit trail verification failed")
		}
		return nil
	}

	serverURL := strings.TrimRight(c.String("server"), "/")
	verifyURL := serverURL + "/api/v1/audit/verify"
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, verifyURL, nil)
	if err != nil {
		return fmt.Errorf("failed creating verify request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req) //nolint:gosec // user CLI command targeting configured server
	if err != nil {
		return fmt.Errorf("failed contacting artifact server: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var report audit.VerificationReport
	if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
		return fmt.Errorf("failed decoding server response: %w", err)
	}

	printAuditReport(&report)
	if !report.Valid {
		return errors.New("audit trail verification failed")
	}
	return nil
}

func printAuditReport(report *audit.VerificationReport) {
	if report.Valid {
		fmt.Printf("✅ Audit trail is VALID and tamper-free!\n")
		fmt.Printf("  • Total records:    %d\n", report.TotalRecords)
		if report.TotalRecords > 0 {
			fmt.Printf("  • Last sequence:    %d\n", report.LastSequence)
			fmt.Printf("  • Last record hash: %s\n", report.LastRecordHash)
		}
	} else {
		fmt.Printf("❌ Audit trail verification FAILED (%d error(s)):\n", len(report.Errors))
		for _, e := range report.Errors {
			fmt.Printf("  • %s\n", e)
		}
	}
}

func runAuditList(c *cli.Context) error {
	limit := c.Int("limit")
	dbPath := c.String("db")
	if dbPath != "" {
		cleanDB := filepath.Clean(dbPath)
		db, err := store.NewDB(fmt.Sprintf("file:%s", cleanDB))
		if err != nil {
			return fmt.Errorf("failed opening database: %w", err)
		}
		defer func() { _ = db.Close() }()

		records, err := db.ListAuditLogs(context.Background(), limit, 0)
		if err != nil {
			return fmt.Errorf("failed listing audit records: %w", err)
		}
		printAuditRecords(records)
		return nil
	}

	serverURL := strings.TrimRight(c.String("server"), "/")
	listURL := fmt.Sprintf("%s/api/v1/audit/records?limit=%d", serverURL, limit)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, listURL, nil)
	if err != nil {
		return fmt.Errorf("failed creating list request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req) //nolint:gosec // user CLI command targeting configured server
	if err != nil {
		return fmt.Errorf("failed contacting artifact server: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var records []audit.Record
	if err := json.NewDecoder(resp.Body).Decode(&records); err != nil {
		return fmt.Errorf("failed decoding server response: %w", err)
	}

	printAuditRecords(records)
	return nil
}

func printAuditRecords(records []audit.Record) {
	if len(records) == 0 {
		fmt.Println("No audit records found.")
		return
	}

	fmt.Printf("%-6s | %-20s | %-20s | %-16s | %-12s | %s\n", "SEQ", "TIMESTAMP", "EVENT TYPE", "ENTITY", "ACTOR", "HASH (PREFIX)")
	fmt.Println("-------+----------------------+----------------------+------------------+--------------+--------------")
	for _, r := range records {
		hashPrefix := r.RecordHash
		if len(hashPrefix) > 12 {
			hashPrefix = hashPrefix[:12]
		}
		fmt.Printf("%-6d | %-20s | %-20s | %-16s | %-12s | %s\n",
			r.Sequence,
			r.Timestamp.Format("2006-01-02 15:04:05"),
			r.EventType,
			r.EntityID,
			r.Actor,
			hashPrefix,
		)
	}
}
