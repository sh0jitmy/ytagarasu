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

package delta_test

import (
	"bytes"
	"crypto/ed25519"
	"os"
	"path/filepath"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/bundle"
	"github.com/sh0jitmy/ytagarasu/internal/delta"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDelta_ComputeDiffAndBuildDeltaBundle(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()

	// 1. Create Base Release (1.0.0) with large package files (simulate .deb/.rpm ~ 2MB)
	baseSrcDir := filepath.Join(workDir, "base-src")
	require.NoError(t, os.MkdirAll(filepath.Join(baseSrcDir, "packages"), 0750))
	require.NoError(t, os.MkdirAll(filepath.Join(baseSrcDir, "bin"), 0750))

	largePackageData := bytes.Repeat([]byte("large-dependency-package-payload-block-"), 50000) // ~2MB
	require.NoError(t, os.WriteFile(filepath.Join(baseSrcDir, "packages", "libssl.deb"), largePackageData, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(baseSrcDir, "bin", "app-binary"), []byte("v1-binary-code"), 0700)) //nolint:gosec

	baseManifestYAML := `
version: "1.0"
bundleVersion: "2026.09.27.1"
release: "1.0.0"
createdAt: "2026-09-27T00:00:00Z"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
applications:
  - name: "my-service"
    type: "golang"
    artifact: "bin/app-binary"
    destination: "/usr/local/bin/my-service"
`
	baseManifestPath := filepath.Join(baseSrcDir, "manifest.yaml")
	require.NoError(t, os.WriteFile(baseManifestPath, []byte(baseManifestYAML), 0600))

	baseBundleTarGz := filepath.Join(workDir, "release-1.0.0.tar.gz")
	_, privKey, err := bundle.GenerateKeyPair()
	require.NoError(t, err)

	_, err = bundle.BuildBundle(bundle.BuildOptions{
		ManifestPath: baseManifestPath,
		SourceDir:    baseSrcDir,
		OutputFile:   baseBundleTarGz,
		PrivateKey:   privKey,
	})
	require.NoError(t, err)
	assert.FileExists(t, baseBundleTarGz)

	// 2. Create Target Release (1.0.1)
	// Keeps libssl.deb identical (~2MB unchanged), updates app-binary (~15 bytes modified), adds config.yaml
	targetSrcDir := filepath.Join(workDir, "target-src")
	require.NoError(t, os.MkdirAll(filepath.Join(targetSrcDir, "packages"), 0750))
	require.NoError(t, os.MkdirAll(filepath.Join(targetSrcDir, "bin"), 0750))

	require.NoError(t, os.WriteFile(filepath.Join(targetSrcDir, "packages", "libssl.deb"), largePackageData, 0600))
	require.NoError(t, os.WriteFile(filepath.Join(targetSrcDir, "bin", "app-binary"), []byte("v2-updated-binary-code"), 0700)) //nolint:gosec
	require.NoError(t, os.WriteFile(filepath.Join(targetSrcDir, "config.yaml"), []byte("setting: true"), 0600))

	targetManifestYAML := `
version: "1.0"
bundleVersion: "2026.09.27.2"
release: "1.0.1"
createdAt: "2026-09-27T01:00:00Z"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
applications:
  - name: "my-service"
    type: "golang"
    artifact: "bin/app-binary"
    destination: "/usr/local/bin/my-service"
`
	targetManifestPath := filepath.Join(targetSrcDir, "manifest.yaml")
	require.NoError(t, os.WriteFile(targetManifestPath, []byte(targetManifestYAML), 0600))

	// 3. Build Delta Bundle
	deltaBundleTarGz := filepath.Join(workDir, "delta-1.0.0-to-1.0.1.tar.gz")
	deltaReportPath := filepath.Join(workDir, "delta-report.yaml")

	report, err := delta.BuildDeltaBundle(delta.BuildDeltaOptions{
		BaseBundlePath: baseBundleTarGz,
		ManifestPath:   targetManifestPath,
		SourceDir:      targetSrcDir,
		OutputFile:     deltaBundleTarGz,
		ReportPath:     deltaReportPath,
		PrivateKey:     privKey,
	})
	require.NoError(t, err)
	require.NotNil(t, report)

	assert.Equal(t, "1.0.0", report.BaseRelease)
	assert.Equal(t, "1.0.1", report.TargetRelease)
	assert.Equal(t, 1, report.AddedCount)     // config.yaml
	assert.Equal(t, 1, report.ModifiedCount)  // bin/app-binary
	assert.Equal(t, 1, report.UnchangedCount) // packages/libssl.deb

	// The delta bundle should save > 80% because largePackageData (2MB) is omitted!
	t.Logf("Full size: %d bytes, Delta size: %d bytes, Reduction: %.2f%%",
		report.FullSizeBytes, report.DeltaSizeBytes, report.ReductionPercent)
	assert.GreaterOrEqual(t, report.ReductionPercent, 80.0)

	// 4. Verify delta bundle integrity via bundle.VerifyBundle
	pubKey := privKey.Public().(ed25519.PublicKey)
	vRes, err := bundle.VerifyBundle(deltaBundleTarGz, pubKey)
	require.NoError(t, err)
	require.NotNil(t, vRes)
	assert.True(t, vRes.Valid)
	assert.Equal(t, "delta", vRes.Manifest.BundleType)
	assert.Equal(t, "1.0.0", vRes.Manifest.BaseRelease)
	assert.NotEmpty(t, vRes.Manifest.BaseReleaseChecksum)
	assert.Equal(t, "1.0.1", vRes.Manifest.TargetRelease)

	// Ensure large libssl.deb was NOT packaged inside delta bundle
	_, hasLibssl := vRes.Files["packages/libssl.deb"]
	assert.False(t, hasLibssl)
	// Ensure updated binary and new config ARE packaged
	_, hasBinary := vRes.Files["bin/app-binary"]
	assert.True(t, hasBinary)
	_, hasConfig := vRes.Files["config.yaml"]
	assert.True(t, hasConfig)
}

func TestDelta_ComputeDiff(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	src1 := filepath.Join(workDir, "src1")
	src2 := filepath.Join(workDir, "src2")
	require.NoError(t, os.MkdirAll(filepath.Join(src1, "bin"), 0750))
	require.NoError(t, os.MkdirAll(filepath.Join(src2, "bin"), 0750))

	require.NoError(t, os.WriteFile(filepath.Join(src1, "bin", "shared"), []byte("shared-bytes"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(src1, "bin", "old-only"), []byte("old-bytes"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(src1, "bin", "modified"), []byte("ver-1"), 0600))

	require.NoError(t, os.WriteFile(filepath.Join(src2, "bin", "shared"), []byte("shared-bytes"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(src2, "bin", "new-only"), []byte("new-bytes"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(src2, "bin", "modified"), []byte("ver-2-longer"), 0600))

	m1YAML := `
version: "1.0"
bundleVersion: "2026.09.27.1"
release: "1.0.0"
createdAt: "2026-09-27T00:00:00Z"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
applications:
  - name: "svc"
    type: "golang"
    artifact: "bin/shared"
    destination: "/usr/local/bin/shared"
`
	m2YAML := `
version: "1.0"
bundleVersion: "2026.09.27.2"
release: "1.0.1"
createdAt: "2026-09-27T01:00:00Z"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
applications:
  - name: "svc"
    type: "golang"
    artifact: "bin/shared"
    destination: "/usr/local/bin/shared"
`
	m1Path := filepath.Join(src1, "manifest.yaml")
	m2Path := filepath.Join(src2, "manifest.yaml")
	require.NoError(t, os.WriteFile(m1Path, []byte(m1YAML), 0600))
	require.NoError(t, os.WriteFile(m2Path, []byte(m2YAML), 0600))

	bundle1 := filepath.Join(workDir, "b1.tar.gz")
	bundle2 := filepath.Join(workDir, "b2.tar.gz")

	_, priv, err := bundle.GenerateKeyPair()
	require.NoError(t, err)

	_, err = bundle.BuildBundle(bundle.BuildOptions{ManifestPath: m1Path, SourceDir: src1, OutputFile: bundle1, PrivateKey: priv})
	require.NoError(t, err)
	_, err = bundle.BuildBundle(bundle.BuildOptions{ManifestPath: m2Path, SourceDir: src2, OutputFile: bundle2, PrivateKey: priv})
	require.NoError(t, err)

	diffReport, err := delta.ComputeDiff(bundle1, bundle2)
	require.NoError(t, err)
	require.NotNil(t, diffReport)

	assert.Equal(t, "1.0.0", diffReport.BaseRelease)
	assert.Equal(t, "1.0.1", diffReport.TargetRelease)
	assert.Equal(t, 1, diffReport.AddedCount)     // bin/new-only
	assert.Equal(t, 1, diffReport.DeletedCount)   // bin/old-only
	assert.Equal(t, 1, diffReport.ModifiedCount)  // bin/modified
	assert.Equal(t, 1, diffReport.UnchangedCount) // bin/shared
}
