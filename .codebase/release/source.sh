#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

# Verifies that the checkout is a valid source for the release tag and prints
# the facts later stages need, one `key=value` per line:
#
#   version=X.Y.Z[-beta.N]   channel=stable|beta   source_sha=<commit>
#
# Every stage of the internal release re-runs this against the tag it was
# triggered for, so a tag moved after preflight fails the stage that notices
# instead of publishing a commit nobody validated. Two rules: the tag must
# equal the package version (the public scripts/release-preflight.js, used
# unchanged), and the tagged commit must already be contained in origin/main —
# for beta tags as well as stable ones, which is stricter than the public
# policy and enforced here.
#
# Usage: .codebase/release/source.sh <tag> [remote]
set -euo pipefail

TAG="${1:?usage: $0 vX.Y.Z[-beta.N] [remote]}"
# RELEASE_REMOTE lets a rehearsal verify against a staging remote; the
# pipeline always uses origin.
REMOTE="${2:-${RELEASE_REMOTE:-origin}}"

cd "$(git rev-parse --show-toplevel)"

git fetch --quiet --no-tags "$REMOTE" \
  "+refs/heads/main:refs/remotes/${REMOTE}/main" \
  "+refs/tags/${TAG}:refs/tags/${TAG}"

head_sha="$(git rev-parse --verify 'HEAD^{commit}')"
tag_sha="$(git rev-parse --verify "refs/tags/${TAG}^{commit}")"
if [[ "$tag_sha" != "$head_sha" ]]; then
  echo "Tag ${TAG} resolves to ${tag_sha} but the checkout is at ${head_sha}." >&2
  exit 1
fi
main_sha="$(git rev-parse --verify "refs/remotes/${REMOTE}/main^{commit}")"
if ! git merge-base --is-ancestor "$head_sha" "$main_sha"; then
  echo "Tag ${TAG} points at ${head_sha}, which is not contained in ${REMOTE}/main. Internal releases, beta included, are cut from main." >&2
  exit 1
fi

# release-preflight.js writes its JSON verdict to stdout on success and to
# stderr (exit 1) on failure, so a bad version stops here with its hint shown.
preflight="$(node scripts/release-preflight.js --tag "$TAG")"

node - "$preflight" "$head_sha" <<'NODE'
const [payload, sourceSha] = process.argv.slice(2);
const result = JSON.parse(payload);
if (result?.ok !== true || !["stable", "beta"].includes(result.data?.releaseChannel)) {
  throw new Error("release preflight returned an invalid success payload");
}
process.stdout.write(`version=${result.data.tagVersion}\nchannel=${result.data.releaseChannel}\nsource_sha=${sourceSha}\n`);
NODE
