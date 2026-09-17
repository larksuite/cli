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
# instead of publishing a commit nobody validated. Two rules, both from the
# public scripts/release-preflight.js used unchanged: the tag must equal the
# package version, and the source policy for the channel must hold — a stable
# tag must point at a commit already contained in origin/main, while a beta
# tag may be cut from a feature branch, which is what a beta is for. A beta
# cut from main is accepted with the public policy's warning on stderr.
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
source_in_main=false
if git merge-base --is-ancestor "$head_sha" "$main_sha"; then
  source_in_main=true
fi

# release-preflight.js writes its JSON verdict to stdout on success and to
# stderr (exit 1) on failure, so a bad version stops here with its hint shown.
preflight="$(node scripts/release-preflight.js --tag "$TAG")"

node - "$preflight" "$head_sha" "$main_sha" "$source_in_main" <<'NODE'
const { validateReleaseSourcePolicy } = require("./scripts/release-preflight");
const [payload, sourceSha, mainSha, sourceInMain] = process.argv.slice(2);
const result = JSON.parse(payload);
if (result?.ok !== true || !["stable", "beta"].includes(result.data?.releaseChannel)) {
  throw new Error("release preflight returned an invalid success payload");
}
const channel = result.data.releaseChannel;
const policy = validateReleaseSourcePolicy(channel, sourceSha, mainSha, sourceInMain === "true");
if (policy.ok !== true) {
  process.stderr.write(`${policy.error.message}. ${policy.error.hint}\n`);
  process.exit(1);
}
// Warnings go to stderr: stdout is eval'd by the caller and carries facts only.
if (policy.data.warning) process.stderr.write(`${policy.data.warning}\n`);
process.stdout.write(`version=${result.data.tagVersion}\nchannel=${channel}\nsource_sha=${sourceSha}\n`);
NODE
