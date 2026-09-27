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
	"errors"
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

	t.Run("UpdateRepositories", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewAptPackageManager(runner)

		err := mgr.UpdateRepositories(context.Background())
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "apt-get", runner.calls[0].name)
		assert.Equal(t, []string{"update", "-qq"}, runner.calls[0].args)
		assert.Contains(t, runner.calls[0].env, "DEBIAN_FRONTEND=noninteractive")
	})

	t.Run("InstallPackages with versions", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewAptPackageManager(runner)

		pkgs := []manifest.PackageItem{
			{Name: "curl", Version: "7.88.1"},
			{Name: "git", Version: ""},
		}

		err := mgr.InstallPackages(context.Background(), pkgs)
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "apt-get", runner.calls[0].name)
		assert.Equal(t, []string{"install", "-y", "--no-install-recommends", "--allow-unauthenticated", "curl=7.88.1", "git"}, runner.calls[0].args)
	})

	t.Run("InstallPackages failure", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{err: errors.New("exit status 100"), out: []byte("E: Unable to locate package")}
		mgr := agent.NewAptPackageManager(runner)

		err := mgr.InstallPackages(context.Background(), []manifest.PackageItem{{Name: "nonexistent"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "apt-get install failed")
	})
}

func TestRpmPackageManager(t *testing.T) {
	t.Parallel()

	t.Run("UpdateRepositories", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewRpmPackageManager(runner)

		err := mgr.UpdateRepositories(context.Background())
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "dnf", runner.calls[0].name)
		assert.Equal(t, []string{"makecache", "-y", "-q"}, runner.calls[0].args)
	})

	t.Run("InstallPackages", func(t *testing.T) {
		t.Parallel()
		runner := &fakeRunner{}
		mgr := agent.NewRpmPackageManager(runner)

		pkgs := []manifest.PackageItem{
			{Name: "nginx", Version: "1.24"},
			{Name: "sqlite", Version: ""},
		}

		err := mgr.InstallPackages(context.Background(), pkgs)
		require.NoError(t, err)
		require.Len(t, runner.calls, 1)
		assert.Equal(t, "dnf", runner.calls[0].name)
		assert.Equal(t, []string{"install", "-y", "--nogpgcheck", "nginx-1.24", "sqlite"}, runner.calls[0].args)
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
	multi := agent.NewMultiPackageManager(agent.NewAptPackageManager(runner))
	multi.Register("dnf", agent.NewRpmPackageManager(runner))
	multi.Register("pip", agent.NewPipPackageManager(runner, "python3", "/wheels"))
	multi.Register("docker", agent.NewDockerPackageManager(runner))

	// Dispatch to DNF
	err := multi.InstallForManager(context.Background(), "dnf", []manifest.PackageItem{{Name: "htop"}})
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

	apt := agent.NewAptPackageManager(runner)
	require.NoError(t, apt.InstallPackages(ctx, nil))
	require.NoError(t, apt.InstallPackages(ctx, []manifest.PackageItem{{Name: " "}}))

	rpm := agent.NewRpmPackageManager(runner)
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
