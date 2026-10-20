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

package supervisor

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var validNetworkNameRegexp = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

// SanitizeDockerNetworkName normalizes a string into a valid Docker network identifier.
func SanitizeDockerNetworkName(name string) string {
	cleaned := validNetworkNameRegexp.ReplaceAllString(strings.TrimSpace(name), "-")
	cleaned = strings.Trim(cleaned, "-._")
	if cleaned == "" {
		return "ytg-default-net"
	}
	return cleaned
}

// DockerNetworkCommander abstracts docker network CLI operations for testability.
type DockerNetworkCommander interface {
	Inspect(ctx context.Context, name string) (bool, error)
	Create(ctx context.Context, name string) error
	Remove(ctx context.Context, name string) error
}

type defaultDockerNetworkCommander struct{}

func (d *defaultDockerNetworkCommander) Inspect(ctx context.Context, name string) (bool, error) {
	//nolint:gosec // Safe execution of docker inspect with validated network name
	cmd := exec.CommandContext(ctx, "docker", "network", "inspect", name)
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() != 0 {
			// Network does not exist
			return false, nil
		}
		return false, fmt.Errorf("failed to inspect docker network %s: %w", name, err)
	}
	return true, nil
}

func (d *defaultDockerNetworkCommander) Create(ctx context.Context, name string) error {
	//nolint:gosec // Safe execution of docker create with sanitized network name
	cmd := exec.CommandContext(ctx, "docker", "network", "create", "--driver", "bridge", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create docker bridge network %s: %w (output: %s)", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (d *defaultDockerNetworkCommander) Remove(ctx context.Context, name string) error {
	//nolint:gosec // Safe execution of docker rm with sanitized network name
	cmd := exec.CommandContext(ctx, "docker", "network", "rm", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to remove docker network %s: %w (output: %s)", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

var currentNetworkCommander DockerNetworkCommander = &defaultDockerNetworkCommander{}

// SetDockerNetworkCommanderForTest allows swapping the commander implementation during unit tests.
func SetDockerNetworkCommanderForTest(c DockerNetworkCommander) func() {
	prev := currentNetworkCommander
	currentNetworkCommander = c
	return func() {
		currentNetworkCommander = prev
	}
}

// EnsureDockerNetwork ensures a Docker bridge network exists, creating it if absent.
// Returns created=true if the network was created in this call.
func EnsureDockerNetwork(ctx context.Context, networkName string) (bool, error) {
	sanitized := SanitizeDockerNetworkName(networkName)
	if sanitized == "bridge" || sanitized == "host" || sanitized == "none" {
		// Built-in networks do not need creation
		return false, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	exists, err := currentNetworkCommander.Inspect(timeoutCtx, sanitized)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}

	createCtx, cancelCreate := context.WithTimeout(ctx, 5*time.Second)
	defer cancelCreate()

	if err := currentNetworkCommander.Create(createCtx, sanitized); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveDockerNetwork removes the specified Docker network.
func RemoveDockerNetwork(ctx context.Context, networkName string) error {
	sanitized := SanitizeDockerNetworkName(networkName)
	if sanitized == "bridge" || sanitized == "host" || sanitized == "none" {
		return nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return currentNetworkCommander.Remove(timeoutCtx, sanitized)
}
