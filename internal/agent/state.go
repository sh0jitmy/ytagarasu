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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// State tracks the host's currently applied release and runtime metadata.
type State struct {
	ServiceID        string    `json:"service_id"`
	CurrentReleaseID string    `json:"current_release_id"`
	CurrentVersion   string    `json:"current_version"`
	LastDeployedAt   time.Time `json:"last_deployed_at"`
	LastCheckedAt    time.Time `json:"last_checked_at"`
	Status           string    `json:"status"` // "synced", "deploying", "failed"
}

// StateStore handles reading and atomic writing of State.
type StateStore struct {
	filePath string
}

// NewStateStore creates a StateStore for the given file path.
func NewStateStore(filePath string) *StateStore {
	return &StateStore{filePath: filepath.Clean(filePath)}
}

// Load reads the State from disk. If the file does not exist, an empty State is returned.
func (s *StateStore) Load() (*State, error) {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{Status: "idle"}, nil
		}
		return nil, fmt.Errorf("failed reading state file: %w", err)
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed parsing state JSON: %w", err)
	}
	return &state, nil
}

// Save atomically writes State to disk using a temporary file and rename.
func (s *StateStore) Save(state *State) error {
	dir := filepath.Dir(s.filePath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed creating state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed marshaling state: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, "state-*.json.tmp")
	if err != nil {
		return fmt.Errorf("failed creating temp state file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed writing state to temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("failed syncing state temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed closing state temp file: %w", err)
	}

	if err := os.Rename(tmpPath, s.filePath); err != nil {
		return fmt.Errorf("failed renaming temp state file: %w", err)
	}

	return nil
}
