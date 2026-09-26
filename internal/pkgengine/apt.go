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
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ulikunitz/xz"
)

// APTRepository represents an in-memory index of parsed Debian/Ubuntu packages.
type APTRepository struct {
	PackagesByName map[string][]PackageMetadata
	PackagesByProv map[string][]PackageMetadata
	AllPackages    []PackageMetadata
}

// NewAPTRepository creates an initialized APT repository index.
func NewAPTRepository() *APTRepository {
	return &APTRepository{
		PackagesByName: make(map[string][]PackageMetadata),
		PackagesByProv: make(map[string][]PackageMetadata),
		AllPackages:    make([]PackageMetadata, 0),
	}
}

// ParsePackagesIndex parses raw, gzipped, or xz-compressed Debian Packages data.
func ParsePackagesIndex(data []byte) (*APTRepository, error) {
	var reader io.Reader = bytes.NewReader(data)

	// Check for gzip magic header (0x1f, 0x8b)
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to open gzip reader: %w", err)
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	} else if len(data) >= 6 && data[0] == 0xfd && string(data[1:6]) == "7zXZ\x00" {
		// Check for XZ magic header (0xfd7zXZ\x00)
		xzr, err := xz.NewReader(reader)
		if err != nil {
			return nil, fmt.Errorf("failed to open xz reader: %w", err)
		}
		reader = xzr
	}

	repo := NewAPTRepository()
	scanner := bufio.NewScanner(reader)
	// Allow large token buffer for extensive Packages metadata
	const maxCapacity = 10 * 1024 * 1024
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, maxCapacity)

	var currentLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			if len(currentLines) > 0 {
				pkg, ok := parseDebianControlBlock(currentLines)
				if ok {
					repo.addPackage(pkg)
				}
				currentLines = currentLines[:0]
			}
			continue
		}
		currentLines = append(currentLines, line)
	}

	if len(currentLines) > 0 {
		pkg, ok := parseDebianControlBlock(currentLines)
		if ok {
			repo.addPackage(pkg)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading packages stream: %w", err)
	}

	return repo, nil
}

func (r *APTRepository) addPackage(pkg PackageMetadata) {
	r.PackagesByName[pkg.Name] = append(r.PackagesByName[pkg.Name], pkg)
	r.AllPackages = append(r.AllPackages, pkg)
	for _, prov := range pkg.Provides {
		r.PackagesByProv[prov] = append(r.PackagesByProv[prov], pkg)
	}
}

// ResolveDependencies recursively discovers all required packages for given root targets.
func (r *APTRepository) ResolveDependencies(rootPkgNames []string) (*ResolutionResult, error) {
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
		TargetOS:     "debian/ubuntu",
	}

	for _, pkg := range resolvedMap {
		result.Resolved = append(result.Resolved, pkg)
		result.TotalSize += pkg.Size
	}

	return result, nil
}

func (r *APTRepository) resolveRecursive(
	pkgName string,
	chain []string,
	resolved map[string]PackageMetadata,
) error {
	// Already resolved
	if _, ok := resolved[pkgName]; ok {
		return nil
	}

	// Find matching package by name or provides
	pkg, found := r.findBestPackage(pkgName)
	if !found {
		return &ResolutionError{
			PackageName: pkgName,
			RequiredBy:  chain,
			Reason:      "package or virtual capability not found in repository mirror index",
		}
	}

	// Register resolved
	resolved[pkg.Name] = pkg

	// Process dependencies
	nextChain := append(chain, pkg.Name)
	for _, dep := range pkg.Depends {
		// If dependency is already satisfied by an already-resolved package, skip
		if _, ok := resolved[dep.Name]; ok {
			continue
		}
		if err := r.resolveRecursive(dep.Name, nextChain, resolved); err != nil {
			return err
		}
	}

	return nil
}

func (r *APTRepository) findBestPackage(name string) (PackageMetadata, bool) {
	// 1. Direct name lookup
	if candidates, ok := r.PackagesByName[name]; ok && len(candidates) > 0 {
		return candidates[len(candidates)-1], true // return latest
	}
	// 2. Virtual capability / Provides lookup
	if candidates, ok := r.PackagesByProv[name]; ok && len(candidates) > 0 {
		return candidates[len(candidates)-1], true
	}
	return PackageMetadata{}, false
}

func parseDebianControlBlock(lines []string) (PackageMetadata, bool) {
	pkg := PackageMetadata{PackageType: PackageTypeAPT}
	fields := make(map[string]string)

	var lastKey string
	for _, line := range lines {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if lastKey != "" {
				fields[lastKey] += "\n" + strings.TrimSpace(line)
			}
			continue
		}
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:colonIdx]))
		val := strings.TrimSpace(line[colonIdx+1:])
		fields[key] = val
		lastKey = key
	}

	pkg.Name = fields["package"]
	if pkg.Name == "" {
		return pkg, false
	}

	pkg.Version = fields["version"]
	pkg.Architecture = fields["architecture"]
	pkg.Filename = fields["filename"]
	pkg.SHA256 = fields["sha256"]

	if sizeStr := fields["size"]; sizeStr != "" {
		pkg.Size, _ = strconv.ParseInt(sizeStr, 10, 64)
	}

	// Parse Depends & Pre-Depends
	rawDepends := fields["depends"]
	if preDepends := fields["pre-depends"]; preDepends != "" {
		if rawDepends != "" {
			rawDepends += ", " + preDepends
		} else {
			rawDepends = preDepends
		}
	}

	pkg.Depends = parseDebianDependencies(rawDepends)

	// Parse Provides
	if rawProv := fields["provides"]; rawProv != "" {
		items := strings.Split(rawProv, ",")
		for _, item := range items {
			name := strings.TrimSpace(item)
			if idx := strings.Index(name, " "); idx != -1 {
				name = name[:idx]
			}
			if name != "" {
				pkg.Provides = append(pkg.Provides, name)
			}
		}
	}

	return pkg, true
}

func parseDebianDependencies(raw string) []Dependency {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	var result []Dependency
	clauses := strings.Split(raw, ",")
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}

		// Handle OR alternatives: "a | b"
		alts := strings.Split(clause, "|")
		first := strings.TrimSpace(alts[0])
		depName, cond := parseDebianSingleDep(first)
		if depName == "" {
			continue
		}

		dep := Dependency{
			Name:      depName,
			Condition: cond,
		}

		for _, alt := range alts[1:] {
			altName, altCond := parseDebianSingleDep(strings.TrimSpace(alt))
			if altName != "" && altCond != nil {
				dep.Alternates = append(dep.Alternates, *altCond)
			}
		}

		result = append(result, dep)
	}

	return result
}

func parseDebianSingleDep(s string) (string, *DependencyCondition) {
	s = strings.TrimSpace(s)
	// Example: "libssl3 (>= 3.0.0)"
	openParen := strings.Index(s, "(")
	if openParen == -1 {
		return s, nil
	}

	name := strings.TrimSpace(s[:openParen])
	closeParen := strings.Index(s, ")")
	if closeParen == -1 || closeParen <= openParen {
		return name, nil
	}

	constraint := strings.TrimSpace(s[openParen+1 : closeParen])
	parts := strings.Fields(constraint)
	if len(parts) >= 2 {
		return name, &DependencyCondition{
			Operator: parts[0],
			Version:  parts[1],
		}
	}

	return name, nil
}
