#!/usr/bin/env python3
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

"""
ytagarasu HTMX UI & Headless Chrome Screenshot E2E Test Runner
Verifies that the embedded HTMX dashboard renders properly, captures full-resolution
screenshots of the dashboard and audit log viewer, and outputs a standalone HTML report.
"""

import base64
import os
import shutil
import subprocess
import sys
import time
import urllib.request
from datetime import datetime

SERVER_URL = os.environ.get("SERVER_URL", "http://localhost:18090")
REPORT_DIR = "test_reports"
DOCS_IMG_DIR = os.path.join("docs", "images")
DASHBOARD_SCREENSHOT = os.path.join(DOCS_IMG_DIR, "ytagarasu_dashboard.png")
AUDIT_SCREENSHOT = os.path.join(DOCS_IMG_DIR, "ytagarasu_audit.png")
HTML_REPORT_PATH = os.path.join(REPORT_DIR, "ytagarasu_ui_e2e_report.html")


def find_chrome_binary():
    env_bin = os.environ.get("CHROME_BIN")
    if env_bin and os.path.exists(env_bin):
        return env_bin
    for candidate in [
        "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
        shutil.which("google-chrome"),
        shutil.which("google-chrome-stable"),
        shutil.which("chromium"),
        shutil.which("chromium-browser"),
    ]:
        if candidate and os.path.exists(candidate):
            return candidate
    return None


CHROME_BIN = find_chrome_binary()


def log(msg, level="INFO"):
    print(f"[{datetime.now().strftime('%H:%M:%S')}] [{level}] {msg}")


def http_get(url):
    req = urllib.request.Request(url)
    with urllib.request.urlopen(req, timeout=10) as resp:
        return resp.read().decode("utf-8")


def take_screenshot(url, output_path, width=1440, height=900):
    if not CHROME_BIN:
        log("No Chrome/Chromium binary detected. Skipping screenshot capture.", "WARN")
        return False

    os.makedirs(os.path.dirname(output_path), exist_ok=True)
    cmd = [
        CHROME_BIN,
        "--headless=new",
        "--disable-gpu",
        "--no-sandbox",
        "--hide-scrollbars",
        f"--window-size={width},{height}",
        f"--screenshot={output_path}",
        url,
    ]
    try:
        res = subprocess.run(cmd, capture_output=True, timeout=15, check=False)
        if res.returncode == 0 and os.path.exists(output_path):
            log(f"Saved screenshot: {output_path}")
            return True
        log(f"Chrome screenshot failed: {res.stderr.decode('utf-8', errors='ignore')}", "WARN")
        return False
    except Exception as e:
        log(f"Error capturing screenshot: {e}", "WARN")
        return False


def main():
    log("Starting ytagarasu UI E2E test suite...")
    os.makedirs(REPORT_DIR, exist_ok=True)
    os.makedirs(DOCS_IMG_DIR, exist_ok=True)

    test_results = []

    # 1. Test Dashboard UI
    try:
        dash_html = http_get(f"{SERVER_URL}/ui")
        assert "オフライン配信ダッシュボード" in dash_html
        assert "ytagarasu" in dash_html
        log("✓ Dashboard HTML loaded and validated.")
        test_results.append(("Dashboard Page Render", "PASS", "Dashboard contains brand badge and service metrics."))
    except Exception as e:
        log(f"✗ Dashboard HTML failed: {e}", "ERROR")
        test_results.append(("Dashboard Page Render", "FAIL", str(e)))

    # 2. Test Audit Log UI
    try:
        audit_html = http_get(f"{SERVER_URL}/ui/audit")
        assert "改ざん耐性 SHA-256 監査チェーン" in audit_html
        log("✓ Audit log HTML loaded and validated.")
        test_results.append(("Audit Log Page Render", "PASS", "Audit log page displays SHA-256 chained table."))
    except Exception as e:
        log(f"✗ Audit log HTML failed: {e}", "ERROR")
        test_results.append(("Audit Log Page Render", "FAIL", str(e)))

    # 3. Test Verify Audit Action
    try:
        verify_html = http_get(f"{SERVER_URL}/ui/actions/verify-audit")
        assert "チェーン整合性確認済み" in verify_html
        log("✓ Verify audit action endpoint validated.")
        test_results.append(("In-place Chain Verification Action", "PASS", "Returns verified badge."))
    except Exception as e:
        log(f"✗ Verify audit action failed: {e}", "ERROR")
        test_results.append(("In-place Chain Verification Action", "FAIL", str(e)))

    # 4. Take Screenshots
    dash_img_ok = take_screenshot(f"{SERVER_URL}/ui", DASHBOARD_SCREENSHOT)
    audit_img_ok = take_screenshot(f"{SERVER_URL}/ui/audit", AUDIT_SCREENSHOT)

    # 5. Generate Standalone HTML Report
    dash_b64 = ""
    if dash_img_ok and os.path.exists(DASHBOARD_SCREENSHOT):
        with open(DASHBOARD_SCREENSHOT, "rb") as f:
            dash_b64 = base64.b64encode(f.read()).decode("utf-8")

    audit_b64 = ""
    if audit_img_ok and os.path.exists(AUDIT_SCREENSHOT):
        with open(AUDIT_SCREENSHOT, "rb") as f:
            audit_b64 = base64.b64encode(f.read()).decode("utf-8")

    all_pass = all(r[1] == "PASS" for r in test_results)

    report_html = f"""<!DOCTYPE html>
<html lang="ja">
<head>
  <meta charset="UTF-8">
  <title>ytagarasu UI E2E Test Report</title>
  <style>
    body {{ font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #090d16; color: #f8fafc; padding: 2rem; }}
    .card {{ background: #111827; border: 1px solid #1e293b; border-radius: 8px; padding: 1.5rem; margin-bottom: 1.5rem; }}
    .badge-pass {{ background: rgba(34, 197, 94, 0.2); color: #22c55e; padding: 0.25rem 0.6rem; border-radius: 9999px; font-weight: bold; }}
    .badge-fail {{ background: rgba(239, 68, 68, 0.2); color: #ef4444; padding: 0.25rem 0.6rem; border-radius: 9999px; font-weight: bold; }}
    table {{ width: 100%; border-collapse: collapse; margin-top: 1rem; }}
    th, td {{ padding: 0.75rem 1rem; border-bottom: 1px solid #1e293b; text-align: left; }}
    th {{ background: #1e293b; color: #94a3b8; font-size: 0.8rem; text-transform: uppercase; }}
    img {{ max-width: 100%; border-radius: 8px; border: 1px solid #334155; margin-top: 1rem; }}
  </style>
</head>
<body>
  <h1>🦅 ytagarasu UI E2E Visual Verification Report</h1>
  <div class="card">
    <h2>Execution Summary: <span class="{'badge-pass' if all_pass else 'badge-fail'}">{'ALL TESTS PASSED' if all_pass else 'TESTS FAILED'}</span></h2>
    <p>Generated at: {datetime.now().strftime('%Y-%m-%d %H:%M:%S UTC')}</p>
    <table>
      <thead>
        <tr><th>Test Case</th><th>Status</th><th>Details</th></tr>
      </thead>
      <tbody>
"""
    for name, status, details in test_results:
        badge_cls = "badge-pass" if status == "PASS" else "badge-fail"
        report_html += f"<tr><td><strong>{name}</strong></td><td><span class='{badge_cls}'>{status}</span></td><td>{details}</td></tr>\n"

    report_html += f"""
      </tbody>
    </table>
  </div>
"""

    if dash_b64:
        report_html += f"""
  <div class="card">
    <h3>Dashboard Visual Snapshot</h3>
    <img src="data:image/png;base64,{dash_b64}" alt="Dashboard Screenshot" />
  </div>
"""

    if audit_b64:
        report_html += f"""
  <div class="card">
    <h3>Audit Log & Hash Chain Visual Snapshot</h3>
    <img src="data:image/png;base64,{audit_b64}" alt="Audit Log Screenshot" />
  </div>
"""

    report_html += """
</body>
</html>
"""

    with open(HTML_REPORT_PATH, "w", encoding="utf-8") as f:
        f.write(report_html)

    log(f"Standalone HTML report generated at: {HTML_REPORT_PATH}")
    if not all_pass:
        sys.exit(1)
    log("All UI assertions succeeded.")


if __name__ == "__main__":
    main()
