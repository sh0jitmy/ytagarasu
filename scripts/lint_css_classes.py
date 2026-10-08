#!/usr/bin/env python3
# Copyright 2026 [Copyright Holder]
# Licensed under the Apache License, Version 2.0 (the "License");
#
# Author: [YOUR_NAME]
"""
CSS Class Completeness Linter for ytagarasu Dashboard.
Scans all HTML templates and verifies that EVERY class used in templates
is defined in internal/web/static/css/dashboard.css.
Exits with code 1 if any class is missing.
"""

import os
import re
import sys

TEMPLATES_DIR = "internal/web/templates"
CSS_FILE = "internal/web/static/css/dashboard.css"

# Standard / dynamic / foreign classes to ignore if any (keep minimal!)
ALLOWED_MISSING = {
    "active",  # state pseudo-class often used dynamically
}

def main():
    if not os.path.exists(CSS_FILE):
        print(f"ERROR: CSS file not found: {CSS_FILE}")
        sys.exit(1)

    with open(CSS_FILE, "r", encoding="utf-8") as f:
        css_content = f.read()

    # Extract all classes defined in CSS (.class-name)
    # Ignore pseudo-elements/classes (:hover, ::backdrop etc.)
    defined_classes = set(re.findall(r"\.([a-zA-Z0-9_-]+)", css_content))

    missing = {}
    total_usages = 0

    for root, _, files in os.walk(TEMPLATES_DIR):
        for file in files:
            if not file.endswith(".html"):
                continue
            path = os.path.join(root, file)
            with open(path, "r", encoding="utf-8") as f:
                content = f.read()

            # Find all class="..." or class='...'
            matches = re.findall(r'class=["\']([^"\']+)["\']', content)
            for m in matches:
                # Split whitespace
                classes = m.split()
                for cls in classes:
                    # Filter out Go template actions like {{if ...}}
                    if "{{" in cls or "}}" in cls or "$" in cls:
                        continue
                    total_usages += 1
                    if cls not in defined_classes and cls not in ALLOWED_MISSING:
                        missing.setdefault(cls, set()).add(path)

    if missing:
        print("❌ [CSS LINT FAILED] The following CSS classes are used in HTML templates but NOT defined in dashboard.css:")
        print("=" * 70)
        for cls, files in sorted(missing.items()):
            file_list = ", ".join(sorted(files))
            print(f"  • .{cls:<24} in {file_list}")
        print("=" * 70)
        print(f"Total missing classes: {len(missing)} (out of {total_usages} usages)")
        print("Please define all missing classes in internal/web/static/css/dashboard.css!")
        sys.exit(1)

    print(f"✅ [CSS LINT PASSED] All classes used across {TEMPLATES_DIR} are fully defined in {CSS_FILE} ({total_usages} usages verified).")
    sys.exit(0)

if __name__ == "__main__":
    main()
