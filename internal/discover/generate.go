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

package discover

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"gopkg.in/yaml.v3"
)

// GenerateOptions defines parameters for generating manifest.yaml from a plan.
type GenerateOptions struct {
	PlanPath       string
	OutputPath     string
	ArtifactsDir   string
	ConfigsDir     string
	IngestOverride bool
	Sysroot        string
}

// Built-in secret patterns for sanitization (SIRT Built-in Safety Valve)
var (
	privateKeyRegex = regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`)
	secretEnvRegex  = regexp.MustCompile(`(?i)([A-Za-z0-9_]*(?:SECRET|PASSWORD|PASSWD|TOKEN|API_KEY|PRIVATE_KEY)[A-Za-z0-9_]*)=([^\s\n]+)`)
)

// LoadPlan reads and unmarshals a DiscoveryPlan from disk.
func LoadPlan(path string) (*DiscoveryPlan, error) {
	data, err := os.ReadFile(filepath.Clean(path)) //nolint:gosec // user specified plan file
	if err != nil {
		return nil, fmt.Errorf("failed to read discovery plan file: %w", err)
	}

	var plan DiscoveryPlan
	if err := yaml.Unmarshal(data, &plan); err != nil {
		return nil, fmt.Errorf("failed to parse discovery plan YAML: %w", err)
	}

	return &plan, nil
}

// GenerateManifest converts an approved DiscoveryPlan into a production manifest.Manifest.
func GenerateManifest(opts GenerateOptions) (*manifest.Manifest, error) {
	plan, err := LoadPlan(opts.PlanPath)
	if err != nil {
		return nil, err
	}

	artifactsDir := opts.ArtifactsDir
	if artifactsDir == "" {
		artifactsDir = "artifacts"
	}
	configsDir := opts.ConfigsDir
	if configsDir == "" {
		configsDir = "configs"
	}
	sysroot := opts.Sysroot
	if sysroot == "" {
		sysroot = "/"
	}

	now := time.Now().UTC()
	m := &manifest.Manifest{
		Version:       "1.0",
		BundleVersion: now.Format("2006.01.02.1"),
		Release:       "v1.0.0",
		CreatedAt:     now,
		Targets: []manifest.Target{
			{
				OS:      runtime.GOOS,
				Release: "default",
				Arch:    runtime.GOARCH,
			},
		},
		Applications: []manifest.Application{},
		Configs:      []manifest.Config{},
		Services:     []manifest.Service{},
		HealthChecks: []manifest.HealthCheck{},
		Rollback: manifest.RollbackPolicy{
			AutoOnFailure: true,
			Strategy:      manifest.RollbackImmediate,
		},
	}

	for _, svc := range plan.Targets.Services {
		appName := strings.TrimSuffix(svc.Name, ".service")
		selector := manifest.Selector{Roles: []string{appName}}

		shouldIngest := svc.Ingest || opts.IngestOverride
		artifactPath := ""

		if shouldIngest {
			// Mode A: Local Ingestion (Copy physical binary into artifacts CAS)
			if err := os.MkdirAll(filepath.Clean(artifactsDir), 0750); err != nil {
				return nil, fmt.Errorf("failed to create artifacts directory: %w", err)
			}
			destArtifact := filepath.Join(artifactsDir, appName)
			srcBinary := filepath.Join(sysroot, strings.TrimPrefix(svc.Binary, "/"))

			if err := copyFile(srcBinary, destArtifact, 0755); err != nil {
				// If copy fails (e.g. file missing), fallback to hint
				artifactPath = fmt.Sprintf("artifacts/%s # NOTE: Local binary not found at %s", appName, svc.Binary)
			} else {
				artifactPath = filepath.Join(artifactsDir, appName)
			}
		} else {
			// Mode B: TODO Hint Mode (GitOps Modernization)
			shaHint := "unknown"
			var sizeHint int64
			if svc.BinaryInfo != nil {
				shaHint = svc.BinaryInfo.SHA256
				sizeHint = svc.BinaryInfo.Size
			}
			artifactPath = fmt.Sprintf("# TODO: CI/CD配布URLを記載 (SHA256: %s, Size: %d bytes)", shaHint, sizeHint)
		}

		app := manifest.Application{
			Name:        appName,
			Type:        manifest.AppTypeGolang,
			Destination: svc.Binary,
			Artifact:    artifactPath,
			Permissions: "0755",
			Selector:    selector,
		}
		m.Applications = append(m.Applications, app)

		// Process configuration files
		for i, confPath := range svc.Configs {
			realConfPath := filepath.Join(sysroot, strings.TrimPrefix(confPath, "/"))
			content, err := os.ReadFile(filepath.Clean(realConfPath)) //nolint:gosec // configuration file path
			if err == nil {
				sanitized := sanitizeSecrets(string(content))
				if err := os.MkdirAll(filepath.Clean(configsDir), 0750); err == nil {
					tmplName := fmt.Sprintf("%s-%s.tmpl", appName, filepath.Base(confPath))
					tmplPath := filepath.Join(configsDir, tmplName)
					_ = os.WriteFile(filepath.Clean(tmplPath), []byte(sanitized), 0600) //nolint:gosec // config template destination within designated configsDir

					m.Configs = append(m.Configs, manifest.Config{
						Template:    tmplPath,
						Destination: confPath,
						Permissions: "0644",
						Selector:    selector,
					})
				}
			} else {
				// Record config entry even if template generation failed
				m.Configs = append(m.Configs, manifest.Config{
					Template:    fmt.Sprintf("configs/%s-conf%d.tmpl", appName, i+1),
					Destination: confPath,
					Permissions: "0644",
					Selector:    selector,
				})
			}
		}

		// Register systemd service management
		m.Services = append(m.Services, manifest.Service{
			Name:     svc.Name,
			Action:   manifest.ServiceActionRestart,
			Selector: selector,
		})

		// Register health check
		m.HealthChecks = append(m.HealthChecks, manifest.HealthCheck{
			Type:            manifest.HealthCheckCommand,
			Command:         fmt.Sprintf("systemctl is-active --quiet %s", svc.Name),
			IntervalSeconds: 2,
			MaxRetries:      3,
			Selector:        selector,
		})
	}

	return m, nil
}

// sanitizeSecrets applies SIRT-grade masking to credentials and private keys.
func sanitizeSecrets(input string) string {
	out := privateKeyRegex.ReplaceAllString(input, "-----BEGIN PRIVATE KEY-----\n<REDACTED_SECRET_PRIVATE_KEY>\n-----END PRIVATE KEY-----")
	out = secretEnvRegex.ReplaceAllString(out, `$1="<REDACTED_SECRET>"`)
	return out
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(filepath.Clean(src)) //nolint:gosec // source binary path
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(filepath.Clean(dst), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode) //nolint:gosec // dest binary
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// SaveManifest outputs the generated Manifest as clean YAML.
func SaveManifest(m *manifest.Manifest, outputPath string) error {
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	header := "# manifest.yaml\n# Generated by ytagarasu discover generate from approved whitelist\n\n"
	fullData := append([]byte(header), data...)

	if err := os.WriteFile(filepath.Clean(outputPath), fullData, 0600); err != nil {
		return fmt.Errorf("failed to write manifest file: %w", err)
	}

	return nil
}
