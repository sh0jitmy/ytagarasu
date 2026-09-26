# AGENTS.md - Antigravity Agent Guidelines

This document provides behavioral constraints, architectural conventions, and execution workflows for Google Antigravity and other AI coding agents working in this repository.

## Repository Overview

`go_template` is an enterprise-grade Go repository template providing:
1. **Zero-npm Standalone HTMX Dashboard & SSG**: High-performance UI server (`internal/web`) using `//go:embed`, local HTMX (`static/js/htmx.min.js`), and pre-rendered static site export.
2. **SQLite Governance & Reliability**: CGO-free WAL-mode persistence, SHA-256 verified backup archives (`tar.gz`), atomic transactional restore, and retention cleaner.
3. **Multi-Tier E2E Testing Framework & HTML Reports**: Fast No-Docker SQLite E2E (`make sqlite-e2e`), Headless Chrome frontend UI assertions (`make frontend-e2e`), Docker Compose full-stack E2E (`make docker-e2e`), and sanitized HTML report export with GitHub Pages automated deployment (`make e2e-report`).
4. **Production Observability & Dewy Deployment**: VictoriaMetrics, Prometheus metric endpoint (`/metrics`), pprof (`127.0.0.1:6060`), pre-provisioned Grafana dashboards (`deploy/grafana/`), and Dewy pull-based binary auto-deployment templates (`deploy/dewy/`).
5. **SSOT Version Management & Governance**: Version centrally defined in `internal/version/version.go`, Go version unified across GitHub Actions and `go.mod` via `go-version-file: 'go.mod'`, release automation via GoReleaser v2, zero-lint tolerance, and UX documentation governance.

---

## Agent Behavioral Rules

1. **Custom Skills First**:
   - Relevant skills are located in `.agents/skills/` and `.claude/skills/`.
   - Adhere to `golang-htmx-frontend`, `golang-sqlite-governance`, `multi-tier-e2e-testing`, `observability-stack`, `dewy-deployment`, `github-pages-e2e-report`, `documentation-governance`, and `golang-lint-governance` for architecture and implementation decisions.
2. **License Header Integrity**:
   - Every Go source file must contain the standard Apache-2.0 and Author header.
   - Always verify with `make license-check` before concluding tasks.
3. **SSOT Principle**:
   - Never hardcode Go versions in GitHub Actions workflows; always use `go-version-file: 'go.mod'`.
   - Version injection must go through `internal/version.Version`.
4. **Test Isolation**:
   - In Go unit/integration tests (`internal/...`), isolate SQLite memory databases using distinct connection names (`fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())`) to ensure parallel safety.
5. **Requirements Compliance**:
   - After updating features or configurations, execute `make self-eval` and ensure 100% compliance in `REQUIREMENTS.md`.

---

## Standard Development & Testing Commands

```bash
# Code generation & formatting
make generate
make fmt
make lint

# Verification & Multi-Tier E2E
make test            # Unit tests with -race and coverage check
make sqlite-e2e      # Ultra-fast No-Docker SQLite E2E
make frontend-e2e    # Headless Chrome HTMX UI & snapshot verification
make docker-e2e      # Multi-container Docker Compose E2E
make ssg-build       # Static site generation export
make e2e-report      # Run E2E tests and generate standalone HTML report

# Release & Governance
make release-check   # Validate GoReleaser v2 configuration
make license-check   # Verify license headers
make check           # Validate skill frontmatter
make workflow-check  # Validate GitHub Actions workflows
make self-eval       # Update REQUIREMENTS.md checklist & compliance score
```
