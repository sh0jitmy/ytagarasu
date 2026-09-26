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

package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/agent"
	"github.com/sh0jitmy/ytagarasu/internal/version"
	"gopkg.in/yaml.v3"
)

type agentConfigFile struct {
	ServerURL  string   `yaml:"server"`
	ServiceID  string   `yaml:"service"`
	Interval   string   `yaml:"interval"`
	StateDir   string   `yaml:"state_dir"`
	InstallDir string   `yaml:"install_dir"`
	Roles      []string `yaml:"roles"`
}

func main() {
	var (
		configPath   string
		serverURL    string
		serviceID    string
		pollInterval time.Duration
		stateDir     string
		installDir   string
		rolesStr     string
		showVersion  bool
	)

	flag.StringVar(&configPath, "config", "", "Path to agent.yaml configuration file")
	flag.StringVar(&configPath, "c", "", "Path to agent.yaml configuration file (short)")
	flag.StringVar(&serverURL, "server", "", "ytagarasu-server base URL")
	flag.StringVar(&serverURL, "s", "", "ytagarasu-server base URL (short)")
	flag.StringVar(&serviceID, "service", "", "Assigned service identifier to deploy")
	flag.DurationVar(&pollInterval, "interval", 0, "Polling interval duration")
	flag.DurationVar(&pollInterval, "i", 0, "Polling interval duration (short)")
	flag.StringVar(&stateDir, "state-dir", "", "Directory to store agent state and lock file")
	flag.StringVar(&stateDir, "d", "", "Directory to store agent state (short)")
	flag.StringVar(&installDir, "install-dir", "", "Root directory to install application binaries")
	flag.StringVar(&rolesStr, "roles", "", "Comma-separated list of host roles (e.g. api,worker)")
	flag.BoolVar(&showVersion, "version", false, "Print version information and exit")
	flag.BoolVar(&showVersion, "v", false, "Print version information and exit (short)")
	flag.Parse()

	if showVersion {
		fmt.Printf("ytagarasu-agent %s (commit: %s, built: %s)\n", version.Version, version.Commit, version.Date)
		return
	}

	// 1. Check config file: explicit flag or standard path
	if configPath == "" {
		if _, err := os.Stat("/etc/ytagarasu/agent.yaml"); err == nil {
			configPath = "/etc/ytagarasu/agent.yaml"
		} else if _, err := os.Stat("/etc/deploy-agent/agent.yaml"); err == nil {
			configPath = "/etc/deploy-agent/agent.yaml"
		}
	}

	var parsedRoles []string
	if configPath != "" {
		cleanConfig := filepath.Clean(configPath)
		data, err := os.ReadFile(cleanConfig) //nolint:gosec // Config file path provided by user or default path
		if err == nil {
			var cf agentConfigFile
			if err := yaml.Unmarshal(data, &cf); err == nil {
				if serverURL == "" && cf.ServerURL != "" {
					serverURL = cf.ServerURL
				}
				if serviceID == "" && cf.ServiceID != "" {
					serviceID = cf.ServiceID
				}
				if pollInterval == 0 && cf.Interval != "" {
					if d, parseErr := time.ParseDuration(cf.Interval); parseErr == nil {
						pollInterval = d
					}
				}
				if stateDir == "" && cf.StateDir != "" {
					stateDir = cf.StateDir
				}
				if installDir == "" && cf.InstallDir != "" {
					installDir = cf.InstallDir
				}
				if len(cf.Roles) > 0 {
					parsedRoles = append(parsedRoles, cf.Roles...)
				}
			}
		}
	}

	// 2. Set defaults if still empty
	if serverURL == "" {
		serverURL = "http://127.0.0.1:8080"
	}
	if serviceID == "" {
		serviceID = "default"
	}
	if pollInterval == 0 {
		pollInterval = 10 * time.Second
	}
	if stateDir == "" {
		stateDir = "./data/ytagarasu-agent"
	}
	if installDir == "" {
		installDir = "/"
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting ytagarasu-agent",
		"version", version.Version,
		"server", serverURL,
		"service", serviceID,
		"interval", pollInterval,
	)

	cleanStateDir := filepath.Clean(stateDir)
	if err := os.MkdirAll(cleanStateDir, 0750); err != nil {
		slog.Error("failed creating agent state directory", "error", err, "path", cleanStateDir)
		os.Exit(1)
	}

	roles := parsedRoles
	if rolesStr != "" {
		for _, r := range strings.Split(rolesStr, ",") {
			trimmed := strings.TrimSpace(r)
			if trimmed != "" {
				roles = append(roles, trimmed)
			}
		}
	}

	// Determine package manager
	var pkgMgr agent.PackageManager
	if _, err := os.Stat("/usr/bin/apt-get"); err == nil {
		pkgMgr = agent.NewAptPackageManager(nil)
	} else if _, err := os.Stat("/usr/bin/dnf"); err == nil {
		pkgMgr = agent.NewRpmPackageManager(nil)
	}

	cfg := agent.Config{
		ServerURL:    serverURL,
		ServiceID:    serviceID,
		PollInterval: pollInterval,
		StateFile:    filepath.Join(cleanStateDir, "state.json"),
		LockFile:     filepath.Join(cleanStateDir, "agent.lock"),
		InstallDir:   installDir,
		Roles:        roles,
	}

	ag := agent.NewAgent(cfg, pkgMgr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := ag.Run(ctx); err != nil && !strings.Contains(err.Error(), "context canceled") {
		slog.Error("agent exited with error", "error", err)
		os.Exit(1)
	}

	slog.Info("ytagarasu-agent stopped cleanly")
}
