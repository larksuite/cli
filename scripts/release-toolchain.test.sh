#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
test_root="$(mktemp -d)"
trap 'rm -rf -- "$test_root"' EXIT

passed=0
failed=0

pass() {
  printf 'PASS  %s\n' "$1"
  passed=$((passed + 1))
}

fail() {
  printf 'FAIL  %s: %s\n' "$1" "$2" >&2
  failed=$((failed + 1))
}

mutate_file() {
  local file=$1
  local needle=$2
  local replacement=$3

  ruby - "$file" "$needle" "$replacement" <<'RUBY'
path, needle, replacement = ARGV
content = File.read(path)
abort("mutation needle not found: #{needle}") unless content.sub!(needle, replacement)
File.write(path, content)
RUBY
}

new_contract_case() {
  local name=$1
  local dir="$test_root/contract-$name"

  mkdir -p "$dir/.github/workflows" "$dir/scripts"
  cp "$repo_root/.github/workflows/release.yml" "$dir/.github/workflows/release.yml"
  cp "$repo_root/.goreleaser.yml" "$dir/.goreleaser.yml"
  cp "$repo_root/scripts/release-workflow.test.sh" "$dir/scripts/release-workflow.test.sh"
  printf '%s\n' "$dir"
}

run_contract_success() {
  local name=$1
  local dir=$2
  local log="$dir/output.log"

  if (cd "$dir" && bash scripts/release-workflow.test.sh) >"$log" 2>&1; then
    pass "contract/$name"
  else
    fail "contract/$name" "unexpected failure; see $log"
  fi
}

run_contract_failure() {
  local name=$1
  local dir=$2
  local expected=$3
  local log="$dir/output.log"

  if (cd "$dir" && bash scripts/release-workflow.test.sh) >"$log" 2>&1; then
    fail "contract/$name" "mutation was not rejected"
  elif grep -Fq "$expected" "$log"; then
    pass "contract/$name rejected"
  else
    fail "contract/$name" "failed for the wrong reason; see $log"
  fi
}

baseline_dir=$(new_contract_case baseline)
run_contract_success baseline "$baseline_dir"

version_dir=$(new_contract_case configured-version)
mutate_file "$version_dir/.github/workflows/release.yml" "RELEASE_GO_VERSION: '1.26.5'" "RELEASE_GO_VERSION: '1.26.4'"
run_contract_failure configured-version "$version_dir" "release Go version"

setup_dir=$(new_contract_case setup-go-input)
mutate_file "$setup_dir/.github/workflows/release.yml" 'go-version: ${{ env.RELEASE_GO_VERSION }}' 'go-version: 1.26.4'
run_contract_failure setup-go-input "$setup_dir" "release Go toolchain input"

expected_dir=$(new_contract_case expected-expression)
mutate_file "$expected_dir/.github/workflows/release.yml" 'expected="go${RELEASE_GO_VERSION}"' 'expected="go1.26.4"'
run_contract_failure expected-expression "$expected_dir" "compare against the configured version"

comparison_dir=$(new_contract_case mismatch-comparison)
mutate_file "$comparison_dir/.github/workflows/release.yml" '[[ "$actual" == "$expected" ]]' '[[ "$actual" == "go1.26.4" ]]'
run_contract_failure mismatch-comparison "$comparison_dir" "reject mismatches"

loop_dir=$(new_contract_case first-binary-only)
mutate_file "$loop_dir/.github/workflows/release.yml" 'for binary in "${release_binaries[@]}"; do' 'for binary in "${release_binaries[0]}"; do'
run_contract_failure first-binary-only "$loop_dir" "check every binary"

metadata_dir=$(new_contract_case bypass-build-metadata)
mutate_file "$metadata_dir/.github/workflows/release.yml" 'actual="$(go version -m "$binary" | awk '\''NR == 1 { print $NF }'\'')"' 'actual="go${RELEASE_GO_VERSION}"'
run_contract_failure bypass-build-metadata "$metadata_dir" "inspect embedded build metadata"

gate="$test_root/release-toolchain-gate.sh"
ruby -ryaml - "$repo_root/.github/workflows/release.yml" "$gate" <<'RUBY'
workflow_path, output_path = ARGV
workflow = YAML.load_file(workflow_path)
step = workflow.fetch("jobs").fetch("build-sign-notarize").fetch("steps").find do |item|
  item["name"] == "Verify release Go toolchain"
end
abort("release toolchain verification step not found") unless step
File.write(output_path, step.fetch("run"))
RUBY
chmod +x "$gate"

fake_bin="$test_root/bin"
mkdir -p "$fake_bin"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  '[[ "$#" -eq 3 && "$1" == version && "$2" == -m ]] || exit 2' \
  'binary=$3' \
  'version="$(head -n 1 "$binary")"' \
  '[[ "$version" == go* ]] || { echo "$binary: not a Go executable" >&2; exit 1; }' \
  'printf "%s: %s\\n" "$binary" "$version"' \
  > "$fake_bin/go"
chmod +x "$fake_bin/go"

write_probe() {
  local output=$1
  local version=$2

  mkdir -p "$(dirname "$output")"
  printf '%s\n' "$version" > "$output"
}

good_dist="$test_root/dist-good"
write_probe "$good_dist/01-darwin-arm64/lark-cli" go1.26.5
write_probe "$good_dist/02-linux-amd64/lark-cli" go1.26.5
write_probe "$good_dist/03-windows-amd64/lark-cli.exe" go1.26.5

stale_dist="$test_root/dist-stale"
write_probe "$stale_dist/01-darwin-arm64/lark-cli" go1.26.2

mixed_dist="$test_root/dist-mixed"
write_probe "$mixed_dist/01-good/lark-cli" go1.26.5
write_probe "$mixed_dist/02-stale/lark-cli" go1.26.2

empty_dist="$test_root/dist-empty"
mkdir -p "$empty_dist"

non_go_dist="$test_root/dist-non-go"
write_probe "$non_go_dist/01-invalid/lark-cli" "not-go-build-metadata"

run_gate_success() {
  local name=$1
  local expected_version=$2
  local source_dist=$3
  local dir="$test_root/runtime-$name"
  local log="$dir/output.log"

  mkdir -p "$dir/dist"
  cp -R "$source_dist/." "$dir/dist/"
  if (cd "$dir" && PATH="$fake_bin:$PATH" RELEASE_GO_VERSION="$expected_version" bash "$gate") >"$log" 2>&1; then
    pass "runtime/$name"
  else
    fail "runtime/$name" "unexpected failure; see $log"
  fi
}

run_gate_failure() {
  local name=$1
  local expected_version=$2
  local source_dist=$3
  local expected_fragment=$4
  local dir="$test_root/runtime-$name"
  local log="$dir/output.log"

  mkdir -p "$dir/dist"
  cp -R "$source_dist/." "$dir/dist/"
  if (cd "$dir" && PATH="$fake_bin:$PATH" RELEASE_GO_VERSION="$expected_version" bash "$gate") >"$log" 2>&1; then
    fail "runtime/$name" "negative case unexpectedly passed"
  elif grep -Fq "$expected_fragment" "$log"; then
    pass "runtime/$name rejected"
  else
    fail "runtime/$name" "failed for the wrong reason; see $log"
  fi
}

run_gate_success all-release-binaries 1.26.5 "$good_dist"
run_gate_failure expected-patch-downgrade 1.26.4 "$good_dist" "expected go1.26.4"
run_gate_failure expected-stale-toolchain 1.26.2 "$good_dist" "expected go1.26.2"
run_gate_failure expected-future-toolchain 1.27.0 "$good_dist" "expected go1.27.0"
run_gate_failure mixed-toolchains 1.26.5 "$mixed_dist" "built with go1.26.2; expected go1.26.5"
run_gate_failure all-stale-binaries 1.26.5 "$stale_dist" "built with go1.26.2; expected go1.26.5"
run_gate_failure empty-binary-set 1.26.5 "$empty_dist" "GoReleaser produced no binaries to verify."
run_gate_failure non-go-binary 1.26.5 "$non_go_dist" "not a Go executable"

printf '\nrelease toolchain negative tests: passed=%d failed=%d\n' "$passed" "$failed"
(( failed == 0 ))
