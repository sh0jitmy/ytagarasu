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

package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sh0jitmy/ytagarasu/internal/manifest"
)

// CommandRunner abstracts shell execution for testing.
type CommandRunner interface {
	Run(ctx context.Context, env []string, name string, args ...string) ([]byte, error)
}

// DefaultRunner executes commands on the OS.
type DefaultRunner struct{}

// Run executes a command with specified environment variables.
func (r *DefaultRunner) Run(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // Package manager command execution is intended
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	return cmd.CombinedOutput()
}

// PackageManager abstracts OS package installation.
type PackageManager interface {
	UpdateRepositories(ctx context.Context) error
	InstallPackages(ctx context.Context, pkgs []manifest.PackageItem) error
}

// AptPackageManager manages Debian/Ubuntu APT packages non-interactively.
type AptPackageManager struct {
	runner CommandRunner
}

// NewAptPackageManager constructs an AptPackageManager.
func NewAptPackageManager(runner CommandRunner) *AptPackageManager {
	if runner == nil {
		runner = &DefaultRunner{}
	}
	return &AptPackageManager{runner: runner}
}

// UpdateRepositories runs `apt-get update`.
func (a *AptPackageManager) UpdateRepositories(ctx context.Context) error {
	env := []string{"DEBIAN_FRONTEND=noninteractive"}
	out, err := a.runner.Run(ctx, env, "apt-get", "update", "-qq")
	if err != nil {
		return fmt.Errorf("apt-get update failed: %w (output: %s)", err, string(out))
	}
	return nil
}

// InstallPackages runs `apt-get install -y --no-install-recommends <packages...>`.
func (a *AptPackageManager) InstallPackages(ctx context.Context, pkgs []manifest.PackageItem) error {
	if len(pkgs) == 0 {
		return nil
	}

	pkgNames := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		if p.Version != "" {
			pkgNames = append(pkgNames, fmt.Sprintf("%s=%s", name, p.Version))
		} else {
			pkgNames = append(pkgNames, name)
		}
	}

	if len(pkgNames) == 0 {
		return nil
	}

	args := append([]string{"install", "-y", "--no-install-recommends", "--allow-unauthenticated"}, pkgNames...)
	env := []string{"DEBIAN_FRONTEND=noninteractive"}

	out, err := a.runner.Run(ctx, env, "apt-get", args...)
	if err != nil {
		return fmt.Errorf("apt-get install failed: %w (output: %s)", err, string(out))
	}
	return nil
}

// RpmPackageManager manages RedHat/CentOS/Rocky DNF packages.
type RpmPackageManager struct {
	runner CommandRunner
}

// NewRpmPackageManager constructs an RpmPackageManager.
func NewRpmPackageManager(runner CommandRunner) *RpmPackageManager {
	if runner == nil {
		runner = &DefaultRunner{}
	}
	return &RpmPackageManager{runner: runner}
}

// UpdateRepositories runs `dnf makecache`.
func (r *RpmPackageManager) UpdateRepositories(ctx context.Context) error {
	out, err := r.runner.Run(ctx, nil, "dnf", "makecache", "-y", "-q")
	if err != nil {
		return fmt.Errorf("dnf makecache failed: %w (output: %s)", err, string(out))
	}
	return nil
}

// InstallPackages runs `dnf install -y <packages...>`.
func (r *RpmPackageManager) InstallPackages(ctx context.Context, pkgs []manifest.PackageItem) error {
	if len(pkgs) == 0 {
		return nil
	}

	pkgNames := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		if p.Version != "" {
			pkgNames = append(pkgNames, fmt.Sprintf("%s-%s", name, p.Version))
		} else {
			pkgNames = append(pkgNames, name)
		}
	}

	if len(pkgNames) == 0 {
		return nil
	}

	args := append([]string{"install", "-y", "--nogpgcheck"}, pkgNames...)
	out, err := r.runner.Run(ctx, nil, "dnf", args...)
	if err != nil {
		return fmt.Errorf("dnf install failed: %w (output: %s)", err, string(out))
	}
	return nil
}
