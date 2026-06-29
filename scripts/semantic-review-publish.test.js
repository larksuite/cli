// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const {
  buildSummaryMarkdown,
  buildChangedLineIndex,
  checkConclusion,
  checkName,
  changedLinesFromPatch,
  findingKey,
  inlineCode,
  inlineCommentBody,
  loadExistingInlineThreads,
  loadDecision,
  parseBlockMode,
  publish,
  sanitizeMarkdownBody,
  selectInlineTarget,
} = require("./semantic-review-publish.js");

describe("semantic-review-publish", () => {
  it("formats author-facing summary groups from decision and facts", () => {
    const decision = {
      block_mode: true,
      blockers: [{
        category: "skill_quality",
        severity: "major",
        review_action: "must_fix",
        evidence: ["facts.skills[0]"],
        fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
        message: "skill references an invalid command",
        suggested_action: "update the command reference",
      }],
      warnings: [
        {
          category: "error_hint",
          severity: "major",
          review_action: "confirm",
          evidence: ["facts.errors[0]"],
          fingerprint: "category:error_hint|errors:file:cmd/root.go:line:77",
          message: "hint is covered by an exception",
          suggested_action: "confirm the exception still applies",
          waiver_id: "err-hint-existing",
        },
        {
          category: "default_output",
          severity: "minor",
          review_action: "observe",
          evidence: ["facts.outputs[0]"],
          fingerprint: "category:default_output|outputs:command:drive files list",
          message: "list output lacks a decision field",
          suggested_action: "track for a later cleanup",
        },
      ],
      system_warnings: [{
        severity: "minor",
        message: "review used a degraded model response",
        suggested_action: "inspect logs",
      }],
    };
    const facts = {
      skills: [{
        source_file: "skills/lark-doc/SKILL.md",
        line: 30,
        command_path: "docs +fetch",
      }],
      errors: [{
        file: "cmd/root.go",
        line: 77,
        command_path: "lark-cli",
      }],
      outputs: [{
        command: "drive files list",
      }],
    };

    const markdown = buildSummaryMarkdown(decision, facts, new Map());

    assert.match(markdown, /### Must fix/);
    assert.match(markdown, /skills\/lark-doc\/SKILL\.md:30/);
    assert.match(markdown, /### Confirm/);
    assert.match(markdown, /err-hint-existing/);
    assert.match(markdown, /cmd\/root\.go:77/);
    assert.doesNotMatch(markdown, /### Non-blocking observations/);
    assert.doesNotMatch(markdown, /drive files list/);
    assert.match(markdown, /### System status/);
    assert.match(markdown, /review used a degraded model response/);
  });

  it("keeps observe findings out of confirm-only PR summaries", () => {
    const markdown = buildSummaryMarkdown({
      block_mode: true,
      blockers: [],
      warnings: [
        {
          category: "error_hint",
          severity: "major",
          review_action: "confirm",
          evidence: ["facts.errors[0]"],
          fingerprint: "confirm-error",
          message: "hint uses an approved exception",
          suggested_action: "confirm the exception still applies",
        },
        {
          category: "default_output",
          severity: "minor",
          review_action: "observe",
          evidence: ["facts.outputs[0]"],
          fingerprint: "observe-output",
          message: "list output lacks a decision field",
          suggested_action: "track for a later cleanup",
        },
      ],
    }, {
      errors: [{ file: "cmd/root.go", line: 77 }],
      outputs: [{ command: "drive files list" }],
    });

    assert.match(markdown, /### Confirm/);
    assert.match(markdown, /hint uses an approved exception/);
    assert.doesNotMatch(markdown, /### Non-blocking observations/);
    assert.doesNotMatch(markdown, /drive files list/);
  });

  it("escapes model-controlled markdown in structured summaries", () => {
    const markdown = buildSummaryMarkdown({
      block_mode: true,
      blockers: [{
        category: "skill_quality",
        severity: "major",
        review_action: "must_fix",
        evidence: ["facts.skills[0]"],
        fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
        message: "@team\n# forged [link](https://example.com)<b>",
        suggested_action: "**do not** trust raw markdown",
      }],
      warnings: [],
    }, {
      skills: [{
        source_file: "skills/lark-doc/SKILL.md",
        line: 30,
      }],
    });

    assert(!markdown.includes("@team"));
    assert(!markdown.includes("\n# forged"));
    assert(!markdown.includes("https://example.com"));
    assert(!markdown.includes("<b>"));
    assert(markdown.includes("@\u200bteam"));
    assert(markdown.includes("\\# forged"));
    assert(markdown.includes("\\[link\\]"));
    assert(markdown.includes("https[:]//example.com"));
    assert(markdown.includes("&lt;b&gt;"));
  });

  it("parses right-side changed lines from a unified diff patch", () => {
    const patch = [
      "@@ -1,4 +1,5 @@",
      " unchanged",
      "-old line",
      "+new line",
      " context",
      "+another new line",
    ].join("\n");

    assert.deepEqual([...changedLinesFromPatch(patch)], [2, 4]);
  });

  it("selects inline target only when evidence maps to a changed diff line", () => {
    const facts = {
      skills: [{
        source_file: "skills/lark-doc/SKILL.md",
        line: 30,
        command_path: "docs +fetch",
      }],
      errors: [{
        file: "cmd/root.go",
        line: 77,
        command_path: "lark-cli",
      }],
    };
    const changedLineIndex = buildChangedLineIndex([{
      filename: "skills/lark-doc/SKILL.md",
      patch: [
        "@@ -29,2 +29,3 @@",
        " context",
        "+changed skill line",
        " another context",
      ].join("\n"),
    }]);

    assert.deepEqual(
      selectInlineTarget({ evidence: ["facts.skills[0]"] }, facts, changedLineIndex),
      { path: "skills/lark-doc/SKILL.md", line: 30 },
    );
    assert.equal(selectInlineTarget({ evidence: ["facts.errors[0]"] }, facts, changedLineIndex), null);
  });

  it("maps public content evidence to changed files but not virtual metadata", () => {
    const restrictedScope = "pri" + "vate";
    const facts = {
      public_content: [
        {
          rule: "public_content_semantic_candidate",
          action: "WARNING",
          file: "docs/public-roadmap.md",
          line: 4,
          source: "file",
        },
        {
          rule: "public_content_semantic_candidate",
          action: "WARNING",
          file: "pull_request_metadata",
          line: 1,
          source: "metadata",
        },
        {
          rule: "public_content_automation_branch",
          action: "WARNING",
          file: "branch",
          line: 1,
          source: "branch",
        },
        {
          rule: "public_content_change_id_trailer",
          action: "REJECT",
          file: "commit:1234abc",
          line: 3,
          source: "commit",
        },
      ],
    };
    const changedLineIndex = buildChangedLineIndex([{
      filename: "docs/public-roadmap.md",
      patch: [
        "@@ -3,2 +3,3 @@",
        " context",
        "+Specific " + restrictedScope + " roadmap detail",
      ].join("\n"),
    }]);

    assert.deepEqual(
      selectInlineTarget({ evidence: ["facts.public_content[0]"] }, facts, changedLineIndex),
      { path: "docs/public-roadmap.md", line: 4 },
    );
    assert.equal(selectInlineTarget({ evidence: ["facts.public_content[1]"] }, facts, changedLineIndex), null);
    assert.equal(selectInlineTarget({ evidence: ["facts.public_content[2]"] }, facts, changedLineIndex), null);
    assert.equal(selectInlineTarget({ evidence: ["facts.public_content[3]"] }, facts, changedLineIndex), null);

    const markdown = buildSummaryMarkdown({
      block_mode: true,
      blockers: [{
        category: "public_content_leakage",
        severity: "major",
        review_action: "must_fix",
        evidence: ["facts.public_content[1]"],
        fingerprint: "public-content-metadata",
        message: "PR metadata contains " + restrictedScope + " rollout detail",
        suggested_action: "Move " + restrictedScope + " detail to an internal channel.",
      }],
      warnings: [],
    }, facts);
    assert.match(markdown, /pull_request_metadata:1/);

    const virtualMarkdown = buildSummaryMarkdown({
      block_mode: true,
      blockers: [
        {
          category: "public_content_leakage",
          severity: "major",
          review_action: "must_fix",
          evidence: ["facts.public_content[2]"],
          fingerprint: "public-content-branch",
          message: "Branch name looks automation-owned.",
          suggested_action: "Use a maintainer-owned public branch name.",
        },
        {
          category: "public_content_leakage",
          severity: "major",
          review_action: "must_fix",
          evidence: ["facts.public_content[3]"],
          fingerprint: "public-content-commit",
          message: "Commit trailer contains " + restrictedScope + " review metadata.",
          suggested_action: "Remove " + restrictedScope + " review metadata from commits.",
        },
      ],
      warnings: [],
    }, facts);
    assert.match(virtualMarkdown, /branch:1/);
    assert.match(virtualMarkdown, /commit:1234abc:3/);
  });

  it("builds finding markers from stable fingerprints and evidence identity", () => {
    const factsA = {
      skills: [{
        source_file: "skills/lark-doc/SKILL.md",
        line: 30,
      }],
    };
    const factsB = {
      skills: [
        {
          source_file: "skills/lark-im/SKILL.md",
          line: 12,
        },
        {
          source_file: "skills/lark-doc/SKILL.md",
          line: 30,
        },
      ],
    };
    const finding = {
      category: "skill_quality",
      severity: "major",
      review_action: "must_fix",
      evidence: ["facts.skills[0]"],
      fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
      message: "skill references an invalid command",
      suggested_action: "update the command reference",
    };
    const sameFindingAfterFactReorder = {
      ...finding,
      evidence: ["facts.skills[1]"],
    };
    const differentFindingOnSameEvidence = {
      ...finding,
      fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30:other",
      message: "skill omits required argument documentation",
    };
    const sameFindingWithDifferentWording = {
      ...finding,
      message: "invalid command reference in skill",
      suggested_action: "fix the referenced command",
    };

    assert.equal(findingKey(finding, factsA), findingKey(sameFindingAfterFactReorder, factsB));
    assert.equal(findingKey(finding, factsA), findingKey(sameFindingWithDifferentWording, factsA));
    assert.notEqual(findingKey(finding, factsA), findingKey(differentFindingOnSameEvidence, factsA));
  });

  it("uses longer markdown code spans when inline labels contain backticks", () => {
    const body = inlineCommentBody({
      category: "skill_quality",
      severity: "major",
      evidence: ["facts.skills[0]"],
      fingerprint: "category:skill_quality|skills:source_file:skills/`doc`.md:line:30",
      message: "skill references an invalid command",
      suggested_action: "update the command reference",
    }, {
      skills: [{
        source_file: "skills/`doc`.md",
        line: 30,
      }],
    }, {
      path: "skills/`doc`.md",
      line: 30,
    });

    assert.match(body, /``skills\/`doc`\.md:30``/);
    assert(!body.includes("skills/\\`doc\\`.md:30"));
  });

  it("keeps inline code labels on one markdown line", () => {
    const got = inlineCode("abc\n\n## INJECTED\n\n[x](http://evil)\t@team\u0001");

    assert.equal(got, "`abc ## INJECTED [x](http://evil) @team`");
    assert(!got.includes("\n"));
    assert(!got.includes("\t"));
    assert(!got.includes("\u0001"));
  });

  it("sanitizes fact labels and exception ids before rendering code spans", () => {
    const markdown = buildSummaryMarkdown({
      block_mode: true,
      blockers: [{
        category: "skill_quality",
        severity: "major",
        review_action: "must_fix",
        evidence: ["facts.skills[0]"],
        fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
        message: "skill references an invalid command",
        suggested_action: "update the command reference",
        waiver_id: "waiver\n\n## INJECTED\n\n[x](http://evil)",
      }],
      warnings: [],
    }, {
      skills: [{
        source_file: "skills/lark-doc/SKILL.md\n\n## INJECTED\n\n[x](http://evil)",
        line: 30,
      }],
    }, new Map());

    assert(!markdown.includes("\n\n## INJECTED"));
    assert.match(markdown, /Evidence: `skills\/lark-doc\/SKILL\.md ## INJECTED \[x\]\(http:\/\/evil\):30`/);
    assert.match(markdown, /Exception: `waiver ## INJECTED \[x\]\(http:\/\/evil\)`/);
  });

  it("parses block mode exactly", () => {
    assert.equal(parseBlockMode("true"), true);
    assert.equal(parseBlockMode("false"), false);
    assert.equal(parseBlockMode("TRUE"), false);
    assert.equal(parseBlockMode("1"), false);
    assert.equal(parseBlockMode(""), false);
  });

  it("uses distinct check names for observe and result modes", () => {
    assert.equal(checkName(false), "semantic-review/observe");
    assert.equal(checkName(true), "semantic-review/result");
  });

  it("keeps missing decision neutral in comment-only mode", () => {
    const decision = loadDecision(path.join(os.tmpdir(), "missing-semantic-review-decision.json"));
    assert.equal(decision.infrastructure_failure, true);
    assert.equal(checkConclusion(decision, false), "neutral");
  });

  it("fails missing decision in blocking mode", () => {
    const decision = loadDecision(path.join(os.tmpdir(), "missing-semantic-review-decision.json"));
    assert.equal(decision.infrastructure_failure, true);
    assert.equal(checkConclusion(decision, true), "failure");
  });

  it("keeps invalid decision neutral in comment-only mode", () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const decisionPath = path.join(dir, "decision.json");
    fs.writeFileSync(decisionPath, "{", "utf8");

    const decision = loadDecision(decisionPath);
    assert.equal(decision.infrastructure_failure, true);
    assert.equal(checkConclusion(decision, false), "neutral");
  });

  it("treats malformed decision shape as infrastructure failure", () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const decisionPath = path.join(dir, "decision.json");
    fs.writeFileSync(decisionPath, JSON.stringify({ blockers: [] }), "utf8");

    const decision = loadDecision(decisionPath);
    assert.equal(decision.infrastructure_failure, true);
    assert.equal(checkConclusion(decision, true), "failure");
    assert.equal(checkConclusion(decision, false), "neutral");
  });

  it("keeps a valid degraded decision neutral in comment-only mode", () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const decisionPath = path.join(dir, "decision.json");
    fs.writeFileSync(decisionPath, JSON.stringify({
      degraded: true,
      block_mode: false,
      blockers: [],
      warnings: [{ message: "review unavailable" }],
    }), "utf8");

    assert.equal(checkConclusion(loadDecision(decisionPath), false), "neutral");
  });

  it("fails a valid degraded decision in blocking mode", () => {
    const decision = {
      degraded: true,
      block_mode: true,
      blockers: [],
      warnings: [{ message: "review unavailable" }],
    };

    assert.equal(checkConclusion(decision, true), "failure");
  });

  it("maps skipped decisions by runtime block mode", () => {
    const decision = {
      skipped: true,
      block_mode: false,
      system_warnings: [{ severity: "minor", message: "reviewer not configured" }],
    };

    assert.equal(checkConclusion(decision, false), "neutral");
    assert.equal(checkConclusion({ ...decision, block_mode: true }, true), "failure");
  });

  it("maps system warnings by runtime block mode", () => {
    const decision = {
      block_mode: false,
      blockers: [],
      warnings: [],
      system_warnings: [{ severity: "minor", message: "review used a degraded model response" }],
    };

    assert.equal(checkConclusion(decision, false), "neutral");
    assert.equal(checkConclusion({ ...decision, block_mode: true }, true), "failure");
  });

  it("fails a blocking decision with blockers", () => {
    const decision = {
      block_mode: true,
      blockers: [{ category: "naming", message: "reproducible blocker" }],
      warnings: [],
    };

    assert.equal(checkConclusion(decision, true), "failure");
  });

  it("treats runtime decision block mode mismatch as infrastructure failure", () => {
    const decision = {
      block_mode: true,
      blockers: [],
      warnings: [],
    };

    assert.equal(checkConclusion(decision, false), "neutral");
    assert.equal(checkConclusion({ ...decision, block_mode: false }, true), "failure");
  });

  it("sanitizes mentions, HTML, links, bare URLs, and controls in published markdown", () => {
    const got = sanitizeMarkdownBody("@team <b>\u0001 [link](https://example.com) ![img](http://example.com/x.png)");
    assert(!got.includes("@team"));
    assert(!got.includes("<b>"));
    assert(!got.includes("https://example.com"));
    assert(!got.includes("http://example.com"));
    assert(got.includes("@\u200bteam"));
    assert(got.includes("&lt;b&gt;"));
    assert(got.includes("https[:]//example.com"));
    assert(got.includes("http[:]//example.com"));
  });

  it("requires verifier-provided publish target", async () => {
    const env = saveEnv();
    try {
      delete process.env.SEMANTIC_REVIEW_PR_NUMBER;
      delete process.env.SEMANTIC_REVIEW_HEAD_SHA;
      delete process.env.SEMANTIC_REVIEW_BASE_SHA;
      await assert.rejects(
        () => publish({ github: {}, context: workflowRunContext(), core: silentCore() }),
        /missing verified semantic review pull request number/,
      );
    } finally {
      restoreEnv(env);
    }
  });

  it("publishes check and comment to verifier-provided PR head", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [] };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [{
          category: "error_hint",
          severity: "major",
          review_action: "must_fix",
          evidence: ["facts.errors[0]"],
          fingerprint: "error-hint",
          message: "error is missing a recovery hint",
          suggested_action: "add a structured hint",
        }],
        warnings: [],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify({
        schema_version: 1,
        errors: [{ file: "cmd/foo.go", line: 10, changed: true, boundary: true, required_hint: true, hint_action_count: 0 }],
      }), "utf8");
      fs.writeFileSync("semantic-review.md", "## Semantic Review\n\nNo semantic blockers.\n", "utf8");

      await publish({
        github: fakeGithub(calls),
        context: workflowRunContext(),
        core: silentCore(),
      });

      assert.equal(calls.comments.length, 1);
      assert.equal(calls.comments[0].issue_number, 42);
      assert.equal(calls.checks.length, 1);
      assert.equal(calls.checks[0].name, "semantic-review/result");
      assert.equal(calls.checks[0].head_sha, "0123456789abcdef0123456789abcdef01234567");
      assert.match(calls.comments[0].body, /### Must fix/);
      assert.deepEqual(calls.order, ["check", "comment"]);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("deletes an existing summary and publishes no comment when there are no action items", async () => {
    await withPublishTempDir(async ({ calls }) => {
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [],
        warnings: [],
      }), "utf8");

      await publish({
        github: fakeGithub(calls, {
          issueComments: [{
            id: 99,
            user: { type: "Bot" },
            body: "<!-- lark-cli-semantic-review head=old base=old run=1 -->",
          }],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks.length, 1);
      assert.equal(calls.checks[0].conclusion, "success");
      assert.equal(calls.comments.length, 0);
      assert.deepEqual(calls.deletedComments.map((c) => c.comment_id), [99]);
      assert.match(calls.checks[0].output.summary, /No PR Quality Summary was published/);
    });
  });

  it("does not publish a summary or inline comment for observe-only findings by default", async () => {
    await withPublishTempDir(async ({ calls }) => {
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [],
        warnings: [{
          category: "default_output",
          severity: "minor",
          review_action: "observe",
          evidence: ["facts.outputs[0]"],
          fingerprint: "default-output",
          message: "list output lacks a decision field",
          suggested_action: "track for a later cleanup",
        }],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify({
        outputs: [{ command: "drive files list" }],
      }), "utf8");

      await publish({
        github: fakeGithub(calls),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks.length, 1);
      assert.equal(calls.checks[0].conclusion, "success");
      assert.equal(calls.comments.length, 0);
      assert.equal(calls.reviewComments.length, 0);
      assert.match(calls.checks[0].output.summary, /Observe: 1/);
      assert.match(calls.checks[0].output.summary, /No PR Quality Summary was published/);
    });
  });

  it("skips publishing when the PR head changed after verification", async () => {
    await withPublishTempDir(async ({ calls }) => {
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [],
        warnings: [],
      }), "utf8");

      await publish({
        github: fakeGithub(calls, {
          currentPullRequest: {
            head: { sha: "9999999999999999999999999999999999999999" },
            base: {
              sha: "fedcba9876543210fedcba9876543210fedcba98",
              repo: { id: 123 },
            },
          },
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks.length, 0);
      assert.equal(calls.comments.length, 0);
      assert.match(calls.notices[0], /PR head changed before publishing/);
    });
  });

  it("skips publishing when the PR base changed after verification", async () => {
    await withPublishTempDir(async ({ calls }) => {
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [],
        warnings: [],
      }), "utf8");

      await publish({
        github: fakeGithub(calls, {
          currentPullRequest: {
            head: { sha: "0123456789abcdef0123456789abcdef01234567" },
            base: {
              sha: "8888888888888888888888888888888888888888",
              repo: { id: 123 },
            },
          },
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks.length, 0);
      assert.equal(calls.comments.length, 0);
      assert.match(calls.notices[0], /PR base changed before publishing/);
    });
  });

  it("skips publishing when the PR closes after verification", async () => {
    await withPublishTempDir(async ({ calls }) => {
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [],
        warnings: [],
      }), "utf8");

      await publish({
        github: fakeGithub(calls, {
          currentPullRequest: {
            state: "closed",
            head: { sha: "0123456789abcdef0123456789abcdef01234567" },
            base: {
              sha: "fedcba9876543210fedcba9876543210fedcba98",
              repo: { id: 123 },
            },
          },
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks.length, 0);
      assert.equal(calls.comments.length, 0);
      assert.match(calls.notices[0], /PR is no longer open before publishing/);
    });
  });

  it("rejects publishing when the PR base repo changed after verification", async () => {
    await withPublishTempDir(async ({ calls }) => {
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [],
        warnings: [],
      }), "utf8");

      await assert.rejects(
        () => publish({
          github: fakeGithub(calls, {
            currentPullRequest: {
              head: { sha: "0123456789abcdef0123456789abcdef01234567" },
              base: {
                sha: "fedcba9876543210fedcba9876543210fedcba98",
                repo: { id: 456 },
              },
            },
          }),
          context: workflowRunContext(),
          core: silentCore(calls),
        }),
        /PR base repo mismatch before publishing/,
      );

      assert.equal(calls.checks.length, 0);
      assert.equal(calls.comments.length, 0);
    });
  });

  it("skips all PR-visible writes when target is stale before inline publishing", async () => {
    await withPublishTempDir(async ({ calls }) => {
      writeInlineCandidateDecisionAndFacts();

      await publish({
        github: fakeGithub(calls, {
          files: changedSkillFilePatch(),
          currentPullRequests: [{
            head: { sha: "9".repeat(40) },
            base: { sha: process.env.SEMANTIC_REVIEW_BASE_SHA, repo: { id: 123 } },
          }],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.reviewComments.length, 0);
      assert.equal(calls.updatedReviewComments?.length || 0, 0);
      assert.equal(calls.checks.length, 0);
      assert.equal(calls.comments.length, 0);
    });
  });

  it("skips inline update and later writes when target becomes stale before updating an existing discussion", async () => {
    await withPublishTempDir(async ({ calls }) => {
      writeInlineCandidateDecisionAndFacts();

      await publish({
        github: fakeGithub(calls, {
          files: changedSkillFilePatch(),
          reviewThreads: [existingUnresolvedFindingThread()],
          currentPullRequests: [
            currentTarget(),
            { head: { sha: process.env.SEMANTIC_REVIEW_HEAD_SHA }, base: { sha: "8".repeat(40), repo: { id: 123 } } },
          ],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.updatedReviewComments?.length || 0, 0);
      assert.equal(calls.checks.length, 0);
      assert.equal(calls.comments.length, 0);
    });
  });

  it("does not create a check or summary when target becomes stale after inline publishing", async () => {
    await withPublishTempDir(async ({ calls }) => {
      writeInlineCandidateDecisionAndFacts();

      await publish({
        github: fakeGithub(calls, {
          files: changedSkillFilePatch(),
          currentPullRequests: [
            currentTarget(),
            currentTarget(),
            { head: { sha: process.env.SEMANTIC_REVIEW_HEAD_SHA }, base: { sha: "8".repeat(40), repo: { id: 123 } } },
          ],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.reviewComments.length, 1);
      assert.equal(calls.checks.length, 0);
      assert.equal(calls.comments.length, 0);
    });
  });

  it("does not create a summary comment when target becomes stale after check creation", async () => {
    await withPublishTempDir(async ({ calls }) => {
      writeDecisionAndFactsWithoutInline();

      await publish({
        github: fakeGithub(calls, {
          currentPullRequests: [
            currentTarget(),
            currentTarget(),
            { head: { sha: process.env.SEMANTIC_REVIEW_HEAD_SHA }, base: { sha: "8".repeat(40), repo: { id: 123 } } },
          ],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks.length, 1);
      assert.equal(calls.comments.length, 0);
    });
  });

  it("does not update an existing summary comment when target is stale before summary update", async () => {
    await withPublishTempDir(async ({ calls }) => {
      writeDecisionAndFactsWithoutInline();

      await publish({
        github: fakeGithub(calls, {
          issueComments: [{ id: 99, user: { type: "Bot" }, body: "<!-- lark-cli-semantic-review head=old base=old run=1 -->" }],
          currentPullRequests: [
            currentTarget(),
            currentTarget(),
            { head: { sha: process.env.SEMANTIC_REVIEW_HEAD_SHA }, base: { sha: "8".repeat(40), repo: { id: 123 } } },
          ],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks.length, 1);
      assert.equal(calls.comments.length, 0);
    });
  });

  it("updates an existing summary comment on the current target", async () => {
    await withPublishTempDir(async ({ calls }) => {
      writeDecisionAndFactsWithoutInline();

      await publish({
        github: fakeGithub(calls, {
          issueComments: [{ id: 99, user: { type: "Bot" }, body: "<!-- lark-cli-semantic-review head=old base=old run=1 -->" }],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks.length, 1);
      assert.equal(calls.comments.length, 1);
      assert.equal(calls.comments[0].comment_id, 99);
      assert.equal(calls.comments[0].issue_number, undefined);
    });
  });

  it("does not update an existing summary comment when target becomes stale after comment listing", async () => {
    await withPublishTempDir(async ({ calls }) => {
      writeDecisionAndFactsWithoutInline();

      await publish({
        github: fakeGithub(calls, {
          issueComments: [{ id: 99, user: { type: "Bot" }, body: "<!-- lark-cli-semantic-review head=old base=old run=1 -->" }],
          currentPullRequests: [
            currentTarget(),
            currentTarget(),
            currentTarget(),
            { head: { sha: process.env.SEMANTIC_REVIEW_HEAD_SHA }, base: { sha: "8".repeat(40), repo: { id: 123 } } },
          ],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks.length, 1);
      assert.equal(calls.comments.length, 0);
    });
  });

  it("does not create a summary comment when target becomes stale after comment listing", async () => {
    await withPublishTempDir(async ({ calls }) => {
      writeDecisionAndFactsWithoutInline();

      await publish({
        github: fakeGithub(calls, {
          currentPullRequests: [
            currentTarget(),
            currentTarget(),
            currentTarget(),
            { head: { sha: process.env.SEMANTIC_REVIEW_HEAD_SHA }, base: { sha: "8".repeat(40), repo: { id: 123 } } },
          ],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks.length, 1);
      assert.equal(calls.comments.length, 0);
    });
  });

  it("includes head base and run id in the summary marker", async () => {
    await withPublishTempDir(async ({ calls }) => {
      process.env.SEMANTIC_REVIEW_RUN_ID = "123456";
      writeDecisionAndFactsWithoutInline();

      await publish({ github: fakeGithub(calls), context: workflowRunContext(), core: silentCore(calls) });

      assert.match(calls.comments[0].body, /<!-- lark-cli-pr-quality-summary head=0123456789abcdef0123456789abcdef01234567 base=fedcba9876543210fedcba9876543210fedcba98 run=123456 -->/);
    });
  });

  it("publishes an infrastructure failure when decision findings omit review_action", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [], reviewComments: [] };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [{
          category: "skill_quality",
          severity: "major",
          evidence: ["facts.skills[0]"],
          fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
          message: "skill references an invalid command",
          suggested_action: "update the command reference",
        }],
        warnings: [],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify({
        skills: [{
          source_file: "skills/lark-doc/SKILL.md",
          line: 30,
        }],
      }), "utf8");

      await publish({
        github: fakeGithub(calls, {
          files: [{
            filename: "skills/lark-doc/SKILL.md",
            patch: "@@ -29,0 +30,1 @@\n+bad command reference",
          }],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks[0].conclusion, "failure");
      assert.equal(calls.reviewComments.length, 0);
      assert.match(calls.comments[0].body, /### System status/);
      assert.match(calls.comments[0].body, /missing review\\_action/);
      assert.doesNotMatch(calls.comments[0].body, /### Must fix\n\n- \*\*/);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("publishes infrastructure failures for invalid finding publication contracts", async () => {
    const cases = [
      {
        name: "missing fingerprint",
        decision: {
          block_mode: true,
          blockers: [{
            category: "skill_quality",
            severity: "major",
            review_action: "must_fix",
            evidence: ["facts.skills[0]"],
            message: "skill references an invalid command",
            suggested_action: "update the command reference",
          }],
          warnings: [],
        },
        want: /missing fingerprint/,
      },
      {
        name: "blank fingerprint",
        decision: {
          block_mode: true,
          blockers: [{
            category: "skill_quality",
            severity: "major",
            review_action: "must_fix",
            evidence: ["facts.skills[0]"],
            fingerprint: " ",
            message: "skill references an invalid command",
            suggested_action: "update the command reference",
          }],
          warnings: [],
        },
        want: /missing fingerprint/,
      },
      {
        name: "invalid action",
        decision: {
          block_mode: true,
          blockers: [{
            category: "skill_quality",
            severity: "major",
            review_action: "repair",
            evidence: ["facts.skills[0]"],
            fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
            message: "skill references an invalid command",
            suggested_action: "update the command reference",
          }],
          warnings: [],
        },
        want: /missing review\\_action/,
      },
      {
        name: "blocker with confirm action",
        decision: {
          block_mode: true,
          blockers: [{
            category: "skill_quality",
            severity: "major",
            review_action: "confirm",
            evidence: ["facts.skills[0]"],
            fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
            message: "skill references an invalid command",
            suggested_action: "update the command reference",
          }],
          warnings: [],
        },
        want: /review\\_action must be must\\_fix/,
      },
      {
        name: "warning with must_fix action",
        decision: {
          block_mode: true,
          blockers: [],
          warnings: [{
            category: "skill_quality",
            severity: "major",
            review_action: "must_fix",
            evidence: ["facts.skills[0]"],
            fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
            message: "skill references an invalid command",
            suggested_action: "update the command reference",
          }],
        },
        want: /review\\_action must not be must\\_fix/,
      },
    ];

    for (const tc of cases) {
      await withPublishTempDir(async ({ calls }) => {
        fs.writeFileSync("decision.json", JSON.stringify(tc.decision), "utf8");
        fs.writeFileSync("facts.json", JSON.stringify({
          skills: [{
            source_file: "skills/lark-doc/SKILL.md",
            line: 30,
          }],
        }), "utf8");

        await publish({
          github: fakeGithub(calls, {
            files: [{
              filename: "skills/lark-doc/SKILL.md",
              patch: "@@ -29,0 +30,1 @@\n+bad command reference",
            }],
          }),
          context: workflowRunContext(),
          core: silentCore(calls),
        });

        assert.equal(calls.checks[0].conclusion, "failure", tc.name);
        assert.equal(calls.reviewComments.length, 0, tc.name);
        assert.match(calls.comments[0].body, /### System status/, tc.name);
        assert.match(calls.comments[0].body, tc.want, tc.name);
      });
    }
  });

  it("keeps check output status-only and leaves finding details in the PR comment", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [], reviewComments: [] };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [{
          category: "skill_quality",
          severity: "major",
          review_action: "must_fix",
          evidence: ["facts.skills[0]"],
          fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
          message: "skill references an invalid command",
          suggested_action: "update the command reference",
        }],
        warnings: [{
          category: "default_output",
          severity: "minor",
          review_action: "observe",
          evidence: ["facts.outputs[0]"],
          fingerprint: "category:default_output|outputs:command:drive files list",
          message: "list output lacks a decision field",
          suggested_action: "track for a later cleanup",
        }],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify({
        skills: [{
          source_file: "skills/lark-doc/SKILL.md",
          line: 30,
        }],
        outputs: [{
          command: "drive files list",
        }],
      }), "utf8");

      await publish({
        github: fakeGithub(calls),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks[0].conclusion, "failure");
      assert.match(calls.checks[0].output.summary, /Must fix: 1/);
      assert.match(calls.checks[0].output.summary, /Observe: 1/);
      assert.doesNotMatch(calls.checks[0].output.summary, /skill references an invalid command/);
      assert.doesNotMatch(calls.checks[0].output.summary, /### Must fix/);
      assert.doesNotMatch(calls.checks[0].output.summary, /Evidence:/);
      assert.match(calls.comments[0].body, /skill references an invalid command/);
      assert.match(calls.comments[0].body, /### Must fix/);
      assert.doesNotMatch(calls.comments[0].body, /### Non-blocking observations/);
      assert.doesNotMatch(calls.comments[0].body, /list output lacks a decision field/);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("marks must-fix findings as summary-only when PR files cannot be listed", async () => {
    await withPublishTempDir(async ({ calls }) => {
      writeDecisionAndFactsWithoutInline();

      await publish({
        github: fakeGithub(calls, { failListFiles: true }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks[0].conclusion, "failure");
      assert.equal(calls.reviewComments.length, 0);
      assert.match(calls.comments[0].body, /summary-only; PR files were not listed/);
      assert.match(calls.warnings[0], /semantic review PR files were not listed/);
    });
  });

  it("marks must-fix findings without a changed diff line as summary-only", async () => {
    await withPublishTempDir(async ({ calls }) => {
      writeDecisionAndFactsWithoutInline();

      await publish({
        github: fakeGithub(calls, {
          files: [{
            filename: "cmd/foo.go",
            patch: "@@ -1,1 +1,1 @@\n unchanged",
          }],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks[0].conclusion, "failure");
      assert.equal(calls.reviewComments.length, 0);
      assert.match(calls.comments[0].body, /summary-only; no stable changed diff line/);
    });
  });

  it("publishes inline review comments for must-fix findings on changed diff lines", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [], reviewComments: [] };
    const facts = {
      skills: [{
        source_file: "skills/lark-doc/SKILL.md",
        line: 30,
        command_path: "docs +fetch",
      }],
      errors: [{
        file: "skills/lark-doc/SKILL.md",
        line: 30,
        command_path: "docs +fetch",
      }],
    };
    const finding = {
      category: "skill_quality",
      severity: "major",
      review_action: "must_fix",
      evidence: ["facts.skills[0]"],
      fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
      message: "skill references an invalid command",
      suggested_action: "update the command reference",
    };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [finding],
        warnings: [],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify(facts), "utf8");

      await publish({
        github: fakeGithub(calls, {
          files: [{
            filename: "skills/lark-doc/SKILL.md",
            patch: "@@ -29,0 +30,1 @@\n+bad command reference",
          }],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.reviewComments.length, 1);
      assert.equal(calls.reviewComments[0].pull_number, 42);
      assert.equal(calls.reviewComments[0].commit_id, "0123456789abcdef0123456789abcdef01234567");
      assert.equal(calls.reviewComments[0].path, "skills/lark-doc/SKILL.md");
      assert.equal(calls.reviewComments[0].line, 30);
      assert.equal(calls.reviewComments[0].side, "RIGHT");
      assert.match(calls.reviewComments[0].body, new RegExp(`lark-cli-semantic-finding:${findingKey(finding, facts)}`));
      assert.match(calls.reviewComments[0].body, /\*\*Semantic Review: Must fix\*\*/);
      assert.match(calls.reviewComments[0].body, /Resolving this discussion does not change the failed check/);
      assert.match(calls.comments[0].body, /Inline: semantic review posted to `skills\/lark-doc\/SKILL\.md:30`/);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("publishes separate inline comments for different findings on the same changed line", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [], reviewComments: [] };
    const facts = {
      skills: [{
        source_file: "skills/lark-doc/SKILL.md",
        line: 30,
        command_path: "docs +fetch",
      }],
      errors: [{
        file: "skills/lark-doc/SKILL.md",
        line: 30,
        command_path: "docs +fetch",
      }],
    };
    const firstFinding = {
      category: "skill_quality",
      severity: "major",
      review_action: "must_fix",
      evidence: ["facts.skills[0]"],
      fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30:invalid-command",
      message: "skill references an invalid command",
      suggested_action: "update the command reference",
    };
    const secondFinding = {
      category: "error_hint",
      severity: "major",
      review_action: "must_fix",
      evidence: ["facts.errors[0]"],
      fingerprint: "category:error_hint|errors:file:skills/lark-doc/SKILL.md:line:30",
      message: "error hint is not actionable",
      suggested_action: "add a concrete recovery hint",
    };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [firstFinding, secondFinding],
        warnings: [],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify(facts), "utf8");

      await publish({
        github: fakeGithub(calls, {
          files: [{
            filename: "skills/lark-doc/SKILL.md",
            patch: "@@ -29,0 +30,1 @@\n+bad command reference",
          }],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.reviewComments.length, 2);
      assert.notEqual(
        calls.reviewComments[0].body.match(/lark-cli-semantic-finding:([a-f0-9]+)/)[1],
        calls.reviewComments[1].body.match(/lark-cli-semantic-finding:([a-f0-9]+)/)[1],
      );
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("publishes a non-blocking summary for confirm warnings without inline comments", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [], reviewComments: [] };
    const finding = {
      category: "skill_quality",
      severity: "major",
      review_action: "confirm",
      evidence: ["facts.skills[0]"],
      fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
      message: "skill issue is covered by an exception",
      suggested_action: "confirm the exception still applies",
      waiver_id: "skill-doc-waiver",
    };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [],
        warnings: [finding],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify({
        skills: [{
          source_file: "skills/lark-doc/SKILL.md",
          line: 30,
          command_path: "docs +fetch",
        }],
      }), "utf8");

      await publish({
        github: fakeGithub(calls, {
          files: [{
            filename: "skills/lark-doc/SKILL.md",
            patch: "@@ -29,0 +30,1 @@\n+bad command reference",
          }],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks[0].conclusion, "success");
      assert.equal(calls.reviewComments.length, 0);
      assert.match(calls.comments[0].body, /### Confirm/);
      assert.match(calls.comments[0].body, /Exception: `skill-doc-waiver`/);
      assert.doesNotMatch(calls.comments[0].body, /Inline:/);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("does not publish duplicate inline comments for repeated findings in the same decision", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [], reviewComments: [] };
    const finding = {
      category: "skill_quality",
      severity: "major",
      review_action: "must_fix",
      evidence: ["facts.skills[0]"],
      fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
      message: "skill references an invalid command",
      suggested_action: "update the command reference",
    };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [finding, finding],
        warnings: [],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify({
        skills: [{
          source_file: "skills/lark-doc/SKILL.md",
          line: 30,
          command_path: "docs +fetch",
        }],
      }), "utf8");

      await publish({
        github: fakeGithub(calls, {
          files: [{
            filename: "skills/lark-doc/SKILL.md",
            patch: "@@ -29,0 +30,1 @@\n+bad command reference",
          }],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.reviewComments.length, 1);
      assert.match(calls.comments[0].body, /Inline: semantic review posted to `skills\/lark-doc\/SKILL\.md:30`/);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("does not duplicate unresolved inline comment threads and refreshes stale body", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [], reviewComments: [] };
    const facts = {
      skills: [{
        source_file: "skills/lark-doc/SKILL.md",
        line: 30,
        command_path: "docs +fetch",
      }],
    };
    const finding = {
      category: "skill_quality",
      severity: "major",
      review_action: "must_fix",
      evidence: ["facts.skills[0]"],
      fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
      message: "skill references an invalid command",
      suggested_action: "update the command reference",
    };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [finding],
        warnings: [],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify(facts), "utf8");

      await publish({
        github: fakeGithub(calls, {
          files: [{
            filename: "skills/lark-doc/SKILL.md",
            patch: "@@ -29,0 +30,1 @@\n+bad command reference",
          }],
          reviewThreads: [{
            isResolved: false,
            comments: [{
              databaseId: 1001,
              body: `<!-- lark-cli-semantic-finding:${findingKey(finding, facts)} -->\nold body`,
              path: "skills/lark-doc/SKILL.md",
              line: 30,
            }],
          }],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.reviewComments.length, 0);
      assert.equal(calls.updatedReviewComments.length, 1);
      assert.equal(calls.updatedReviewComments[0].comment_id, 1001);
      assert.match(calls.updatedReviewComments[0].body, /Status: Must fix/);
      assert.match(calls.updatedReviewComments[0].body, /skill references an invalid command/);
      assert.match(calls.comments[0].body, /Inline: semantic review updated existing unresolved discussion/);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("ignores existing inline markers from non-bot review comments", async () => {
    const calls = { warnings: [], comments: [], checks: [], order: [], reviewComments: [] };
    const facts = inlineFacts();
    const finding = inlineFinding();
    const key = findingKey(finding, facts);
    const existing = await loadExistingInlineThreads(fakeGithub(calls, {
      reviewThreads: [{
        isResolved: false,
        comments: [{
          author: { __typename: "User", login: "contributor" },
          databaseId: 1001,
          body: `<!-- lark-cli-semantic-finding:${key} -->\nold body`,
          path: "skills/lark-doc/SKILL.md",
          line: 30,
        }],
      }],
    }), workflowRunContext(), silentCore(calls), 42);

    assert.equal(existing.has(key), false);
  });

  it("reuses an unchanged unresolved inline discussion without reporting it as updated", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [], reviewComments: [] };
    const facts = inlineFacts();
    const finding = inlineFinding();
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [finding],
        warnings: [],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify(facts), "utf8");

      await publish({
        github: fakeGithub(calls, {
          files: changedSkillFilePatch(),
          reviewThreads: [{
            isResolved: false,
            comments: [{
              databaseId: 1001,
              body: inlineCommentBody(finding, facts, { path: "skills/lark-doc/SKILL.md", line: 30 }),
              path: "skills/lark-doc/SKILL.md",
              line: 30,
            }],
          }],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.reviewComments.length, 0);
      assert.equal(calls.updatedReviewComments?.length || 0, 0);
      assert.match(calls.comments[0].body, /Inline: semantic review reused existing unresolved discussion/);
      assert.doesNotMatch(calls.comments[0].body, /updated existing unresolved discussion/);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("prefers an unresolved existing thread when the same marker also has a resolved thread", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [], reviewComments: [] };
    const facts = {
      skills: [{
        source_file: "skills/lark-doc/SKILL.md",
        line: 30,
        command_path: "docs +fetch",
      }],
    };
    const finding = {
      category: "skill_quality",
      severity: "major",
      review_action: "must_fix",
      evidence: ["facts.skills[0]"],
      fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
      message: "skill references an invalid command",
      suggested_action: "update the command reference",
    };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [finding],
        warnings: [],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify(facts), "utf8");

      await publish({
        github: fakeGithub(calls, {
          files: [{
            filename: "skills/lark-doc/SKILL.md",
            patch: "@@ -29,0 +30,1 @@\n+bad command reference",
          }],
          reviewThreads: [
            {
              isResolved: true,
              comments: [{
                databaseId: 1001,
                body: `<!-- lark-cli-semantic-finding:${findingKey(finding, facts)} -->\nold resolved body`,
                path: "skills/lark-doc/SKILL.md",
                line: 30,
              }],
            },
            {
              isResolved: false,
              comments: [{
                databaseId: 1002,
                body: `<!-- lark-cli-semantic-finding:${findingKey(finding, facts)} -->\nnew unresolved body`,
                path: "skills/lark-doc/SKILL.md",
                line: 30,
              }],
            },
          ],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.reviewComments.length, 0);
      assert.equal(calls.updatedReviewComments.length, 1);
      assert.equal(calls.updatedReviewComments[0].comment_id, 1002);
      assert.match(calls.comments[0].body, /Inline: semantic review updated existing unresolved discussion/);
      assert.doesNotMatch(calls.comments[0].body, /Inline: semantic review existing resolved discussion/);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("reads paginated review threads before deciding whether to publish inline comments", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [], reviewComments: [] };
    const facts = {
      skills: [{
        source_file: "skills/lark-doc/SKILL.md",
        line: 30,
        command_path: "docs +fetch",
      }],
    };
    const finding = {
      category: "skill_quality",
      severity: "major",
      review_action: "must_fix",
      evidence: ["facts.skills[0]"],
      fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
      message: "skill references an invalid command",
      suggested_action: "update the command reference",
    };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [finding],
        warnings: [],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify(facts), "utf8");

      await publish({
        github: fakeGithub(calls, {
          files: [{
            filename: "skills/lark-doc/SKILL.md",
            patch: "@@ -29,0 +30,1 @@\n+bad command reference",
          }],
          reviewThreadPages: [
            {
              hasNextPage: true,
              endCursor: "second-page",
              nodes: [],
            },
            {
              hasNextPage: false,
              endCursor: null,
              nodes: [{
                isResolved: true,
                comments: [{
                  databaseId: 1001,
                  body: `<!-- lark-cli-semantic-finding:${findingKey(finding, facts)} -->\nold body`,
                  path: "skills/lark-doc/SKILL.md",
                  line: 30,
                }],
              }],
            },
          ],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.reviewComments.length, 1);
      assert.match(calls.comments[0].body, /Inline: semantic review posted to `skills\/lark-doc\/SKILL\.md:30`/);
      assert.doesNotMatch(calls.comments[0].body, /existing resolved discussion/);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("falls back to REST review comments and preserves unknown resolution when updating", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [], reviewComments: [] };
    const facts = {
      skills: [{
        source_file: "skills/lark-doc/SKILL.md",
        line: 30,
        command_path: "docs +fetch",
      }],
    };
    const finding = {
      category: "skill_quality",
      severity: "major",
      review_action: "must_fix",
      evidence: ["facts.skills[0]"],
      fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
      message: "skill references an invalid command",
      suggested_action: "update the command reference",
    };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [finding],
        warnings: [],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify(facts), "utf8");

      await publish({
        github: fakeGithub(calls, {
          failGraphql: true,
          files: [{
            filename: "skills/lark-doc/SKILL.md",
            patch: "@@ -29,0 +30,1 @@\n+bad command reference",
          }],
          reviewComments: [{
            id: 1003,
            body: `<!-- lark-cli-semantic-finding:${findingKey(finding, facts)} -->\nold body`,
            path: "skills/lark-doc/SKILL.md",
            line: 30,
          }],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.reviewComments.length, 0);
      assert.equal(calls.updatedReviewComments.length, 1);
      assert.equal(calls.updatedReviewComments[0].comment_id, 1003);
      assert.match(calls.comments[0].body, /Inline: semantic review updated existing discussion with unknown resolution/);
      assert.doesNotMatch(calls.comments[0].body, /updated existing unresolved discussion/);
      assert.match(calls.warnings[0], /thread state was not read/);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("posts a new inline comment when only a resolved discussion has the same finding marker", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [], reviewComments: [] };
    const facts = {
      skills: [{
        source_file: "skills/lark-doc/SKILL.md",
        line: 30,
        command_path: "docs +fetch",
      }],
    };
    const finding = {
      category: "skill_quality",
      severity: "major",
      review_action: "must_fix",
      evidence: ["facts.skills[0]"],
      fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
      message: "skill references an invalid command",
      suggested_action: "update the command reference",
    };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [finding],
        warnings: [],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify(facts), "utf8");

      await publish({
        github: fakeGithub(calls, {
          files: [{
            filename: "skills/lark-doc/SKILL.md",
            patch: "@@ -29,0 +30,1 @@\n+bad command reference",
          }],
          reviewThreads: [{
            isResolved: true,
            comments: [{
              databaseId: 1001,
              body: `<!-- lark-cli-semantic-finding:${findingKey(finding, facts)} -->\nold body`,
              path: "skills/lark-doc/SKILL.md",
              line: 30,
            }],
          }],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.reviewComments.length, 1);
      assert.equal(calls.checks[0].conclusion, "failure");
      assert.match(calls.comments[0].body, /Inline: semantic review posted to `skills\/lark-doc\/SKILL\.md:30`/);
      assert.doesNotMatch(calls.comments[0].body, /existing resolved discussion/);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("does not duplicate inline comments when fact indexes change but evidence location is the same", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [], reviewComments: [] };
    const oldFacts = {
      skills: [{
        source_file: "skills/lark-doc/SKILL.md",
        line: 30,
        command_path: "docs +fetch",
      }],
    };
    const newFacts = {
      skills: [
        {
          source_file: "skills/lark-im/SKILL.md",
          line: 12,
          command_path: "im +fetch",
        },
        {
          source_file: "skills/lark-doc/SKILL.md",
          line: 30,
          command_path: "docs +fetch",
        },
      ],
    };
    const oldFinding = {
      category: "skill_quality",
      severity: "major",
      review_action: "must_fix",
      evidence: ["facts.skills[0]"],
      fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
      message: "skill references an invalid command",
      suggested_action: "update the command reference",
    };
    const newFinding = {
      ...oldFinding,
      evidence: ["facts.skills[1]"],
      message: "invalid command reference in skill",
      suggested_action: "fix the referenced command",
    };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [newFinding],
        warnings: [],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify(newFacts), "utf8");

      await publish({
        github: fakeGithub(calls, {
          files: [{
            filename: "skills/lark-doc/SKILL.md",
            patch: "@@ -29,0 +30,1 @@\n+bad command reference",
          }],
          reviewThreads: [{
            isResolved: true,
            comments: [{
              databaseId: 1001,
              body: `<!-- lark-cli-semantic-finding:${findingKey(oldFinding, oldFacts)} -->\nold body`,
              path: "skills/lark-doc/SKILL.md",
              line: 30,
            }],
          }],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.reviewComments.length, 1);
      assert.match(calls.comments[0].body, /Inline: semantic review posted to `skills\/lark-doc\/SKILL\.md:30`/);
      assert.doesNotMatch(calls.comments[0].body, /existing resolved discussion/);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("updates the semantic check when summary cleanup cannot complete", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [] };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: false,
      }), "utf8");

      await publish({
        github: fakeGithub(calls, { failComments: true }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks.length, 1);
      assert.equal(calls.checks[0].name, "semantic-review/observe");
      assert.equal(calls.checks[0].conclusion, "success");
      assert.equal(calls.checkUpdates.length, 1);
      assert.equal(calls.checkUpdates[0].conclusion, "failure");
      assert.match(calls.checkUpdates[0].output.summary, /PR Quality Summary publication failed/);
      assert.equal(calls.comments.length, 0);
      assert.equal(calls.warnings.length, 1);
      assert.match(calls.warnings[0], /semantic review summary comment was not published or cleaned up/);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("updates the semantic check when a required summary cannot be published", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], checkUpdates: [], order: [], warnings: [], reviewComments: [] };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [],
        warnings: [{
          category: "skill_quality",
          severity: "major",
          review_action: "confirm",
          evidence: ["facts.skills[0]"],
          fingerprint: "confirm-required-summary",
          message: "exception needs confirmation",
          suggested_action: "confirm the exception still applies",
        }],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify({
        skills: [{ source_file: "skills/lark-doc/SKILL.md", line: 30 }],
      }), "utf8");

      await publish({
        github: fakeGithub(calls, { failComments: true }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks.length, 1);
      assert.equal(calls.checks[0].conclusion, "success");
      assert.equal(calls.checkUpdates.length, 1);
      assert.equal(calls.checkUpdates[0].conclusion, "failure");
      assert.match(calls.checkUpdates[0].output.summary, /PR Quality Summary publication failed/);
      assert.match(calls.warnings[0], /semantic review summary comment was not published or cleaned up/);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });

  it("still publishes check and summary when inline comments fail", async () => {
    const env = saveEnv();
    const cwd = process.cwd();
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
    const calls = { comments: [], checks: [], order: [], warnings: [], reviewComments: [] };
    try {
      process.chdir(dir);
      process.env.SEMANTIC_REVIEW_BLOCK = "true";
      process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
      process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
      process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
      fs.writeFileSync("decision.json", JSON.stringify({
        block_mode: true,
        blockers: [{
          category: "skill_quality",
          severity: "major",
          review_action: "must_fix",
          evidence: ["facts.skills[0]"],
          fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
          message: "skill references an invalid command",
          suggested_action: "update the command reference",
        }],
        warnings: [],
      }), "utf8");
      fs.writeFileSync("facts.json", JSON.stringify({
        skills: [{
          source_file: "skills/lark-doc/SKILL.md",
          line: 30,
          command_path: "docs +fetch",
        }],
      }), "utf8");

      await publish({
        github: fakeGithub(calls, {
          failReviewComments: true,
          files: [{
            filename: "skills/lark-doc/SKILL.md",
            patch: "@@ -29,0 +30,1 @@\n+bad command reference",
          }],
        }),
        context: workflowRunContext(),
        core: silentCore(calls),
      });

      assert.equal(calls.checks.length, 1);
      assert.equal(calls.comments.length, 1);
      assert.equal(calls.reviewComments.length, 0);
      assert.equal(calls.warnings.length, 1);
      assert.match(calls.warnings[0], /inline semantic review comment was not published/);
    } finally {
      process.chdir(cwd);
      restoreEnv(env);
    }
  });
});

function saveEnv() {
  return {
    SEMANTIC_REVIEW_BLOCK: process.env.SEMANTIC_REVIEW_BLOCK,
    SEMANTIC_REVIEW_PR_NUMBER: process.env.SEMANTIC_REVIEW_PR_NUMBER,
    SEMANTIC_REVIEW_HEAD_SHA: process.env.SEMANTIC_REVIEW_HEAD_SHA,
    SEMANTIC_REVIEW_BASE_SHA: process.env.SEMANTIC_REVIEW_BASE_SHA,
    SEMANTIC_REVIEW_RUN_ID: process.env.SEMANTIC_REVIEW_RUN_ID,
  };
}

async function withPublishTempDir(fn) {
  const env = saveEnv();
  const cwd = process.cwd();
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-publish-"));
  const calls = { comments: [], checks: [], order: [], warnings: [], reviewComments: [] };
  try {
    process.chdir(dir);
    process.env.SEMANTIC_REVIEW_BLOCK = "true";
    process.env.SEMANTIC_REVIEW_PR_NUMBER = "42";
    process.env.SEMANTIC_REVIEW_HEAD_SHA = "0123456789abcdef0123456789abcdef01234567";
    process.env.SEMANTIC_REVIEW_BASE_SHA = "fedcba9876543210fedcba9876543210fedcba98";
    process.env.SEMANTIC_REVIEW_RUN_ID = "123456";
    await fn({ calls, dir });
  } finally {
    process.chdir(cwd);
    restoreEnv(env);
  }
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

function workflowRunContext() {
  return {
    repo: { owner: "larksuite", repo: "cli" },
    payload: {
      repository: {
        id: 123,
      },
      workflow_run: {
        event: "pull_request",
        conclusion: "success",
        head_sha: "ffffffffffffffffffffffffffffffffffffffff",
        pull_requests: [{ number: 7 }],
      },
    },
  };
}

function silentCore(calls = { warnings: [] }) {
  return {
    notice(message) {
      calls.notices ||= [];
      calls.notices.push(message);
    },
    warning(message) {
      calls.warnings.push(message);
    },
  };
}

function inlineFacts() {
  return {
    schema_version: 1,
    skills: [{
      source_file: "skills/lark-doc/SKILL.md",
      line: 30,
      raw: "lark-cli bad",
      command_path: "docs +fetch",
      references_invalid_command: true,
    }],
  };
}

function inlineFinding() {
  return {
    category: "skill_quality",
    severity: "major",
    evidence: ["facts.skills[0]"],
    review_action: "must_fix",
    fingerprint: "category:skill_quality|skills:source_file:skills/lark-doc/SKILL.md:line:30",
    message: "skill references an invalid command",
    suggested_action: "update the command reference",
  };
}

function changedSkillFilePatch() {
  return [{
    filename: "skills/lark-doc/SKILL.md",
    patch: "@@ -29,0 +30,1 @@\n+bad command reference",
  }];
}

function currentTarget() {
  return {
    head: { sha: process.env.SEMANTIC_REVIEW_HEAD_SHA },
    base: { sha: process.env.SEMANTIC_REVIEW_BASE_SHA, repo: { id: 123 } },
  };
}

function writeInlineCandidateDecisionAndFacts() {
  fs.writeFileSync("decision.json", JSON.stringify({
    block_mode: true,
    blockers: [inlineFinding()],
    warnings: [],
  }), "utf8");
  fs.writeFileSync("facts.json", JSON.stringify(inlineFacts()), "utf8");
}

function writeDecisionAndFactsWithoutInline() {
  fs.writeFileSync("decision.json", JSON.stringify({
    block_mode: true,
    blockers: [{
      category: "error_hint",
      severity: "major",
      evidence: ["facts.errors[0]"],
      review_action: "must_fix",
      fingerprint: "error-hint",
      message: "error is missing a recovery hint",
      suggested_action: "add a structured hint",
    }],
    warnings: [],
  }), "utf8");
  fs.writeFileSync("facts.json", JSON.stringify({
    schema_version: 1,
    errors: [{ file: "cmd/foo.go", line: 10, changed: true, boundary: true, required_hint: true, hint_action_count: 0 }],
  }), "utf8");
}

function existingUnresolvedFindingThread() {
  const facts = inlineFacts();
  const finding = inlineFinding();
  return {
    isResolved: false,
    comments: [{
      author: { __typename: "Bot", login: "github-actions[bot]" },
      databaseId: 1001,
      body: `<!-- lark-cli-semantic-finding:${findingKey(finding, facts)} -->\nold body`,
      path: "skills/lark-doc/SKILL.md",
      line: 30,
    }],
  };
}

function fakeGithub(calls, options = {}) {
  let graphqlPage = 0;
  let pullGetCount = 0;
  const api = {
    paginate: async (endpoint) => {
      if (options.failComments) {
        throw new Error("comment API unavailable");
      }
      if (endpoint === api.rest.issues.listComments) {
        return options.issueComments || [];
      }
      if (endpoint === api.rest.pulls.listFiles) {
        if (options.failListFiles) {
          throw new Error("list files unavailable");
        }
        return options.files || [];
      }
      if (endpoint === api.rest.pulls.listReviewComments) {
        return (options.reviewComments || []).map((comment) => ({
          user: { type: "Bot", login: "github-actions[bot]" },
          ...comment,
        }));
      }
      return [];
    },
    graphql: async () => {
      if (options.failGraphql) {
        throw new Error("GraphQL unavailable");
      }
      const pages = options.reviewThreadPages || [{
        hasNextPage: false,
        endCursor: null,
        nodes: options.reviewThreads || [],
      }];
      const page = pages[Math.min(graphqlPage, pages.length - 1)];
      graphqlPage++;
      return {
        repository: {
          pullRequest: {
            reviewThreads: {
              pageInfo: { hasNextPage: !!page.hasNextPage, endCursor: page.endCursor || null },
              nodes: (page.nodes || []).map((thread) => ({
                id: thread.id || "thread-id",
                isResolved: !!thread.isResolved,
                comments: {
                  nodes: (thread.comments || []).map((comment) => ({
                    author: { __typename: "Bot", login: "github-actions[bot]" },
                    ...comment,
                  })),
                },
              })),
            },
          },
        },
      };
    },
    rest: {
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
          calls.deletedComments ||= [];
          calls.deletedComments.push(args);
          calls.order.push("comment-delete");
        },
      },
      checks: {
        create: async (args) => {
          calls.checks.push(args);
          calls.order.push("check");
          return { data: { id: calls.checks.length } };
        },
        update: async (args) => {
          calls.checkUpdates ||= [];
          calls.checkUpdates.push(args);
          calls.order.push("check-update");
        },
      },
      pulls: {
        get: async () => {
          const pull = Array.isArray(options.currentPullRequests)
            ? options.currentPullRequests[Math.min(pullGetCount++, options.currentPullRequests.length - 1)]
            : options.currentPullRequest || {
            head: { sha: process.env.SEMANTIC_REVIEW_HEAD_SHA },
            base: {
              sha: process.env.SEMANTIC_REVIEW_BASE_SHA,
              repo: { id: 123 },
            },
          };
          return { data: { state: "open", ...pull } };
        },
        listFiles() {},
        listReviewComments() {},
        createReviewComment: async (args) => {
          if (options.failReviewComments) {
            throw new Error("review comment API unavailable");
          }
          calls.reviewComments.push(args);
          calls.order.push("review-comment");
        },
        updateReviewComment: async (args) => {
          if (options.failReviewComments) {
            throw new Error("review comment API unavailable");
          }
          calls.updatedReviewComments ||= [];
          calls.updatedReviewComments.push(args);
          calls.order.push("review-comment-update");
        },
      },
    },
  };
  return api;
}
