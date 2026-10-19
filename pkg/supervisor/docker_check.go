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
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CheckDockerDaemonReachable performs preflight validation to ensure Docker daemon is accessible.
func CheckDockerDaemonReachable(ctx context.Context) error {
	dockerHost := os.Getenv("DOCKER_HOST")

	if strings.HasPrefix(dockerHost, "unix://") {
		socketPath := strings.TrimPrefix(dockerHost, "unix://")
		cleanSocket := filepath.Clean(socketPath)
		// #nosec G703 -- socket file status inspection for preflight reachability
		if _, err := os.Stat(cleanSocket); err != nil {
			return fmt.Errorf("docker daemon is unreachable: unix socket '%s' does not exist or is inaccessible: %w", cleanSocket, err)
		}
	} else if strings.HasPrefix(dockerHost, "tcp://") {
		addr := strings.TrimPrefix(dockerHost, "tcp://")
		d := net.Dialer{Timeout: 1 * time.Second}
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return fmt.Errorf("docker daemon is unreachable: tcp endpoint '%s' unreachable: %w", addr, err)
		}
		_ = conn.Close()
	}

	// Execute fast lightweight preflight probe via docker version
	probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, "docker", "version", "--format", "{{.Server.Version}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker daemon is unreachable or not responsive: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	return nil
}
