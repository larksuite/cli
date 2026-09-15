#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

# Installs the release candidate the way a user would — `npm install -g` of the
# launcher package with this platform's package resolving next to it — into a
# throwaway prefix, and checks that the CLI that comes out is the candidate:
# right version, this platform's binary and no other, and `lark-cli update`
# owned by the launcher. This is the gate between building the candidate and
# publishing it.
#
# Nothing is published yet, so the platform package cannot come from the
# registry: its tarball is passed to npm alongside the launcher's, and npm
# satisfies the launcher's optionalDependency with it instead of fetching. The
# other platforms' optionalDependencies do not exist on the registry either;
# npm skips them, as it would skip them for the wrong os/cpu after publishing.
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

# The platform package for this host, as the launcher will look for it.
platform_name="$(node -p "require(process.argv[1]).platformPackageName(process.argv[2])" "$PWD/.codebase/release/run.js" "$name")"
[[ "$platform_name" != null ]] || { echo "The launcher has no platform name for $(uname -sm); verify_install cannot run here." >&2; exit 1; }
platform_filename="$(node -p "(require(process.argv[1]).platform_packages.find((p) => p.name === process.argv[2]) || {}).filename || ''" "$PWD/$manifest" "$platform_name")"
[[ -n "$platform_filename" ]] || { echo "The candidate has no package ${platform_name} for this host." >&2; exit 1; }
platform_tgz="$CANDIDATE/$platform_filename"
[[ -f "$platform_tgz" ]] || { echo "${platform_tgz} named by the manifest is missing." >&2; exit 1; }

prefix="$(mktemp -d)"
trap 'rm -rf -- "$prefix"' EXIT

npm install -g "./$tgz" "./$platform_tgz" --prefix "$prefix" --registry "$registry" --no-audit --no-fund --loglevel=error

launcher="$prefix/bin/lark-cli"
[[ -x "$launcher" ]] || { echo "npm did not link ${launcher}." >&2; exit 1; }
reported="$("$launcher" --version)"
escaped_version="${version//./\\.}"
if ! grep -Eq "(^|[^0-9A-Za-z.])${escaped_version}([^0-9A-Za-z.]|$)" <<<"$reported"; then
  echo "Installed CLI reports '${reported}', expected version ${version}." >&2
  exit 1
fi
"$launcher" --help >/dev/null

# Exactly the launcher and this platform's package are installed: no other
# platform's binary came along, and the launcher itself carries none.
installed="$(cd "$prefix/lib/node_modules" && find . -mindepth 2 -maxdepth 3 -name package.json -not -path '*/node_modules/*' -exec dirname {} \; | sed 's#^\./##' | sort)"
expected_installed="$(printf '%s\n%s\n' "$name" "$platform_name" | sort)"
if [[ "$installed" != "$expected_installed" ]]; then
  echo "Installed packages differ from the launcher plus this platform's package:" >&2
  echo "$installed" >&2
  exit 1
fi
[[ ! -e "$prefix/lib/node_modules/$name/bin" ]] || { echo "The launcher package ships a bin/ directory; binaries belong to the platform packages." >&2; exit 1; }
bin_dir="$prefix/lib/node_modules/$platform_name/bin"
remaining="$(ls -1 "$bin_dir" | wc -l | tr -d '[:space:]')"
if [[ "$remaining" != 1 ]]; then
  echo "${bin_dir} holds ${remaining} files; expected exactly one binary." >&2
  exit 1
fi

# The launcher, not the binary, owns `update`: a flag the binary would accept
# is rejected by the launcher's own parser, without any network access.
if "$launcher" update --force >/dev/null 2>&1; then
  echo "lark-cli update is not being handled by the internal launcher." >&2
  exit 1
fi

echo "Verified ${name}@${version} from ${filename} with ${platform_name} from ${platform_filename}: ${reported}"
