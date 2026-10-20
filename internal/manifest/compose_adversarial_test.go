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

// TestCompose_Adversarial_NumericPortsAndTypeCoercion verifies that unquoted numeric ports,
// boolean/integer environment variables, and depends_on blocks do not fail parsing.
func TestCompose_Adversarial_NumericPortsAndTypeCoercion(t *testing.T) {
	t.Parallel()

	composeYAML := []byte(`
version: '3.8'
services:
  web:
    image: nginx:alpine
    ports:
      - 80
      - 8080:80
      - "127.0.0.1:3000:3000"
    environment:
      DEBUG: true
      PORT: 8080
      TIMEOUT_SEC: 30
      EMPTY_KEY:
      NORMAL_STRING: "production"
    depends_on:
      - db
      - redis
    command: "nginx -g 'daemon off;'"

  db:
    image: postgres:16-alpine
    ports:
      - 5432
    environment:
      - POSTGRES_DB=testdb
      - POSTGRES_PORT=5432
      - POSTGRES_SSL=false
      - EMPTY_LIST_KEY=
      - STANDALONE_KEY
    volumes:
      - ./data:/var/lib/postgresql/data:ro
      - pgdata:/var/lib/postgresql
    depends_on:
      redis:
        condition: service_healthy
`)

	m, err := manifest.ConvertComposeToManifest(composeYAML, "adversarial-release")
	require.NoError(t, err, "ConvertComposeToManifest must parse unquoted ports, typed environment, and depends_on without error")
	require.NotNil(t, m)
	assert.Equal(t, "adversarial-release", m.Release)
	require.Len(t, m.Containers, 2)

	// Validate 'web' container
	var webCtr *manifest.ContainerSpec
	var dbCtr *manifest.ContainerSpec
	for i := range m.Containers {
		switch m.Containers[i].Name {
		case "web":
			webCtr = &m.Containers[i]
		case "db":
			dbCtr = &m.Containers[i]
		}
	}

	require.NotNil(t, webCtr)
	require.NotNil(t, dbCtr)

	// Verify numeric ports coercion
	assert.Contains(t, webCtr.Ports, "80")
	assert.Contains(t, webCtr.Ports, "8080:80")
	assert.Contains(t, webCtr.Ports, "127.0.0.1:3000:3000")

	// Verify typed environment coercion
	assert.Equal(t, "true", webCtr.Environment["DEBUG"])
	assert.Equal(t, "8080", webCtr.Environment["PORT"])
	assert.Equal(t, "30", webCtr.Environment["TIMEOUT_SEC"])
	assert.Empty(t, webCtr.Environment["EMPTY_KEY"])
	assert.Equal(t, "production", webCtr.Environment["NORMAL_STRING"])

	// Verify command string splitting
	assert.Equal(t, []string{"nginx", "-g", "'daemon", "off;'"}, webCtr.Command)

	// Verify DB container numeric port and list environment
	assert.Contains(t, dbCtr.Ports, "5432")
	assert.Equal(t, "testdb", dbCtr.Environment["POSTGRES_DB"])
	assert.Equal(t, "5432", dbCtr.Environment["POSTGRES_PORT"])
	assert.Equal(t, "false", dbCtr.Environment["POSTGRES_SSL"])
	assert.Empty(t, dbCtr.Environment["EMPTY_LIST_KEY"])
	assert.Empty(t, dbCtr.Environment["STANDALONE_KEY"])

	// Verify volumes
	assert.Contains(t, dbCtr.Volumes, "./data:/var/lib/postgresql/data:ro")
	assert.Contains(t, dbCtr.Volumes, "pgdata:/var/lib/postgresql")

	// Verify depends_on captured if supported
	assert.Contains(t, webCtr.DependsOn, "db")
	assert.Contains(t, webCtr.DependsOn, "redis")
	assert.Contains(t, dbCtr.DependsOn, "redis")
}

// TestCompose_Adversarial_ErrorCases verifies that empty input, missing services,
// or missing required image attribute return specific, actionable errors.
func TestCompose_Adversarial_ErrorCases(t *testing.T) {
	t.Parallel()

	// 1. Zero bytes
	_, err := manifest.ConvertComposeToManifest([]byte{}, "empty")
	require.Error(t, err)

	// 2. Services map is empty
	_, err = manifest.ConvertComposeToManifest([]byte("services: {}\n"), "no-svc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not contain any services")

	// 3. Service missing required image
	noImageYAML := []byte(`
services:
  bad-service:
    ports:
      - "8080:80"
`)
	_, err = manifest.ConvertComposeToManifest(noImageYAML, "bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required 'image' attribute")
}
