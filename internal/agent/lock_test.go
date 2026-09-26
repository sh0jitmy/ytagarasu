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

package agent_test

import (
	"path/filepath"
	"testing"

	"github.com/sh0jitmy/ytagarasu/internal/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileLock_ExclusiveAndRelease(t *testing.T) {
	t.Parallel()

	workDir := t.TempDir()
	lockPath := filepath.Join(workDir, "agent.lock")

	// 1. Acquire initial lock
	lock1, err := agent.AcquireLock(lockPath)
	require.NoError(t, err)
	require.NotNil(t, lock1)

	// 2. Second attempt must fail with ErrLocked
	lock2, err := agent.AcquireLock(lockPath)
	require.ErrorIs(t, err, agent.ErrLocked)
	assert.Nil(t, lock2)

	// 3. Release first lock
	err = lock1.Release()
	require.NoError(t, err)

	// 4. Re-acquire should now succeed
	lock3, err := agent.AcquireLock(lockPath)
	require.NoError(t, err)
	require.NotNil(t, lock3)
	_ = lock3.Release()
}
