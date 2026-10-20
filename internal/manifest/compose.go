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

package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ComposeServiceSpec represents a single service definition in docker-compose.yml.
type ComposeServiceSpec struct {
	Image       string    `yaml:"image"`
	Ports       yaml.Node `yaml:"ports,omitempty"`
	Volumes     yaml.Node `yaml:"volumes,omitempty"`
	Environment yaml.Node `yaml:"environment,omitempty"`
	Restart     string    `yaml:"restart,omitempty"`
	Networks    yaml.Node `yaml:"networks,omitempty"`
	Command     yaml.Node `yaml:"command,omitempty"`
	DependsOn   yaml.Node `yaml:"depends_on,omitempty"`
}

// ComposeFileSpec represents the top-level docker-compose file schema.
type ComposeFileSpec struct {
	Version  string                        `yaml:"version,omitempty"`
	Services map[string]ComposeServiceSpec `yaml:"services"`
}

// ConvertComposeToManifest converts docker-compose YAML definitions into a ytagarasu Manifest.
func ConvertComposeToManifest(composeYAML []byte, releaseName string) (*Manifest, error) {
	var composeSpec ComposeFileSpec
	if err := yaml.Unmarshal(composeYAML, &composeSpec); err != nil {
		return nil, fmt.Errorf("failed to parse docker-compose YAML: %w", err)
	}

	if len(composeSpec.Services) == 0 {
		return nil, fmt.Errorf("docker-compose file does not contain any services")
	}

	if releaseName == "" {
		releaseName = "compose-migration"
	}

	containers := make([]ContainerSpec, 0, len(composeSpec.Services))

	for name, svc := range composeSpec.Services {
		if svc.Image == "" {
			return nil, fmt.Errorf("service '%s' missing required 'image' attribute", name)
		}

		portsList := parseComposeStringList(svc.Ports)
		volumesList := parseComposeStringList(svc.Volumes)
		dependsList := parseComposeDependsOn(svc.DependsOn)
		envMap := parseComposeEnvironment(svc.Environment)
		cmdList := parseComposeCommand(svc.Command)
		netName := parseComposeNetwork(svc.Networks)

		autoRestart := true
		if svc.Restart == "no" {
			autoRestart = false
		}

		archivePath := filepath.ToSlash(filepath.Join("images", fmt.Sprintf("%s.tar", name)))

		ctr := ContainerSpec{
			Name:          name,
			Image:         svc.Image,
			ImageArchive:  archivePath,
			Ports:         portsList,
			Volumes:       volumesList,
			Environment:   envMap,
			Network:       netName,
			RestartPolicy: svc.Restart,
			AutoRestart:   &autoRestart,
			MaxRetries:    5,
			Command:       cmdList,
			DependsOn:     dependsList,
			LogPath:       fmt.Sprintf("./logs/%s-container.log", name),
			LogMaxSizeMB:  50,
			LogMaxBackups: 5,
			LogCompress:   true,
		}
		containers = append(containers, ctr)
	}

	m := &Manifest{
		Version:       "1.0",
		BundleVersion: "1.0.0",
		Release:       releaseName,
		CreatedAt:     time.Now().UTC(),
		Targets: []Target{
			{OS: "linux", Release: "ubuntu-24.04", Arch: "amd64"},
		},
		Applications: []Application{},
		Containers:   containers,
		Rollback: RollbackPolicy{
			AutoOnFailure:             true,
			Strategy:                  RollbackImmediate,
			PreserveBackupGenerations: 3,
		},
	}

	return m, nil
}

// ConvertComposeFile reads a docker-compose.yml file and produces a ytagarasu Manifest.
func ConvertComposeFile(composePath string, releaseName string) (*Manifest, error) {
	cleanPath := filepath.Clean(composePath)
	data, err := os.ReadFile(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read compose file %s: %w", composePath, err)
	}
	return ConvertComposeToManifest(data, releaseName)
}

func parseComposeStringList(node yaml.Node) []string {
	var res []string
	switch node.Kind {
	case yaml.SequenceNode:
		for _, item := range node.Content {
			val := strings.TrimSpace(item.Value)
			if val != "" {
				res = append(res, val)
			}
		}
	case yaml.ScalarNode:
		val := strings.TrimSpace(node.Value)
		if val != "" {
			res = append(res, val)
		}
	}
	return res
}

func parseComposeDependsOn(node yaml.Node) []string {
	var res []string
	switch node.Kind {
	case yaml.SequenceNode:
		for _, item := range node.Content {
			val := strings.TrimSpace(item.Value)
			if val != "" {
				res = append(res, val)
			}
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			k := strings.TrimSpace(node.Content[i].Value)
			if k != "" {
				res = append(res, k)
			}
		}
	}
	return res
}

func parseComposeEnvironment(node yaml.Node) map[string]string {
	res := make(map[string]string)
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			k := node.Content[i].Value
			v := ""
			if i+1 < len(node.Content) {
				v = node.Content[i+1].Value
			}
			res[k] = v
		}
	case yaml.SequenceNode:
		for _, item := range node.Content {
			parts := strings.SplitN(item.Value, "=", 2)
			if len(parts) == 2 {
				res[parts[0]] = parts[1]
			} else if len(parts) == 1 {
				res[parts[0]] = ""
			}
		}
	}
	return res
}

func parseComposeCommand(node yaml.Node) []string {
	var res []string
	if node.Kind == yaml.ScalarNode && node.Value != "" {
		res = strings.Fields(node.Value)
	} else if node.Kind == yaml.SequenceNode {
		for _, item := range node.Content {
			res = append(res, item.Value)
		}
	}
	return res
}

func parseComposeNetwork(node yaml.Node) string {
	if node.Kind == yaml.ScalarNode {
		return node.Value
	} else if node.Kind == yaml.SequenceNode && len(node.Content) > 0 {
		return node.Content[0].Value
	} else if node.Kind == yaml.MappingNode && len(node.Content) > 0 {
		return node.Content[0].Value
	}
	return ""
}
