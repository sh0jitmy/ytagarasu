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
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/server/handler"
	"github.com/sh0jitmy/ytagarasu/internal/server/importer"
	"github.com/sh0jitmy/ytagarasu/internal/server/store"
	"github.com/sh0jitmy/ytagarasu/internal/version"
)

func main() {
	var (
		listenAddr  string
		dataDir     string
		dbPath      string
		showVersion bool
	)

	flag.StringVar(&listenAddr, "listen", ":8080", "HTTP server listen address")
	flag.StringVar(&listenAddr, "l", ":8080", "HTTP server listen address (short)")
	flag.StringVar(&dataDir, "data-dir", "./data/ytagarasu", "Base data directory for storage, namespaces, and DB")
	flag.StringVar(&dataDir, "d", "./data/ytagarasu", "Base data directory (short)")
	flag.StringVar(&dbPath, "db", "", "SQLite database path (defaults to <data-dir>/ytagarasu.db)")
	flag.BoolVar(&showVersion, "version", false, "Print version information and exit")
	flag.BoolVar(&showVersion, "v", false, "Print version information and exit (short)")
	flag.Parse()

	if showVersion {
		fmt.Printf("ytagarasu-server %s (commit: %s, built: %s)\n", version.Version, version.Commit, version.Date)
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting ytagarasu-server",
		"version", version.Version,
		"listen", listenAddr,
		"data_dir", dataDir,
	)

	cleanDataDir := filepath.Clean(dataDir)
	if err := os.MkdirAll(cleanDataDir, 0750); err != nil {
		slog.Error("failed creating base data directory", "error", err, "path", cleanDataDir)
		os.Exit(1)
	}

	// 1. Initialize CAS
	casDir := filepath.Join(cleanDataDir, "storage")
	cas, err := store.NewCAS(casDir)
	if err != nil {
		slog.Error("failed initializing CAS", "error", err, "path", casDir)
		os.Exit(1)
	}

	// 2. Initialize SQLite Release Database
	if dbPath == "" {
		dbPath = filepath.Join(cleanDataDir, "ytagarasu.db")
	}
	dbDSN := fmt.Sprintf("file:%s", filepath.Clean(dbPath))
	db, err := store.NewDB(dbDSN)
	if err != nil {
		slog.Error("failed opening SQLite database", "error", err, "dsn", dbDSN)
		os.Exit(1)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			slog.Warn("error closing database", "error", closeErr)
		}
	}()

	// 3. Initialize Importer
	namespacesDir := filepath.Join(cleanDataDir, "namespaces")
	imp, err := importer.NewImporter(cas, db, namespacesDir)
	if err != nil {
		slog.Error("failed initializing bundle importer", "error", err, "path", namespacesDir)
		os.Exit(1)
	}

	// 4. Construct Router & HTTP Server
	srvHandler := handler.NewServer(db, imp, namespacesDir)

	httpServer := &http.Server{
		Addr:              listenAddr,
		Handler:           srvHandler.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	// Graceful shutdown handling
	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErrChan := make(chan error, 1)
	go func() {
		slog.Info("ytagarasu-server HTTP listener active", "addr", listenAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrChan <- err
		}
	}()

	select {
	case <-shutdownCtx.Done():
		slog.Info("shutdown signal received, draining active connections...")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(ctx); err != nil {
			slog.Error("error during server graceful shutdown", "error", err)
		}
		slog.Info("ytagarasu-server shutdown complete")
	case err := <-serverErrChan:
		slog.Error("server encountered fatal runtime error", "error", err)
		os.Exit(1)
	}
}
