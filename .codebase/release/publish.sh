#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

# Publishes the verified release candidate to its registry, or confirms it is
# already there. The tarball is published as built — never rebuilt — and the
# decision follows scripts/release-publish-policy.js exactly as the public
# workflow does: a missing version is published unless the channel already
# points past it; an identical existing version only has its dist-tag advanced
# or is left alone; an existing version with different bytes is refused, and
# so is moving a channel backwards.
#
# Usage: .codebase/release/publish.sh [candidate-dir]
# Env:   BNPM_AUTH_TOKEN  publish token for the registry (pipeline variable).
set -euo pipefail

CANDIDATE="${1:-release-candidate}"

cd "$(git rev-parse --show-toplevel)"
set -a
# shellcheck disable=SC1091
. .codebase/release/config.env
set +a

manifest="$CANDIDATE/manifest.json"
[[ -f "$manifest" ]] || { echo "${manifest} not found." >&2; exit 1; }
read_manifest() { node -p "require(process.argv[1])[process.argv[2]]" "$PWD/$manifest" "$1"; }
name="$(read_manifest name)"
version="$(read_manifest version)"
registry="$(read_manifest registry)"
filename="$(read_manifest filename)"
integrity="$(read_manifest integrity)"
tag="$(read_manifest tag)"
candidate_sha="$(read_manifest source_sha)"
channel="$(read_manifest channel)"
tgz="$CANDIDATE/$filename"

if [[ "$name" != "$RELEASE_PACKAGE" || "$registry" != "$RELEASE_REGISTRY" ]]; then
  echo "Candidate is ${name} on ${registry}; this checkout releases ${RELEASE_PACKAGE} on ${RELEASE_REGISTRY}." >&2
  exit 1
fi
[[ -f "$tgz" ]] || { echo "${tgz} named by the manifest is missing." >&2; exit 1; }

# The tag must still point at the commit the candidate was built from.
eval "$(bash .codebase/release/source.sh "$tag")"
if [[ "$source_sha" != "$candidate_sha" ]]; then
  echo "Tag ${tag} now points at ${source_sha}; the candidate was built from ${candidate_sha}." >&2
  exit 1
fi

: "${BNPM_AUTH_TOKEN:?BNPM_AUTH_TOKEN is not set; configure it as a pipeline variable}"
host="$(node -p "new URL(process.argv[1]).host" "$registry")"
npmrc="$(mktemp)"
trap 'rm -f -- "$npmrc"' EXIT
chmod 0600 "$npmrc"
printf '//%s/:_authToken=%s\n' "$host" "$BNPM_AUTH_TOKEN" > "$npmrc"
export NPM_CONFIG_USERCONFIG="$npmrc"

dist_tag=latest
[[ "$channel" != beta ]] || dist_tag=beta

version_exists=false
integrity_matches=false
case "$(bash .codebase/release/registry-version.sh "$name" "$version" "$registry")" in
  exists)
    version_exists=true
    published="$(npm view "${name}@${version}" dist.integrity --json --registry "$registry" | tr -d '"[:space:]')"
    [[ "$published" != "$integrity" ]] || integrity_matches=true
    ;;
  missing) ;;
  *) echo "Unexpected registry probe result." >&2; exit 1 ;;
esac

if ! current="$(npm view "$name" "dist-tags.${dist_tag}" --json --registry "$registry" | tr -d '"[:space:]')"; then
  echo "Could not determine the ${dist_tag} dist-tag of ${name}." >&2
  exit 1
fi
[[ -n "$current" && "$current" != null ]] || current=null

decision="$(node - "$version" "$version_exists" "$integrity_matches" "$current" <<'NODE'
const { decideNpmPublish } = require("./scripts/release-publish-policy");
const [version, versionExists, integrityMatches, channelVersion] = process.argv.slice(2);
process.stdout.write(JSON.stringify(decideNpmPublish({
  version,
  versionExists: versionExists === "true",
  integrityMatches: integrityMatches === "true",
  channelVersion: channelVersion === "null" ? null : channelVersion,
})));
NODE
)"
action="$(node -e 'process.stdout.write(JSON.parse(process.argv[1]).action)' "$decision")"

case "$action" in
  publish)
    npm publish "./$tgz" --registry "$registry" --tag "$dist_tag"
    ;;
  advance-tag)
    npm dist-tag add "${name}@${version}" "$dist_tag" --registry "$registry"
    ;;
  verify)
    echo "${name} ${dist_tag} already points to ${version} or a newer verified version."
    exit 0
    ;;
  reject)
    node -e 'console.error(JSON.parse(process.argv[1]).reason); process.exit(1)' "$decision"
    ;;
  *)
    echo "Unexpected publish decision: ${action}" >&2
    exit 1
    ;;
esac

# What the registry now serves must be the bytes that were verified.
published="$(npm view "${name}@${version}" dist.integrity --json --registry "$registry" | tr -d '"[:space:]')"
if [[ "$published" != "$integrity" ]]; then
  echo "Registry integrity ${published} differs from the candidate ${integrity}." >&2
  exit 1
fi
echo "Published ${name}@${version} to ${registry} as ${dist_tag}."
