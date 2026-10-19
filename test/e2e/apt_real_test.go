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

package e2e_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/agent"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skipIfNotLinuxWithApt(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("Skipping APT real E2E test on non-Linux platform (current: " + runtime.GOOS + ")")
	}
	if _, err := exec.LookPath("apt-get"); err != nil {
		t.Skip("apt-get binary not found on host system, skipping")
	}
}

func TestAptReal_DirectPackageValidation(t *testing.T) {
	t.Parallel()
	skipIfNotLinuxWithApt(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)

	tmpDir := t.TempDir()
	debPath := filepath.Join(tmpDir, "dummy-test-pkg_1.0.0_all.deb")
	debBytes := []byte("!<arch>\ndebian-binary   1729000000  0     0     100644  4         `\n2.0\n")
	require.NoError(t, os.WriteFile(debPath, debBytes, 0600))

	h := sha256.Sum256(debBytes)
	expectedHash := hex.EncodeToString(h[:])

	// Use real OS runner without sudo escalation for safe CI environment testing
	runner := &agent.DefaultRunner{}
	aptMgr := agent.NewAptPackageManager(runner).WithSudo(false)

	t.Run("Valid local deb package verification passes", func(t *testing.T) {
		t.Parallel()
		pkgs := []manifest.PackageItem{
			{
				Name:   debPath,
				SHA256: expectedHash,
			},
		}
		// InstallPackages will validate checksum and existence before executing apt-get.
		// In non-root CI runner without sudo, apt-get install may return permission error
		// but checksum validation must have succeeded.
		err := aptMgr.InstallPackages(ctx, pkgs)
		if err != nil {
			require.NotErrorIs(t, err, agent.ErrChecksumMismatch)
			assert.Contains(t, err.Error(), "apt-get install failed")
		}
	})

	t.Run("Tampered package triggers immediate ErrChecksumMismatch", func(t *testing.T) {
		t.Parallel()
		tamperedItem := manifest.PackageItem{
			Name:   debPath,
			SHA256: "0000000000000000000000000000000000000000000000000000000000000000",
		}
		err := aptMgr.InstallPackages(ctx, []manifest.PackageItem{tamperedItem})
		require.Error(t, err)
		require.ErrorIs(t, err, agent.ErrChecksumMismatch)
	})

	t.Run("DirectInstaller interface handles local deb safely", func(t *testing.T) {
		t.Parallel()
		err := aptMgr.InstallLocalPackage(ctx, debPath, expectedHash)
		if err != nil {
			require.NotErrorIs(t, err, agent.ErrChecksumMismatch)
		}
	})
}

func TestAptReal_ElevateCommandConstruction(t *testing.T) {
	t.Parallel()

	cmdName, args := agent.ElevateCommand(false, "apt-get", "update", "-qq")
	assert.Equal(t, "apt-get", cmdName)
	assert.Equal(t, []string{"update", "-qq"}, args)

	cmdName, args = agent.ElevateCommand(true, "apt-get", "update", "-qq")
	assert.Equal(t, "sudo", cmdName)
	assert.Equal(t, []string{"-n", "apt-get", "update", "-qq"}, args)
}
