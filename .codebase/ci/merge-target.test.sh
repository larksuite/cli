#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

# Behavioural tests for merge-target.sh, run against a real git repository that
# reproduces the shape of a merge request checkout: a source branch cut from an
# older target, and a target that has moved since.
set -euo pipefail

repo="$(git rev-parse --show-toplevel)"
script="$repo/.codebase/ci/merge-target.sh"
work="$(mktemp -d)"
trap 'rm -rf -- "$work"' EXIT

fail() { echo "merge-target: $*" >&2; exit 1; }

git init -q -b main "$work/src"
cd "$work/src"
git config user.email merge-target-test@example.com
git config user.name "merge target test"
git config commit.gpgsign false

# helper.txt is the file both sides edit, in different regions.
printf 'one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\n' > helper.txt
git add -A && git commit -q -m "base"
base="$(git rev-parse HEAD)"

# The target branch moves on.
sed -i.bak 's/^one$/ONE/' helper.txt && rm -f helper.txt.bak
git commit -qam "target moves"
target="$(git rev-parse HEAD)"

# The source branch was cut before that and edits the other end of the file.
git checkout -q -b source "$base"
sed -i.bak 's/^ten$/TEN/' helper.txt && rm -f helper.txt.bak
git commit -qam "source changes"
source_head="$(git rev-parse HEAD)"

run() { CI_EVENT_CHANGE_TARGET_SHA="${1-}" CI_EVENT_CHANGE_TARGET_BRANCH=main bash "$script"; }

# --- no target: a trunk, tag or manual run is left alone ---
out="$(run "")"
[[ "$out" == *"no merge request target"* ]] || fail "empty target: unexpected output: $out"
[[ "$(git rev-parse HEAD)" == "$source_head" ]] || fail "empty target: HEAD moved"

# --- an all-zero target is treated as no target ---
out="$(run "0000000000000000000000000000000000000000")"
[[ "$out" == *"no merge request target"* ]] || fail "zero target: unexpected output: $out"
[[ "$(git rev-parse HEAD)" == "$source_head" ]] || fail "zero target: HEAD moved"

# --- target already in HEAD: nothing to do ---
out="$(run "$base")"
[[ "$out" == *"already in HEAD"* ]] || fail "ancestor target: unexpected output: $out"
[[ "$(git rev-parse HEAD)" == "$source_head" ]] || fail "ancestor target: HEAD moved"

# --- the real case: the merge happens and both edits are present ---
out="$(run "$target")"
[[ "$out" == *"result of the merge"* ]] || fail "moved target: unexpected output: $out"
[[ "$(git rev-parse HEAD)" != "$source_head" ]] || fail "moved target: HEAD did not move"
git merge-base --is-ancestor "$target" HEAD || fail "moved target: target is not in HEAD"
git merge-base --is-ancestor "$source_head" HEAD || fail "moved target: source is not in HEAD"
grep -q '^ONE$' helper.txt || fail "moved target: the target's edit is missing"
grep -q '^TEN$' helper.txt || fail "moved target: the source's edit is missing"

# --- a conflict fails the job and leaves no half-merged tree behind ---
git checkout -q -b conflicting "$base"
sed -i.bak 's/^one$/uno/' helper.txt && rm -f helper.txt.bak
git commit -qam "source touches the same line"
conflicting_head="$(git rev-parse HEAD)"
if out="$(run "$target" 2>&1)"; then
  fail "conflicting target: expected a failure, got: $out"
fi
[[ "$out" == *"Rebase the source branch"* ]] || fail "conflicting target: unexpected output: $out"
[[ "$(git rev-parse HEAD)" == "$conflicting_head" ]] || fail "conflicting target: HEAD moved"
[[ -z "$(git status --porcelain)" ]] || fail "conflicting target: the tree was left dirty"

# --- a target this checkout does not have fails instead of verifying the branch alone ---
git checkout -q source
if out="$(run "0123456789012345678901234567890123456789" 2>&1)"; then
  fail "missing target: expected a failure, got: $out"
fi
[[ "$out" == *"missing from this checkout"* ]] || fail "missing target: unexpected output: $out"

echo "merge-target: ok"
