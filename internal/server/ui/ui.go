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

package ui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"runtime"
	"strings"

	"github.com/sh0jitmy/ytagarasu/internal/audit"
	"github.com/sh0jitmy/ytagarasu/internal/server/importer"
	"github.com/sh0jitmy/ytagarasu/internal/server/store"
)

//go:embed templates/* static/*
var EmbeddedAssets embed.FS

// ServiceView represents a service in the dashboard.
type ServiceView struct {
	ServiceID     string
	ActiveRelease string
	Checksum      string
	ArtifactCount int
	UpdatedAt     string
}

// AuditView represents an audit log entry in the UI.
type AuditView struct {
	Sequence       int64
	Timestamp      string
	EventType      string
	Actor          string
	EntityID       string
	PrevRecordHash string
	RecordHash     string
}

// DashboardData is the view model for the main dashboard.
type DashboardData struct {
	ActiveTab         string
	TotalServices     int
	TotalReleases     int
	TotalAuditRecords int
	Goroutines        int
	MemoryMB          uint64
	Services          []ServiceView
}

// AuditData is the view model for the audit page.
type AuditData struct {
	ActiveTab   string
	TotalCount  int
	Verified    bool
	VerifyError string
	Records     []AuditView
}

// UI handles dashboard and audit HTML pages and HTMX components.
type UI struct {
	db             *store.DB
	imp            *importer.Importer
	dashboardTmpl  *template.Template
	auditTmpl      *template.Template
	componentsTmpl *template.Template
	staticSub      http.Handler
}

// NewUI initializes and parses templates.
func NewUI(db *store.DB, imp *importer.Importer) (*UI, error) {
	dashTmpl, err := template.ParseFS(
		EmbeddedAssets,
		"templates/layout.html",
		"templates/dashboard.html",
		"templates/components/*.html",
	)
	if err != nil {
		return nil, fmt.Errorf("failed parsing dashboard template: %w", err)
	}

	auditTmpl, err := template.ParseFS(
		EmbeddedAssets,
		"templates/layout.html",
		"templates/audit.html",
		"templates/components/*.html",
	)
	if err != nil {
		return nil, fmt.Errorf("failed parsing audit template: %w", err)
	}

	compTmpl, err := template.ParseFS(
		EmbeddedAssets,
		"templates/components/*.html",
	)
	if err != nil {
		return nil, fmt.Errorf("failed parsing components template: %w", err)
	}

	staticFS, err := fs.Sub(EmbeddedAssets, "static")
	if err != nil {
		return nil, fmt.Errorf("failed creating static sub filesystem: %w", err)
	}

	return &UI{
		db:             db,
		imp:            imp,
		dashboardTmpl:  dashTmpl,
		auditTmpl:      auditTmpl,
		componentsTmpl: compTmpl,
		staticSub:      http.StripPrefix("/ui/static/", http.FileServer(http.FS(staticFS))),
	}, nil
}

// RegisterRoutes registers all UI and HTMX routes to the serve mux.
func (u *UI) RegisterRoutes(mux *http.ServeMux) {
	// Static assets
	mux.Handle("GET /ui/static/", u.staticSub)

	// Full HTML Pages
	mux.HandleFunc("GET /ui", u.handleDashboard)
	mux.HandleFunc("GET /ui/audit", u.handleAudit)

	// HTMX Partial Components
	mux.HandleFunc("GET /ui/components/services", u.handleServicesComponent)
	mux.HandleFunc("GET /ui/components/system-metrics", u.handleSystemMetricsComponent)
	mux.HandleFunc("GET /ui/components/audit-table", u.handleAuditTableComponent)

	// HTMX Interactive Actions
	mux.HandleFunc("POST /ui/actions/import-bundle", u.handleImportBundleAction)
	mux.HandleFunc("GET /ui/actions/verify-audit", u.handleVerifyAuditAction)
}

func (u *UI) buildDashboardData(ctx context.Context) (DashboardData, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	services, err := u.db.ListServices(ctx)
	if err != nil {
		return DashboardData{}, err
	}

	auditLogs, err := u.db.ListAuditLogs(ctx, 1000, 0)
	if err != nil {
		return DashboardData{}, err
	}

	totalReleases := 0
	serviceViews := make([]ServiceView, 0, len(services))
	for _, s := range services {
		artCount := 0
		checksum := ""
		activeRelVer := ""

		if s.ActiveReleaseID != "" {
			rel, getErr := u.db.GetActiveRelease(ctx, s.ID)
			if getErr == nil && rel != nil {
				totalReleases++
				activeRelVer = rel.ReleaseVersion
				h := sha256.Sum256([]byte(rel.ManifestYAML))
				checksum = hex.EncodeToString(h[:])
				artifacts, artErr := u.db.GetReleaseArtifacts(ctx, rel.ID)
				if artErr == nil {
					artCount = len(artifacts)
				}
			}
		}

		serviceViews = append(serviceViews, ServiceView{
			ServiceID:     s.ID,
			ActiveRelease: activeRelVer,
			Checksum:      checksum,
			ArtifactCount: artCount,
			UpdatedAt:     s.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	return DashboardData{
		ActiveTab:         "dashboard",
		TotalServices:     len(services),
		TotalReleases:     totalReleases,
		TotalAuditRecords: len(auditLogs),
		Goroutines:        runtime.NumGoroutine(),
		MemoryMB:          m.Alloc / 1024 / 1024,
		Services:          serviceViews,
	}, nil
}

func (u *UI) buildAuditData(ctx context.Context) (AuditData, error) {
	records, err := u.db.ListAuditLogs(ctx, 200, 0)
	if err != nil {
		return AuditData{}, err
	}

	auditViews := make([]AuditView, 0, len(records))
	for _, r := range records {
		auditViews = append(auditViews, AuditView{
			Sequence:       r.Sequence,
			Timestamp:      r.Timestamp.Format("2006-01-02 15:04:05"),
			EventType:      r.EventType,
			Actor:          r.Actor,
			EntityID:       r.EntityID,
			PrevRecordHash: r.PrevRecordHash,
			RecordHash:     r.RecordHash,
		})
	}

	// Verify chain integrity
	var verifyErrStr string
	verified := true
	report, err := u.db.VerifyAuditTrail(ctx)
	if err != nil {
		verified = false
		verifyErrStr = err.Error()
	} else if report != nil && !report.Valid {
		verified = false
		verifyErrStr = strings.Join(report.Errors, "; ")
	}

	return AuditData{
		ActiveTab:   "audit",
		TotalCount:  len(records),
		Verified:    verified,
		VerifyError: verifyErrStr,
		Records:     auditViews,
	}, nil
}

func (u *UI) handleDashboard(w http.ResponseWriter, r *http.Request) {
	data, err := u.buildDashboardData(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed loading dashboard: %v", err), http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := u.dashboardTmpl.ExecuteTemplate(&buf, "layout.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (u *UI) handleAudit(w http.ResponseWriter, r *http.Request) {
	data, err := u.buildAuditData(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("failed loading audit logs: %v", err), http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := u.auditTmpl.ExecuteTemplate(&buf, "layout.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (u *UI) handleServicesComponent(w http.ResponseWriter, r *http.Request) {
	data, err := u.buildDashboardData(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := u.componentsTmpl.ExecuteTemplate(&buf, "components/services.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (u *UI) handleSystemMetricsComponent(w http.ResponseWriter, r *http.Request) {
	data, err := u.buildDashboardData(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := u.componentsTmpl.ExecuteTemplate(&buf, "components/system_metrics.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (u *UI) handleAuditTableComponent(w http.ResponseWriter, r *http.Request) {
	data, err := u.buildAuditData(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := u.componentsTmpl.ExecuteTemplate(&buf, "components/audit_table.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

func (u *UI) handleVerifyAuditAction(w http.ResponseWriter, r *http.Request) {
	report, err := u.db.VerifyAuditTrail(r.Context())
	all, _ := u.db.ListAuditLogs(r.Context(), 1000, 0)
	count := len(all)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err != nil {
		_, _ = fmt.Fprintf(w, `<span id="verify-badge" class="badge badge-danger">❌ 改ざん検知: %s</span>`, template.HTMLEscapeString(err.Error()))
	} else if report != nil && !report.Valid {
		_, _ = fmt.Fprintf(w, `<span id="verify-badge" class="badge badge-danger">❌ 改ざん検知: %s</span>`, template.HTMLEscapeString(strings.Join(report.Errors, "; ")))
	} else {
		_, _ = fmt.Fprintf(w, `<span id="verify-badge" class="badge badge-success">✅ チェーン整合性確認済み (%d レコード)</span>`, count)
	}
}

type importResultData struct {
	Success bool
	Message string
	Error   string
}

func (u *UI) handleImportBundleAction(w http.ResponseWriter, r *http.Request) {
	// Guard against memory exhaustion by limiting request body size (gosec G120)
	r.Body = http.MaxBytesReader(w, r.Body, 500<<20)
	//nolint:gosec // G120: request body is strictly bounded by http.MaxBytesReader above
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		u.renderImportResult(w, importResultData{
			Success: false,
			Error:   fmt.Sprintf("マルチパート解析エラー: %v", err),
		})
		return
	}

	file, header, err := r.FormFile("bundle")
	if err != nil {
		u.renderImportResult(w, importResultData{
			Success: false,
			Error:   fmt.Sprintf("バンドルファイル取得失敗: %v", err),
		})
		return
	}
	defer func() {
		_ = file.Close()
	}()

	res, err := u.imp.Import(r.Context(), file, "")
	if err != nil {
		_, _ = u.db.AppendAuditLog(r.Context(), "release.import.failed", header.Filename, "web-ui", []byte(err.Error()))

		u.renderImportResult(w, importResultData{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	_, _ = u.db.AppendAuditLog(r.Context(), audit.EventReleaseImport, fmt.Sprintf("%s:%s", res.ServiceID, res.ReleaseVersion), "web-ui", []byte(fmt.Sprintf("imported files=%d bytes=%d", res.FileCount, res.TotalBytes)))

	u.renderImportResult(w, importResultData{
		Success: true,
		Message: fmt.Sprintf("サービス [%s] バージョン [%s] を正常に CAS へ展開しました (登録ファイル: %d 件, サイズ: %d bytes)",
			res.ServiceID, res.ReleaseVersion, res.FileCount, res.TotalBytes),
	})
}

func (u *UI) renderImportResult(w http.ResponseWriter, data importResultData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var buf bytes.Buffer
	if err := u.componentsTmpl.ExecuteTemplate(&buf, "components/import_result.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// HTMX swaps this directly into #import-result-container
	_, _ = w.Write(buf.Bytes())
}
