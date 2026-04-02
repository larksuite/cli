#!/usr/bin/env python3
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
"""Fetch meta_data.json from remote API for build-time embedding.

Usage:
    python3 scripts/fetch_meta.py              # fetch from feishu (default)
    python3 scripts/fetch_meta.py --brand lark  # fetch from larksuite
"""

import argparse
import json
import os
import subprocess
import sys
import urllib.request
import urllib.error

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.join(SCRIPT_DIR, "..")
OUT_PATH = os.path.join(ROOT, "internal", "registry", "meta_data.json")

API_HOSTS = {
    "feishu": "https://open.feishu.cn/api/tools/open/api_definition",
    "lark": "https://open.larksuite.com/api/tools/open/api_definition",
}

TIMEOUT = 10  # seconds


def get_version():
    """Get version from git tags, matching Makefile logic."""
    try:
        return subprocess.check_output(
            ["git", "describe", "--tags", "--always", "--dirty"],
            stderr=subprocess.DEVNULL,
            text=True,
            cwd=ROOT,
        ).strip()
    except Exception:
        return "dev"


def fetch_remote(brand):
    """Fetch MergedRegistry from remote API."""
    base = API_HOSTS.get(brand, API_HOSTS["feishu"])
    version = get_version()
    url = f"{base}?protocol=meta&client_version={urllib.request.quote(version)}"

    print(f"fetch-meta: GET {url}", file=sys.stderr)
    req = urllib.request.Request(url)
    resp = urllib.request.urlopen(req, timeout=TIMEOUT)
    body = resp.read()

    envelope = json.loads(body)
    if envelope.get("msg") != "succeeded":
        raise RuntimeError(f"unexpected response msg: {envelope.get('msg')!r}")

    data = envelope.get("data", {})
    if not data.get("services"):
        raise RuntimeError("remote returned empty services")

    return data


def main():
    parser = argparse.ArgumentParser(description="Fetch meta_data.json for build-time embedding")
    parser.add_argument("--brand", default="feishu", choices=["feishu", "lark"],
                        help="API brand (default: feishu)")
    parser.add_argument("--force", action="store_true",
                        help="force refresh from remote even if local file exists")
    args = parser.parse_args()

    if os.path.exists(OUT_PATH) and not args.force:
        if os.path.isfile(OUT_PATH):
            try:
                with open(OUT_PATH, "r", encoding="utf-8") as fp:
                    local = json.load(fp)
                if local.get("services"):
                    print(f"fetch-meta: {OUT_PATH} already exists, skipping (use --force to re-fetch)", file=sys.stderr)
                    return
                print(f"fetch-meta: {OUT_PATH} has no services, re-fetching", file=sys.stderr)
            except (OSError, json.JSONDecodeError):
                print(f"fetch-meta: {OUT_PATH} is invalid JSON, re-fetching", file=sys.stderr)
        else:
            print(f"fetch-meta: {OUT_PATH} is not a file, re-fetching", file=sys.stderr)

    data = fetch_remote(args.brand)
    count = len(data.get("services", []))
    print(f"fetch-meta: OK, {count} services from remote API", file=sys.stderr)

    with open(OUT_PATH, "w") as fp:
        json.dump(data, fp, ensure_ascii=False, indent=2)
        fp.write("\n")


if __name__ == "__main__":
    main()
