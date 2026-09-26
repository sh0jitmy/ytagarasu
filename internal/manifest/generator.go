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
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GenerateAuto statically inspects a directory and generates a draft Manifest.
func GenerateAuto(dir string) (*Manifest, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve directory: %w", err)
	}

	projectName := filepath.Base(absDir)
	if projectName == "." || projectName == "/" || projectName == "" {
		projectName = "app"
	}

	now := time.Now().UTC().Truncate(time.Second)
	expires := now.AddDate(1, 0, 0) // Default 1 year validity

	m := &Manifest{
		Version:       "1.0",
		BundleVersion: now.Format("2006.01.02.1"),
		Release:       "1.0.0",
		CreatedAt:     now,
		ExpiresAt:     &expires,
		Author:        "Ytagarasu Automated Scanner <release@example.internal>",
		Targets: []Target{
			{OS: "ubuntu", Release: "24.04", Arch: "amd64"},
			{OS: "rhel", Release: "9", Arch: "amd64"},
		},
		Applications: make([]Application, 0),
		Packages:     make(map[string]PackageTarget),
		Certificates: make([]Certificate, 0),
		Configs:      make([]Config, 0),
		Services:     make([]Service, 0),
		HealthChecks: make([]HealthCheck, 0),
		Rollback: RollbackPolicy{
			AutoOnFailure:             true,
			Strategy:                  RollbackImmediate,
			PreserveBackupGenerations: 3,
		},
	}

	hasGo := fileExists(filepath.Join(absDir, "go.mod"))
	hasPython := fileExists(filepath.Join(absDir, "requirements.txt")) || fileExists(filepath.Join(absDir, "pyproject.toml"))
	hasReact := fileExists(filepath.Join(absDir, "package.json"))
	hasHTMX := dirContains(absDir, "htmx")

	// 1. Go application detection
	if hasGo {
		appName := projectName
		m.Applications = append(m.Applications, Application{
			Name:        appName,
			Type:        AppTypeGolang,
			Selector:    Selector{Roles: []string{"api"}},
			Artifact:    filepath.Join("artifacts/golang", appName),
			Destination: filepath.Join("/usr/local/bin", appName),
			Permissions: "0755",
			Owner:       "app",
			Group:       "app",
		})
		m.Services = append(m.Services, Service{
			Name:           appName,
			Action:         ServiceActionRestart,
			UnitFile:       fmt.Sprintf("configs/%s.service", appName),
			TimeoutSeconds: 30,
			Selector:       Selector{Roles: []string{"api"}},
		})
		m.HealthChecks = append(m.HealthChecks, HealthCheck{
			Type:            HealthCheckHTTP,
			Endpoint:        "http://127.0.0.1:8080/healthz",
			ExpectedStatus:  200,
			IntervalSeconds: 2,
			MaxRetries:      10,
			Selector:        Selector{Roles: []string{"api"}},
		})
	}

	// 2. Python application detection
	if hasPython {
		appName := projectName + "-worker"
		m.Applications = append(m.Applications, Application{
			Name:             appName,
			Type:             AppTypePython,
			Selector:         Selector{Roles: []string{"worker"}},
			SourceDir:        "artifacts/python/app",
			Destination:      filepath.Join("/opt", appName, "app"),
			VenvPath:         filepath.Join("/opt", appName, "venv"),
			WheelsDir:        "artifacts/python/wheels",
			RequirementsFile: "artifacts/python/requirements.lock",
			PythonBinary:     "/usr/bin/python3",
			Owner:            "app",
			Group:            "app",
		})
		m.Services = append(m.Services, Service{
			Name:           appName,
			Action:         ServiceActionRestart,
			UnitFile:       fmt.Sprintf("configs/%s.service", appName),
			TimeoutSeconds: 30,
			Selector:       Selector{Roles: []string{"worker"}},
		})
	}

	// 3. React SPA frontend detection
	if hasReact {
		appName := projectName + "-web"
		m.Applications = append(m.Applications, Application{
			Name:        appName,
			Type:        AppTypeReact,
			Selector:    Selector{Roles: []string{"web"}},
			DistDir:     "artifacts/react/dist",
			Destination: filepath.Join("/var/www", appName),
			Owner:       "www-data",
			Group:       "www-data",
		})
	}

	// 4. HTMX UI detection
	if hasHTMX {
		embeddedIn := ""
		if hasGo {
			embeddedIn = projectName
		}
		m.Applications = append(m.Applications, Application{
			Name:       projectName + "-htmx",
			Type:       AppTypeHTMX,
			Selector:   Selector{Roles: []string{"api"}},
			EmbeddedIn: embeddedIn,
		})
	}

	// Fallback if nothing specific was detected
	if len(m.Applications) == 0 {
		m.Applications = append(m.Applications, Application{
			Name:        projectName,
			Type:        AppTypeGolang,
			Selector:    Selector{Roles: []string{"api"}},
			Artifact:    filepath.Join("artifacts/golang", projectName),
			Destination: filepath.Join("/usr/local/bin", projectName),
			Permissions: "0755",
			Owner:       "app",
			Group:       "app",
		})
		m.Services = append(m.Services, Service{
			Name:           projectName,
			Action:         ServiceActionRestart,
			TimeoutSeconds: 30,
			Selector:       Selector{Roles: []string{"api"}},
		})
		m.HealthChecks = append(m.HealthChecks, HealthCheck{
			Type:            HealthCheckHTTP,
			Endpoint:        "http://127.0.0.1:8080/healthz",
			ExpectedStatus:  200,
			IntervalSeconds: 2,
			MaxRetries:      10,
			Selector:        Selector{Roles: []string{"api"}},
		})
	}

	return m, nil
}

// InitInteractive conducts an interactive terminal wizard to create a manifest.
func InitInteractive(r io.Reader, w io.Writer) (*Manifest, error) {
	scanner := bufio.NewScanner(r)
	prompt := func(msg, defaultVal string) string {
		if defaultVal != "" {
			_, _ = fmt.Fprintf(w, "%s [%s]: ", msg, defaultVal)
		} else {
			_, _ = fmt.Fprintf(w, "%s: ", msg)
		}
		if !scanner.Scan() {
			return defaultVal
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			return defaultVal
		}
		return input
	}

	_, _ = fmt.Fprintln(w, "=== Ytagarasu Manifest Interactive Wizard ===")
	appName := prompt("Application Name", "core-api")
	appTypeStr := prompt("Application Type (golang, python, react, htmx)", "golang")
	role := prompt("Target Host Role (e.g., api, worker, web)", "api")
	dest := prompt("Destination Path", "/usr/local/bin/"+appName)
	enableTLS := prompt("Enable TLS certificate management? (y/n)", "n")

	now := time.Now().UTC().Truncate(time.Second)
	expires := now.AddDate(1, 0, 0)

	appType := ApplicationType(appTypeStr)
	switch appType {
	case AppTypeGolang, AppTypePython, AppTypeReact, AppTypeHTMX:
		// Valid
	default:
		appType = AppTypeGolang
	}

	m := &Manifest{
		Version:       "1.0",
		BundleVersion: now.Format("2006.01.02.1"),
		Release:       "1.0.0",
		CreatedAt:     now,
		ExpiresAt:     &expires,
		Author:        "Release Engineering Team <release@example.internal>",
		Targets: []Target{
			{OS: "ubuntu", Release: "24.04", Arch: "amd64"},
		},
		Applications: []Application{
			{
				Name:        appName,
				Type:        appType,
				Selector:    Selector{Roles: []string{role}},
				Destination: dest,
				Permissions: "0755",
				Owner:       "app",
				Group:       "app",
			},
		},
		Services: []Service{
			{
				Name:           appName,
				Action:         ServiceActionRestart,
				TimeoutSeconds: 30,
				Selector:       Selector{Roles: []string{role}},
			},
		},
		HealthChecks: []HealthCheck{
			{
				Type:            HealthCheckHTTP,
				Endpoint:        "http://127.0.0.1:8080/healthz",
				ExpectedStatus:  200,
				IntervalSeconds: 2,
				MaxRetries:      10,
				Selector:        Selector{Roles: []string{role}},
			},
		},
		Rollback: RollbackPolicy{
			AutoOnFailure:             true,
			Strategy:                  RollbackImmediate,
			PreserveBackupGenerations: 3,
		},
	}

	if strings.ToLower(enableTLS) == "y" || strings.ToLower(enableTLS) == "yes" {
		m.Certificates = append(m.Certificates, Certificate{
			Name:                      appName + "-tls",
			Selector:                  Selector{Roles: []string{role}},
			CertPath:                  fmt.Sprintf("/etc/ssl/certs/%s.crt", appName),
			KeyPath:                   fmt.Sprintf("/etc/ssl/private/%s.key", appName),
			Permissions:               "0600",
			Owner:                     "app",
			Group:                     "app",
			Provisioning:              ProvisioningACME,
			ACMEServer:                "https://ytagarasu-server:9000/acme/acme/directory",
			CommonName:                appName + ".example.internal",
			AutoRenewDaysBeforeExpiry: 30,
			ReloadService:             appName,
		})
	}

	return m, nil
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func dirContains(dir, sub string) bool {
	found := false
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if strings.Contains(strings.ToLower(p), sub) {
			found = true
			return filepath.SkipDir
		}
		return nil
	})
	return found
}
