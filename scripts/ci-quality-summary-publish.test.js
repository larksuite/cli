// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const {
  buildCIQualitySummary,
  failedJobs,
  isFailedJob,
  publish,
  verifiedPublishTarget,
} = require("./ci-quality-summary-publish.js");

describe("ci-quality-summary-publish", () => {
  it("classifies failed CI job conclusions", () => {
    assert.equal(isFailedJob({ conclusion: "failure" }), true);
    assert.equal(isFailedJob({ conclusion: "cancelled" }), true);
    assert.equal(isFailedJob({ conclusion: "timed_out" }), true);
    assert.equal(isFailedJob({ conclusion: "success" }), false);
    assert.deepEqual(failedJobs([
      { name: "unit-test", conclusion: "success" },
      { name: "lint", conclusion: "failure" },
    ]).map((job) => job.name), ["lint"]);
  });

  it("builds no summary for successful CI with no failed jobs", () => {
    const markdown = buildCIQualitySummary({
      run: { conclusion: "success" },
      jobs: [{ name: "results", conclusion: "success" }],
    });

    assert.equal(markdown, "");
  });

  it("builds a regular CI failure summary with check links", () => {
    const markdown = buildCIQualitySummary({
      run: { conclusion: "failure" },
      jobs: [
        { name: "unit-test", conclusion: "failure", html_url: "https://github.example/jobs/1" },
        { name: "results", conclusion: "failure", html_url: "https://github.example/jobs/2" },
      ],
    });

    assert.match(markdown, /## PR Quality Summary/);
    assert.match(markdown, /### Failed checks/);
    assert.match(markdown, /\*\*unit-test\*\* — failure/);
    assert.match(markdown, /\[details\]\(https:\/\/github.example\/jobs\/1\)/);
    assert.doesNotMatch(markdown, /### deterministic-gate/);
  });

  it("adds deterministic diagnostics when deterministic-gate fails with facts", () => {
    const markdown = buildCIQualitySummary({
      run: { conclusion: "failure" },
      jobs: [{ name: "deterministic-gate", conclusion: "failure", html_url: "https://github.example/jobs/dg" }],
      facts: {
        diagnostics: [{
          rule: "error_hint",
          action: "REJECT",
          file: "shortcuts/contact/contact_get_user.go",
          line: 30,
          message: "Boundary invalid-argument error lacks an actionable recovery step.",
          suggestion: "Update the hint with supported --user-id-type values.",
        }],
      },
    });

    assert.match(markdown, /### deterministic-gate/);
    assert.match(markdown, /error\\_hint/);
    assert.match(markdown, /shortcuts\/contact\/contact_get_user.go:30/);
    assert.match(markdown, /Action: Update the hint/);
  });

  it("reports deterministic facts as a system issue when artifact data is missing", () => {
    const markdown = buildCIQualitySummary({
      run: { conclusion: "failure" },
      jobs: [{ name: "deterministic-gate", conclusion: "failure" }],
      facts: {},
      artifactError: "quality-gate facts artifact expired",
    });

    assert.match(markdown, /System issue/);
    assert.match(markdown, /quality-gate facts artifact expired/);
  });

  it("requires verifier-provided publish target", () => {
    const env = saveEnv();
    try {
      delete process.env.CI_QUALITY_SUMMARY_PR_NUMBER;
      assert.throws(() => verifiedPublishTarget(), /missing verified PR quality summary pull request number/);
    } finally {
      restoreEnv(env);
    }
  });

  it("deletes an existing summary when CI succeeds", async () => {
    await withPublishTempDir(async ({ calls }) => {
      await publish({
        github: fakeGithub(calls, {
          jobs: [{ name: "results", conclusion: "success" }],
          issueComments: [{
            id: 99,
            user: { type: "Bot" },
            body: "<!-- lark-cli-pr-quality-summary head=old -->",
          }],
        }),
        context: workflowRunContext({ conclusion: "success" }),
        core: silentCore(calls),
      });

      assert.equal(calls.comments.length, 0);
      assert.deepEqual(calls.deletedComments.map((c) => c.comment_id), [99]);
    });
  });

  it("publishes a summary when CI has failed jobs", async () => {
    await withPublishTempDir(async ({ calls }) => {
      await publish({
        github: fakeGithub(calls, {
          jobs: [{ name: "unit-test", conclusion: "failure", html_url: "https://github.example/jobs/1" }],
        }),
        context: workflowRunContext({ conclusion: "failure" }),
        core: silentCore(calls),
      });

      assert.equal(calls.comments.length, 1);
      assert.equal(calls.comments[0].issue_number, 42);
      assert.match(calls.comments[0].body, /^<!-- lark-cli-pr-quality-summary /);
      assert.match(calls.comments[0].body, /\*\*unit-test\*\*/);
    });
  });

  it("does not publish a summary when the PR head changes before comment creation", async () => {
    await withPublishTempDir(async ({ calls }) => {
      await publish({
        github: fakeGithub(calls, {
          jobs: [{ name: "unit-test", conclusion: "failure", html_url: "https://github.example/jobs/1" }],
          pullResponses: [
            currentPullResponse(),
            currentPullResponse({ headSha: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" }),
          ],
        }),
        context: workflowRunContext({ conclusion: "failure" }),
        core: silentCore(calls),
      });

      assert.equal(calls.comments.length, 0);
      assert.match(calls.notices.join("\n"), /PR head changed/);
    });
  });

  it("does not publish a summary when the PR closes before comment creation", async () => {
    await withPublishTempDir(async ({ calls }) => {
      await publish({
        github: fakeGithub(calls, {
          jobs: [{ name: "unit-test", conclusion: "failure", html_url: "https://github.example/jobs/1" }],
          pullResponses: [
            currentPullResponse(),
            currentPullResponse({ state: "closed" }),
          ],
        }),
        context: workflowRunContext({ conclusion: "failure" }),
        core: silentCore(calls),
      });

      assert.equal(calls.comments.length, 0);
      assert.match(calls.notices.join("\n"), /PR is no longer open/);
    });
  });

  it("does not delete an existing summary when the PR base changes before cleanup", async () => {
    await withPublishTempDir(async ({ calls }) => {
      await publish({
        github: fakeGithub(calls, {
          jobs: [{ name: "results", conclusion: "success" }],
          issueComments: [{
            id: 99,
            user: { type: "Bot" },
            body: "<!-- lark-cli-pr-quality-summary head=old -->",
          }],
          pullResponses: [
            currentPullResponse(),
            currentPullResponse({ baseSha: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" }),
          ],
        }),
        context: workflowRunContext({ conclusion: "success" }),
        core: silentCore(calls),
      });

      assert.equal(calls.deletedComments.length, 0);
      assert.match(calls.notices.join("\n"), /PR base changed/);
    });
  });

  it("publishes deterministic diagnostics from facts.json", async () => {
    await withPublishTempDir(async ({ calls }) => {
      fs.writeFileSync("facts.json", JSON.stringify({
        diagnostics: [{
          rule: "skill_reference",
          action: "REJECT",
          file: "skills/lark-doc/SKILL.md",
          line: 9,
          message: "Invalid command reference.",
          suggestion: "Use docs +fetch.",
        }],
      }), "utf8");

      await publish({
        github: fakeGithub(calls, {
          jobs: [{ name: "deterministic-gate", conclusion: "failure" }],
        }),
        context: workflowRunContext({ conclusion: "failure" }),
        core: silentCore(calls),
      });

      assert.match(calls.comments[0].body, /### deterministic-gate/);
      assert.match(calls.comments[0].body, /skills\/lark-doc\/SKILL\.md:9/);
      assert.match(calls.comments[0].body, /Use docs \+fetch/);
    });
  });

  it("fails visibly when a required CI summary cannot be published", async () => {
    await withPublishTempDir(async ({ calls }) => {
      await publish({
        github: fakeGithub(calls, {
          failComments: true,
          jobs: [{ name: "unit-test", conclusion: "failure" }],
        }),
        context: workflowRunContext({ conclusion: "failure" }),
        core: silentCore(calls),
      });

      assert.equal(calls.comments.length, 0);
      assert.match(calls.warnings[0], /PR quality summary comment was not published/);
      assert.match(calls.failures[0], /PR quality summary comment was not published/);
    });
  });
});

function saveEnv() {
  return {
    CI_QUALITY_SUMMARY_PR_NUMBER: process.env.CI_QUALITY_SUMMARY_PR_NUMBER,
    CI_QUALITY_SUMMARY_HEAD_SHA: process.env.CI_QUALITY_SUMMARY_HEAD_SHA,
    CI_QUALITY_SUMMARY_BASE_SHA: process.env.CI_QUALITY_SUMMARY_BASE_SHA,
    CI_QUALITY_SUMMARY_RUN_ID: process.env.CI_QUALITY_SUMMARY_RUN_ID,
    CI_QUALITY_SUMMARY_ARTIFACT_ERROR: process.env.CI_QUALITY_SUMMARY_ARTIFACT_ERROR,
  };
}

function restoreEnv(env) {
  for (const [key, value] of Object.entries(env)) {
    if (value === undefined) {
      delete process.env[key];
    } else {
      process.env[key] = value;
    }
  }
}

async function withPublishTempDir(fn) {
  const env = saveEnv();
  const cwd = process.cwd();
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "ci-quality-summary-"));
  const calls = { comments: [], deletedComments: [], failures: [], notices: [], order: [], warnings: [] };
  try {
    process.chdir(dir);
    process.env.CI_QUALITY_SUMMARY_PR_NUMBER = "42";
    process.env.CI_QUALITY_SUMMARY_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
    process.env.CI_QUALITY_SUMMARY_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
    process.env.CI_QUALITY_SUMMARY_RUN_ID = "123456";
    await fn({ calls, dir });
  } finally {
    process.chdir(cwd);
    restoreEnv(env);
  }
}

function workflowRunContext({ conclusion }) {
  return {
    repo: { owner: "larksuite", repo: "cli" },
    payload: {
      repository: { id: 123 },
      workflow_run: {
        id: 123456,
        event: "pull_request",
        conclusion,
      },
    },
  };
}

function silentCore(calls) {
  return {
    notice(message) {
      calls.notices.push(message);
    },
    warning(message) {
      calls.warnings.push(message);
    },
    setFailed(message) {
      calls.failures.push(message);
    },
  };
}

function fakeGithub(calls, options = {}) {
  const pullResponses = Array.isArray(options.pullResponses) ? [...options.pullResponses] : null;
  const api = {
    paginate: async (endpoint) => {
      if (options.failComments && endpoint === api.rest.issues.listComments) {
        throw new Error("comment API unavailable");
      }
      if (endpoint === api.rest.actions.listJobsForWorkflowRun) {
        return options.jobs || [];
      }
      if (endpoint === api.rest.issues.listComments) {
        return options.issueComments || [];
      }
      return [];
    },
    rest: {
      actions: {
        listJobsForWorkflowRun() {},
      },
      issues: {
        listComments() {},
        createComment: async (args) => {
          if (options.failComments) {
            throw new Error("comment API unavailable");
          }
          calls.comments.push(args);
          calls.order.push("comment");
        },
        updateComment: async (args) => {
          if (options.failComments) {
            throw new Error("comment API unavailable");
          }
          calls.comments.push(args);
          calls.order.push("comment");
        },
        deleteComment: async (args) => {
          calls.deletedComments.push(args);
          calls.order.push("comment-delete");
        },
      },
      pulls: {
        get: async () => pullResponses && pullResponses.length > 0 ? pullResponses.shift() : currentPullResponse(),
      },
    },
  };
  return api;
}

function currentPullResponse(overrides = {}) {
  return {
    data: {
      state: overrides.state || "open",
      head: { sha: overrides.headSha || process.env.CI_QUALITY_SUMMARY_HEAD_SHA },
      base: {
        sha: overrides.baseSha || process.env.CI_QUALITY_SUMMARY_BASE_SHA,
        repo: { id: 123 },
      },
    },
  };
}
