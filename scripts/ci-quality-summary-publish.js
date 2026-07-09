// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const fs = require("fs");
const {
  deleteQualitySummaries,
  inlineCode,
  markdownText,
  publishQualitySummary,
} = require("./pr-quality-summary.js");

function readJSON(path) {
  try {
    const raw = fs.readFileSync(path, "utf8");
    const value = JSON.parse(raw);
    return value && typeof value === "object" && !Array.isArray(value) ? value : {};
  } catch {
    return {};
  }
}

function verifiedPublishTarget() {
  const pr = Number(process.env.CI_QUALITY_SUMMARY_PR_NUMBER || 0);
  if (!Number.isInteger(pr) || pr <= 0) {
    throw new Error("missing verified PR quality summary pull request number");
  }
  const headSha = process.env.CI_QUALITY_SUMMARY_HEAD_SHA || "";
  if (!/^[a-f0-9]{40}$/i.test(headSha)) {
    throw new Error("missing verified PR quality summary head sha");
  }
  const baseSha = process.env.CI_QUALITY_SUMMARY_BASE_SHA || "";
  if (!/^[a-f0-9]{40}$/i.test(baseSha)) {
    throw new Error("missing verified PR quality summary base sha");
  }
  const runId = process.env.CI_QUALITY_SUMMARY_RUN_ID || "";
  if (!/^\d+$/.test(runId)) {
    throw new Error("missing verified PR quality summary workflow run id");
  }
  return { pr, headSha, baseSha, runId };
}

async function publishTargetStillCurrent(github, context, core, target, phase = "publishing") {
  const { data: pr } = await github.rest.pulls.get({
    owner: context.repo.owner,
    repo: context.repo.repo,
    pull_number: target.pr,
  });
  if (pr.state !== "open") {
    core.notice(`PR quality summary skipped: PR is no longer open before ${phase}`);
    return false;
  }
  if (pr.head.sha !== target.headSha) {
    core.notice(`PR quality summary skipped: PR head changed before ${phase}`);
    return false;
  }
  if (pr.base.sha !== target.baseSha) {
    core.notice(`PR quality summary skipped: PR base changed before ${phase}`);
    return false;
  }
  if (pr.base.repo.id !== context.payload.repository.id) {
    throw new Error("PR base repo mismatch before PR quality summary publishing");
  }
  return true;
}

function isFailedJob(job) {
  const conclusion = String(job?.conclusion || "").toLowerCase();
  return conclusion === "failure" ||
    conclusion === "cancelled" ||
    conclusion === "timed_out" ||
    conclusion === "action_required";
}

function failedJobs(jobs) {
  return (Array.isArray(jobs) ? jobs : []).filter(isFailedJob);
}

function jobName(job) {
  return String(job?.name || job?.job_name || "unknown");
}

function jobConclusion(job) {
  return String(job?.conclusion || job?.status || "unknown");
}

function jobDetails(job) {
  const url = String(job?.html_url || "");
  return url ? `[details](${url})` : "details unavailable";
}

function diagnosticLocation(diagnostic) {
  const file = String(diagnostic?.file || "");
  const line = Number(diagnostic?.line || 0);
  if (file && Number.isInteger(line) && line > 0) {
    return `${file}:${line}`;
  }
  const command = String(diagnostic?.command_path || "");
  if (command) {
    return command;
  }
  return "summary-only";
}

function rejectDiagnostics(facts) {
  return (Array.isArray(facts?.diagnostics) ? facts.diagnostics : [])
    .filter((diagnostic) => String(diagnostic?.action || "").toUpperCase() === "REJECT");
}

function buildCIQualitySummary({ run, jobs, facts = {}, artifactError = "" }) {
  const failed = failedJobs(jobs);
  const runConclusion = String(run?.conclusion || "");
  if (failed.length === 0 && runConclusion === "success") {
    return "";
  }

  const lines = [
    "## PR Quality Summary",
    "",
    "CI did not complete successfully. Use the failed check links below to decide whether this PR needs a code change or a rerun.",
    "",
  ];

  if (failed.length > 0) {
    lines.push("### Failed checks", "");
    for (const job of failed) {
      lines.push(`- **${markdownText(jobName(job))}** — ${markdownText(jobConclusion(job))} — ${jobDetails(job)}`);
    }
    lines.push("");
  } else {
    lines.push(`### CI status`, "", `- Workflow conclusion: ${markdownText(runConclusion || "unknown")}.`, "");
  }

  const deterministicFailed = failed.some((job) => jobName(job) === "deterministic-gate");
  if (deterministicFailed) {
    const diagnostics = rejectDiagnostics(facts);
    lines.push("### deterministic-gate", "");
    if (diagnostics.length === 0) {
      const reason = artifactError || "quality-gate facts did not include a blocking diagnostic for this failed run";
      lines.push(`- System issue: deterministic-gate failed, but quality-gate facts were unavailable. ${markdownText(reason)}`);
    } else {
      for (const diagnostic of diagnostics.slice(0, 20)) {
        const parts = [
          `**${markdownText(diagnostic?.rule || "quality-gate")}**`,
          inlineCode(diagnosticLocation(diagnostic)),
          markdownText(diagnostic?.message || ""),
        ];
        if (diagnostic?.suggestion) {
          parts.push(`Action: ${markdownText(diagnostic.suggestion)}`);
        }
        lines.push(`- ${parts.filter(Boolean).join(" — ")}`);
      }
      if (diagnostics.length > 20) {
        lines.push(`- ${diagnostics.length - 20} additional deterministic findings are available in the check logs.`);
      }
    }
    lines.push("");
  }

  return lines.join("\n");
}

async function listWorkflowRunJobs(github, context, runId) {
  return github.paginate(github.rest.actions.listJobsForWorkflowRun, {
    owner: context.repo.owner,
    repo: context.repo.repo,
    run_id: Number(runId),
    per_page: 100,
  });
}

async function publish({ github, context, core }) {
  const run = context.payload.workflow_run;
  if (!run || run.event !== "pull_request") {
    core.notice("PR quality summary skipped: workflow_run is not a pull_request run");
    return;
  }
  const target = verifiedPublishTarget();
  if (!(await publishTargetStillCurrent(github, context, core, target))) {
    return;
  }

  const jobs = await listWorkflowRunJobs(github, context, target.runId);
  const facts = readJSON("facts.json");
  const artifactError = process.env.CI_QUALITY_SUMMARY_ARTIFACT_ERROR || "";
  const markdown = buildCIQualitySummary({ run, jobs, facts, artifactError });

  try {
    if (!markdown) {
      if (!(await publishTargetStillCurrent(github, context, core, target, "summary cleanup"))) {
        return;
      }
      await deleteQualitySummaries({
        github,
        context,
        pr: target.pr,
        target,
        beforeWrite: (action) => publishTargetStillCurrent(github, context, core, target, `summary ${action}`),
      });
      return;
    }

    if (!(await publishTargetStillCurrent(github, context, core, target, "summary comment"))) {
      return;
    }
    await publishQualitySummary({
      github,
      context,
      pr: target.pr,
      target,
      markdown,
      beforeWrite: (action) => publishTargetStillCurrent(github, context, core, target, `summary comment ${action}`),
    });
  } catch (err) {
    core.warning(`PR quality summary comment was not published: ${err.message}`);
    if (typeof core.setFailed === "function") {
      core.setFailed(`PR quality summary comment was not published: ${err.message}`);
    } else {
      throw err;
    }
  }
}

module.exports = {
  buildCIQualitySummary,
  failedJobs,
  isFailedJob,
  publish,
  verifiedPublishTarget,
};
