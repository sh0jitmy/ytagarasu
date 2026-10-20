# 🐦‍⬛ ytagarasu Documentation (English)

<p align="center">
  <a href="../manual.md"><b>🇯🇵 日本語マニュアル</b></a> | <a href="index.md"><b>🇺🇸 English Portal</b></a>
</p>

Welcome to the **ytagarasu (八咫烏)** documentation. This portal provides comprehensive technical guides, architecture specifications, and operational manuals in English.

---

## 📚 Table of Contents

1. **[System Architecture (architecture.md)](architecture.md)**
   - End-to-end design from online preparation to air-gapped distribution.
   - Component interactions (`ytg`, `ytagarasu-server`, `ytagarasu-agent`).
   - Cryptographic verification pipeline and Ed25519 signatures.
   - Pure Go WAL SQLite database design and SHA-256 CAS storage.

2. **[Operator Manual & Quick Start (manual.md)](manual.md)**
   - Installing and building ytagarasu binaries.
   - Declarative manifest schema (`manifest.yaml`).
   - Headless CLI operations via `ytg` (`status`, `inspect`, `restart`, `diff`).
   - Bundle packaging, signing, and verification.
   - Deploying containers and native background processes.
   - Disaster recovery and atomic rollback mechanism.

3. **[Architecture Decision Records (ADRs)](../adr/)**
   - [ADR-0001: Go Template Repository Design](../adr/0001-go-template-repository-design.md)
   - [ADR-0002: Schema-Driven Development & Custom Skills](../adr/0002-schema-driven-development-custom-skills.md)
   - [ADR-0003: Standalone HTMX Frontend Architecture](../adr/0003-standalone-htmx-frontend-architecture.md)
   - [ADR-0004: Multi-Tier E2E Testing Framework](../adr/0004-multi-tier-e2e-testing-framework.md)
   - [ADR-0005: Offline Deployment Platform Architecture](../adr/0005-offline-deployment-platform-architecture.md)
   - [ADR-0006: Manifest Discovery & Whitelist Workflow](../adr/0006-manifest-discovery-and-whitelist-workflow.md)

---

## 🏛️ System Philosophy: The Three Pillars

- **Pure Go & Zero-Cgo Portability**: Single static binary deployments with no runtime dependencies.
- **Strict Air-Gap Completeness**: Everything required to install, run, supervise, and rollback operates without internet access or external package managers.
- **KISS & Zero-Magic Operations**: Declarative YAML configurations, guard clauses, left-aligned error returns, and predictable failure-safe fallbacks.
