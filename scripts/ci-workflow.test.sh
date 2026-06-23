#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

workflow=".github/workflows/ci.yml"
workflow_permissions="$(awk '
  /^permissions:/ { in_permissions = 1; print; next }
  in_permissions && /^[^[:space:]]/ { exit }
  in_permissions { print }
' "$workflow")"
lint_section="$(awk '
  /^  lint:/ { in_job = 1 }
  in_job { print }
  /^  deterministic-gate:/ { exit }
' "$workflow")"
deterministic_section="$(awk '
  /^  deterministic-gate:/ { in_job = 1 }
  in_job { print }
  /^  coverage:/ { exit }
' "$workflow")"
section="$(awk '
  /^  e2e-live:/ { in_job = 1 }
  in_job { print }
  /^  security:/ { exit }
' "$workflow")"
results_section="$(awk '
  /^  results:/ { in_job = 1 }
  in_job { print }
' "$workflow")"
fork_safe_guard="github.event_name != 'pull_request' || !github.event.pull_request.head.repo.fork"

for denied_permission in "checks: write" "pull-requests: write" "issues: write"; do
  if grep -Eq "^[[:space:]]*${denied_permission}$" <<<"$workflow_permissions"; then
    echo "CI workflow must not grant ${denied_permission} at the workflow level" >&2
    exit 1
  fi
done

if ! grep -Fq "contents: read" <<<"$workflow_permissions" || ! grep -Fq "actions: read" <<<"$workflow_permissions"; then
  echo "CI workflow should keep only read permissions at the workflow level"
  exit 1
fi

if ! grep -Fq "deterministic-gate:" <<<"$deterministic_section"; then
  echo "CI should expose deterministic-gate as a standalone job"
  exit 1
fi

if grep -Fq "make quality-gate" <<<"$lint_section"; then
  echo "lint job should not run deterministic quality gate"
  exit 1
fi

if ! grep -Fq "needs: fast-gate" <<<"$deterministic_section"; then
  echo "deterministic-gate should depend on fast-gate"
  exit 1
fi

if ! grep -Fq "permissions:" <<<"$deterministic_section"; then
  echo "deterministic-gate should define job-level permissions"
  exit 1
fi

if ! grep -Fq "contents: read" <<<"$deterministic_section"; then
  echo "deterministic-gate should only need read access to repository contents"
  exit 1
fi

if ! grep -Fq "actions: read" <<<"$deterministic_section"; then
  echo "deterministic-gate should keep actions access read-only"
  exit 1
fi

if grep -Fq "checks: write" <<<"$deterministic_section"; then
  echo "deterministic-gate should not inherit check write permission"
  exit 1
fi

if grep -Fq "pull-requests: write" <<<"$deterministic_section"; then
  echo "deterministic-gate should not inherit pull request write permission"
  exit 1
fi

if grep -Fq '${{ secrets.' <<<"$deterministic_section"; then
  echo "deterministic-gate must not reference secrets"
  exit 1
fi

if ! grep -Fq "Run CLI deterministic gate" <<<"$deterministic_section"; then
  echo "deterministic-gate should run the CLI deterministic gate step"
  exit 1
fi

if ! grep -Fq "make quality-gate" <<<"$deterministic_section"; then
  echo "deterministic-gate should invoke make quality-gate"
  exit 1
fi

if ! grep -Fq "name: quality-gate-facts-\${{ github.event.pull_request.base.sha }}-\${{ github.event.pull_request.head.sha }}" <<<"$deterministic_section"; then
  echo "deterministic-gate should upload base/head-bound quality-gate-facts for semantic review"
  exit 1
fi

if ! grep -Fq "needs: [unit-test, lint, deterministic-gate]" "$workflow"; then
  echo "E2E jobs should wait for deterministic-gate"
  exit 1
fi

if ! grep -Fq "deterministic-gate" <<<"$results_section"; then
  echo "results job should include deterministic-gate"
  exit 1
fi

if ! grep -Fq "if: \${{ $fork_safe_guard }}" <<<"$section"; then
  echo "e2e-live should run on push and same-repository pull_request, but skip fork pull_request"
  exit 1
fi

if ! grep -Fq "permissions:" <<<"$section" ||
   ! grep -Fq "contents: read" <<<"$section" ||
   ! grep -Fq "checks: write" <<<"$section"; then
  echo "e2e-live should grant only the job-level permissions needed to publish test reports"
  exit 1
fi

if grep -Fq "pull-requests: write" <<<"$section" || grep -Fq "issues: write" <<<"$section"; then
  echo "e2e-live should not grant pull request or issue write permission"
  exit 1
fi

if grep -Fq "live_e2e_credentials" <<<"$section" || grep -Fq "configured=false" <<<"$section"; then
  echo "e2e-live should fail, not silently skip, when required credentials are unavailable on eligible runs"
  exit 1
fi

if ! grep -Fq "::error::Missing required secrets: TEST_BOT1_APP_ID / TEST_BOT1_APP_SECRET" <<<"$section"; then
  echo "e2e-live should make missing bot credentials a visible configuration failure on eligible runs"
  exit 1
fi

if grep -Fq "steps.live_e2e_credentials.outputs.configured" <<<"$section"; then
  echo "e2e-live build, configure, test, and report steps should not be gated by a skip-state output"
  exit 1
fi

if ! grep -Fq "if: \${{ !cancelled() }}" <<<"$section"; then
  echo "e2e-live report step should run after attempted live tests unless the workflow is cancelled"
  exit 1
fi

if grep -Fq "continue-on-error: true" <<<"$section"; then
  echo "e2e-live report publishing should use explicit checks write permission instead of hiding publish failures"
  exit 1
fi

coverage_step="$(awk '
  /^      - name: Upload coverage to Codecov/ { in_step = 1 }
  in_step { print }
  in_step && /^      - name: Check coverage threshold/ { exit }
' "$workflow")"

if grep -Fq '${{ secrets.CODECOV_TOKEN }}' <<<"$coverage_step" &&
   ! grep -Fq "if: \${{ $fork_safe_guard }}" <<<"$coverage_step"; then
  echo "Codecov token should be available on push and same-repository pull_request, but not fork pull_request" >&2
  exit 1
fi

if grep -Fq '${{ secrets.' <<<"$section" &&
   ! grep -Fq "if: \${{ $fork_safe_guard }}" <<<"$section"; then
  echo "live E2E secrets should be available on push and same-repository pull_request, but not fork pull_request" >&2
  exit 1
fi

if ! awk -v guard="$fork_safe_guard" '
  /^  [A-Za-z0-9_-]+:/ {
    job_if = "";
    step_if = "";
  }
  /^    if:/ {
    job_if = $0;
  }
  /^      - (name|uses):/ {
    step_if = "";
  }
  /^        if:/ {
    step_if = $0;
  }
  /\$\{\{ secrets\./ {
    if (index(job_if, guard) || index(step_if, guard)) {
      next;
    }
    printf("secret reference at %s:%d must be guarded away from pull_request runs\n", FILENAME, FNR) > "/dev/stderr";
    bad = 1;
  }
  END { exit bad ? 1 : 0 }
' "$workflow"; then
  exit 1
fi

make_output="$(QUALITY_GATE_CHANGED_FROM= make -n quality-gate)"
if grep -Fq -- "--changed-from  \\" <<<"$make_output"; then
  echo "quality-gate should resolve an empty QUALITY_GATE_CHANGED_FROM before passing --changed-from"
  exit 1
fi

if ! grep -Fq "go run ./internal/qualitygate/cmd/manifest-export" <<<"$make_output"; then
  echo "quality-gate should generate command manifests through manifest-export"
  exit 1
fi

if ! grep -Fq -- "--manifest .tmp/quality-gate/command-manifest.json" <<<"$make_output" ||
   ! grep -Fq -- "--command-index .tmp/quality-gate/command-index.json" <<<"$make_output"; then
  echo "quality-gate check should consume both exported command snapshots"
  exit 1
fi

if ! awk '
  function finish_upload() {
    if (!in_upload) {
      return;
    }
    uploads++;
    if (path != ".tmp/quality-gate/facts.json") {
      printf("deterministic-gate upload-artifact path must be .tmp/quality-gate/facts.json, got %s\n", path) > "/dev/stderr";
      bad = 1;
    }
    in_upload = 0;
    path = "";
  }
  /^      - (name|uses):/ {
    finish_upload();
  }
  /uses: actions\/upload-artifact@/ {
    in_upload = 1;
  }
  in_upload && /^[[:space:]]*path:/ {
    path = $0;
    sub(/^[[:space:]]*path:[[:space:]]*/, "", path);
  }
  END {
    finish_upload();
    if (uploads == 0) {
      print "deterministic-gate should upload quality gate facts" > "/dev/stderr";
      bad = 1;
    }
    exit bad ? 1 : 0;
  }
' <<<"$deterministic_section"; then
  exit 1
fi
