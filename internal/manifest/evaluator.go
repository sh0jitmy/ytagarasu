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

package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// EvaluateBundle evaluates a manifest and artifact bundle directory for a given target platform,
// generating a detailed PreparationResult with risk assessment and readiness status.
func EvaluateBundle(m *Manifest, bundleRoot string, target Target) (*PreparationResult, error) {
	if m == nil {
		return nil, fmt.Errorf("manifest cannot be nil")
	}

	res := &PreparationResult{
		Version:       "1.0",
		EvaluatedAt:   time.Now().UTC().Truncate(time.Second),
		Status:        StatusReady,
		Release:       m.Release,
		Target:        target,
		RequiredFiles: make([]RequiredFile, 0),
		Missing:       make([]MissingItem, 0),
		Risks:         make([]Risk, 0),
	}

	if len(m.Applications) > 0 {
		res.Application = m.Applications[0].Name
	} else {
		res.Application = "unknown"
	}

	var totalBundleSize int64
	var totalInstalledSize int64

	// 1. Verify Application Artifacts
	for _, app := range m.Applications {
		switch app.Type {
		case AppTypeGolang:
			if app.Artifact != "" {
				fullPath := filepath.Join(bundleRoot, app.Artifact)
				rf, err := checkFile(app.Artifact, fullPath)
				res.RequiredFiles = append(res.RequiredFiles, rf)
				if err != nil || !rf.Exists {
					res.Missing = append(res.Missing, MissingItem{
						Name:   app.Artifact,
						Type:   "artifact",
						Reason: fmt.Sprintf("compiled binary for golang application '%s' not found", app.Name),
					})
					res.Risks = append(res.Risks, Risk{
						ID:          fmt.Sprintf("RISK-BIN-%s", strings.ToUpper(app.Name)),
						Severity:    SeverityCritical,
						Category:    "artifact",
						Description: fmt.Sprintf("required executable artifact '%s' is missing", app.Artifact),
						Mitigation:  "compile and place the static binary in artifacts directory before building the bundle",
					})
				} else {
					fi, _ := os.Stat(fullPath)
					if fi != nil {
						totalBundleSize += fi.Size()
						totalInstalledSize += fi.Size()
					}
				}
			}
		case AppTypePython:
			if app.RequirementsFile != "" {
				fullPath := filepath.Join(bundleRoot, app.RequirementsFile)
				rf, _ := checkFile(app.RequirementsFile, fullPath)
				res.RequiredFiles = append(res.RequiredFiles, rf)
				if !rf.Exists {
					res.Missing = append(res.Missing, MissingItem{
						Name:   app.RequirementsFile,
						Type:   "requirements",
						Reason: "Python requirements lockfile not found",
					})
				}
			}
		case AppTypeReact:
			if app.DistDir != "" {
				fullPath := filepath.Join(bundleRoot, app.DistDir)
				exists := fileExists(fullPath) || dirExists(fullPath)
				res.RequiredFiles = append(res.RequiredFiles, RequiredFile{
					Path:   app.DistDir,
					Exists: exists,
				})
				if !exists {
					res.Missing = append(res.Missing, MissingItem{
						Name:   app.DistDir,
						Type:   "dist",
						Reason: "React static build dist directory not found",
					})
				}
			}
		case AppTypeHTMX:
			// HTMX files may be embedded or standalone
		}
	}

	// 2. Verify Config Templates
	for _, cfg := range m.Configs {
		if cfg.Template != "" {
			fullPath := filepath.Join(bundleRoot, cfg.Template)
			rf, err := checkFile(cfg.Template, fullPath)
			res.RequiredFiles = append(res.RequiredFiles, rf)
			if err != nil || !rf.Exists {
				res.Missing = append(res.Missing, MissingItem{
					Name:   cfg.Template,
					Type:   "config_template",
					Reason: fmt.Sprintf("configuration template '%s' not found", cfg.Template),
				})
			}
		}
	}

	// 3. Status Decision Logic
	hasCritical := false
	for _, r := range res.Risks {
		if r.Severity == SeverityCritical {
			hasCritical = true
			break
		}
	}

	if len(res.Missing) > 0 || hasCritical {
		res.Status = StatusBlocked
	} else if len(res.Risks) > 0 {
		res.Status = StatusWarning
	} else {
		res.Status = StatusReady
	}

	// Resource estimation
	if totalInstalledSize < 50*1024*1024 {
		totalInstalledSize = 120 * 1024 * 1024 // standard fallback estimate ~120MB
	}
	if totalBundleSize < 20*1024*1024 {
		totalBundleSize = 45 * 1024 * 1024 // standard fallback estimate ~45MB
	}
	res.ResourceEstimates = ResourceEstimates{
		BundleSizeBytes:       totalBundleSize,
		InstalledSizeBytes:    totalInstalledSize,
		RequiredDiskFreeBytes: totalInstalledSize * 3, // Recommend 3x for atomic extraction and backup
	}

	return res, nil
}

// SavePreparationResult encodes and writes the preparation result to a YAML file.
func SavePreparationResult(res *PreparationResult, outputPath string) error {
	data, err := yaml.Marshal(res)
	if err != nil {
		return fmt.Errorf("failed to marshal preparation result: %w", err)
	}
	return os.WriteFile(outputPath, data, 0640) //nolint:gosec
}

func checkFile(relPath, fullPath string) (RequiredFile, error) {
	fi, err := os.Stat(fullPath)
	if err != nil {
		return RequiredFile{Path: relPath, Exists: false}, err
	}
	if fi.IsDir() {
		return RequiredFile{Path: relPath, Exists: true}, nil
	}

	f, err := os.Open(filepath.Clean(fullPath))
	if err != nil {
		return RequiredFile{Path: relPath, Exists: true}, nil
	}
	defer func() {
		_ = f.Close()
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return RequiredFile{Path: relPath, Exists: true}, nil
	}

	return RequiredFile{
		Path:   relPath,
		Exists: true,
		SHA256: hex.EncodeToString(h.Sum(nil)),
	}, nil
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	if err != nil {
		return false
	}
	return fi.IsDir()
}
