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

package configmgr

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/manifest"
)

var (
	// ErrHealthCheckFailed indicates a health check probe failed all retries.
	ErrHealthCheckFailed = errors.New("health check verification failed")
)

// HealthChecker executes health check probes with retries.
type HealthChecker struct {
	client *http.Client
}

// NewHealthChecker constructs a HealthChecker.
func NewHealthChecker(client *http.Client) *HealthChecker {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	return &HealthChecker{client: client}
}

// Check probes the specified health check endpoint/command until passing or exceeding max retries.
func (h *HealthChecker) Check(ctx context.Context, hc manifest.HealthCheck) error {
	retries := hc.MaxRetries
	if retries <= 0 {
		retries = 1
	}

	interval := time.Duration(hc.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}

	var lastErr error
	for i := 0; i < retries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		switch hc.Type {
		case manifest.HealthCheckHTTP:
			lastErr = h.checkHTTP(ctx, hc)
		case manifest.HealthCheckCommand:
			lastErr = h.checkCommand(ctx, hc)
		default:
			return fmt.Errorf("unsupported health check type: %s", hc.Type)
		}

		if lastErr == nil {
			return nil
		}

		if i < retries-1 {
			time.Sleep(interval)
		}
	}

	return fmt.Errorf("%w: %v", ErrHealthCheckFailed, lastErr)
}

func (h *HealthChecker) checkHTTP(ctx context.Context, hc manifest.HealthCheck) error {
	expectedStatus := hc.ExpectedStatus
	if expectedStatus <= 0 {
		expectedStatus = http.StatusOK
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hc.Endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != expectedStatus {
		return fmt.Errorf("expected HTTP %d but got %d", expectedStatus, resp.StatusCode)
	}

	return nil
}

func (h *HealthChecker) checkCommand(ctx context.Context, hc manifest.HealthCheck) error {
	parts := strings.Fields(hc.Command)
	if len(parts) == 0 {
		return errors.New("empty healthcheck command")
	}

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...) //nolint:gosec // Command from trusted manifest
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command '%s' exited with error: %v (output: %s)", hc.Command, err, string(output))
	}
	return nil
}
