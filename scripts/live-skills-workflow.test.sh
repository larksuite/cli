#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

workflow=.github/workflows/live-skills.yml

if [[ ! -f "$workflow" ]]; then
  echo "live skills workflow is missing" >&2
  exit 1
fi

# Parse the workflow as real YAML so a structurally invalid file (for example a
# run: block scalar broken by an un-indented continuation line) fails here
# instead of silently failing to load at GitHub Actions runtime. The grep
# assertions below check content but cannot catch YAML structural errors. Ruby
# ships a YAML parser on both ubuntu-latest CI and dev machines.
ruby -ryaml -e "YAML.load_file(ARGV[0])" "$workflow" >/dev/null

grep -Eq '^[[:space:]]+workflow_dispatch:[[:space:]]*$' "$workflow"
grep -Eq '^[[:space:]]+schedule:[[:space:]]*$' "$workflow"
if grep -Eq '^[[:space:]]*(push|pull_request|pull_request_target|merge_group):|^[[:space:]]*on:[[:space:]]*\[[^]]*(push|pull_request|pull_request_target|merge_group)' "$workflow"; then
  echo "live skills workflow must only run on schedule or workflow_dispatch" >&2
  exit 1
fi

grep -Fq "permissions:" "$workflow"
grep -Fq "contents: read" "$workflow"
grep -Fq "persist-credentials: false" "$workflow"
grep -Fq "timeout-minutes: 15" "$workflow"
grep -Fq "runs-on: ubuntu-latest" "$workflow"
grep -Fq "node-version: '22'" "$workflow"
grep -Fq "make live-skills-test" "$workflow"

if grep -Fq '${{ secrets.' "$workflow"; then
  echo "live skills workflow must not reference secrets" >&2
  exit 1
fi
uses_lines=$(grep -E '^[[:space:]]*-[[:space:]]+uses:' "$workflow" || true)
if [[ -z "$uses_lines" ]]; then
  echo "live skills workflow must use pinned setup actions" >&2
  exit 1
fi
if grep -Ev 'uses:[[:space:]]+[^@[:space:]]+@[0-9a-f]{40}([[:space:]]+#.*)?$' <<<"$uses_lines" >/dev/null; then
  echo "workflow actions must be pinned to commit SHAs" >&2
  exit 1
fi
