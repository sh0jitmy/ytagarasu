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
	"bufio"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SurveyOptions provides configuration parameters for the survey engine.
type SurveyOptions struct {
	Sysroot          string
	Service          string
	ExcludedServices []string
}

// DefaultExcludedPrefixes lists system-level unit prefixes that are skipped by default.
var DefaultExcludedPrefixes = []string{
	"systemd-", "dbus", "sshd", "ssh.", "cron", "getty@", "rsyslog",
	"network-", "emergency.", "rescue.", "ytagarasu-", "apt-", "unattended-",
	"snapd", "cloud-", "plymouth", "udev", "containerd",
}

// Survey inspects existing server state and returns a draft DiscoveryPlan.
func Survey(opts SurveyOptions) (*DiscoveryPlan, error) {
	sysroot := opts.Sysroot
	if sysroot == "" {
		sysroot = "/"
	}

	hostname, _ := os.Hostname()
	plan := &DiscoveryPlan{
		Version:   "1.0",
		CreatedAt: time.Now().UTC(),
		Hostname:  hostname,
		Targets: PlanTargets{
			Services:   []PlanService{},
			CustomDirs: []PlanCustomDir{},
		},
	}

	searchDirs := []string{
		filepath.Join(sysroot, "etc", "systemd", "system"),
		filepath.Join(sysroot, "lib", "systemd", "system"),
		filepath.Join(sysroot, "usr", "lib", "systemd", "system"),
	}

	seenServices := make(map[string]bool)

	for _, dir := range searchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue // skip directories that don't exist or cannot be read
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".service") {
				continue
			}

			svcName := entry.Name()
			if seenServices[svcName] {
				continue
			}

			// Filter by target service if specified
			if opts.Service != "" {
				if svcName != opts.Service && svcName != opts.Service+".service" {
					continue
				}
			} else {
				// Apply default exclusions
				if isExcluded(svcName, opts.ExcludedServices) {
					continue
				}
			}

			unitPath := filepath.Join(dir, svcName)
			parsedSvc, err := parseServiceFile(unitPath, sysroot)
			if err != nil {
				continue
			}

			if parsedSvc.Binary != "" {
				inspectBinary(sysroot, parsedSvc)
			}

			seenServices[svcName] = true
			plan.Targets.Services = append(plan.Targets.Services, *parsedSvc)
		}
	}

	return plan, nil
}

func isExcluded(svcName string, customExcludes []string) bool {
	for _, custom := range customExcludes {
		if svcName == custom || strings.HasPrefix(svcName, custom) {
			return true
		}
	}
	for _, prefix := range DefaultExcludedPrefixes {
		if strings.HasPrefix(svcName, prefix) {
			return true
		}
	}
	return false
}

// parseServiceFile parses a systemd unit file and extracts relevant metadata.
func parseServiceFile(path, sysroot string) (*PlanService, error) {
	file, err := os.Open(filepath.Clean(path)) //nolint:gosec // systemd unit file on host
	if err != nil {
		return nil, fmt.Errorf("failed to open unit file: %w", err)
	}
	defer func() { _ = file.Close() }()

	svc := &PlanService{
		Name:     filepath.Base(path),
		UnitFile: path,
		Configs:  []string{},
		Ingest:   false, // Default: Mode B (TODO Hint)
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if strings.HasPrefix(line, "Description=") {
			svc.Description = strings.TrimPrefix(line, "Description=")
		} else if strings.HasPrefix(line, "ExecStart=") {
			rawExec := strings.TrimPrefix(line, "ExecStart=")
			// Handle leading minus (-/path/to/bin) which tells systemd to ignore exit code
			rawExec = strings.TrimPrefix(rawExec, "-")
			parts := strings.Fields(rawExec)
			if len(parts) > 0 {
				svc.Binary = parts[0]
			}
		} else if strings.HasPrefix(line, "EnvironmentFile=") {
			rawEnv := strings.TrimPrefix(line, "EnvironmentFile=")
			rawEnv = strings.TrimPrefix(rawEnv, "-")
			fields := strings.Fields(rawEnv)
			svc.Configs = append(svc.Configs, fields...)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading unit file: %w", err)
	}

	return svc, nil
}

// inspectBinary inspects the candidate executable binary and populates BinaryMeta.
func inspectBinary(sysroot string, svc *PlanService) {
	realPath := filepath.Join(sysroot, strings.TrimPrefix(svc.Binary, "/"))
	info, err := os.Stat(realPath)
	if err != nil {
		return // File does not exist or unreadable
	}

	meta := &BinaryMeta{
		Size: info.Size(),
		Type: "Executable",
	}

	// Calculate SHA-256
	if hash, err := calculateSHA256(realPath); err == nil {
		meta.SHA256 = hash
	}

	// Inspect ELF header
	if elfFile, err := elf.Open(filepath.Clean(realPath)); err == nil {
		defer func() { _ = elfFile.Close() }()
		meta.Type = "ELF Native"
		switch elfFile.Machine {
		case elf.EM_X86_64:
			meta.Architecture = "amd64"
		case elf.EM_AARCH64:
			meta.Architecture = "arm64"
		case elf.EM_386:
			meta.Architecture = "386"
		case elf.EM_ARM:
			meta.Architecture = "arm"
		default:
			meta.Architecture = elfFile.Machine.String()
		}

		// Check if it is a Go binary by inspecting Go-specific sections
		for _, s := range elfFile.Sections {
			if s.Name == ".note.go.buildid" || s.Name == ".gopclntab" {
				meta.Type = "ELF Go"
				break
			}
		}
	}

	svc.BinaryInfo = meta
}

func calculateSHA256(path string) (string, error) {
	f, err := os.Open(filepath.Clean(path)) //nolint:gosec // inspected binary path
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// SavePlan writes a DiscoveryPlan to the designated YAML output path with human-friendly comments.
func SavePlan(plan *DiscoveryPlan, outputPath string) error {
	var buf strings.Builder
	buf.WriteString("# discovery-plan.yaml\n")
	buf.WriteString("# Generated by ytagarasu discover survey at " + plan.CreatedAt.Format(time.RFC3339) + "\n")
	buf.WriteString("# \n")
	buf.WriteString("# Instructions (KISS Rule):\n")
	buf.WriteString("# 1. This file is your strict WHITELIST. Only items listed below will be generated.\n")
	buf.WriteString("# 2. To exclude an unwanted service or directory, DELETE the line or comment it out with '#'.\n")
	buf.WriteString("# 3. For binaries with unknown repository origins, set 'ingest: true' to copy the local binary directly.\n")
	buf.WriteString("#\n\n")

	data, err := yaml.Marshal(plan)
	if err != nil {
		return fmt.Errorf("failed to marshal discovery plan: %w", err)
	}
	buf.Write(data)

	if err := os.WriteFile(filepath.Clean(outputPath), []byte(buf.String()), 0600); err != nil {
		return fmt.Errorf("failed to write discovery plan file: %w", err)
	}

	return nil
}
