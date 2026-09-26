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

// Package manifest defines the data structures, parser, validator, and generation
// utilities for Ytagarasu offline deployment manifests and bundle preparation results.
package manifest

import "time"

// ApplicationType defines supported application runtime stacks.
type ApplicationType string

const (
	AppTypeGolang ApplicationType = "golang"
	AppTypePython ApplicationType = "python"
	AppTypeReact  ApplicationType = "react"
	AppTypeHTMX   ApplicationType = "htmx"
)

// ProvisioningType defines certificate provisioning methods.
type ProvisioningType string

const (
	ProvisioningACME          ProvisioningType = "acme"
	ProvisioningEncryptedSOPS ProvisioningType = "encrypted_sops"
)

// ServiceAction defines systemd unit actions.
type ServiceAction string

const (
	ServiceActionRestart ServiceAction = "restart"
	ServiceActionReload  ServiceAction = "reload"
	ServiceActionStart   ServiceAction = "start"
	ServiceActionStop    ServiceAction = "stop"
)

// HealthCheckType defines probe mechanisms.
type HealthCheckType string

const (
	HealthCheckHTTP    HealthCheckType = "http"
	HealthCheckCommand HealthCheckType = "command"
	HealthCheckTCP     HealthCheckType = "tcp"
)

// RollbackStrategy defines rollback execution modes.
type RollbackStrategy string

const (
	RollbackImmediate RollbackStrategy = "immediate"
	RollbackManual    RollbackStrategy = "manual"
)

// PreparationStatus defines status of bundle preparation evaluation.
type PreparationStatus string

const (
	StatusReady   PreparationStatus = "ready"
	StatusWarning PreparationStatus = "warning"
	StatusBlocked PreparationStatus = "blocked"
)

// RiskSeverity defines risk severity levels.
type RiskSeverity string

const (
	SeverityCritical RiskSeverity = "critical"
	SeverityHigh     RiskSeverity = "high"
	SeverityMedium   RiskSeverity = "medium"
	SeverityLow      RiskSeverity = "low"
)

// Selector specifies target hosts by role and/or hostname.
type Selector struct {
	Roles     []string `json:"roles,omitempty" yaml:"roles,omitempty"`
	Hostnames []string `json:"hostnames,omitempty" yaml:"hostnames,omitempty"`
}

// Matches checks whether the given host matches the selector criteria.
func (s *Selector) Matches(roles []string, hostname string) bool {
	if s == nil || (len(s.Roles) == 0 && len(s.Hostnames) == 0) {
		return true // No selector means matching all hosts
	}

	// Wildcard check
	for _, r := range s.Roles {
		if r == "*" {
			return true
		}
	}

	// Hostname match
	if hostname != "" {
		for _, h := range s.Hostnames {
			if h == hostname {
				return true
			}
		}
	}

	// Role match
	for _, requiredRole := range s.Roles {
		for _, hostRole := range roles {
			if requiredRole == hostRole {
				return true
			}
		}
	}

	return false
}

// Target defines supported OS distribution, version, and architecture.
type Target struct {
	OS      string `json:"os" yaml:"os"`
	Release string `json:"release" yaml:"release"`
	Arch    string `json:"arch" yaml:"arch"`
}

// Application represents a deployable application target.
type Application struct {
	Name             string          `json:"name" yaml:"name"`
	Type             ApplicationType `json:"type" yaml:"type"`
	Selector         Selector        `json:"selector" yaml:"selector"`
	Artifact         string          `json:"artifact,omitempty" yaml:"artifact,omitempty"`
	Destination      string          `json:"destination,omitempty" yaml:"destination,omitempty"`
	Permissions      string          `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	Owner            string          `json:"owner,omitempty" yaml:"owner,omitempty"`
	Group            string          `json:"group,omitempty" yaml:"group,omitempty"`
	SourceDir        string          `json:"sourceDir,omitempty" yaml:"sourceDir,omitempty"`
	DistDir          string          `json:"distDir,omitempty" yaml:"distDir,omitempty"`
	VenvPath         string          `json:"venvPath,omitempty" yaml:"venvPath,omitempty"`
	WheelsDir        string          `json:"wheelsDir,omitempty" yaml:"wheelsDir,omitempty"`
	RequirementsFile string          `json:"requirementsFile,omitempty" yaml:"requirementsFile,omitempty"`
	PythonBinary     string          `json:"pythonBinary,omitempty" yaml:"pythonBinary,omitempty"`
	EmbeddedIn       string          `json:"embeddedIn,omitempty" yaml:"embeddedIn,omitempty"`
	TemplatesDir     string          `json:"templatesDir,omitempty" yaml:"templatesDir,omitempty"`
	StaticDir        string          `json:"staticDir,omitempty" yaml:"staticDir,omitempty"`
}

// PackageItem defines a required OS package item.
type PackageItem struct {
	Name     string   `json:"name" yaml:"name"`
	Version  string   `json:"version" yaml:"version"`
	Selector Selector `json:"selector,omitempty" yaml:"selector,omitempty"`
}

// PackageTarget defines target-specific package repository configuration.
type PackageTarget struct {
	Manager string        `json:"manager" yaml:"manager"` // apt, dnf, yum
	Items   []PackageItem `json:"items" yaml:"items"`
}

// Certificate defines application server TLS certificate management.
type Certificate struct {
	Name                      string           `json:"name" yaml:"name"`
	Selector                  Selector         `json:"selector" yaml:"selector"`
	CertPath                  string           `json:"certPath" yaml:"certPath"`
	KeyPath                   string           `json:"keyPath" yaml:"keyPath"`
	Permissions               string           `json:"permissions" yaml:"permissions"`
	Owner                     string           `json:"owner" yaml:"owner"`
	Group                     string           `json:"group" yaml:"group"`
	Provisioning              ProvisioningType `json:"provisioning" yaml:"provisioning"`
	ACMEServer                string           `json:"acmeServer,omitempty" yaml:"acmeServer,omitempty"`
	CommonName                string           `json:"commonName,omitempty" yaml:"commonName,omitempty"`
	DNSNames                  []string         `json:"dnsNames,omitempty" yaml:"dnsNames,omitempty"`
	IPAddresses               []string         `json:"ipAddresses,omitempty" yaml:"ipAddresses,omitempty"`
	AutoRenewDaysBeforeExpiry int              `json:"autoRenewDaysBeforeExpiry,omitempty" yaml:"autoRenewDaysBeforeExpiry,omitempty"`
	ReloadService             string           `json:"reloadService,omitempty" yaml:"reloadService,omitempty"`
	EncryptedKeyArtifact      string           `json:"encryptedKeyArtifact,omitempty" yaml:"encryptedKeyArtifact,omitempty"`
	CertArtifact              string           `json:"certArtifact,omitempty" yaml:"certArtifact,omitempty"`
}

// Config defines a configuration template and its deployment target.
type Config struct {
	Template        string   `json:"template" yaml:"template"`
	Destination     string   `json:"destination" yaml:"destination"`
	Permissions     string   `json:"permissions" yaml:"permissions"`
	Owner           string   `json:"owner" yaml:"owner"`
	Group           string   `json:"group" yaml:"group"`
	SOPSEncrypted   bool     `json:"sopsEncrypted,omitempty" yaml:"sopsEncrypted,omitempty"`
	Schema          string   `json:"schema,omitempty" yaml:"schema,omitempty"`
	ValidateCommand string   `json:"validateCommand,omitempty" yaml:"validateCommand,omitempty"`
	Selector        Selector `json:"selector,omitempty" yaml:"selector,omitempty"`
}

// Service defines systemd unit lifecycle management.
type Service struct {
	Name           string        `json:"name" yaml:"name"`
	Action         ServiceAction `json:"action" yaml:"action"`
	UnitFile       string        `json:"unitFile,omitempty" yaml:"unitFile,omitempty"`
	TimeoutSeconds int           `json:"timeoutSeconds,omitempty" yaml:"timeoutSeconds,omitempty"`
	Selector       Selector      `json:"selector,omitempty" yaml:"selector,omitempty"`
}

// HealthCheck defines post-deployment health verification.
type HealthCheck struct {
	Type             HealthCheckType `json:"type" yaml:"type"`
	Endpoint         string          `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	ExpectedStatus   int             `json:"expectedStatus,omitempty" yaml:"expectedStatus,omitempty"`
	Command          string          `json:"command,omitempty" yaml:"command,omitempty"`
	ExpectedExitCode int             `json:"expectedExitCode,omitempty" yaml:"expectedExitCode,omitempty"`
	IntervalSeconds  int             `json:"intervalSeconds" yaml:"intervalSeconds"`
	MaxRetries       int             `json:"maxRetries" yaml:"maxRetries"`
	Selector         Selector        `json:"selector,omitempty" yaml:"selector,omitempty"`
}

// RollbackPolicy defines automatic recovery actions on failure.
type RollbackPolicy struct {
	AutoOnFailure             bool             `json:"autoOnFailure" yaml:"autoOnFailure"`
	Strategy                  RollbackStrategy `json:"strategy" yaml:"strategy"`
	PreserveBackupGenerations int              `json:"preserveBackupGenerations,omitempty" yaml:"preserveBackupGenerations,omitempty"`
}

// Manifest represents the top-level bundle deployment specification (manifest.yaml).
type Manifest struct {
	Version             string                   `json:"version" yaml:"version"`
	BundleVersion       string                   `json:"bundleVersion" yaml:"bundleVersion"`
	Release             string                   `json:"release" yaml:"release"`
	BundleType          string                   `json:"bundleType,omitempty" yaml:"bundleType,omitempty"`
	BaseRelease         string                   `json:"baseRelease,omitempty" yaml:"baseRelease,omitempty"`
	BaseReleaseChecksum string                   `json:"baseReleaseChecksum,omitempty" yaml:"baseReleaseChecksum,omitempty"`
	TargetRelease       string                   `json:"targetRelease,omitempty" yaml:"targetRelease,omitempty"`
	CreatedAt           time.Time                `json:"createdAt" yaml:"createdAt"`
	ExpiresAt           *time.Time               `json:"expiresAt,omitempty" yaml:"expiresAt,omitempty"`
	Author              string                   `json:"author,omitempty" yaml:"author,omitempty"`
	Targets             []Target                 `json:"targets" yaml:"targets"`
	Applications        []Application            `json:"applications" yaml:"applications"`
	Packages            map[string]PackageTarget `json:"packages,omitempty" yaml:"packages,omitempty"`
	Certificates        []Certificate            `json:"certificates,omitempty" yaml:"certificates,omitempty"`
	Configs             []Config                 `json:"configs,omitempty" yaml:"configs,omitempty"`
	Services            []Service                `json:"services,omitempty" yaml:"services,omitempty"`
	HealthChecks        []HealthCheck            `json:"healthChecks,omitempty" yaml:"healthChecks,omitempty"`
	Rollback            RollbackPolicy           `json:"rollback" yaml:"rollback"`
}

// RequiredFile represents a checked bundle artifact file status.
type RequiredFile struct {
	Path   string `json:"path" yaml:"path"`
	Exists bool   `json:"exists" yaml:"exists"`
	SHA256 string `json:"sha256,omitempty" yaml:"sha256,omitempty"`
}

// MissingItem represents a missing package, artifact, or secret.
type MissingItem struct {
	Name   string `json:"name" yaml:"name"`
	Type   string `json:"type" yaml:"type"` // package, artifact, secret_variable
	Reason string `json:"reason" yaml:"reason"`
}

// ResourceEstimates represents estimated storage and memory footprints.
type ResourceEstimates struct {
	BundleSizeBytes       int64 `json:"bundleSizeBytes" yaml:"bundleSizeBytes"`
	InstalledSizeBytes    int64 `json:"installedSizeBytes" yaml:"installedSizeBytes"`
	RequiredDiskFreeBytes int64 `json:"requiredDiskFreeBytes" yaml:"requiredDiskFreeBytes"`
}

// Risk represents an identified deployment risk and recommended mitigation.
type Risk struct {
	ID          string       `json:"id" yaml:"id"`
	Severity    RiskSeverity `json:"severity" yaml:"severity"`
	Category    string       `json:"category" yaml:"category"`
	Description string       `json:"description" yaml:"description"`
	Mitigation  string       `json:"mitigation" yaml:"mitigation"`
}

// PreparationResult represents the evaluation report before bundle export.
type PreparationResult struct {
	Version           string            `json:"version" yaml:"version"`
	EvaluatedAt       time.Time         `json:"evaluatedAt" yaml:"evaluatedAt"`
	Status            PreparationStatus `json:"status" yaml:"status"`
	Application       string            `json:"application" yaml:"application"`
	Release           string            `json:"release" yaml:"release"`
	Target            Target            `json:"target" yaml:"target"`
	RequiredFiles     []RequiredFile    `json:"requiredFiles" yaml:"requiredFiles"`
	Missing           []MissingItem     `json:"missing,omitempty" yaml:"missing,omitempty"`
	ResourceEstimates ResourceEstimates `json:"resourceEstimates" yaml:"resourceEstimates"`
	Risks             []Risk            `json:"risks,omitempty" yaml:"risks,omitempty"`
}
