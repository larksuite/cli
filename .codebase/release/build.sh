#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

# Builds the internal release candidate for <tag>: the unchanged public
# GoReleaser build, every binary checked for the pinned Go toolchain, and the
# binaries packed into the registry package by
# .codebase/release/build-registry-package.js.
#
# Output: release-candidate/<name>-<version>.tgz and release-candidate/manifest.json.
# Nothing here publishes; the candidate is what later stages verify and ship.
#
# Usage: .codebase/release/build.sh <tag>
# Env:   GORELEASER  path of a GoReleaser binary to use instead of installing
#                    v2.17.1 through the Go module proxy.
set -euo pipefail

TAG="${1:?usage: $0 vX.Y.Z[-beta.N]}"
GORELEASER_VERSION=v2.17.1
OUT=release-candidate

cd "$(git rev-parse --show-toplevel)"
set -a
# shellcheck disable=SC1091
. .codebase/release/config.env
set +a
: "${RELEASE_PACKAGE:?}" "${RELEASE_REGISTRY:?}" "${RELEASE_GO_VERSION:?}"

eval "$(bash .codebase/release/source.sh "$TAG")"
: "${version:?}" "${channel:?}" "${source_sha:?}"

# The same toolchain the public release pins. The image's own Go is only the
# bootstrap: GOTOOLCHAIN fetches the pinned one through the module proxy, and
# that download is refused unless a checksum database is reachable, which the
# proxy serves under /sumdb/sum.golang.org.
export GOTOOLCHAIN="go${RELEASE_GO_VERSION}"
export GOSUMDB=sum.golang.org
export GOPROXY="${GOPROXY:-https://goproxy.byted.org}"
go version

# GoReleaser derives the changelog from git history and refuses to guess on a
# shallow clone.
if [[ "$(git rev-parse --is-shallow-repository)" == true ]]; then
  git fetch --quiet --unshallow origin
fi

# Upstream #2232 removed scripts/fetch_meta.py: the API catalog is committed
# under internal/registry/catalog/ and needs no fetch. Keep the fetch for
# tags cut before that landed.
if [[ -f scripts/fetch_meta.py ]]; then
  python3 scripts/fetch_meta.py
fi

if [[ -z "${GORELEASER:-}" ]]; then
  GOBIN="$PWD/.tmp/release-bin" go install "github.com/goreleaser/goreleaser/v2@${GORELEASER_VERSION}"
  GORELEASER="$PWD/.tmp/release-bin/goreleaser"
fi
"$GORELEASER" --version

GORELEASER_CURRENT_TAG="$TAG" "$GORELEASER" release --clean --skip=publish

# Every binary must carry the pinned toolchain.
expected="go${RELEASE_GO_VERSION}"
count=0
while IFS= read -r -d '' binary; do
  count=$((count + 1))
  actual="$(go version -m "$binary" | awk 'NR == 1 { print $NF }')"
  if [[ "$actual" != "$expected" ]]; then
    echo "Release binary ${binary} was built with ${actual:-an unknown Go version}; expected ${expected}." >&2
    exit 1
  fi
  echo "Verified ${binary}: ${actual}"
done < <(find dist -type f \( -name lark-cli -o -name lark-cli.exe \) -print0)
if (( count == 0 )); then
  echo "GoReleaser produced no binaries to verify." >&2
  exit 1
fi

rm -rf "$OUT"
node .codebase/release/build-registry-package.js \
  --dist dist --out "$OUT" --name "$RELEASE_PACKAGE" --registry "$RELEASE_REGISTRY"

node - "$OUT/manifest.json" "$TAG" "$source_sha" "$channel" "$version" "$expected" <<'NODE'
const fs = require("node:fs");
const [file, tag, sourceSha, channel, version, goVersion] = process.argv.slice(2);
const manifest = JSON.parse(fs.readFileSync(file, "utf8"));
if (manifest.version !== version) {
  throw new Error(`packaged version ${manifest.version} does not match tag version ${version}`);
}
Object.assign(manifest, { tag, source_sha: sourceSha, channel, go_version: goVersion });
fs.writeFileSync(file, JSON.stringify(manifest, null, 2) + "\n");
console.log(JSON.stringify(manifest, null, 2));
NODE
