#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

# layering_ratchet_extract_keys validates every row and prints the comparison
# key. fields=key prints (from, denied) — what the ratchet forbids adding.
# fields=dated appends added_at, so a later pass can tell an approved row's date
# from a rewritten one. Both come from this one validator: a row that is
# malformed for one mode is malformed for the other.
layering_ratchet_extract_keys() {
  local source_file="$1"
  local output_file="$2"
  local fields="${3:-key}"
  awk -F '\t' -v fields="$fields" '
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
      if (fields == "dated") {
        print from "\t" denied "\t" added_at
      } else {
        print from "\t" denied
      }
    }
  ' "$source_file" | LC_ALL=C sort >"$output_file" || return

  # Only the key form can speak about duplicates: two rows sharing a key differ
  # in the dated form, so the dated pass would report nothing.
  if [[ "$fields" != "key" ]]; then
    return
  fi
  local duplicates
  duplicates="$(uniq -d "$output_file")" || return
  if [[ -n "$duplicates" ]]; then
    echo "Layering ratchet contains duplicate (from, denied) keys: $source_file" >&2
    printf '%s\n' "$duplicates" >&2
    return 1
  fi
}

# layering_ratchet_check_added_at rejects a moved added_at on a row that already
# existed on the base revision. The date is the ratchet's clock: it records when
# the debt was accepted, so rewriting it makes an old exception look fresh (or a
# fresh one look grandfathered) without changing a single import.
#
# owner and reason stay editable on purpose — they are maintenance fields whose
# change is legible in the diff, and locking them would leave no way to hand an
# exception over short of deleting a row the dependency still needs.
layering_ratchet_check_added_at() {
  local base_dated="$1"
  local current_dated="$2"
  local drift
  drift="$(LC_ALL=C awk -F '\t' '
    NR == FNR {
      base_added_at[$1 "\t" $2] = $3
      next
    }
    {
      key = $1 "\t" $2
      if (key in base_added_at && base_added_at[key] != $3) {
        printf "from=%s denied=%s added_at=%s -> %s\n", $1, $2, base_added_at[key], $3
      }
    }
  ' "$base_dated" "$current_dated")" || return
  if [[ -z "$drift" ]]; then
    return
  fi
  echo "::error::Layering ratchet moved added_at on an already-approved exception. The date records when the debt was accepted; keep it, or remove the row by fixing the dependency." >&2
  printf '%s\n' "$drift" >&2
  return 1
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

# layering_ratchet_validate_bootstrap_snapshot runs when the base revision has no
# registry at all. That happens in two situations the check cannot tell apart
# from the base alone — the commit that introduces the gate, and any branch that
# forked before it — so the failure names both and the action each one takes.
# Without that, a branch on a stale base that legitimately registers one row is
# told its bootstrap snapshot is wrong, and the obvious response (edit the
# approved baseline) is the opposite of the intended one.
layering_ratchet_validate_bootstrap_snapshot() {
  local current_keys="$1"
  local expected_count="$2"
  local expected_hash="$3"
  local base_revision="$4"
  local ratchet_file="$5"
  local current_count
  local current_hash
  current_count="$(wc -l <"$current_keys" | tr -d '[:space:]')" || return
  current_hash="$(layering_ratchet_hash_keys "$current_keys")" || return
  if [[ "$current_count" == "$expected_count" && "$current_hash" == "$expected_hash" ]]; then
    return
  fi
  echo "::error::Layering ratchet bootstrap differs from the approved $expected_count-edge snapshot: found $current_count key(s)." >&2
  echo "Base revision $base_revision carries no $ratchet_file, so this run had nothing to compare against and fell back to the approved bootstrap snapshot." >&2
  echo "If your branch forked before the gate landed: rebase onto the target branch and re-run — a row you legitimately register is then reported as a new (from, denied) key, not as a bootstrap mismatch." >&2
  echo "If your branch is the one introducing the gate: the registry has to land empty, so fix the dependency instead of registering it." >&2
  if [[ -s "$current_keys" ]]; then
    echo "Keys found in $ratchet_file:" >&2
    while IFS=$'\t' read -r from denied; do
      printf 'from=%s denied=%s\n' "$from" "$denied" >&2
    done <"$current_keys"
  fi
  return 1
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
  # The registry first reaches the target branch with no approved exceptions.
  # Only consulted while the target branch has no registry file, so bootstrap
  # requires the extracted key set to stay empty. Hardcoded on purpose — a
  # configurable baseline could be raised in CI without review.
  local approved_initial_count="${2:-0}"
  local approved_initial_hash="${3:-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855}"
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
  local base_dated="$tmp_dir/base.dated"
  local current_dated="$tmp_dir/current.dated"
  local additions="$tmp_dir/additions.keys"

  layering_ratchet_cleanup_current_run() {
    rm -f "$base_file" "$base_keys" "$current_keys" "$base_dated" "$current_dated" "$additions"
    rmdir "$tmp_dir"
  }
  trap layering_ratchet_cleanup_current_run EXIT

  layering_ratchet_extract_keys "$ratchet_file" "$current_keys" || return

  if ! git cat-file -e "$base_revision:$ratchet_file" 2>/dev/null; then
    layering_ratchet_validate_bootstrap_snapshot \
      "$current_keys" "$approved_initial_count" "$approved_initial_hash" \
      "$base_revision" "$ratchet_file" || return
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

  layering_ratchet_extract_keys "$ratchet_file" "$current_dated" dated || return
  layering_ratchet_extract_keys "$base_file" "$base_dated" dated || return
  layering_ratchet_check_added_at "$base_dated" "$current_dated" || return
)

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  layering_ratchet_main "${1:-${QUALITY_GATE_CHANGED_FROM:-}}"
fi
