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

package bundle

import (
	"archive/tar"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/fsutil"

	"gopkg.in/yaml.v3"
)

var (
	// ErrMissingPart indicates one or more parts of a multi-part bundle are missing.
	ErrMissingPart = errors.New("multi-part bundle has missing parts")
	// ErrChecksumMismatch indicates a checksum mismatch in a multi-part bundle chunk.
	ErrChecksumMismatch = errors.New("checksum mismatch in multi-part bundle chunk")
	// ErrInvalidMultiPartManifest indicates the multi-part manifest is malformed or invalid.
	ErrInvalidMultiPartManifest = errors.New("invalid multi-part bundle manifest")
)

// MultiPartManifest describes a split bundle archive and its cryptographic integrity.
type MultiPartManifest struct {
	Version   string            `yaml:"version"`
	Kind      string            `yaml:"kind"`
	Metadata  MultiPartMetadata `yaml:"metadata"`
	Parts     MultiPartList     `yaml:"parts"`
	Signature *MultiPartSig     `yaml:"signature,omitempty"`
}

// MultiPartMetadata contains summary information about the un-split archive.
type MultiPartMetadata struct {
	BundleName    string    `yaml:"bundle_name"`
	ArchiveName   string    `yaml:"archive_name"`
	TotalBytes    int64     `yaml:"total_bytes"`
	ArchiveSHA256 string    `yaml:"archive_sha256"`
	CreatedAt     time.Time `yaml:"created_at"`
}

// MultiPartList contains the ordered collection of file chunks.
type MultiPartList struct {
	TotalParts int64      `yaml:"total_parts"`
	ChunkSize  int64      `yaml:"chunk_size_bytes"`
	Items      []PartItem `yaml:"items"`
}

// PartItem describes a single split chunk.
type PartItem struct {
	Index     int64  `yaml:"index"`
	FileName  string `yaml:"file_name"`
	SizeBytes int64  `yaml:"size_bytes"`
	SHA256    string `yaml:"sha256"`
}

// MultiPartSig stores digital signature data for the multi-part manifest.
type MultiPartSig struct {
	Algorithm            string `yaml:"algorithm"`
	PublicKeyFingerprint string `yaml:"public_key_fingerprint,omitempty"`
	SignatureValue       string `yaml:"signature_value"`
}

// ParseSize parses human-readable sizes like "25MB", "100KB", "1GB", or raw integer bytes.
func ParseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	if s == "" {
		return 0, errors.New("empty size string")
	}

	var multiplier int64
	var numStr string

	switch {
	case strings.HasSuffix(s, "GB") || strings.HasSuffix(s, "G"):
		multiplier = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(strings.TrimSuffix(s, "GB"), "G")
	case strings.HasSuffix(s, "MB") || strings.HasSuffix(s, "M"):
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(strings.TrimSuffix(s, "MB"), "M")
	case strings.HasSuffix(s, "KB") || strings.HasSuffix(s, "K"):
		multiplier = 1024
		numStr = strings.TrimSuffix(strings.TrimSuffix(s, "KB"), "K")
	case strings.HasSuffix(s, "B"):
		multiplier = 1
		numStr = strings.TrimSuffix(s, "B")
	default:
		multiplier = 1
		numStr = s
	}

	val, err := strconv.ParseInt(strings.TrimSpace(numStr), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size format %q: %w", s, err)
	}
	if val <= 0 {
		return 0, fmt.Errorf("size must be positive, got %d", val)
	}

	return val * multiplier, nil
}

// SplitFile splits a source archive file into chunks of chunkSizeBytes and writes a manifest.
func SplitFile(srcPath string, chunkSizeBytes int64, outDir string, baseName string) (*MultiPartManifest, error) {
	if chunkSizeBytes <= 0 {
		return nil, fmt.Errorf("invalid chunk size: %d", chunkSizeBytes)
	}

	srcClean := filepath.Clean(srcPath)
	srcFile, err := os.Open(srcClean)
	if err != nil {
		return nil, fmt.Errorf("failed to open source archive: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	stat, err := srcFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat source archive: %w", err)
	}
	totalBytes := stat.Size()
	if totalBytes == 0 {
		return nil, errors.New("cannot split empty file")
	}

	if baseName == "" {
		baseName = filepath.Base(srcClean)
	}

	outClean := filepath.Clean(outDir)
	if mkErr := os.MkdirAll(outClean, 0750); mkErr != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", mkErr)
	}

	// Calculate overall archive SHA-256 while splitting
	overallHasher := sha256.New()
	reader := io.TeeReader(srcFile, overallHasher)

	buf := make([]byte, 64*1024)
	var items []PartItem
	var partIndex int64 = 1

	for {
		partFileName := fmt.Sprintf("%s.part%03d", baseName, partIndex)
		partPath := filepath.Join(outClean, partFileName)

		partFile, createErr := os.OpenFile(filepath.Clean(partPath), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640) //nolint:gosec
		if createErr != nil {
			return nil, fmt.Errorf("failed to create part file %s: %w", partFileName, createErr)
		}

		partHasher := sha256.New()
		partWriter := io.MultiWriter(partFile, partHasher)

		var writtenForThisPart int64
		var reachedEOF bool

		for writtenForThisPart < chunkSizeBytes {
			toRead := chunkSizeBytes - writtenForThisPart
			if toRead > int64(len(buf)) {
				toRead = int64(len(buf))
			}

			n, rErr := reader.Read(buf[:toRead])
			if n > 0 {
				if _, wErr := partWriter.Write(buf[:n]); wErr != nil {
					_ = partFile.Close()
					return nil, fmt.Errorf("failed writing to %s: %w", partFileName, wErr)
				}
				writtenForThisPart += int64(n)
			}

			if errors.Is(rErr, io.EOF) {
				reachedEOF = true
				break
			}
			if rErr != nil {
				_ = partFile.Close()
				return nil, fmt.Errorf("read error while splitting: %w", rErr)
			}
		}

		_ = partFile.Close()

		if writtenForThisPart == 0 {
			_ = os.Remove(partPath)
			break
		}

		items = append(items, PartItem{
			Index:     partIndex,
			FileName:  partFileName,
			SizeBytes: writtenForThisPart,
			SHA256:    hex.EncodeToString(partHasher.Sum(nil)),
		})

		if reachedEOF {
			break
		}
		partIndex++
	}

	manifest := &MultiPartManifest{
		Version: "1.0",
		Kind:    "MultiPartBundleManifest",
		Metadata: MultiPartMetadata{
			BundleName:    strings.TrimSuffix(baseName, filepath.Ext(baseName)),
			ArchiveName:   baseName,
			TotalBytes:    totalBytes,
			ArchiveSHA256: hex.EncodeToString(overallHasher.Sum(nil)),
			CreatedAt:     time.Now().UTC(),
		},
		Parts: MultiPartList{
			TotalParts: int64(len(items)),
			ChunkSize:  chunkSizeBytes,
			Items:      items,
		},
	}

	// Write bundle.manifest.yaml in output directory
	manifestPath := filepath.Join(outClean, "bundle.manifest.yaml")
	manifestBytes, err := yaml.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal multi-part manifest: %w", err)
	}

	if err := os.WriteFile(filepath.Clean(manifestPath), manifestBytes, 0640); err != nil { //nolint:gosec
		return nil, fmt.Errorf("failed to write bundle.manifest.yaml: %w", err)
	}

	return manifest, nil
}

// FindManifestFile resolves the bundle.manifest.yaml path given either a file or directory path.
func FindManifestFile(path string) (string, error) {
	clean := filepath.Clean(path)
	info, err := os.Stat(clean)
	if err != nil {
		return "", err
	}

	if info.IsDir() {
		candidate := filepath.Join(clean, "bundle.manifest.yaml")
		if _, cErr := os.Stat(candidate); cErr == nil {
			return candidate, nil
		}
		// Also try *.manifest.yaml
		matches, mErr := filepath.Glob(filepath.Join(clean, "*.manifest.yaml"))
		if mErr == nil && len(matches) > 0 {
			return matches[0], nil
		}
		return "", fmt.Errorf("no bundle.manifest.yaml found in directory %s", path)
	}

	return clean, nil
}

// ValidateMultiPartManifest verifies that all parts defined in the manifest exist,
// have matching file sizes, and have matching SHA-256 checksums.
func ValidateMultiPartManifest(manifestPathOrDir string) (*MultiPartManifest, string, error) {
	manifestPath, err := FindManifestFile(manifestPathOrDir)
	if err != nil {
		return nil, "", fmt.Errorf("failed locating multi-part manifest: %w", err)
	}

	data, err := os.ReadFile(filepath.Clean(manifestPath))
	if err != nil {
		return nil, "", fmt.Errorf("failed reading multi-part manifest: %w", err)
	}

	var m MultiPartManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrInvalidMultiPartManifest, err)
	}

	if m.Kind != "MultiPartBundleManifest" && m.Parts.TotalParts == 0 {
		return nil, "", fmt.Errorf("%w: kind is %q and total_parts is 0", ErrInvalidMultiPartManifest, m.Kind)
	}

	baseDir := filepath.Dir(manifestPath)

	// Check if part count matches items
	if int64(len(m.Parts.Items)) != m.Parts.TotalParts {
		return nil, "", fmt.Errorf("%w: total_parts declared as %d but contains %d item definitions",
			ErrInvalidMultiPartManifest, m.Parts.TotalParts, len(m.Parts.Items))
	}

	// Validate presence, size, and checksum of each part
	var missingParts []string
	var corruptedParts []string

	for _, item := range m.Parts.Items {
		partPath := filepath.Join(baseDir, item.FileName)
		pInfo, pErr := os.Stat(partPath)
		if pErr != nil {
			missingParts = append(missingParts, fmt.Sprintf("Part %d (%s, expected %d bytes)", item.Index, item.FileName, item.SizeBytes))
			continue
		}

		if pInfo.Size() != item.SizeBytes {
			corruptedParts = append(corruptedParts, fmt.Sprintf("Part %d (%s: size mismatch, expected %d, got %d)",
				item.Index, item.FileName, item.SizeBytes, pInfo.Size()))
			continue
		}

		// Verify SHA-256
		hash, hErr := fsutil.FileSHA256(partPath)
		if hErr != nil {
			corruptedParts = append(corruptedParts, fmt.Sprintf("Part %d (%s: failed calculating hash: %v)", item.Index, item.FileName, hErr))
			continue
		}

		if !strings.EqualFold(hash, item.SHA256) {
			corruptedParts = append(corruptedParts, fmt.Sprintf("Part %d (%s: checksum mismatch, expected %s, got %s)",
				item.Index, item.FileName, item.SHA256, hash))
		}
	}

	if len(missingParts) > 0 {
		return &m, baseDir, fmt.Errorf("%w: %d missing parts detected:\n  - %s",
			ErrMissingPart, len(missingParts), strings.Join(missingParts, "\n  - "))
	}

	if len(corruptedParts) > 0 {
		return &m, baseDir, fmt.Errorf("%w: %d corrupted parts detected:\n  - %s",
			ErrChecksumMismatch, len(corruptedParts), strings.Join(corruptedParts, "\n  - "))
	}

	return &m, baseDir, nil
}

// multiReadCloser sequences multiple ReadClosers and closes each when exhausted or on Close.
type multiReadCloser struct {
	openFiles []*os.File
	reader    io.Reader
}

func (m *multiReadCloser) Read(p []byte) (n int, err error) {
	return m.reader.Read(p)
}

func (m *multiReadCloser) Close() error {
	var firstErr error
	for _, f := range m.openFiles {
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// OpenMultiPartReader returns an io.ReadCloser that streams through all chunks seamlessly,
// without assembling an intermediate file on disk (zero temporary disk waste).
func OpenMultiPartReader(manifestPathOrDir string) (io.ReadCloser, *MultiPartManifest, error) {
	m, baseDir, err := ValidateMultiPartManifest(manifestPathOrDir)
	if err != nil {
		return nil, nil, err
	}

	// Sort items by index to guarantee in-order streaming
	sortedItems := make([]PartItem, len(m.Parts.Items))
	copy(sortedItems, m.Parts.Items)
	sort.Slice(sortedItems, func(i, j int) bool {
		return sortedItems[i].Index < sortedItems[j].Index
	})

	openFiles := make([]*os.File, 0, len(sortedItems))
	readers := make([]io.Reader, 0, len(sortedItems))

	for _, item := range sortedItems {
		partPath := filepath.Join(baseDir, item.FileName)
		f, oErr := os.Open(filepath.Clean(partPath))
		if oErr != nil {
			// Clean up already opened files
			for _, opened := range openFiles {
				_ = opened.Close()
			}
			return nil, nil, fmt.Errorf("failed opening part %s: %w", item.FileName, oErr)
		}
		openFiles = append(openFiles, f)
		readers = append(readers, f)
	}

	mrc := &multiReadCloser{
		openFiles: openFiles,
		reader:    io.MultiReader(readers...),
	}

	return mrc, m, nil
}

// AssembleMultiPart reads chunks via streaming and writes out a single consolidated archive file.
func AssembleMultiPart(manifestPathOrDir string, outPath string) error {
	stream, m, err := OpenMultiPartReader(manifestPathOrDir)
	if err != nil {
		return fmt.Errorf("preflight validation failed: %w", err)
	}
	defer func() { _ = stream.Close() }()

	cleanOut := filepath.Clean(outPath)
	if mkErr := os.MkdirAll(filepath.Dir(cleanOut), 0750); mkErr != nil {
		return fmt.Errorf("failed creating output directory: %w", mkErr)
	}

	outFile, err := os.OpenFile(cleanOut, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed opening output archive: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	hasher := sha256.New()
	writer := io.MultiWriter(outFile, hasher)

	copiedBytes, err := io.Copy(writer, stream)
	if err != nil {
		return fmt.Errorf("failed copying chunks to output archive: %w", err)
	}

	if copiedBytes != m.Metadata.TotalBytes {
		return fmt.Errorf("assembled size mismatch: expected %d bytes, got %d", m.Metadata.TotalBytes, copiedBytes)
	}

	calcHash := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(calcHash, m.Metadata.ArchiveSHA256) {
		return fmt.Errorf("assembled checksum mismatch: expected %s, got %s", m.Metadata.ArchiveSHA256, calcHash)
	}

	return nil
}

// VerifyMultiPartBundle verifies in-memory the full contents, embedded signatures,
// and manifest integrity of a multi-part bundle without creating intermediate disk files.
func VerifyMultiPartBundle(manifestPathOrDir string, expectedPubKey ed25519.PublicKey) (*VerificationResult, error) {
	stream, m, err := OpenMultiPartReader(manifestPathOrDir)
	if err != nil {
		return nil, fmt.Errorf("multi-part bundle validation failed: %w", err)
	}
	defer func() { _ = stream.Close() }()

	overallHasher := sha256.New()
	teeStream := io.TeeReader(stream, overallHasher)

	gr, err := gzip.NewReader(teeStream)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid gzip stream: %v", ErrCorruptBundle, err)
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)

	files := make(map[string][]byte)
	for {
		header, tErr := tr.Next()
		if errors.Is(tErr, io.EOF) {
			break
		}
		if tErr != nil {
			return nil, fmt.Errorf("%w: tar read error: %v", ErrCorruptBundle, tErr)
		}

		cleanName := filepath.Clean(header.Name)
		if strings.HasPrefix(cleanName, "..") || strings.HasPrefix(cleanName, "/") {
			return nil, fmt.Errorf("%w: insecure path detected in tar: %s", ErrCorruptBundle, header.Name)
		}

		if header.Typeflag == tar.TypeReg {
			content, readErr := io.ReadAll(tr)
			if readErr != nil {
				return nil, fmt.Errorf("%w: failed to read file %s: %v", ErrCorruptBundle, header.Name, readErr)
			}
			files[header.Name] = content
		}
	}

	calcHash := hex.EncodeToString(overallHasher.Sum(nil))
	if !strings.EqualFold(calcHash, m.Metadata.ArchiveSHA256) {
		return nil, fmt.Errorf("overall archive checksum mismatch: expected %s, got %s", m.Metadata.ArchiveSHA256, calcHash)
	}

	// Now inspect internal signatures and manifest from in-memory files map
	res := &VerificationResult{
		Valid:     true,
		FileCount: len(files),
		Files:     files,
	}

	// Verify SHA256SUMS if present
	if sumBytes, ok := files["SHA256SUMS"]; ok {
		scanner := strings.Split(string(sumBytes), "\n")
		for _, line := range scanner {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") {
				continue
			}
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				expectedHash := parts[0]
				relPath := parts[1]
				content, found := files[relPath]
				if !found {
					res.Valid = false
					res.Errors = append(res.Errors, fmt.Sprintf("missing file declared in SHA256SUMS: %s", relPath))
					continue
				}
				actualHash := fmt.Sprintf("%x", sha256.Sum256(content))
				if !strings.EqualFold(actualHash, expectedHash) {
					res.Valid = false
					res.Errors = append(res.Errors, fmt.Sprintf("hash mismatch for %s: expected %s, got %s", relPath, expectedHash, actualHash))
				}
			}
		}
		res.ChecksumsValid = len(res.Errors) == 0
	}

	// Verify signatures/manifest.sig if present
	manifestBytes, hasManifest := files["manifest.yaml"]
	sigBytes, hasSig := files["signatures/manifest.sig"]

	if hasManifest && hasSig {
		pubKey := expectedPubKey
		if pubKey == nil {
			if keyBytes, hasKey := files["signatures/public.key"]; hasKey {
				keyHex := strings.TrimSpace(string(keyBytes))
				decoded, dErr := hex.DecodeString(keyHex)
				if dErr == nil && len(decoded) == ed25519.PublicKeySize {
					pubKey = ed25519.PublicKey(decoded)
				}
			}
		}

		if pubKey != nil {
			if ed25519.Verify(pubKey, manifestBytes, sigBytes) {
				res.SignatureValid = true
			} else {
				res.Valid = false
				res.SignatureValid = false
				res.Errors = append(res.Errors, "ed25519 signature verification failed for manifest.yaml")
			}
		}
	}

	return res, nil
}
