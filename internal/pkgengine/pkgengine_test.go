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

package pkgengine_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/shjtmy/go_sh0jitmy_template/internal/pkgengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleDebianPackages = `
Package: libc6
Version: 2.38-1ubuntu6
Architecture: amd64
Filename: pool/main/g/glibc/libc6_2.38-1ubuntu6_amd64.deb
Size: 3200000
SHA256: 0000000000000000000000000000000000000000000000000000000000000001

Package: libssl3
Version: 3.0.13-0ubuntu3.4
Architecture: amd64
Depends: libc6 (>= 2.38)
Filename: pool/main/o/openssl/libssl3_3.0.13-0ubuntu3.4_amd64.deb
Size: 1800000
SHA256: 0000000000000000000000000000000000000000000000000000000000000002

Package: nginx
Version: 1.24.0-2ubuntu7
Architecture: amd64
Depends: libc6 (>= 2.38), libssl3 (>= 3.0.0)
Provides: httpd, httpd-cgi
Filename: pool/main/n/nginx/nginx_1.24.0-2ubuntu7_amd64.deb
Size: 550000
SHA256: 0000000000000000000000000000000000000000000000000000000000000003

Package: cycle-a
Version: 1.0
Architecture: amd64
Depends: cycle-b
Filename: pool/main/c/cycle-a.deb
Size: 1000
SHA256: 000000000000000000000000000000000000000000000000000000000000000a

Package: cycle-b
Version: 1.0
Architecture: amd64
Depends: cycle-a
Filename: pool/main/c/cycle-b.deb
Size: 1000
SHA256: 000000000000000000000000000000000000000000000000000000000000000b
`

const sampleRPMPrimaryXML = `<?xml version="1.0" encoding="UTF-8"?>
<metadata xmlns="http://linux.duke.edu/metadata/common" xmlns:rpm="http://linux.duke.edu/metadata/rpm" packages="3">
  <package type="rpm">
    <name>glibc</name>
    <arch>x86_64</arch>
    <version ver="2.34" rel="60.el9"/>
    <checksum type="sha256">1111111111111111111111111111111111111111111111111111111111111111</checksum>
    <size package="2500000"/>
    <location href="Packages/glibc-2.34-60.el9.x86_64.rpm"/>
    <format>
      <rpm:provides>
        <rpm:entry name="glibc" ver="2.34" rel="60.el9"/>
        <rpm:entry name="libc.so.6()(64bit)"/>
      </rpm:provides>
    </format>
  </package>
  <package type="rpm">
    <name>openssl-libs</name>
    <arch>x86_64</arch>
    <version ver="3.0.7" rel="27.el9"/>
    <checksum type="sha256">2222222222222222222222222222222222222222222222222222222222222222</checksum>
    <size package="1900000"/>
    <location href="Packages/openssl-libs-3.0.7-27.el9.x86_64.rpm"/>
    <format>
      <rpm:provides>
        <rpm:entry name="openssl-libs"/>
        <rpm:entry name="libcrypto.so.3()(64bit)"/>
      </rpm:provides>
      <rpm:requires>
        <rpm:entry name="libc.so.6()(64bit)"/>
      </rpm:requires>
    </format>
  </package>
  <package type="rpm">
    <name>nginx</name>
    <arch>x86_64</arch>
    <version ver="1.24.0" rel="1.el9"/>
    <checksum type="sha256">3333333333333333333333333333333333333333333333333333333333333333</checksum>
    <size package="600000"/>
    <location href="Packages/nginx-1.24.0-1.el9.x86_64.rpm"/>
    <format>
      <rpm:provides>
        <rpm:entry name="nginx"/>
        <rpm:entry name="webserver"/>
      </rpm:provides>
      <rpm:requires>
        <rpm:entry name="libcrypto.so.3()(64bit)"/>
        <rpm:entry name="libc.so.6()(64bit)"/>
      </rpm:requires>
    </format>
  </package>
</metadata>
`

func TestAPT_ResolveDependencies_Success(t *testing.T) {
	t.Parallel()

	repo, err := pkgengine.ParsePackagesIndex([]byte(sampleDebianPackages))
	require.NoError(t, err)
	require.NotNil(t, repo)

	// Resolve nginx -> depends on libssl3 -> depends on libc6
	result, err := repo.ResolveDependencies([]string{"nginx"})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Len(t, result.Resolved, 3) // nginx, libssl3, libc6
	resolvedNames := make([]string, 0)
	for _, p := range result.Resolved {
		resolvedNames = append(resolvedNames, p.Name)
	}
	assert.Contains(t, resolvedNames, "nginx")
	assert.Contains(t, resolvedNames, "libssl3")
	assert.Contains(t, resolvedNames, "libc6")
	assert.Equal(t, int64(550000+1800000+3200000), result.TotalSize)
}

func TestAPT_ResolveDependencies_VirtualCapability(t *testing.T) {
	t.Parallel()

	repo, err := pkgengine.ParsePackagesIndex([]byte(sampleDebianPackages))
	require.NoError(t, err)

	// "httpd" is provided by nginx
	result, err := repo.ResolveDependencies([]string{"httpd"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Resolved, 3)
}

func TestAPT_ResolveDependencies_CyclicDependencies(t *testing.T) {
	t.Parallel()

	repo, err := pkgengine.ParsePackagesIndex([]byte(sampleDebianPackages))
	require.NoError(t, err)

	// cycle-a -> cycle-b -> cycle-a: must terminate safely
	result, err := repo.ResolveDependencies([]string{"cycle-a"})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, result.Resolved, 2)
}

func TestAPT_ResolveDependencies_MissingPackageError(t *testing.T) {
	t.Parallel()

	repo, err := pkgengine.ParsePackagesIndex([]byte(sampleDebianPackages))
	require.NoError(t, err)

	result, err := repo.ResolveDependencies([]string{"nonexistent-pkg"})
	require.Error(t, err)
	assert.Nil(t, result)

	var resErr *pkgengine.ResolutionError
	require.ErrorAs(t, err, &resErr)
	assert.Equal(t, "nonexistent-pkg", resErr.PackageName)
	assert.Contains(t, err.Error(), "not found in repository mirror index")
}

func TestAPT_ParseGzipPackages(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, err := gw.Write([]byte(sampleDebianPackages))
	require.NoError(t, err)
	require.NoError(t, gw.Close())

	repo, err := pkgengine.ParsePackagesIndex(buf.Bytes())
	require.NoError(t, err)
	assert.NotEmpty(t, repo.AllPackages)
}

func TestRPM_ResolveDependencies_Success(t *testing.T) {
	t.Parallel()

	repo, err := pkgengine.ParseRPMPrimary([]byte(sampleRPMPrimaryXML))
	require.NoError(t, err)
	require.NotNil(t, repo)

	// Resolve nginx -> requires libcrypto.so.3 (provided by openssl-libs) -> requires libc.so.6 (provided by glibc)
	result, err := repo.ResolveDependencies([]string{"nginx"})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Len(t, result.Resolved, 3)
	names := make([]string, 0)
	for _, p := range result.Resolved {
		names = append(names, p.Name)
	}
	assert.Contains(t, names, "nginx")
	assert.Contains(t, names, "openssl-libs")
	assert.Contains(t, names, "glibc")
	assert.Equal(t, int64(600000+1900000+2500000), result.TotalSize)
}

func TestRPM_ResolveDependencies_MissingCapabilityError(t *testing.T) {
	t.Parallel()

	repo, err := pkgengine.ParseRPMPrimary([]byte(sampleRPMPrimaryXML))
	require.NoError(t, err)

	result, err := repo.ResolveDependencies([]string{"libmissing.so.1()(64bit)"})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "not found in repository index")
}

func TestRPM_ParseGzipPrimary(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, err := gw.Write([]byte(sampleRPMPrimaryXML))
	require.NoError(t, err)
	require.NoError(t, gw.Close())

	repo, err := pkgengine.ParseRPMPrimary(buf.Bytes())
	require.NoError(t, err)
	assert.NotEmpty(t, repo.AllPackages)
}

func TestDownloader_DownloadAll_SuccessAndVerify(t *testing.T) {
	t.Parallel()

	file1Content := []byte("fake-deb-content-1")
	file2Content := []byte("fake-deb-content-2")

	hash1 := sha256Hex(file1Content)
	hash2 := sha256Hex(file2Content)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pool/pkg1.deb":
			_, _ = w.Write(file1Content)
		case "/pool/pkg2.deb":
			_, _ = w.Write(file2Content)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	downloader := pkgengine.NewDownloader(server.URL, tmpDir, 2)

	pkgs := []pkgengine.PackageMetadata{
		{
			Name:     "pkg1",
			Filename: "pool/pkg1.deb",
			SHA256:   hash1,
			Size:     int64(len(file1Content)),
		},
		{
			Name:     "pkg2",
			Filename: "pool/pkg2.deb",
			SHA256:   hash2,
			Size:     int64(len(file2Content)),
		},
	}

	// 1. Initial download
	err := downloader.DownloadAll(context.Background(), pkgs)
	require.NoError(t, err)

	dest1 := filepath.Join(tmpDir, "pkg1.deb")
	dest2 := filepath.Join(tmpDir, "pkg2.deb")
	assert.FileExists(t, dest1)
	assert.FileExists(t, dest2)

	content1, err := os.ReadFile(filepath.Clean(dest1))
	require.NoError(t, err)
	assert.Equal(t, file1Content, content1)

	// 2. Idempotent check (re-download with existing verified files)
	err = downloader.DownloadAll(context.Background(), pkgs)
	require.NoError(t, err)
}

func TestDownloader_ChecksumMismatchError(t *testing.T) {
	t.Parallel()

	fileContent := []byte("some-corrupted-content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fileContent)
	}))
	defer server.Close()

	tmpDir := t.TempDir()
	downloader := pkgengine.NewDownloader(server.URL, tmpDir, 1)

	pkgs := []pkgengine.PackageMetadata{
		{
			Name:     "corrupt-pkg",
			Filename: "corrupt.deb",
			SHA256:   "0000000000000000000000000000000000000000000000000000000000000000", // invalid
		},
	}

	err := downloader.DownloadAll(context.Background(), pkgs)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SHA-256 checksum mismatch")
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
