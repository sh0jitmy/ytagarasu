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

package agent_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/agent"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordedCommand struct {
	env  []string
	name string
	args []string
}

type fakeRunner struct {
	calls []recordedCommand
	err   error
	out   []byte
}

func (f *fakeRunner) Run(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, recordedCommand{
		env:  env,
		name: name,
		args: args,
	})
	return f.out, f.err
}

func TestAptPackageManager(t *testing.T) {
	t.Parallel()

	t.Run("UpdateRepositories without sudo", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewAptPackageManager(runner).WithSudo(false)

		err := mgr.UpdateRepositories(context.Background())
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, []string{"DEBIAN_FRONTEND=noninteractive"}, runner.calls[0].env)
		assert.Equal(t, "apt-get", runner.calls[0].name)
		assert.Equal(t, []string{"update", "-qq", "-o", "Acquire::http::Timeout=10", "-o", "Acquire::Retries=1"}, runner.calls[0].args)
	})

	t.Run("UpdateRepositories with sudo escalation", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewAptPackageManager(runner).WithSudo(true)

		err := mgr.UpdateRepositories(context.Background())
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "sudo", runner.calls[0].name)
		assert.Equal(t, []string{"-n", "apt-get", "update", "-qq", "-o", "Acquire::http::Timeout=10", "-o", "Acquire::Retries=1"}, runner.calls[0].args)
	})

	t.Run("ConfigureRepository", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewAptPackageManager(runner).WithSudo(true)

		err := mgr.ConfigureRepository(context.Background(), "http://10.0.0.1:18090/repos/billing/apt/", true)
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "sudo", runner.calls[0].name)
		assert.Contains(t, runner.calls[0].args[3], "[trusted=yes] http://10.0.0.1:18090/repos/billing/apt/")
		assert.Contains(t, runner.calls[0].args[3], "/etc/apt/sources.list.d/ytagarasu.list")
	})

	t.Run("InstallPackages with versions and local deb", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewAptPackageManager(runner).WithSudo(false)

		tmpDir := t.TempDir()
		debPath := filepath.Join(tmpDir, "app_1.0.0.deb")
		debContent := []byte("fake-deb-binary-payload")
		require.NoError(t, os.WriteFile(debPath, debContent, 0600))
		h := sha256.Sum256(debContent)
		hashHex := hex.EncodeToString(h[:])

		pkgs := []manifest.PackageItem{
			{Name: "curl", Version: "7.88.1"},
			{Name: "git", Version: ""},
			{Name: debPath, SHA256: hashHex},
		}

		err := mgr.InstallPackages(context.Background(), pkgs)
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "apt-get", runner.calls[0].name)
		assert.Equal(t, []string{
			"install", "-y", "--no-install-recommends", "--allow-unauthenticated",
			"-o", "Acquire::http::Timeout=10", "-o", "Acquire::Retries=1",
			"curl=7.88.1", "git", debPath,
		}, runner.calls[0].args)
	})

	t.Run("InstallLocalPackage direct", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewAptPackageManager(runner).WithSudo(false)

		tmpDir := t.TempDir()
		debPath := filepath.Join(tmpDir, "service_1.0.0.deb")
		debContent := []byte("service-deb-bytes")
		require.NoError(t, os.WriteFile(debPath, debContent, 0600))
		h := sha256.Sum256(debContent)
		hashHex := hex.EncodeToString(h[:])

		err := mgr.InstallLocalPackage(context.Background(), debPath, hashHex)
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "apt-get", runner.calls[0].name)
		assert.Equal(t, []string{
			"install", "-y", "--no-install-recommends", "--allow-unauthenticated",
			"-o", "Acquire::http::Timeout=10", "-o", "Acquire::Retries=1",
			debPath,
		}, runner.calls[0].args)
	})

	t.Run("InstallLocalPackage checksum mismatch", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewAptPackageManager(runner).WithSudo(false)

		tmpDir := t.TempDir()
		debPath := filepath.Join(tmpDir, "tampered.deb")
		require.NoError(t, os.WriteFile(debPath, []byte("tampered-content"), 0600))

		err := mgr.InstallLocalPackage(context.Background(), debPath, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
		require.Error(t, err)
		require.ErrorIs(t, err, agent.ErrChecksumMismatch)
	})

	t.Run("InstallLocalPackage file not found or empty", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewAptPackageManager(runner).WithSudo(false)

		tmpDir := t.TempDir()
		missingDeb := filepath.Join(tmpDir, "missing.deb")
		err := mgr.InstallLocalPackage(context.Background(), missingDeb, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "local package file not found")

		emptyDeb := filepath.Join(tmpDir, "empty.deb")
		require.NoError(t, os.WriteFile(emptyDeb, []byte{}, 0600))
		err = mgr.InstallLocalPackage(context.Background(), emptyDeb, "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is empty")
	})

	t.Run("InstallPackages with sudo", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewAptPackageManager(runner).WithSudo(true)

		err := mgr.InstallPackages(context.Background(), []manifest.PackageItem{{Name: "htop"}})
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "sudo", runner.calls[0].name)
		assert.Equal(t, []string{
			"-n", "apt-get", "install", "-y", "--no-install-recommends", "--allow-unauthenticated",
			"-o", "Acquire::http::Timeout=10", "-o", "Acquire::Retries=1", "htop",
		}, runner.calls[0].args)
	})

	t.Run("InstallPackages failure", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{err: errors.New("broken package"), out: []byte("E: Unable to locate package")}
		mgr := agent.NewAptPackageManager(runner).WithSudo(false)

		err := mgr.InstallPackages(context.Background(), []manifest.PackageItem{{Name: "nonexistent"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "apt-get install failed")
		assert.Contains(t, err.Error(), "Unable to locate package")
	})
}

func TestRpmPackageManager(t *testing.T) {
	t.Parallel()

	t.Run("UpdateRepositories without sudo", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewRpmPackageManager(runner).WithSudo(false)

		err := mgr.UpdateRepositories(context.Background())
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "dnf", runner.calls[0].name)
		assert.Equal(t, []string{"makecache", "-y", "-q"}, runner.calls[0].args)
	})

	t.Run("UpdateRepositories with sudo", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewRpmPackageManager(runner).WithSudo(true)

		err := mgr.UpdateRepositories(context.Background())
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "sudo", runner.calls[0].name)
		assert.Equal(t, []string{"-n", "dnf", "makecache", "-y", "-q"}, runner.calls[0].args)
	})

	t.Run("ConfigureRepository", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewRpmPackageManager(runner).WithSudo(true)

		err := mgr.ConfigureRepository(context.Background(), "http://10.0.0.1:18090/repos/billing/rpm/", true)
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "sudo", runner.calls[0].name)
		assert.Contains(t, runner.calls[0].args[3], "baseurl=http://10.0.0.1:18090/repos/billing/rpm/")
		assert.Contains(t, runner.calls[0].args[3], "/etc/yum.repos.d/ytagarasu.repo")
	})

	t.Run("InstallPackages with rpm path", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewRpmPackageManager(runner).WithSudo(false)

		tmpDir := t.TempDir()
		rpmPath := filepath.Join(tmpDir, "custom.rpm")
		rpmContent := []byte("dummy-rpm-payload")
		require.NoError(t, os.WriteFile(rpmPath, rpmContent, 0600))
		h := sha256.Sum256(rpmContent)
		hashHex := hex.EncodeToString(h[:])

		pkgs := []manifest.PackageItem{
			{Name: "nginx", Version: "1.24"},
			{Name: "sqlite", Version: ""},
			{Name: rpmPath, SHA256: hashHex},
		}

		err := mgr.InstallPackages(context.Background(), pkgs)
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "dnf", runner.calls[0].name)
		assert.Equal(t, []string{"install", "-y", "--nogpgcheck", "nginx-1.24", "sqlite", rpmPath}, runner.calls[0].args)
	})

	t.Run("InstallLocalPackage direct", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewRpmPackageManager(runner).WithSudo(false)

		tmpDir := t.TempDir()
		rpmPath := filepath.Join(tmpDir, "service-1.0.0.rpm")
		rpmContent := []byte("service-rpm-bytes")
		require.NoError(t, os.WriteFile(rpmPath, rpmContent, 0600))
		h := sha256.Sum256(rpmContent)
		hashHex := hex.EncodeToString(h[:])

		err := mgr.InstallLocalPackage(context.Background(), rpmPath, hashHex)
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "dnf", runner.calls[0].name)
		assert.Equal(t, []string{"install", "-y", "--nogpgcheck", rpmPath}, runner.calls[0].args)
	})
}

func TestPipPackageManager(t *testing.T) {
	t.Parallel()

	t.Run("InstallPackages with wheelsDir", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewPipPackageManager(runner, "python3", "/opt/wheels")

		pkgs := []manifest.PackageItem{
			{Name: "fastapi", Version: "0.110.0"},
			{Name: "uvicorn", Version: ""},
		}

		err := mgr.InstallPackages(context.Background(), pkgs)
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "python3", runner.calls[0].name)
		assert.Equal(t, []string{
			"-m", "pip", "install", "--no-index",
			"--find-links", "/opt/wheels",
			"fastapi==0.110.0", "uvicorn",
		}, runner.calls[0].args)
	})

	t.Run("UpdateRepositories no-op", func(t *testing.T) {
		t.Parallel()
		mgr := agent.NewPipPackageManager(nil, "", "")
		assert.NoError(t, mgr.UpdateRepositories(context.Background()))
	})
}

func TestDockerPackageManager(t *testing.T) {
	t.Parallel()

	t.Run("InstallPackages loads tar archives", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewDockerPackageManager(runner)

		pkgs := []manifest.PackageItem{
			{Name: "/var/bundles/images/redis-7.tar"},
			{Name: "/var/bundles/images/app.tar"},
		}

		err := mgr.InstallPackages(context.Background(), pkgs)
		require.NoError(t, err)
		require.Len(t, runner.calls, 2)
		assert.Equal(t, "docker", runner.calls[0].name)
		assert.Equal(t, []string{"load", "-i", "/var/bundles/images/redis-7.tar"}, runner.calls[0].args)
		assert.Equal(t, []string{"load", "-i", "/var/bundles/images/app.tar"}, runner.calls[1].args)
	})
}

func TestMultiPackageManagerAndFactory(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{}
	aptMgr := agent.NewAptPackageManager(runner).WithSudo(false)
	rpmMgr := agent.NewRpmPackageManager(runner).WithSudo(false)
	multi := agent.NewMultiPackageManager(aptMgr)
	multi.Register("apt", aptMgr)
	multi.Register("dnf", rpmMgr)
	multi.Register("pip", agent.NewPipPackageManager(runner, "python3", "/wheels"))
	multi.Register("docker", agent.NewDockerPackageManager(runner))

	// ConfigureRepository dispatch
	err := multi.ConfigureRepository(context.Background(), "apt", "http://mirror.lan/apt", true)
	require.NoError(t, err)

	// Dispatch to DNF
	err = multi.InstallForManager(context.Background(), "dnf", []manifest.PackageItem{{Name: "htop"}})
	require.NoError(t, err)
	assert.Equal(t, "dnf", runner.calls[len(runner.calls)-1].name)

	// Dispatch to Pip
	err = multi.InstallForManager(context.Background(), "pip", []manifest.PackageItem{{Name: "requests"}})
	require.NoError(t, err)
	assert.Equal(t, "python3", runner.calls[len(runner.calls)-1].name)

	// Dispatch to Docker
	err = multi.InstallForManager(context.Background(), "docker", []manifest.PackageItem{{Name: "/img/app.tar"}})
	require.NoError(t, err)
	assert.Equal(t, "docker", runner.calls[len(runner.calls)-1].name)

	// Dispatch to unknown fallback
	err = multi.InstallForManager(context.Background(), "unknown", []manifest.PackageItem{{Name: "bash"}})
	require.NoError(t, err)
	assert.Equal(t, "apt-get", runner.calls[len(runner.calls)-1].name)

	// Dispatch DirectInstaller to APT
	tmpDir := t.TempDir()
	localDeb := filepath.Join(tmpDir, "local.deb")
	require.NoError(t, os.WriteFile(localDeb, []byte("pkg-bytes"), 0600))
	hDeb := sha256.Sum256([]byte("pkg-bytes"))
	hDebHex := hex.EncodeToString(hDeb[:])

	err = multi.InstallLocalPackage(context.Background(), "apt", localDeb, hDebHex)
	require.NoError(t, err)
	assert.Equal(t, "apt-get", runner.calls[len(runner.calls)-1].name)

	// Dispatch DirectInstaller to unknown manager fallback
	err = multi.InstallLocalPackage(context.Background(), "nonexistent", localDeb, hDebHex)
	require.NoError(t, err)
	assert.Equal(t, "apt-get", runner.calls[len(runner.calls)-1].name)

	// Test Factory
	assert.NotNil(t, agent.NewPackageManager("apt", runner))
	assert.NotNil(t, agent.NewPackageManager("dnf", runner))
	assert.NotNil(t, agent.NewPackageManager("pip", runner))
	assert.NotNil(t, agent.NewPackageManager("docker", runner))
	assert.NotNil(t, agent.NewPackageManager("dewy", runner))
	assert.NotNil(t, agent.NewPackageManager("custom", runner))
}

func TestDewyPackageManager(t *testing.T) {
	t.Parallel()

	t.Run("InstallPackages with artifact and version", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewDewyPackageManager(runner, "dewy", "/etc/dewy/dewy.env")

		err := mgr.InstallPackages(context.Background(), []manifest.PackageItem{
			{Name: "billing-svc", Version: "v1.1.0"},
		})
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "dewy", runner.calls[0].name)
		assert.Equal(t, []string{"pull", "--config", "/etc/dewy/dewy.env", "--artifact", "billing-svc", "--version", "v1.1.0"}, runner.calls[0].args)

		// Test UpdateRepositories is no-op
		require.NoError(t, mgr.UpdateRepositories(context.Background()))
	})

	t.Run("InstallPackages failure", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{err: errors.New("dewy pull connection refused")}
		mgr := agent.NewDewyPackageManager(runner, "", "")

		err := mgr.InstallPackages(context.Background(), []manifest.PackageItem{
			{Name: "billing-svc"},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dewy pull failed")
	})
}

func TestEmptyPackages(t *testing.T) {
	t.Parallel()
	runner := &fakeRunner{}
	ctx := context.Background()

	apt := agent.NewAptPackageManager(runner).WithSudo(false)
	require.NoError(t, apt.InstallPackages(ctx, nil))
	require.NoError(t, apt.InstallPackages(ctx, []manifest.PackageItem{{Name: " "}}))

	rpm := agent.NewRpmPackageManager(runner).WithSudo(false)
	require.NoError(t, rpm.InstallPackages(ctx, nil))

	pip := agent.NewPipPackageManager(runner, "", "")
	require.NoError(t, pip.InstallPackages(ctx, nil))

	doc := agent.NewDockerPackageManager(runner)
	require.NoError(t, doc.InstallPackages(ctx, nil))
	require.NoError(t, doc.InstallPackages(ctx, []manifest.PackageItem{{Name: ""}}))

	dewy := agent.NewDewyPackageManager(runner, "", "")
	require.NoError(t, dewy.InstallPackages(ctx, nil))

	multi := agent.NewMultiPackageManager(nil)
	require.NoError(t, multi.InstallPackages(ctx, nil))
	require.NoError(t, multi.UpdateRepositories(ctx))

	assert.Empty(t, runner.calls)
}

func TestSortPackageTargets(t *testing.T) {
	t.Parallel()

	t.Run("Default Tier Priority (apt -> pip -> docker -> dewy)", func(t *testing.T) {
		t.Parallel()
		pkgs := map[string]manifest.PackageTarget{
			"docker-images": {Manager: "docker", Items: []manifest.PackageItem{{Name: "img.tar"}}},
			"dewy-binaries": {Manager: "dewy", Items: []manifest.PackageItem{{Name: "app"}}},
			"os-packages":   {Manager: "apt", Items: []manifest.PackageItem{{Name: "libssl3"}}},
			"python-wheels": {Manager: "pip", Items: []manifest.PackageItem{{Name: "fastapi"}}},
		}

		sorted := agent.SortPackageTargets(pkgs)
		require.Len(t, sorted, 4)
		assert.Equal(t, "os-packages", sorted[0].Key)
		assert.Equal(t, "apt", sorted[0].Target.Manager)

		assert.Equal(t, "python-wheels", sorted[1].Key)
		assert.Equal(t, "pip", sorted[1].Target.Manager)

		assert.Equal(t, "docker-images", sorted[2].Key)
		assert.Equal(t, "docker", sorted[2].Target.Manager)

		assert.Equal(t, "dewy-binaries", sorted[3].Key)
		assert.Equal(t, "dewy", sorted[3].Target.Manager)
	})

	t.Run("Explicit Order overrides default priority", func(t *testing.T) {
		t.Parallel()
		pkgs := map[string]manifest.PackageTarget{
			"step-3-os":  {Manager: "apt", Order: 3, Items: []manifest.PackageItem{{Name: "curl"}}},
			"step-1-bin": {Manager: "dewy", Order: 1, Items: []manifest.PackageItem{{Name: "init"}}},
			"step-2-pip": {Manager: "pip", Order: 2, Items: []manifest.PackageItem{{Name: "tools"}}},
		}

		sorted := agent.SortPackageTargets(pkgs)
		require.Len(t, sorted, 3)
		assert.Equal(t, "step-1-bin", sorted[0].Key)
		assert.Equal(t, "step-2-pip", sorted[1].Key)
		assert.Equal(t, "step-3-os", sorted[2].Key)
	})

	t.Run("DependsOn Dependency Enforced", func(t *testing.T) {
		t.Parallel()
		pkgs := map[string]manifest.PackageTarget{
			"docker-runtime": {
				Manager:   "docker",
				DependsOn: []string{"docker-prerequisites"},
				Items:     []manifest.PackageItem{{Name: "container.tar"}},
			},
			"docker-prerequisites": {
				Manager: "apt",
				Items:   []manifest.PackageItem{{Name: "iptables"}, {Name: "ca-certificates"}},
			},
		}

		sorted := agent.SortPackageTargets(pkgs)
		require.Len(t, sorted, 2)
		assert.Equal(t, "docker-prerequisites", sorted[0].Key)
		assert.Equal(t, "docker-runtime", sorted[1].Key)
	})

	t.Run("Empty or nil map returns nil", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, agent.SortPackageTargets(nil))
		assert.Nil(t, agent.SortPackageTargets(map[string]manifest.PackageTarget{}))
	})
}
