#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

# Keep the release pipeline's trust boundaries visible in a fast, dependency-free
# test. This deliberately inspects the workflow as text: GitHub Actions has no
# stable local schema validator for all of the expression and inline-script
# constructs used here.
set -euo pipefail

workflow=".github/workflows/release.yml"
goreleaser=".goreleaser.yml"

fail() {
  echo "release workflow contract: $*" >&2
  exit 1
}

require() {
  local needle="$1"
  local haystack="$2"
  local message="$3"
  grep -Fq -- "$needle" <<<"$haystack" || fail "$message (missing: $needle)"
}

job_section() {
  local job="$1"
  awk -v job="$job" '
    $0 == "  " job ":" { in_job = 1; print; next }
    in_job && /^  [A-Za-z0-9_-]+:$/ { exit }
    in_job { print }
  ' "$workflow"
}

[[ -f "$workflow" ]] || fail "missing $workflow"
[[ -f "$goreleaser" ]] || fail "missing $goreleaser"

mapfile -t jobs < <(awk '
  /^jobs:$/ { in_jobs = 1; next }
  in_jobs && /^  [A-Za-z0-9_-]+:$/ {
    name = $0
    sub(/^  /, "", name)
    sub(/:$/, "", name)
    print name
  }
' "$workflow")
expected_jobs=(preflight build-sign-notarize create-draft-release publish-github publish-npm verify-macos)
if [[ "${jobs[*]}" != "${expected_jobs[*]}" ]]; then
  fail "expected exactly the six release jobs: ${expected_jobs[*]}; got: ${jobs[*]:-(none)}"
fi

preflight_section="$(job_section preflight)"
build_section="$(job_section build-sign-notarize)"
draft_section="$(job_section create-draft-release)"
github_section="$(job_section publish-github)"
npm_section="$(job_section publish-npm)"
macos_section="$(job_section verify-macos)"

[[ -n "$preflight_section" && -n "$build_section" && -n "$draft_section" && -n "$github_section" && -n "$npm_section" && -n "$macos_section" ]] || fail "every release job must have a nonempty section"
if grep -Eq '^[[:space:]]*needs:' <<<"$preflight_section"; then
  fail "preflight must start the release dependency graph"
fi
for requirement in 'needs: preflight'; do require "$requirement" "$build_section" "build-sign-notarize must follow preflight"; done
for requirement in '      - preflight' '      - build-sign-notarize'; do require "$requirement" "$draft_section" "create-draft-release must wait for the signed candidate"; done
for requirement in '      - preflight' '      - create-draft-release'; do require "$requirement" "$macos_section" "verify-macos must verify the draft Release"; done
for requirement in '      - preflight' '      - build-sign-notarize' '      - create-draft-release' '      - verify-macos'; do require "$requirement" "$github_section" "publish-github must wait for every verification gate"; done
for requirement in '      - preflight' '      - build-sign-notarize' '      - publish-github'; do require "$requirement" "$npm_section" "publish-npm must happen after GitHub publication"; done

concurrency_section="$(awk '
  /^concurrency:$/ { in_concurrency = 1; print; next }
  in_concurrency && /^[^[:space:]]/ { exit }
  in_concurrency { print }
' "$workflow")"
require 'group: release-${{ github.ref_name }}' "$concurrency_section" "release concurrency must be per tag"
require 'cancel-in-progress: false' "$concurrency_section" "release tags must not cancel an active publication"
if grep -Eiq 'channel|lark-cli-release|release-[[:space:]]*$' <<<"$concurrency_section"; then
  fail "release concurrency must not use a channel-wide lock"
fi

# Every third-party action must be immutable: action tags can be retargeted.
awk '
  /^[[:space:]]*[-]?[[:space:]]*uses:[[:space:]]*/ {
    action = $0
    sub(/^.*uses:[[:space:]]*/, "", action)
    sub(/[[:space:]]*(#.*)?$/, "", action)
    split(action, parts, "@")
    if (length(parts) != 2 || length(parts[2]) != 40 || parts[2] !~ /^[0-9a-f]{40}$/) {
      printf "un-pinned action: %s\\n", action > "/dev/stderr"
      bad = 1
    }
    count++
  }
  END { if (count == 0 || bad) exit 1 }
' "$workflow" || fail "all release actions must be pinned to 40-hex commit SHAs"

require 'version: 2' "$(head -n 1 "$goreleaser")" "GoReleaser config must use v2 schema"
require 'version: v2.17.1' "$build_section" "build must use the approved GoReleaser version"
require 'args: release --clean --skip=publish' "$build_section" "GoReleaser must build only; publication is separately gated"
for requirement in 'notarize:' 'enabled:' 'MACOS_SIGN_P12' 'MACOS_NOTARY_KEY_PATH'; do require "$requirement" "$(<"$goreleaser")" "GoReleaser must retain macOS signing/notarization configuration"; done
for requirement in '      - darwin' '      - linux' '      - windows' '      - amd64' '      - arm64' '      - riscv64' 'formats: [tar.gz]' 'formats: [zip]'; do require "$requirement" "$(<"$goreleaser")" "GoReleaser must produce the supported release archive matrix"; done

require 'contents: read' "$build_section" "the signing build job must remain read-only"
if grep -Eq '^[[:space:]]*(contents|actions|packages|id-token):[[:space:]]*write' <<<"$build_section"; then
  fail "the signing build job must not receive write permissions"
fi
for secret in MACOS_NOTARY_KEY MACOS_SIGN_P12 MACOS_SIGN_PASSWORD; do
  require "secrets.${secret}" "$build_section" "build-sign-notarize must receive ${secret}"
  for job in preflight create-draft-release publish-github publish-npm verify-macos; do
    if grep -Fq "secrets.${secret}" <<<"$(job_section "$job")"; then
      fail "${secret} must be available only to build-sign-notarize"
    fi
  done
done
for requirement in 'umask 077' 'mktemp "${RUNNER_TEMP}/macos-notary-key.XXXXXX"' 'chmod 0600 "$notary_key"' 'trap cleanup EXIT'; do require "$requirement" "$build_section" "Apple key preparation must securely handle the temporary key"; done
require 'name: Clean up Apple notarization key' "$build_section" "Apple key cleanup must always run"
require 'if: ${{ always() }}' "$build_section" "Apple key cleanup must run after failures"
require 'rm -f -- "$MACOS_NOTARY_KEY_PATH"' "$build_section" "Apple key cleanup must remove the temporary key"

require 'uses: actions/upload-artifact@' "$build_section" "the signed release candidate must be uploaded as an artifact"
require 'if-no-files-found: error' "$build_section" "candidate upload must fail closed"
for section_name in draft_section github_section npm_section; do
  section="${!section_name}"
  require 'uses: actions/download-artifact@' "$section" "each release gate must download the exact candidate artifact"
  require 'digest-mismatch: error' "$section" "candidate artifact digests must fail closed"
done
require 'draft: true' "$draft_section" "the candidate must first be created as a draft GitHub Release"
require 'matrix:' "$macos_section" "macOS verification must cover both supported architectures"
for requirement in 'runner: macos-15-intel' 'arch: amd64' 'runner: macos-15' 'arch: arm64' 'codesign --verify --strict --verbose=4' 'spctl --assess --type execute --verbose=4' 'source=Notarized Developer ID'; do require "$requirement" "$macos_section" "macOS verification must retain signing and Gatekeeper checks"; done

require 'Install the candidate through the public Release' "$github_section" "GitHub publication must be followed by a public candidate install"
require 'npm install --global --prefix' "$github_section" "candidate install must exercise the packed npm artifact"
require 'id-token: write' "$npm_section" "npm trusted publishing requires GitHub OIDC"
require 'path.resolve("release-candidate", manifest.npmPackage.name)' "$npm_section" "npm publish must use the original verified candidate tarball"
require 'npm", ["publish", tgz, "--access", "public", "--provenance", "--tag", before.distTag]' "$npm_section" "npm publish must use provenance and the evaluated dist-tag"
for requirement in '"dist-tags"' 'evaluateNpmState' 'afterState.distTags?.[before.distTag] !== env.VERSION'; do require "$requirement" "$npm_section" "npm publication must validate the target dist-tag"; done

checkout_jobs=0
while IFS= read -r job; do
  section="$(job_section "$job")"
  if grep -Fq 'actions/checkout@' <<<"$section"; then
    checkout_jobs=$((checkout_jobs + 1))
    require 'persist-credentials: false' "$section" "checkout in ${job} must not persist credentials"
  fi
done < <(printf '%s\n' "${jobs[@]}")
(( checkout_jobs > 0 )) || fail "release workflow must explicitly check out source where needed"

echo "release workflow contract passed"
