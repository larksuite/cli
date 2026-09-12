#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

layering_ratchet_extract_keys() {
  local source_file="$1"
  local output_file="$2"
  awk -F '\t' '
    function trim(value) {
      sub(/^[[:space:]]+/, "", value)
      sub(/[[:space:]]+$/, "", value)
      return value
    }
    {
      content = trim($0)
      if (content == "" || substr(content, 1, 1) == "#") {
        next
      }
      if (NF != 5) {
        printf "Malformed layering ratchet row at %s:%d: expected five tab-separated fields.\n", FILENAME, FNR > "/dev/stderr"
        exit 2
      }
      from = $1
      denied = $2
      owner = $3
      reason = $4
      added_at = $5
      sub(/\r+$/, "", added_at)
      if (from != trim(from) || denied != trim(denied) || owner != trim(owner) || reason != trim(reason) || added_at != trim(added_at)) {
        printf "Malformed layering ratchet row at %s:%d: fields must not have surrounding whitespace.\n", FILENAME, FNR > "/dev/stderr"
        exit 2
      }
      if (from == "" || denied == "" || owner == "" || reason == "" || added_at !~ /^[0-9]{4}-[0-9]{2}-[0-9]{2}$/) {
        printf "Malformed layering ratchet row at %s:%d: fields must be non-empty and added_at must use YYYY-MM-DD.\n", FILENAME, FNR > "/dev/stderr"
        exit 2
      }
      print from "\t" denied
    }
  ' "$source_file" | LC_ALL=C sort >"$output_file" || return

  local duplicates
  duplicates="$(uniq -d "$output_file")" || return
  if [[ -n "$duplicates" ]]; then
    echo "Layering ratchet contains duplicate (from, denied) keys: $source_file" >&2
    printf '%s\n' "$duplicates" >&2
    return 1
  fi
}

layering_ratchet_hash_keys() {
  local source_file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$source_file" | awk '{ print $1 }' || return
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$source_file" | awk '{ print $1 }' || return
    return
  fi
  echo "Layering ratchet requires sha256sum or shasum." >&2
  return 1
}

layering_ratchet_validate_bootstrap_snapshot() {
  local current_keys="$1"
  local expected_count="$2"
  local expected_hash="$3"
  local current_count
  local current_hash
  current_count="$(wc -l <"$current_keys" | tr -d '[:space:]')" || return
  current_hash="$(layering_ratchet_hash_keys "$current_keys")" || return
  if [[ "$current_count" != "$expected_count" || "$current_hash" != "$expected_hash" ]]; then
    echo "::error::Layering ratchet bootstrap differs from the approved $expected_count-edge snapshot." >&2
    return 1
  fi
}

layering_ratchet_main() (
  set -euo pipefail

  local ratchet_file="internal/qualitygate/deptest/layering-edges.txt"
  if root="$(git rev-parse --show-toplevel 2>/dev/null)"; then
    cd "$root" || return
  else
    echo "Layering ratchet must run inside a Git worktree." >&2
    return 1
  fi

  local base_revision="${1:-${QUALITY_GATE_CHANGED_FROM:-}}"
  local approved_initial_count="${2:-39}"
  local approved_initial_hash="${3:-5636d50d10b9de1e08dc9f06cd66671b3f438650fa5b7f28b95aa7d5a69a1c21}"
  if [[ -z "$base_revision" ]]; then
    echo "Layering ratchet requires a base revision." >&2
    return 1
  fi
  if ! git cat-file -e "$base_revision^{commit}" 2>/dev/null; then
    echo "Layering ratchet base revision does not exist: $base_revision" >&2
    return 1
  fi
  if [[ ! -f "$ratchet_file" ]]; then
    echo "Layering ratchet file is missing: $ratchet_file" >&2
    return 1
  fi

  local tmp_dir
  tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/layering-ratchet.XXXXXX")" || return
  local base_file="$tmp_dir/base.txt"
  local base_keys="$tmp_dir/base.keys"
  local current_keys="$tmp_dir/current.keys"
  local additions="$tmp_dir/additions.keys"

  layering_ratchet_cleanup_current_run() {
    rm -f "$base_file" "$base_keys" "$current_keys" "$additions"
    rmdir "$tmp_dir"
  }
  trap layering_ratchet_cleanup_current_run EXIT

  layering_ratchet_extract_keys "$ratchet_file" "$current_keys" || return

  if ! git cat-file -e "$base_revision:$ratchet_file" 2>/dev/null; then
    layering_ratchet_validate_bootstrap_snapshot "$current_keys" "$approved_initial_count" "$approved_initial_hash" || return
    return
  fi

  git show "$base_revision:$ratchet_file" >"$base_file" || return
  layering_ratchet_extract_keys "$base_file" "$base_keys" || return
  LC_ALL=C comm -13 "$base_keys" "$current_keys" >"$additions" || return

  if [[ -s "$additions" ]]; then
    echo "::error::Layering ratchet contains new (from, denied) keys. Fix the dependency instead of adding rows." >&2
    while IFS=$'\t' read -r from denied; do
      printf 'from=%s denied=%s\n' "$from" "$denied" >&2
    done <"$additions"
    return 1
  fi
)

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  layering_ratchet_main "${1:-${QUALITY_GATE_CHANGED_FROM:-}}"
fi
