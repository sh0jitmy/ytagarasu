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
	"testing"
	"time"
)

func TestProcessSupervisor_LifecycleAndRestart(t *testing.T) {
	t.Parallel()

	spec := ProcessConfig{
		Name:            "test-worker",
		Binary:          "sleep",
		Args:            []string{"1"},
		AutoRestart:     true,
		MaxRetries:      2,
		RollbackOnError: true,
	}

	sup := NewProcessSupervisor(spec)
	ctx := context.Background()

	// 1. Start process
	if err := sup.Start(ctx); err != nil {
		t.Fatalf("Failed to start supervisor: %v", err)
	}

	status := sup.Status()
	if status.PID == 0 {
		t.Errorf("Expected valid PID, got 0")
	}

	time.Sleep(100 * time.Millisecond)

	// 2. Restart process
	if err := sup.Restart(ctx); err != nil {
		t.Fatalf("Failed to restart process: %v", err)
	}

	// 3. Stop process
	if err := sup.Stop(1 * time.Second); err != nil {
		t.Fatalf("Failed to stop process: %v", err)
	}

	status = sup.Status()
	if status.State != StateStopped {
		t.Errorf("Expected StateStopped, got %v", status.State)
	}
}
