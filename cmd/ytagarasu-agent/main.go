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
)

func main() {
	var (
		serverURL    string
		serviceID    string
		pollInterval time.Duration
		stateDir     string
		installDir   string
		rolesStr     string
		showVersion  bool
	)

	flag.StringVar(&serverURL, "server", "http://127.0.0.1:8080", "ytagarasu-server base URL")
	flag.StringVar(&serverURL, "s", "http://127.0.0.1:8080", "ytagarasu-server base URL (short)")
	flag.StringVar(&serviceID, "service", "default", "Assigned service identifier to deploy")
	flag.DurationVar(&pollInterval, "interval", 10*time.Second, "Polling interval duration")
	flag.DurationVar(&pollInterval, "i", 10*time.Second, "Polling interval duration (short)")
	flag.StringVar(&stateDir, "state-dir", "./data/ytagarasu-agent", "Directory to store agent state and lock file")
	flag.StringVar(&stateDir, "d", "./data/ytagarasu-agent", "Directory to store agent state (short)")
	flag.StringVar(&installDir, "install-dir", "/", "Root directory to install application binaries")
	flag.StringVar(&rolesStr, "roles", "", "Comma-separated list of host roles (e.g. api,worker)")
	flag.BoolVar(&showVersion, "version", false, "Print version information and exit")
	flag.BoolVar(&showVersion, "v", false, "Print version information and exit (short)")
	flag.Parse()

	if showVersion {
		fmt.Printf("ytagarasu-agent %s (commit: %s, built: %s)\n", version.Version, version.Commit, version.Date)
		return
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

	var roles []string
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
