#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

# Contract test for .codebase/pipelines/ci.yaml: every job must check out the
# repository and then merge the merge request's target branch, in that order
# and before anything else. A job that skips the merge silently verifies the
# source branch alone, which is how main broke on 2026-09-14 with every check
# green. Adding a job without the step is the easy mistake; this catches it.
set -euo pipefail

ruby -ryaml <<'RUBY'
ci = YAML.load_file(".codebase/pipelines/ci.yaml")
merge_command = "bash .codebase/ci/merge-target.sh"

failures = []
ci.fetch("jobs").each do |name, job|
  steps = job.fetch("steps")
  unless steps[0].is_a?(Hash) && steps[0]["uses"] == "actions/checkout"
    failures << "#{name}: the first step must be actions/checkout"
    next
  end
  commands = steps[1].is_a?(Hash) ? Array(steps[1]["commands"]) : []
  unless commands.include?(merge_command)
    failures << "#{name}: the step after actions/checkout must run `#{merge_command}`"
  end
end

# Which trigger fired is a property of the event. Deriving it from how HEAD
# compares with a ref that moves under the run raced with the next merge and
# broke a trunk run on 2026-09-14. Assert the positive: a step that picks a
# baseline has to read the event to do it. A negated check on one spelling of
# the old comparison would miss every equivalent way of writing it.
ci.fetch("jobs").each do |name, job|
  job.fetch("steps").each do |step|
    commands = Array(step["commands"]).map(&:to_s)
    next unless commands.any? { |c| c.include?("QUALITY_GATE_CHANGED_FROM=") }
    next if commands.any? { |c| c.include?("CI_EVENT_NAME") }
    failures << "#{name}: the step that picks a baseline must read CI_EVENT_NAME, not infer the trigger from repository state"
  end
end

abort("ci pipeline contract:\n  " + failures.join("\n  ")) unless failures.empty?
puts "ci pipeline contract: #{ci.fetch("jobs").size} jobs verify the merge result"
RUBY
