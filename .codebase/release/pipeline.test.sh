#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

# Declarative contract of the internal release pipeline; the stage scripts it
# calls are exercised by .codebase/release/scripts.test.sh.
ruby -ropen3 -ryaml <<'RUBY'
pipeline = YAML.load_file(".codebase/pipelines/release.yaml")
ci = YAML.load_file(".codebase/pipelines/ci.yaml")
public_release = YAML.load_file(".github/workflows/release.yml")
release_env = File.readlines(".codebase/release/config.env").grep(/^[A-Z]/).to_h { |line| line.chomp.split("=", 2) }

def contract_error(message)
  abort("internal release pipeline contract: #{message}")
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

expect_equal(pipeline.fetch("trigger"), { "push" => { "types" => ["tag"], "tags" => ["v*"] } }, "release trigger")

jobs = pipeline.fetch("jobs")
expect_equal(jobs.keys, %w[preflight build verify_install publish], "release jobs in order")

image = ci.fetch("jobs").fetch("fast_gate").fetch("image")
jobs.each do |job_name, job|
  expect_equal(job.fetch("image"), image, "#{job_name} uses the merge gate image")
  expect_equal(job.fetch("steps").first, { "uses" => "actions/checkout" }, "#{job_name} checks out first")
  job.fetch("steps").each do |step|
    Array(step["commands"]).each do |command|
      _stdout, stderr, status = Open3.capture3("bash", "-n", stdin_data: command)
      contract_error("#{job_name}/#{step["name"]} has invalid bash syntax: #{stderr}") unless status.success?
    end
  end
end

expected_depends = { "preflight" => nil, "build" => ["preflight"], "verify_install" => ["build"], "publish" => ["verify_install"] }
expected_depends.each do |job_name, depends|
  job = jobs.fetch(job_name)
  expect_equal(job["depends"], depends, "#{job_name} dependencies")
  next if depends.nil?
  expect_equal(job["if"], "${{Jobs.#{depends.first}.Conclusion == \"success\"}}", "#{job_name} gates on its dependency succeeding")
end

expected_timeouts = { "preflight" => 15, "build" => 45, "verify_install" => 15, "publish" => 15 }
expected_timeouts.each do |job_name, timeout|
  expect_equal(jobs.fetch(job_name)["timeout"], timeout, "#{job_name} timeout")
end

# Node comes from the same internal mirror the merge gate already relies on.
node_install = ci.fetch("jobs").fetch("unit_test").fetch("steps").find { |step| step["name"] == "Install Node" }.fetch("commands")
jobs.each do |job_name, job|
  step = job.fetch("steps").find { |candidate| candidate["name"] == "Install Node" }
  contract_error("#{job_name} must install Node") unless step
  expect_equal(step.fetch("commands").first, node_install.first, "#{job_name} Node tarball source")
end

# Only the publish job may see the registry token, and it must come from a
# pipeline variable rather than the YAML.
jobs.each do |job_name, job|
  mentions = scalar_values(job).grep(String).any? { |value| value.include?("BNPM_AUTH_TOKEN") }
  expect_equal(mentions, job_name == "publish", "#{job_name} references the publish token")
end
expect_equal(jobs.fetch("publish").fetch("envs"), { "BNPM_AUTH_TOKEN" => "${{ BNPM_AUTH_TOKEN }}" }, "publish token injection")
expect_equal(jobs.fetch("build").fetch("envs"), { "GOPROXY" => "https://goproxy.byted.org" }, "build module proxy")

# Nothing in the pipeline reaches the public release channels.
scalar_values(pipeline).grep(String).each do |value|
  %w[--access\ public --provenance gh\ release npmjs.org github.com].each do |forbidden|
    contract_error("release pipeline must not reference #{forbidden.inspect}: #{value[0, 80]}") if value.include?(forbidden)
  end
end

upload = jobs.fetch("build").fetch("steps").find { |step| step["uses"] == "actions/upload-artifact" }
expect_equal(upload&.fetch("inputs"), { "name" => "release-candidate", "path" => "release-candidate" }, "candidate upload")
%w[verify_install publish].each do |job_name|
  download = jobs.fetch(job_name).fetch("steps").find { |step| step["uses"] == "actions/download-artifact" }
  expect_equal(download&.fetch("inputs"), { "name" => "release-candidate", "path" => "./" }, "#{job_name} candidate download")
end

# Every stage runs through a script that exists and parses.
stage_scripts = {
  "preflight" => %w[.codebase/release/source.sh .codebase/release/registry-version.sh],
  "build" => %w[.codebase/release/build.sh],
  "verify_install" => %w[.codebase/release/verify.sh],
  "publish" => %w[.codebase/release/publish.sh],
}
stage_scripts.each do |job_name, scripts|
  commands = scalar_values(jobs.fetch(job_name).fetch("steps")).grep(String).join("\n")
  scripts.each do |script|
    contract_error("#{job_name} must call #{script}") unless commands.include?(script)
    contract_error("#{script} is missing") unless File.file?(script)
    _stdout, stderr, status = Open3.capture3("bash", "-n", script)
    contract_error("#{script} has invalid bash syntax: #{stderr}") unless status.success?
  end
end
contract_error("publish must only publish through the stage script") if scalar_values(jobs.fetch("publish")).grep(String).any? { |value| value.include?("npm publish") }

# The distribution identity is the internal package on the internal registry,
# and the toolchain pin matches the public release.
expect_equal(release_env.keys, %w[RELEASE_PACKAGE RELEASE_REGISTRY RELEASE_GO_VERSION], "config.env keys")
contract_error("RELEASE_PACKAGE must be a scoped npm name") unless release_env["RELEASE_PACKAGE"] =~ %r{\A@[a-z0-9._-]+/[a-z0-9._-]+\z}
contract_error("RELEASE_PACKAGE must not be the public package") if release_env["RELEASE_PACKAGE"] == "@larksuite/cli"
contract_error("RELEASE_REGISTRY must be an https URL") unless release_env["RELEASE_REGISTRY"] =~ %r{\Ahttps://[^/]+\z}
expect_equal(release_env["RELEASE_GO_VERSION"], public_release.dig("env", "RELEASE_GO_VERSION"), "internal Go toolchain pin follows the public release")

# The pipeline touches nothing outside .codebase/: every path it names lives
# there, and the public scripts it relies on are reached only through the
# stage scripts.
scalar_values(jobs).grep(String).each do |value|
  value.scan(%r{(?<![\w./])(?:scripts|internal|cmd)/\S+}).each do |ref|
    contract_error("release pipeline must not reference #{ref} outside .codebase/") unless ref.start_with?(".codebase/")
  end
end

puts "internal release pipeline contract passed"
RUBY
