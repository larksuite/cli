#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

script="scripts/e2e_scope.sh"

run_scope() {
  local file="$1"
  E2E_SCOPE_CHANGED_FILES="$file" "$script"
}

write_changes() {
  local file="$1"
  shift
  printf '%s\n' "$@" >"$file"
}

field() {
  local key="$1"
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }'
}

assert_field() {
  local output="$1"
  local key="$2"
  local want="$3"
  local got
  got="$(printf '%s\n' "$output" | field "$key")"
  if [ "$got" != "$want" ]; then
    printf 'expected %s=%s, got %s\nfull output:\n%s\n' "$key" "$want" "$got" "$output" >&2
    exit 1
  fi
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  if ! grep -Fq "$needle" <<<"$haystack"; then
    printf 'expected output to contain %s\nfull output:\n%s\n' "$needle" "$haystack" >&2
    exit 1
  fi
}

assert_not_contains() {
  local haystack="$1"
  local needle="$2"
  if grep -Fq "$needle" <<<"$haystack"; then
    printf 'expected output not to contain %s\nfull output:\n%s\n' "$needle" "$haystack" >&2
    exit 1
  fi
}

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

changes="$tmpdir/changes.txt"

write_changes "$changes" "shortcuts/im/messages/send.go"
output="$(run_scope "$changes")"
assert_field "$output" mode subset
assert_field "$output" domains im
assert_contains "$output" "github.com/larksuite/cli/tests/cli_e2e/im"
assert_not_contains "$output" "github.com/larksuite/cli/tests/cli_e2e/drive"

write_changes "$changes" "shortcuts/doc/update.go"
output="$(run_scope "$changes")"
assert_field "$output" mode subset
assert_field "$output" domains doc,docs
assert_contains "$output" "github.com/larksuite/cli/tests/cli_e2e/doc"
assert_contains "$output" "github.com/larksuite/cli/tests/cli_e2e/docs"

write_changes "$changes" "tests/cli_e2e/drive/helpers.go"
output="$(run_scope "$changes")"
assert_field "$output" mode subset
assert_field "$output" domains drive
assert_contains "$output" "github.com/larksuite/cli/tests/cli_e2e/drive"

write_changes "$changes" "tests/cli_e2e/core.go"
output="$(run_scope "$changes")"
assert_field "$output" mode full
assert_field "$output" domains all
assert_contains "$output" "shared CLI E2E harness changed"

write_changes "$changes" "cmd/root.go"
output="$(run_scope "$changes")"
assert_field "$output" mode full
assert_field "$output" domains all
assert_contains "$output" "shared/runtime path changed"

write_changes "$changes" "docs/usage.md" "README.md"
output="$(run_scope "$changes")"
assert_field "$output" mode skip
assert_field "$output" domains ""
assert_field "$output" live_packages ""

write_changes "$changes" "skills/lark-sheets/SKILL.md"
output="$(run_scope "$changes")"
assert_field "$output" mode subset
assert_field "$output" domains sheets
assert_contains "$output" "github.com/larksuite/cli/tests/cli_e2e/sheets"
