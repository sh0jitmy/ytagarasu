<!--
Copyright 2026 [Copyright Holder]

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Author: [YOUR_NAME]
-->

# System Architecture & Technical Specifications (English)

<p align="center">
  <a href="../architecture.md"><b>🇯🇵 日本語アーキテクチャ</b></a> | <a href="architecture.md"><b>🇺🇸 English Architecture</b></a>
</p>

This document details the complete system architecture, component boundaries, cryptographic pipelines, and data flow for **ytagarasu (八咫烏)**.

---

## 1. High-Level Architecture Overview

```
 [ Online Build / Preparation Host ]
        │
        ├── 1. manifest.yaml definition & preflight risk evaluation (ytagarasu manifest init / eval)
        ├── 2. APT / RPM dependency recursive resolution & caching (pkgengine)
        ├── 3. Ed25519 digital signature & full/delta bundle generation (ytagarasu bundle export)
        │
 ═══════╪════════════════════════════════════════════════════════════════════════
        │  Physical Media Transport (Encrypted USB / External Storage)
 ═══════╪════════════════════════════════════════════════════════════════════════
        ▼
 [ Offline / Air-Gapped Environment ]
        │
   ┌────┴───────────────────────────────────────────────────────────────────┐
   │ 4. Offline Distribution Server (ytagarasu-server)                      │
   │    ├── Cryptographic signature & expiration validation pipeline        │
   │    ├── Content-Addressable Storage (SHA-256 CAS)                       │
   │    ├── Pure Go CGO-free WAL SQLite release management & virtual repos   │
   │    ├── Tamper-evident SHA-256 audit hash-chain (/api/v1/audit/...)     │
   │    ├── Prometheus-compatible metrics (/metrics) & health probes        │
   │    └── Zero-CDN embedded HTMX operational dashboard (/ui)              │
   └────┬───────────────────────────────────────────────────────────────────┘
        │ HTTP (Desired State Polling)
        ▼
   ┌────────────────────────────────────────────────────────────────────────┐
   │ 5. Autonomous Agent & Process Supervisor (ytagarasu-agent / ytg)       │
   │    ├── 3-character headless CLI 'ytg' (status, inspect, restart)       │
   │    ├── Internal zero-cron log rotation & disk space protection         │
   │    ├── Linux cgroup v2 memory.events monitoring & OOM recovery         │
   │    ├── flock mutual exclusion guard preventing concurrency races       │
   │    ├── Non-interactive package installation (DEBIAN_FRONTEND / -y)     │
   │    ├── renameat atomic swap & immediate snapshot recovery on failure   │
   │    └── Pre/post HTTP & command health probes                           │
   └────────────────────────────────────────────────────────────────────────┘
```

---

## 2. Core Components & Responsibilities

### ① Headless Supervisor & CLI (`cmd/ytg`, `pkg/supervisor`)
- Lightweight process supervisor designed for edge servers where systemd/k8s are impractical or prohibited.
- Manages native background processes and Docker container lifecycles from a single declarative manifest.
- Features cgroup v2 memory monitoring, zero-cron log rotation, exponential backoff retries, and host port conflict pre-checks.

### ② Cryptographic Bundle & Verification Pipeline (`internal/bundle`, `internal/manifest`)
- Gzip/Tar archives containing `manifest.yaml`, application payloads, package pools, and digital signatures.
- Secured by asymmetric Ed25519 keys; untrusted or modified archives are rejected prior to unpacking.
- Enforces preflight validation for 0-byte, truncated, or corrupt archives.

### ③ Offline Distribution Server (`cmd/ytagarasu-server`, `internal/server`)
- Embedded HTTP server serving static assets, release APIs, and virtual package mirrors.
- Implements SHA-256 Content-Addressable Storage (CAS) for deduplicated package storage.
- Operates on a pure Go WAL SQLite database requiring zero Cgo dependencies.

### ④ Tamper-Evident Audit Trail (`internal/audit`)
- Cryptographic hash-chain linking all deployment events.
- Any unauthorized direct database modification breaks the SHA-256 chain and triggers instant alerting during audits.
