#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$repo_root/scripts/check-layering-ratchet.sh"
ratchet_file="internal/qualitygate/deptest/layering-edges.txt"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/check-layering-ratchet-test.XXXXXX")"
source "$script"

cleanup_tmp() {
  rm -rf "$tmp"
}
trap cleanup_tmp EXIT

row() {
  printf '%s\t%s\towner\treason\t%s\n' "$1" "$2" "${row_added_at:-2026-07-24}"
}

git_init() {
  local dir="$1"
  git init -q -b main "$dir"
  git -C "$dir" config user.name test
  git -C "$dir" config user.email test@example.com
  mkdir -p "$dir/$(dirname "$ratchet_file")"
}

write_rows() {
  local dir="$1"
  shift
  {
    printf '# from\tdenied\towner\treason\tadded_at\n'
    while (( $# > 0 )); do
      row "$1" "$2"
      shift 2
    done
  } >"$dir/$ratchet_file"
}

commit_ratchet() {
  local dir="$1"
  git -C "$dir" add "$ratchet_file"
  git -C "$dir" commit -q -m "ratchet"
}

expect_pass() {
  local dir="$1"
  local base="$2"
  if ! (cd "$dir" && bash "$script" "$base"); then
    echo "Expected layering ratchet check to pass in $dir." >&2
    return 1
  fi
}

expect_fail() {
  local dir="$1"
  local base="$2"
  local expected="$3"
  local output
  if output="$(cd "$dir" && bash "$script" "$base" 2>&1)"; then
    echo "Expected layering ratchet check to fail in $dir." >&2
    return 1
  fi
  if ! grep -Fq "$expected" <<<"$output"; then
    printf 'Layering ratchet failure did not include %q:\n%s\n' "$expected" "$output" >&2
    return 1
  fi
}

hash_file() {
  local source_file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$source_file" | awk '{ print $1 }'
  else
    shasum -a 256 "$source_file" | awk '{ print $1 }'
  fi
}

bootstrap_keys() {
  local source_file="$1"
  awk -F '\t' 'NF == 5 && $1 !~ /^[[:space:]]*#/ { print $1 "\t" $2 }' "$source_file" | LC_ALL=C sort
}

expect_bootstrap_pass() {
  local dir="$1"
  local base="$2"
  local count="$3"
  local hash="$4"
  if ! (cd "$dir" && layering_ratchet_main "$base" "$count" "$hash"); then
    echo "Expected layering ratchet bootstrap check to pass in $dir." >&2
    return 1
  fi
}

expect_bootstrap_fail() {
  local dir="$1"
  local base="$2"
  local count="$3"
  local hash="$4"
  local output
  if output="$(cd "$dir" && layering_ratchet_main "$base" "$count" "$hash" 2>&1)"; then
    echo "Expected layering ratchet bootstrap check to fail in $dir." >&2
    return 1
  fi
  if ! grep -Fq "bootstrap differs from the approved" <<<"$output"; then
    printf 'Unexpected layering ratchet bootstrap failure:\n%s\n' "$output" >&2
    return 1
  fi
}

expect_sourced_main_fail() {
  local dir="$1"
  local base="$2"
  local expected="$3"
  local output
  if output="$(cd "$dir" && layering_ratchet_main "$base" 2>&1)"; then
    echo "Expected sourced layering ratchet main to fail in $dir." >&2
    return 1
  fi
  if ! grep -Fq "$expected" <<<"$output"; then
    printf 'Sourced layering ratchet failure did not include %q:\n%s\n' "$expected" "$output" >&2
    return 1
  fi
}

test_unchanged_and_metadata_changes_pass() {
  local dir="$tmp/unchanged"
  git_init "$dir"
  write_rows "$dir" from/a denied/a from/b denied/b
  commit_ratchet "$dir"
  local base
  base="$(git -C "$dir" rev-parse HEAD)"

  expect_pass "$dir" "$base"
  sed -i.bak 's/\towner\treason\t/\tnew-owner\tnew-reason\t/' "$dir/$ratchet_file"
  rm -f "$dir/$ratchet_file.bak"
  expect_pass "$dir" "$base"
}

test_deletion_passes() {
  local dir="$tmp/deletion"
  git_init "$dir"
  write_rows "$dir" from/a denied/a from/b denied/b
  commit_ratchet "$dir"
  local base
  base="$(git -C "$dir" rev-parse HEAD)"

  write_rows "$dir" from/a denied/a
  expect_pass "$dir" "$base"
}

test_addition_fails() {
  local dir="$tmp/addition"
  git_init "$dir"
  write_rows "$dir" from/a denied/a
  commit_ratchet "$dir"
  local base
  base="$(git -C "$dir" rev-parse HEAD)"

  write_rows "$dir" from/a denied/a from/b denied/b
  expect_fail "$dir" "$base" "from=from/b denied=denied/b"
}

test_equal_count_replacement_fails() {
  local dir="$tmp/replacement"
  git_init "$dir"
  write_rows "$dir" from/a denied/a from/b denied/b
  commit_ratchet "$dir"
  local base
  base="$(git -C "$dir" rev-parse HEAD)"

  write_rows "$dir" from/a denied/a from/c denied/c
  expect_fail "$dir" "$base" "from=from/c denied=denied/c"
}

test_malformed_and_missing_current_file_fail() {
  local dir="$tmp/malformed"
  git_init "$dir"
  write_rows "$dir" from/a denied/a
  commit_ratchet "$dir"
  local base
  base="$(git -C "$dir" rev-parse HEAD)"

  printf 'from/a\tdenied/a\towner\treason\n' >"$dir/$ratchet_file"
  expect_fail "$dir" "$base" "expected five tab-separated fields"
  expect_sourced_main_fail "$dir" "$base" "expected five tab-separated fields"
  rm -f "$dir/$ratchet_file"
  expect_fail "$dir" "$base" "Layering ratchet file is missing"
}

test_surrounding_whitespace_fails() {
  local dir="$tmp/surrounding-whitespace"
  git_init "$dir"
  write_rows "$dir" from/a denied/a
  commit_ratchet "$dir"
  local base
  base="$(git -C "$dir" rev-parse HEAD)"

  printf '# from\tdenied\towner\treason\tadded_at\nfrom/a\t denied/a \towner\treason\t2026-07-24\n' >"$dir/$ratchet_file"
  expect_fail "$dir" "$base" "fields must not have surrounding whitespace"
  expect_sourced_main_fail "$dir" "$base" "fields must not have surrounding whitespace"
}

test_crlf_rows_pass() {
  local dir="$tmp/crlf"
  git_init "$dir"
  write_rows "$dir" from/a denied/a
  commit_ratchet "$dir"
  local base
  base="$(git -C "$dir" rev-parse HEAD)"

  printf '# from\tdenied\towner\treason\tadded_at\r\nfrom/a\tdenied/a\towner\treason\t2026-07-24\r\n' >"$dir/$ratchet_file"
  expect_pass "$dir" "$base"
}

test_duplicate_key_fails() {
  local dir="$tmp/duplicate"
  git_init "$dir"
  write_rows "$dir" from/a denied/a
  commit_ratchet "$dir"
  local base
  base="$(git -C "$dir" rev-parse HEAD)"

  write_rows "$dir" from/a denied/a from/a denied/a
  expect_fail "$dir" "$base" "duplicate (from, denied) keys"
}

test_bootstrap_requires_the_approved_snapshot() {
  local dir="$tmp/bootstrap"
  git_init "$dir"
  printf 'base\n' >"$dir/base.txt"
  git -C "$dir" add base.txt
  git -C "$dir" commit -q -m "base"
  local base
  base="$(git -C "$dir" rev-parse HEAD)"

  local args=()
  local index
  for index in $(seq 1 39); do
    args+=("from/$index" "denied/$index")
  done
  write_rows "$dir" "${args[@]}"
  local keys_file="$dir/initial.keys"
  bootstrap_keys "$dir/$ratchet_file" >"$keys_file"
  local expected_hash
  expected_hash="$(hash_file "$keys_file")"
  expect_bootstrap_pass "$dir" "$base" 39 "$expected_hash"

  args[76]="from/replacement"
  write_rows "$dir" "${args[@]}"
  expect_bootstrap_fail "$dir" "$base" 39 "$expected_hash"

  args[76]="from/39"
  args+=("from/40" "denied/40")
  write_rows "$dir" "${args[@]}"
  expect_bootstrap_fail "$dir" "$base" 39 "$expected_hash"
}

test_added_at_rewrite_fails() {
  local dir="$tmp/added-at"
  git_init "$dir"
  write_rows "$dir" from/a denied/a from/b denied/b
  commit_ratchet "$dir"
  local base
  base="$(git -C "$dir" rev-parse HEAD)"

  # Refreshing the date on an approved row would make an old debt look new
  # without touching a single import.
  sed -i.bak 's/\t2026-07-24$/\t2026-09-01/' "$dir/$ratchet_file"
  rm -f "$dir/$ratchet_file.bak"
  expect_fail "$dir" "$base" "from=from/a denied=denied/a added_at=2026-07-24 -> 2026-09-01"
  expect_fail "$dir" "$base" "moved added_at on an already-approved exception"

  # Backdating is the same rule.
  write_rows "$dir" from/a denied/a from/b denied/b
  sed -i.bak 's/^from\/b\(.*\)\t2026-07-24$/from\/b\1\t2020-01-01/' "$dir/$ratchet_file"
  rm -f "$dir/$ratchet_file.bak"
  expect_fail "$dir" "$base" "from=from/b denied=denied/b added_at=2026-07-24 -> 2020-01-01"

  # A row that is newly registered carries whatever date it likes: the new-key
  # error owns that case, and the date check must not double-report it.
  write_rows "$dir" from/a denied/a from/b denied/b
  row_added_at=2026-09-01 write_rows_appended "$dir" from/c denied/c
  expect_fail "$dir" "$base" "from=from/c denied=denied/c"
}

# write_rows_appended adds rows to an existing registry, keeping the rows already
# written (write_rows rewrites the file from scratch).
write_rows_appended() {
  local dir="$1"
  shift
  while (( $# > 0 )); do
    row "$1" "$2" >>"$dir/$ratchet_file"
    shift 2
  done
}

test_stale_base_points_at_the_rebase() {
  local dir="$tmp/stale-base"
  git_init "$dir"
  printf 'base\n' >"$dir/base.txt"
  git -C "$dir" add base.txt
  git -C "$dir" commit -q -m "base predating the gate"
  local base
  base="$(git -C "$dir" rev-parse HEAD)"

  # A branch that forked before the gate landed and registers one exception must
  # be told to rebase — and must still see the key it registered, so it can fix
  # the dependency instead of editing the approved baseline.
  write_rows "$dir" from/a denied/a
  expect_fail "$dir" "$base" "rebase onto the target branch"
  expect_fail "$dir" "$base" "carries no $ratchet_file"
  expect_fail "$dir" "$base" "from=from/a denied=denied/a"

  # The same stale base with nothing registered stays green: an old base is not
  # itself a failure.
  write_rows "$dir"
  expect_pass "$dir" "$base"
}

test_invalid_base_fails() {
  local dir="$tmp/invalid-base"
  git_init "$dir"
  write_rows "$dir" from/a denied/a
  commit_ratchet "$dir"
  expect_fail "$dir" missing-revision "base revision does not exist"
}

test_unchanged_and_metadata_changes_pass
test_deletion_passes
test_addition_fails
test_equal_count_replacement_fails
test_malformed_and_missing_current_file_fail
test_surrounding_whitespace_fails
test_crlf_rows_pass
test_duplicate_key_fails
test_bootstrap_requires_the_approved_snapshot
test_added_at_rewrite_fails
test_stale_base_points_at_the_rebase
test_invalid_base_fails
