#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

# Publishes the verified release candidate to its registry, or confirms it is
# already there. The candidate is one launcher package plus one platform
# package per binary; every tarball is published as built — never rebuilt —
# and each package follows scripts/release-publish-policy.js exactly as the
# public workflow does: a missing version is published unless the channel
# already points past it; an identical existing version only has its dist-tag
# advanced or is left alone; an existing version with different bytes is
# refused, and so is moving a channel backwards.
#
# The platform packages go first and the launcher last, so from the moment
# the launcher is visible its optionalDependencies resolve. A run that failed
# midway is re-run as is: every package already on the registry is recognised
# and at most has its dist-tag advanced.
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
tag="$(read_manifest tag)"
candidate_sha="$(read_manifest source_sha)"
channel="$(read_manifest channel)"

if [[ "$name" != "$RELEASE_PACKAGE" || "$registry" != "$RELEASE_REGISTRY" ]]; then
  echo "Candidate is ${name} on ${registry}; this checkout releases ${RELEASE_PACKAGE} on ${RELEASE_REGISTRY}." >&2
  exit 1
fi

# Every package of the candidate as "<name> <filename> <integrity>", platform
# packages first and the launcher last. A platform package must be named
# after the launcher, or the launcher's optionalDependencies would point
# elsewhere.
packages="$(node - "$PWD/$manifest" "$RELEASE_PACKAGE" <<'NODE'
const manifest = require(process.argv[2]);
const launcher = process.argv[3];
const platforms = manifest.platform_packages;
if (!Array.isArray(platforms) || platforms.length === 0) throw new Error("manifest lists no platform packages");
for (const p of platforms) {
  if (!p.name.startsWith(`${launcher}-`)) throw new Error(`platform package ${p.name} is not named after ${launcher}`);
}
for (const p of [...platforms, manifest]) {
  for (const field of ["name", "filename", "integrity"]) {
    if (typeof p[field] !== "string" || !p[field]) throw new Error(`manifest entry ${p.name || "?"} lacks ${field}`);
  }
  process.stdout.write(`${p.name} ${p.filename} ${p.integrity}\n`);
}
NODE
)"
while read -r _ filename _; do
  [[ -f "$CANDIDATE/$filename" ]] || { echo "${CANDIDATE}/${filename} named by the manifest is missing." >&2; exit 1; }
done <<<"$packages"

# The tag must still point at the commit the candidate was built from.
eval "$(bash .codebase/release/source.sh "$tag")"
if [[ "$source_sha" != "$candidate_sha" ]]; then
  echo "Tag ${tag} now points at ${source_sha}; the candidate was built from ${candidate_sha}." >&2
  exit 1
fi

: "${BNPM_AUTH_TOKEN:?BNPM_AUTH_TOKEN is not set; configure it as a pipeline variable}"
host="$(node -p "new URL(process.argv[1]).host" "$registry")"
npmrc="$(mktemp)"
dist_tag_err="$(mktemp)"
trap 'rm -f -- "$npmrc" "$dist_tag_err"' EXIT
chmod 0600 "$npmrc"
printf '//%s/:_authToken=%s\n' "$host" "$BNPM_AUTH_TOKEN" > "$npmrc"
export NPM_CONFIG_USERCONFIG="$npmrc"

dist_tag=latest
[[ "$channel" != beta ]] || dist_tag=beta

# Applies the publication policy to one package and publishes, advances its
# dist-tag, or leaves it alone. Exits 1 when the policy refuses.
publish_one() {
  local pkg="$1" tgz="$CANDIDATE/$2" expected="$3"
  local version_exists=false integrity_matches=false published current decision action

  case "$(bash .codebase/release/registry-version.sh "$pkg" "$version" "$registry")" in
    exists)
      version_exists=true
      published="$(npm view "${pkg}@${version}" dist.integrity --json --registry "$registry" | tr -d '"[:space:]')"
      [[ "$published" != "$expected" ]] || integrity_matches=true
      ;;
    missing) ;;
    *) echo "Unexpected registry probe result for ${pkg}." >&2; return 1 ;;
  esac

  # Before the first publication the package itself is missing and npm
  # answers E404; that is "no channel yet". It can only mean that while the
  # version is missing too: a 404 for a package whose version the registry
  # just confirmed is a transient error, and reading it as an empty channel
  # would let advance-tag move the channel backwards.
  if current="$(npm view "$pkg" "dist-tags.${dist_tag}" --json --registry "$registry" 2>"$dist_tag_err" | tr -d '"[:space:]')"; then
    :
  elif [[ "$version_exists" == false ]] && node - "$dist_tag_err" <<'NODE'
const fs = require("node:fs");
const { isNpmVersionMissing } = require("./scripts/release-publish-policy");
process.exit(isNpmVersionMissing(fs.readFileSync(process.argv[2], "utf8")) ? 0 : 1);
NODE
  then
    current=""
  else
    cat "$dist_tag_err" >&2
    echo "Could not determine the ${dist_tag} dist-tag of ${pkg}." >&2
    return 1
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
      npm dist-tag add "${pkg}@${version}" "$dist_tag" --registry "$registry"
      ;;
    verify)
      echo "${pkg} ${dist_tag} already points to ${version} or a newer verified version."
      return 0
      ;;
    reject)
      node -e 'console.error(process.argv[2] + ": " + JSON.parse(process.argv[1]).reason); process.exit(1)' "$decision" "$pkg"
      ;;
    *)
      echo "Unexpected publish decision for ${pkg}: ${action}" >&2
      return 1
      ;;
  esac

  # What the registry now serves must be the bytes that were verified.
  published="$(npm view "${pkg}@${version}" dist.integrity --json --registry "$registry" | tr -d '"[:space:]')"
  if [[ "$published" != "$expected" ]]; then
    echo "Registry integrity ${published} of ${pkg} differs from the candidate ${expected}." >&2
    return 1
  fi
  echo "Published ${pkg}@${version} to ${registry} as ${dist_tag}."
}

# The list is read on its own descriptor so that nothing publish_one runs can
# consume it from stdin and end the loop early; then the count is checked,
# because a loop that quietly stopped would leave the launcher unpublished
# behind a green job.
published=0
while read -r -u 3 pkg filename integrity; do
  publish_one "$pkg" "$filename" "$integrity"
  published=$((published + 1))
done 3<<<"$packages"
expected_count="$(wc -l <<<"$packages" | tr -d '[:space:]')"
if [[ "$published" != "$expected_count" ]]; then
  echo "Processed ${published} of ${expected_count} packages." >&2
  exit 1
fi
