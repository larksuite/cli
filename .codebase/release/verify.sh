#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

# Installs the release candidate the way a user would — `npm install -g` of the
# packed tarball, postinstall included — into a throwaway prefix, and checks
# that the CLI that comes out is the candidate: right version, only this
# platform's binary left behind, and `lark-cli update` owned by the launcher.
# This is the gate between building the candidate and publishing it.
#
# Usage: .codebase/release/verify.sh [candidate-dir]
set -euo pipefail

CANDIDATE="${1:-release-candidate}"

cd "$(git rev-parse --show-toplevel)"
manifest="$CANDIDATE/manifest.json"
[[ -f "$manifest" ]] || { echo "${manifest} not found; run .codebase/release/build.sh first." >&2; exit 1; }

read_manifest() { node -p "require(process.argv[1])[process.argv[2]]" "$PWD/$manifest" "$1"; }
name="$(read_manifest name)"
version="$(read_manifest version)"
registry="$(read_manifest registry)"
filename="$(read_manifest filename)"
tgz="$CANDIDATE/$filename"
[[ -f "$tgz" ]] || { echo "${tgz} named by the manifest is missing." >&2; exit 1; }

prefix="$(mktemp -d)"
trap 'rm -rf -- "$prefix"' EXIT

# The package has no dependencies; the registry is named only so npm never
# consults a default that might be unreachable here.
npm install -g "./$tgz" --prefix "$prefix" --registry "$registry" --no-audit --no-fund --loglevel=error

launcher="$prefix/bin/lark-cli"
[[ -x "$launcher" ]] || { echo "npm did not link ${launcher}." >&2; exit 1; }
reported="$("$launcher" --version)"
escaped_version="${version//./\\.}"
if ! grep -Eq "(^|[^0-9A-Za-z.])${escaped_version}([^0-9A-Za-z.]|$)" <<<"$reported"; then
  echo "Installed CLI reports '${reported}', expected version ${version}." >&2
  exit 1
fi
"$launcher" --help >/dev/null

bin_dir="$prefix/lib/node_modules/$name/bin"
remaining="$(ls -1 "$bin_dir" | wc -l | tr -d '[:space:]')"
if [[ "$remaining" != 1 ]]; then
  echo "postinstall left ${remaining} binaries in ${bin_dir}; expected only this platform's." >&2
  exit 1
fi

# The launcher, not the binary, owns `update`: a flag the binary would accept
# is rejected by the launcher's own parser, without any network access.
if "$launcher" update --force >/dev/null 2>&1; then
  echo "lark-cli update is not being handled by the internal launcher." >&2
  exit 1
fi

echo "Verified ${name}@${version} from ${filename}: ${reported}"
