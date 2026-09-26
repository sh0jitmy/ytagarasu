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

// Package pkgengine provides dependency resolution and artifact fetching engines
// for Debian/Ubuntu (APT) and RHEL (RPM/DNF) packages in online and air-gapped environments.
package pkgengine

import (
	"fmt"
	"strings"
)

// PackageType defines the OS package ecosystem.
type PackageType string

const (
	PackageTypeAPT PackageType = "apt"
	PackageTypeRPM PackageType = "rpm"
)

// DependencyCondition represents an operator and version constraint.
type DependencyCondition struct {
	Operator string `json:"operator,omitempty"` // =, >=, <=, >>, <<
	Version  string `json:"version,omitempty"`
}

// Dependency represents a required package constraint, supporting OR alternatives.
type Dependency struct {
	Name       string                `json:"name"`
	Condition  *DependencyCondition  `json:"condition,omitempty"`
	Alternates []DependencyCondition `json:"alternates,omitempty"` // For "a | b"
}

// PackageMetadata holds metadata extracted from Debian Packages or RPM primary.xml.
type PackageMetadata struct {
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Architecture string       `json:"architecture"`
	Filename     string       `json:"filename"` // Relative pool path or URL
	SHA256       string       `json:"sha256"`
	Size         int64        `json:"size"`
	Depends      []Dependency `json:"depends,omitempty"`
	Provides     []string     `json:"provides,omitempty"`
	PackageType  PackageType  `json:"packageType"`
}

// Key returns a canonical identifier string for the package (name_version_arch).
func (p *PackageMetadata) Key() string {
	return fmt.Sprintf("%s_%s_%s", p.Name, p.Version, p.Architecture)
}

// ResolutionResult holds the resolved package tree and statistics.
type ResolutionResult struct {
	RootPackages []string          `json:"rootPackages"`
	Resolved     []PackageMetadata `json:"resolved"`
	TotalSize    int64             `json:"totalSize"`
	TargetOS     string            `json:"targetOS"`
}

// ResolutionError records detailed information when package resolution fails.
type ResolutionError struct {
	PackageName string   `json:"packageName"`
	RequiredBy  []string `json:"requiredBy"`
	Reason      string   `json:"reason"`
}

func (e *ResolutionError) Error() string {
	if len(e.RequiredBy) > 0 {
		return fmt.Sprintf("failed to resolve package '%s' (required by: %s): %s",
			e.PackageName, strings.Join(e.RequiredBy, " -> "), e.Reason)
	}
	return fmt.Sprintf("failed to resolve package '%s': %s", e.PackageName, e.Reason)
}
