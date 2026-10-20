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

package manifest_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertComposeToManifest_Valid(t *testing.T) {
	t.Parallel()

	composeYAML := `
version: "3.8"
services:
  postgres:
    image: postgres:16-alpine
    restart: always
    ports:
      - "5432:5432"
    volumes:
      - "/var/lib/postgresql/data:/var/lib/postgresql/data"
    environment:
      POSTGRES_DB: musubi
      POSTGRES_USER: musubi
      POSTGRES_PASSWORD: secret_password
  grafana:
    image: grafana/grafana:10.4.0
    restart: on-failure
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
`

	m, err := manifest.ConvertComposeToManifest([]byte(composeYAML), "musubi-stack")
	require.NoError(t, err)
	require.NotNil(t, m)

	assert.Equal(t, "1.0", m.Version)
	assert.Equal(t, "musubi-stack", m.Release)
	require.Len(t, m.Containers, 2)

	// Check postgres container
	var pgCtr *manifest.ContainerSpec
	var grafanaCtr *manifest.ContainerSpec
	for i := range m.Containers {
		switch m.Containers[i].Name {
		case "postgres":
			pgCtr = &m.Containers[i]
		case "grafana":
			grafanaCtr = &m.Containers[i]
		}
	}

	require.NotNil(t, pgCtr)
	assert.Equal(t, "postgres:16-alpine", pgCtr.Image)
	assert.Equal(t, "images/postgres.tar", pgCtr.ImageArchive)
	assert.Equal(t, []string{"5432:5432"}, pgCtr.Ports)
	assert.Equal(t, []string{"/var/lib/postgresql/data:/var/lib/postgresql/data"}, pgCtr.Volumes)
	assert.Equal(t, "musubi", pgCtr.Environment["POSTGRES_DB"])
	assert.Equal(t, "musubi", pgCtr.Environment["POSTGRES_USER"])
	assert.Equal(t, "secret_password", pgCtr.Environment["POSTGRES_PASSWORD"])
	assert.True(t, *pgCtr.AutoRestart)

	require.NotNil(t, grafanaCtr)
	assert.Equal(t, "grafana/grafana:10.4.0", grafanaCtr.Image)
	assert.Equal(t, "images/grafana.tar", grafanaCtr.ImageArchive)
	assert.Equal(t, []string{"3000:3000"}, grafanaCtr.Ports)
	assert.Equal(t, "admin", grafanaCtr.Environment["GF_SECURITY_ADMIN_PASSWORD"])
}

func TestConvertComposeToManifest_MissingImage(t *testing.T) {
	t.Parallel()

	invalidYAML := `
services:
  broken:
    ports:
      - "8080:8080"
`

	_, err := manifest.ConvertComposeToManifest([]byte(invalidYAML), "invalid")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required 'image'")
}

func TestParse_DirectComposeIngestion(t *testing.T) {
	t.Parallel()

	composeYAML := `
version: "3"
services:
  web:
    image: nginx:alpine
    ports:
      - "80:80"
  worker:
    image: python:3.11-slim
    command: ["python", "worker.py"]
`

	m, err := manifest.Parse([]byte(composeYAML))
	require.NoError(t, err)
	require.NotNil(t, m)

	assert.Equal(t, "1.0", m.Version)
	assert.Equal(t, "compose-app", m.Release)
	assert.Len(t, m.Containers, 2)
}

func TestValidate_ContainersOnly(t *testing.T) {
	t.Parallel()

	m := &manifest.Manifest{
		Version:       "1.0",
		BundleVersion: "1.0.0",
		Release:       "container-only-release",
		Targets: []manifest.Target{
			{OS: "linux", Release: "ubuntu-24.04", Arch: "amd64"},
		},
		Containers: []manifest.ContainerSpec{
			{Name: "redis", Image: "redis:7"},
		},
	}

	err := manifest.Validate(m)
	require.NoError(t, err)
}

func TestParseFile_WithComposeFileDirective(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	composePath := filepath.Join(tmpDir, "docker-compose.yml")
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")

	composeContent := `
services:
  db:
    image: postgres:16
    ports:
      - "5432:5432"
`
	require.NoError(t, os.WriteFile(composePath, []byte(composeContent), 0600))

	manifestContent := `
version: "1.0"
bundleVersion: "1.0.0"
release: "hospital-system"
composeFile: "docker-compose.yml"
targets:
  - os: "linux"
    release: "ubuntu-24.04"
    arch: "amd64"
applications:
  - name: "gateway"
    type: "golang"
    destination: "/opt/gateway"
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0600))

	parsed, err := manifest.ParseFile(manifestPath)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	assert.Equal(t, "hospital-system", parsed.Release)
	require.Len(t, parsed.Applications, 1)
	assert.Equal(t, "gateway", parsed.Applications[0].Name)

	require.Len(t, parsed.Containers, 1)
	assert.Equal(t, "db", parsed.Containers[0].Name)
	assert.Equal(t, "postgres:16", parsed.Containers[0].Image)
}
