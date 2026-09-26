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

package importer_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/bundle"
	"github.com/sh0jitmy/ytagarasu/internal/delta"
	"github.com/sh0jitmy/ytagarasu/internal/server/importer"
	"github.com/sh0jitmy/ytagarasu/internal/server/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImporter_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	workDir := t.TempDir()

	// 1. Setup CAS and DB
	casDir := filepath.Join(workDir, "cas")
	cas, err := store.NewCAS(casDir)
	require.NoError(t, err)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := store.NewDB(dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	nsDir := filepath.Join(workDir, "namespaces")
	imp, err := importer.NewImporter(cas, db, nsDir)
	require.NoError(t, err)

	// 2. Generate signing keypair
	pubKey, privKey, err := bundle.GenerateKeyPair()
	require.NoError(t, err)

	// 3. Create test source bundle directory
	srcDir := filepath.Join(workDir, "bundle-src")
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "artifacts", "golang"), 0750))
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "configs"), 0750))
	require.NoError(t, os.MkdirAll(filepath.Join(srcDir, "apt"), 0750))

	require.NoError(t, os.WriteFile(filepath.Clean(filepath.Join(srcDir, "artifacts", "golang", "test-api")), []byte("bin-content"), 0700)) //nolint:gosec
	require.NoError(t, os.WriteFile(filepath.Clean(filepath.Join(srcDir, "configs", "config.yaml.tmpl")), []byte("port: {{ .Port }}"), 0600))
	require.NoError(t, os.WriteFile(filepath.Clean(filepath.Join(srcDir, "apt", "test-pkg.deb")), []byte("deb-package-content"), 0600))

	manifestContent := `
version: "1.0"
bundleVersion: "2026.09.26.1"
release: "2.1.0"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
applications:
  - name: "test-service"
    type: "golang"
    artifact: "artifacts/golang/test-api"
    destination: "/usr/local/bin/test-api"
configs:
  - template: "configs/config.yaml.tmpl"
    destination: "/etc/test/config.yaml"
`
	manifestFile := filepath.Join(srcDir, "manifest.yaml")
	require.NoError(t, os.WriteFile(filepath.Clean(manifestFile), []byte(manifestContent), 0600))

	// Build bundle
	bundlePath := filepath.Join(workDir, "test-bundle.tar.gz")
	evalResult, err := bundle.BuildBundle(bundle.BuildOptions{
		ManifestPath: manifestFile,
		SourceDir:    srcDir,
		OutputFile:   bundlePath,
		PrivateKey:   privKey,
	})
	require.NoError(t, err)
	require.Equal(t, "ready", string(evalResult.Status))

	// 4. Import bundle
	bundleData, err := os.ReadFile(filepath.Clean(bundlePath)) //nolint:gosec
	require.NoError(t, err)

	res, err := imp.Import(ctx, bytes.NewReader(bundleData), hex.EncodeToString(pubKey))
	require.NoError(t, err)
	assert.Equal(t, "test-service", res.ServiceID)
	assert.Equal(t, "2.1.0", res.ReleaseVersion)
	assert.Positive(t, res.FileCount)

	// 5. Verify database state
	activeRel, err := db.GetActiveRelease(ctx, "test-service")
	require.NoError(t, err)
	assert.Equal(t, res.ReleaseID, activeRel.ID)
	assert.Equal(t, "2.1.0", activeRel.ReleaseVersion)
	assert.Equal(t, "active", activeRel.Status)

	// 6. Verify filesystem namespace projection
	debPath := filepath.Join(nsDir, "test-service", "apt", "test-pkg.deb")
	debData, err := os.ReadFile(filepath.Clean(debPath)) //nolint:gosec
	require.NoError(t, err)
	assert.Equal(t, []byte("deb-package-content"), debData)
}

func TestImporter_TamperedBundle_Rejected(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	workDir := t.TempDir()

	cas, err := store.NewCAS(filepath.Join(workDir, "cas"))
	require.NoError(t, err)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := store.NewDB(dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	imp, err := importer.NewImporter(cas, db, filepath.Join(workDir, "namespaces"))
	require.NoError(t, err)

	// Construct an invalid / unsigned tar.gz archive
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	_ = tw.WriteHeader(&tar.Header{
		Name:     "manifest.yaml",
		Mode:     0644,
		Size:     14,
		Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write([]byte("fake-manifest"))
	_ = tw.Close()
	_ = gw.Close()

	// Import should fail due to missing signatures and checksums
	_, err = imp.Import(ctx, bytes.NewReader(buf.Bytes()), "")
	require.Error(t, err)

	// Verify no release is created
	_, err = db.GetActiveRelease(ctx, "test-service")
	require.Error(t, err)
}

func TestImporter_DeltaBundle_Success_And_Mismatch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	workDir := t.TempDir()

	cas, err := store.NewCAS(filepath.Join(workDir, "cas"))
	require.NoError(t, err)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := store.NewDB(dsn)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	nsDir := filepath.Join(workDir, "namespaces")
	imp, err := importer.NewImporter(cas, db, nsDir)
	require.NoError(t, err)

	// 1. Build Base Release (1.0.0) containing shared-binary and base-file
	baseSrc := filepath.Join(workDir, "base-src")
	require.NoError(t, os.MkdirAll(filepath.Join(baseSrc, "bin"), 0750))
	require.NoError(t, os.WriteFile(filepath.Join(baseSrc, "bin", "shared-app"), []byte("shared-v1"), 0700)) //nolint:gosec
	require.NoError(t, os.WriteFile(filepath.Join(baseSrc, "bin", "base-only"), []byte("base-data"), 0600))

	baseManifestYAML := `
version: "1.0"
bundleVersion: "2026.09.27.1"
release: "1.0.0"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
applications:
  - name: "delta-service"
    type: "golang"
    artifact: "bin/shared-app"
    destination: "/usr/local/bin/shared-app"
`
	baseManifestPath := filepath.Join(baseSrc, "manifest.yaml")
	require.NoError(t, os.WriteFile(baseManifestPath, []byte(baseManifestYAML), 0600))

	baseBundleTarGz := filepath.Join(workDir, "base.tar.gz")
	pubKey, privKey, err := bundle.GenerateKeyPair()
	require.NoError(t, err)

	_, err = bundle.BuildBundle(bundle.BuildOptions{
		ManifestPath: baseManifestPath,
		SourceDir:    baseSrc,
		OutputFile:   baseBundleTarGz,
		PrivateKey:   privKey,
	})
	require.NoError(t, err)

	// 2. Build Delta Bundle (Target Release 1.0.1) - keeps shared-app, adds updated-config
	targetSrc := filepath.Join(workDir, "target-src")
	require.NoError(t, os.MkdirAll(filepath.Join(targetSrc, "bin"), 0750))
	require.NoError(t, os.WriteFile(filepath.Join(targetSrc, "bin", "shared-app"), []byte("shared-v1"), 0700)) //nolint:gosec
	require.NoError(t, os.WriteFile(filepath.Join(targetSrc, "config.yaml"), []byte("new-config"), 0600))

	targetManifestYAML := `
version: "1.0"
bundleVersion: "2026.09.27.2"
release: "1.0.1"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
applications:
  - name: "delta-service"
    type: "golang"
    artifact: "bin/shared-app"
    destination: "/usr/local/bin/shared-app"
`
	targetManifestPath := filepath.Join(targetSrc, "manifest.yaml")
	require.NoError(t, os.WriteFile(targetManifestPath, []byte(targetManifestYAML), 0600))

	deltaBundleTarGz := filepath.Join(workDir, "delta.tar.gz")
	_, err = delta.BuildDeltaBundle(delta.BuildDeltaOptions{
		BaseBundlePath: baseBundleTarGz,
		ManifestPath:   targetManifestPath,
		SourceDir:      targetSrc,
		OutputFile:     deltaBundleTarGz,
		PrivateKey:     privKey,
	})
	require.NoError(t, err)

	// 3. Attempt to import delta bundle BEFORE base release is imported -> must fail (Base Release Mismatch)
	deltaBytes, err := os.ReadFile(filepath.Clean(deltaBundleTarGz)) //nolint:gosec
	require.NoError(t, err)

	_, err = imp.Import(ctx, bytes.NewReader(deltaBytes), hex.EncodeToString(pubKey))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "base release mismatch")

	// 4. Import Base Release (1.0.0)
	baseBytes, err := os.ReadFile(filepath.Clean(baseBundleTarGz)) //nolint:gosec
	require.NoError(t, err)

	resBase, err := imp.Import(ctx, bytes.NewReader(baseBytes), hex.EncodeToString(pubKey))
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", resBase.ReleaseVersion)

	// 5. Now import Delta Bundle -> must SUCCEED!
	resDelta, err := imp.Import(ctx, bytes.NewReader(deltaBytes), hex.EncodeToString(pubKey))
	require.NoError(t, err)
	assert.Equal(t, "1.0.1", resDelta.ReleaseVersion)

	// 6. Verify virtual repository has both the updated config and the carried-over base files
	activeRel, err := db.GetActiveRelease(ctx, "delta-service")
	require.NoError(t, err)
	assert.Equal(t, "1.0.1", activeRel.ReleaseVersion)

	// Check shared-app exists in service namespace
	sharedAppFile := filepath.Join(nsDir, "delta-service", "bin", "shared-app")
	assert.FileExists(t, sharedAppFile)
	configFile := filepath.Join(nsDir, "delta-service", "config.yaml")
	assert.FileExists(t, configFile)
}
