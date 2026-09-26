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

package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sh0jitmy/ytagarasu/internal/audit"
	"github.com/sh0jitmy/ytagarasu/internal/server/importer"
	"github.com/sh0jitmy/ytagarasu/internal/server/store"
)

// Server provides HTTP API routing and static repository hosting.
type Server struct {
	db             *store.DB
	imp            *importer.Importer
	namespacesRoot string
	mux            *http.ServeMux
}

// NewServer initializes a new Server and mounts all HTTP routes.
func NewServer(db *store.DB, imp *importer.Importer, namespacesRoot string) *Server {
	cleanNS := filepath.Clean(namespacesRoot)
	s := &Server{
		db:             db,
		imp:            imp,
		namespacesRoot: cleanNS,
		mux:            http.NewServeMux(),
	}
	s.routes()
	return s
}

// Handler returns the underlying http.Handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	// Health & System
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /api/v1/system/healthz", s.handleHealthz)

	// Services & Desired Manifests
	s.mux.HandleFunc("GET /api/v1/services", s.handleListServices)
	s.mux.HandleFunc("GET /api/v1/services/{service_id}/desired", s.handleDesiredManifest)

	// Bundles & Reports
	s.mux.HandleFunc("POST /api/v1/bundles/import", s.handleBundleImport)
	s.mux.HandleFunc("POST /api/v1/audit/logs", s.handleAuditLogs)
	s.mux.HandleFunc("GET /api/v1/audit/records", s.handleListAuditRecords)
	s.mux.HandleFunc("GET /api/v1/audit/verify", s.handleVerifyAuditTrail)

	// Virtual Repositories
	s.mux.HandleFunc("GET /repos/", s.handleRepos)
}

type auditLogRequest struct {
	EventType string `json:"event_type"`
	EntityID  string `json:"entity_id"`
	Actor     string `json:"actor"`
	Payload   string `json:"payload"`
}

func (s *Server) handleAuditLogs(w http.ResponseWriter, r *http.Request) {
	var req auditLogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("invalid audit log body: %v", err))
		return
	}
	if req.EventType == "" {
		req.EventType = "system.event"
	}
	if req.EntityID == "" {
		req.EntityID = "system"
	}
	if req.Actor == "" {
		req.Actor = "anonymous"
	}

	rec, err := s.db.AppendAuditLog(r.Context(), req.EventType, req.EntityID, req.Actor, []byte(req.Payload))
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed appending audit log: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(rec)
}

func (s *Server) handleListAuditRecords(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	limit := 100
	offset := 0
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}
	if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
		offset = o
	}

	records, err := s.db.ListAuditLogs(r.Context(), limit, offset)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed listing audit records: %v", err))
		return
	}
	if records == nil {
		records = []audit.Record{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(records)
}

func (s *Server) handleVerifyAuditTrail(w http.ResponseWriter, r *http.Request) {
	report, err := s.db.VerifyAuditTrail(r.Context())
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed verifying audit trail: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if !report.Valid {
		w.WriteHeader(http.StatusConflict) // 409 Conflict if tampered
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_ = json.NewEncoder(w).Encode(report)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleListServices(w http.ResponseWriter, r *http.Request) {
	services, err := s.db.ListServices(r.Context())
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed listing services: %v", err))
		return
	}
	if services == nil {
		services = []store.Service{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(services)
}

func (s *Server) handleDesiredManifest(w http.ResponseWriter, r *http.Request) {
	serviceID := r.PathValue("service_id")
	if serviceID == "" {
		s.writeJSONError(w, http.StatusBadRequest, "service_id is required")
		return
	}

	activeRel, err := s.db.GetActiveRelease(r.Context(), serviceID)
	if err != nil {
		if errors.Is(err, store.ErrNoActiveRelease) || strings.Contains(err.Error(), "no active release") {
			s.writeJSONError(w, http.StatusNotFound, fmt.Sprintf("no active release found for service: %s", serviceID))
			return
		}
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("failed fetching desired release: %v", err))
		return
	}

	// Serve manifest YAML directly
	w.Header().Set("Content-Type", "application/x-yaml")
	w.Header().Set("X-Release-ID", activeRel.ID)
	w.Header().Set("X-Release-Version", activeRel.ReleaseVersion)
	_, _ = w.Write([]byte(activeRel.ManifestYAML)) //nolint:gosec // Manifest YAML is served as raw configuration
}

func (s *Server) handleBundleImport(w http.ResponseWriter, r *http.Request) {
	var reader io.Reader
	expectedKey := r.Header.Get("X-Public-Key")
	if expectedKey == "" {
		expectedKey = r.URL.Query().Get("public_key")
	}

	// Check if multipart form upload
	contentType := r.Header.Get("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Limit to 1GB max upload (gosec G120)
		r.Body = http.MaxBytesReader(w, r.Body, 1024<<20)
		if err := r.ParseMultipartForm(32 << 20); err != nil { //nolint:gosec // Request body is explicitly bounded by MaxBytesReader above
			s.writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("failed parsing multipart form: %v", err))
			return
		}
		defer func() {
			if r.MultipartForm != nil {
				_ = r.MultipartForm.RemoveAll()
			}
		}()

		file, _, err := r.FormFile("bundle")
		if err != nil {
			file, _, err = r.FormFile("bundle_file")
		}
		if err != nil {
			file, _, err = r.FormFile("file")
		}
		if err != nil {
			s.writeJSONError(w, http.StatusBadRequest, "no bundle file found in multipart form (key 'bundle' or 'file')")
			return
		}
		defer func() { _ = file.Close() }()
		reader = file
	} else {
		// Direct stream body
		reader = r.Body
	}

	res, err := s.imp.Import(r.Context(), reader, expectedKey)
	if err != nil {
		s.writeJSONError(w, http.StatusUnprocessableEntity, fmt.Sprintf("bundle import rejected: %v", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleRepos(w http.ResponseWriter, r *http.Request) {
	// Path format: /repos/{service_id}/{repo_type}/{subpath...}
	subPath := strings.TrimPrefix(r.URL.Path, "/repos/")
	parts := strings.SplitN(subPath, "/", 2)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		s.writeJSONError(w, http.StatusBadRequest, "invalid repository path format: /repos/{service_id}/...")
		return
	}

	serviceID := parts[0]
	filePathRel := parts[1]

	// Security: prevent directory traversal
	cleanRel := filepath.Clean(filePathRel)
	if strings.HasPrefix(cleanRel, "..") || strings.HasPrefix(cleanRel, "/") {
		s.writeJSONError(w, http.StatusForbidden, "invalid relative path")
		return
	}

	targetFile := filepath.Clean(filepath.Join(s.namespacesRoot, serviceID, cleanRel))
	if !strings.HasPrefix(targetFile, s.namespacesRoot) {
		s.writeJSONError(w, http.StatusForbidden, "invalid path traversal")
		return
	}

	info, err := os.Stat(targetFile) //nolint:gosec // Path is verified to be inside namespacesRoot
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.writeJSONError(w, http.StatusNotFound, "file not found in repository")
			return
		}
		s.writeJSONError(w, http.StatusInternalServerError, "failed accessing repository asset")
		return
	}

	if info.IsDir() {
		s.writeJSONError(w, http.StatusForbidden, "directory listing forbidden")
		return
	}

	// Content-Type handling for APT / RPM / artifacts
	s.setMIMETypes(w, cleanRel)
	http.ServeFile(w, r, targetFile) //nolint:gosec // Path is verified to be inside namespacesRoot
}

func (s *Server) setMIMETypes(w http.ResponseWriter, path string) {
	switch {
	case strings.HasSuffix(path, ".deb"):
		w.Header().Set("Content-Type", "application/vnd.debian.binary-package")
	case strings.HasSuffix(path, ".rpm"):
		w.Header().Set("Content-Type", "application/x-rpm")
	case strings.HasSuffix(path, "Release") || strings.HasSuffix(path, "InRelease"):
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	case strings.HasSuffix(path, ".gz"):
		w.Header().Set("Content-Type", "application/gzip")
	case strings.HasSuffix(path, ".xml"):
		w.Header().Set("Content-Type", "application/xml")
	case strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml"):
		w.Header().Set("Content-Type", "application/x-yaml")
	}
}

func (s *Server) writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}
