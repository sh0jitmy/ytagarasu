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

package manifest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Common validation errors.
var (
	ErrMissingVersion      = errors.New("manifest version is required")
	ErrMissingBundleVer    = errors.New("bundleVersion is required")
	ErrMissingRelease      = errors.New("release is required")
	ErrMissingTargets      = errors.New("at least one target platform must be defined")
	ErrMissingApplications = errors.New("at least one application must be defined")
	ErrInvalidAppType      = errors.New("invalid application type")
	ErrInvalidServiceAct   = errors.New("invalid service action")
	ErrInvalidHealthCheck  = errors.New("invalid health check configuration")
	ErrInsecurePermission  = errors.New("insecure file permission detected")
)

// LintSeverity represents the severity level of a lint issue.
type LintSeverity string

const (
	LintError   LintSeverity = "ERROR"
	LintWarning LintSeverity = "WARNING"
	LintInfo    LintSeverity = "INFO"
)

// LintIssue represents a diagnostic finding during manifest inspection.
type LintIssue struct {
	Severity LintSeverity `json:"severity"`
	Field    string       `json:"field"`
	Message  string       `json:"message"`
}

func (l LintIssue) String() string {
	return fmt.Sprintf("[%s] %s: %s", l.Severity, l.Field, l.Message)
}

// Parse unmarshals raw YAML bytes into a Manifest structure.
func Parse(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest YAML: %w", err)
	}
	if err := Validate(&m); err != nil {
		return nil, fmt.Errorf("manifest validation failed: %w", err)
	}
	return &m, nil
}

// ParseFile reads and parses a manifest YAML file from disk.
func ParseFile(filePath string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}
	return Parse(data)
}

// Validate performs structural and semantic checks on the Manifest.
func Validate(m *Manifest) error {
	if m == nil {
		return errors.New("manifest cannot be nil")
	}
	if strings.TrimSpace(m.Version) == "" {
		return ErrMissingVersion
	}
	if strings.TrimSpace(m.BundleVersion) == "" {
		return ErrMissingBundleVer
	}
	if strings.TrimSpace(m.Release) == "" {
		return ErrMissingRelease
	}
	if len(m.Targets) == 0 {
		return ErrMissingTargets
	}
	for i, t := range m.Targets {
		if strings.TrimSpace(t.OS) == "" || strings.TrimSpace(t.Release) == "" || strings.TrimSpace(t.Arch) == "" {
			return fmt.Errorf("target[%d] missing os, release, or arch", i)
		}
	}
	if len(m.Applications) == 0 {
		return ErrMissingApplications
	}

	for i, app := range m.Applications {
		if strings.TrimSpace(app.Name) == "" {
			return fmt.Errorf("application[%d] name is required", i)
		}
		switch app.Type {
		case AppTypeGolang:
			if strings.TrimSpace(app.Destination) == "" {
				return fmt.Errorf("application[%s] golang requires destination path", app.Name)
			}
		case AppTypePython:
			if strings.TrimSpace(app.Destination) == "" {
				return fmt.Errorf("application[%s] python requires destination path", app.Name)
			}
		case AppTypeReact:
			if strings.TrimSpace(app.Destination) == "" {
				return fmt.Errorf("application[%s] react requires destination path", app.Name)
			}
		case AppTypeHTMX:
			if strings.TrimSpace(app.EmbeddedIn) == "" && strings.TrimSpace(app.Destination) == "" {
				return fmt.Errorf("application[%s] htmx requires either embeddedIn or destination path", app.Name)
			}
		default:
			return fmt.Errorf("%w: %s for application %s", ErrInvalidAppType, app.Type, app.Name)
		}
	}

	for i, cert := range m.Certificates {
		if strings.TrimSpace(cert.Name) == "" {
			return fmt.Errorf("certificate[%d] name is required", i)
		}
		if strings.TrimSpace(cert.CertPath) == "" || strings.TrimSpace(cert.KeyPath) == "" {
			return fmt.Errorf("certificate[%s] requires certPath and keyPath", cert.Name)
		}
		switch cert.Provisioning {
		case ProvisioningACME:
			if strings.TrimSpace(cert.ACMEServer) == "" {
				return fmt.Errorf("certificate[%s] acme provisioning requires acmeServer", cert.Name)
			}
		case ProvisioningEncryptedSOPS:
			if strings.TrimSpace(cert.EncryptedKeyArtifact) == "" {
				return fmt.Errorf("certificate[%s] encrypted_sops provisioning requires encryptedKeyArtifact", cert.Name)
			}
		default:
			return fmt.Errorf("certificate[%s] invalid provisioning type: %s", cert.Name, cert.Provisioning)
		}
	}

	for i, cfg := range m.Configs {
		if strings.TrimSpace(cfg.Template) == "" || strings.TrimSpace(cfg.Destination) == "" {
			return fmt.Errorf("config[%d] requires template and destination", i)
		}
	}

	for i, svc := range m.Services {
		if strings.TrimSpace(svc.Name) == "" {
			return fmt.Errorf("service[%d] name is required", i)
		}
		switch svc.Action {
		case ServiceActionRestart, ServiceActionReload, ServiceActionStart, ServiceActionStop:
			// valid
		default:
			return fmt.Errorf("%w: %s for service %s", ErrInvalidServiceAct, svc.Action, svc.Name)
		}
	}

	for i, hc := range m.HealthChecks {
		switch hc.Type {
		case HealthCheckHTTP:
			if strings.TrimSpace(hc.Endpoint) == "" {
				return fmt.Errorf("%w: http health check requires endpoint", ErrInvalidHealthCheck)
			}
		case HealthCheckCommand:
			if strings.TrimSpace(hc.Command) == "" {
				return fmt.Errorf("%w: command health check requires command string", ErrInvalidHealthCheck)
			}
		case HealthCheckTCP:
			if strings.TrimSpace(hc.Endpoint) == "" {
				return fmt.Errorf("%w: tcp health check requires endpoint (host:port)", ErrInvalidHealthCheck)
			}
		default:
			return fmt.Errorf("%w: unknown health check type %s", ErrInvalidHealthCheck, hc.Type)
		}
		if hc.IntervalSeconds <= 0 {
			m.HealthChecks[i].IntervalSeconds = 2
		}
		if hc.MaxRetries <= 0 {
			m.HealthChecks[i].MaxRetries = 10
		}
	}

	if m.Rollback.Strategy == "" {
		m.Rollback.Strategy = RollbackImmediate
	}

	return nil
}

// Lint runs static analysis checks and returns any identified warnings or errors.
func Lint(m *Manifest) []LintIssue {
	var issues []LintIssue
	if m == nil {
		return []LintIssue{{Severity: LintError, Field: "root", Message: "manifest is nil"}}
	}

	// Check expiration
	if m.ExpiresAt != nil && m.ExpiresAt.Before(time.Now()) {
		issues = append(issues, LintIssue{
			Severity: LintError,
			Field:    "expiresAt",
			Message:  fmt.Sprintf("bundle expired at %s", m.ExpiresAt.Format(time.RFC3339)),
		})
	}

	// Check certificate key permissions
	for _, cert := range m.Certificates {
		if cert.Permissions != "" {
			permVal, err := strconv.ParseUint(cert.Permissions, 8, 32)
			if err == nil {
				// Private key should not be world-readable or group-writable (e.g. 0600 or 0400 recommended)
				if permVal&0077 != 0 {
					issues = append(issues, LintIssue{
						Severity: LintError,
						Field:    fmt.Sprintf("certificates[%s].permissions", cert.Name),
						Message:  fmt.Sprintf("private key permissions '%s' must not allow group or world access (recommended: 0600)", cert.Permissions),
					})
				}
			}
		}
	}

	// Collect all defined roles in applications to check consistency
	definedRoles := make(map[string]bool)
	for _, app := range m.Applications {
		for _, r := range app.Selector.Roles {
			if r != "*" {
				definedRoles[r] = true
			}
		}
	}

	// Check if certificates or configs reference roles not used in applications
	for _, cert := range m.Certificates {
		for _, r := range cert.Selector.Roles {
			if r != "*" && len(definedRoles) > 0 && !definedRoles[r] {
				issues = append(issues, LintIssue{
					Severity: LintWarning,
					Field:    fmt.Sprintf("certificates[%s].selector.roles", cert.Name),
					Message:  fmt.Sprintf("role '%s' is not defined in any application selector", r),
				})
			}
		}
	}

	for _, cfg := range m.Configs {
		for _, r := range cfg.Selector.Roles {
			if r != "*" && len(definedRoles) > 0 && !definedRoles[r] {
				issues = append(issues, LintIssue{
					Severity: LintWarning,
					Field:    fmt.Sprintf("configs[%s].selector.roles", cfg.Destination),
					Message:  fmt.Sprintf("role '%s' is not defined in any application selector", r),
				})
			}
		}
	}

	return issues
}

// FilterByNode creates a specialized subset Manifest containing only definitions
// applicable to a target host matching the specified roles and hostname.
func FilterByNode(m *Manifest, roles []string, hostname string) *Manifest {
	if m == nil {
		return nil
	}

	filtered := *m
	filtered.Applications = make([]Application, 0)
	for _, app := range m.Applications {
		if app.Selector.Matches(roles, hostname) {
			filtered.Applications = append(filtered.Applications, app)
		}
	}

	filtered.Certificates = make([]Certificate, 0)
	for _, cert := range m.Certificates {
		if cert.Selector.Matches(roles, hostname) {
			filtered.Certificates = append(filtered.Certificates, cert)
		}
	}

	filtered.Configs = make([]Config, 0)
	for _, cfg := range m.Configs {
		if cfg.Selector.Matches(roles, hostname) {
			filtered.Configs = append(filtered.Configs, cfg)
		}
	}

	filtered.Services = make([]Service, 0)
	for _, svc := range m.Services {
		if svc.Selector.Matches(roles, hostname) {
			filtered.Services = append(filtered.Services, svc)
		}
	}

	filtered.HealthChecks = make([]HealthCheck, 0)
	for _, hc := range m.HealthChecks {
		if hc.Selector.Matches(roles, hostname) {
			filtered.HealthChecks = append(filtered.HealthChecks, hc)
		}
	}

	return &filtered
}
