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

package agent

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

var (
	// ErrLocked is returned when the lock cannot be acquired because another instance is running.
	ErrLocked = errors.New("agent lock already held by another process")
)

// FileLock guards the agent execution against concurrent runs using flock.
type FileLock struct {
	path string
	file *os.File
}

// AcquireLock attempts to acquire an exclusive lock on lockPath.
// If another process holds the lock, it immediately returns ErrLocked without blocking.
func AcquireLock(lockPath string) (*FileLock, error) {
	cleanPath := filepath.Clean(lockPath)
	if err := os.MkdirAll(filepath.Dir(cleanPath), 0750); err != nil {
		return nil, fmt.Errorf("failed creating directory for lock file: %w", err)
	}

	f, err := os.OpenFile(cleanPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed opening lock file: %w", err)
	}

	// Try non-blocking exclusive flock
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, ErrLocked
		}
		return nil, fmt.Errorf("failed acquiring flock on %s: %w", cleanPath, err)
	}

	return &FileLock{
		path: cleanPath,
		file: f,
	}, nil
}

// Release unlocks and closes the lock file.
func (l *FileLock) Release() error {
	if l.file == nil {
		return nil
	}
	defer func() {
		_ = l.file.Close()
		l.file = nil
	}()

	return syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
}
