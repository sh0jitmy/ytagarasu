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
check_pages_links.py: Zero-404 Static & Live Link Auditor for GitHub Pages

Performs comprehensive verification on test_reports/ directory:
1. Static Scan:
   - Finds all HTML files.
   - Extracts all <a href>, <img src>, <link href>, <script src>.
   - Prohibits root-relative absolute paths (/foo) which break on GitHub Pages subpaths.
   - Validates that target local files exist on filesystem (0 broken links).
   - Validates #anchor targets exist in destination HTML (id="xxx" or name="xxx").
   - Flags external CDN dependencies (Zero-CDN policy enforcement).
2. Live HTTP Verification:
   - Starts a local ThreadingHTTPServer serving test_reports/.
   - Dispatches real HTTP GET requests to every discovered internal page and resource.
   - Verifies HTTP 200 OK for every single link.
   - Exits with 0 only if 100% of links resolve with 0 broken links and 0 404 errors.
"""

import glob
import http.server
import os
import re
import socket
import sys
import threading
import time
import urllib.parse
import urllib.request
from html.parser import HTMLParser

REPORT_DIR = "test_reports"


class HTMLLinkAndAnchorParser(HTMLParser):
    def __init__(self):
        super().__init__()
        self.links = []   # list of (tag, attr, val, line_no)
        self.anchors = set()  # set of IDs and NAMEs in this file

    def handle_starttag(self, tag, attrs):
        line_no = self.getpos()[0]
        attr_dict = dict(attrs)

        # Record element IDs and anchor names for fragment resolution
        if "id" in attr_dict:
            self.anchors.add(attr_dict["id"])
        if tag == "a" and "name" in attr_dict:
            self.anchors.add(attr_dict["name"])

        # Extract links
        for attr in ("href", "src"):
            if attr in attr_dict:
                val = attr_dict[attr].strip()
                if val:
                    self.links.append((tag, attr, val, line_no))


def get_free_port():
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("", 0))
        return s.getsockname()[1]


class QuietHTTPHandler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, format, *args):
        pass  # suppress normal request logs


def main():
    print("======================================================================")
    print(" 🦅 ytagarasu GitHub Pages Link & 404 Integrity Auditor (Zero-404)     ")
    print("======================================================================")

    if not os.path.isdir(REPORT_DIR):
        print(f"❌ Error: Target report directory '{REPORT_DIR}' does not exist!")
        sys.exit(1)

    html_files = glob.glob(os.path.join(REPORT_DIR, "**", "*.html"), recursive=True)
    if not html_files:
        print(f"❌ Error: No HTML files found in '{REPORT_DIR}'!")
        sys.exit(1)

    print(f"==> Found {len(html_files)} HTML pages in {REPORT_DIR}/:")
    for hf in sorted(html_files):
        print(f"    - {hf} ({os.path.getsize(hf):,} bytes)")

    # 1. Parse all HTML files and collect links and anchors
    file_parsers = {}
    for hf in html_files:
        norm_path = os.path.normpath(hf)
        parser = HTMLLinkAndAnchorParser()
        with open(hf, "r", encoding="utf-8", errors="replace") as f:
            content = f.read()
            parser.feed(content)
        file_parsers[norm_path] = parser

    errors = []
    warnings = []
    total_links_checked = 0
    internal_urls_to_test = set()

    print("\n==> [Phase 1/2] Static Link Analysis & Filesystem Resolution...")

    for file_path, parser in file_parsers.items():
        base_dir = os.path.dirname(file_path)

        for tag, attr, raw_val, line_no in parser.links:
            total_links_checked += 1

            # Skip data URIs, javascript:, mailto:, tel:
            if raw_val.startswith(("data:", "javascript:", "mailto:", "tel:")):
                continue

            parsed = urllib.parse.urlparse(raw_val)

            # Check external URLs
            if parsed.scheme in ("http", "https"):
                if any(cdn in raw_val.lower() for cdn in ["cdnjs.cloudflare.com", "cdn.jsdelivr.net", "unpkg.com", "fonts.googleapis.com"]):
                    warnings.append(f"{file_path}:{line_no} Zero-CDN Warning: External CDN link detected: <{tag} {attr}=\"{raw_val}\">")
                continue

            # Root-relative path check (/foo.html) -> fatal for GitHub Pages
            if raw_val.startswith("/"):
                errors.append(f"{file_path}:{line_no} 404 Risk: Root-relative absolute link detected: '{raw_val}'. GitHub Pages hosts at a subdirectory (/repo-name/), which will cause a 404 error! Use relative paths (e.g. 'index.html' or './foo').")
                continue

            # Local path resolution
            target_path_part = parsed.path
            fragment = parsed.fragment

            if target_path_part == "":
                # In-page anchor link (e.g., "#summary")
                target_norm_file = file_path
            else:
                target_raw_full = os.path.join(base_dir, target_path_part)
                target_norm_file = os.path.normpath(target_raw_full)

            # Check if file exists
            if not os.path.exists(target_norm_file):
                errors.append(f"{file_path}:{line_no} ❌ 404 NOT FOUND: Link target '{raw_val}' does not exist on disk (resolved to '{target_norm_file}')")
                continue

            # If anchor is present, verify it exists in target HTML
            if fragment:
                target_parser = file_parsers.get(target_norm_file)
                if target_parser is not None:
                    if fragment not in target_parser.anchors:
                        errors.append(f"{file_path}:{line_no} ❌ 404 ANCHOR NOT FOUND: Fragment '#{fragment}' not found in target '{target_norm_file}'")

            # Collect internal URL for live testing
            rel_to_report = os.path.relpath(target_norm_file, REPORT_DIR)
            internal_urls_to_test.add(rel_to_report)

    print(f"    - Total links audited: {total_links_checked}")
    print(f"    - Unique internal targets: {len(internal_urls_to_test)}")

    if warnings:
        print(f"\n⚠️  Zero-CDN / Policy Warnings ({len(warnings)}):")
        for w in warnings:
            print(f"    [WARN] {w}")

    if errors:
        print(f"\n❌ Static Analysis Failed with {len(errors)} 404 / Link Errors:")
        for e in errors:
            print(f"    [FAIL] {e}")
        sys.exit(1)
    else:
        print("    ✓ All local files and anchors exist (0 static 404s).")

    # 2. Live HTTP Server Verification
    print("\n==> [Phase 2/2] Live HTTP Server 404 Verification...")
    port = get_free_port()

    # Change directory to test_reports for simple server
    server_address = ("127.0.0.1", port)

    class CustomDirHandler(QuietHTTPHandler):
        def __init__(self, *args, **kwargs):
            super().__init__(*args, directory=REPORT_DIR, **kwargs)

    httpd = http.server.ThreadingHTTPServer(server_address, CustomDirHandler)
    server_thread = threading.Thread(target=httpd.serve_forever, daemon=True)
    server_thread.start()
    time.sleep(0.3)

    live_errors = []
    live_checked = 0

    try:
        # Check all HTML files directly plus internal targets
        all_live_targets = set()
        for hf in html_files:
            rel = os.path.relpath(hf, REPORT_DIR)
            all_live_targets.add(rel)
        all_live_targets.update(internal_urls_to_test)

        for rel_target in sorted(all_live_targets):
            url = f"http://127.0.0.1:{port}/{urllib.parse.quote(rel_target)}"
            live_checked += 1
            try:
                req = urllib.request.Request(url, method="GET")
                with urllib.request.urlopen(req, timeout=10) as resp:
                    status = resp.status
                    # Drain response body to prevent socket broken pipe on server
                    _ = resp.read()
                    if status != 200:
                        live_errors.append(f"HTTP {status} for {url} (expected 200 OK)")
                    else:
                        print(f"    [HTTP {status} OK] {rel_target}")
            except urllib.error.HTTPError as e:
                live_errors.append(f"HTTP {e.code} for {url} ({e.reason})")
            except Exception as e:
                live_errors.append(f"Connection error for {url}: {e}")

    finally:
        httpd.shutdown()
        httpd.server_close()

    print(f"\n==> Live HTTP Audit Summary:")
    print(f"    - URLs Tested: {live_checked}")
    print(f"    - Broken Links / 404s: {len(live_errors)}")

    if live_errors:
        print(f"\n❌ Live HTTP Verification Failed with {len(live_errors)} Errors:")
        for err in live_errors:
            print(f"    [FAIL] {err}")
        sys.exit(1)

    print("\n======================================================================")
    print(" ✅ ALL PAGES AND LINKS VERIFIED! 100% CLEAN - 0 404 ERRORS DETECTED!   ")
    print("    - Ready for GitHub Pages Deployment and Pull Request Submission.   ")
    print("======================================================================")
    sys.exit(0)


if __name__ == "__main__":
    main()
