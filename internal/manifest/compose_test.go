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
