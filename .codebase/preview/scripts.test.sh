#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

# Behavioural test of .codebase/preview/build.sh against a fixture repository
# and a stub `go` on PATH that records every invocation and writes a file where
# the real compiler would. What is asserted is the tarball: its version, what
# it carries, and that it cannot be published — the things a reviewer installs
# and the things that keep a preview from being mistaken for a release.
set -euo pipefail

repo="$(git rev-parse --show-toplevel)"
work="$(mktemp -d)"
trap 'rm -rf -- "$work"' EXIT

fail() { echo "preview scripts: $*" >&2; exit 1; }

# --- fixture: a repository carrying only what the preview build reads --------
src="$work/src"
mkdir -p "$src/.codebase/preview" "$src/.codebase/release" "$src/skills/lark-base/references"
cp "$repo"/.codebase/preview/build.sh "$repo"/.codebase/preview/build-preview-package.js "$repo"/.codebase/preview/config.env "$src/.codebase/preview/"
cp "$repo"/.codebase/release/build-registry-package.js "$repo"/.codebase/release/run.js "$repo"/.codebase/release/postinstall.js "$repo"/.codebase/release/config.env "$src/.codebase/release/"
printf '{"name":"@larksuite/cli","version":"1.2.3","description":"The official CLI","os":["darwin"],"cpu":["arm64"],"engines":{"node":">=16"},"license":"MIT"}\n' > "$src/package.json"
for file in CHANGELOG.md LICENSE README.md; do printf '%s\n' "$file" > "$src/$file"; done
printf '# base\n' > "$src/skills/lark-base/SKILL.md"
printf '# read\n' > "$src/skills/lark-base/references/read.md"
git -C "$src" init -q -b main
git -C "$src" config user.email preview-test@example.com
git -C "$src" config user.name "preview test"
git -C "$src" config commit.gpgsign false
git -C "$src" add -A
git -C "$src" commit -qm "fixture"
sha="$(git -C "$src" rev-parse --short=7 HEAD)"

# --- stub go: records the call and writes the file the loop expects ----------
stub="$work/stub"
mkdir -p "$stub"
cat > "$stub/go" <<'STUB'
#!/usr/bin/env bash
out=""
prev=""
for arg in "$@"; do
  [ "$prev" != "-o" ] || out="$arg"
  prev="$arg"
done
[ -n "$out" ] || { echo "stub go: no -o in $*" >&2; exit 1; }
printf 'go %s %s %s\n' "${GOOS:-}" "${GOARCH:-}" "${!#}" >> "$GO_CALL_LOG"
printf 'fake %s/%s\n' "${GOOS:-}" "${GOARCH:-}" > "$out"
STUB
chmod +x "$stub/go"
export GO_CALL_LOG="$work/go-calls.log"
export PATH="$stub:$PATH"

# --- run ---------------------------------------------------------------------
cd "$src"
PREVIEW_LABEL='feat/MR #42!' bash .codebase/preview/build.sh preview-package > "$work/manifest-stdout.json"

version="1.2.3-feat-MR--42.g${sha}"

# The label is folded to a semver prerelease identifier rather than rejected:
# it is a human hint, and the commit is what makes the version unique.
manifest="preview-package/manifest.json"
[ -f "$manifest" ] || fail "no manifest at $manifest"
got_version="$(node -p "require('./$manifest').version")"
[ "$got_version" = "$version" ] || fail "version is $got_version, expected $version"
[ "$(node -p "require('./$manifest').name")" = "@byted/larksuite-cli-preview" ] || fail "unexpected package name"
[ "$(node -p "require('./$manifest').commit")" = "$sha" ] || fail "manifest commit is not HEAD"
[ "$(node -p "require('./$manifest').releasePackage")" = "@byted/larksuite-cli" ] || fail "manifest lacks the release package"
diff <(cat "$manifest") <(cat "$work/manifest-stdout.json") > /dev/null || fail "stdout manifest differs from the written one"

# One cross-compile per configured platform, each building the root package so
# the //go:embed of skills/ and affordance/ survives into the binary.
expected_calls="$(
  IFS=','
  for platform in $(sed -n 's/^PREVIEW_PLATFORMS=//p' .codebase/preview/config.env); do
    printf 'go %s %s .\n' "${platform%%/*}" "${platform##*/}"
  done | sort
)"
[ "$(sort "$GO_CALL_LOG")" = "$expected_calls" ] || fail "go calls were:$(printf '\n%s' "$(cat "$GO_CALL_LOG")")"

# --- the tarball a reviewer installs ------------------------------------------
tarball="preview-package/$(node -p "require('./$manifest').filename")"
[ -f "$tarball" ] || fail "no tarball at $tarball"
extract="$work/extract"
mkdir -p "$extract"
tar -xzf "$tarball" -C "$extract"
pkg="$extract/package"

# Never publishable: npm refuses `publish` on a private package, and the
# launcher reads the absent publishConfig as "this build cannot update".
[ "$(node -p "require('$pkg/package.json').private")" = "true" ] || fail "preview package is not private"
[ "$(node -p "require('$pkg/package.json').publishConfig")" = "undefined" ] || fail "preview package names a publish target"
[ "$(node -p "require('$pkg/package.json').larkPreview.label")" = "feat-MR--42" ] || fail "package.json lacks the sanitised label"

# skills/ travels inside: the intranet has no git host `npx skills add` can clone.
[ -f "$pkg/skills/lark-base/SKILL.md" ] || fail "skills are not in the package"
[ -f "$pkg/skills/lark-base/references/read.md" ] || fail "skill references are not in the package"
[ -f "$pkg/scripts/run.js" ] || fail "launcher is not in the package"
[ -f "$pkg/scripts/postinstall.js" ] || fail "postinstall is not in the package"
[ -f "$pkg/bin/lark-cli-darwin-arm64" ] || fail "darwin/arm64 binary is not in the package"
[ -f "$pkg/bin/lark-cli-darwin-amd64" ] || fail "darwin/amd64 binary is not in the package"

# --- an unset label falls back to the branch ---------------------------------
# The merge gate passes no label and relies on this: the branch names the
# merge request without the pipeline knowing the platform's variables.
rm -rf preview-package
: > "$GO_CALL_LOG"
bash .codebase/preview/build.sh preview-package > /dev/null
default_version="$(node -p "require('./$manifest').version")"
[ "$default_version" = "1.2.3-main.g${sha}" ] || fail "default version is $default_version, expected the branch"

# A branch name is folded and capped, and a detached HEAD still yields a
# usable version rather than one labelled "HEAD".
rm -rf preview-package
git -C "$src" checkout -q -b 'feat/a-very-long-branch-name-that-keeps-going'
bash .codebase/preview/build.sh preview-package > /dev/null
long_version="$(node -p "require('./$manifest').version")"
[ "$long_version" = "1.2.3-feat-a-very-long-branch.g${sha}" ] || fail "long branch version is $long_version"

rm -rf preview-package
git -C "$src" checkout -q --detach
bash .codebase/preview/build.sh preview-package > /dev/null
detached_version="$(node -p "require('./$manifest').version")"
[ "$detached_version" = "1.2.3-preview.g${sha}" ] || fail "detached HEAD version is $detached_version"
git -C "$src" checkout -q main

# --- PREVIEW_PLATFORMS from the caller wins over config.env -------------------
# Sourcing config.env would otherwise clobber it, which is silent: the build
# succeeds and quietly carries the wrong platforms.
rm -rf preview-package
: > "$GO_CALL_LOG"
PREVIEW_PLATFORMS=linux/amd64 bash .codebase/preview/build.sh preview-package > /dev/null
[ "$(cat "$GO_CALL_LOG")" = "go linux amd64 ." ] || fail "override was ignored, go calls were:$(printf '\n%s' "$(cat "$GO_CALL_LOG")")"
[ "$(node -p "require('./$manifest').binaries.join(',')")" = "lark-cli-linux-amd64" ] || fail "override did not reach the package"

echo "preview scripts test passed"
