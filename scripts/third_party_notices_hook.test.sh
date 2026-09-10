#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

mkdir -p "$TMP_DIR/.githooks"
cp "$ROOT_DIR/.githooks/pre-commit" "$TMP_DIR/.githooks/pre-commit"
chmod +x "$TMP_DIR/.githooks/pre-commit"

git -C "$TMP_DIR" init --quiet
git -C "$TMP_DIR" config core.hooksPath .githooks
git -C "$TMP_DIR" config user.name "Third-Party Notices Test"
git -C "$TMP_DIR" config user.email "notices-test@example.invalid"

printf 'module example.com/test\n\nrequire example.com/dependency v1.2.3\n' > "$TMP_DIR/go.mod"
git -C "$TMP_DIR" add go.mod
if git -C "$TMP_DIR" commit --quiet -m "test: missing notices"; then
  echo "pre-commit should block staged dependency metadata without notices" >&2
  exit 1
fi

printf '# Third-Party Notices\n' > "$TMP_DIR/THIRD_PARTY_NOTICES.md"
git -C "$TMP_DIR" add THIRD_PARTY_NOTICES.md
git -C "$TMP_DIR" commit --quiet -m "test: include notices"

printf 'go 1.25.0\n' >> "$TMP_DIR/go.mod"
git -C "$TMP_DIR" add go.mod
git -C "$TMP_DIR" commit --quiet -m "test: allow non-dependency metadata"

printf '{"dependencies":{"example":"1.0.0"}}\n' > "$TMP_DIR/package.json"
git -C "$TMP_DIR" add package.json
if git -C "$TMP_DIR" commit --quiet -m "test: missing npm notices"; then
  echo "pre-commit should block staged npm dependency declarations without notices" >&2
  exit 1
fi

printf '# Third-Party Notices\n\nUpdated for npm dependency.\n' > "$TMP_DIR/THIRD_PARTY_NOTICES.md"
git -C "$TMP_DIR" add THIRD_PARTY_NOTICES.md
git -C "$TMP_DIR" commit --quiet -m "test: include npm notices"

printf '{"description":"metadata only","dependencies":{"example":"1.0.0"}}\n' > "$TMP_DIR/package.json"
git -C "$TMP_DIR" add package.json
git -C "$TMP_DIR" commit --quiet -m "test: allow npm metadata"

if rg -q '^[[:space:]]*(make|python3?|git[[:space:]]+add)\b' "$ROOT_DIR/.githooks/pre-commit"; then
  echo "pre-commit must only inspect the Git index; it must not execute or stage worktree code" >&2
  exit 1
fi
