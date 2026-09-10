#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

# Builds the merge-request preview package for the current checkout: one
# cross-compiled binary per configured platform, then the tarball.
#
# Runs the same way by hand as it does in the Preview Package job, so a
# preview can be produced locally without the pipeline.
#
# Progress goes to stderr and the manifest to stdout, so a caller can read
# the build it just produced.
#
# Usage: .codebase/preview/build.sh [output directory]
#   PREVIEW_LABEL      names the build in the version; defaults to the branch,
#                      then to "preview" on a detached HEAD.
#   PREVIEW_PLATFORMS  overrides the platform list from config.env.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"

# Sourcing config.env would overwrite what the caller passed, so hold the
# override across it. config.env stays declarative KEY=VALUE, which is what
# pipeline.test.sh reads.
platforms_override="${PREVIEW_PLATFORMS:-}"

set -a
# shellcheck source=.codebase/preview/config.env
. .codebase/preview/config.env
# The release identity travels into the package so the launcher can tell
# someone on a preview build how to get back to a released one.
# shellcheck source=.codebase/release/config.env
. .codebase/release/config.env
set +a

PREVIEW_PLATFORMS="${platforms_override:-$PREVIEW_PLATFORMS}"

OUT_DIR="${1:-preview-package}"

# The branch is the default label, so a package built from a merge request
# names it without the pipeline passing anything. A detached HEAD reports
# "HEAD", which says nothing, so fall through to a fixed word.
default_label="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || true)"
[ "$default_label" != HEAD ] || default_label=""

# The label becomes a semver prerelease identifier, which admits only
# alphanumerics and hyphens. Fold everything else rather than failing: the
# label is a human hint, and the commit below is what makes the version unique.
# Capped so a long branch name does not bury the commit in `--version`.
label="$(printf '%s' "${PREVIEW_LABEL:-${default_label:-preview}}" | tr -c 'A-Za-z0-9-' '-' | cut -c1-24 | sed 's/^-*//; s/-*$//')"
[ -n "$label" ] || label=preview

commit="$(git rev-parse --short=7 HEAD)"
base_version="$(node -p "require('./package.json').version")"
# The g prefix keeps an all-digit short hash from becoming a numeric semver
# identifier, which npm rejects when it carries a leading zero.
version="${base_version}-${label}.g${commit}"
date="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

# Same guard as the merge gate: the fetch runs only while the script exists.
if [ -f scripts/fetch_meta.py ]; then
  python3 scripts/fetch_meta.py
else
  echo "no scripts/fetch_meta.py: the committed API catalog serves the registry" >&2
fi

bin_dir="$(mktemp -d)"
trap 'rm -rf -- "$bin_dir"' EXIT

ldflags="-s -w -X github.com/larksuite/cli/internal/build.Version=${version} -X github.com/larksuite/cli/internal/build.Date=${date}"

IFS=',' read -r -a platforms <<< "$PREVIEW_PLATFORMS"
for platform in "${platforms[@]}"; do
  goos="${platform%%/*}"
  goarch="${platform##*/}"
  if [ -z "$goos" ] || [ -z "$goarch" ] || [ "$goos" = "$platform" ]; then
    echo "PREVIEW_PLATFORMS entry ${platform} is not <goos>/<goarch>" >&2
    exit 1
  fi
  ext=""
  [ "$goos" != windows ] || ext=".exe"
  echo "building ${goos}/${goarch}" >&2
  # `go build .` and not `./main.go`. The root package carries the //go:embed
  # of skills/ and affordance/, so `lark-cli skills read` serves this branch's
  # content. The public preview build uses ./main.go to drop that content and
  # stay under pkg.pr.new's size limit; nothing here has that limit, and a
  # preview whose skills are empty is wrong exactly where it is used most.
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "$ldflags" -o "${bin_dir}/lark-cli-${goos}-${goarch}${ext}" .
done

rm -rf -- "$OUT_DIR"
mkdir -p "$OUT_DIR"
node .codebase/preview/build-preview-package.js \
  --bin-dir "$bin_dir" \
  --out "$OUT_DIR" \
  --name "$PREVIEW_PACKAGE" \
  --version "$version" \
  --label "$label" \
  --commit "$commit" \
  --release-package "$RELEASE_PACKAGE" \
  --release-registry "$RELEASE_REGISTRY"
