#!/usr/bin/env bash
# Copyright 2026 ytagarasu & onpremise-oss-develop-org
# Loop Detection Circuit Breaker Verification Script
set -euo pipefail

LOG_FILE="${1:-}"
MAX_CONSECUTIVE_ERRORS=3

echo "=== [CIRCUIT BREAKER] Evaluating Execution State ==="

if [ -z "${LOG_FILE}" ] || [ ! -f "${LOG_FILE}" ]; then
  echo "✓ No log file provided or log file empty. Circuit status: CLOSED (Normal)."
  exit 0
fi

# Count recurring error signatures
RECURRING_ERRORS=$(grep -i "error:" "${LOG_FILE}" | sort | uniq -c | sort -nr | head -n 1 || true)

if [ -n "${RECURRING_ERRORS}" ]; then
  COUNT=$(echo "${RECURRING_ERRORS}" | awk '{print $1}')
  MSG=$(echo "${RECURRING_ERRORS}" | cut -d' ' -f2-)

  if [ "${COUNT}" -ge "${MAX_CONSECUTIVE_ERRORS}" ]; then
    echo "🚨 [CIRCUIT BREAKER TRIPPED -> OPEN] Infinite loop detected!"
    echo "Identical error occurred ${COUNT} times (Threshold: ${MAX_CONSECUTIVE_ERRORS}):"
    echo "  Signature: ${MSG}"
    echo "Action required: Cease immediate retry. Fall back to alternative approach or escalate."
    exit 2
  fi
fi

echo "✓ [CIRCUIT CLOSED] Normal execution. Consecutive errors within threshold."
exit 0
