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

# E2E Test Report Generator Wrapper
# Runs multi-tier E2E tests with `go test -json`, then generates
# a sanitized, standalone HTML dashboard via generate_e2e_report.py.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
REPORT_DIR="${PROJECT_ROOT}/test_reports"
RAW_JSON="${REPORT_DIR}/e2e_raw.json"
HTML_REPORT="${REPORT_DIR}/index.html"
HISTORY_FILE="${REPORT_DIR}/history.json"

echo "============================================================"
echo "  E2E Test Report Generation"
echo "============================================================"

mkdir -p "$REPORT_DIR"

# --- Step 1: Run E2E tests with JSON output ---
echo "==> Building binaries..."
cd "$PROJECT_ROOT"
make build 2>&1

echo "==> Running E2E tests with JSON output..."
# Run sqlite-e2e test script inline to capture JSON output
# Uses go test -json for structured output
set +e
go test -v -json -race -count=1 ./... 2>&1 | tee "$RAW_JSON"
TEST_EXIT=$?
set -e

echo "==> E2E test execution completed (exit code: $TEST_EXIT)"

# --- Step 2: Generate HTML report ---
echo "==> Generating HTML report from test results..."
python3 "$SCRIPT_DIR/generate_e2e_report.py" "$RAW_JSON" "$HTML_REPORT" "$HISTORY_FILE"

echo ""
echo "============================================================"
echo "  Report Generated Successfully"
echo "  HTML:    $HTML_REPORT"
echo "  History: $HISTORY_FILE"
echo "============================================================"

if [ $TEST_EXIT -ne 0 ]; then
    echo "WARNING: Some tests failed. See report for details."
fi

exit $TEST_EXIT
