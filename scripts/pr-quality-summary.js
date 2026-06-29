// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const SUMMARY_MARKER_PREFIX = "<!-- lark-cli-pr-quality-summary";
const LEGACY_SUMMARY_MARKER_PREFIXES = [
  "<!-- lark-cli-semantic-review",
];

function sanitizeMarkdownBody(text) {
  return String(text || "")
    .replace(/[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]/g, "")
    .replace(/[\r\n\t]+/g, " ")
    .replace(/@/g, "@\u200b")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/\\/g, "\\\\")
    .replace(/`/g, "\\`")
    .replace(/\*/g, "\\*")
    .replace(/_/g, "\\_")
    .replace(/#/g, "\\#")
    .replace(/\|/g, "\\|")
    .replace(/!/g, "\\!")
    .replace(/\[/g, "\\[")
    .replace(/\]/g, "\\]")
    .replace(/\(/g, "\\(")
    .replace(/\)/g, "\\)")
    .replace(/\bhttps:\/\//g, "https[:]//")
    .replace(/\bhttp:\/\//g, "http[:]//")
    .split(/\s+/)
    .filter(Boolean)
    .join(" ");
}

function markdownText(value) {
  return sanitizeMarkdownBody(String(value || ""));
}

function inlineCodeText(value) {
  return String(value || "")
    .replace(/[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]/g, "")
    .replace(/[\r\n\t]+/g, " ")
    .split(/\s+/)
    .filter(Boolean)
    .join(" ");
}

function inlineCode(value) {
  const text = inlineCodeText(value);
  const runs = text.match(/`+/g) || [];
  const fence = "`".repeat(runs.reduce((max, run) => Math.max(max, run.length), 0) + 1);
  const body = text.startsWith("`") || text.endsWith("`") ? ` ${text} ` : text;
  return fence + body + fence;
}

function summaryMarker(target = {}) {
  return `${SUMMARY_MARKER_PREFIX} head=${target.headSha || ""} base=${target.baseSha || ""} run=${target.runId || ""} -->`;
}

function parseSummaryMarker(body) {
  const match = /<!--\s*lark-cli-(?:pr-quality-summary|semantic-review)\s+([^>]*)-->/.exec(String(body || ""));
  if (!match) {
    return {};
  }
  const metadata = {};
  for (const part of match[1].trim().split(/\s+/)) {
    const attr = /^([A-Za-z0-9_-]+)=([^ ]*)$/.exec(part);
    if (attr) {
      metadata[attr[1]] = attr[2];
    }
  }
  return metadata;
}

function markerRunNumber(value) {
  const run = Number(String(value || "").trim());
  return Number.isInteger(run) && run > 0 ? run : 0;
}

function summaryCommentRunNumber(comment) {
  return markerRunNumber(parseSummaryMarker(comment?.body).run);
}

function targetRunNumber(target) {
  return markerRunNumber(target?.runId);
}

function hasNewerSummaryComment(comments, target) {
  const currentRun = targetRunNumber(target);
  return qualitySummaryComments(comments)
    .some((comment) => summaryCommentRunNumber(comment) > currentRun);
}

function isBotComment(comment) {
  return !!(comment && comment.user && comment.user.type === "Bot");
}

function hasQualitySummaryMarker(body) {
  const text = String(body || "");
  return text.includes(SUMMARY_MARKER_PREFIX) ||
    LEGACY_SUMMARY_MARKER_PREFIXES.some((prefix) => text.includes(prefix));
}

function qualitySummaryComments(comments) {
  return (Array.isArray(comments) ? comments : [])
    .filter((comment) => isBotComment(comment) && hasQualitySummaryMarker(comment.body));
}

function findQualitySummaryComment(comments) {
  return qualitySummaryComments(comments)[0] || null;
}

function finalSummaryBody(target, markdown) {
  return `${summaryMarker(target)}\n${String(markdown || "")}`.slice(0, 60000);
}

async function listIssueComments(github, context, pr) {
  return github.paginate(github.rest.issues.listComments, {
    owner: context.repo.owner,
    repo: context.repo.repo,
    issue_number: pr,
    per_page: 100,
  });
}

async function publishQualitySummary({ github, context, pr, target, markdown, beforeWrite }) {
  const body = finalSummaryBody(target, markdown);
  const comments = await listIssueComments(github, context, pr);
  const summaries = qualitySummaryComments(comments);
  if (hasNewerSummaryComment(summaries, target)) {
    return { action: "skipped-newer-summary" };
  }
  const existing = summaries[0] || null;
  if (beforeWrite && !(await beforeWrite(existing ? "update" : "creation"))) {
    return { action: "skipped" };
  }
  for (const duplicate of summaries.slice(1)) {
    await github.rest.issues.deleteComment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      comment_id: duplicate.id,
    });
  }
  if (existing) {
    await github.rest.issues.updateComment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      comment_id: existing.id,
      body,
    });
    return { action: "updated", commentId: existing.id, body };
  }
  await github.rest.issues.createComment({
    owner: context.repo.owner,
    repo: context.repo.repo,
    issue_number: pr,
    body,
  });
  return { action: "created", body };
}

async function deleteQualitySummaries({ github, context, pr, target, beforeWrite }) {
  const comments = await listIssueComments(github, context, pr);
  const existing = qualitySummaryComments(comments);
  if (hasNewerSummaryComment(existing, target)) {
    return { deleted: 0, skipped: true };
  }
  if (existing.length > 0 && beforeWrite && !(await beforeWrite("delete"))) {
    return { deleted: 0, skipped: true };
  }
  for (const comment of existing) {
    await github.rest.issues.deleteComment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      comment_id: comment.id,
    });
  }
  return { deleted: existing.length };
}

module.exports = {
  SUMMARY_MARKER_PREFIX,
  deleteQualitySummaries,
  finalSummaryBody,
  findQualitySummaryComment,
  hasQualitySummaryMarker,
  inlineCode,
  listIssueComments,
  markdownText,
  publishQualitySummary,
  qualitySummaryComments,
  sanitizeMarkdownBody,
  summaryMarker,
};
