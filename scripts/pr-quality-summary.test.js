// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");

const {
  deleteQualitySummaries,
  finalSummaryBody,
  findQualitySummaryComment,
  hasQualitySummaryMarker,
  markdownText,
  publishQualitySummary,
  qualitySummaryComments,
  summaryMarker,
  inlineCode,
} = require("./pr-quality-summary.js");

describe("pr-quality-summary", () => {
  it("writes a current PR quality summary marker", () => {
    const marker = summaryMarker({
      headSha: "0123456789abcdef0123456789abcdef01234567",
      baseSha: "fedcba9876543210fedcba9876543210fedcba98",
      runId: "123",
    });

    assert.equal(
      marker,
      "<!-- lark-cli-pr-quality-summary head=0123456789abcdef0123456789abcdef01234567 base=fedcba9876543210fedcba9876543210fedcba98 run=123 -->",
    );
  });

  it("recognizes current and legacy bot summary comments", () => {
    const comments = [
      { id: 1, user: { type: "User" }, body: "<!-- lark-cli-pr-quality-summary head=a -->" },
      { id: 2, user: { type: "Bot" }, body: "plain comment" },
      { id: 3, user: { type: "Bot" }, body: "<!-- lark-cli-semantic-review head=old -->" },
      { id: 4, user: { type: "Bot" }, body: "<!-- lark-cli-pr-quality-summary head=new -->" },
    ];

    assert.equal(hasQualitySummaryMarker(comments[3].body), true);
    assert.deepEqual(qualitySummaryComments(comments).map((c) => c.id), [3, 4]);
    assert.equal(findQualitySummaryComment(comments).id, 3);
  });

  it("creates a summary when no existing marker is present", async () => {
    const calls = { comments: [], order: [] };
    await publishQualitySummary({
      github: fakeGithub(calls),
      context: context(),
      pr: 42,
      target: target(),
      markdown: "## PR Quality Summary\n\n- fix this",
    });

    assert.equal(calls.comments.length, 1);
    assert.equal(calls.comments[0].issue_number, 42);
    assert.match(calls.comments[0].body, /^<!-- lark-cli-pr-quality-summary /);
    assert.match(calls.comments[0].body, /## PR Quality Summary/);
  });

  it("updates a legacy summary instead of creating a second stable comment", async () => {
    const calls = { comments: [], order: [] };
    await publishQualitySummary({
      github: fakeGithub(calls, {
        issueComments: [{
          id: 99,
          user: { type: "Bot" },
          body: "<!-- lark-cli-semantic-review head=old base=old run=1 -->",
        }],
      }),
      context: context(),
      pr: 42,
      target: target(),
      markdown: "## PR Quality Summary\n\n- updated",
    });

    assert.equal(calls.comments.length, 1);
    assert.equal(calls.comments[0].comment_id, 99);
    assert.equal(calls.comments[0].issue_number, undefined);
    assert.match(calls.comments[0].body, /^<!-- lark-cli-pr-quality-summary /);
  });

  it("removes duplicate summary comments when publishing a new body", async () => {
    const calls = { comments: [], deletedComments: [], order: [] };
    await publishQualitySummary({
      github: fakeGithub(calls, {
        issueComments: [
          { id: 99, user: { type: "Bot" }, body: "<!-- lark-cli-semantic-review head=old -->" },
          { id: 100, user: { type: "Bot" }, body: "<!-- lark-cli-pr-quality-summary head=new -->" },
        ],
      }),
      context: context(),
      pr: 42,
      target: target(),
      markdown: "## PR Quality Summary\n\n- updated",
    });

    assert.equal(calls.comments.length, 1);
    assert.equal(calls.comments[0].comment_id, 99);
    assert.deepEqual(calls.deletedComments.map((c) => c.comment_id), [100]);
  });

  it("does not let an older run overwrite a newer summary", async () => {
    const calls = { comments: [], deletedComments: [], order: [] };
    const result = await publishQualitySummary({
      github: fakeGithub(calls, {
        issueComments: [{
          id: 99,
          user: { type: "Bot" },
          body: "<!-- lark-cli-pr-quality-summary head=0123456789abcdef0123456789abcdef01234567 base=fedcba9876543210fedcba9876543210fedcba98 run=123457 -->",
        }],
      }),
      context: context(),
      pr: 42,
      target: target(),
      markdown: "## PR Quality Summary\n\n- older",
    });

    assert.equal(result.action, "skipped-newer-summary");
    assert.equal(calls.comments.length, 0);
    assert.equal(calls.deletedComments.length, 0);
  });

  it("deletes all current and legacy summaries during clean no-action runs", async () => {
    const calls = { deletedComments: [] };
    const result = await deleteQualitySummaries({
      github: fakeGithub(calls, {
        issueComments: [
          { id: 10, user: { type: "Bot" }, body: "<!-- lark-cli-semantic-review head=old -->" },
          { id: 11, user: { type: "Bot" }, body: "<!-- lark-cli-pr-quality-summary head=new -->" },
          { id: 12, user: { type: "Bot" }, body: "unrelated" },
        ],
      }),
      context: context(),
      pr: 42,
      target: target(),
    });

    assert.equal(result.deleted, 2);
    assert.deepEqual(calls.deletedComments.map((c) => c.comment_id), [10, 11]);
  });

  it("does not let an older cleanup delete a newer summary", async () => {
    const calls = { deletedComments: [] };
    const result = await deleteQualitySummaries({
      github: fakeGithub(calls, {
        issueComments: [{
          id: 99,
          user: { type: "Bot" },
          body: "<!-- lark-cli-pr-quality-summary head=0123456789abcdef0123456789abcdef01234567 base=fedcba9876543210fedcba9876543210fedcba98 run=123457 -->",
        }],
      }),
      context: context(),
      pr: 42,
      target: target(),
    });

    assert.equal(result.skipped, true);
    assert.equal(calls.deletedComments.length, 0);
  });

  it("sanitizes model-controlled text for markdown summaries", () => {
    const got = markdownText("@team\n# forged [link](https://example.com)<b>");

    assert(!got.includes("@team"));
    assert(!got.includes("\n# forged"));
    assert(!got.includes("https://example.com"));
    assert(!got.includes("<b>"));
    assert(got.includes("@\u200bteam"));
    assert(got.includes("\\# forged"));
    assert(got.includes("https[:]//example.com"));
    assert(got.includes("&lt;b&gt;"));
  });

  it("keeps inline code labels on one markdown line", () => {
    const got = inlineCode("abc\n\n## INJECTED\n\n[x](http://evil)\t@team\u0001");

    assert.equal(got, "`abc ## INJECTED [x](http://evil) @team`");
    assert(!got.includes("\n"));
    assert(!got.includes("\t"));
    assert(!got.includes("\u0001"));
  });

  it("caps final summary body size", () => {
    const body = finalSummaryBody(target(), "x".repeat(70000));
    assert.equal(body.length, 60000);
    assert.match(body, /^<!-- lark-cli-pr-quality-summary /);
  });
});

function context() {
  return { repo: { owner: "larksuite", repo: "cli" } };
}

function target() {
  return {
    headSha: "0123456789abcdef0123456789abcdef01234567",
    baseSha: "fedcba9876543210fedcba9876543210fedcba98",
    runId: "123456",
  };
}

function fakeGithub(calls, options = {}) {
  const api = {
    paginate: async (endpoint) => {
      if (endpoint === api.rest.issues.listComments) {
        return options.issueComments || [];
      }
      return [];
    },
    rest: {
      issues: {
        listComments() {},
        createComment: async (args) => {
          calls.comments.push(args);
          calls.order?.push("comment");
        },
        updateComment: async (args) => {
          calls.comments.push(args);
          calls.order?.push("comment");
        },
        deleteComment: async (args) => {
          calls.deletedComments.push(args);
        },
      },
    },
  };
  return api;
}
