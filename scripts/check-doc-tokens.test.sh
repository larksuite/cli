#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
#
# Tests check-doc-tokens.sh against tests/fixtures/doc-tokens.
#
# The check is only worth running if it reports realistic values and stays quiet
# on placeholders. Measured before the fixture existed, the script caught 3 of
# the 6 realistic values below and reported one placeholder, and nothing in CI
# would have noticed either way.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
script="$repo_root/scripts/check-doc-tokens.sh"
fixtures="$repo_root/tests/fixtures/doc-tokens"

failures=0

fail() {
  echo "FAIL: $*" >&2
  failures=$((failures + 1))
}

# --- the realistic fixture must be reported, value by value -----------------
catch_out=""
catch_rc=0
catch_out=$("$script" "$fixtures/lark-catch") || catch_rc=$?

if [[ $catch_rc -ne 1 ]]; then
  fail "realistic fixture: expected exit 1, got $catch_rc"
fi

for value in \
  'wikcnAbc1Abc1Abc1' \
  'ou_00112233445566778899aabbccddeeff' \
  'Qw3rQw3rQw3rQw3rQw3rQw3r' \
  'bascn***************Qw3rT' \
  'Nq7bNq7bNq7bNq7bNq7bNq7b' \
  'cli_00112233445566aabb' \
  'Zx9pZx9pZx9pZx9pZx9pZx9pxxxx'; do
  if [[ "$catch_out" != *"$value"* ]]; then
    fail "realistic fixture: $value was not reported"
  fi
done

# --- the placeholder fixture must produce nothing at all --------------------
skip_out=""
skip_rc=0
skip_out=$("$script" "$fixtures/lark-skip") || skip_rc=$?

if [[ $skip_rc -ne 0 ]]; then
  fail "placeholder fixture: expected exit 0, got $skip_rc. Reported:"
  echo "$skip_out" >&2
fi

# --- a value must be reported once, however many rules it trips -------------
# An `ou_` id in a JSON body matches rule 1 on its prefix and rule 2 on its key.
open_id_lines=$(grep -c 'ou_00112233445566778899aabbccddeeff' <<<"$catch_out" || true)
if [[ "$open_id_lines" -ne 1 ]]; then
  fail "realistic fixture: open id reported $open_id_lines times, want 1"
fi

if [[ $failures -gt 0 ]]; then
  echo "check-doc-tokens.test: $failures assertion(s) failed" >&2
  exit 1
fi

echo "✅  check-doc-tokens.test: fixture coverage holds."
