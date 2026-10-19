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
	"archive/tar"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrEmptyArchive indicates that a container image archive file is 0 bytes.
var ErrEmptyArchive = errors.New("container archive is empty (0 bytes)")

// ErrCorruptArchive indicates that a container image archive is corrupt or has an invalid tar header.
var ErrCorruptArchive = errors.New("container archive is corrupt or invalid")

// ValidateTarArchive performs preflight validation on an offline tar image archive
// before handing it over to docker load, guarding against incomplete or corrupted files.
func ValidateTarArchive(archivePath string) error {
	cleanPath := filepath.Clean(archivePath)
	stat, err := os.Stat(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to stat image archive %s: %w", archivePath, err)
	}

	if stat.Size() == 0 {
		return fmt.Errorf("%w: image archive %s is empty (0 bytes)", ErrEmptyArchive, archivePath)
	}

	f, err := os.Open(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to open image archive %s: %w", archivePath, err)
	}
	defer func() {
		_ = f.Close()
	}()

	tr := tar.NewReader(f)
	_, nextErr := tr.Next()
	if nextErr != nil {
		return fmt.Errorf("%w: corrupt or invalid tar header in %s: %v", ErrCorruptArchive, archivePath, nextErr)
	}

	return nil
}
