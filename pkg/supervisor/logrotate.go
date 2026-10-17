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
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultMaxLogSizeMB is default rotation threshold (50MB).
	DefaultMaxLogSizeMB = 50
	// DefaultMaxLogBackups is default number of rotated log generations retained.
	DefaultMaxLogBackups = 5
	// megabyte in bytes
	megabyte = 1024 * 1024
)

// RotatingLogConfig configures automatic zero-dependency log rotation.
type RotatingLogConfig struct {
	Filename   string
	MaxSizeMB  int
	MaxBackups int
	Compress   bool
}

// RotatingWriter implements io.WriteCloser with thread-safe file size tracking,
// automatic rotation, retention trimming, and optional gzip compression.
type RotatingWriter struct {
	mu         sync.Mutex
	filename   string
	maxSize    int64
	maxBackups int
	compress   bool
	size       int64
	file       *os.File
}

// NewRotatingWriter creates a configured RotatingWriter instance.
func NewRotatingWriter(cfg RotatingLogConfig) (*RotatingWriter, error) {
	if cfg.Filename == "" {
		return nil, fmt.Errorf("log filename cannot be empty")
	}

	maxSize := int64(cfg.MaxSizeMB) * megabyte
	if maxSize <= 0 {
		maxSize = int64(DefaultMaxLogSizeMB) * megabyte
	}

	maxBackups := cfg.MaxBackups
	if maxBackups <= 0 {
		maxBackups = DefaultMaxLogBackups
	}

	cleanFilename := filepath.Clean(cfg.Filename)
	dir := filepath.Dir(cleanFilename)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create log directory '%s': %w", dir, err)
	}

	w := &RotatingWriter{
		filename:   cleanFilename,
		maxSize:    maxSize,
		maxBackups: maxBackups,
		compress:   cfg.Compress,
	}

	return w, nil
}

// Write appends data to the log file, triggering rotation if the threshold is exceeded.
func (w *RotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	writeLen := int64(len(p))

	if w.file == nil {
		if err := w.openExistingOrNew(writeLen); err != nil {
			return 0, err
		}
	}

	if w.size > 0 && w.size+writeLen > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, err
		}
	}

	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// Sync commits written data to storage.
func (w *RotatingWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}
	return w.file.Sync()
}

// Close closes the active log file safely.
func (w *RotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.closeFile()
}

func (w *RotatingWriter) closeFile() error {
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// Rotate forces an immediate log file rotation.
func (w *RotatingWriter) Rotate() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.rotate()
}

func (w *RotatingWriter) openExistingOrNew(writeLen int64) error {
	info, err := os.Stat(w.filename)
	if os.IsNotExist(err) {
		return w.openNew()
	}
	if err != nil {
		return fmt.Errorf("failed to stat log file '%s': %w", w.filename, err)
	}

	if info.Size()+writeLen > w.maxSize {
		return w.rotate()
	}

	// #nosec G304 -- Log file path is sanitized with filepath.Clean during initialization
	file, err := os.OpenFile(w.filename, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return w.openNew()
	}

	w.file = file
	w.size = info.Size()
	return nil
}

func (w *RotatingWriter) openNew() error {
	dir := filepath.Dir(w.filename)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// #nosec G304 -- Log file path is sanitized with filepath.Clean during initialization
	file, err := os.OpenFile(w.filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to open new log file '%s': %w", w.filename, err)
	}

	w.file = file
	w.size = 0
	return nil
}

func (w *RotatingWriter) rotate() error {
	if err := w.closeFile(); err != nil {
		return fmt.Errorf("failed to close file before rotation: %w", err)
	}

	// Rename current file to timestamped archive name
	if _, err := os.Stat(w.filename); err == nil {
		timestamp := time.Now().Format("2006-01-02T15-04-05.000000")
		backupName := fmt.Sprintf("%s.%s", w.filename, timestamp)
		if err := os.Rename(w.filename, backupName); err != nil {
			return fmt.Errorf("failed to rename rotated log: %w", err)
		}

		if w.compress {
			go func(src string) {
				_ = compressLogFile(src)
			}(backupName)
		}
	}

	if err := w.openNew(); err != nil {
		return err
	}

	go w.cleanupOldBackups()

	return nil
}

func compressLogFile(src string) error {
	cleanSrc := filepath.Clean(src)
	// #nosec G304 -- Controlled archive file within supervisor log directory
	f, err := os.Open(cleanSrc)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
	}()

	dest := filepath.Clean(cleanSrc + ".gz")
	// #nosec G304 -- Controlled archive file within supervisor log directory
	gzFile, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer func() {
		_ = gzFile.Close()
	}()

	zw := gzip.NewWriter(gzFile)
	if _, err := io.Copy(zw, f); err != nil {
		_ = zw.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	_ = f.Close()
	return os.Remove(cleanSrc)
}

func (w *RotatingWriter) cleanupOldBackups() {
	dir := filepath.Dir(w.filename)
	base := filepath.Base(w.filename)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	type backupFile struct {
		path    string
		modTime time.Time
	}

	var backups []backupFile
	prefix := base + "."
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, prefix) && name != base {
			info, statErr := e.Info()
			if statErr != nil {
				continue
			}
			backups = append(backups, backupFile{
				path:    filepath.Join(dir, name),
				modTime: info.ModTime(),
			})
		}
	}

	if len(backups) <= w.maxBackups {
		return
	}

	// Sort oldest first
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].modTime.Before(backups[j].modTime)
	})

	toRemove := len(backups) - w.maxBackups
	for i := 0; i < toRemove; i++ {
		_ = os.Remove(backups[i].path)
	}
}
