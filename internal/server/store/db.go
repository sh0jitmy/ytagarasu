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

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sqlite "github.com/glebarez/go-sqlite"
	"github.com/sh0jitmy/ytagarasu/internal/audit"
)

var (
	// ErrNoActiveRelease indicates no active release exists for the service.
	ErrNoActiveRelease = errors.New("no active release for service")
)

func init() {
	var found bool
	for _, d := range sql.Drivers() {
		if d == "sqlite" {
			found = true
			break
		}
	}
	if !found {
		sql.Register("sqlite", &sqlite.Driver{})
	}
}

// Service represents a registered system service.
type Service struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	ActiveReleaseID string    `json:"active_release_id"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// Release represents a verified release deployment bundle.
type Release struct {
	ID             string    `json:"id"`
	ServiceID      string    `json:"service_id"`
	ReleaseVersion string    `json:"release_version"`
	Status         string    `json:"status"` // "active", "inactive", "revoked"
	ManifestYAML   string    `json:"manifest_yaml"`
	Signature      string    `json:"signature"`
	PublicKey      string    `json:"public_key"`
	CreatedAt      time.Time `json:"created_at"`
}

// ReleaseArtifact represents a file packaged in a release.
type ReleaseArtifact struct {
	ID         string    `json:"id"`
	ReleaseID  string    `json:"release_id"`
	RelPath    string    `json:"rel_path"`
	SHA256Hash string    `json:"sha256_hash"`
	SizeBytes  int64     `json:"size_bytes"`
	CreatedAt  time.Time `json:"created_at"`
}

// DB encapsulates the SQLite database connection and release queries.
type DB struct {
	db *sql.DB
}

// NewDB initializes the SQLite database with WAL mode and tables.
func NewDB(dsn string) (*DB, error) {
	// Ensure SQLite governance parameters
	isMemory := strings.Contains(dsn, ":memory:") || strings.Contains(dsn, "mode=memory")
	if !strings.Contains(dsn, "_pragma=foreign_keys(1)") {
		separator := "?"
		if strings.Contains(dsn, "?") {
			separator = "&"
		}
		if isMemory {
			dsn = fmt.Sprintf("%s%s_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", dsn, separator)
		} else {
			dsn = fmt.Sprintf("%s%s_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)", dsn, separator)
		}
	}

	// Create parent directory if file-based
	if !isMemory {
		cleanDSN := strings.TrimPrefix(dsn, "file:")
		if idx := strings.Index(cleanDSN, "?"); idx != -1 {
			cleanDSN = cleanDSN[:idx]
		}
		dir := filepath.Dir(cleanDSN)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0750); err != nil {
				return nil, fmt.Errorf("failed to create db directory: %w", err)
			}
		}
	}

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// Connection pool tuning (golang-sqlite-governance)
	if isMemory {
		sqlDB.SetMaxOpenConns(1)
	} else {
		sqlDB.SetMaxOpenConns(10)
		sqlDB.SetMaxIdleConns(5)
	}
	sqlDB.SetConnMaxLifetime(0)

	s := &DB{db: sqlDB}
	if err := s.initSchema(context.Background()); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return s, nil
}

func (s *DB) initSchema(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS services (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		active_release_id TEXT,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS releases (
		id TEXT PRIMARY KEY,
		service_id TEXT NOT NULL,
		release_version TEXT NOT NULL,
		status TEXT NOT NULL,
		manifest_yaml TEXT NOT NULL,
		signature TEXT NOT NULL,
		public_key TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		FOREIGN KEY (service_id) REFERENCES services(id)
	);

	CREATE TABLE IF NOT EXISTS release_artifacts (
		id TEXT PRIMARY KEY,
		release_id TEXT NOT NULL,
		rel_path TEXT NOT NULL,
		sha256_hash TEXT NOT NULL,
		size_bytes INTEGER NOT NULL,
		created_at DATETIME NOT NULL,
		FOREIGN KEY (release_id) REFERENCES releases(id)
	);

	CREATE INDEX IF NOT EXISTS idx_releases_service ON releases(service_id, status);
	CREATE INDEX IF NOT EXISTS idx_artifacts_release ON release_artifacts(release_id);

	CREATE TABLE IF NOT EXISTS audit_log (
		sequence INTEGER PRIMARY KEY,
		timestamp DATETIME NOT NULL,
		event_type TEXT NOT NULL,
		entity_id TEXT NOT NULL,
		actor TEXT NOT NULL,
		payload_digest TEXT NOT NULL,
		prev_record_hash TEXT NOT NULL,
		record_hash TEXT NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_audit_log_event ON audit_log(event_type, timestamp);
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// Close closes the underlying database connection.
func (s *DB) Close() error {
	return s.db.Close()
}

// DB returns the underlying sql.DB instance (for transactions or testing).
func (s *DB) DB() *sql.DB {
	return s.db
}

// RegisterRelease atomically saves the release, marks previous active releases as inactive,
// updates the active release on the service, and stores its artifacts.
func (s *DB) RegisterRelease(ctx context.Context, rel *Release, artifacts []ReleaseArtifact) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	// 1. Ensure service exists or update
	upsertService := `
	INSERT INTO services (id, name, active_release_id, updated_at)
	VALUES (?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		active_release_id = excluded.active_release_id,
		updated_at = excluded.updated_at;
	`
	if _, upsertErr := tx.ExecContext(ctx, upsertService, rel.ServiceID, rel.ServiceID, rel.ID, nowStr); upsertErr != nil {
		return fmt.Errorf("failed upserting service: %w", upsertErr)
	}

	// 2. Mark any current active release for this service as inactive
	inactivateOld := `UPDATE releases SET status = 'inactive' WHERE service_id = ? AND status = 'active';`
	if _, inactErr := tx.ExecContext(ctx, inactivateOld, rel.ServiceID); inactErr != nil {
		return fmt.Errorf("failed inactivating old releases: %w", inactErr)
	}

	// 3. Insert new release
	insertRelease := `
	INSERT INTO releases (id, service_id, release_version, status, manifest_yaml, signature, public_key, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?);
	`
	if _, insertErr := tx.ExecContext(ctx, insertRelease, rel.ID, rel.ServiceID, rel.ReleaseVersion, "active", rel.ManifestYAML, rel.Signature, rel.PublicKey, nowStr); insertErr != nil {
		return fmt.Errorf("failed inserting release: %w", insertErr)
	}

	// 4. Insert artifacts
	insertArtifact := `
	INSERT INTO release_artifacts (id, release_id, rel_path, sha256_hash, size_bytes, created_at)
	VALUES (?, ?, ?, ?, ?, ?);
	`
	stmt, err := tx.PrepareContext(ctx, insertArtifact)
	if err != nil {
		return fmt.Errorf("failed preparing artifact statement: %w", err)
	}

	for _, a := range artifacts {
		if _, err := stmt.ExecContext(ctx, a.ID, rel.ID, a.RelPath, a.SHA256Hash, a.SizeBytes, nowStr); err != nil {
			_ = stmt.Close()
			return fmt.Errorf("failed inserting artifact %s: %w", a.RelPath, err)
		}
	}
	if closeErr := stmt.Close(); closeErr != nil {
		return fmt.Errorf("failed closing prepared statement: %w", closeErr)
	}

	return tx.Commit()
}

// GetActiveRelease returns the currently active release for a service.
func (s *DB) GetActiveRelease(ctx context.Context, serviceID string) (*Release, error) {
	query := `SELECT id, service_id, release_version, status, manifest_yaml, signature, public_key, created_at FROM releases WHERE service_id = ? AND status = 'active' LIMIT 1;`
	rows, err := s.db.QueryContext(ctx, query, serviceID)
	if err != nil {
		return nil, fmt.Errorf("failed executing query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("row error: %w", err)
		}
		return nil, fmt.Errorf("no active release for service: %s", serviceID)
	}

	rel := &Release{}
	var createdAtStr string
	if err := rows.Scan(&rel.ID, &rel.ServiceID, &rel.ReleaseVersion, &rel.Status, &rel.ManifestYAML, &rel.Signature, &rel.PublicKey, &createdAtStr); err != nil {
		return nil, fmt.Errorf("failed scanning active release: %w", err)
	}
	if parsedTime, parseErr := time.Parse(time.RFC3339, createdAtStr); parseErr == nil {
		rel.CreatedAt = parsedTime
	}
	return rel, nil
}

// ListServices returns all registered services and their current active release ID.
func (s *DB) ListServices(ctx context.Context) ([]Service, error) {
	query := `SELECT id, name, COALESCE(active_release_id, ''), updated_at FROM services ORDER BY id ASC;`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed querying services: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var services []Service
	for rows.Next() {
		var svc Service
		var updatedAtStr string
		if err := rows.Scan(&svc.ID, &svc.Name, &svc.ActiveReleaseID, &updatedAtStr); err != nil {
			return nil, fmt.Errorf("failed scanning service row: %w", err)
		}
		if parsedTime, parseErr := time.Parse(time.RFC3339, updatedAtStr); parseErr == nil {
			svc.UpdatedAt = parsedTime
		}
		services = append(services, svc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during services iteration: %w", err)
	}
	return services, nil
}

// GetReleaseArtifacts returns all artifacts registered under a specific release.
func (s *DB) GetReleaseArtifacts(ctx context.Context, releaseID string) ([]ReleaseArtifact, error) {
	query := `SELECT id, release_id, rel_path, sha256_hash, size_bytes, created_at FROM release_artifacts WHERE release_id = ?;`
	rows, err := s.db.QueryContext(ctx, query, releaseID)
	if err != nil {
		return nil, fmt.Errorf("failed querying release artifacts: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var artifacts []ReleaseArtifact
	for rows.Next() {
		var a ReleaseArtifact
		var createdAtStr string
		if err := rows.Scan(&a.ID, &a.ReleaseID, &a.RelPath, &a.SHA256Hash, &a.SizeBytes, &createdAtStr); err != nil {
			return nil, fmt.Errorf("failed scanning artifact row: %w", err)
		}
		if parsedTime, parseErr := time.Parse(time.RFC3339, createdAtStr); parseErr == nil {
			a.CreatedAt = parsedTime
		}
		artifacts = append(artifacts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during artifacts iteration: %w", err)
	}
	return artifacts, nil
}

// AppendAuditLog atomically appends a new audit record to the hash chain.
func (s *DB) AppendAuditLog(ctx context.Context, eventType, entityID, actor string, payload []byte) (*audit.Record, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed beginning audit tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var lastSeq int64
	var lastHash string
	row := tx.QueryRowContext(ctx, "SELECT sequence, record_hash FROM audit_log ORDER BY sequence DESC LIMIT 1;")
	if err := row.Scan(&lastSeq, &lastHash); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("failed fetching latest audit record: %w", err)
		}
		lastSeq = 0
		lastHash = audit.GenesisHash
	}

	newSeq := lastSeq + 1
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	rec := audit.NewRecord(lastHash, newSeq, now, eventType, entityID, actor, payload)

	insertQuery := `
	INSERT INTO audit_log (sequence, timestamp, event_type, entity_id, actor, payload_digest, prev_record_hash, record_hash)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?);
	`
	if _, err := tx.ExecContext(ctx, insertQuery, rec.Sequence, nowStr, rec.EventType, rec.EntityID, rec.Actor, rec.PayloadDigest, rec.PrevRecordHash, rec.RecordHash); err != nil {
		return nil, fmt.Errorf("failed inserting audit record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed committing audit record: %w", err)
	}

	return &rec, nil
}

// ListAuditLogs returns paginated audit records ordered by sequence.
func (s *DB) ListAuditLogs(ctx context.Context, limit, offset int) ([]audit.Record, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	query := "SELECT sequence, timestamp, event_type, entity_id, actor, payload_digest, prev_record_hash, record_hash FROM audit_log ORDER BY sequence ASC LIMIT ? OFFSET ?;"
	rows, err := s.db.QueryContext(ctx, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed querying audit log: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []audit.Record
	for rows.Next() {
		var rec audit.Record
		var tsStr string
		if err := rows.Scan(&rec.Sequence, &tsStr, &rec.EventType, &rec.EntityID, &rec.Actor, &rec.PayloadDigest, &rec.PrevRecordHash, &rec.RecordHash); err != nil {
			return nil, fmt.Errorf("failed scanning audit row: %w", err)
		}
		if t, parseErr := time.Parse(time.RFC3339, tsStr); parseErr == nil {
			rec.Timestamp = t
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during audit rows iteration: %w", err)
	}
	return records, nil
}

// VerifyAuditTrail reads all audit logs and verifies the mathematical integrity of the hash chain.
func (s *DB) VerifyAuditTrail(ctx context.Context) (*audit.VerificationReport, error) {
	query := "SELECT sequence, timestamp, event_type, entity_id, actor, payload_digest, prev_record_hash, record_hash FROM audit_log ORDER BY sequence ASC;"
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed querying audit trail for verification: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var records []audit.Record
	for rows.Next() {
		var rec audit.Record
		var tsStr string
		if err := rows.Scan(&rec.Sequence, &tsStr, &rec.EventType, &rec.EntityID, &rec.Actor, &rec.PayloadDigest, &rec.PrevRecordHash, &rec.RecordHash); err != nil {
			return nil, fmt.Errorf("failed scanning audit row: %w", err)
		}
		if t, parseErr := time.Parse(time.RFC3339, tsStr); parseErr == nil {
			rec.Timestamp = t
		}
		records = append(records, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error during audit rows scan: %w", err)
	}

	return audit.VerifyChain(records), nil
}
