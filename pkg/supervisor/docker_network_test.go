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

package supervisor_test

import (
	"context"
	"testing"

	"github.com/sh0jitmy/ytagarasu/pkg/supervisor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockNetworkCommander struct {
	inspectFunc func(ctx context.Context, name string) (bool, error)
	createFunc  func(ctx context.Context, name string) error
	removeFunc  func(ctx context.Context, name string) error
	inspected   []string
	created     []string
	removed     []string
}

func (m *mockNetworkCommander) Inspect(ctx context.Context, name string) (bool, error) {
	m.inspected = append(m.inspected, name)
	if m.inspectFunc != nil {
		return m.inspectFunc(ctx, name)
	}
	return false, nil
}

func (m *mockNetworkCommander) Create(ctx context.Context, name string) error {
	m.created = append(m.created, name)
	if m.createFunc != nil {
		return m.createFunc(ctx, name)
	}
	return nil
}

func (m *mockNetworkCommander) Remove(ctx context.Context, name string) error {
	m.removed = append(m.removed, name)
	if m.removeFunc != nil {
		return m.removeFunc(ctx, name)
	}
	return nil
}

func TestSanitizeDockerNetworkName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid simple",
			input:    "musubi-stack",
			expected: "musubi-stack",
		},
		{
			name:     "invalid symbols",
			input:    "musubi$stack@v1.0!",
			expected: "musubi-stack-v1.0",
		},
		{
			name:     "empty or only symbols",
			input:    "---",
			expected: "ytg-default-net",
		},
		{
			name:     "whitespace trim",
			input:    "  hospital_ehr_net  ",
			expected: "hospital_ehr_net",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			actual := supervisor.SanitizeDockerNetworkName(tt.input)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func TestEnsureDockerNetwork_Builtins(t *testing.T) {
	t.Parallel()

	for _, builtin := range []string{"bridge", "host", "none"} {
		created, err := supervisor.EnsureDockerNetwork(context.Background(), builtin)
		require.NoError(t, err)
		assert.False(t, created)
	}
}

//nolint:paralleltest // Mutates global test commander
func TestEnsureDockerNetwork_Lifecycle(t *testing.T) {
	var mock *mockNetworkCommander
	mock = &mockNetworkCommander{
		inspectFunc: func(ctx context.Context, name string) (bool, error) {
			// First call: not exists
			if len(mock.inspected) <= 1 {
				return false, nil
			}
			// Second call: already created
			return true, nil
		},
	}

	cleanup := supervisor.SetDockerNetworkCommanderForTest(mock)
	defer cleanup()

	ctx := context.Background()

	// 1. First ensure: creates network
	created, err := supervisor.EnsureDockerNetwork(ctx, "custom-net")
	require.NoError(t, err)
	assert.True(t, created)
	assert.Contains(t, mock.created, "custom-net")

	// 2. Second ensure: already exists, no create
	created2, err2 := supervisor.EnsureDockerNetwork(ctx, "custom-net")
	require.NoError(t, err2)
	assert.False(t, created2)
	assert.Len(t, mock.created, 1)

	// 3. Remove network
	err3 := supervisor.RemoveDockerNetwork(ctx, "custom-net")
	require.NoError(t, err3)
	assert.Contains(t, mock.removed, "custom-net")
}

func TestManifestSupervisor_NetworkAutoAssignment(t *testing.T) {
	t.Parallel()

	yamlData := `
version: "1.0"
release: "telecom-edge"
containers:
  - name: "redis"
    image: "redis:7.2"
    ports:
      - "6379:6379"
  - name: "worker"
    image: "worker:latest"
    network: "custom-cluster-net"
`

	ms, err := supervisor.NewManifestSupervisorFromYAML([]byte(yamlData))
	require.NoError(t, err)
	require.NotNil(t, ms)

	statuses := ms.Status()
	assert.Contains(t, statuses, "redis")
	assert.Contains(t, statuses, "worker")
}

func TestManifestSupervisor_NetworkArgumentsAssigned(t *testing.T) {
	t.Parallel()

	yamlData := `
version: "1.0"
release: "hospital-ehr"
containers:
  - name: "db"
    image: "postgres:16"
  - name: "api"
    image: "api:v1"
    network: "isolated-net"
`

	ms, err := supervisor.NewManifestSupervisorFromYAML([]byte(yamlData))
	require.NoError(t, err)
	require.NotNil(t, ms)

	// Verify statuses exist
	statuses := ms.Status()
	assert.Contains(t, statuses, "db")
	assert.Contains(t, statuses, "api")
}
