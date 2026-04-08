#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# Source helpers without running the backup entrypoint.
source "$SCRIPT_DIR/lark-backup.sh"

assert_eq() {
  local expected="$1"
  local actual="$2"
  local message="$3"

  if [[ "$expected" != "$actual" ]]; then
    echo "assertion failed: $message" >&2
    echo "expected: $expected" >&2
    echo "actual:   $actual" >&2
    exit 1
  fi
}

assert_eq "2" "$(index_width 0)" "empty collections keep two-digit compatibility"
assert_eq "2" "$(index_width 9)" "single-digit totals still render two digits"
assert_eq "2" "$(index_width 99)" "double-digit totals stay two digits"
assert_eq "3" "$(index_width 100)" "three-digit totals widen to three digits"
assert_eq "4" "$(index_width 1000)" "four-digit totals widen to four digits"

assert_eq "01" "$(format_index 1 9)" "single-digit totals stay zero-padded"
assert_eq "99" "$(format_index 99 99)" "two-digit totals do not over-pad"
assert_eq "001" "$(format_index 1 100)" "three-digit totals pad to width three"
assert_eq "020" "$(format_index 20 100)" "intermediate indices share the same width"
assert_eq "100" "$(format_index 100 100)" "last index keeps natural width"
assert_eq "0007" "$(format_index 7 1000)" "four-digit totals pad to width four"

echo "lark-backup smoke test OK"
