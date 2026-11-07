#!/usr/bin/env bash
# Copyright 2026 ytagarasu & onpremise-oss-develop-org
# Test Tampering Guardrail Enforcement Script
set -euo pipefail

TARGET_BASE="${1:-origin/main}"

echo "=== [GUARDRAIL] Inspecting test modifications against ${TARGET_BASE} ==="

DIFF_OUTPUT=$(git diff "${TARGET_BASE}" -- '*_test.go' || true)

if [ -z "${DIFF_OUTPUT}" ]; then
  echo "✓ No test files modified or no diff detected."
  exit 0
fi

VIOLATIONS=0

# 1. Check for suspicious assertion / test deletions
DELETED_ASSERTIONS=$(echo "${DIFF_OUTPUT}" | grep -E "^-[[:space:]]*(assert\.|require\.|t\.(Fatal|Error|Fail|FailNow)|if err != nil)" || true)
if [ -n "${DELETED_ASSERTIONS}" ]; then
  echo "❌ [VIOLATION: TEST_DELETION] Deleted test assertions detected:"
  echo "${DELETED_ASSERTIONS}"
  VIOLATIONS=$((VIOLATIONS + 1))
fi

# 2. Check for unauthorized test skipping
ADDED_SKIPS=$(echo "${DIFF_OUTPUT}" | grep -E "^\+[[:space:]]*(t\.Skip|t\.Skipf|t\.SkipNow)" || true)
if [ -n "${ADDED_SKIPS}" ]; then
  echo "❌ [VIOLATION: TEST_SKIPPING] New t.Skip() bypass detected:"
  echo "${ADDED_SKIPS}"
  VIOLATIONS=$((VIOLATIONS + 1))
fi

if [ "${VIOLATIONS}" -gt 0 ]; then
  echo "🚨 Test Tampering Guardrail: FAILED with ${VIOLATIONS} violations. Pull Request cannot proceed."
  exit 1
fi

echo "✓ [GUARDRAIL PASSED] All test modifications comply with the Additive-Only test policy."
exit 0
