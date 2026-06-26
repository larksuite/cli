#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

set -euo pipefail

workflow=".github/workflows/semantic-review.yml"

extract_step() {
  local name="$1"
  awk -v name="$name" '
    $0 == "      - name: " name { in_step = 1; print; next }
    in_step && /^      - (name|uses):/ { exit }
    in_step { print }
  ' "$workflow"
}

extract_job() {
  local name="$1"
  awk -v name="$name" '
    $0 == "  " name ":" { in_job = 1; print; next }
    in_job && /^  [A-Za-z0-9_-]+:/ { exit }
    in_job { print }
  ' "$workflow"
}

require_in_step() {
  local step="$1"
  local needle="$2"
  local message="$3"
  if ! awk -v needle="$needle" '
    index($0, needle) && $0 !~ /^[[:space:]]*(#|\/\/)/ { found = 1 }
    END { exit found ? 0 : 1 }
  ' <<<"$step"; then
    echo "$message" >&2
    exit 1
  fi
}

require_unique_step() {
  local name="$1"
  local count
  count="$(grep -Fc "      - name: $name" "$workflow")"
  if [ "$count" -ne 1 ]; then
    echo "semantic-review workflow should contain exactly one step named '$name', got $count" >&2
    exit 1
  fi
}

for unique_step in \
  "Verify summary facts artifact metadata" \
  "Verify and extract summary facts artifact" \
  "Verify semantic facts artifact metadata" \
  "Verify and extract semantic facts artifact"; do
  require_unique_step "$unique_step"
done

verify_step="$(extract_step "Verify workflow run and pull request")"
summary_verify_step="$(extract_step "Verify workflow run and pull request for summary")"
summary_job="$(extract_job "pr-quality-summary")"
summary_artifact_step="$(extract_step "Verify summary facts artifact metadata")"
artifact_step="$(extract_step "Verify semantic facts artifact metadata")"
waiver_step="$(extract_step "Download PR semantic waiver config")"
semantic_step="$(extract_step "Run semantic review")"
precheckout_step="$(extract_step "Publish pre-checkout semantic review failure")"
summary_publish_step="$(extract_step "Publish PR quality summary")"
publish_step="$(extract_step "Publish semantic review")"
summary_extract_facts_step="$(extract_step "Verify and extract summary facts artifact")"
extract_facts_step="$(extract_step "Verify and extract semantic facts artifact")"

workflow_permissions="$(awk '
  /^permissions:/ { in_permissions = 1; print; next }
  in_permissions && /^jobs:/ { exit }
  in_permissions { print }
' "$workflow")"

for denied_permission in "checks: write" "pull-requests: write" "issues: write"; do
  if grep -Fq "$denied_permission" <<<"$workflow_permissions"; then
    echo "semantic-review workflow should not grant write permissions at the workflow level" >&2
    exit 1
  fi
done

if ! grep -q 'pull-requests: write' "$workflow"; then
  echo "semantic-review should request pull request write permission for PR comments" >&2
  exit 1
fi

if ! grep -Fq 'pull-requests: write' <<<"$summary_job"; then
  echo "pr-quality-summary should request pull request write permission for PR summary comments" >&2
  exit 1
fi

if grep -q 'actions/github-script@60a0d83039c74a4aee543508d2ffcb1c3799cdea' "$workflow"; then
  echo "semantic-review should not use the Node.js 20 github-script action" >&2
  exit 1
fi

if ! grep -q 'actions/github-script@ed597411d8f924073f98dfc5c65a23a2325f34cd' "$workflow"; then
  echo "semantic-review should pin github-script v8" >&2
  exit 1
fi

if ! awk '
  function finish_checkout() {
    if (!in_checkout) {
      return;
    }
    checkouts++;
    if (step !~ /ref: \$\{\{ steps\.pr\.outputs\.base_sha \}\}/) {
      printf("semantic-review trusted checkout must use verified base_sha:\n%s\n", step) > "/dev/stderr";
      bad = 1;
    }
    if (step !~ /persist-credentials: false/) {
      printf("semantic-review trusted checkout must not persist credentials:\n%s\n", step) > "/dev/stderr";
      bad = 1;
    }
    if (step ~ /(head_sha|head_ref|workflow_run\.head_sha|github\.head_ref)/) {
      printf("semantic-review trusted checkout must not reference PR head inputs:\n%s\n", step) > "/dev/stderr";
      bad = 1;
    }
    in_checkout = 0;
    step = "";
  }
  /^      - (name|uses):/ {
    finish_checkout();
  }
  /uses: actions\/checkout@/ {
    in_checkout = 1;
    step = $0 "\n";
    next;
  }
  in_checkout {
    step = step $0 "\n";
  }
  END {
    finish_checkout();
    if (checkouts < 2) {
      printf("semantic-review should have at least two trusted checkout steps, got %d\n", checkouts) > "/dev/stderr";
      bad = 1;
    }
    exit bad ? 1 : 0;
  }
' "$workflow"; then
  exit 1
fi

for forbidden in \
  "manifest-export" \
  "quality-gate manifest" \
  "quality-gate command-index" \
  "make quality-gate"; do
  if grep -Fq "$forbidden" "$workflow"; then
    echo "semantic-review trusted workflow must not contain: $forbidden" >&2
    exit 1
  fi
done

if ! grep -q '^  pr-quality-summary:' "$workflow"; then
  echo "semantic-review workflow should publish a PR quality summary for CI workflow_run results" >&2
  exit 1
fi

if ! grep -Fq "needs: pr-quality-summary" "$workflow"; then
  echo "semantic-review job should wait for PR quality summary cleanup/publication" >&2
  exit 1
fi

if grep -Fq "needs.pr-quality-summary.result == 'success'" "$workflow"; then
  echo "semantic-review job should still run after PR quality summary publication fails" >&2
  exit 1
fi

if ! grep -Fq "if: always() && github.event.workflow_run.conclusion == 'success' && github.event.workflow_run.event == 'pull_request'" "$workflow"; then
  echo "semantic-review job should use always() so its check still runs after PR quality summary failures" >&2
  exit 1
fi

require_in_step "$summary_verify_step" 'workflowPath !== ".github/workflows/ci.yml"' "PR quality summary must verify the triggering workflow path"
require_in_step "$summary_verify_step" 'run.event !== "pull_request"' "PR quality summary must only handle pull_request workflow_run events"
require_in_step "$summary_verify_step" 'run.repository.id !== context.payload.repository.id' "PR quality summary must verify workflow_run repository id"
require_in_step "$summary_verify_step" 'const targetHeadSha = run.head_sha' "PR quality summary must use the CI run head SHA as the verified PR head"
require_in_step "$summary_verify_step" 'eventHeadSha && eventHeadSha.toLowerCase() !== targetHeadSha.toLowerCase()' "PR quality summary should tolerate mutable workflow_run PR head metadata"
require_in_step "$summary_verify_step" 'factsArtifactPattern' "PR quality summary should use the base-bound facts artifact name when available"
require_in_step "$summary_verify_step" 'const baseSha = artifactBaseSha || eventBaseSha || pr.base.sha' "PR quality summary must prefer the CI-time artifact base SHA"
require_in_step "$summary_verify_step" 'core.setOutput("artifact_error"' "PR quality summary must expose artifact binding failures"
require_in_step "$summary_verify_step" 'state: "all"' "PR quality summary fallback must inspect closed PRs before failing"
require_in_step "$summary_verify_step" 'candidate.state === "open"' "PR quality summary fallback must still prefer open PRs"
require_in_step "$summary_verify_step" 'workflow_run target PR is no longer open' "PR quality summary must skip stale workflow_run events after PR closure"
require_in_step "$summary_verify_step" 'pr.state !== "open"' "PR quality summary must skip direct workflow_run PR bindings after PR closure"
require_in_step "$summary_artifact_step" 'factsArtifactName' "PR quality summary artifact step must use the verified facts artifact binding"
require_in_step "$summary_extract_facts_step" 'SEMANTIC_REVIEW_DECISION_OUT' "PR quality summary artifact verifier must write an infrastructure decision on verifier failure"

if grep -Fq 'run.conclusion !== "success"' <<<"$summary_verify_step"; then
  echo "PR quality summary must run for failed pull_request CI runs, not only successful runs" >&2
  exit 1
fi

require_in_step "$summary_publish_step" 'CI_QUALITY_SUMMARY_HEAD_SHA' "PR quality summary publisher must receive verified head SHA"
require_in_step "$summary_publish_step" 'CI_QUALITY_SUMMARY_BASE_SHA' "PR quality summary publisher must receive verified base SHA"
require_in_step "$summary_publish_step" 'CI_QUALITY_SUMMARY_RUN_ID' "PR quality summary publisher must receive verified workflow run id"
require_in_step "$summary_publish_step" 'require("./scripts/ci-quality-summary-publish.js")' "PR quality summary publisher must use the shared CI publisher script"

require_in_step "$verify_step" 'workflowPath !== ".github/workflows/ci.yml"' "semantic-review must verify the triggering workflow path"
require_in_step "$verify_step" 'run.repository.id !== context.payload.repository.id' "semantic-review must verify workflow_run repository id"
require_in_step "$verify_step" 'run.event !== "pull_request"' "semantic-review must only handle pull_request workflow_run events"
require_in_step "$verify_step" 'run.conclusion !== "success"' "semantic-review must only consume successful CI runs"
require_in_step "$verify_step" 'const eventHeadSha = runPRs[0]?.head?.sha || ""' "semantic-review should inspect workflow_run PR head metadata"
require_in_step "$verify_step" 'const targetHeadSha = run.head_sha' "semantic-review target PR head must come from the completed CI run"
require_in_step "$verify_step" 'eventHeadSha && eventHeadSha.toLowerCase() !== targetHeadSha.toLowerCase()' "semantic-review should tolerate mutable workflow_run PR head metadata"
require_in_step "$verify_step" 'factsArtifactPattern' "semantic-review must use a base-bound facts artifact name"
require_in_step "$verify_step" 'listWorkflowRunArtifacts' "semantic-review must read the workflow_run artifacts before resolving fallback base SHA"
require_in_step "$verify_step" 'artifactHeadSha.toLowerCase() !== targetHeadSha.toLowerCase()' "semantic-review must not let the artifact choose a different PR head"
require_in_step "$verify_step" 'artifactError =' "semantic-review must preserve PR target outputs when artifact binding is unavailable"
require_in_step "$verify_step" 'runPRs.length > 1' "semantic-review must fail closed on ambiguous workflow_run PR bindings"
require_in_step "$verify_step" 'listPullRequestsAssociatedWithCommit' "semantic-review must resolve fork workflow_run PRs when pull_requests is empty"
require_in_step "$verify_step" 'commit_sha: targetHeadSha' "semantic-review fallback must resolve PRs by the workflow_run PR head SHA"
require_in_step "$verify_step" 'github.rest.pulls.list' "semantic-review must have a pull-list fallback when commit association is empty"
require_in_step "$verify_step" 'openCandidatePRs.length > 1' "semantic-review must fail closed when commit-to-PR fallback is ambiguous"
require_in_step "$verify_step" 'state: "all"' "semantic-review fallback must inspect closed PRs before failing"
require_in_step "$verify_step" 'candidate.state === "open"' "semantic-review fallback must still prefer open PRs"
require_in_step "$verify_step" 'workflow_run target PR is no longer open' "semantic-review must skip stale workflow_run events after PR closure"
require_in_step "$verify_step" 'pr.state !== "open"' "semantic-review must skip direct workflow_run PR bindings after PR closure"
require_in_step "$verify_step" '!pr.head.repo' "semantic-review must skip unavailable PR head repositories before reading owner/repo"
require_in_step "$verify_step" 'pr.head.sha !== targetHeadSha' "semantic-review must skip stale PR heads"
require_in_step "$verify_step" 'eventBaseSha && parsedBaseSha.toLowerCase() !== eventBaseSha.toLowerCase()' "semantic-review should tolerate mutable workflow_run PR base metadata"
require_in_step "$verify_step" 'const baseSha = artifactBaseSha || eventBaseSha || pr.base.sha' "semantic-review must prefer the CI-time artifact base SHA"
require_in_step "$verify_step" 'pr.base.sha !== baseSha' "semantic-review must skip stale PR bases"
require_in_step "$verify_step" 'core.setOutput("run_id"' "semantic-review must pass verified workflow run id to publisher"
require_in_step "$verify_step" 'core.setOutput("head_repo_id"' "semantic-review must pass verified head repo id"
require_in_step "$verify_step" 'core.setOutput("head_is_base_repo"' "semantic-review must expose same-repo versus fork boundary"
require_in_step "$verify_step" 'core.setOutput("facts_artifact_name"' "semantic-review must pass the verified facts artifact binding"
require_in_step "$verify_step" 'core.setOutput("artifact_error"' "semantic-review must expose artifact binding failures for infrastructure reporting"

require_in_step "$artifact_step" 'factsArtifactName' "semantic-review artifact step must use the verified facts artifact binding"
require_in_step "$artifact_step" 'a.name === factsArtifactName' "semantic-review must select only the verified quality-gate-facts artifact"
require_in_step "$artifact_step" 'artifacts.length !== 1' "semantic-review must reject missing or duplicate facts artifacts"
require_in_step "$artifact_step" 'artifact.expired' "semantic-review must reject expired facts artifacts"
require_in_step "$artifact_step" 'artifact.size_in_bytes > 5 * 1024 * 1024' "semantic-review must cap facts artifact size"
require_in_step "$artifact_step" 'artifact.digest' "semantic-review must require the GitHub artifact digest"
require_in_step "$extract_facts_step" 'SEMANTIC_REVIEW_DECISION_OUT' "semantic-review artifact verifier must write an infrastructure decision on verifier failure"
require_in_step "$extract_facts_step" 'SEMANTIC_REVIEW_MARKDOWN_OUT' "semantic-review artifact verifier must write markdown on verifier failure"

require_in_step "$waiver_step" 'SEMANTIC_REVIEW_HEAD_IS_BASE_REPO' "waiver step must know whether PR head is in the base repo"
require_in_step "$waiver_step" 'fork PR semantic waiver config is ignored' "fork PR head waiver must be ignored"
require_in_step "$waiver_step" 'core.setOutput("path", "")' "fork PR must not pass an empty waiver override file"
require_in_step "$waiver_step" 'owner: headOwner' "same-repo waiver fetch must use the verified head owner"
require_in_step "$waiver_step" 'repo: headRepo' "same-repo waiver fetch must use the verified head repo"
require_in_step "$waiver_step" 'ref: headSha' "same-repo waiver fetch must use the verified head sha"
require_in_step "$waiver_step" 'data.size > 256 * 1024' "semantic-review should cap PR waiver config size before parsing"

if ! awk '
  /Download PR semantic waiver config/ { in_step = 1 }
  in_step && /const headIsBaseRepo/ { seen = 1 }
  seen && /fork PR semantic waiver config is ignored/ { notice = 1 }
  notice && /core\.setOutput\("path", ""\)/ { output = 1 }
  output && /return;/ { returned = 1 }
  in_step && /github\.rest\.repos\.getContent/ { if (!returned) exit 2 }
  in_step && /^      - name:/ && !/Download PR semantic waiver config/ { exit }
  END { exit returned ? 0 : 1 }
' "$workflow"; then
  echo "fork PR waiver config must be ignored before any head repo content fetch" >&2
  exit 1
fi

require_in_step "$semantic_step" 'if [ -n "${{ steps.waiver_config.outputs.path }}" ]; then' "semantic review must not pass an empty waivers-file override"
require_in_step "$semantic_step" 'args+=(--waivers-file' "same-repo PR head waiver path must still be passed when present"

require_in_step "$precheckout_step" 'SEMANTIC_REVIEW_BASE_SHA' "pre-checkout failure publisher must receive verified base SHA"
require_in_step "$precheckout_step" 'SEMANTIC_REVIEW_RUN_ID' "pre-checkout failure publisher must receive verified run id"
require_in_step "$precheckout_step" 'github.rest.pulls.get' "pre-checkout failure publisher must recheck PR target before writing"
require_in_step "$precheckout_step" 'pull.state !== "open"' "pre-checkout failure publisher must skip closed PRs before writing"
require_in_step "$precheckout_step" 'pull.head.sha !== headSha' "pre-checkout failure publisher must skip stale PR heads"
require_in_step "$precheckout_step" 'pull.base.sha !== baseSha' "pre-checkout failure publisher must skip stale PR bases"

require_in_step "$publish_step" 'SEMANTIC_REVIEW_HEAD_SHA' "semantic-review publisher must receive verified head SHA"
require_in_step "$publish_step" 'SEMANTIC_REVIEW_BASE_SHA' "semantic-review publisher must receive verified base SHA"
require_in_step "$publish_step" 'SEMANTIC_REVIEW_RUN_ID' "semantic-review publisher must receive verified run id"
require_in_step "$publish_step" 'require("./scripts/semantic-review-publish.js")' "semantic-review publisher must use the shared publisher script"
