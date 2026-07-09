// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const fs = require("fs");
const crypto = require("crypto");
const {
  deleteQualitySummaries,
  publishQualitySummary,
} = require("./pr-quality-summary.js");

function readText(path, fallback) {
  try {
    return fs.readFileSync(path, "utf8");
  } catch {
    return fallback;
  }
}

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

function parseBlockMode(value) {
  return value === "true";
}

function checkName(runtimeBlockMode) {
  return runtimeBlockMode ? "semantic-review/result" : "semantic-review/observe";
}

function infrastructureFailureDecision(message) {
  return {
    degraded: true,
    infrastructure_failure: true,
    system_warnings: [{
      severity: "critical",
      message,
      suggested_action: "inspect semantic-review workflow logs and quality-gate artifact",
    }],
    blockers: [],
    warnings: [],
  };
}

function validateDecisionShape(decision) {
  if (!decision || typeof decision !== "object" || Array.isArray(decision)) {
    return "semantic review decision must be an object";
  }
  if (typeof decision.block_mode !== "boolean") {
    return "semantic review decision block_mode must be boolean";
  }
  if ("degraded" in decision && typeof decision.degraded !== "boolean") {
    return "semantic review decision degraded must be boolean";
  }
  if ("skipped" in decision && typeof decision.skipped !== "boolean") {
    return "semantic review decision skipped must be boolean";
  }
  if ("infrastructure_failure" in decision && typeof decision.infrastructure_failure !== "boolean") {
    return "semantic review decision infrastructure_failure must be boolean";
  }
  if ("blockers" in decision && !Array.isArray(decision.blockers)) {
    return "semantic review decision blockers must be an array";
  }
  if ("warnings" in decision && !Array.isArray(decision.warnings)) {
    return "semantic review decision warnings must be an array";
  }
  if ("system_warnings" in decision && !Array.isArray(decision.system_warnings)) {
    return "semantic review decision system_warnings must be an array";
  }
  if (!("blockers" in decision)) {
    decision.blockers = [];
  }
  if (!("warnings" in decision)) {
    decision.warnings = [];
  }
  return "";
}

function loadDecision(path = "decision.json") {
  const raw = readText(path, "");
  if (!raw) {
    return infrastructureFailureDecision("semantic review decision is missing");
  }
  try {
    const decision = JSON.parse(raw);
    const shapeError = validateDecisionShape(decision);
    if (shapeError) {
      return infrastructureFailureDecision(shapeError);
    }
    return decision;
  } catch (err) {
    return infrastructureFailureDecision(`semantic review decision is invalid JSON: ${err.message}`);
  }
}

function checkConclusion(decision, runtimeBlockMode) {
  if (typeof decision.block_mode === "boolean" && decision.block_mode !== runtimeBlockMode) {
    return runtimeBlockMode ? "failure" : "neutral";
  }
  const systemWarnings = Array.isArray(decision?.system_warnings) ? decision.system_warnings : [];
  if (systemWarnings.length > 0) {
    return runtimeBlockMode ? "failure" : "neutral";
  }
  if (decision.infrastructure_failure) {
    return runtimeBlockMode ? "failure" : "neutral";
  }
  if (decision.skipped) {
    return runtimeBlockMode ? "failure" : "neutral";
  }
  if (decision.degraded) {
    return runtimeBlockMode ? "failure" : "neutral";
  }
  if (runtimeBlockMode && Array.isArray(decision.blockers) && decision.blockers.length > 0) {
    return "failure";
  }
  return "success";
}

function loadFacts(path = "facts.json") {
  const raw = readText(path, "");
  if (!raw) {
    return {};
  }
  try {
    const facts = JSON.parse(raw);
    if (!facts || typeof facts !== "object" || Array.isArray(facts)) {
      return {};
    }
    return facts;
  } catch {
    return {};
  }
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

function parseEvidenceRef(ref) {
  const match = /^facts\.(commands|skills|errors|outputs|public_content)\[(\d+)\]$/.exec(String(ref || ""));
  if (!match) {
    return null;
  }
  return { kind: match[1], index: Number(match[2]) };
}

function evidenceLocation(facts, ref) {
  const parsed = parseEvidenceRef(ref);
  if (!parsed) {
    return null;
  }
  const items = Array.isArray(facts?.[parsed.kind]) ? facts[parsed.kind] : [];
  const item = items[parsed.index];
  if (!item || typeof item !== "object") {
    return null;
  }
  switch (parsed.kind) {
    case "skills":
      if (item.source_file && Number.isInteger(item.line) && item.line > 0) {
        return {
          kind: parsed.kind,
          path: item.source_file,
          line: item.line,
          label: `${item.source_file}:${item.line}`,
        };
      }
      if (item.command_path) {
        return { kind: parsed.kind, command: item.command_path, label: item.command_path };
      }
      return null;
    case "errors":
      if (item.file && Number.isInteger(item.line) && item.line > 0) {
        return {
          kind: parsed.kind,
          path: item.file,
          line: item.line,
          label: `${item.file}:${item.line}`,
        };
      }
      if (item.command_path || item.command) {
        const command = item.command_path || item.command;
        return { kind: parsed.kind, command, label: command };
      }
      return null;
    case "outputs":
      if (item.command) {
        return { kind: parsed.kind, command: item.command, label: item.command };
      }
      return null;
    case "commands":
      if (item.path) {
        return { kind: parsed.kind, command: item.path, label: item.path };
      }
      return null;
    case "public_content":
      if (item.file && Number.isInteger(item.line) && item.line > 0) {
        const label = `${item.file}:${item.line}`;
        if (item.file === "branch" || item.file === "pull_request_metadata" || String(item.file).startsWith("commit:")) {
          return { kind: parsed.kind, label };
        }
        return {
          kind: parsed.kind,
          path: item.file,
          line: item.line,
          label,
        };
      }
      return null;
    default:
      return null;
  }
}

function resolveFindingEvidence(facts, finding) {
  const evidence = Array.isArray(finding?.evidence) ? finding.evidence : [];
  return evidence
    .map((ref) => evidenceLocation(facts, ref))
    .filter(Boolean);
}

function changedLinesFromPatch(patch) {
  const changed = new Set();
  if (typeof patch !== "string" || patch === "") {
    return changed;
  }
  let rightLine = 0;
  for (const line of patch.split("\n")) {
    const hunk = /^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@/.exec(line);
    if (hunk) {
      rightLine = Number(hunk[1]);
      continue;
    }
    if (rightLine <= 0 || line.startsWith("\\ No newline")) {
      continue;
    }
    if (line.startsWith("+") && !line.startsWith("+++")) {
      changed.add(rightLine);
      rightLine++;
      continue;
    }
    if (line.startsWith("-") && !line.startsWith("---")) {
      continue;
    }
    rightLine++;
  }
  return changed;
}

function buildChangedLineIndex(files) {
  const index = new Map();
  for (const file of Array.isArray(files) ? files : []) {
    if (!file || typeof file.filename !== "string") {
      continue;
    }
    index.set(file.filename, changedLinesFromPatch(file.patch || ""));
  }
  return index;
}

function selectInlineTarget(finding, facts, changedLineIndex) {
  const evidence = resolveFindingEvidence(facts, finding);
  for (const item of evidence) {
    if (!item.path || !Number.isInteger(item.line) || item.line <= 0) {
      continue;
    }
    const changed = changedLineIndex instanceof Map ? changedLineIndex.get(item.path) : null;
    if (changed && changed.has(item.line)) {
      return { path: item.path, line: item.line };
    }
  }
  return null;
}

function findingActionGroup(finding) {
  if (finding && typeof finding.review_action === "string") {
    switch (finding.review_action) {
      case "must_fix":
        return "must_fix";
      case "confirm":
        return "confirm";
      case "observe":
        return "observe";
      default:
        break;
    }
  }
  return "";
}

function findingStatusLabel(finding) {
  switch (findingActionGroup(finding)) {
    case "must_fix":
      return "Must fix";
    case "confirm":
      return "Confirm";
    case "observe":
      return "Observe";
    default:
      return "Review";
  }
}

function validatePublishFinding(finding, listName, index) {
  const location = `${listName}[${index}]`;
  const action = findingActionGroup(finding);
  if (!action) {
    return `${location} missing review_action`;
  }
  if (typeof finding?.fingerprint !== "string" || finding.fingerprint.trim() === "") {
    return `${location} missing fingerprint`;
  }
  if (listName === "blockers" && action !== "must_fix") {
    return `${location} review_action must be must_fix`;
  }
  if (listName === "warnings" && action === "must_fix") {
    return `${location} review_action must not be must_fix`;
  }
  return "";
}

function validateDecisionForPublish(decision) {
  const blockers = Array.isArray(decision?.blockers) ? decision.blockers : [];
  const warnings = Array.isArray(decision?.warnings) ? decision.warnings : [];
  for (let i = 0; i < blockers.length; i++) {
    const err = validatePublishFinding(blockers[i], "blockers", i);
    if (err) {
      return `semantic review decision finding ${err}`;
    }
  }
  for (let i = 0; i < warnings.length; i++) {
    const err = validatePublishFinding(warnings[i], "warnings", i);
    if (err) {
      return `semantic review decision finding ${err}`;
    }
  }
  return "";
}

function publishableDecision(decision, runtimeBlockMode) {
  const err = validateDecisionForPublish(decision);
  if (!err) {
    return decision;
  }
  const failed = infrastructureFailureDecision(err);
  failed.block_mode = runtimeBlockMode;
  return failed;
}

function findingLine(finding, facts, inlineState) {
  const evidence = resolveFindingEvidence(facts, finding);
  const evidenceText = evidence.length > 0
    ? evidence.map((item) => inlineCode(item.label)).join(", ")
    : "not mapped to a source location";
  const key = findingKey(finding, facts);
  const inline = inlineState instanceof Map && key ? inlineState.get(key) : null;
  const parts = [
    `**${markdownText(finding?.category || "finding")}**`,
    markdownText(finding?.message || ""),
  ];
  if (finding?.suggested_action) {
    parts.push(`Action: ${markdownText(finding.suggested_action)}`);
  }
  parts.push(`Evidence: ${evidenceText}`);
  if (finding?.waiver_id) {
    parts.push(`Exception: ${inlineCode(finding.waiver_id)}`);
  }
  if (inline?.label) {
    parts.push(`Inline: semantic review ${inline.label}`);
  }
  return `- ${parts.filter(Boolean).join(" — ")}`;
}

function appendFindingSection(lines, title, findings, facts, inlineState) {
  if (findings.length === 0) {
    return;
  }
  lines.push(`### ${title}`, "");
  for (const finding of findings) {
    lines.push(findingLine(finding, facts, inlineState));
  }
  lines.push("");
}

function buildSummaryMarkdown(decision, facts = {}, inlineState = new Map()) {
  const blockers = Array.isArray(decision?.blockers) ? decision.blockers : [];
  const warnings = Array.isArray(decision?.warnings) ? decision.warnings : [];
  const systemWarnings = Array.isArray(decision?.system_warnings) ? decision.system_warnings : [];
  const grouped = {
    must_fix: blockers.filter((finding) => findingActionGroup(finding) === "must_fix"),
    confirm: [],
    observe: [],
  };
  for (const finding of warnings) {
    const action = findingActionGroup(finding);
    if (action === "confirm") {
      grouped.confirm.push(finding);
    } else {
      grouped.observe.push(finding);
    }
  }
  const counts = findingActionCounts(decision);
  const lines = ["## PR Quality Summary", ""];
  if (counts.mustFix > 0) {
    lines.push("This PR has items that need changes before merge.", "");
  } else if (counts.confirm > 0) {
    lines.push("This PR has items that need confirmation. They do not block this PR.", "");
  } else if (counts.systemWarnings > 0 || decision?.infrastructure_failure || decision?.degraded || decision?.skipped) {
    lines.push("The semantic review system could not produce a fully trusted result. This is not reported as a code defect.", "");
  } else {
    lines.push("No action required.", "");
  }
  appendFindingSection(lines, "Must fix", grouped.must_fix, facts, inlineState);
  appendFindingSection(lines, "Confirm", grouped.confirm, facts, inlineState);
  if (systemWarnings.length > 0) {
    lines.push("### System status", "");
    for (const warning of systemWarnings) {
      const parts = [markdownText(warning?.message || "")];
      if (warning?.suggested_action) {
        parts.push(`Action: ${markdownText(warning.suggested_action)}`);
      }
      lines.push(`- ${parts.filter(Boolean).join(" — ")}`);
    }
    lines.push("");
  }
  if (counts.mustFix > 0) {
    lines.push("Resolving an inline discussion only closes the conversation. To change the check result, update the PR or land a recorded exception and rerun checks.");
  }
  return lines.join("\n");
}

function findingActionCounts(decision) {
  const blockers = Array.isArray(decision?.blockers) ? decision.blockers : [];
  const warnings = Array.isArray(decision?.warnings) ? decision.warnings : [];
  const systemWarnings = Array.isArray(decision?.system_warnings) ? decision.system_warnings : [];
  const counts = {
    mustFix: blockers.filter((finding) => findingActionGroup(finding) === "must_fix").length,
    confirm: 0,
    observe: 0,
    systemWarnings: systemWarnings.length,
  };
  for (const finding of warnings) {
    if (findingActionGroup(finding) === "confirm") {
      counts.confirm++;
    } else {
      counts.observe++;
    }
  }
  return counts;
}

function buildCheckSummary(decision, conclusion) {
  const options = arguments.length > 2 && arguments[2] ? arguments[2] : {};
  const counts = findingActionCounts(decision);
  const lines = [
    `Result: ${conclusion}.`,
    `Must fix: ${counts.mustFix}. Confirm: ${counts.confirm}. Observe: ${counts.observe}. System warnings: ${counts.systemWarnings}.`,
  ];
  if (options.summaryPublicationError) {
    lines.push(`PR Quality Summary publication failed: ${options.summaryPublicationError}.`);
  } else if (options.summaryRequired) {
    lines.push("See the PR Quality Summary for action-required findings. Observe-only findings are not published as PR comments.");
  } else {
    lines.push("No PR Quality Summary was published because there are no required actions.");
  }
  if (options.inlineFailureCount > 0) {
    lines.push(`Inline comment publication failures: ${options.inlineFailureCount}.`);
  }
  return lines.join("\n");
}

function hasSystemProblem(decision, runtimeBlockMode) {
  const systemWarnings = Array.isArray(decision?.system_warnings) ? decision.system_warnings : [];
  if (systemWarnings.length > 0 || decision?.infrastructure_failure || decision?.skipped || decision?.degraded) {
    return true;
  }
  return typeof decision?.block_mode === "boolean" && decision.block_mode !== runtimeBlockMode;
}

function semanticSummaryRequired(decision, runtimeBlockMode) {
  const counts = findingActionCounts(decision);
  if (hasSystemProblem(decision, runtimeBlockMode)) {
    return true;
  }
  if (counts.confirm > 0) {
    return true;
  }
  return runtimeBlockMode && counts.mustFix > 0;
}

function buildCheckTitle(decision, conclusion, runtimeBlockMode, options = {}) {
  if (options.summaryPublicationError) {
    return "Semantic review publication failure";
  }
  if (hasSystemProblem(decision, runtimeBlockMode)) {
    return "Semantic review system problem";
  }
  if (conclusion === "failure") {
    return "Semantic review blockers";
  }
  return "Semantic review";
}

function inlineFailureCount(inlineState) {
  if (!(inlineState instanceof Map)) {
    return 0;
  }
  let failures = 0;
  for (const state of inlineState.values()) {
    if (state && state.failed) {
      failures++;
    }
  }
  return failures;
}

function stableEvidenceIdentity(facts, ref) {
  const location = evidenceLocation(facts, ref);
  if (!location) {
    return `ref:${String(ref || "")}`;
  }
  if (location.path && Number.isInteger(location.line) && location.line > 0) {
    return `path:${location.path}:${location.line}`;
  }
  if (location.command) {
    return `command:${location.command}`;
  }
  return `label:${location.label || ""}`;
}

function stableFindingIdentity(finding, facts) {
  const fingerprint = String(finding?.fingerprint || "").trim();
  if (fingerprint !== "") {
    return `fingerprint:${fingerprint}`;
  }
  const evidence = Array.isArray(finding?.evidence) ? finding.evidence : [];
  return `evidence:${evidence.map((ref) => stableEvidenceIdentity(facts, ref)).sort().join("|")}`;
}

function findingKey(finding, facts = {}) {
  const payload = JSON.stringify({
    category: finding?.category || "",
    identity: stableFindingIdentity(finding, facts),
  });
  return crypto.createHash("sha1").update(payload).digest("hex").slice(0, 16);
}

function findingMarker(key) {
  return `<!-- lark-cli-semantic-finding:${key} -->`;
}

function markerKeyFromBody(body) {
  const match = /<!--\s*lark-cli-semantic-finding:([a-f0-9]{8,40})\s*-->/.exec(String(body || ""));
  return match ? match[1] : "";
}

function inlineCommentBody(finding, facts, target) {
  const key = findingKey(finding, facts);
  const evidence = resolveFindingEvidence(facts, finding);
  const evidenceText = evidence.length > 0
    ? evidence.map((item) => inlineCode(item.label)).join(", ")
    : "not mapped to a source location";
  return [
    findingMarker(key),
    `**Semantic Review: ${findingStatusLabel(finding)}**`,
    "",
    `**${markdownText(finding?.category || "finding")}**: ${markdownText(finding?.message || "")}`,
    "",
    `Status: ${findingStatusLabel(finding)}`,
    finding?.suggested_action ? `Action: ${markdownText(finding.suggested_action)}` : "",
    `Evidence: ${evidenceText}`,
    finding?.waiver_id ? `Exception: ${inlineCode(finding.waiver_id)}` : "",
    "",
    `This comment is anchored to ${inlineCode(`${target.path}:${target.line}`)}. Resolving this discussion does not change the failed check. Commit a fix or add an approved semantic-review waiver, then rerun CI.`,
  ].filter((line) => line !== "").join("\n");
}

function inlineCandidates(decision, runtimeBlockMode) {
  if (!runtimeBlockMode) {
    return [];
  }
  const blockers = Array.isArray(decision?.blockers) ? decision.blockers : [];
  return blockers.filter((finding) => findingActionGroup(finding) === "must_fix");
}

function threadStateFromComment(comment, isResolved) {
  const key = markerKeyFromBody(comment?.body);
  if (!key) {
    return null;
  }
  const path = comment?.path || "";
  const line = Number(comment?.line || 0);
  const location = path && line > 0 ? ` at ${inlineCode(`${path}:${line}`)}` : "";
  const resolutionKnown = arguments.length >= 2;
  const label = resolutionKnown
    ? `reused existing ${isResolved ? "resolved" : "unresolved"} discussion${location}`
    : `reused existing discussion with unknown resolution${location}`;
  return {
    key,
    commentId: Number(comment?.databaseId || comment?.id || 0),
    body: String(comment?.body || ""),
    path,
    line,
    location,
    label,
    resolutionKnown,
    resolved: !!isResolved,
  };
}

function isBotReviewComment(comment) {
  const restUser = comment?.user;
  if (restUser?.type === "Bot") {
    return true;
  }
  const graphqlAuthor = comment?.author;
  return graphqlAuthor?.__typename === "Bot";
}

async function loadExistingInlineThreads(github, context, core, pr) {
  const existing = new Map();
  if (typeof github.graphql === "function") {
    try {
      let cursor = null;
      for (;;) {
        const result = await github.graphql(`
          query($owner: String!, $repo: String!, $number: Int!, $cursor: String) {
            repository(owner: $owner, name: $repo) {
              pullRequest(number: $number) {
                reviewThreads(first: 100, after: $cursor) {
                  pageInfo { hasNextPage endCursor }
                  nodes {
                    id
                    isResolved
                    comments(first: 50) {
                      nodes {
                        databaseId
                        body
                        path
                        line
                        author {
                          __typename
                          login
                        }
                      }
                    }
                  }
                }
              }
            }
          }
        `, {
          owner: context.repo.owner,
          repo: context.repo.repo,
          number: pr,
          cursor,
        });
        const threads = result?.repository?.pullRequest?.reviewThreads;
        for (const thread of threads?.nodes || []) {
          for (const comment of thread?.comments?.nodes || []) {
            if (!isBotReviewComment(comment)) {
              continue;
            }
            const state = threadStateFromComment(comment, thread.isResolved);
            if (state && (!existing.has(state.key) || (existing.get(state.key).resolved && !state.resolved))) {
              existing.set(state.key, state);
            }
          }
        }
        if (!threads?.pageInfo?.hasNextPage) {
          break;
        }
        cursor = threads.pageInfo.endCursor;
      }
      return existing;
    } catch (err) {
      core.warning(`semantic review thread state was not read: ${err.message}`);
    }
  }

  try {
    const comments = await github.paginate(github.rest.pulls.listReviewComments, {
      owner: context.repo.owner,
      repo: context.repo.repo,
      pull_number: pr,
      per_page: 100,
    });
    for (const comment of comments) {
      if (!isBotReviewComment(comment)) {
        continue;
      }
      const state = threadStateFromComment(comment);
      if (state && (!existing.has(state.key) || (existing.get(state.key).resolved && !state.resolved))) {
        existing.set(state.key, state);
      }
    }
  } catch (err) {
    core.warning(`semantic review review comments were not listed: ${err.message}`);
  }
  return existing;
}

async function loadChangedLineIndex(github, context, pr) {
  const files = await github.paginate(github.rest.pulls.listFiles, {
    owner: context.repo.owner,
    repo: context.repo.repo,
    pull_number: pr,
    per_page: 100,
  });
  return buildChangedLineIndex(files);
}

async function publishInlineComments({ github, context, core, target: publishTarget, pr, headSha, decision, facts, runtimeBlockMode }) {
  const inlineState = new Map();
  const candidates = inlineCandidates(decision, runtimeBlockMode);
  if (candidates.length === 0) {
    return inlineState;
  }

  let changedLineIndex;
  try {
    changedLineIndex = await loadChangedLineIndex(github, context, pr);
  } catch (err) {
    core.warning(`semantic review PR files were not listed: ${err.message}`);
    for (const finding of candidates) {
      const key = findingKey(finding, facts);
      inlineState.set(key, { label: "summary-only; PR files were not listed" });
    }
    return inlineState;
  }

  const existing = await loadExistingInlineThreads(github, context, core, pr);
  for (const finding of candidates) {
    const key = findingKey(finding, facts);
    const current = existing.get(key);
    if (current && !current.resolved) {
      const inlineTarget = current.path && current.line > 0
        ? { path: current.path, line: current.line }
        : selectInlineTarget(finding, facts, changedLineIndex);
      const nextBody = inlineTarget ? inlineCommentBody(finding, facts, inlineTarget) : "";
      if (!current.resolved && current.commentId > 0 && nextBody && current.body !== nextBody) {
        try {
          if (!(await publishTargetStillCurrent(github, context, core, publishTarget, "inline comment"))) {
            return inlineState;
          }
          await github.rest.pulls.updateReviewComment({
            owner: context.repo.owner,
            repo: context.repo.repo,
            comment_id: current.commentId,
            body: nextBody,
          });
          current.body = nextBody;
          current.label = current.resolutionKnown
            ? `updated existing unresolved discussion${current.location || ""}`
            : `updated existing discussion with unknown resolution${current.location || ""}`;
        } catch (err) {
          core.warning(`inline semantic review comment was not updated: ${err.message}`);
          current.label = `${current.label}; update failed`;
          current.failed = true;
        }
      }
      inlineState.set(key, { label: current.label, resolved: current.resolved, failed: !!current.failed });
      continue;
    }
    const inlineTarget = selectInlineTarget(finding, facts, changedLineIndex);
    if (!inlineTarget) {
      const state = { label: "summary-only; no stable changed diff line" };
      inlineState.set(key, state);
      existing.set(key, state);
      continue;
    }
    try {
      if (!(await publishTargetStillCurrent(github, context, core, publishTarget, "inline comment"))) {
        return inlineState;
      }
      await github.rest.pulls.createReviewComment({
        owner: context.repo.owner,
        repo: context.repo.repo,
        pull_number: pr,
        commit_id: headSha,
        path: inlineTarget.path,
        line: inlineTarget.line,
        side: "RIGHT",
        body: inlineCommentBody(finding, facts, inlineTarget),
      });
      const state = { label: `posted to ${inlineCode(`${inlineTarget.path}:${inlineTarget.line}`)}` };
      inlineState.set(key, state);
      existing.set(key, state);
    } catch (err) {
      core.warning(`inline semantic review comment was not published: ${err.message}`);
      const state = { label: "inline comment failed; see workflow warning", failed: true };
      inlineState.set(key, state);
      existing.set(key, state);
    }
  }
  return inlineState;
}

function verifiedPublishTarget() {
  const pr = Number(process.env.SEMANTIC_REVIEW_PR_NUMBER || 0);
  if (!Number.isInteger(pr) || pr <= 0) {
    throw new Error("missing verified semantic review pull request number");
  }
  const headSha = process.env.SEMANTIC_REVIEW_HEAD_SHA || "";
  if (!/^[a-f0-9]{40}$/i.test(headSha)) {
    throw new Error("missing verified semantic review head sha");
  }
  const baseSha = process.env.SEMANTIC_REVIEW_BASE_SHA || "";
  if (!/^[a-f0-9]{40}$/i.test(baseSha)) {
    throw new Error("missing verified semantic review base sha");
  }
  const runId = process.env.SEMANTIC_REVIEW_RUN_ID || "";
  if (runId && !/^\d+$/.test(runId)) {
    throw new Error("invalid verified semantic review run id");
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
    core.notice(`semantic review skipped: PR is no longer open before ${phase}`);
    return false;
  }
  if (pr.head.sha !== target.headSha) {
    core.notice(`semantic review skipped: PR head changed before ${phase}`);
    return false;
  }
  if (pr.base.sha !== target.baseSha) {
    core.notice(`semantic review skipped: PR base changed before ${phase}`);
    return false;
  }
  if (pr.base.repo.id !== context.payload.repository.id) {
    throw new Error("PR base repo mismatch before publishing");
  }
  return true;
}

async function publish({ github, context, core }) {
  const run = context.payload.workflow_run;
  if (!run || run.event !== "pull_request" || run.conclusion !== "success") {
    core.notice("semantic review skipped: workflow_run is not a successful pull_request run");
    return;
  }
  const runtimeBlockMode = parseBlockMode(process.env.SEMANTIC_REVIEW_BLOCK || "");
  const target = verifiedPublishTarget();
  if (!(await publishTargetStillCurrent(github, context, core, target))) {
    return;
  }
  const { pr, headSha } = target;

  const decision = publishableDecision(loadDecision(), runtimeBlockMode);
  const facts = loadFacts();
  const inlineState = await publishInlineComments({ github, context, core, target, pr, headSha, decision, facts, runtimeBlockMode });
  const conclusion = checkConclusion(decision, runtimeBlockMode);
  const summaryRequired = semanticSummaryRequired(decision, runtimeBlockMode);
  const inlineFailures = inlineFailureCount(inlineState);
  let checkConclusionValue = conclusion;
  let summaryPublicationError = "";
  let checkRunId = 0;

  if (!(await publishTargetStillCurrent(github, context, core, target, "check creation"))) {
    return;
  }
  const check = await github.rest.checks.create({
    owner: context.repo.owner,
    repo: context.repo.repo,
    name: checkName(runtimeBlockMode),
    head_sha: headSha,
    status: "completed",
    conclusion: checkConclusionValue,
    output: {
      title: buildCheckTitle(decision, checkConclusionValue, runtimeBlockMode),
      summary: buildCheckSummary(decision, checkConclusionValue, {
        summaryRequired,
        inlineFailureCount: inlineFailures,
      }).slice(0, 65000),
    },
  });
  checkRunId = Number(check?.data?.id || 0);

  try {
    if (summaryRequired) {
      const body = buildSummaryMarkdown(decision, facts, inlineState);
      if (!(await publishTargetStillCurrent(github, context, core, target, "summary comment"))) {
        return;
      }
      await publishQualitySummary({
        github,
        context,
        pr,
        target,
        markdown: body,
        beforeWrite: (action) => publishTargetStillCurrent(github, context, core, target, `summary comment ${action}`),
      });
    } else {
      if (!(await publishTargetStillCurrent(github, context, core, target, "summary comment cleanup"))) {
        return;
      }
      await deleteQualitySummaries({
        github,
        context,
        pr,
        target,
        beforeWrite: (action) => publishTargetStillCurrent(github, context, core, target, `summary comment ${action}`),
      });
    }
  } catch (err) {
    summaryPublicationError = err.message;
    core.warning(`semantic review summary comment was not published or cleaned up: ${summaryPublicationError}`);
    if (checkRunId > 0) {
      checkConclusionValue = "failure";
      await github.rest.checks.update({
        owner: context.repo.owner,
        repo: context.repo.repo,
        check_run_id: checkRunId,
        conclusion: checkConclusionValue,
        output: {
          title: buildCheckTitle(decision, checkConclusionValue, runtimeBlockMode, { summaryPublicationError }),
          summary: buildCheckSummary(decision, checkConclusionValue, {
            summaryRequired,
            summaryPublicationError,
            inlineFailureCount: inlineFailures,
          }).slice(0, 65000),
        },
      });
    }
  }
}

module.exports = {
  buildCheckSummary,
  buildSummaryMarkdown,
  buildChangedLineIndex,
  buildCheckTitle,
  checkConclusion,
  checkName,
  changedLinesFromPatch,
  evidenceLocation,
  findingKey,
  inlineCode,
  inlineCommentBody,
  loadDecision,
  loadExistingInlineThreads,
  loadFacts,
  parseBlockMode,
  publish,
  publishInlineComments,
  resolveFindingEvidence,
  sanitizeMarkdownBody,
  selectInlineTarget,
  semanticSummaryRequired,
  verifiedPublishTarget,
};
