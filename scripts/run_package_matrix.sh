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

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

echo "================================================================"
echo "   ytagarasu Full Package Matrix Verification Suite             "
echo "   Ecosystems: APT (Debian/Ubuntu), DNF (RHEL/Rocky),           "
echo "               Pip (Python Wheels), Docker (Container Tarballs) "
echo "================================================================"

echo ""
echo "==> [1/3] Running Package Manager Unit Tests..."
go test -v -race ./internal/agent -run 'Test(Apt|Rpm|Pip|Docker|Multi)PackageManager'
echo "✓ Unit tests passed for all package managers."

echo ""
echo "==> [2/3] Running Full-Matrix Offline Deployment E2E Tests (-tags=matrix_test)..."
go test -tags=matrix_test -v -race ./test/e2e/... -run TestPackageMatrix_AllEcosystems
echo "✓ Full matrix E2E tests passed."

echo ""
echo "==> [3/3] Checking Tooling Availability on Host..."
for tool in apt-get dnf python3 docker; do
  if command -v "${tool}" >/dev/null 2>&1; then
    echo "  [FOUND]     ${tool}: $(command -v "${tool}")"
  else
    echo "  [SIMULATED] ${tool}: Not found on local host (using simulated safe runner)"
  fi
done

echo ""
echo "================================================================"
echo "   ✓ All Package Matrix E2E Tests Completed Successfully!       "
echo "================================================================"
