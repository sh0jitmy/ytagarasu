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

// PipPackageManager manages offline Python packages via pre-downloaded wheels.
type PipPackageManager struct {
	runner    CommandRunner
	pythonBin string
	wheelsDir string
}

// NewPipPackageManager constructs a PipPackageManager.
func NewPipPackageManager(runner CommandRunner, pythonBin, wheelsDir string) *PipPackageManager {
	if runner == nil {
		runner = &DefaultRunner{}
	}
	if pythonBin == "" {
		pythonBin = "python3"
	}
	return &PipPackageManager{
		runner:    runner,
		pythonBin: pythonBin,
		wheelsDir: wheelsDir,
	}
}

// UpdateRepositories is a no-op for air-gapped pip.
func (p *PipPackageManager) UpdateRepositories(ctx context.Context) error {
	return nil
}

// InstallPackages runs `python3 -m pip install --no-index [--find-links <dir>] <packages...>`.
func (p *PipPackageManager) InstallPackages(ctx context.Context, pkgs []manifest.PackageItem) error {
	if len(pkgs) == 0 {
		return nil
	}

	args := []string{"-m", "pip", "install", "--no-index"}
	if p.wheelsDir != "" {
		args = append(args, "--find-links", p.wheelsDir)
	}

	for _, pkg := range pkgs {
		name := strings.TrimSpace(pkg.Name)
		if name == "" {
			continue
		}
		if pkg.Version != "" {
			args = append(args, fmt.Sprintf("%s==%s", name, pkg.Version))
		} else {
			args = append(args, name)
		}
	}

	out, err := p.runner.Run(ctx, nil, p.pythonBin, args...)
	if err != nil {
		return fmt.Errorf("pip install failed: %w (output: %s)", err, string(out))
	}
	return nil
}

// DockerPackageManager manages offline Docker container images loaded from archives.
type DockerPackageManager struct {
	runner CommandRunner
}

// NewDockerPackageManager constructs a DockerPackageManager.
func NewDockerPackageManager(runner CommandRunner) *DockerPackageManager {
	if runner == nil {
		runner = &DefaultRunner{}
	}
	return &DockerPackageManager{runner: runner}
}

// UpdateRepositories is a no-op for offline Docker images.
func (d *DockerPackageManager) UpdateRepositories(ctx context.Context) error {
	return nil
}

// InstallPackages loads offline container archives via `docker load -i <path>`.
func (d *DockerPackageManager) InstallPackages(ctx context.Context, pkgs []manifest.PackageItem) error {
	if len(pkgs) == 0 {
		return nil
	}

	for _, pkg := range pkgs {
		archivePath := strings.TrimSpace(pkg.Name)
		if archivePath == "" {
			continue
		}
		out, err := d.runner.Run(ctx, nil, "docker", "load", "-i", archivePath)
		if err != nil {
			return fmt.Errorf("docker load failed for '%s': %w (output: %s)", archivePath, err, string(out))
		}
	}
	return nil
}

// DewyPackageManager manages pull-based binary releases using Dewy.
type DewyPackageManager struct {
	runner     CommandRunner
	dewyBin    string
	configPath string
}

// NewDewyPackageManager constructs a DewyPackageManager.
func NewDewyPackageManager(runner CommandRunner, dewyBin string, configPath string) *DewyPackageManager {
	if runner == nil {
		runner = &DefaultRunner{}
	}
	if dewyBin == "" {
		dewyBin = "dewy"
	}
	return &DewyPackageManager{
		runner:     runner,
		dewyBin:    dewyBin,
		configPath: configPath,
	}
}

// UpdateRepositories is a no-op for Dewy pull manager.
func (d *DewyPackageManager) UpdateRepositories(ctx context.Context) error {
	return nil
}

// InstallPackages executes pull-based deployment via `dewy pull`.
func (d *DewyPackageManager) InstallPackages(ctx context.Context, pkgs []manifest.PackageItem) error {
	if len(pkgs) == 0 {
		return nil
	}

	for _, pkg := range pkgs {
		args := []string{"pull"}
		if d.configPath != "" {
			args = append(args, "--config", d.configPath)
		}
		if pkg.Name != "" {
			args = append(args, "--artifact", pkg.Name)
		}
		if pkg.Version != "" {
			args = append(args, "--version", pkg.Version)
		}

		out, err := d.runner.Run(ctx, nil, d.dewyBin, args...)
		if err != nil {
			return fmt.Errorf("dewy pull failed for '%s': %w (output: %s)", pkg.Name, err, string(out))
		}
	}
	return nil
}

// MultiPackageManager dispatches package installations to manager-specific implementations.
type MultiPackageManager struct {
	managers map[string]PackageManager
	fallback PackageManager
}

// NewMultiPackageManager constructs a MultiPackageManager.
func NewMultiPackageManager(fallback PackageManager) *MultiPackageManager {
	return &MultiPackageManager{
		managers: make(map[string]PackageManager),
		fallback: fallback,
	}
}

// Register registers a manager implementation for a given manager name (e.g. "apt", "dnf", "pip", "docker", "dewy").
func (m *MultiPackageManager) Register(name string, mgr PackageManager) {
	m.managers[strings.ToLower(strings.TrimSpace(name))] = mgr
}

// UpdateRepositories updates all registered repositories.
func (m *MultiPackageManager) UpdateRepositories(ctx context.Context) error {
	for name, mgr := range m.managers {
		if err := mgr.UpdateRepositories(ctx); err != nil {
			return fmt.Errorf("update repositories failed for manager '%s': %w", name, err)
		}
	}
	if m.fallback != nil {
		return m.fallback.UpdateRepositories(ctx)
	}
	return nil
}

// InstallPackages delegates installation using the package item or fallback.
func (m *MultiPackageManager) InstallPackages(ctx context.Context, pkgs []manifest.PackageItem) error {
	if len(pkgs) == 0 {
		return nil
	}
	if m.fallback != nil {
		return m.fallback.InstallPackages(ctx, pkgs)
	}
	return nil
}

// InstallForManager installs packages specifically using the named manager.
func (m *MultiPackageManager) InstallForManager(ctx context.Context, managerName string, pkgs []manifest.PackageItem) error {
	mgr, ok := m.managers[strings.ToLower(strings.TrimSpace(managerName))]
	if !ok {
		if m.fallback != nil {
			return m.fallback.InstallPackages(ctx, pkgs)
		}
		return fmt.Errorf("no package manager registered for '%s'", managerName)
	}
	return mgr.InstallPackages(ctx, pkgs)
}

// NewPackageManager returns a default PackageManager implementation for the given manager type.
func NewPackageManager(managerType string, runner CommandRunner) PackageManager {
	switch strings.ToLower(strings.TrimSpace(managerType)) {
	case "apt", "debian", "ubuntu":
		return NewAptPackageManager(runner)
	case "dnf", "yum", "rpm", "rhel", "rocky", "centos":
		return NewRpmPackageManager(runner)
	case "pip", "python", "wheel":
		return NewPipPackageManager(runner, "python3", "")
	case "docker", "container", "oci":
		return NewDockerPackageManager(runner)
	case "dewy", "s3", "pull":
		return NewDewyPackageManager(runner, "dewy", "")
	default:
		return NewAptPackageManager(runner)
	}
}
