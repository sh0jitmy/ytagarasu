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

package pkgengine

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// RPMRepository represents an in-memory index of parsed RPM / DNF packages.
type RPMRepository struct {
	PackagesByName map[string][]PackageMetadata
	PackagesByCap  map[string][]PackageMetadata // capability -> packages providing it
	AllPackages    []PackageMetadata
}

// NewRPMRepository creates an initialized RPM repository index.
func NewRPMRepository() *RPMRepository {
	return &RPMRepository{
		PackagesByName: make(map[string][]PackageMetadata),
		PackagesByCap:  make(map[string][]PackageMetadata),
		AllPackages:    make([]PackageMetadata, 0),
	}
}

// XML structures for primary.xml
type rpmEntry struct {
	Name  string `xml:"name,attr"`
	Flags string `xml:"flags,attr"`
	Ver   string `xml:"ver,attr"`
	Rel   string `xml:"rel,attr"`
}

type rpmFormat struct {
	Provides []rpmEntry `xml:"provides>entry"`
	Requires []rpmEntry `xml:"requires>entry"`
}

type rpmPackageXML struct {
	Name     string   `xml:"name"`
	Arch     string   `xml:"arch"`
	Version  rpmEntry `xml:"version"`
	Checksum string   `xml:"checksum"`
	Size     struct {
		Package int64 `xml:"package,attr"`
	} `xml:"size"`
	Location struct {
		Href string `xml:"href,attr"`
	} `xml:"location"`
	Format rpmFormat `xml:"format"`
}

type rpmMetadataXML struct {
	XMLName  xml.Name        `xml:"metadata"`
	Packages []rpmPackageXML `xml:"package"`
}

// ParseRPMPrimary parses raw or gzipped primary.xml metadata.
func ParseRPMPrimary(data []byte) (*RPMRepository, error) {
	var reader io.Reader = bytes.NewReader(data)

	// Check for gzip magic header
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to open gzip reader: %w", err)
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	}

	var meta rpmMetadataXML
	decoder := xml.NewDecoder(reader)
	if err := decoder.Decode(&meta); err != nil {
		return nil, fmt.Errorf("failed to decode RPM primary.xml: %w", err)
	}

	repo := NewRPMRepository()
	for _, p := range meta.Packages {
		versionStr := p.Version.Ver
		if p.Version.Rel != "" {
			versionStr = fmt.Sprintf("%s-%s", p.Version.Ver, p.Version.Rel)
		}

		pkg := PackageMetadata{
			Name:         p.Name,
			Version:      versionStr,
			Architecture: p.Arch,
			Filename:     p.Location.Href,
			SHA256:       p.Checksum,
			Size:         p.Size.Package,
			PackageType:  PackageTypeRPM,
		}

		// Parse provides / capabilities
		for _, prov := range p.Format.Provides {
			if prov.Name != "" {
				pkg.Provides = append(pkg.Provides, prov.Name)
			}
		}

		// Parse requires (filter out internal rpmlib dependencies)
		for _, req := range p.Format.Requires {
			if req.Name == "" || strings.HasPrefix(req.Name, "rpmlib(") {
				continue
			}
			cond := &DependencyCondition{
				Operator: req.Flags,
				Version:  req.Ver,
			}
			pkg.Depends = append(pkg.Depends, Dependency{
				Name:      req.Name,
				Condition: cond,
			})
		}

		repo.addPackage(pkg)
	}

	return repo, nil
}

func (r *RPMRepository) addPackage(pkg PackageMetadata) {
	r.PackagesByName[pkg.Name] = append(r.PackagesByName[pkg.Name], pkg)
	r.AllPackages = append(r.AllPackages, pkg)
	for _, prov := range pkg.Provides {
		r.PackagesByCap[prov] = append(r.PackagesByCap[prov], pkg)
	}
}

// ResolveDependencies recursively resolves required RPM packages using capabilities and names.
func (r *RPMRepository) ResolveDependencies(rootPkgNames []string) (*ResolutionResult, error) {
	resolvedMap := make(map[string]PackageMetadata) // key: package name
	visitedStack := make([]string, 0)

	for _, rootName := range rootPkgNames {
		cleanName := strings.TrimSpace(rootName)
		if cleanName == "" {
			continue
		}
		if err := r.resolveRecursive(cleanName, visitedStack, resolvedMap); err != nil {
			return nil, err
		}
	}

	result := &ResolutionResult{
		RootPackages: rootPkgNames,
		Resolved:     make([]PackageMetadata, 0, len(resolvedMap)),
		TargetOS:     "rhel/rocky",
	}

	for _, pkg := range resolvedMap {
		result.Resolved = append(result.Resolved, pkg)
		result.TotalSize += pkg.Size
	}

	return result, nil
}

func (r *RPMRepository) resolveRecursive(
	target string,
	chain []string,
	resolved map[string]PackageMetadata,
) error {
	// Find provider for target (by name or capability)
	pkg, found := r.findBestPackage(target)
	if !found {
		return &ResolutionError{
			PackageName: target,
			RequiredBy:  chain,
			Reason:      "RPM package or capability not found in repository index",
		}
	}

	// Already resolved
	if _, ok := resolved[pkg.Name]; ok {
		return nil
	}

	resolved[pkg.Name] = pkg

	nextChain := append(chain, pkg.Name)
	for _, dep := range pkg.Depends {
		if _, ok := resolved[dep.Name]; ok {
			continue
		}
		// Also check if any already resolved package provides this capability
		if r.isCapabilitySatisfied(dep.Name, resolved) {
			continue
		}
		if err := r.resolveRecursive(dep.Name, nextChain, resolved); err != nil {
			return err
		}
	}

	return nil
}

func (r *RPMRepository) findBestPackage(target string) (PackageMetadata, bool) {
	// 1. Exact name match
	if candidates, ok := r.PackagesByName[target]; ok && len(candidates) > 0 {
		return candidates[len(candidates)-1], true
	}
	// 2. Capability match
	if candidates, ok := r.PackagesByCap[target]; ok && len(candidates) > 0 {
		return candidates[len(candidates)-1], true
	}
	return PackageMetadata{}, false
}

func (r *RPMRepository) isCapabilitySatisfied(capName string, resolved map[string]PackageMetadata) bool {
	for _, pkg := range resolved {
		for _, p := range pkg.Provides {
			if p == capName {
				return true
			}
		}
	}
	return false
}
