#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

# This verifies the release workflow's declarative contract. The shell commands
# inside individual steps are exercised by the beta release rehearsal instead.
ruby -ryaml <<'RUBY'
workflow = YAML.load_file(".github/workflows/release.yml")
goreleaser = YAML.load_file(".goreleaser.yml")

def fail(message)
  abort("release workflow contract: #{message}")
end

def expect_equal(actual, expected, description)
  return if actual == expected
  fail("#{description}; expected #{expected.inspect}, got #{actual.inspect}")
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
  "verify-macos" => %w[preflight create-draft-release],
  "publish-github" => %w[preflight create-draft-release verify-macos],
  "publish-npm" => %w[preflight build-sign-notarize publish-github],
  "retry-guidance" => %w[preflight build-sign-notarize create-draft-release verify-macos publish-github publish-npm],
}
expected_needs.each do |job_name, needs|
  expect_equal(jobs.fetch(job_name)["needs"], needs, "#{job_name} dependencies")
end

expected_permissions = {
  "preflight" => { "contents" => "read" },
  "build-sign-notarize" => { "contents" => "read" },
  "create-draft-release" => { "contents" => "write" },
  "verify-macos" => { "contents" => "write" },
  "publish-github" => { "contents" => "write" },
  "publish-npm" => { "contents" => "read", "id-token" => "write" },
  "retry-guidance" => { "contents" => "read" },
}
expected_permissions.each do |job_name, permissions|
  expect_equal(jobs.fetch(job_name)["permissions"], permissions, "#{job_name} permissions")
end
expect_equal(jobs.fetch("publish-npm").fetch("environment"), "npm-production", "npm publish environment")

retry_guidance = jobs.fetch("retry-guidance")
retry_condition = "${{ always() && (needs.preflight.result == 'failure' || needs.build-sign-notarize.result == 'failure' || needs.create-draft-release.result == 'failure' || needs.verify-macos.result == 'failure' || needs.publish-github.result == 'failure' || needs.publish-npm.result == 'failure') }}"
expect_equal(retry_guidance.fetch("if"), retry_condition, "retry guidance failure condition")
expect_equal(retry_guidance.fetch("runs-on"), "ubuntu-22.04", "retry guidance runner")

retry_steps = retry_guidance.fetch("steps")
expect_equal(retry_steps.length, 1, "number of retry guidance steps")
retry_step = retry_steps.first
expect_equal(retry_step.fetch("name"), "Write retry guidance", "retry guidance step name")
fail("retry guidance must write to the GitHub step summary") unless retry_step.fetch("run").include?("GITHUB_STEP_SUMMARY")

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

macos = jobs.fetch("verify-macos")
expect_equal(macos.fetch("strategy").fetch("matrix").fetch("include"), [
  { "runner" => "macos-15-intel", "arch" => "amd64" },
  { "runner" => "macos-15", "arch" => "arm64" },
], "macOS verification matrix")
expect_equal(macos.fetch("runs-on"), "${{ matrix.runner }}", "macOS matrix runner")

npm_steps = jobs.fetch("publish-npm").fetch("steps")
pinned_npm = npm_steps.find { |step| step["name"] == "Install pinned npm" }
fail("publish-npm must install npm 11.16.0 for trusted publishing") unless pinned_npm&.fetch("run", nil) == "npm install --global npm@11.16.0"

action_references(workflow).each do |reference|
  fail("action is not pinned to a full commit SHA: #{reference}") unless reference.match?(%r{\A[^@]+@[0-9a-f]{40}\z})
end

notarize = goreleaser.fetch("notarize").fetch("macos")
expect_equal(notarize.length, 1, "number of macOS notarization configurations")
macos_notarize = notarize.first
expect_equal(macos_notarize.fetch("ids"), ["lark-cli"], "notarized build IDs")
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
