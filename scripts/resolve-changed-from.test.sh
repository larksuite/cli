#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$repo_root/scripts/resolve-changed-from.sh"

tmp="${TMPDIR:-/tmp}/resolve-changed-from-test-$$"

cleanup_tmp() {
  local attempt
  for attempt in 1 2 3; do
    rm -rf "$tmp" && return 0
    sleep 1
  done
  rm -rf "$tmp"
}

trap cleanup_tmp EXIT
mkdir -p "$tmp"

git_init() {
  local dir="$1"
  git init -q -b main "$dir"
  git -C "$dir" config user.name test
  git -C "$dir" config user.email test@example.com
}

commit_file() {
  local dir="$1"
  local name="$2"
  local content="$3"
  printf '%s\n' "$content" >"$dir/$name"
  git -C "$dir" add "$name"
  git -C "$dir" commit -q -m "$content"
}

test_uses_candidate_when_it_is_head_ancestor() {
  local dir="$tmp/ancestor"
  git_init "$dir"
  commit_file "$dir" file.txt base
  local base
  base="$(git -C "$dir" rev-parse HEAD)"
  commit_file "$dir" file.txt head

  local got
  got="$(cd "$dir" && QUALITY_GATE_CHANGED_FROM="$base" bash "$script")"
  if [[ "$got" != "$base" ]]; then
    echo "ancestor candidate = $got, want $base" >&2
    return 1
  fi
}

test_uses_merge_base_when_candidate_is_related_but_not_head_ancestor() {
  local dir="$tmp/non-ancestor"
  git_init "$dir"
  commit_file "$dir" file.txt base
  local base
  base="$(git -C "$dir" rev-parse HEAD)"
  git -C "$dir" checkout -q -b old
  commit_file "$dir" old.txt old
  local old
  old="$(git -C "$dir" rev-parse HEAD)"
  git -C "$dir" checkout -q main
  commit_file "$dir" file.txt head-1
  commit_file "$dir" file.txt head-2

  local got
  got="$(cd "$dir" && QUALITY_GATE_CHANGED_FROM="$old" bash "$script")"
  if [[ "$got" != "$base" ]]; then
    echo "non-ancestor candidate = $got, want merge-base $base" >&2
    return 1
  fi
}

test_uses_origin_main_merge_base_when_candidate_is_missing() {
  local dir="$tmp/origin-main"
  git_init "$dir"
  commit_file "$dir" file.txt base
  local base
  base="$(git -C "$dir" rev-parse HEAD)"
  git -C "$dir" branch feature
  commit_file "$dir" file.txt main
  git -C "$dir" update-ref refs/remotes/origin/main HEAD
  git -C "$dir" checkout -q feature
  commit_file "$dir" feature.txt feature-1
  commit_file "$dir" feature.txt feature-2

  local got
  got="$(cd "$dir" && bash "$script")"
  if [[ "$got" != "$base" ]]; then
    echo "missing candidate = $got, want origin/main merge-base $base" >&2
    return 1
  fi
}

test_falls_back_from_zero_sha() {
  local dir="$tmp/zero"
  git_init "$dir"
  commit_file "$dir" file.txt base
  commit_file "$dir" file.txt head

  local got
  got="$(cd "$dir" && QUALITY_GATE_CHANGED_FROM="0000000000000000000000000000000000000000" bash "$script")"
  if [[ "$got" != "HEAD~1" ]]; then
    echo "zero candidate = $got, want HEAD~1" >&2
    return 1
  fi
}

test_uses_candidate_when_it_is_head_ancestor
test_uses_merge_base_when_candidate_is_related_but_not_head_ancestor
test_uses_origin_main_merge_base_when_candidate_is_missing
test_falls_back_from_zero_sha
