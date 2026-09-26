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
E2E Test Report Generator for GitHub Pages & CI Artifacts.
Parses `go test -json` output, sanitizes sensitive data (tokens/secrets),
merges execution history, and generates a standalone, modern HTML dashboard.
"""

import datetime
import html
import json
import os
import re
import sys
import urllib.request
import urllib.error

MAX_HISTORY_ITEMS = 50

# Sensitive pattern regexes for masking
SENSITIVE_PATTERNS = [
    (re.compile(r'Bearer\s+([A-Za-z0-9_\-\.]{10,})', re.IGNORECASE), r'Bearer ***MASKED_TOKEN***'),
    (re.compile(r'(client_secret=)([A-Za-z0-9_\-\.]{8,})', re.IGNORECASE), r'\1***MASKED_SECRET***'),
    (re.compile(r'("client_secret":\s*")([^"]+)(")', re.IGNORECASE), r'\1***MASKED_SECRET***\3'),
    (re.compile(r'("access_token":\s*")([^"]+)(")', re.IGNORECASE), r'\1***MASKED_TOKEN***\3'),
    (re.compile(r'(token-val\s+)([A-Za-z0-9_\-\.]{8,})', re.IGNORECASE), r'\1***MASKED_TOKEN***'),
    (re.compile(r'(key-auth\s+)([A-Za-z0-9_\-\.]{10,})', re.IGNORECASE), r'\1***MASKED_KEY_AUTH***'),
    (re.compile(r'([A-Za-z0-9_\-\.]{10,}\.auth-key-[A-Za-z0-9_\-\.]{8,})', re.IGNORECASE), r'***MASKED_KEY_AUTH***'),
    (re.compile(r'(-----BEGIN [A-Z ]+PRIVATE KEY-----[\s\S]+?-----END [A-Z ]+PRIVATE KEY-----)'), r'***MASKED_PRIVATE_KEY***'),
]


def sanitize(text: str) -> str:
    """Mask sensitive tokens, credentials, and secrets from test output."""
    if not text:
        return ""
    sanitized = text
    for pattern, replacement in SENSITIVE_PATTERNS:
        sanitized = pattern.sub(replacement, sanitized)
    return sanitized


def fetch_previous_history(history_file: str, repo: str) -> list:
    """Load previous history from local file or live GitHub Pages."""
    # 1. Check local file first
    if os.path.isfile(history_file):
        try:
            with open(history_file, "r", encoding="utf-8") as f:
                data = json.load(f)
                if isinstance(data, list):
                    return data
        except Exception as e:
            print(f"Notice: Failed to parse local {history_file}: {e}", file=sys.stderr)

    # 2. Try fetching from GitHub Pages if repo is available (e.g. owner/repo)
    if repo and "/" in repo:
        owner, repo_name = repo.split("/", 1)
        pages_url = f"https://{owner}.github.io/{repo_name}/history.json"
        try:
            req = urllib.request.Request(pages_url, headers={"User-Agent": "E2E-Report-Generator"})
            with urllib.request.urlopen(req, timeout=5) as response:
                if response.status == 200:
                    data = json.loads(response.read().decode("utf-8"))
                    if isinstance(data, list):
                        print(f"Successfully fetched previous history from {pages_url}")
                        return data
        except Exception:
            # First run or 404 is expected
            pass

    return []


def parse_go_test_json(raw_json_file: str):
    """Parse go test -json output lines and extract structured test results."""
    tests = {}
    failed_tests = []
    total_count = 0
    passed_count = 0
    failed_count = 0
    skipped_count = 0
    total_duration = 0.0

    if not os.path.isfile(raw_json_file):
        return {
            "status": "ERROR",
            "passed": 0,
            "failed": 1,
            "skipped": 0,
            "total": 1,
            "duration": 0.0,
            "tests": [],
            "failed_tests": [{"name": "Execution", "error": f"Missing input file: {raw_json_file}"}],
        }

    with open(raw_json_file, "r", encoding="utf-8", errors="replace") as f:
        for line in f:
            line = line.strip()
            if not line or not line.startswith("{"):
                continue
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue

            test_name = event.get("Test")
            action = event.get("Action")
            output = event.get("Output", "")

            if not test_name:
                continue

            if test_name not in tests:
                tests[test_name] = {
                    "name": test_name,
                    "package": event.get("Package", ""),
                    "status": "UNKNOWN",
                    "duration": 0.0,
                    "output": [],
                }

            if output:
                tests[test_name]["output"].append(sanitize(output))

            if action in ("pass", "fail", "skip"):
                tests[test_name]["status"] = action.upper()
                elapsed = event.get("Elapsed", 0.0)
                tests[test_name]["duration"] = elapsed
                total_duration += elapsed

                if action == "pass":
                    passed_count += 1
                elif action == "fail":
                    failed_count += 1
                    failed_tests.append({
                        "name": test_name,
                        "duration": elapsed,
                        "output": "".join(tests[test_name]["output"]),
                    })
                elif action == "skip":
                    skipped_count += 1

    total_count = passed_count + failed_count + skipped_count
    overall_status = "SUCCESS" if (failed_count == 0 and total_count > 0) else "FAILURE"

    # Sort test list: failed tests first, then alphabetical
    test_list = sorted(
        tests.values(),
        key=lambda x: (0 if x["status"] == "FAIL" else 1, x["name"]),
    )

    return {
        "status": overall_status,
        "passed": passed_count,
        "failed": failed_count,
        "skipped": skipped_count,
        "total": total_count,
        "duration": round(total_duration, 3),
        "tests": test_list,
        "failed_tests": failed_tests,
    }


def generate_html_report(current_run: dict, history: list, repo: str, project_name: str = "") -> str:
    """Generate a self-contained, responsive modern HTML report."""
    if not project_name:
        project_name = repo.split("/")[-1] if repo else "Go Project"
    status = current_run["status"]
    is_success = status == "SUCCESS"
    badge_bg = "#10b981" if is_success else "#ef4444"
    badge_text = "PASSED" if is_success else "FAILED"

    commit_sha = current_run.get("commit_sha", "unknown")
    short_sha = commit_sha[:7] if len(commit_sha) >= 7 else commit_sha
    commit_url = f"https://github.com/{repo}/commit/{commit_sha}" if repo else "#"
    branch = current_run.get("branch", "unknown")
    timestamp = current_run.get("timestamp", datetime.datetime.now(datetime.timezone.utc).isoformat())

    # Build Failed Tests Section
    failed_section_html = ""
    if current_run["failed_tests"]:
        failed_items = []
        for ft in current_run["failed_tests"]:
            name = html.escape(ft["name"])
            out = html.escape(ft.get("output", ""))
            failed_items.append(f"""
            <div class="test-card failed">
                <div class="test-card-header">
                    <span class="status-badge fail">FAIL</span>
                    <span class="test-name">{name}</span>
                </div>
                <pre class="test-output">{out}</pre>
            </div>
            """)
        failed_section_html = f"""
        <section class="section">
            <h2 class="section-title text-danger">⚠️ Failed Tests ({len(current_run['failed_tests'])})</h2>
            <div class="test-list">
                {"".join(failed_items)}
            </div>
        </section>
        """

    # Build All Tests Table
    all_tests_rows = []
    for t in current_run["tests"]:
        t_name = html.escape(t["name"])
        t_status = t["status"]
        t_badge_class = "pass" if t_status == "PASS" else ("fail" if t_status == "FAIL" else "skip")
        t_dur = f"{t['duration']:.3f}s"
        t_out = html.escape("".join(t.get("output", [])))
        has_out = bool(t_out.strip())
        detail_btn = f'<button class="btn-toggle" onclick="toggleDetails(\'{t_name}\')">Log</button>' if has_out else ''
        detail_row = f'<tr id="detail-{t_name}" class="detail-row" style="display:none;"><td colspan="4"><pre class="test-output">{t_out}</pre></td></tr>' if has_out else ''

        all_tests_rows.append(f"""
        <tr>
            <td><span class="status-badge {t_badge_class}">{t_status}</span></td>
            <td class="font-mono">{t_name}</td>
            <td class="text-right font-mono">{t_dur}</td>
            <td class="text-center">{detail_btn}</td>
        </tr>
        {detail_row}
        """)

    # Build History Table
    history_rows = []
    for idx, h in enumerate(history):
        h_status = h.get("status", "UNKNOWN")
        h_class = "pass" if h_status == "SUCCESS" else "fail"
        h_badge = "PASS" if h_status == "SUCCESS" else "FAIL"
        h_sha = h.get("commit_sha", "")[:7]
        h_branch = html.escape(h.get("branch", "-"))
        h_time = html.escape(h.get("timestamp", "-")[:19].replace("T", " "))
        h_url = f"https://github.com/{repo}/commit/{h.get('commit_sha', '')}" if repo else "#"
        h_pass = h.get("passed", 0)
        h_fail = h.get("failed", 0)
        h_dur = f"{h.get('duration', 0):.2f}s"
        is_current = " (Current)" if idx == 0 else ""

        history_rows.append(f"""
        <tr>
            <td><span class="status-badge {h_class}">{h_badge}</span>{is_current}</td>
            <td><a href="{h_url}" target="_blank" class="link font-mono">{h_sha}</a></td>
            <td class="font-mono">{h_branch}</td>
            <td>{h_time} UTC</td>
            <td class="text-right font-mono text-success">{h_pass}</td>
            <td class="text-right font-mono text-danger">{h_fail}</td>
            <td class="text-right font-mono">{h_dur}</td>
        </tr>
        """)

    html_content = f"""<!DOCTYPE html>
<html lang="ja">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>E2E Test Report - {html.escape(project_name)}</title>
    <style>
        :root {{
            --bg-color: #0f172a;
            --surface-color: #1e293b;
            --surface-hover: #334155;
            --border-color: #334155;
            --text-primary: #f8fafc;
            --text-secondary: #94a3b8;
            --accent-color: #38bdf8;
            --success-color: #10b981;
            --danger-color: #ef4444;
            --warning-color: #f59e0b;
        }}
        * {{ margin: 0; padding: 0; box-sizing: border-box; }}
        body {{
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
            background-color: var(--bg-color);
            color: var(--text-primary);
            line-height: 1.6;
            padding: 2rem 1rem;
        }}
        .container {{ max-width: 1200px; margin: 0 auto; }}
        header {{
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding-bottom: 1.5rem;
            border-bottom: 1px solid var(--border-color);
            margin-bottom: 2rem;
            flex-wrap: wrap;
            gap: 1rem;
        }}
        h1 {{ font-size: 1.75rem; font-weight: 700; color: var(--text-primary); }}
        .header-sub {{ color: var(--text-secondary); font-size: 0.9rem; margin-top: 0.25rem; }}
        .badge-main {{
            background-color: {badge_bg};
            color: white;
            padding: 0.5rem 1.25rem;
            border-radius: 9999px;
            font-weight: 700;
            font-size: 1.1rem;
            letter-spacing: 0.05em;
            box-shadow: 0 4px 14px rgba(0,0,0,0.3);
        }}
        .metrics-grid {{
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 1rem;
            margin-bottom: 2rem;
        }}
        .metric-card {{
            background-color: var(--surface-color);
            border: 1px solid var(--border-color);
            border-radius: 0.75rem;
            padding: 1.25rem;
            box-shadow: 0 4px 6px -1px rgba(0, 0, 0, 0.2);
        }}
        .metric-label {{ font-size: 0.85rem; color: var(--text-secondary); font-weight: 600; text-transform: uppercase; }}
        .metric-value {{ font-size: 1.8rem; font-weight: 700; margin-top: 0.25rem; }}
        .text-success {{ color: var(--success-color); }}
        .text-danger {{ color: var(--danger-color); }}
        .text-warning {{ color: var(--warning-color); }}
        .text-accent {{ color: var(--accent-color); }}
        .meta-card {{
            background-color: var(--surface-color);
            border: 1px solid var(--border-color);
            border-radius: 0.75rem;
            padding: 1rem 1.5rem;
            margin-bottom: 2rem;
            display: flex;
            flex-wrap: wrap;
            gap: 2rem;
            font-size: 0.95rem;
        }}
        .meta-item span:first-child {{ color: var(--text-secondary); margin-right: 0.5rem; }}
        .font-mono {{ font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }}
        .link {{ color: var(--accent-color); text-decoration: none; }}
        .link:hover {{ text-decoration: underline; }}
        .section {{ margin-bottom: 2.5rem; }}
        .section-title {{ font-size: 1.3rem; margin-bottom: 1rem; font-weight: 600; }}
        .test-card {{
            background-color: var(--surface-color);
            border: 1px solid var(--border-color);
            border-radius: 0.5rem;
            margin-bottom: 1rem;
            overflow: hidden;
        }}
        .test-card.failed {{ border-left: 4px solid var(--danger-color); }}
        .test-card-header {{ padding: 1rem; display: flex; align-items: center; gap: 0.75rem; }}
        .test-name {{ font-weight: 600; font-family: ui-monospace, monospace; }}
        .status-badge {{
            display: inline-block;
            padding: 0.2rem 0.6rem;
            border-radius: 0.375rem;
            font-size: 0.75rem;
            font-weight: 700;
            text-transform: uppercase;
        }}
        .status-badge.pass {{ background-color: rgba(16, 185, 129, 0.2); color: var(--success-color); }}
        .status-badge.fail {{ background-color: rgba(239, 68, 68, 0.2); color: var(--danger-color); }}
        .status-badge.skip {{ background-color: rgba(245, 158, 11, 0.2); color: var(--warning-color); }}
        .test-output {{
            background-color: #090d16;
            color: #e2e8f0;
            padding: 1rem;
            font-size: 0.85rem;
            overflow-x: auto;
            white-space: pre-wrap;
            border-top: 1px solid var(--border-color);
            font-family: ui-monospace, monospace;
        }}
        table {{
            width: 100%;
            border-collapse: collapse;
            background-color: var(--surface-color);
            border: 1px solid var(--border-color);
            border-radius: 0.5rem;
            overflow: hidden;
        }}
        th, td {{
            padding: 0.85rem 1rem;
            text-align: left;
            border-bottom: 1px solid var(--border-color);
            font-size: 0.9rem;
        }}
        th {{ background-color: rgba(255, 255, 255, 0.03); color: var(--text-secondary); font-weight: 600; }}
        tr:hover td {{ background-color: var(--surface-hover); }}
        .text-right {{ text-align: right; }}
        .text-center {{ text-align: center; }}
        .btn-toggle {{
            background-color: var(--border-color);
            color: var(--text-primary);
            border: none;
            padding: 0.25rem 0.6rem;
            border-radius: 0.25rem;
            cursor: pointer;
            font-size: 0.75rem;
        }}
        .btn-toggle:hover {{ background-color: var(--accent-color); color: #0f172a; }}
        footer {{
            text-align: center;
            color: var(--text-secondary);
            font-size: 0.85rem;
            margin-top: 3rem;
            padding-top: 1.5rem;
            border-top: 1px solid var(--border-color);
        }}
    </style>
    <script>
        function toggleDetails(id) {{
            const row = document.getElementById('detail-' + id);
            if (row) {{
                row.style.display = row.style.display === 'none' ? 'table-row' : 'none';
            }}
        }}
    </script>
</head>
<body>
    <div class="container">
        <header>
            <div>
                <h1>{html.escape(project_name)} E2E Test Report</h1>
                <p class="header-sub">Multi-Tier End-to-End Test Verification Suite</p>
            </div>
            <div>
                <span class="badge-main">{badge_text}</span>
            </div>
        </header>

        <div class="meta-card">
            <div class="meta-item">
                <span>Execution Time:</span>
                <strong>{html.escape(timestamp[:19].replace('T', ' '))} UTC</strong>
            </div>
            <div class="meta-item">
                <span>Branch:</span>
                <strong class="font-mono">{html.escape(branch)}</strong>
            </div>
            <div class="meta-item">
                <span>Commit:</span>
                <a href="{commit_url}" target="_blank" class="link font-mono"><strong>{html.escape(short_sha)}</strong></a>
            </div>
            <div class="meta-item">
                <span>Total Duration:</span>
                <strong class="font-mono">{current_run['duration']:.3f}s</strong>
            </div>
        </div>

        <div class="metrics-grid">
            <div class="metric-card">
                <div class="metric-label">Total Tests</div>
                <div class="metric-value text-accent">{current_run['total']}</div>
            </div>
            <div class="metric-card">
                <div class="metric-label">Passed</div>
                <div class="metric-value text-success">{current_run['passed']}</div>
            </div>
            <div class="metric-card">
                <div class="metric-label">Failed</div>
                <div class="metric-value text-danger">{current_run['failed']}</div>
            </div>
            <div class="metric-card">
                <div class="metric-label">Skipped</div>
                <div class="metric-value text-warning">{current_run['skipped']}</div>
            </div>
        </div>

        {failed_section_html}

        <section class="section">
            <h2 class="section-title">📋 Test Results Detail</h2>
            <table>
                <thead>
                    <tr>
                        <th style="width: 100px;">Status</th>
                        <th>Test Case Name</th>
                        <th style="width: 120px;" class="text-right">Duration</th>
                        <th style="width: 80px;" class="text-center">Action</th>
                    </tr>
                </thead>
                <tbody>
                    {"".join(all_tests_rows) if all_tests_rows else '<tr><td colspan="4" class="text-center">No test cases recorded</td></tr>'}
                </tbody>
            </table>
        </section>

        <section class="section">
            <h2 class="section-title">📊 Execution History (Recent Runs)</h2>
            <table>
                <thead>
                    <tr>
                        <th style="width: 110px;">Result</th>
                        <th style="width: 110px;">Commit</th>
                        <th>Branch</th>
                        <th>Timestamp</th>
                        <th style="width: 90px;" class="text-right">Passed</th>
                        <th style="width: 90px;" class="text-right">Failed</th>
                        <th style="width: 110px;" class="text-right">Duration</th>
                    </tr>
                </thead>
                <tbody>
                    {"".join(history_rows) if history_rows else '<tr><td colspan="7" class="text-center">No previous history available</td></tr>'}
                </tbody>
            </table>
        </section>

        <footer>
            Automated E2E Test Suite &bull; {html.escape(project_name)} &bull; Masked Audit Safe
        </footer>
    </div>
</body>
</html>
"""
    return html_content


def main():
    raw_json_file = sys.argv[1] if len(sys.argv) > 1 else "test_reports/e2e_raw.json"
    output_html_file = sys.argv[2] if len(sys.argv) > 2 else "test_reports/index.html"
    history_file = sys.argv[3] if len(sys.argv) > 3 else "test_reports/history.json"

    repo = os.environ.get("GITHUB_REPOSITORY", "")
    branch = os.environ.get("GITHUB_REF_NAME", os.environ.get("GITHUB_HEAD_REF", "local"))
    commit_sha = os.environ.get("GITHUB_SHA", "HEAD")

    # If running locally, fetch git info via git commands if possible
    if commit_sha == "HEAD" and os.path.exists(".git"):
        try:
            import subprocess
            commit_sha = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
            branch = subprocess.check_output(["git", "branch", "--show-current"], text=True).strip() or branch
        except Exception:
            pass

    out_dir = os.path.dirname(output_html_file)
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)

    print(f"==> Parsing E2E results from {raw_json_file}...")
    run_data = parse_go_test_json(raw_json_file)
    run_data["timestamp"] = datetime.datetime.now(datetime.timezone.utc).isoformat()
    run_data["branch"] = branch
    run_data["commit_sha"] = commit_sha

    print(f"==> Loading previous test history...")
    prev_history = fetch_previous_history(history_file, repo)

    # Prepend current run to history (without bulky detailed test output in history)
    history_entry = {
        "status": run_data["status"],
        "timestamp": run_data["timestamp"],
        "branch": run_data["branch"],
        "commit_sha": run_data["commit_sha"],
        "passed": run_data["passed"],
        "failed": run_data["failed"],
        "skipped": run_data["skipped"],
        "total": run_data["total"],
        "duration": run_data["duration"],
    }
    updated_history = [history_entry] + [h for h in prev_history if h.get("timestamp") != run_data["timestamp"]][:MAX_HISTORY_ITEMS - 1]

    # Save updated history.json
    with open(history_file, "w", encoding="utf-8") as f:
        json.dump(updated_history, f, indent=2, ensure_ascii=False)
    print(f"==> Saved history to {history_file} ({len(updated_history)} runs)")

    # Derive project name for report header
    project_name = os.environ.get("E2E_REPORT_TITLE", "")
    if not project_name and repo:
        project_name = repo.split("/")[-1]
    if not project_name:
        project_name = "Go Project"

    # Generate HTML report
    html_content = generate_html_report(run_data, updated_history, repo, project_name)
    with open(output_html_file, "w", encoding="utf-8") as f:
        f.write(html_content)
    print(f"==> Generated HTML report at {output_html_file}")

    # Return exit code based on test outcome
    if run_data["status"] != "SUCCESS":
        print(f"WARNING: E2E tests contained failures ({run_data['failed']} failed).", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
