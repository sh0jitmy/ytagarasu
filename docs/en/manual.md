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

# ytagarasu Operator Manual (English)

<p align="center">
  <a href="../manual.md"><b>🇯🇵 日本語マニュアル</b></a> | <a href="manual.md"><b>🇺🇸 English Manual</b></a>
</p>

This manual outlines operational procedures, command references, and troubleshooting workflows for **ytagarasu (八咫烏)** in air-gapped environments.

---

## 1. Quick Start Guide

### Building Binaries
```bash
git clone https://github.com/sh0jitmy/ytagarasu.git
cd ytagarasu
make build
```

This compiles all binaries into `bin/`:
- `bin/ytg`: Headless CLI & micro-supervisor.
- `bin/ytagarasu`: Manifest and bundle administration tool.
- `bin/ytagarasu-server`: Distribution repository and Web UI.
- `bin/ytagarasu-agent`: Node deployment agent.

---

## 2. Using the 3-Character CLI: `ytg`

### Supervise Native & Containerized Services
```bash
# Launch supervisor from manifest
./bin/ytg supervisor run -c manifest.yaml

# Check running processes and uptime
./bin/ytg status

# Inspect service metrics (memory, pid, restarts)
./bin/ytg inspect backend-api

# Gracefully restart a service
./bin/ytg restart backend-api
```

### Detect Drift with Desired State (`ytg diff`)
```bash
# Compare desired manifest with actual runtime state
./bin/ytg diff -f manifest.yaml -s .ytg/supervisor_state.json

# Approve and apply detected changes
./bin/ytg diff -f manifest.yaml -s .ytg/supervisor_state.json --approve
```

---

## 3. Creating & Signing Deployment Bundles

### 1. Key Generation
```bash
./bin/ytagarasu keygen -d ./keys
```

### 2. Export & Sign Bundle
```bash
./bin/ytagarasu bundle export \
    --manifest examples/manifest.yaml \
    --source examples/ \
    --output ./bundle-v1.0.0.tar.gz \
    --key ./keys/private.key \
    --force
```

### 3. Verify Bundle Integrity
```bash
./bin/ytagarasu bundle verify \
    --bundle ./bundle-v1.0.0.tar.gz \
    --key ./keys/public.key
```

---

## 4. Disaster Recovery & Rollback

- **Atomic File Swaps**: ytagarasu uses atomic directory renames (`renameat`) during deployment.
- **Automatic Snapshotting**: A full backup snapshot is stored prior to applying changes.
- **Automated Rollback**: If health checks or startup verification fails, ytagarasu rolls back to the previous snapshot instantly with zero manual intervention required.
