#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

# Behavioural tests for the internal release stage scripts. The source policy
# runs against a real git repository with a bare "origin"; the registry
# interactions run against a stub `npm` on PATH that records every call, so the
# publish decision is asserted on what would actually be sent to the registry.
set -euo pipefail

repo="$(git rev-parse --show-toplevel)"
work="$(mktemp -d)"
trap 'rm -rf -- "$work"' EXIT

fail() { echo "internal release scripts: $*" >&2; exit 1; }

# --- fixture: a repository with the release scripts, a bare origin, and main ---
git init -q --bare "$work/origin.git"
git init -q -b main "$work/src"
cd "$work/src"
git config user.email release-test@example.com
git config user.name "release test"
git config commit.gpgsign false
mkdir -p scripts .codebase/release
cp "$repo"/scripts/release-preflight.js "$repo"/scripts/release-publish-policy.js scripts/
cp "$repo"/.codebase/release/source.sh "$repo"/.codebase/release/registry-version.sh "$repo"/.codebase/release/publish.sh .codebase/release/
cp "$repo"/.codebase/release/config.env .codebase/release/config.env
set_version() {
  printf '{"name":"@larksuite/cli","version":"%s"}\n' "$1" > package.json
  printf '{"name":"@larksuite/cli","version":"%s","packages":{"":{"version":"%s"}}}\n' "$1" "$1" > package-lock.json
}
set_version 1.2.3
git add -A && git commit -q -m "release 1.2.3"
git remote add origin "$work/origin.git"
git push -q origin main

source_check() { bash .codebase/release/source.sh "$@"; }

# stable tag on main
git tag v1.2.3 && git push -q origin refs/tags/v1.2.3
out="$(source_check v1.2.3)"
expected="version=1.2.3
channel=stable
source_sha=$(git rev-parse HEAD)"
[[ "$out" == "$expected" ]] || fail "stable tag on main: unexpected output: $out"

# beta tag on main is accepted
set_version 1.2.4-beta.1
git commit -q -am "release 1.2.4-beta.1"
git push -q origin main
git tag v1.2.4-beta.1 && git push -q origin refs/tags/v1.2.4-beta.1
out="$(source_check v1.2.4-beta.1)"
[[ "$out" == *"channel=beta"* && "$out" == *"source_sha=$(git rev-parse HEAD)"* ]] || fail "beta tag on main: unexpected output: $out"

# a tag that does not point at the checkout is refused
if err="$(source_check v1.2.3 2>&1)"; then fail "tag not at HEAD was accepted"; fi
[[ "$err" == *"resolves to"* ]] || fail "tag not at HEAD: unexpected error: $err"

# a tag whose version differs from the package is refused by the preflight
git tag v9.9.9 && git push -q origin refs/tags/v9.9.9
if err="$(source_check v9.9.9 2>&1)"; then fail "version mismatch was accepted"; fi
[[ "$err" == *"release_preflight"* ]] || fail "version mismatch: unexpected error: $err"

# beta from a branch that is not in main is refused
git checkout -q -b feature
set_version 1.2.5-beta.1
git commit -q -am "branch beta"
git tag v1.2.5-beta.1 && git push -q origin refs/tags/v1.2.5-beta.1
if err="$(source_check v1.2.5-beta.1 2>&1)"; then fail "beta tag off main was accepted"; fi
[[ "$err" == *"contained in origin/main"* ]] || fail "beta off main: unexpected error: $err"
git checkout -q main

# --- stub npm ---
mkdir -p "$work/bin"
cat > "$work/bin/npm" <<'STUB'
#!/usr/bin/env bash
# Records every invocation; answers from STUB_* knobs.
printf '%s\n' "$*" >> "$NPM_STUB_LOG"
case "$1 $2" in
  "view "*)
    if [[ "$3" == "version" ]]; then
      if [[ "${STUB_VIEW_ERROR:-}" == 1 ]]; then echo "npm error code E401" >&2; echo "npm error Unable to authenticate" >&2; exit 1; fi
      if [[ "${STUB_EXISTS:-0}" == 1 ]]; then echo '"1.2.4-beta.1"'; exit 0; fi
      echo "npm error code E404" >&2; echo "npm error 404 No match found for version" >&2; exit 1
    fi
    if [[ "$3" == "dist.integrity" ]]; then echo "\"${STUB_INTEGRITY}\""; exit 0; fi
    if [[ "$3" == dist-tags.* ]]; then
      if [[ "${STUB_PACKAGE_MISSING:-}" == 1 ]]; then echo "npm error code E404" >&2; echo "npm error 404 Not Found - GET .../$2" >&2; exit 1; fi
      [[ -n "${STUB_CURRENT:-}" ]] && echo "\"${STUB_CURRENT}\""; exit 0
    fi
    echo "stub npm: unexpected view $*" >&2; exit 2
    ;;
  "publish "*|"dist-tag add") exit 0 ;;
  *) echo "stub npm: unexpected $*" >&2; exit 2 ;;
esac
STUB
chmod +x "$work/bin/npm"
export PATH="$work/bin:$PATH"
export NPM_STUB_LOG="$work/npm.log"

registry_version() { bash .codebase/release/registry-version.sh @byted/larksuite-cli 1.2.4-beta.1 https://bnpm.byted.org; }
: > "$NPM_STUB_LOG"
[[ "$(STUB_EXISTS=1 registry_version)" == exists ]] || fail "registry probe: existing version not reported"
[[ "$(STUB_EXISTS=0 registry_version)" == missing ]] || fail "registry probe: E404 not reported as missing"
if STUB_VIEW_ERROR=1 registry_version >/dev/null 2>&1; then fail "registry probe: auth failure was read as an answer"; fi

# --- publish decisions against the beta candidate built from main HEAD ---
# The candidate is the launcher plus two platform packages; every package
# goes through the same policy, platform packages first.
integrity="sha512-candidate"
write_candidate() {
  rm -rf release-candidate && mkdir release-candidate
  printf 'tarball' > release-candidate/byted-larksuite-cli-1.2.4-beta.1.tgz
  printf 'linux' > release-candidate/byted-larksuite-cli-linux-amd64-1.2.4-beta.1.tgz
  printf 'darwin' > release-candidate/byted-larksuite-cli-darwin-arm64-1.2.4-beta.1.tgz
  cat > release-candidate/manifest.json <<JSON
{"name":"@byted/larksuite-cli","version":"1.2.4-beta.1","registry":"https://bnpm.byted.org","filename":"byted-larksuite-cli-1.2.4-beta.1.tgz","integrity":"$integrity","tag":"v1.2.4-beta.1","source_sha":"$1","channel":"beta",
 "platform_packages":[
  {"name":"@byted/larksuite-cli-darwin-arm64","goos":"darwin","goarch":"arm64","filename":"byted-larksuite-cli-darwin-arm64-1.2.4-beta.1.tgz","integrity":"$integrity"},
  {"name":"@byted/larksuite-cli-linux-amd64","goos":"linux","goarch":"amd64","filename":"byted-larksuite-cli-linux-amd64-1.2.4-beta.1.tgz","integrity":"$integrity"}]}
JSON
}
publish() { bash .codebase/release/publish.sh release-candidate; }
head_sha="$(git rev-parse HEAD)"

# missing version, beta channel older -> published as beta, then integrity confirmed
write_candidate "$head_sha"; : > "$NPM_STUB_LOG"
BNPM_AUTH_TOKEN=secret-token STUB_EXISTS=0 STUB_CURRENT=1.2.3 STUB_INTEGRITY="$integrity" publish >/dev/null || fail "publish of a missing beta version failed"
published="$(grep '^publish ' "$NPM_STUB_LOG")"
expected_publish="publish ./release-candidate/byted-larksuite-cli-darwin-arm64-1.2.4-beta.1.tgz --registry https://bnpm.byted.org --tag beta
publish ./release-candidate/byted-larksuite-cli-linux-amd64-1.2.4-beta.1.tgz --registry https://bnpm.byted.org --tag beta
publish ./release-candidate/byted-larksuite-cli-1.2.4-beta.1.tgz --registry https://bnpm.byted.org --tag beta"
[[ "$published" == "$expected_publish" ]] || fail "expected every package published with --tag beta, platform packages first, got: $(cat "$NPM_STUB_LOG")"
grep -q "view @byted/larksuite-cli-linux-amd64 dist-tags.beta" "$NPM_STUB_LOG" || fail "publish did not consult the beta dist-tag of a platform package"
grep -q "view @byted/larksuite-cli dist-tags.beta" "$NPM_STUB_LOG" || fail "publish did not consult the beta dist-tag of the launcher"
grep -q "^dist-tag add" "$NPM_STUB_LOG" && fail "publish must not also move the dist-tag by hand"

# existing identical version, channel behind -> dist-tag advanced, no publish
write_candidate "$head_sha"; : > "$NPM_STUB_LOG"
BNPM_AUTH_TOKEN=secret-token STUB_EXISTS=1 STUB_CURRENT=1.2.3 STUB_INTEGRITY="$integrity" publish >/dev/null || fail "advancing the dist-tag failed"
for pkg in @byted/larksuite-cli-darwin-arm64 @byted/larksuite-cli-linux-amd64 @byted/larksuite-cli; do
  grep -Fxq "dist-tag add ${pkg}@1.2.4-beta.1 beta --registry https://bnpm.byted.org" "$NPM_STUB_LOG" || fail "expected dist-tag add for ${pkg}, got: $(cat "$NPM_STUB_LOG")"
done
grep -q "^publish " "$NPM_STUB_LOG" && fail "an identical existing version must not be published again"

# package not on the registry at all: the dist-tag probe answers 404 -> an empty channel,
# first publication of every package
write_candidate "$head_sha"; : > "$NPM_STUB_LOG"
BNPM_AUTH_TOKEN=secret-token STUB_EXISTS=0 STUB_PACKAGE_MISSING=1 STUB_INTEGRITY="$integrity" publish >/dev/null || fail "first publication of a new package failed"
[[ "$(grep -c '^publish ' "$NPM_STUB_LOG")" == 3 ]] || fail "first publication must publish every package, got: $(cat "$NPM_STUB_LOG")"

# existing version but the dist-tag probe answers 404 -> transient error, refused; reading it
# as an empty channel would let advance-tag move the channel backwards
write_candidate "$head_sha"; : > "$NPM_STUB_LOG"
if err="$(BNPM_AUTH_TOKEN=secret-token STUB_EXISTS=1 STUB_PACKAGE_MISSING=1 STUB_INTEGRITY="$integrity" publish 2>&1)"; then fail "a 404 dist-tag probe for an existing version was accepted"; fi
[[ "$err" == *"Could not determine the beta dist-tag"* ]] || fail "existing version + 404 dist-tag: unexpected error: $err"
grep -Eq "^(publish |dist-tag add)" "$NPM_STUB_LOG" && fail "a 404 dist-tag probe for an existing version must not touch the registry"

# existing version with different bytes -> refused, nothing sent
write_candidate "$head_sha"; : > "$NPM_STUB_LOG"
if BNPM_AUTH_TOKEN=secret-token STUB_EXISTS=1 STUB_CURRENT=1.2.3 STUB_INTEGRITY="sha512-other" publish >/dev/null 2>&1; then fail "different bytes under an existing version were accepted"; fi
grep -Eq "^(publish |dist-tag add)" "$NPM_STUB_LOG" && fail "a refused candidate must not touch the registry"

# channel already ahead -> refused
write_candidate "$head_sha"; : > "$NPM_STUB_LOG"
if BNPM_AUTH_TOKEN=secret-token STUB_EXISTS=0 STUB_CURRENT=1.2.9 STUB_INTEGRITY="$integrity" publish >/dev/null 2>&1; then fail "moving the beta channel backwards was accepted"; fi
grep -Eq "^(publish |dist-tag add)" "$NPM_STUB_LOG" && fail "a channel regression must not touch the registry"

# candidate built from a commit the tag no longer points at -> refused before auth
write_candidate "0000000000000000000000000000000000000000"; : > "$NPM_STUB_LOG"
if BNPM_AUTH_TOKEN=secret-token STUB_EXISTS=0 STUB_CURRENT=1.2.3 publish >/dev/null 2>&1; then fail "a moved tag was accepted"; fi
[[ ! -s "$NPM_STUB_LOG" ]] || fail "a moved tag must be refused before any registry call"

# no token -> refused before any registry call
write_candidate "$head_sha"; : > "$NPM_STUB_LOG"
if env -u BNPM_AUTH_TOKEN STUB_EXISTS=0 STUB_CURRENT=1.2.3 publish >/dev/null 2>&1; then fail "publishing without a token was accepted"; fi
[[ ! -s "$NPM_STUB_LOG" ]] || fail "a missing token must be refused before any registry call"

# a candidate built for another distribution is refused
write_candidate "$head_sha"; : > "$NPM_STUB_LOG"
sed -i.bak 's#"name":"@byted/larksuite-cli"#"name":"@other/lark-cli"#' release-candidate/manifest.json
if BNPM_AUTH_TOKEN=secret-token STUB_EXISTS=0 STUB_CURRENT=1.2.3 publish >/dev/null 2>&1; then fail "a candidate for another package was accepted"; fi
[[ ! -s "$NPM_STUB_LOG" ]] || fail "a foreign candidate must be refused before any registry call"

# a platform package not named after the launcher is refused
write_candidate "$head_sha"; : > "$NPM_STUB_LOG"
sed -i.bak 's#"name":"@byted/larksuite-cli-linux-amd64"#"name":"@byted/other-linux-amd64"#' release-candidate/manifest.json
if err="$(BNPM_AUTH_TOKEN=secret-token STUB_EXISTS=0 STUB_CURRENT=1.2.3 publish 2>&1)"; then fail "a stray platform package was accepted"; fi
[[ "$err" == *"not named after @byted/larksuite-cli"* ]] || fail "stray platform package: unexpected error: $err"
[[ ! -s "$NPM_STUB_LOG" ]] || fail "a stray platform package must be refused before any registry call"

# a platform tarball missing from the candidate is refused
write_candidate "$head_sha"; : > "$NPM_STUB_LOG"
rm release-candidate/byted-larksuite-cli-linux-amd64-1.2.4-beta.1.tgz
if BNPM_AUTH_TOKEN=secret-token STUB_EXISTS=0 STUB_CURRENT=1.2.3 publish >/dev/null 2>&1; then fail "a candidate missing a platform tarball was accepted"; fi
[[ ! -s "$NPM_STUB_LOG" ]] || fail "a missing tarball must be refused before any registry call"

echo "internal release scripts tests passed"
