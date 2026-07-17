#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

workflow=".github/workflows/ci.yml"
job_section() {
  local job="$1"
  awk -v job="$job" '
    $0 == "  " job ":" { in_job = 1; print; next }
    in_job && /^  [A-Za-z0-9_-]+:/ { exit }
    in_job { print }
  ' "$workflow"
}
workflow_permissions="$(awk '
  /^permissions:/ { in_permissions = 1; print; next }
  in_permissions && /^[^[:space:]]/ { exit }
  in_permissions { print }
' "$workflow")"
fast_gate_section="$(job_section fast-gate)"
unit_test_section="$(job_section unit-test)"
lint_section="$(awk '
  /^  lint:/ { in_job = 1 }
  in_job { print }
  /^  script-test:/ { exit }
' "$workflow")"
script_test_section="$(job_section script-test)"
deterministic_section="$(awk '
  /^  deterministic-gate:/ { in_job = 1 }
  in_job { print }
  /^  coverage:/ { exit }
' "$workflow")"
coverage_job_section="$(job_section coverage)"
deadcode_section="$(job_section deadcode)"
dry_run_section="$(job_section e2e-dry-run)"
section="$(awk '
  /^  e2e-live:/ { in_job = 1 }
  in_job { print }
  /^  security:/ { exit }
' "$workflow")"
security_section="$(job_section security)"
license_header_section="$(job_section license-header)"
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

if ! grep -Fq "Write public content metadata" <<<"$deterministic_section"; then
  echo "deterministic-gate should write PR title/body metadata before quality-gate"
  exit 1
fi

if ! grep -Fq "types: [opened, synchronize, reopened, edited]" "$workflow"; then
  echo "CI pull_request trigger should include edited so PR title/body changes are rescanned"
  exit 1
fi

if ! grep -Fq "script-test:" <<<"$script_test_section"; then
  echo "CI should run make script-test so workflow and publisher contract tests are not local-only"
  exit 1
fi

if ! grep -Fq "make script-test" <<<"$script_test_section"; then
  echo "script-test job should invoke make script-test"
  exit 1
fi

if ! grep -Fq "actions/setup-node" <<<"$script_test_section"; then
  echo "script-test job should install Node for JavaScript workflow tests"
  exit 1
fi

if grep -Fq '${{ secrets.' <<<"$script_test_section"; then
  echo "script-test must not reference secrets"
  exit 1
fi

if grep -Fq "metadata-gate:" "$workflow"; then
  echo "metadata-gate should not run alongside deterministic-gate because both would upload the same facts artifact"
  exit 1
fi

if grep -Fq "github.event.action != 'edited'" <<<"$fast_gate_section"; then
  echo "fast-gate must run on pull_request edited events so title/body edits cannot replace failed CI with a light success"
  exit 1
fi

for full_job in \
  "$unit_test_section" \
  "$lint_section" \
  "$script_test_section" \
  "$deterministic_section" \
  "$coverage_job_section" \
  "$dry_run_section" \
  "$security_section"; do
  if grep -Fq "github.event.action != 'edited'" <<<"$full_job"; then
    echo "full CI jobs must run on pull_request edited events; do not skip title/body-only edits"
    exit 1
  fi
done

for pull_request_job in "$deadcode_section" "$license_header_section"; do
  if grep -Fq "github.event.action != 'edited'" <<<"$pull_request_job"; then
    echo "pull_request-only CI jobs must run on edited events"
    exit 1
  fi
done

if grep -Fq '${{ secrets.' <<<"$deterministic_section"; then
  echo "deterministic-gate must not reference secrets"
  exit 1
fi

if ! grep -Fq "PUBLIC_CONTENT_METADATA=" <<<"$deterministic_section"; then
  echo "deterministic-gate should pass public content metadata into make quality-gate"
  exit 1
fi

if ! grep -Fq "PR_BRANCH:" <<<"$deterministic_section"; then
  echo "deterministic-gate should pass the pull request branch into public content metadata"
  exit 1
fi

if ! grep -Fq "name: quality-gate-facts-\${{ github.event.pull_request.base.sha }}-\${{ github.event.pull_request.head.sha }}" <<<"$deterministic_section"; then
  echo "deterministic-gate should upload base/head-bound quality-gate-facts for semantic review"
  exit 1
fi

if ! grep -Fq "needs: [unit-test, lint, script-test, deterministic-gate]" "$workflow"; then
  echo "E2E jobs should wait for script-test and deterministic-gate"
  exit 1
fi

if ! grep -Fq "script-test" <<<"$results_section"; then
  echo "results job should include script-test"
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

if ! grep -Fq "name: Resolve CLI E2E domains" <<<"$dry_run_section" ||
   ! grep -Fq "id: e2e_domains" <<<"$dry_run_section" ||
   ! grep -Fq "run: node scripts/e2e_domains.js" <<<"$dry_run_section"; then
  echo "e2e-dry-run should resolve changed-file CLI E2E domains before running tests"
  exit 1
fi

if ! grep -Fq "steps.e2e_domains.outputs.dry_packages" <<<"$dry_run_section"; then
  echo "e2e-dry-run should use resolved dry_packages instead of always running the full suite"
  exit 1
fi

if ! grep -Fq "E2E_REASON: \${{ steps.e2e_domains.outputs.reason }}" <<<"$dry_run_section" ||
   ! grep -Fq 'echo "Dry-run CLI E2E domains: $E2E_MODE ($E2E_REASON)"' <<<"$dry_run_section"; then
  echo "e2e-dry-run should pass dynamic domain output through env before shell use"
  exit 1
fi

if ! grep -Fq "E2E_DRY_ROOT_PACKAGE: \${{ steps.e2e_domains.outputs.dry_root_package }}" <<<"$dry_run_section" ||
   ! grep -Fq 'go test -v -count=1 -timeout=5m "$E2E_DRY_ROOT_PACKAGE"' <<<"$dry_run_section"; then
  echo "e2e-dry-run should run the root CLI E2E harness package without the DryRun/Regression filter"
  exit 1
fi

if ! grep -Fq "No dry-run CLI E2E needed" <<<"$dry_run_section"; then
  echo "e2e-dry-run should explicitly skip when domain mode is skip"
  exit 1
fi

if ! grep -Fq "name: Resolve CLI E2E domains" <<<"$section" ||
   ! grep -Fq "id: e2e_domains" <<<"$section" ||
   ! grep -Fq "run: node scripts/e2e_domains.js" <<<"$section"; then
  echo "e2e-live should resolve changed-file CLI E2E domains before credentials and tests"
  exit 1
fi

if ! grep -Fq "steps.e2e_domains.outputs.live_packages" <<<"$section"; then
  echo "e2e-live should use resolved live_packages instead of always running the full suite"
  exit 1
fi

if ! grep -Fq "E2E_REASON: \${{ steps.e2e_domains.outputs.reason }}" <<<"$section" ||
   ! grep -Fq 'echo "Live CLI E2E domains: $E2E_MODE ($E2E_REASON)"' <<<"$section"; then
  echo "e2e-live should pass dynamic domain output through env before shell use"
  exit 1
fi

if ! awk '
  /^      - name: Build lark-cli/ { in_step = 1 }
  in_step && /if: \$\{\{ steps\.e2e_domains\.outputs\.mode != '\''skip'\'' \}\}/ { found = 1 }
  in_step && /^      - name:/ && !/Build lark-cli/ { in_step = 0 }
  END { exit found ? 0 : 1 }
' <<<"$dry_run_section"; then
  echo "e2e-dry-run should skip building lark-cli when domain mode is skip"
  exit 1
fi

if ! awk '
  /^      - name: Build lark-cli/ { in_step = 1 }
  in_step && /if: \$\{\{ steps\.e2e_domains\.outputs\.mode != '\''skip'\'' \}\}/ { found = 1 }
  in_step && /^      - name:/ && !/Build lark-cli/ { in_step = 0 }
  END { exit found ? 0 : 1 }
' <<<"$section"; then
  echo "e2e-live should skip building lark-cli when domain mode is skip"
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

if ! grep -Fq "node scripts/fetch_e2e_tat.js" <<<"$section"; then
  echo "e2e-live should fetch the tenant token via the dedicated script"
  exit 1
fi

if grep -Fq "config init" <<<"$section"; then
  echo "e2e-live should use env credentials instead of config init"
  exit 1
fi

if ! grep -Fq "TEST_BOT1_APP_ID: \${{ secrets.TEST_BOT1_APP_ID }}" <<<"$section"; then
  echo "e2e-live should keep the bot app id under a test-only job env name"
  exit 1
fi

if awk '
  /^  e2e-live:/ { in_job = 1; next }
  in_job && /^  [A-Za-z0-9_-]+:/ { in_job = 0 }
  in_job && /^    env:/ { in_env = 1; next }
  in_env && /^    steps:/ { in_env = 0 }
  in_env && /LARKSUITE_CLI_APP_ID:/ { found_standard_app_id = 1 }
  END { exit found_standard_app_id ? 0 : 1 }
' "$workflow"; then
  echo "e2e-live should not activate the env credential provider at job scope"
  exit 1
fi

if ! grep -Fq "LARKSUITE_CLI_BRAND: feishu" <<<"$section"; then
  echo "e2e-live should pin the env credential brand to feishu"
  exit 1
fi

if awk '
  /^  e2e-live:/ { in_job = 1; next }
  in_job && /^  [A-Za-z0-9_-]+:/ { in_job = 0 }
  in_job && /^    env:/ { in_env = 1; next }
  in_env && /^    steps:/ { in_env = 0 }
  in_env && /(SECRET|ACCESS_TOKEN):/ { found_sensitive = 1 }
  END { exit found_sensitive ? 0 : 1 }
' "$workflow"; then
  echo "e2e-live should not expose live E2E credentials through job-level env"
  exit 1
fi

if ! awk '
  /^      - name: Prepare shared live E2E tenant token/ { in_step = 1 }
  in_step && /id: live_e2e_tat/ { has_id = 1 }
  in_step && /if: \$\{\{ steps\.e2e_domains\.outputs\.mode != '\''skip'\'' \}\}/ { has_if = 1 }
  in_step && /LARKSUITE_CLI_APP_ID: \$\{\{ secrets\.TEST_BOT1_APP_ID \}\}/ { has_app_id = 1 }
  in_step && /secrets\.TEST_BOT1_APP_SECRET/ { has_bot_credential = 1 }
  in_step && /node scripts\/fetch_e2e_tat\.js/ { has_script = 1 }
  in_step && /GITHUB_ENV/ { uses_github_env = 1 }
  in_step && /^      - name:/ && !/Prepare shared live E2E tenant token/ { in_step = 0 }
  END { exit has_id && has_if && has_app_id && has_bot_credential && has_script && !uses_github_env ? 0 : 1 }
' <<<"$section"; then
  echo "e2e-live should pass only a private tenant token file path through step output"
  exit 1
fi

if ! awk '
  /^      - name: Run CLI E2E tests/ { in_step = 1 }
  in_step && /E2E_TENANT_AUTH_FILE: \$\{\{ steps\.live_e2e_tat\.outputs\.path \}\}/ { has_file = 1 }
  in_step && /secrets\.TEST_USER_ACCESS_TOKEN/ { has_user_credential = 1 }
  in_step && /Missing shared live E2E tenant token file/ { checks_file = 1 }
  in_step && /^ *export / && /TEST_TENANT_ACCESS_TOKEN/ && /E2E_TENANT_AUTH_FILE/ { exports_test_tat = 1 }
  in_step && /^ *export / && /LARKSUITE_CLI_TENANT_ACCESS_TOKEN/ { exports_standard_tat = 1 }
  in_step && /LARKSUITE_CLI_APP_ID="\$TEST_BOT1_APP_ID"/ { scopes_preflight_app_id = 1 }
  in_step && /LARKSUITE_CLI_TENANT_ACCESS_TOKEN="\$TEST_TENANT_ACCESS_TOKEN"/ { scopes_preflight_tat = 1 }
  in_step && /lark-cli whoami --as bot/ { has_preflight = 1 }
  in_step && /Tenant credential preflight failed/ { checks_preflight = 1 }
  in_step && /TEST_USER_ACCESS_TOKEN/ && /secrets\.TEST_USER_ACCESS_TOKEN/ { has_user_env = 1 }
  in_step && /LARKSUITE_CLI_USER_ACCESS_TOKEN/ && /secrets\.TEST_USER_ACCESS_TOKEN/ { has_global_user_env = 1 }
  in_step && /trap / { has_trap = 1 }
  in_step && /^      - name:/ && !/Run CLI E2E tests/ { in_step = 0 }
  END { exit has_file && has_user_credential && checks_file && exports_test_tat && !exports_standard_tat && scopes_preflight_app_id && scopes_preflight_tat && has_preflight && checks_preflight && has_user_env && !has_global_user_env && !has_trap ? 0 : 1 }
' <<<"$section"; then
  echo "e2e-live should expose live E2E credentials only inside the test shell step"
  exit 1
fi

if grep -Fq "steps.live_e2e_credentials.outputs.configured" <<<"$section"; then
  echo "e2e-live build, configure, test, and report steps should not be gated by a skip-state output"
  exit 1
fi

if ! grep -Fq "if: \${{ !cancelled() && steps.e2e_domains.outputs.mode != 'skip' }}" <<<"$section"; then
  echo "e2e-live report step should run after attempted live tests unless the workflow is cancelled or domain mode is skip"
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

if ! grep -Fq -- "--public-content-metadata .tmp/quality-gate/public-content-metadata.json" <<<"$make_output"; then
  echo "quality-gate check should consume public content metadata"
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
