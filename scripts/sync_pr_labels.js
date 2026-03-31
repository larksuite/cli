#!/usr/bin/env node
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const fs = require("node:fs/promises");
const path = require("node:path");

const API_BASE = "https://api.github.com";
const SCRIPT_DIR = __dirname;
const ROOT = path.join(SCRIPT_DIR, "..");

const LABEL_DEFINITIONS = {
  "size/S": { color: "77bb00", description: "Low-risk docs, CI, test, or chore only changes" },
  "size/M": { color: "eebb00", description: "Single-domain feat or fix with limited business impact" },
  "size/L": { color: "ff8800", description: "Large or sensitive change across domains or core paths" },
  "size/XL": { color: "ee0000", description: "Architecture-level or global-impact change" },
};

const MANAGED_LABELS = new Set(Object.keys(LABEL_DEFINITIONS));

const DOC_SUFFIXES = [".md", ".mdx", ".txt", ".rst"];
const LOW_RISK_PREFIXES = [".github/", "docs/", ".changeset/", "testdata/", "tests/", "skill-template/"];
const LOW_RISK_FILENAMES = new Set(["readme.md", "readme.zh.md", "changelog.md", "license", "cla.md"]);
const LOW_RISK_TEST_SUFFIXES = ["_test.go", ".snap"];

const CORE_PREFIXES = ["internal/auth/", "internal/engine/", "internal/config/", "cmd/"];
const HEAD_BUSINESS_DOMAINS = new Set(["im", "contact", "ccm", "base", "docx"]);
const LOW_RISK_TYPES = new Set(["docs", "ci", "test", "chore"]);

const CLASS_STANDARDS = {
  "size/S": {
    channel: "Fast track (S)",
    gates: [
      "Code quality: AI code review passed",
      "Dependency and configuration security checks passed",
    ],
  },
  "size/M": {
    channel: "Fast track (M)",
    gates: [
      "Code quality: AI code review passed",
      "Dependency and configuration security checks passed",
      "Skill format validation: added or modified Skills load successfully",
      "CLI automation tests: all required business-line tests passed",
    ],
  },
  "size/L": {
    channel: "Standard track (L)",
    gates: [
      "Code quality: AI code review passed",
      "Dependency and configuration security checks passed",
      "Skill format validation: added or modified Skills load successfully",
      "CLI automation tests: all required business-line tests passed",
      "Domain evaluation passed: reported success rate is greater than 95%",
    ],
  },
  "size/XL": {
    channel: "Strict track (XL)",
    gates: [
      "Code quality: AI code review passed",
      "Dependency and configuration security checks passed",
      "Skill format validation: added or modified Skills load successfully",
      "CLI automation tests: all required business-line tests passed",
      "Domain evaluation passed: reported success rate is greater than 95%",
      "Cross-domain release gate: all domains and full integration evaluations passed",
    ],
  },
};

function printHelp() {
  const lines = [
    "Usage:",
    "  node scripts/sync_pr_labels.js",
    "  node scripts/sync_pr_labels.js --dry-run --pr-url <github-pr-url> [--token <token>] [--json]",
    "  node scripts/sync_pr_labels.js --dry-run --repo <owner/name> --pr-number <number> [--token <token>] [--json]",
    "",
    "Modes:",
    "  default    Read the GitHub Actions event payload and apply labels",
    "  --dry-run  Fetch the PR, compute the managed label, and print the result without writing labels",
    "",
    "Options:",
    "  --pr-url <url>       GitHub pull request URL, for example https://github.com/larksuite/cli/pull/123",
    "  --repo <owner/name>  Repository name, used with --pr-number",
    "  --pr-number <n>      Pull request number, used with --repo",
    "  --token <token>      GitHub token override; falls back to GITHUB_TOKEN",
    "  --json               Print dry-run output as JSON instead of the default one-line summary",
    "  --help               Show this message",
  ];
  console.log(lines.join("\n"));
}

function log(message) {
  console.error(`sync-pr-labels: ${message}`);
}

function normalizePath(input) {
  return String(input || "").trim().toLowerCase();
}

function envValue(name) {
  return (process.env[name] || "").trim();
}

function envOrFail(name) {
  const value = envValue(name);
  if (!value) {
    throw new Error(`missing required environment variable: ${name}`);
  }
  return value;
}

function buildHeaders(token, hasBody = false) {
  const headers = {
    Accept: "application/vnd.github+json",
    "X-GitHub-Api-Version": "2022-11-28",
  };
  if (token) {
    headers.Authorization = `Bearer ${token}`;
  }
  if (hasBody) {
    headers["Content-Type"] = "application/json";
  }
  return headers;
}

function parseArgs(argv) {
  const options = {
    dryRun: false,
    json: false,
    help: false,
    prUrl: "",
    repo: "",
    prNumber: "",
    token: "",
  };

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (arg === "--dry-run") {
      options.dryRun = true;
    } else if (arg === "--json") {
      options.json = true;
    } else if (arg === "--help" || arg === "-h") {
      options.help = true;
    } else if (arg === "--pr-url") {
      options.prUrl = argv[i + 1] || "";
      i += 1;
    } else if (arg === "--repo") {
      options.repo = argv[i + 1] || "";
      i += 1;
    } else if (arg === "--pr-number") {
      options.prNumber = argv[i + 1] || "";
      i += 1;
    } else if (arg === "--token") {
      options.token = argv[i + 1] || "";
      i += 1;
    } else {
      throw new Error(`unknown argument: ${arg}`);
    }
  }

  return options;
}

function parsePrUrl(prUrl) {
  let parsed;
  try {
    parsed = new URL(prUrl);
  } catch {
    throw new Error(`invalid PR URL: ${prUrl}`);
  }

  const match = parsed.pathname.match(/^\/([^/]+)\/([^/]+)\/pull\/(\d+)\/?$/);
  if (!match) {
    throw new Error(`unsupported PR URL format: ${prUrl}`);
  }

  return {
    repo: `${match[1]}/${match[2]}`,
    prNumber: Number(match[3]),
  };
}

async function githubRequest(url, token, options = {}) {
  const { method = "GET", payload, allow404 = false } = options;
  const hasBody = payload !== undefined;
  const response = await fetch(url, {
    method,
    headers: buildHeaders(token, hasBody),
    body: hasBody ? JSON.stringify(payload) : undefined,
  });

  if (allow404 && response.status === 404) {
    return null;
  }

  if (!response.ok) {
    const detail = await response.text();
    throw new Error(`GitHub API ${method} ${url} failed: ${response.status} ${detail}`);
  }

  const text = await response.text();
  return text ? JSON.parse(text) : null;
}

async function loadEventPayload(filePath) {
  return JSON.parse(await fs.readFile(filePath, "utf8"));
}

async function getPullRequest(repo, prNumber, token) {
  const url = `${API_BASE}/repos/${repo}/pulls/${prNumber}`;
  return githubRequest(url, token);
}

async function listPrFiles(repo, prNumber, token) {
  const files = [];
  for (let page = 1; ; page += 1) {
    const params = new URLSearchParams({ per_page: "100", page: String(page) });
    const url = `${API_BASE}/repos/${repo}/pulls/${prNumber}/files?${params.toString()}`;
    const batch = await githubRequest(url, token);
    if (!batch || batch.length === 0) {
      break;
    }
    files.push(...batch);
    if (batch.length < 100) {
      break;
    }
  }
  return files;
}

async function listIssueLabels(repo, prNumber, token) {
  const url = `${API_BASE}/repos/${repo}/issues/${prNumber}/labels`;
  const labels = await githubRequest(url, token);
  return new Set(labels.map((item) => item.name));
}

async function syncLabelDefinition(repo, token, name) {
  const label = LABEL_DEFINITIONS[name];
  const createUrl = `${API_BASE}/repos/${repo}/labels`;
  const updateUrl = `${API_BASE}/repos/${repo}/labels/${encodeURIComponent(name)}`;

  try {
    await githubRequest(createUrl, token, {
      method: "POST",
      payload: {
        name,
        color: label.color,
        description: label.description,
      },
    });
    log(`created label ${name}`);
  } catch (error) {
    if (!String(error.message || error).includes(" 422 ")) {
      throw error;
    }
    await githubRequest(updateUrl, token, {
      method: "PATCH",
      payload: {
        new_name: name,
        color: label.color,
        description: label.description,
      },
    });
    log(`updated label ${name}`);
  }
}

async function addLabels(repo, prNumber, token, labels) {
  if (labels.length === 0) {
    return;
  }
  const url = `${API_BASE}/repos/${repo}/issues/${prNumber}/labels`;
  await githubRequest(url, token, {
    method: "POST",
    payload: { labels },
  });
  log(`added labels: ${labels.join(", ")}`);
}

async function removeLabel(repo, prNumber, token, name) {
  const url = `${API_BASE}/repos/${repo}/issues/${prNumber}/labels/${encodeURIComponent(name)}`;
  await githubRequest(url, token, { method: "DELETE", allow404: true });
  log(`removed label: ${name}`);
}

function parsePrType(title) {
  const match = String(title || "").trim().match(/^([a-z]+)(?:\([^)]+\))?!?:/i);
  return match ? match[1].toLowerCase() : "";
}

function isLowRiskPath(filePath) {
  const normalized = normalizePath(filePath);
  const basename = path.posix.basename(normalized);

  if (DOC_SUFFIXES.some((suffix) => normalized.endsWith(suffix))) {
    return true;
  }
  if (LOW_RISK_FILENAMES.has(basename)) {
    return true;
  }
  if (LOW_RISK_PREFIXES.some((prefix) => normalized.startsWith(prefix))) {
    return true;
  }
  if (LOW_RISK_TEST_SUFFIXES.some((suffix) => normalized.endsWith(suffix))) {
    return true;
  }
  return normalized.includes("/testdata/");
}

function isBusinessSkillPath(filePath) {
  const normalized = normalizePath(filePath);
  return normalized.startsWith("shortcuts/") || normalized.startsWith("skills/lark-");
}

function shortcutDomainForPath(filePath) {
  const parts = normalizePath(filePath).split("/");
  return parts.length >= 2 && parts[0] === "shortcuts" ? parts[1] : "";
}

function skillDomainForPath(filePath) {
  const parts = normalizePath(filePath).split("/");
  return parts.length >= 2 && parts[0] === "skills" && parts[1].startsWith("lark-")
    ? parts[1].slice("lark-".length)
    : "";
}

function getImportantArea(filePath) {
  const normalized = normalizePath(filePath);
  if (normalized.startsWith("shortcuts/")) return "shortcuts";
  if (normalized.startsWith("skills/") || normalized.startsWith("skill-template/")) return "skills";
  if (normalized.startsWith("cmd/")) return "cmd";
  return "";
}

async function detectNewShortcutDomain(files) {
  for (const item of files) {
    if (item.status !== "added") {
      continue;
    }
    const domain = shortcutDomainForPath(item.filename);
    if (!domain) {
      continue;
    }
    try {
      await fs.access(path.join(ROOT, "shortcuts", domain));
    } catch {
      return domain;
    }
  }
  return "";
}

function collectCoreAreas(filenames) {
  const areas = new Set();
  for (const name of filenames) {
    const normalized = normalizePath(name);
    if (normalized.startsWith("cmd/")) {
      areas.add("cmd");
    } else if (normalized.startsWith("internal/auth/")) {
      areas.add("internal/auth");
    } else if (normalized.startsWith("internal/engine/")) {
      areas.add("internal/engine");
    } else if (normalized.startsWith("internal/config/")) {
      areas.add("internal/config");
    }
  }
  return areas;
}

function collectSensitiveKeywords(filenames) {
  const pattern = /(^|\/)(auth|permission|permissions|security)(\/|_|\.|$)/;
  const hits = new Set();
  for (const name of filenames) {
    const normalized = normalizePath(name);
    const match = normalized.match(pattern);
    if (match && match[2]) {
      hits.add(match[2]);
    }
  }
  return [...hits].sort();
}

async function classifyPr(payload, files) {
  const pr = payload.pull_request;
  const title = pr.title || "";
  const prType = parsePrType(title);
  const filenames = files.map((item) => item.filename || "");
  // Filter out docs, tests, and other low-risk paths so the size label tracks business impact.
  const effectiveChanges = files.reduce(
    (sum, item) => sum + (isLowRiskPath(item.filename) ? 0 : (item.changes || 0)),
    0,
  );
  const totalChanges = files.reduce((sum, item) => sum + (item.changes || 0), 0);
  const domains = new Set();
  const importantAreas = new Set();

  for (const name of filenames) {
    const shortcutDomain = shortcutDomainForPath(name);
    if (shortcutDomain) {
      domains.add(shortcutDomain);
    }
    const skillDomain = skillDomainForPath(name);
    if (skillDomain) {
      domains.add(skillDomain);
    }
    
    const area = getImportantArea(name);
    if (area) {
      importantAreas.add(area);
    }
  }

  const coreAreas = collectCoreAreas(filenames);
  const newShortcutDomain = await detectNewShortcutDomain(files);
  const lowRiskOnly = filenames.length > 0 && filenames.every((name) => isLowRiskPath(name));
  const singleDomain = domains.size <= 1;
  const multiDomain = domains.size >= 2;
  const headDomains = [...domains].filter((domain) => HEAD_BUSINESS_DOMAINS.has(domain));
  const coreSignals = [...coreAreas].sort();
  const sensitiveKeywords = collectSensitiveKeywords(filenames);
  const sensitive = coreSignals.length > 0 || sensitiveKeywords.length > 0;

  const reasons = [];
  let label;

  if (lowRiskOnly && (LOW_RISK_TYPES.has(prType) || effectiveChanges === 0)) {
    reasons.push("Only low-risk docs, CI, test, or chore paths were changed, with no effective business code or Skill changes");
    label = "size/S";
  } else {
    // XL is reserved for architecture-level or global-impact changes.
    const architectureLevel =
      effectiveChanges > 1200
      || (prType === "refactor" && sensitive && effectiveChanges >= 300)
      || (coreAreas.size >= 2 && (multiDomain || effectiveChanges >= 300))
      || (headDomains.length >= 2 && sensitive);

    if (architectureLevel) {
      if (effectiveChanges > 1200) {
        reasons.push("Effective business code or Skill changes are far beyond the L threshold");
      }
      if (prType === "refactor" && sensitive && effectiveChanges >= 300) {
        reasons.push("Refactor PR touches core or sensitive paths");
      }
      if (coreAreas.size >= 2) {
        reasons.push("Touches multiple core areas at the same time");
      }
      if (headDomains.length >= 2) {
        reasons.push("Impacts multiple major business domains");
      }
      for (const signal of coreSignals) {
        reasons.push(`Core area hit: ${signal}`);
      }
      for (const keyword of sensitiveKeywords) {
        reasons.push(`Sensitive keyword hit: ${keyword}`);
      }
      label = "size/XL";
    } else if (
      prType === "refactor"
      || effectiveChanges >= 300
      || Boolean(newShortcutDomain)
      || multiDomain
      || sensitive
    ) {
      if (prType === "refactor") {
        reasons.push("PR type is refactor");
      }
      if (effectiveChanges >= 300) {
        reasons.push("Effective business code or Skill changes exceed 300 lines");
      }
      if (newShortcutDomain) {
        reasons.push(`Introduces a new business domain directory: shortcuts/${newShortcutDomain}/`);
      }
      if (multiDomain) {
        reasons.push("Touches multiple business domains");
      }
      for (const signal of coreSignals) {
        reasons.push(`Core area hit: ${signal}`);
      }
      for (const keyword of sensitiveKeywords) {
        reasons.push(`Sensitive keyword hit: ${keyword}`);
      }
      label = "size/L";
    } else {
      if (filenames.some((name) => isBusinessSkillPath(name)) || effectiveChanges > 0) {
        reasons.push("Regular feat, fix, or Skill change within a single business domain");
      }
      if (singleDomain && domains.size > 0) {
        reasons.push(`Impact is limited to a single business domain: ${[...domains].sort().join(", ")}`);
      }
      if (effectiveChanges < 300) {
        reasons.push("Effective business code or Skill changes are below 300 lines");
      }
      label = "size/M";
    }
  }

  return {
    label,
    title,
    prType: prType || "unknown",
    totalChanges,
    effectiveChanges,
    domains: [...domains].sort(),
    importantAreas: [...importantAreas].sort(),
    coreAreas: [...coreAreas].sort(),
    coreSignals,
    sensitiveKeywords,
    newShortcutDomain,
    reasons,
    lowRiskOnly,
    filenames,
  };
}

async function writeStepSummary(prNumber, classification) {
  const summaryPath = (process.env.GITHUB_STEP_SUMMARY || "").trim();
  if (!summaryPath) {
    return;
  }

  const standard = CLASS_STANDARDS[classification.label];
  const domains = classification.domains.join(", ") || "-";
  const areas = classification.importantAreas.join(", ") || "-";
  const coreAreas = classification.coreAreas.join(", ") || "-";
  const reasons = classification.reasons.length > 0
    ? classification.reasons
    : ["No higher-severity rule matched, so the PR defaults to medium classification"];

  const lines = [
    "## PR Size Classification",
    "",
    `- PR: #${prNumber}`,
    `- Label: \`${classification.label}\``,
    `- PR Type: \`${classification.prType}\``,
    `- Total Changes: \`${classification.totalChanges}\``,
    `- Effective Business/SKILL Changes: \`${classification.effectiveChanges}\``,
    `- Business Domains: \`${domains}\``,
    `- Impacted Areas: \`${areas}\``,
    `- Core Areas: \`${coreAreas}\``,
    `- CI/CD Channel: \`${standard.channel}\``,
    `- Low Risk Only: \`${classification.lowRiskOnly}\``,
    "",
    "### Reasons",
    "",
    ...reasons.map((reason) => `- ${reason}`),
    "",
    "### Pipeline Gates",
    "",
    ...standard.gates.map((gate) => `- ${gate}`),
    "",
  ];

  await fs.appendFile(summaryPath, `${lines.join("\n")}\n`, "utf8");
}

function formatDryRunResult(repo, prNumber, classification) {
  const standard = CLASS_STANDARDS[classification.label];
  return {
    repo,
    prNumber,
    label: classification.label,
    prType: classification.prType,
    totalChanges: classification.totalChanges,
    effectiveChanges: classification.effectiveChanges,
    lowRiskOnly: classification.lowRiskOnly,
    domains: classification.domains,
    importantAreas: classification.importantAreas,
    coreAreas: classification.coreAreas,
    coreSignals: classification.coreSignals,
    sensitiveKeywords: classification.sensitiveKeywords,
    reasons: classification.reasons,
    channel: standard.channel,
    gates: standard.gates,
  };
}

function printDryRunResult(result, options) {
  if (options.json) {
    console.log(JSON.stringify(result, null, 2));
    return;
  }

  const signalParts = [
    ...result.coreSignals.map((signal) => `core:${signal}`),
    ...result.sensitiveKeywords.map((keyword) => `keyword:${keyword}`),
    ...(result.domains.length > 0 ? [`domains:${result.domains.join(",")}`] : []),
    ...(result.importantAreas.length > 0 ? [`areas:${result.importantAreas.join(",")}`] : []),
  ];
  const reasonParts = result.reasons.length > 0
    ? result.reasons
    : ["No higher-severity rule matched, so the PR defaults to medium classification"];
  console.log(
    `${result.label} | #${result.prNumber} | type:${result.prType} | eff:${result.effectiveChanges} | `
      + `sig:${signalParts.join(";") || "-"} | reason:${reasonParts.join("; ")}`,
  );
}

async function resolveContext(options) {
  if (options.prUrl) {
    const { repo, prNumber } = parsePrUrl(options.prUrl);
    const payload = {
      repository: { full_name: repo },
      pull_request: await getPullRequest(repo, prNumber, options.token),
    };
    return { repo, prNumber, payload };
  }

  if (options.repo || options.prNumber) {
    if (!options.repo || !options.prNumber) {
      throw new Error("--repo and --pr-number must be provided together");
    }
    const prNumber = Number(options.prNumber);
    if (!Number.isInteger(prNumber) || prNumber <= 0) {
      throw new Error(`invalid PR number: ${options.prNumber}`);
    }
    const payload = {
      repository: { full_name: options.repo },
      pull_request: await getPullRequest(options.repo, prNumber, options.token),
    };
    return { repo: options.repo, prNumber, payload };
  }

  const eventPath = envOrFail("GITHUB_EVENT_PATH");
  const payload = await loadEventPayload(eventPath);
  return {
    repo: payload.repository.full_name,
    prNumber: payload.pull_request.number,
    payload,
  };
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (options.help) {
    printHelp();
    return;
  }

  options.token = options.token || envValue("GITHUB_TOKEN");
  const { repo, prNumber, payload } = await resolveContext(options);

  if (!options.dryRun && !options.token) {
    throw new Error("missing required GitHub token; set GITHUB_TOKEN or pass --token");
  }

  const files = await listPrFiles(repo, prNumber, options.token);
  const classification = await classifyPr(payload, files);

  if (options.dryRun) {
    printDryRunResult(formatDryRunResult(repo, prNumber, classification), options);
    return;
  }

  const desired = new Set([classification.label]);
  for (const area of classification.importantAreas) {
    desired.add(`area/${area}`);
  }

  const current = await listIssueLabels(repo, prNumber, options.token);

  const managedCurrent = [...current].filter((label) => MANAGED_LABELS.has(label) || label.startsWith("area/"));
  const toAdd = [...desired].filter((label) => !current.has(label)).sort();
  const toRemove = managedCurrent.filter((label) => !desired.has(label)).sort();

  for (const area of classification.importantAreas) {
    const labelName = `area/${area}`;
    if (!LABEL_DEFINITIONS[labelName]) {
      LABEL_DEFINITIONS[labelName] = {
        color: "1d76db",
        description: `PR touches the ${area} area`,
      };
    }
  }

  // Keep label metadata consistent even when labels already exist in the repository.
  for (const label of Object.keys(LABEL_DEFINITIONS)) {
    await syncLabelDefinition(repo, options.token, label);
  }

  await addLabels(repo, prNumber, options.token, toAdd);

  for (const label of toRemove) {
    await removeLabel(repo, prNumber, options.token, label);
  }

  await writeStepSummary(prNumber, classification);

  log(
    `pr #${prNumber} type=${classification.prType} total_changes=${classification.totalChanges} `
      + `effective_changes=${classification.effectiveChanges} files=${files.length} `
      + `desired=${[...desired].sort().join(",") || "-"} current_managed=${managedCurrent.sort().join(",") || "-"} `
      + `reasons=${classification.reasons.join(" | ") || "-"}`,
  );
}

main().catch((error) => {
  log(error.message || String(error));
  process.exit(1);
});
