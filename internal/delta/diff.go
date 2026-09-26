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

package delta

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
	"strings"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/bundle"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
	"gopkg.in/yaml.v3"
)

// Change types for files in a delta comparison.
const (
	ChangeAdded     = "added"
	ChangeModified  = "modified"
	ChangeDeleted   = "deleted"
	ChangeUnchanged = "unchanged"
)

// FileDiff records the differential status of an individual asset between releases.
type FileDiff struct {
	Path       string `json:"path" yaml:"path"`
	ChangeType string `json:"change_type" yaml:"changeType"`
	OldHash    string `json:"old_hash,omitempty" yaml:"oldHash,omitempty"`
	NewHash    string `json:"new_hash,omitempty" yaml:"newHash,omitempty"`
	OldSize    int64  `json:"old_size,omitempty" yaml:"oldSize,omitempty"`
	NewSize    int64  `json:"new_size,omitempty" yaml:"newSize,omitempty"`
}

// DiffReport details the differences and data transfer savings between base and target releases.
type DiffReport struct {
	BaseRelease      string     `json:"base_release" yaml:"baseRelease"`
	BaseChecksum     string     `json:"base_checksum" yaml:"baseChecksum"`
	TargetRelease    string     `json:"target_release" yaml:"targetRelease"`
	Files            []FileDiff `json:"files" yaml:"files"`
	FullSizeBytes    int64      `json:"full_size_bytes" yaml:"fullSizeBytes"`
	DeltaSizeBytes   int64      `json:"delta_size_bytes" yaml:"deltaSizeBytes"`
	ReductionPercent float64    `json:"reduction_percent" yaml:"reductionPercent"`
	AddedCount       int        `json:"added_count" yaml:"addedCount"`
	ModifiedCount    int        `json:"modified_count" yaml:"modifiedCount"`
	DeletedCount     int        `json:"deleted_count" yaml:"deletedCount"`
	UnchangedCount   int        `json:"unchanged_count" yaml:"unchangedCount"`
}

// BuildDeltaOptions specifies parameters to create a delta deployment bundle.
type BuildDeltaOptions struct {
	BaseBundlePath string
	ManifestPath   string
	SourceDir      string
	OutputFile     string
	ReportPath     string
	PrivateKey     ed25519.PrivateKey
}

// ComputeDiff compares an extracted base bundle and target bundle files, computing diff metrics.
func ComputeDiff(baseBundlePath, targetBundlePath string) (*DiffReport, error) {
	baseFiles, baseManifest, baseChecksum, err := readBundleArchive(baseBundlePath)
	if err != nil {
		return nil, fmt.Errorf("failed reading base bundle: %w", err)
	}

	targetFiles, targetManifest, _, err := readBundleArchive(targetBundlePath)
	if err != nil {
		return nil, fmt.Errorf("failed reading target bundle: %w", err)
	}

	return generateDiffReport(baseManifest.Release, baseChecksum, targetManifest.Release, baseFiles, targetFiles)
}

// BuildDeltaBundle produces a delta .tar.gz bundle packaging only added or modified files.
func BuildDeltaBundle(opts BuildDeltaOptions) (*DiffReport, error) {
	// 1. Verify and read base bundle
	baseFiles, baseManifest, baseChecksum, err := readBundleArchive(opts.BaseBundlePath)
	if err != nil {
		return nil, fmt.Errorf("failed inspecting base bundle: %w", err)
	}

	// 2. Parse target manifest
	targetM, err := manifest.ParseFile(opts.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed parsing target manifest: %w", err)
	}

	// 3. Scan target files
	cleanSrc := filepath.Clean(opts.SourceDir)
	targetFiles := make(map[string][]byte)

	err = filepath.Walk(cleanSrc, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() {
			return walkErr
		}
		rel, relErr := filepath.Rel(cleanSrc, path)
		if relErr != nil {
			return relErr
		}
		relSlash := filepath.ToSlash(rel)
		if strings.HasPrefix(relSlash, ".git") || strings.HasSuffix(relSlash, ".tar.gz") {
			return nil
		}
		data, readErr := os.ReadFile(filepath.Clean(path)) //nolint:gosec
		if readErr != nil {
			return readErr
		}
		targetFiles[relSlash] = data
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed scanning target source directory: %w", err)
	}

	// 4. Compute differences
	report, err := generateDiffReport(baseManifest.Release, baseChecksum, targetM.Release, baseFiles, targetFiles)
	if err != nil {
		return nil, err
	}

	// 5. Build delta manifest
	deltaM := *targetM
	deltaM.BundleType = "delta"
	deltaM.BaseRelease = baseManifest.Release
	deltaM.BaseReleaseChecksum = baseChecksum
	deltaM.TargetRelease = targetM.Release

	deltaManifestBytes, err := yaml.Marshal(deltaM)
	if err != nil {
		return nil, fmt.Errorf("failed marshaling delta manifest: %w", err)
	}

	// 6. Gather files to package in delta archive (only added, modified, plus delta manifest)
	archiveFiles := make(map[string][]byte)
	archiveFiles["manifest.yaml"] = deltaManifestBytes

	var checksumLines []string
	checksumLines = append(checksumLines, fmt.Sprintf("%s  manifest.yaml", hexHash(deltaManifestBytes)))

	for _, fd := range report.Files {
		if fd.ChangeType == ChangeAdded || fd.ChangeType == ChangeModified {
			if fd.Path == "manifest.yaml" {
				continue
			}
			content, ok := targetFiles[fd.Path]
			if !ok {
				continue
			}
			archiveFiles[fd.Path] = content
			checksumLines = append(checksumLines, fmt.Sprintf("%s  %s", fd.NewHash, fd.Path))
		}
	}

	sort.Strings(checksumLines)
	checksumBytes := []byte(strings.Join(checksumLines, "\n") + "\n")
	archiveFiles["checksums"] = checksumBytes

	// Sign manifest
	privKey := opts.PrivateKey
	if privKey == nil {
		var genErr error
		_, privKey, genErr = bundle.GenerateKeyPair()
		if genErr != nil {
			return nil, fmt.Errorf("failed generating ephemeral signing key: %w", genErr)
		}
	}

	pubKey := privKey.Public().(ed25519.PublicKey)
	sig := bundle.SignData(privKey, deltaManifestBytes)
	archiveFiles["signatures/manifest.sig"] = sig
	archiveFiles["signatures/public.key"] = []byte(hex.EncodeToString(pubKey) + "\n")

	// 7. Write delta archive
	cleanOut := filepath.Clean(opts.OutputFile)
	if mkdirErr := os.MkdirAll(filepath.Dir(cleanOut), 0750); mkdirErr != nil {
		return nil, fmt.Errorf("failed creating output dir: %w", mkdirErr)
	}

	outF, err := os.Create(cleanOut)
	if err != nil {
		return nil, fmt.Errorf("failed creating output delta bundle file: %w", err)
	}
	defer func() { _ = outF.Close() }()

	gw := gzip.NewWriter(outF)
	tw := tar.NewWriter(gw)

	for p, content := range archiveFiles {
		hdr := &tar.Header{
			Name:    filepath.ToSlash(p),
			Mode:    0644,
			Size:    int64(len(content)),
			ModTime: time.Now().UTC(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			_ = tw.Close()
			_ = gw.Close()
			return nil, fmt.Errorf("failed writing tar header for %s: %w", p, err)
		}
		if _, err := tw.Write(content); err != nil {
			_ = tw.Close()
			_ = gw.Close()
			return nil, fmt.Errorf("failed writing tar body for %s: %w", p, err)
		}
	}

	if err := tw.Close(); err != nil {
		_ = gw.Close()
		return nil, fmt.Errorf("failed closing tar writer: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("failed closing gzip writer: %w", err)
	}

	// Update delta size in report
	fi, statErr := os.Stat(cleanOut)
	if statErr == nil {
		report.DeltaSizeBytes = fi.Size()
		if report.FullSizeBytes > 0 {
			saved := report.FullSizeBytes - report.DeltaSizeBytes
			if saved > 0 {
				report.ReductionPercent = float64(saved) / float64(report.FullSizeBytes) * 100.0
			}
		}
	}

	// Write report if requested
	if opts.ReportPath != "" {
		reportBytes, err := yaml.Marshal(report)
		if err == nil {
			_ = os.WriteFile(filepath.Clean(opts.ReportPath), reportBytes, 0640) //nolint:gosec
		}
	}

	return report, nil
}

func readBundleArchive(path string) (map[string][]byte, *manifest.Manifest, string, error) {
	cleanPath := filepath.Clean(path)
	f, err := os.Open(cleanPath)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed opening bundle: %w", err)
	}
	defer func() { _ = f.Close() }()

	hasher := sha256.New()
	tee := io.TeeReader(f, hasher)

	gr, err := gzip.NewReader(tee)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed creating gzip reader: %w", err)
	}
	defer func() { _ = gr.Close() }()

	tr := tar.NewReader(gr)
	files := make(map[string][]byte)

	for {
		hdr, nextErr := tr.Next()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return nil, nil, "", fmt.Errorf("failed reading tar: %w", nextErr)
		}
		if hdr.Typeflag == tar.TypeReg {
			data, readErr := io.ReadAll(tr)
			if readErr != nil {
				return nil, nil, "", fmt.Errorf("failed reading entry %s: %w", hdr.Name, readErr)
			}
			files[filepath.ToSlash(hdr.Name)] = data
		}
	}

	manifestData, ok := files["manifest.yaml"]
	if !ok {
		return nil, nil, "", errors.New("archive is missing manifest.yaml")
	}

	m, err := manifest.Parse(manifestData)
	if err != nil {
		return nil, nil, "", fmt.Errorf("invalid manifest.yaml in archive: %w", err)
	}

	bundleChecksum := hex.EncodeToString(hasher.Sum(nil))
	return files, m, bundleChecksum, nil
}

func generateDiffReport(baseRel, baseChecksum, targetRel string, baseFiles, targetFiles map[string][]byte) (*DiffReport, error) {
	report := &DiffReport{
		BaseRelease:   baseRel,
		BaseChecksum:  baseChecksum,
		TargetRelease: targetRel,
		Files:         make([]FileDiff, 0),
	}

	allPathsMap := make(map[string]struct{})
	for p := range baseFiles {
		if !strings.HasPrefix(p, "signatures/") && p != "checksums" && p != "manifest.yaml" {
			allPathsMap[p] = struct{}{}
		}
	}
	for p := range targetFiles {
		if !strings.HasPrefix(p, "signatures/") && p != "checksums" && p != "manifest.yaml" {
			allPathsMap[p] = struct{}{}
		}
	}

	var allPaths []string
	for p := range allPathsMap {
		allPaths = append(allPaths, p)
	}
	sort.Strings(allPaths)

	for _, p := range allPaths {
		oldData, inOld := baseFiles[p]
		newData, inNew := targetFiles[p]

		switch {
		case inOld && !inNew:
			h := hexHash(oldData)
			report.Files = append(report.Files, FileDiff{
				Path:       p,
				ChangeType: ChangeDeleted,
				OldHash:    h,
				OldSize:    int64(len(oldData)),
			})
			report.DeletedCount++
		case !inOld && inNew:
			h := hexHash(newData)
			report.Files = append(report.Files, FileDiff{
				Path:       p,
				ChangeType: ChangeAdded,
				NewHash:    h,
				NewSize:    int64(len(newData)),
			})
			report.FullSizeBytes += int64(len(newData))
			report.AddedCount++
		default:
			oldH := hexHash(oldData)
			newH := hexHash(newData)
			report.FullSizeBytes += int64(len(newData))
			if oldH == newH {
				report.Files = append(report.Files, FileDiff{
					Path:       p,
					ChangeType: ChangeUnchanged,
					OldHash:    oldH,
					NewHash:    newH,
					OldSize:    int64(len(oldData)),
					NewSize:    int64(len(newData)),
				})
				report.UnchangedCount++
			} else {
				report.Files = append(report.Files, FileDiff{
					Path:       p,
					ChangeType: ChangeModified,
					OldHash:    oldH,
					NewHash:    newH,
					OldSize:    int64(len(oldData)),
					NewSize:    int64(len(newData)),
				})
				report.ModifiedCount++
			}
		}
	}

	return report, nil
}

func hexHash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
