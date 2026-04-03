/*
 * Issue labeler for this repository.
 *
 * Implements only:
 * - Type labels (Section 2)
 * - Domain labels (Section 4)
 *
 * Notes:
 * - Type: only applied when keyword matched. If no match, keep current type labels unchanged.
 * - Domain: default is add-only; strict sync is optional via --sync-domains.
 */

const API_BASE = "https://api.github.com";

const TYPE_LABELS = [
  "bug",
  "enhancement",
  "question",
  "documentation",
  "performance",
  "security",
];
const TYPE_LABEL_SET = new Set(TYPE_LABELS);

const DOMAIN_SERVICES = [
  "im",
  "doc",
  "base",
  "sheets",
  "calendar",
  "mail",
  "task",
  "vc",
  "whiteboard",
  "minutes",
  "wiki",
  "event",
  "auth",
  "core",
];
const DOMAIN_LABELS = DOMAIN_SERVICES.map((s) => `domain/${s}`);
const DOMAIN_LABEL_SET = new Set(DOMAIN_LABELS);

const TYPE_TIE_BREAKER = [
  "security",
  "bug",
  "performance",
  "documentation",
  "enhancement",
  "question",
];

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function envValue(name) {
  const value = process.env[name];
  return value ? String(value).trim() : "";
}

function envOrFail(name) {
  const value = envValue(name);
  if (!value) throw new Error(`missing required environment variable: ${name}`);
  return value;
}

function toInt(value, fallback) {
  const n = Number.parseInt(String(value || ""), 10);
  return Number.isFinite(n) ? n : fallback;
}

function toBool(value) {
  if (typeof value === "boolean") return value;
  const v = String(value || "").trim().toLowerCase();
  if (!v) return false;
  if (v === "1" || v === "true" || v === "yes" || v === "y") return true;
  return false;
}

function normalizeText(title, body) {
  return `${String(title || "")}\n\n${String(body || "")}`.toLowerCase();
}

function collectDomainsFromText(title, body) {
  const text = normalizeText(title, body);
  const pattern = /\blark-cli\s+(im|doc|base|sheets|calendar|mail|task|vc|whiteboard|minutes|wiki|event|auth|core)\b/gi;
  const hits = new Set();
  for (const match of text.matchAll(pattern)) {
    const svc = match && match[1] ? String(match[1]).toLowerCase() : "";
    if (DOMAIN_SERVICES.includes(svc)) hits.add(svc);
  }
  return [...hits].sort();
}

function scoreTypeFromText(title, body) {
  const text = normalizeText(title, body);

  const rules = {
    bug: [
      /\bbug\b/i,
      /报错|异常|崩溃|无法|失败|不工作/, 
      /\berror\b|\bexception\b|\bcrash\b|\bbroken\b|\bfails?\b/i,
    ],
    enhancement: [
      /希望支持|建议|新增|能否|是否可以/, 
      /\bfeature request\b|\badd support\b|\bplease add\b|\bwish\b/i,
      /\benhancement\b|\bfeature\b|\brequest\b/i,
    ],
    question: [
      /如何使用|怎么配置|请问|怎么用|是否支持/, 
      /\bhow to\b|\busage\b|\bis it possible\b|\bdoes it support\b|\bquestion\b/i,
    ],
    documentation: [
      /\bdocumentation\b|\bdocs\b|\breadme\b|\btypo\b|\bexample\b/i,
      /文档|示例|说明|拼写/, 
    ],
    performance: [
      /慢|卡住|超时|高内存|响应慢|耗时/, 
      /\bperformance\b|\bperf\b|\bslow\b|\bhang\b|\btimeout\b|\blatency\b|\boom\b/i,
    ],
    security: [
      /凭据泄漏|注入|权限绕过|token\s*暴露|密钥泄露/, 
      /\bsecurity\b|\bvuln\b|\bcve\b|\bcredential\b|\binjection\b|\btoken exposure\b|\bpermission bypass\b/i,
    ],
  };

  const scores = {};
  for (const type of TYPE_LABELS) {
    scores[type] = 0;
    for (const re of rules[type] || []) {
      if (re.test(text)) scores[type] += 1;
    }
  }
  return scores;
}

function chooseTypeFromScores(scores) {
  let max = 0;
  for (const v of Object.values(scores || {})) {
    if (v > max) max = v;
  }
  if (max <= 0) return null;

  const candidates = TYPE_LABELS.filter((t) => scores[t] === max);
  if (candidates.length === 1) return candidates[0];
  for (const t of TYPE_TIE_BREAKER) {
    if (candidates.includes(t)) return t;
  }
  return candidates[0] || null;
}

function classifyIssueText(title, body) {
  const scores = scoreTypeFromText(title, body);
  const type = chooseTypeFromScores(scores);
  const domains = collectDomainsFromText(title, body);
  return { type, domains };
}

function formatIssueRef(repo, number) {
  return `${repo}#${number}`;
}

class GitHubClient {
  constructor(token, repo) {
    this.token = token;
    this.repo = repo;
  }

  buildHeaders(hasBody = false) {
    const headers = {
      Accept: "application/vnd.github+json",
      "X-GitHub-Api-Version": "2022-11-28",
    };
    if (this.token) headers.Authorization = `Bearer ${this.token}`;
    if (hasBody) headers["Content-Type"] = "application/json";
    return headers;
  }

  async request(endpoint, options = {}) {
    const {
      method = "GET",
      payload,
      allow404 = false,
      retry = 5,
    } = options;

    const hasBody = payload !== undefined;
    const url = endpoint.startsWith("http") ? endpoint : `${API_BASE}${endpoint}`;

    for (let attempt = 0; attempt <= retry; attempt += 1) {
      const response = await fetch(url, {
        method,
        headers: this.buildHeaders(hasBody),
        body: hasBody ? JSON.stringify(payload) : undefined,
      });

      if (allow404 && response.status === 404) return null;

      const text = await response.text();
      const remaining = toInt(response.headers.get("x-ratelimit-remaining"), -1);
      const reset = toInt(response.headers.get("x-ratelimit-reset"), -1);
      const retryAfter = toInt(response.headers.get("retry-after"), -1);
      const lower = String(text || "").toLowerCase();
      const isSecondary = lower.includes("secondary rate") || lower.includes("abuse detection");

      if (response.ok) {
        return text ? JSON.parse(text) : null;
      }

      const canRetry = attempt < retry;
      if (!canRetry) {
        const error = new Error(`GitHub API ${method} ${url} failed: ${response.status} ${text}`);
        error.status = response.status;
        throw error;
      }

      // Rate-limit handling
      if (response.status === 429 || isSecondary) {
        const waitMs = retryAfter > 0 ? retryAfter * 1000 : (attempt + 1) * 1000;
        await sleep(waitMs);
        continue;
      }
      if (response.status === 403 && remaining === 0 && reset > 0) {
        const nowSec = Math.floor(Date.now() / 1000);
        const waitMs = Math.max(1, reset - nowSec + 1) * 1000;
        await sleep(waitMs);
        continue;
      }

      // transient-ish failures
      if (response.status >= 500) {
        await sleep((attempt + 1) * 500);
        continue;
      }

      const error = new Error(`GitHub API ${method} ${url} failed: ${response.status} ${text}`);
      error.status = response.status;
      throw error;
    }

    throw new Error(`unreachable: request retry loop exceeded for ${method} ${url}`);
  }

  async listIssues(params) {
    const issues = [];
    const {
      state = "open",
      since,
      maxPages = 10,
      maxIssues = 300,
    } = params || {};

    for (let page = 1; page <= maxPages; page += 1) {
      const search = new URLSearchParams({
        state,
        sort: "updated",
        direction: "desc",
        per_page: "100",
        page: String(page),
      });
      if (since instanceof Date && !Number.isNaN(since.getTime())) {
        search.set("since", since.toISOString());
      }

      const batch = await this.request(`/repos/${this.repo}/issues?${search}`);
      if (!batch || batch.length === 0) break;

      for (const item of batch) {
        issues.push(item);
        if (issues.length >= maxIssues) break;
      }

      if (issues.length >= maxIssues) break;
      if (batch.length < 100) break;

      // early stop if we're beyond the since window
      if (since instanceof Date && batch[batch.length - 1] && batch[batch.length - 1].updated_at) {
        const lastUpdated = new Date(batch[batch.length - 1].updated_at);
        if (!Number.isNaN(lastUpdated.getTime()) && lastUpdated < since) break;
      }
    }

    return issues;
  }

  async addIssueLabels(issueNumber, labels) {
    if (!labels || labels.length === 0) return;
    await this.request(`/repos/${this.repo}/issues/${issueNumber}/labels`, {
      method: "POST",
      payload: { labels },
    });
  }

  async removeIssueLabel(issueNumber, name) {
    await this.request(`/repos/${this.repo}/issues/${issueNumber}/labels/${encodeURIComponent(name)}`, {
      method: "DELETE",
      allow404: true,
    });
  }
}

function planIssueLabelChanges(params) {
  const {
    currentLabels,
    desiredType,
    desiredDomainLabels,
    syncDomains,
  } = params;

  const current = currentLabels instanceof Set ? currentLabels : new Set(currentLabels || []);
  const toAdd = new Set();
  const toRemove = new Set();

  // Type: only apply when desiredType exists.
  if (desiredType) {
    const currentType = [...current].filter((l) => TYPE_LABEL_SET.has(l));
    if (!current.has(desiredType)) {
      toAdd.add(desiredType);
    }
    for (const t of currentType) {
      if (t !== desiredType) toRemove.add(t);
    }
  }

  // Domain: add-only by default; strict sync via --sync-domains.
  const desiredDomains = new Set(desiredDomainLabels || []);
  for (const d of desiredDomains) {
    if (!current.has(d)) toAdd.add(d);
  }

  // Safety: only remove domains when we can positively match at least one domain.
  if (syncDomains && desiredDomains.size > 0) {
    for (const d of current) {
      if (DOMAIN_LABEL_SET.has(d) && !desiredDomains.has(d)) {
        toRemove.add(d);
      }
    }
  }

  return {
    toAdd: [...toAdd],
    toRemove: [...toRemove],
  };
}

function parseArgs(argv) {
  const args = {
    dryRun: false,
    json: false,
    token: "",
    repo: "",
    since: "",
    lookbackHours: 24,
    maxPages: 10,
    maxIssues: 300,
    onlyMissing: true,
    syncDomains: false,
    state: "open",
  };

  for (let i = 0; i < argv.length; i += 1) {
    const a = argv[i];
    if (a === "--help" || a === "-h") {
      args.help = true;
      continue;
    }
    if (a === "--dry-run") {
      args.dryRun = true;
      continue;
    }
    if (a === "--json") {
      args.json = true;
      continue;
    }
    if (a === "--token") {
      args.token = String(argv[++i] || "");
      continue;
    }
    if (a === "--repo") {
      args.repo = String(argv[++i] || "");
      continue;
    }
    if (a === "--since") {
      args.since = String(argv[++i] || "");
      continue;
    }
    if (a === "--lookback-hours") {
      args.lookbackHours = toInt(argv[++i], args.lookbackHours);
      continue;
    }
    if (a === "--max-pages") {
      args.maxPages = toInt(argv[++i], args.maxPages);
      continue;
    }
    if (a === "--max-issues") {
      args.maxIssues = toInt(argv[++i], args.maxIssues);
      continue;
    }
    if (a === "--process-all") {
      args.onlyMissing = false;
      continue;
    }
    if (a === "--only-missing") {
      args.onlyMissing = true;
      continue;
    }
    if (a === "--sync-domains") {
      args.syncDomains = true;
      continue;
    }
    if (a === "--state") {
      args.state = String(argv[++i] || "open");
      continue;
    }
    throw new Error(`unknown argument: ${a}`);
  }

  return args;
}

function printHelp() {
  const msg = `Usage: node scripts/issue-labels/index.js [options]

Options:
  --dry-run            Do not write labels
  --json               Output JSON (useful with --dry-run)
  --repo <owner/name>  Override GITHUB_REPOSITORY
  --token <token>      Override GITHUB_TOKEN
  --since <iso8601>    Only scan issues updated since this timestamp
  --lookback-hours <n> Compute since=now-n hours (default: 24)
  --max-pages <n>      Max pages to scan (default: 10)
  --max-issues <n>     Max issues to process (default: 300)
  --only-missing       Only write when changes are needed (default)
  --process-all        Evaluate all scanned issues
  --sync-domains       Strictly sync domain/* (remove stale) when domain matched
  --state open|all     Issue state to scan (default: open)
`;
  console.log(msg);
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    printHelp();
    return;
  }

  const token = args.token || envOrFail("GITHUB_TOKEN");
  const repo = args.repo || envOrFail("GITHUB_REPOSITORY");
  const client = new GitHubClient(token, repo);

  let since = null;
  if (args.since) {
    const d = new Date(args.since);
    if (Number.isNaN(d.getTime())) throw new Error(`invalid --since: ${args.since}`);
    since = d;
  } else if (args.lookbackHours > 0) {
    since = new Date(Date.now() - args.lookbackHours * 3600 * 1000);
  }

  const scanned = await client.listIssues({
    state: args.state,
    since,
    maxPages: args.maxPages,
    maxIssues: args.maxIssues,
  });

  const results = {
    repo,
    dryRun: args.dryRun,
    since: since ? since.toISOString() : null,
    scanned: 0,
    skippedPR: 0,
    updated: 0,
    changes: [],
  };

  for (const issue of scanned) {
    results.scanned += 1;
    if (issue && issue.pull_request) {
      results.skippedPR += 1;
      continue;
    }

    const currentLabels = new Set((issue.labels || []).map((l) => l.name));
    const { type: desiredType, domains } = classifyIssueText(issue.title, issue.body);
    const desiredDomainLabels = domains.map((d) => `domain/${d}`);

    const { toAdd, toRemove } = planIssueLabelChanges({
      currentLabels,
      desiredType,
      desiredDomainLabels,
      syncDomains: args.syncDomains,
    });

    const hasChange = toAdd.length > 0 || toRemove.length > 0;
    if (args.onlyMissing && !hasChange) continue;

    const record = {
      issue: {
        number: issue.number,
        title: issue.title,
        url: issue.html_url,
      },
      desired: {
        type: desiredType,
        domains,
      },
      change: { toAdd, toRemove },
    };

    if (args.json) {
      results.changes.push(record);
    } else {
      console.log(`[${formatIssueRef(repo, issue.number)}] +${toAdd.join(", ") || "-"} -${toRemove.join(", ") || "-"}`);
    }

    if (!args.dryRun) {
      // Add first to avoid leaving a temporary empty state.
      if (toAdd.length > 0) {
        await client.addIssueLabels(issue.number, toAdd);
      }
      for (const name of toRemove) {
        await client.removeIssueLabel(issue.number, name);
      }
    }

    if (hasChange) {
      results.updated += 1;
    }
  }

  if (args.json) {
    console.log(JSON.stringify(results));
  } else {
    console.log(`done: scanned=${results.scanned} updated=${results.updated} skipped_pr=${results.skippedPR}`);
  }
}

if (require.main === module) {
  main().catch((err) => {
    console.error(err && err.stack ? err.stack : String(err));
    process.exit(1);
  });
}

module.exports = {
  classifyIssueText,
  collectDomainsFromText,
  scoreTypeFromText,
  chooseTypeFromScores,
  planIssueLabelChanges,
  TYPE_LABELS,
  DOMAIN_SERVICES,
};
