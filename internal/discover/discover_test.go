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

package discover_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/discover"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSurvey_MockFileSystem(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// Setup mock systemd directories
	sysSystemd := filepath.Join(tmpDir, "etc", "systemd", "system")
	require.NoError(t, os.MkdirAll(sysSystemd, 0750))

	binDir := filepath.Join(tmpDir, "usr", "local", "bin")
	require.NoError(t, os.MkdirAll(binDir, 0750))

	confDir := filepath.Join(tmpDir, "etc", "billing")
	require.NoError(t, os.MkdirAll(confDir, 0750))

	// Mock binary
	mockBinPath := filepath.Join(binDir, "billing-worker")
	require.NoError(t, os.WriteFile(mockBinPath, []byte("#!/bin/sh\necho billing"), 0600))

	// Mock config
	mockConfPath := filepath.Join(confDir, "worker.env")
	require.NoError(t, os.WriteFile(mockConfPath, []byte("PORT=8080\nDB_HOST=localhost"), 0600))

	// Mock unit 1: billing-worker.service (should be discovered)
	unit1Content := `[Unit]
Description=Billing Worker Service

[Service]
ExecStart=/usr/local/bin/billing-worker --port 8080
EnvironmentFile=/etc/billing/worker.env
Restart=always
`
	require.NoError(t, os.WriteFile(filepath.Join(sysSystemd, "billing-worker.service"), []byte(unit1Content), 0600))

	// Mock unit 2: systemd-resolved.service (should be ignored by default)
	unit2Content := `[Unit]
Description=Network Name Resolution

[Service]
ExecStart=/lib/systemd/systemd-resolved
`
	require.NoError(t, os.WriteFile(filepath.Join(sysSystemd, "systemd-resolved.service"), []byte(unit2Content), 0600))

	// Test 1: Full survey (system services excluded)
	opts := discover.SurveyOptions{
		Sysroot: tmpDir,
	}
	plan, err := discover.Survey(opts)
	require.NoError(t, err)
	require.NotNil(t, plan)
	assert.Equal(t, "1.0", plan.Version)

	require.Len(t, plan.Targets.Services, 1)
	svc := plan.Targets.Services[0]
	assert.Equal(t, "billing-worker.service", svc.Name)
	assert.Equal(t, "Billing Worker Service", svc.Description)
	assert.Equal(t, "/usr/local/bin/billing-worker", svc.Binary)
	assert.Contains(t, svc.Configs, "/etc/billing/worker.env")
	assert.False(t, svc.Ingest)
	require.NotNil(t, svc.BinaryInfo)
	assert.NotEmpty(t, svc.BinaryInfo.SHA256)

	// Test 2: Save and Load Plan
	planPath := filepath.Join(tmpDir, "discovery-plan.yaml")
	err = discover.SavePlan(plan, planPath)
	require.NoError(t, err)

	loadedPlan, err := discover.LoadPlan(planPath)
	require.NoError(t, err)
	assert.Equal(t, plan.Version, loadedPlan.Version)
	assert.Len(t, loadedPlan.Targets.Services, len(plan.Targets.Services))
}

func TestGenerate_ModeB_And_ModeA(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	binDir := filepath.Join(tmpDir, "usr", "local", "bin")
	require.NoError(t, os.MkdirAll(binDir, 0750))
	mockBinPath := filepath.Join(binDir, "billing-worker")
	require.NoError(t, os.WriteFile(mockBinPath, []byte("dummy-binary-payload"), 0600))

	confDir := filepath.Join(tmpDir, "etc", "billing")
	require.NoError(t, os.MkdirAll(confDir, 0750))
	mockConfPath := filepath.Join(confDir, "worker.env")
	confContent := `PORT=8080
DB_PASSWORD=supersecret
PRIVATE_KEY="-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC...
-----END PRIVATE KEY-----"
`
	require.NoError(t, os.WriteFile(mockConfPath, []byte(confContent), 0600))

	plan := &discover.DiscoveryPlan{
		Version: "1.0",
		Targets: discover.PlanTargets{
			Services: []discover.PlanService{
				{
					Name:        "billing-worker.service",
					Binary:      "/usr/local/bin/billing-worker",
					Configs:     []string{"/etc/billing/worker.env"},
					Ingest:      false, // Mode B
					Description: "Billing Service",
					BinaryInfo: &discover.BinaryMeta{
						Size:   20,
						SHA256: "dummy-sha256",
					},
				},
			},
		},
	}

	planPath := filepath.Join(tmpDir, "discovery-plan.yaml")
	require.NoError(t, discover.SavePlan(plan, planPath))

	// Case 1: Mode B (Default: TODO hint)
	manifestPathB := filepath.Join(tmpDir, "manifest-mode-b.yaml")
	artifactsDirB := filepath.Join(tmpDir, "artifacts-b")
	configsDirB := filepath.Join(tmpDir, "configs-b")

	mB, err := discover.GenerateManifest(discover.GenerateOptions{
		PlanPath:     planPath,
		OutputPath:   manifestPathB,
		ArtifactsDir: artifactsDirB,
		ConfigsDir:   configsDirB,
		Sysroot:      tmpDir,
	})
	require.NoError(t, err)
	require.NotNil(t, mB)

	require.Len(t, mB.Applications, 1)
	assert.Contains(t, mB.Applications[0].Artifact, "TODO:")
	assert.Equal(t, "/usr/local/bin/billing-worker", mB.Applications[0].Destination)

	// Verify SIRT Secret Sanitization
	require.Len(t, mB.Configs, 1)
	generatedConfData, err := os.ReadFile(mB.Configs[0].Template)
	require.NoError(t, err)
	assert.NotContains(t, string(generatedConfData), "supersecret")
	assert.Contains(t, string(generatedConfData), "<REDACTED_SECRET>")

	// Case 2: Mode A (Ingest Override)
	manifestPathA := filepath.Join(tmpDir, "manifest-mode-a.yaml")
	artifactsDirA := filepath.Join(tmpDir, "artifacts-a")
	configsDirA := filepath.Join(tmpDir, "configs-a")

	mA, err := discover.GenerateManifest(discover.GenerateOptions{
		PlanPath:       planPath,
		OutputPath:     manifestPathA,
		ArtifactsDir:   artifactsDirA,
		ConfigsDir:     configsDirA,
		IngestOverride: true,
		Sysroot:        tmpDir,
	})
	require.NoError(t, err)
	require.NotNil(t, mA)

	require.Len(t, mA.Applications, 1)
	assert.True(t, strings.HasPrefix(mA.Applications[0].Artifact, artifactsDirA))
	// Ensure binary was physically copied
	copiedBin := filepath.Join(artifactsDirA, "billing-worker")
	assert.FileExists(t, copiedBin)
	copiedData, err := os.ReadFile(filepath.Clean(copiedBin)) //nolint:gosec // test verification
	require.NoError(t, err)
	assert.Equal(t, "dummy-binary-payload", string(copiedData))

	// Verify SaveManifest
	err = discover.SaveManifest(mA, manifestPathA)
	require.NoError(t, err)
	assert.FileExists(t, manifestPathA)
}
