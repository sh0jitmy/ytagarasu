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
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type stagedItem struct {
	tempPath   string
	targetPath string
}

type backupItem struct {
	targetPath string
	backupPath string
}

// Transaction coordinates snapshot backups, temporary staging, validation,
// atomic replacement via renameat, and automatic rollback on failure.
type Transaction struct {
	backupDir string
	staged    []stagedItem
	backups   []backupItem
	committed bool
}

// NewTransaction initializes a Transaction saving snapshot backups to backupDir.
func NewTransaction(backupDir string) (*Transaction, error) {
	cleanDir := filepath.Clean(backupDir)
	if err := os.MkdirAll(cleanDir, 0750); err != nil {
		return nil, fmt.Errorf("failed creating backup directory: %w", err)
	}
	return &Transaction{
		backupDir: cleanDir,
		staged:    make([]stagedItem, 0),
		backups:   make([]backupItem, 0),
	}, nil
}

// StageFile prepares and validates a new configuration or artifact file.
// If the target file already exists, it is backed up first.
// If validation fails, the temporary file is removed and the target file remains completely untouched.
func (tx *Transaction) StageFile(ctx context.Context, targetPath string, content []byte, mode os.FileMode, validateCmd string) error {
	cleanTarget := filepath.Clean(targetPath)
	targetDir := filepath.Dir(cleanTarget)
	if err := os.MkdirAll(targetDir, 0750); err != nil {
		return fmt.Errorf("failed creating directory for %s: %w", cleanTarget, err)
	}

	// 1. Snapshot Current: backup existing file if it exists
	if _, err := os.Stat(cleanTarget); err == nil {
		backupFileName := fmt.Sprintf("%s-%d.bak", filepath.Base(cleanTarget), time.Now().UnixNano())
		backupPath := filepath.Join(tx.backupDir, backupFileName)
		if copyErr := copyFile(cleanTarget, backupPath); copyErr != nil {
			return fmt.Errorf("failed creating snapshot backup for %s: %w", cleanTarget, copyErr)
		}
		tx.backups = append(tx.backups, backupItem{
			targetPath: cleanTarget,
			backupPath: backupPath,
		})
	}

	// 2. Stage new content in a hidden temporary file in the same directory (ensures same filesystem for renameat)
	tmpFile, err := os.CreateTemp(targetDir, fmt.Sprintf(".%s.tmp.*", filepath.Base(cleanTarget)))
	if err != nil {
		return fmt.Errorf("failed creating temporary staging file: %w", err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(content); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed writing to staging file: %w", err)
	}
	if err := tmpFile.Chmod(mode); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed setting permissions on staging file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed syncing staging file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed closing staging file: %w", err)
	}

	// 3. Syntax Validation: execute validateCommand on temporary file
	if validateCmd != "" {
		if valErr := ValidateConfig(ctx, validateCmd, tmpPath); valErr != nil {
			_ = os.Remove(tmpPath) // Cleanup temporary file
			return fmt.Errorf("syntax validation rejected file: %w", valErr)
		}
	}

	tx.staged = append(tx.staged, stagedItem{
		tempPath:   tmpPath,
		targetPath: cleanTarget,
	})

	return nil
}

// Commit atomically applies all staged files to their final destinations via renameat.
func (tx *Transaction) Commit() error {
	for _, item := range tx.staged {
		if err := os.Rename(item.tempPath, item.targetPath); err != nil {
			// On rename failure, initiate immediate rollback
			_ = tx.Rollback()
			return fmt.Errorf("atomic rename failed for %s -> %s: %w", item.tempPath, item.targetPath, err)
		}
	}
	tx.committed = true
	return nil
}

// Rollback restores all backed-up files and cleans up any uncommitted staging files.
func (tx *Transaction) Rollback() error {
	var errs []error

	// If already committed, restore from snapshot backups
	if tx.committed {
		for _, b := range tx.backups {
			if err := copyFile(b.backupPath, b.targetPath); err != nil {
				errs = append(errs, fmt.Errorf("failed restoring backup %s -> %s: %w", b.backupPath, b.targetPath, err))
			}
		}
	} else {
		// If not yet committed, remove all temporary staged files
		for _, item := range tx.staged {
			_ = os.Remove(item.tempPath)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors during rollback: %v", errs)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(filepath.Clean(src)) //nolint:gosec // Trusted internal file copying
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(filepath.Clean(dst), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm()) //nolint:gosec
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
