#!/usr/bin/env bash
# Copyright 2026 [Copyright Holder]
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# Author: [YOUR_NAME]

set -euo pipefail

CYAN='\033[0;36m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${CYAN}================================================================${NC}"
echo -e "${CYAN}   ytagarasu Multi-Tier Agent-Server & HTMX UI E2E Test Suite    ${NC}"
echo -e "${CYAN}================================================================${NC}"

WORK_DIR=$(mktemp -d -t ytagarasu-e2e-XXXXXX)
SERVER_DATA="$WORK_DIR/server_data"
AGENT_WORK="$WORK_DIR/agent_work"
APP_ETC="$WORK_DIR/etc_app"
mkdir -p "$SERVER_DATA" "$AGENT_WORK" "$APP_ETC"

SERVER_PORT=18090
SERVER_PID=""

cleanup() {
    echo -e "\n${YELLOW}===> [E2E Cleanup] Stopping ytagarasu-server and cleaning temporary directory...${NC}"
    if [ -n "$SERVER_PID" ] && kill -0 "$SERVER_PID" > /dev/null 2>&1; then
        kill "$SERVER_PID" > /dev/null 2>&1 || true
    fi
    rm -rf "$WORK_DIR"
    echo -e "${GREEN}===> [E2E Cleanup] Complete.${NC}"
}
trap cleanup EXIT INT TERM

# Step 1: Compile binaries
echo -e "\n${YELLOW}[Step 1/6] Compiling ytagarasu binaries...${NC}"
go build -o "$WORK_DIR/ytagarasu" ./cmd/ytagarasu
go build -o "$WORK_DIR/ytagarasu-server" ./cmd/ytagarasu-server
go build -o "$WORK_DIR/ytagarasu-agent" ./cmd/ytagarasu-agent
echo -e "${GREEN}✓ All binaries compiled successfully.${NC}"

# Step 2: Start ytagarasu-server
echo -e "\n${YELLOW}[Step 2/6] Starting ytagarasu-server on port ${SERVER_PORT}...${NC}"
"$WORK_DIR/ytagarasu-server" --listen ":${SERVER_PORT}" --data-dir "$SERVER_DATA" > "$WORK_DIR/server.log" 2>&1 &
SERVER_PID=$!

# Wait for server ready
for i in {1..30}; do
    if curl -s -f "http://127.0.0.1:${SERVER_PORT}/healthz" > /dev/null 2>&1; then
        echo -e "${GREEN}✓ ytagarasu-server is ready and healthy.${NC}"
        break
    fi
    sleep 0.2
    if [ "$i" -eq 30 ]; then
        echo -e "${RED}Server failed to start within timeout. Server logs:${NC}"
        cat "$WORK_DIR/server.log"
        exit 1
    fi
done

# Step 2.5: Reverse-engineering & Manifest Discovery Support (CLI)
echo -e "\n${YELLOW}[Step 2.5/6] Testing Reverse-engineering & Manifest Discovery Support (survey/generate)...${NC}"
MOCK_SYSROOT="$WORK_DIR/mock_sysroot"
mkdir -p "$MOCK_SYSROOT/etc/systemd/system" "$MOCK_SYSROOT/usr/local/bin" "$MOCK_SYSROOT/etc/billing"

echo '#!/bin/sh' > "$MOCK_SYSROOT/usr/local/bin/billing-worker"
echo 'echo "worker active"' >> "$MOCK_SYSROOT/usr/local/bin/billing-worker"
chmod +x "$MOCK_SYSROOT/usr/local/bin/billing-worker"

cat <<EOF > "$MOCK_SYSROOT/etc/billing/worker.env"
PORT=8080
DB_PASSWORD=secret_value_123
EOF

cat <<EOF > "$MOCK_SYSROOT/etc/systemd/system/billing-worker.service"
[Unit]
Description=Billing Worker Service

[Service]
ExecStart=/usr/local/bin/billing-worker --port 8080
EnvironmentFile=/etc/billing/worker.env
Restart=always
EOF

DISCOVER_PLAN="$WORK_DIR/test-discovery-plan.yaml"
DISCOVER_MANIFEST="$WORK_DIR/test-discovered-manifest.yaml"

# Run survey
"$WORK_DIR/ytagarasu" discover survey --sysroot "$MOCK_SYSROOT" -o "$DISCOVER_PLAN"
if [ ! -f "$DISCOVER_PLAN" ]; then
    echo -e "${RED}Discovery survey failed: $DISCOVER_PLAN not created!${NC}"
    exit 1
fi
grep -q "billing-worker.service" "$DISCOVER_PLAN"
echo -e "${GREEN}✓ CLI discover survey succeeded and generated discovery-plan.yaml.${NC}"

# Run generate (Mode A with --ingest)
"$WORK_DIR/ytagarasu" discover generate -p "$DISCOVER_PLAN" -o "$DISCOVER_MANIFEST" --sysroot "$MOCK_SYSROOT" --artifacts-dir "$WORK_DIR/discovered_artifacts" --configs-dir "$WORK_DIR/discovered_configs" --ingest
if [ ! -f "$DISCOVER_MANIFEST" ]; then
    echo -e "${RED}Discovery generate failed: $DISCOVER_MANIFEST not created!${NC}"
    exit 1
fi
grep -q "billing-worker" "$DISCOVER_MANIFEST"
# Verify SIRT secret sanitization
grep -q "<REDACTED_SECRET>" "$WORK_DIR/discovered_configs/billing-worker-worker.env.tmpl"
echo -e "${GREEN}✓ CLI discover generate succeeded with Mode A ingest and SIRT secret redaction.${NC}"

# Step 3: Key Generation & Bundle Packaging
echo -e "\n${YELLOW}[Step 3/6] Generating Ed25519 signing keys and building release bundle...${NC}"
KEY_DIR="$WORK_DIR/keys"
mkdir -p "$KEY_DIR"
"$WORK_DIR/ytagarasu" keygen -d "$KEY_DIR"

BUNDLE_SRC="$WORK_DIR/bundle_src"
mkdir -p "$BUNDLE_SRC/artifacts" "$BUNDLE_SRC/configs"
echo '#!/bin/sh' > "$BUNDLE_SRC/artifacts/billing-svc"
echo 'echo "billing service active"' >> "$BUNDLE_SRC/artifacts/billing-svc"
chmod +x "$BUNDLE_SRC/artifacts/billing-svc"

cat <<EOF > "$BUNDLE_SRC/configs/billing.conf.tmpl"
app_env={{.Environment}}
server_port={{.Port}}
status=running
EOF

cat <<EOF > "$BUNDLE_SRC/manifest.yaml"
version: "1.0"
bundleVersion: "2026.09.27.1"
release: "v1.0.0"
createdAt: "2026-09-27T00:00:00Z"
targets:
  - os: "ubuntu"
    release: "24.04"
    arch: "amd64"
applications:
  - name: "billing-svc"
    type: "golang"
    artifact: "artifacts/billing-svc"
    destination: "$WORK_DIR/bin_app/billing-svc"
    selector:
      roles: ["billing"]
configs:
  - template: "configs/billing.conf.tmpl"
    destination: "$APP_ETC/billing.conf"
    permissions: "0644"
    validateCommand: "cat {{.TempFile}}"
healthChecks:
  - type: "command"
    command: "test -f $APP_ETC/billing.conf"
    intervalSeconds: 1
    maxRetries: 2
rollback:
  autoOnFailure: true
  strategy: "immediate"
EOF

mkdir -p "$WORK_DIR/bin_app"
EXPORT_BUNDLE="$WORK_DIR/billing-bundle-v1.0.0.tar.gz"

"$WORK_DIR/ytagarasu" bundle export \
    --manifest "$BUNDLE_SRC/manifest.yaml" \
    --source "$BUNDLE_SRC" \
    --output "$EXPORT_BUNDLE" \
    --key "$KEY_DIR/private.key" \
    --force

echo -e "${GREEN}✓ Signed bundle exported: $EXPORT_BUNDLE${NC}"

# Step 4: Import Bundle via HTTP API
echo -e "\n${YELLOW}[Step 4/6] Importing bundle into ytagarasu-server CAS and SQLite...${NC}"
HTTP_CODE=$(curl -s -o "$WORK_DIR/import.json" -w "%{http_code}" -X POST "http://127.0.0.1:${SERVER_PORT}/api/v1/bundles/import" \
    -F "bundle=@$EXPORT_BUNDLE")

BODY=$(cat "$WORK_DIR/import.json")

if [ "$HTTP_CODE" != "201" ]; then
    echo -e "${RED}Import failed with code $HTTP_CODE: $BODY${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Bundle successfully imported (HTTP $HTTP_CODE): $BODY${NC}"

# Step 5: Run Autonomous Agent Deployment
echo -e "\n${YELLOW}[Step 5/6] Executing ytagarasu-agent deployment cycle...${NC}"
cat <<EOF > "$AGENT_WORK/agent.yaml"
server_url: "http://127.0.0.1:${SERVER_PORT}"
service_id: "billing-svc"
state_file: "$AGENT_WORK/state.json"
lock_file: "$AGENT_WORK/agent.lock"
install_dir: "$AGENT_WORK"
roles:
  - "billing"
hostname: "e2e-node-01"
EOF

# Run agent with --once
"$WORK_DIR/ytagarasu-agent" --config "$AGENT_WORK/agent.yaml" --once

# Verify generated config file
if [ ! -f "$APP_ETC/billing.conf" ]; then
    echo -e "${RED}Agent deployment failed: $APP_ETC/billing.conf not found!${NC}"
    exit 1
fi
grep -q "status=running" "$APP_ETC/billing.conf"
echo -e "${GREEN}✓ Target config file successfully rendered and verified.${NC}"

# Step 6: Verify HTMX UI and Audit Hash Chain
echo -e "\n${YELLOW}[Step 6/6] Verifying HTMX Dashboard, Audit UI, and Hash Chain...${NC}"

# 1. Dashboard UI check
UI_DASH=$(curl -s -f "http://127.0.0.1:${SERVER_PORT}/ui")
[[ "$UI_DASH" == *"オフライン配信ダッシュボード"* ]]
[[ "$UI_DASH" == *"billing-svc"* ]]
echo -e "${GREEN}✓ Dashboard UI rendered with active billing-svc release.${NC}"

# 2. Audit UI check
UI_AUDIT=$(curl -s -f "http://127.0.0.1:${SERVER_PORT}/ui/audit")
[[ "$UI_AUDIT" == *"改ざん耐性 SHA-256 監査チェーン"* ]]
[[ "$UI_AUDIT" == *"release.import"* ]]
echo -e "${GREEN}✓ Audit log UI rendered with SHA-256 chained records.${NC}"

# 3. Verify audit action endpoint
VERIFY_HTML=$(curl -s -f "http://127.0.0.1:${SERVER_PORT}/ui/actions/verify-audit")
[[ "$VERIFY_HTML" == *"チェーン整合性確認済み"* ]]
echo -e "${GREEN}✓ In-place HTMX audit verification action confirmed.${NC}"

# 4. CLI audit verify check
"$WORK_DIR/ytagarasu" audit verify --server "http://127.0.0.1:${SERVER_PORT}"
echo -e "${GREEN}✓ CLI audit verify succeeded with zero tampering.${NC}"

# 5. Manifest Discovery UI check
UI_DISCOVER=$(curl -s -f "http://127.0.0.1:${SERVER_PORT}/ui/discover")
[[ "$UI_DISCOVER" == *"マニフェスト作成支援"* ]]
[[ "$UI_DISCOVER" == *"Step 1: 安全な下見"* ]]
echo -e "${GREEN}✓ Manifest Discovery UI rendered properly.${NC}"

# 6. Manifest Discovery Survey Action check
UI_SURVEY=$(curl -s -f -X POST "http://127.0.0.1:${SERVER_PORT}/ui/discover/survey" -d "service=")
[[ "$UI_SURVEY" == *"Step 2: 検出された構成候補の取捨選択"* ]]
echo -e "${GREEN}✓ Manifest Discovery Survey action verified.${NC}"

# 7. Visual UI E2E & Snapshot generation
if command -v python3 > /dev/null 2>&1; then
    echo -e "\n${YELLOW}Running visual UI verification and screenshot capture...${NC}"
    SERVER_URL="http://127.0.0.1:${SERVER_PORT}" python3 scripts/test_ytagarasu_ui.py
    if [ -f "test_reports/ytagarasu_ui_e2e_report.html" ] && [ ! -f "test_reports/index.html" ]; then
        cp "test_reports/ytagarasu_ui_e2e_report.html" "test_reports/index.html"
    fi
fi

echo -e "\n${GREEN}================================================================${NC}"
echo -e "${GREEN}   ✓ All ytagarasu E2E Agent-Server and UI tests PASSED!         ${NC}"
echo -e "${GREEN}================================================================${NC}"
