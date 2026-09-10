#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

# Declarative contract of the Preview Package job in the merge gate. The
# builder it calls is exercised by .codebase/preview/build-preview-package.test.js.
#
# The invariant worth a test of its own is the credential one: this job runs
# branch code every time somebody presses its button, so a token reaching it
# would hand every merge request author the ability to publish under the
# release name. Nothing here may reference one.
ruby -ropen3 -ryaml -rjson <<'RUBY'
ci = YAML.load_file(".codebase/pipelines/ci.yaml")
preview_env = File.readlines(".codebase/preview/config.env").grep(/^[A-Z]/).to_h { |line| line.chomp.split("=", 2) }
release_env = File.readlines(".codebase/release/config.env").grep(/^[A-Z]/).to_h { |line| line.chomp.split("=", 2) }
public_package = JSON.parse(File.read("package.json")).fetch("name")

def contract_error(message)
  abort("preview package contract: #{message}")
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

jobs = ci.fetch("jobs")
job = jobs["preview_package"] or contract_error("the merge gate has no preview_package job")

# Every run produces one, so nobody has to know the job exists to receive a
# package. No job in this pipeline is manual.
contract_error("preview_package must not be manual: every run produces a package") if job["manual"]
expect_equal(jobs.select { |_, other| other["manual"] }.keys, [], "manual jobs")

# Automatic means required here, so it gates on fast_gate: a branch that does
# not compile fails there rather than failing twice.
expect_equal(job["depends"], ["fast_gate"], "preview_package dependencies")
expect_equal(job["if"], '${{Jobs.fast_gate.Conclusion == "success"}}', "preview_package gates on fast_gate succeeding")

# Nothing may wait on it: a package is a by-product, never a prerequisite.
jobs.each do |name, other|
  contract_error("#{name} must not depend on preview_package") if Array(other["depends"]).include?("preview_package")
end
expect_equal(job["timeout"], 30, "preview_package timeout")
expect_equal(job.fetch("image"), jobs.fetch("fast_gate").fetch("image"), "preview_package uses the merge gate image")
expect_equal(job.fetch("steps").first, { "uses" => "actions/checkout" }, "preview_package checks out first")

# Node comes from the same internal mirror every other job uses.
node_install = jobs.fetch("unit_test").fetch("steps").find { |step| step["name"] == "Install Node" }.fetch("commands")
step = job.fetch("steps").find { |candidate| candidate["name"] == "Install Node" }
contract_error("preview_package must install Node") unless step
expect_equal(step.fetch("commands").first, node_install.first, "preview_package Node tarball source")

job.fetch("steps").each do |candidate|
  Array(candidate["commands"]).each do |command|
    _stdout, stderr, status = Open3.capture3("bash", "-n", stdin_data: command)
    contract_error("#{candidate["name"]} has invalid bash syntax: #{stderr}") unless status.success?
  end
end

# No credentials, and no way to publish. `private: true` on the package the
# builder writes is the second lock; this is the first.
scalar_values(job).grep(String).each do |value|
  %w[TOKEN SECRET PASSWORD CREDENTIAL BNPM_ APP_ID].each do |forbidden|
    contract_error("preview_package must not reference #{forbidden}: #{value[0, 80]}") if value.include?(forbidden)
  end
  ["npm publish", "npm dist-tag", "--provenance"].each do |forbidden|
    contract_error("preview_package must not run #{forbidden.inspect}: #{value[0, 80]}") if value.include?(forbidden)
  end
end
contract_error("preview_package must declare no envs") if job.key?("envs")

# Everything it runs lives under .codebase/, like the release pipeline.
scalar_values(job).grep(String).each do |value|
  value.scan(%r{(?<![\w./])(?:scripts|internal|cmd)/\S+}).each do |ref|
    contract_error("preview_package must not reference #{ref} outside .codebase/") unless ref.start_with?(".codebase/")
  end
end

commands = scalar_values(job.fetch("steps")).grep(String).join("\n")
%w[.codebase/preview/build.sh].each do |script|
  contract_error("preview_package must call #{script}") unless commands.include?(script)
  contract_error("#{script} is missing") unless File.file?(script)
  _stdout, stderr, status = Open3.capture3("bash", "-n", script)
  contract_error("#{script} has invalid bash syntax: #{stderr}") unless status.success?
end
contract_error(".codebase/preview/build-preview-package.js is missing") unless File.file?(".codebase/preview/build-preview-package.js")

# The tarball is the hand-off. Until it is hosted behind a URL, this artifact
# is the only way anyone gets the build.
upload = job.fetch("steps").find { |candidate| candidate["uses"] == "actions/upload-artifact" }
expect_equal(upload&.fetch("inputs"), { "name" => "preview-package", "path" => "preview-package" }, "preview package upload")

# The preview identity must stay distinct from both published packages: a
# shared name would give `npm install -g` a path to a merge request build.
expect_equal(preview_env.keys, %w[PREVIEW_PACKAGE PREVIEW_PLATFORMS], "preview config.env keys")
preview_package = preview_env.fetch("PREVIEW_PACKAGE")
contract_error("PREVIEW_PACKAGE must be a scoped npm name") unless preview_package =~ %r{\A@[a-z0-9._-]+/[a-z0-9._-]+\z}
contract_error("PREVIEW_PACKAGE must differ from the public package") if preview_package == public_package
contract_error("PREVIEW_PACKAGE must differ from RELEASE_PACKAGE") if preview_package == release_env["RELEASE_PACKAGE"]

platforms = preview_env.fetch("PREVIEW_PLATFORMS").split(",")
contract_error("PREVIEW_PLATFORMS must list at least one platform") if platforms.empty?
platforms.each do |platform|
  contract_error("PREVIEW_PLATFORMS entry #{platform.inspect} must be <goos>/<goarch>") unless platform =~ %r{\A(darwin|linux|windows)/(amd64|arm64|riscv64)\z}
end
contract_error("PREVIEW_PLATFORMS must not repeat a platform") unless platforms.uniq == platforms

# The embedded skill content is the reason the preview build is worth having:
# an agent under test reads it through `lark-cli skills`. `go build ./main.go`
# compiles the root package's //go:embed away.
build = File.readlines(".codebase/preview/build.sh").reject { |line| line.strip.start_with?("#") }.join
go_build = build.lines.grep(/go build/)
contract_error("build.sh must run `go build`") if go_build.empty?
go_build.each do |line|
  contract_error("build.sh must build the root package, not ./main.go, so skills stay embedded") if line.include?("main.go")
  contract_error("build.sh must pass the root package to go build: #{line.strip}") unless line.strip.end_with?(" .")
end

puts "preview package contract passed"
RUBY
