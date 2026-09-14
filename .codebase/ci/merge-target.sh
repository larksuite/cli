#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

# Makes a merge request run verify what landing the change would produce.
#
# actions/checkout checks out CI_HEAD_SHA, the source branch tip, so every job
# reads a tree that has never met the current target branch. Two changes that
# touch different regions of the same file merge without a conflict and can
# still be incompatible, and nothing in the run notices. main broke exactly
# that way on 2026-09-14: the upstream sync 8629a625 gave an affordance test
# helper a third parameter and fixed the call sites that existed then, while
# the branch behind !17 had been cut before that sync and added new two-argument
# calls. Both runs were green; `go vet ./...` failed the moment they sat on main
# together. The public repository does not have this problem, because a
# pull_request run checks out refs/pull/<n>/merge rather than the branch tip.
#
# The platform exposes no merge ref, so the merge happens here. actions/checkout
# fetches every branch (INPUT_DEPTH=0), so the target commit is already present.
#
# Nothing happens on a trunk, tag or manual run, where there is no target, and
# nothing happens when the target is already in HEAD. A conflict fails the job:
# the author has to rebase, which is what the platform would ask for at merge
# time anyway.

set -euo pipefail

if root="$(git rev-parse --show-toplevel 2>/dev/null)"; then
  cd "$root"
fi

target="${CI_EVENT_CHANGE_TARGET_SHA:-}"
target_branch="${CI_EVENT_CHANGE_TARGET_BRANCH:-the target branch}"

if [ -z "$target" ] || [[ "$target" =~ ^0{40}$ ]]; then
  echo "no merge request target in this run; the checkout is used as it is"
  exit 0
fi

if ! git cat-file -e "${target}^{commit}" 2>/dev/null; then
  # Only reachable if the checkout stops fetching every branch.
  git fetch --no-tags --quiet origin "$target" 2>/dev/null || true
fi
if ! git cat-file -e "${target}^{commit}" 2>/dev/null; then
  echo "the target commit $target is missing from this checkout, so the run" >&2
  echo "would verify the source branch alone; failing instead" >&2
  exit 1
fi

if git merge-base --is-ancestor "$target" HEAD; then
  echo "$target_branch ($target) is already in HEAD; nothing to merge"
  exit 0
fi

# --no-ff keeps HEAD a commit of its own even when the source branch is behind
# the target. A fast-forward would land HEAD exactly on origin/main, which the
# baseline resolution in ci.yaml reads as a trunk run.
echo "merging $target_branch ($target) into $(git rev-parse HEAD)"
if ! git -c user.name="Codebase CI" -c user.email="ci@localhost" \
  merge --no-edit --no-ff "$target"; then
  git merge --abort 2>/dev/null || true
  echo "cannot merge $target_branch into this branch without conflicts." >&2
  echo "Rebase the source branch onto $target_branch and push again." >&2
  exit 1
fi

echo "verifying $(git rev-parse HEAD), the result of the merge"
