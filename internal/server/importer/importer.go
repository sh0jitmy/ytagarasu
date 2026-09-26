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

package importer

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shjtmy/go_sh0jitmy_template/internal/bundle"
	"github.com/shjtmy/go_sh0jitmy_template/internal/server/store"
)

// Result contains metadata of a successfully imported release bundle.
type Result struct {
	ReleaseID      string    `json:"release_id"`
	ServiceID      string    `json:"service_id"`
	ReleaseVersion string    `json:"release_version"`
	FileCount      int       `json:"file_count"`
	TotalBytes     int64     `json:"total_bytes"`
	Artifacts      []string  `json:"artifacts"`
	ImportedAt     time.Time `json:"imported_at"`
}

// Importer coordinates bundle verification, CAS storage, and SQLite release registration.
type Importer struct {
	cas            *store.CAS
	db             *store.DB
	namespacesRoot string
}

// NewImporter constructs an Importer with the given CAS, DB, and base namespaces directory.
func NewImporter(cas *store.CAS, db *store.DB, namespacesRoot string) (*Importer, error) {
	cleanNS := filepath.Clean(namespacesRoot)
	if err := os.MkdirAll(cleanNS, 0750); err != nil {
		return nil, fmt.Errorf("failed to create namespaces directory: %w", err)
	}
	return &Importer{
		cas:            cas,
		db:             db,
		namespacesRoot: cleanNS,
	}, nil
}

// Import reads a bundle archive from r, cryptographically verifies it,
// deduplicates artifacts into CAS, projects them into the service's virtual namespace,
// and activates the release in the SQLite database.
func (imp *Importer) Import(ctx context.Context, r io.Reader, expectedPubKeyHex string) (*Result, error) {
	// Write to temporary file for inspection and verification
	tmpDir := filepath.Join(imp.namespacesRoot, ".tmp")
	if err := os.MkdirAll(tmpDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create importer temp dir: %w", err)
	}

	tmpFile, err := os.CreateTemp(tmpDir, "import-bundle-*.tar.gz")
	if err != nil {
		return nil, fmt.Errorf("failed creating temporary file for import: %w", err)
	}
	tmpBundlePath := tmpFile.Name()
	defer func() {
		_ = os.Remove(tmpBundlePath)
	}()

	if _, copyErr := io.Copy(tmpFile, r); copyErr != nil {
		_ = tmpFile.Close()
		return nil, fmt.Errorf("failed writing bundle stream to temp file: %w", copyErr)
	}
	if closeErr := tmpFile.Close(); closeErr != nil {
		return nil, fmt.Errorf("failed closing temp bundle file: %w", closeErr)
	}

	// 1. Strict Cryptographic Verification
	var expectedPubKey ed25519.PublicKey
	if expectedPubKeyHex != "" {
		decoded, decErr := hex.DecodeString(strings.TrimSpace(expectedPubKeyHex))
		if decErr != nil {
			return nil, fmt.Errorf("invalid public key hex: %w", decErr)
		}
		expectedPubKey = ed25519.PublicKey(decoded)
	}

	verifyRes, err := bundle.VerifyBundle(tmpBundlePath, expectedPubKey)
	if err != nil {
		return nil, fmt.Errorf("bundle verification failed: %w", err)
	}
	if !verifyRes.Valid {
		errMsg := "verification failed"
		if len(verifyRes.Errors) > 0 {
			errMsg = strings.Join(verifyRes.Errors, "; ")
		}
		return nil, fmt.Errorf("bundle signature or checksum verification rejected: %s", errMsg)
	}

	// 2. Parse Manifest & Validate
	manifestData, ok := verifyRes.Files["manifest.yaml"]
	if !ok {
		return nil, errors.New("bundle does not contain manifest.yaml")
	}

	m := verifyRes.Manifest
	if m == nil {
		return nil, errors.New("parsed manifest missing from verification result")
	}

	serviceID := ""
	if len(m.Applications) > 0 && m.Applications[0].Name != "" {
		serviceID = m.Applications[0].Name
	} else if len(m.Services) > 0 && m.Services[0].Name != "" {
		serviceID = m.Services[0].Name
	}
	if serviceID == "" {
		serviceID = "default"
	}
	releaseVersion := m.Release
	if releaseVersion == "" {
		releaseVersion = m.Version
	}
	releaseID := fmt.Sprintf("%s-%s-%d", serviceID, releaseVersion, time.Now().Unix())

	// 3. Ingest files into CAS & Prepare Namespace
	serviceNamespaceDir := filepath.Join(imp.namespacesRoot, serviceID)
	if err := os.MkdirAll(serviceNamespaceDir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create service namespace directory: %w", err)
	}

	releaseArtifacts := make([]store.ReleaseArtifact, 0, len(verifyRes.Files))
	artifactPaths := make([]string, 0, len(verifyRes.Files))
	var totalBytes int64

	for relPath, content := range verifyRes.Files {
		hash, size, putErr := imp.cas.Put(bytes.NewReader(content))
		if putErr != nil {
			return nil, fmt.Errorf("failed storing file %s into CAS: %w", relPath, putErr)
		}

		totalBytes += size

		// Project into service namespace
		destPath := filepath.Join(serviceNamespaceDir, filepath.FromSlash(relPath))
		if linkErr := imp.cas.Link(hash, destPath); linkErr != nil {
			return nil, fmt.Errorf("failed linking blob to namespace for %s: %w", relPath, linkErr)
		}

		artID := fmt.Sprintf("%s-%s", releaseID, hash[:12])
		releaseArtifacts = append(releaseArtifacts, store.ReleaseArtifact{
			ID:         artID,
			ReleaseID:  releaseID,
			RelPath:    relPath,
			SHA256Hash: hash,
			SizeBytes:  size,
			CreatedAt:  time.Now().UTC(),
		})
		artifactPaths = append(artifactPaths, relPath)
	}

	// 4. Register Release in SQLite Database
	sigData := verifyRes.Files["signatures/manifest.sig"]
	pubKeyData := verifyRes.Files["signatures/public.key"]

	rel := &store.Release{
		ID:             releaseID,
		ServiceID:      serviceID,
		ReleaseVersion: releaseVersion,
		Status:         "active",
		ManifestYAML:   string(manifestData),
		Signature:      string(bytes.TrimSpace(sigData)),
		PublicKey:      string(bytes.TrimSpace(pubKeyData)),
		CreatedAt:      time.Now().UTC(),
	}

	if err := imp.db.RegisterRelease(ctx, rel, releaseArtifacts); err != nil {
		return nil, fmt.Errorf("failed registering release in database: %w", err)
	}

	return &Result{
		ReleaseID:      releaseID,
		ServiceID:      serviceID,
		ReleaseVersion: releaseVersion,
		FileCount:      len(releaseArtifacts),
		TotalBytes:     totalBytes,
		Artifacts:      artifactPaths,
		ImportedAt:     time.Now().UTC(),
	}, nil
}
