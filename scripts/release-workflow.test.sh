#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

# This verifies the release workflow's declarative contract. The shell commands
# inside individual steps are exercised by the beta release rehearsal instead.
ruby -ropen3 -ryaml <<'RUBY'
workflow = YAML.load_file(".github/workflows/release.yml")
goreleaser = YAML.load_file(".goreleaser.yml")

def contract_error(message)
  abort("release workflow contract: #{message}")
end

def expect_equal(actual, expected, description)
  return if actual == expected
  contract_error("#{description}; expected #{expected.inspect}, got #{actual.inspect}")
end

def scalar_values(value)
  case value
  when Hash then value.values.flat_map { |item| scalar_values(item) }
  when Array then value.flat_map { |item| scalar_values(item) }
  else [value]
  end
end

def action_references(value)
  case value
  when Hash
    value.flat_map { |key, item| key == "uses" ? [item] : action_references(item) }
  when Array
    value.flat_map { |item| action_references(item) }
  else
    []
  end
end

jobs = workflow.fetch("jobs")
jobs.each do |job_name, job|
  job.fetch("steps", []).each do |step|
    run = step["run"]
    next unless run.is_a?(String)

    _stdout, stderr, status = Open3.capture3("bash", "-n", stdin_data: run)
    contract_error("#{job_name}/#{step["name"]} has invalid bash syntax: #{stderr}") unless status.success?
  end
end

expect_equal(workflow.dig("env", "RELEASE_GO_VERSION"), "1.26.5", "release Go version")

expected_jobs = %w[preflight build-sign-notarize create-draft-release verify-macos publish-github publish-npm retry-guidance]
expect_equal(jobs.keys.sort, expected_jobs.sort, "release jobs")

expect_equal(workflow.fetch("concurrency"), {
  "group" => "release-${{ github.ref_name }}",
  "cancel-in-progress" => false,
}, "release concurrency")

expected_needs = {
  "preflight" => nil,
  "build-sign-notarize" => "preflight",
  "create-draft-release" => %w[preflight build-sign-notarize],
  "verify-macos" => %w[preflight build-sign-notarize create-draft-release],
  "publish-github" => %w[preflight create-draft-release verify-macos],
  "publish-npm" => %w[preflight build-sign-notarize publish-github],
  "retry-guidance" => %w[preflight build-sign-notarize create-draft-release verify-macos publish-github publish-npm],
}
expected_needs.each do |job_name, needs|
  expect_equal(jobs.fetch(job_name)["needs"], needs, "#{job_name} dependencies")
end

expected_permissions = {
  "preflight" => { "contents" => "read" },
  "build-sign-notarize" => { "contents" => "read", "id-token" => "write", "attestations" => "write" },
  "create-draft-release" => { "contents" => "write" },
  "verify-macos" => { "contents" => "read" },
  "publish-github" => { "contents" => "write" },
  "publish-npm" => { "contents" => "read", "id-token" => "write" },
  "retry-guidance" => { "contents" => "read" },
}
expected_permissions.each do |job_name, permissions|
  expect_equal(jobs.fetch(job_name)["permissions"], permissions, "#{job_name} permissions")
end

expected_timeouts = {
  "build-sign-notarize" => 45,
  "create-draft-release" => 15,
  "verify-macos" => 20,
  "publish-github" => 15,
  "publish-npm" => 15,
}
expected_timeouts.each do |job_name, timeout|
  expect_equal(jobs.fetch(job_name)["timeout-minutes"], timeout, "#{job_name} timeout")
end

expect_equal(jobs.fetch("build-sign-notarize").fetch("environment"), "npm-production", "signing approval environment")
contract_error("publish-npm must not request a second Environment approval") if jobs.fetch("publish-npm").key?("environment")
expect_equal(jobs.fetch("publish-npm").fetch("concurrency"), {
  "group" => "npm-release-${{ needs.preflight.outputs.channel }}",
  "queue" => "max",
  "cancel-in-progress" => false,
}, "npm publication concurrency")

retry_guidance = jobs.fetch("retry-guidance")
retry_condition = "${{ always() && (needs.preflight.result == 'failure' || needs.build-sign-notarize.result == 'failure' || needs.create-draft-release.result == 'failure' || needs.verify-macos.result == 'failure' || needs.publish-github.result == 'failure' || needs.publish-npm.result == 'failure') }}"
expect_equal(retry_guidance.fetch("if"), retry_condition, "retry guidance failure condition")
expect_equal(retry_guidance.fetch("runs-on"), "ubuntu-22.04", "retry guidance runner")

retry_steps = retry_guidance.fetch("steps")
expect_equal(retry_steps.length, 1, "number of retry guidance steps")
retry_step = retry_steps.first
expect_equal(retry_step.fetch("name"), "Write retry guidance", "retry guidance step name")
contract_error("retry guidance must write to the GitHub step summary") unless retry_step.fetch("run").include?("GITHUB_STEP_SUMMARY")
contract_error("retry guidance must direct recoveries to failed-job retries") unless retry_step.fetch("run").include?("Re-run failed jobs")
contract_error("retry guidance must explain Draft cleanup before a rebuild") unless retry_step.fetch("run").include?("delete the Draft, then retry build")
contract_error("retry guidance must explain public Release cleanup after npm policy rejection") unless retry_step.fetch("run").include?("delete the public GitHub Release")

signing_references = %w[
  secrets.MACOS_SIGN_P12
  secrets.MACOS_SIGN_PASSWORD
  secrets.MACOS_NOTARY_KEY
  vars.MACOS_NOTARY_KEY_ID
  vars.MACOS_NOTARY_ISSUER_ID
]
team_reference = "vars.MACOS_TEAM_ID"
jobs.each do |job_name, job|
  references = scalar_values(job).grep(String).flat_map do |value|
    (signing_references + [team_reference]).select { |reference| value.include?(reference) }
  end.uniq.sort
  expected_references = case job_name
                        when "build-sign-notarize" then signing_references + [team_reference]
                        when "verify-macos" then [team_reference]
                        else []
                        end
  expect_equal(
    references,
    expected_references.sort,
    "#{job_name} Apple credential scope",
  )
end

build_steps = jobs.fetch("build-sign-notarize").fetch("steps")
setup_go = build_steps.find { |step| step["uses"]&.start_with?("actions/setup-go@") }
expect_equal(setup_go&.dig("with", "go-version"), "${{ env.RELEASE_GO_VERSION }}", "release Go toolchain input")
fetch_metadata_index = build_steps.index { |step| step["name"] == "Fetch build metadata" }
prepare_key_index = build_steps.index { |step| step["name"] == "Prepare Apple notarization key" }
contract_error("build metadata must be fetched before Apple credentials are prepared") unless fetch_metadata_index && prepare_key_index && fetch_metadata_index < prepare_key_index
contract_error("build metadata must be fetched outside GoReleaser hooks") if goreleaser.dig("before", "hooks")&.include?("python3 scripts/fetch_meta.py")

goreleaser_index = build_steps.index { |step| step["name"] == "Run GoReleaser" }
toolchain_verify_index = build_steps.index { |step| step["name"] == "Verify release Go toolchain" }
candidate_index = build_steps.index { |step| step["name"] == "Build release candidate" }
unless goreleaser_index && toolchain_verify_index && candidate_index && goreleaser_index < toolchain_verify_index && toolchain_verify_index < candidate_index
  contract_error("release Go toolchain must be verified after GoReleaser and before candidate packaging")
end
toolchain_verify_run = build_steps.fetch(toolchain_verify_index).fetch("run")
contract_error("release toolchain verification must reject an empty binary set") unless toolchain_verify_run.include?("${#release_binaries[@]} > 0")
contract_error("release toolchain verification must inspect embedded build metadata") unless toolchain_verify_run.include?('go version -m "$binary"')
contract_error("release toolchain verification must compare against the configured version") unless toolchain_verify_run.include?('expected="go${RELEASE_GO_VERSION}"')
contract_error("release toolchain verification must check every binary") unless toolchain_verify_run.include?('for binary in "${release_binaries[@]}"; do')
contract_error("release toolchain verification must reject mismatches") unless toolchain_verify_run.include?('[[ "$actual" == "$expected" ]]')

goreleaser_step = build_steps.find { |step| step["uses"].to_s.start_with?("goreleaser/goreleaser-action@") }
contract_error("the workflow must own Release publication instead of GoReleaser") unless goreleaser_step&.dig("with", "args")&.include?("--skip=publish")
contract_error("GoReleaser must not configure Release publication") if goreleaser.key?("release")

candidate_order = [
  "Build release candidate",
  "Verify release edition identities",
  "Verify release platform asset matrix",
  "Attest release archives",
  "Upload release candidate",
]
candidate_indexes = candidate_order.map do |name|
  expect_equal(build_steps.count { |step| step["name"] == name }, 1, "number of '#{name}' steps")
  build_steps.index { |step| step["name"] == name }
end
contract_error("the release candidate must be verified and attested before it leaves the build job") unless candidate_indexes == candidate_indexes.sort

candidate_run = build_steps.fetch(candidate_indexes.first).fetch("run")
contract_error("the release candidate must stage the Extended installers beside the archives") unless candidate_run.include?("cp scripts/install-extended.sh scripts/install-extended.ps1 dist/")
contract_error("the release candidate must carry the Extended installers") unless candidate_run.include?("cp dist/install-extended.sh dist/install-extended.ps1 release-candidate/")

identity_run = build_steps.find { |step| step["name"] == "Verify release edition identities" }.fetch("run")
contract_error("edition identity must be read from the compiled binary") unless identity_run.include?("version --json")
%w[standard extended].each do |edition|
  contract_error("edition identity verification must assert the #{edition} edition") unless identity_run.include?(edition)
end

platform_run = build_steps.find { |step| step["name"] == "Verify release platform asset matrix" }.fetch("run")
contract_error("platform coverage must be verified by the release asset verifier") unless platform_run.include?("scripts/verify-release-assets.sh dist")

attest_step = build_steps.find { |step| step["name"] == "Attest release archives" }
expect_equal(
  attest_step.fetch("uses"),
  "actions/attest-build-provenance@e8998f949152b193b063cb0ec769d69d929409be",
  "release provenance action",
)
expect_equal(
  attest_step.fetch("with").fetch("subject-path").split("\n").map(&:strip).reject(&:empty?),
  %w[dist/*.tar.gz dist/*.zip dist/checksums.txt dist/install-extended.sh dist/install-extended.ps1],
  "release provenance subjects",
)

macos = jobs.fetch("verify-macos")
expect_equal(macos.fetch("strategy").fetch("matrix").fetch("include"), [
  { "runner" => "macos-15-intel", "arch" => "amd64" },
  { "runner" => "macos-15", "arch" => "arm64" },
], "macOS verification matrix")
expect_equal(macos.fetch("runs-on"), "${{ matrix.runner }}", "macOS matrix runner")
macos_verify_step = macos.fetch("steps").find { |step| step["name"] == "Verify notarized macOS binaries" }
macos_verify_run = macos_verify_step&.fetch("run", nil)
macos_download_step = macos.fetch("steps").find { |step| step["name"] == "Download release candidate" }
contract_error("verify-macos must download the build candidate artifact") unless macos_download_step&.fetch("uses", nil)&.start_with?("actions/download-artifact@")
contract_error("verify-macos must not download mutable Draft Release assets") if macos_verify_run&.include?("gh release download")
contract_error("verify-macos must verify notarization through codesign") unless macos_verify_run&.include?("--check-notarization -R='notarized'")
contract_error("verify-macos must check Developer ID authority") unless macos_verify_run&.include?("^Authority=Developer ID Application: .+")
contract_error("verify-macos must check the expected Team ID") unless macos_verify_run&.include?("TeamIdentifier=${MACOS_TEAM_ID}")
contract_error("verify-macos must detect hardened runtime in CodeDirectory metadata") unless macos_verify_run&.include?("^CodeDirectory .*flags=0x")
contract_error("verify-macos must check the signing timestamp") unless macos_verify_run&.include?("^Timestamp=.+")
contract_error("verify-macos must match the complete release version") unless macos_verify_run&.include?("escaped_version")
contract_error("verify-macos must verify every distributed macOS edition") unless macos_verify_run&.include?('"lark-cli-${VERSION}-darwin-${ARCH}.tar.gz" "lark-cli-extended-${VERSION}-darwin-${ARCH}.tar.gz"')

draft_step = jobs.fetch("create-draft-release").fetch("steps").find { |step| step["name"] == "Create or reuse Draft Release" }
draft_run = draft_step&.fetch("run", nil)
contract_error("Draft Release creation must write generated release notes") unless draft_run&.include?("--notes-file")
contract_error("Draft Release reuse must validate target commit and prerelease state") unless draft_run&.include?("targetCommitish") && draft_run.include?("isPrerelease")
contract_error("Draft Release creation must target the validated source commit") unless draft_run&.include?("--target \"$SOURCE_SHA\"")
contract_error("Draft Release creation must require the existing remote tag") unless draft_run&.include?("--verify-tag")
%w[install-extended.sh install-extended.ps1].each do |installer|
  contract_error("the Draft Release must carry #{installer}") unless draft_run&.include?("release-candidate/#{installer}")
end

github_steps = jobs.fetch("publish-github").fetch("steps")
github_check = github_steps.find { |step| step["name"] == "Verify Draft assets match the candidate" }
contract_error("GitHub publication must verify Draft assets against the candidate") unless github_check&.fetch("run", nil)&.include?("release-candidate/checksums.txt")
%w[install-extended.sh install-extended.ps1].each do |installer|
  contract_error("GitHub publication must reconcile #{installer}") unless github_check&.fetch("run", nil)&.include?("-name #{installer}")
end
github_npm_guard = github_steps.find { |step| step["name"] == "Refuse GitHub publication if npm channel is newer" }
contract_error("GitHub publication must refuse a version behind the npm channel") unless github_npm_guard&.fetch("run", nil)&.include?("compareReleaseVersions")
contract_error("GitHub publication must query the matching npm dist-tag") unless github_npm_guard&.fetch("run", nil)&.include?("dist-tags.${dist_tag}")

npm_steps = jobs.fetch("publish-npm").fetch("steps")
pinned_npm = npm_steps.find { |step| step["name"] == "Install pinned npm" }
contract_error("publish-npm must install npm 11.16.0 for trusted publishing") unless pinned_npm&.fetch("run", nil) == "npm install --global npm@11.16.0"
publish_step = npm_steps.find { |step| step["name"] == "Publish or verify npm package" }
contract_error("publish-npm must explicitly pass the candidate tarball as a local path") unless publish_step&.fetch("run", nil).include?('npm publish "./$tgz"')
contract_error("publish-npm must publish provenance under the selected channel tag") unless publish_step&.fetch("run", nil).include?('--provenance --tag "$dist_tag"')
contract_error("beta releases must publish under the beta dist-tag") unless publish_step&.fetch("run", nil).include?('[[ "$CHANNEL" != "beta" ]] || dist_tag=beta')

action_references(workflow).each do |reference|
  contract_error("action is not pinned to a full commit SHA: #{reference}") unless reference.match?(%r{\A[^@]+@[0-9a-f]{40}\z})
end

builds = goreleaser.fetch("builds")
expect_equal(builds.map { |build| build.fetch("id") }, %w[standard extended], "GoReleaser build IDs")
expect_equal(builds.map { |build| build.fetch("binary") }, %w[lark-cli lark-cli], "GoReleaser binary names")
expect_equal(builds.fetch(0)["tags"], nil, "Standard build tags")
expect_equal(builds.fetch(1).fetch("tags"), ["extended"], "Extended build tags")
builds.each do |build|
  contract_error("GoReleaser must build a darwin artifact for the #{build.fetch("id")} edition") unless build.fetch("goos").include?("darwin")
end

archives = goreleaser.fetch("archives")
expect_equal(archives.map { |archive| archive.fetch("id") }, %w[standard extended], "release archive IDs")
expect_equal(archives.map { |archive| archive.fetch("ids") }, [%w[standard], %w[extended]], "release archive build bindings")
expect_equal(archives.fetch(0).fetch("name_template"), "lark-cli-{{ .Version }}-{{ .Os }}-{{ .Arch }}", "Standard archive name")
expect_equal(archives.fetch(1).fetch("name_template"), "lark-cli-extended-{{ .Version }}-{{ .Os }}-{{ .Arch }}", "Extended archive name")

expect_equal(
  goreleaser.fetch("checksum").fetch("extra_files").map { |entry| entry.fetch("glob") },
  ["./scripts/install-extended.sh", "./scripts/install-extended.ps1"],
  "checksummed extra release files",
)

notarize = goreleaser.fetch("notarize").fetch("macos")
expect_equal(notarize.length, 1, "number of macOS notarization configurations")
macos_notarize = notarize.first
expect_equal(macos_notarize.fetch("enabled"), '{{ isEnvSet "MACOS_SIGN_P12" }}', "macOS notarization enablement")
expect_equal(macos_notarize.fetch("ids"), %w[standard extended], "notarized build IDs")
expect_equal(macos_notarize.fetch("sign"), {
  "certificate" => "{{ .Env.MACOS_SIGN_P12 }}",
  "password" => "{{ .Env.MACOS_SIGN_PASSWORD }}",
}, "macOS signing inputs")
expect_equal(macos_notarize.fetch("notarize"), {
  "issuer_id" => "{{ .Env.MACOS_NOTARY_ISSUER_ID }}",
  "key_id" => "{{ .Env.MACOS_NOTARY_KEY_ID }}",
  "key" => "{{ .Env.MACOS_NOTARY_KEY_PATH }}",
  "wait" => true,
  "timeout" => "20m",
}, "macOS notarization inputs")

puts "release workflow contract passed"
RUBY

# The asset verifier gates publication, so exercise it against a synthetic dist
# tree rather than trusting its presence in the workflow alone.
verifier="scripts/verify-release-assets.sh"
bash -n "$verifier"

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/lark-cli-release-contract.XXXXXX")"
trap 'rm -rf "$tmp_dir"' EXIT
unix_payload="$tmp_dir/unix"
windows_payload="$tmp_dir/windows"
mkdir -p "$unix_payload" "$windows_payload"

for payload in "$unix_payload" "$windows_payload"; do
  printf 'changelog\n' >"$payload/CHANGELOG.md"
  printf 'license\n' >"$payload/LICENSE"
  printf 'readme\n' >"$payload/README.md"
done
printf '#!/bin/sh\nexit 0\n' >"$unix_payload/lark-cli"
printf 'synthetic windows binary\n' >"$windows_payload/lark-cli.exe"

platforms=(
  darwin-amd64.tar.gz
  darwin-arm64.tar.gz
  linux-amd64.tar.gz
  linux-arm64.tar.gz
  linux-riscv64.tar.gz
  windows-amd64.zip
  windows-arm64.zip
)

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
    return
  fi
  shasum -a 256 "$1" | awk '{print $1}'
}

synthesize_dist() {
  local dist_dir="$1" version="$2" platform prefix archive asset_path
  mkdir -p "$dist_dir"
  for platform in "${platforms[@]}"; do
    for prefix in lark-cli lark-cli-extended; do
      archive="$dist_dir/${prefix}-${version}-${platform}"
      if [[ "$platform" == *.zip ]]; then
        (
          cd "$windows_payload"
          zip -q "$archive" CHANGELOG.md LICENSE README.md lark-cli.exe
        )
      else
        tar -czf "$archive" -C "$unix_payload" CHANGELOG.md LICENSE README.md lark-cli
      fi
    done
  done
  printf '#!/bin/sh\n' >"$dist_dir/install-extended.sh"
  printf 'Write-Host "install"\n' >"$dist_dir/install-extended.ps1"
  for asset_path in "$dist_dir"/lark-cli-*.tar.gz "$dist_dir"/lark-cli-*.zip \
    "$dist_dir/install-extended.sh" "$dist_dir/install-extended.ps1"; do
    printf '%s  %s\n' "$(sha256_file "$asset_path")" "$(basename "$asset_path")"
  done | LC_ALL=C sort -k2 >"$dist_dir/checksums.txt"
}

# Stable and beta tags both reach this verifier.
for version in 1.2.3 1.2.3-beta.4; do
  dist_dir="$tmp_dir/dist-$version"
  synthesize_dist "$dist_dir" "$version"
  bash "$verifier" "$dist_dir" "$version" >/dev/null
  printf 'tamper\n' >>"$dist_dir/lark-cli-${version}-linux-amd64.tar.gz"
  if bash "$verifier" "$dist_dir" "$version" >/dev/null 2>&1; then
    echo "release asset verifier must reject a checksum mismatch" >&2
    exit 1
  fi
done

if bash "$verifier" "$tmp_dir/dist-1.2.3" 1.2 >/dev/null 2>&1; then
  echo "release asset verifier must reject a malformed release version" >&2
  exit 1
fi

echo "release asset verifier contract passed"
