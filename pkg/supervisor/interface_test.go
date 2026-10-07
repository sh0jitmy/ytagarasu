package supervisor

import (
	"context"
	"testing"
	"time"
)

func TestProcessSupervisor_LifecycleAndRestart(t *testing.T) {
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
