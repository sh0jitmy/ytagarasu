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

package bundle_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/bundle"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testManifestYAML = `
version: "1.0"
bundleVersion: "2026.09.27.1"
release: "1.0.0"
createdAt: "2026-09-27T00:00:00Z"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
applications:
  - name: "test-app"
    type: "golang"
    selector:
      roles: ["api"]
    artifact: "artifacts/golang/test-app"
    destination: "/usr/local/bin/test-app"
    permissions: "0755"
`

func setupValidBundleDir(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()

	manifestPath := filepath.Join(dir, "manifest.yaml")
	require.NoError(t, os.WriteFile(manifestPath, []byte(testManifestYAML), 0600))

	binDir := filepath.Join(dir, "artifacts", "golang")
	require.NoError(t, os.MkdirAll(binDir, 0750))
	require.NoError(t, os.WriteFile(filepath.Join(binDir, "test-app"), []byte("binary-payload"), 0750)) //nolint:gosec

	return dir, manifestPath
}

func TestBuildAndVerifyBundle_Success(t *testing.T) {
	t.Parallel()

	srcDir, manifestPath := setupValidBundleDir(t)
	outTarGz := filepath.Join(t.TempDir(), "test-bundle.tar.gz")
	reportFile := filepath.Join(t.TempDir(), "preparation-result.yaml")

	pubKey, privKey, err := bundle.GenerateKeyPair()
	require.NoError(t, err)

	opts := bundle.BuildOptions{
		ManifestPath:     manifestPath,
		SourceDir:        srcDir,
		OutputFile:       outTarGz,
		ReportOutputFile: reportFile,
		PrivateKey:       privKey,
		TargetPlatform:   manifest.Target{OS: "ubuntu", Release: "24.04", Arch: "amd64"},
	}

	evalRes, err := bundle.BuildBundle(opts)
	require.NoError(t, err)
	require.NotNil(t, evalRes)
	assert.Equal(t, manifest.StatusReady, evalRes.Status)
	assert.FileExists(t, outTarGz)
	assert.FileExists(t, reportFile)

	// Verify the generated bundle
	vRes, err := bundle.VerifyBundle(outTarGz, pubKey)
	require.NoError(t, err)
	require.NotNil(t, vRes)
	assert.True(t, vRes.Valid)
	assert.True(t, vRes.SignatureValid)
	assert.True(t, vRes.ChecksumsValid)
	assert.Positive(t, vRes.FileCount)
	assert.Empty(t, vRes.Errors)
	assert.Equal(t, "test-app", vRes.Manifest.Applications[0].Name)
}

func TestBuildBundle_BlockedWhenMissingArtifact(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "manifest.yaml")
	// Points to nonexistent binary
	require.NoError(t, os.WriteFile(manifestPath, []byte(testManifestYAML), 0600))

	outTarGz := filepath.Join(t.TempDir(), "blocked-bundle.tar.gz")
	reportFile := filepath.Join(t.TempDir(), "prep-blocked.yaml")

	opts := bundle.BuildOptions{
		ManifestPath:     manifestPath,
		SourceDir:        dir,
		OutputFile:       outTarGz,
		ReportOutputFile: reportFile,
		TargetPlatform:   manifest.Target{OS: "ubuntu", Release: "24.04", Arch: "amd64"},
	}

	evalRes, err := bundle.BuildBundle(opts)
	require.Error(t, err)
	require.ErrorIs(t, err, bundle.ErrBundleBlocked)
	assert.Equal(t, manifest.StatusBlocked, evalRes.Status)
	assert.NoFileExists(t, outTarGz) // Archive must NOT be produced when blocked
	assert.FileExists(t, reportFile)
}

func TestVerifyBundle_TamperedSignature(t *testing.T) {
	t.Parallel()

	srcDir, manifestPath := setupValidBundleDir(t)
	outTarGz := filepath.Join(t.TempDir(), "test-bundle.tar.gz")

	_, privKey, err := bundle.GenerateKeyPair()
	require.NoError(t, err)

	opts := bundle.BuildOptions{
		ManifestPath: manifestPath,
		SourceDir:    srcDir,
		OutputFile:   outTarGz,
		PrivateKey:   privKey,
	}

	_, err = bundle.BuildBundle(opts)
	require.NoError(t, err)

	// Verify using different random public key -> must detect mismatch
	otherPub, _, err := bundle.GenerateKeyPair()
	require.NoError(t, err)

	vRes, err := bundle.VerifyBundle(outTarGz, otherPub)
	require.NoError(t, err)
	assert.False(t, vRes.Valid)
	assert.False(t, vRes.SignatureValid)
	assert.NotEmpty(t, vRes.Errors)
}

func TestCrypto_SaveAndLoadKeys(t *testing.T) {
	t.Parallel()

	pub, priv, err := bundle.GenerateKeyPair()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	privFile := filepath.Join(tmpDir, "private.key")
	pubFile := filepath.Join(tmpDir, "public.key")

	require.NoError(t, bundle.SaveKeyToFile(privFile, priv, 0600))
	require.NoError(t, bundle.SaveKeyToFile(pubFile, pub, 0640))

	loadedPriv, err := bundle.LoadPrivateKeyFromFile(privFile)
	require.NoError(t, err)
	assert.Equal(t, priv, loadedPriv)

	loadedPub, err := bundle.LoadPublicKeyFromFile(pubFile)
	require.NoError(t, err)
	assert.Equal(t, pub, loadedPub)

	// Sign and verify with loaded keys
	data := []byte("hello-air-gap")
	sig := bundle.SignData(loadedPriv, data)
	assert.True(t, bundle.VerifySignature(loadedPub, data, sig))
	assert.False(t, bundle.VerifySignature(loadedPub, []byte("tampered-data"), sig))
}
