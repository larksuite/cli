#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

if root="$(git rev-parse --show-toplevel 2>/dev/null)"; then
  cd "$root"
fi

is_zero_sha() {
  [[ "$1" =~ ^0{40}$ ]]
}

commit_exists() {
  git cat-file -e "$1^{commit}" 2>/dev/null
}

is_head_ancestor() {
  git merge-base --is-ancestor "$1" HEAD 2>/dev/null
}

merge_base_with_head() {
  git merge-base "$1" HEAD 2>/dev/null
}

candidate="${QUALITY_GATE_CHANGED_FROM:-}"
if [[ -n "$candidate" ]] && ! is_zero_sha "$candidate"; then
  if commit_exists "$candidate"; then
    if is_head_ancestor "$candidate"; then
      printf '%s\n' "$candidate"
      exit 0
    fi
    if base="$(merge_base_with_head "$candidate")"; then
      printf '%s\n' "$base"
      exit 0
    fi
  fi
fi

if commit_exists origin/main; then
  if base="$(merge_base_with_head origin/main)"; then
    printf '%s\n' "$base"
    exit 0
  fi
fi

if git rev-parse --verify --quiet HEAD~1 >/dev/null; then
  printf '%s\n' "HEAD~1"
  exit 0
fi

printf '%s\n' "HEAD"
