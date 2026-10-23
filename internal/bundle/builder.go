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
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sh0jitmy/ytagarasu/internal/fsutil"
	"github.com/sh0jitmy/ytagarasu/internal/manifest"
)

var (
	ErrBundleBlocked = errors.New("bundle generation blocked due to critical risk or missing dependencies")
)

// BuildOptions parameters for building a deployment bundle.
type BuildOptions struct {
	ManifestPath       string
	SourceDir          string
	OutputFile         string
	ReportOutputFile   string
	PrivateKey         ed25519.PrivateKey
	TargetPlatform     manifest.Target
	AllowWarningStatus bool
	SplitBytes         int64
	SplitOutputDir     string
}

// BuildBundle validates, risk-evaluates, signs, and packages an offline deployment bundle archive.
func BuildBundle(opts BuildOptions) (*manifest.PreparationResult, error) {
	if opts.SourceDir == "" {
		opts.SourceDir = "."
	}
	cleanSourceDir := filepath.Clean(opts.SourceDir)

	manifestFile := opts.ManifestPath
	if manifestFile == "" {
		manifestFile = filepath.Join(cleanSourceDir, "manifest.yaml")
	}

	// 1. Parse and validate manifest
	m, err := manifest.ParseFile(manifestFile)
	if err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}

	// Target matching
	target := opts.TargetPlatform
	if target.OS == "" && len(m.Targets) > 0 {
		target = m.Targets[0]
	}

	// 2. Perform risk assessment and dependency verification
	evalResult, err := manifest.EvaluateBundle(m, cleanSourceDir, target)
	if err != nil {
		return nil, fmt.Errorf("evaluation failed: %w", err)
	}

	// Save preparation report if requested
	if opts.ReportOutputFile != "" {
		_ = manifest.SavePreparationResult(evalResult, opts.ReportOutputFile)
	}

	// Blocked check
	if evalResult.Status == manifest.StatusBlocked {
		return evalResult, fmt.Errorf("%w: %d missing items, %d critical risks",
			ErrBundleBlocked, len(evalResult.Missing), len(evalResult.Risks))
	}

	if evalResult.Status == manifest.StatusWarning && !opts.AllowWarningStatus {
		return evalResult, fmt.Errorf("bundle has warnings (use --force or allow-warnings to proceed): %d risks",
			len(evalResult.Risks))
	}

	// 3. Setup signing keys
	privKey := opts.PrivateKey
	var pubKey ed25519.PublicKey
	if privKey == nil {
		var keyErr error
		pubKey, privKey, keyErr = GenerateKeyPair()
		if keyErr != nil {
			return nil, fmt.Errorf("failed to generate signing key: %w", keyErr)
		}
	} else {
		pubKey = privKey.Public().(ed25519.PublicKey)
	}

	// 4. Calculate checksums and sign manifest
	manifestBytes, err := os.ReadFile(filepath.Clean(manifestFile))
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest for signing: %w", err)
	}
	signature := SignData(privKey, manifestBytes)

	// Auto-export container images if defined in manifest and not yet saved
	if exportErr := autoExportContainers(cleanSourceDir, m.Containers); exportErr != nil {
		return nil, fmt.Errorf("failed auto-exporting container images: %w", exportErr)
	}

	// Create temporary staging directory for bundle contents
	stageDir, err := os.MkdirTemp("", "ytagarasu-bundle-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create staging dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(stageDir) }()

	// Write signatures directory
	sigDir := filepath.Join(stageDir, "signatures")
	if dirErr := os.MkdirAll(sigDir, 0750); dirErr != nil {
		return nil, fmt.Errorf("failed to create signatures dir: %w", dirErr)
	}
	if sigErr := os.WriteFile(filepath.Clean(filepath.Join(sigDir, "manifest.sig")), signature, 0600); sigErr != nil { //nolint:gosec // path inside temporary staging directory
		return nil, fmt.Errorf("failed to write signature: %w", sigErr)
	}
	if keyErr := os.WriteFile(filepath.Clean(filepath.Join(sigDir, "public.key")), []byte(hex.EncodeToString(pubKey)), 0640); keyErr != nil { //nolint:gosec // path inside temporary staging directory
		return nil, fmt.Errorf("failed to write public key: %w", keyErr)
	}

	// Create tar.gz archive
	if opts.OutputFile == "" {
		opts.OutputFile = fmt.Sprintf("bundle-%s-%s.tar.gz", m.Release, target.OS)
	}
	cleanOutPath := filepath.Clean(opts.OutputFile)
	if outDirErr := os.MkdirAll(filepath.Dir(cleanOutPath), 0750); outDirErr != nil {
		return nil, fmt.Errorf("failed to create destination dir: %w", outDirErr)
	}

	outFile, err := os.OpenFile(cleanOutPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0640) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("failed to open output bundle archive: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	gw := gzip.NewWriter(outFile)
	defer func() { _ = gw.Close() }()

	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	// Map of path to sha256 checksums
	checksumLines := make([]string, 0)

	// Add files from sourceDir
	err = filepath.Walk(cleanSourceDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relPath, relErr := filepath.Rel(cleanSourceDir, path)
		if relErr != nil || relPath == "." {
			return nil
		}

		// Skip hidden git directories or the output bundle itself
		if strings.HasPrefix(relPath, ".git") || path == cleanOutPath {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Calculate checksum if it's a regular file
		if !info.IsDir() {
			hashStr, hashErr := fsutil.FileSHA256(path)
			if hashErr == nil {
				checksumLines = append(checksumLines, fmt.Sprintf("%s  %s", hashStr, relPath))
			}
		}

		return addFileToTar(tw, path, relPath, info)
	})
	if err != nil {
		return nil, fmt.Errorf("failed adding source files to bundle: %w", err)
	}

	// Add signatures and checksums
	sigRel := filepath.Join("signatures", "manifest.sig")
	sigPath := filepath.Join(sigDir, "manifest.sig")
	sigInfo, _ := os.Stat(sigPath)
	if sigTarErr := addFileToTar(tw, sigPath, sigRel, sigInfo); sigTarErr != nil {
		return nil, sigTarErr
	}
	sigHash, _ := fsutil.FileSHA256(sigPath)
	checksumLines = append(checksumLines, fmt.Sprintf("%s  %s", sigHash, sigRel))

	pubRel := filepath.Join("signatures", "public.key")
	pubPath := filepath.Join(sigDir, "public.key")
	pubInfo, _ := os.Stat(pubPath)
	if pubTarErr := addFileToTar(tw, pubPath, pubRel, pubInfo); pubTarErr != nil {
		return nil, pubTarErr
	}
	pubHash, _ := fsutil.FileSHA256(pubPath)
	checksumLines = append(checksumLines, fmt.Sprintf("%s  %s", pubHash, pubRel))

	// Add checksums file to archive
	checksumContent := []byte(strings.Join(checksumLines, "\n") + "\n")
	checksumHeader := &tar.Header{
		Name:     "checksums",
		Mode:     0640,
		Size:     int64(len(checksumContent)),
		Typeflag: tar.TypeReg,
	}
	if chkHdrErr := tw.WriteHeader(checksumHeader); chkHdrErr != nil {
		return nil, fmt.Errorf("failed writing checksums tar header: %w", chkHdrErr)
	}
	if _, chkWriteErr := tw.Write(checksumContent); chkWriteErr != nil {
		return nil, fmt.Errorf("failed writing checksums to tar: %w", chkWriteErr)
	}

	// If split is requested, flush and split the generated archive into chunks
	if opts.SplitBytes > 0 {
		if closeErr := tw.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed closing tar writer before split: %w", closeErr)
		}
		if closeErr := gw.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed closing gzip writer before split: %w", closeErr)
		}
		if closeErr := outFile.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed closing output file before split: %w", closeErr)
		}

		splitDir := opts.SplitOutputDir
		if splitDir == "" {
			splitDir = filepath.Dir(cleanOutPath)
		}
		baseName := filepath.Base(cleanOutPath)
		if _, splitErr := SplitFile(cleanOutPath, opts.SplitBytes, splitDir, baseName); splitErr != nil {
			return nil, fmt.Errorf("failed splitting bundle archive: %w", splitErr)
		}
	}

	return evalResult, nil
}

func addFileToTar(tw *tar.Writer, srcPath, relPath string, info os.FileInfo) error {
	header, err := tar.FileInfoHeader(info, info.Name())
	if err != nil {
		return fmt.Errorf("failed creating tar header: %w", err)
	}
	header.Name = filepath.ToSlash(relPath)

	if hdrErr := tw.WriteHeader(header); hdrErr != nil {
		return fmt.Errorf("failed writing tar header for %s: %w", relPath, hdrErr)
	}

	if info.IsDir() {
		return nil
	}

	f, openErr := os.Open(filepath.Clean(srcPath))
	if openErr != nil {
		return fmt.Errorf("failed opening file %s: %w", srcPath, openErr)
	}
	defer func() { _ = f.Close() }()

	_, copyErr := io.Copy(tw, f)
	return copyErr
}

// autoExportContainers inspects declarative containers in the manifest,
// and automatically invokes `docker save` for any images that do not yet have an offline tar archive.
func autoExportContainers(sourceDir string, containers []manifest.ContainerSpec) error {
	if len(containers) == 0 {
		return nil
	}

	for _, ctr := range containers {
		if ctr.Image == "" {
			continue
		}
		archiveRel := ctr.ImageArchive
		if archiveRel == "" {
			archiveRel = filepath.Join("images", fmt.Sprintf("%s.tar", ctr.Name))
		}
		archivePath := filepath.Join(sourceDir, archiveRel)
		cleanPath := filepath.Clean(archivePath)

		// If archive already exists, skip docker save
		if _, err := os.Stat(cleanPath); err == nil {
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(cleanPath), 0750); err != nil {
			return fmt.Errorf("failed creating directory for container image %s: %w", ctr.Name, err)
		}

		// Check if docker CLI is available on staging host
		if _, err := exec.LookPath("docker"); err != nil {
			// Docker not available, proceed without auto-save
			continue
		}

		// Execute docker save <image> -o <cleanPath>
		//nolint:gosec // Safe execution of docker save with cleaned destination path
		cmd := exec.Command("docker", "save", ctr.Image, "-o", cleanPath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("auto-exporting container image %s (%s) failed: %w (output: %s)",
				ctr.Name, ctr.Image, err, string(out))
		}
	}
	return nil
}
