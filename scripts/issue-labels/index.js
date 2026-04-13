// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

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
  "drive",
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
const DOMAIN_ALIASES = ["docs"];
const DOMAIN_REGEX_ALTERNATION = [...DOMAIN_SERVICES, ...DOMAIN_ALIASES].join("|");
const DOMAIN_LABELS = DOMAIN_SERVICES.map((s) => `domain/${s}`);
const DOMAIN_LABEL_SET = new Set(DOMAIN_LABELS);
const MANAGED_LABELS = [...TYPE_LABELS, ...DOMAIN_LABELS];

const TYPE_TIE_BREAKER = [
  "security",
  "bug",
  "performance",
  "enhancement",
  "documentation",
  "question",
];

// More conservative type labeling: prefer "no label" over mislabeling.
// - Require a minimum score.
// - When the top two candidates are too close, treat as ambiguous and do not label.
const TYPE_MIN_SCORE = 2;
const TYPE_MIN_MARGIN = 1;

/**
 * Pause execution for the provided number of milliseconds.
 *
 * @param {number} ms
 * @returns {Promise<void>}
 */
function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * Read an environment variable and trim surrounding whitespace.
 *
 * @param {string} name
 * @returns {string}
 */
function envValue(name) {
  const value = process.env[name];
  return value ? String(value).trim() : "";
}

/**
 * Read a required environment variable.
 *
 * @param {string} name
 * @returns {string}
 */
function envOrFail(name) {
  const value = envValue(name);
  if (!value) throw new Error(`missing required environment variable: ${name}`);
  return value;
}

/**
 * Parse an integer value with a fallback when parsing fails.
 *
 * @param {string|number|undefined|null} value
 * @param {number} fallback
 * @returns {number}
 */
function toInt(value, fallback) {
  const n = Number.parseInt(String(value || ""), 10);
  return Number.isFinite(n) ? n : fallback;
}

/**
 * Parse a boolean-ish value from CLI or environment input.
 *
 * @param {string|boolean|undefined|null} value
 * @returns {boolean}
 */
function toBool(value) {
  if (typeof value === "boolean") return value;
  const v = String(value || "").trim().toLowerCase();
  if (!v) return false;
  if (v === "1" || v === "true" || v === "yes" || v === "y") return true;
  return false;
}

/**
 * Normalize issue title and body into a single lowercase string.
 *
 * @param {string} title
 * @param {string} body
 * @returns {string}
 */
function normalizeText(title, body) {
  return `${String(title || "")}\n\n${String(body || "")}`.toLowerCase();
}

/**
 * Infer candidate domain services from issue title and body text.
 *
 * @param {string} title
 * @param {string} body
 * @returns {string[]}
 */
function collectDomainsFromText(title, body) {
  const normalizedBody = String(body || "")
    .replace(/(["'])(?:(?=(\\?))\2.)*?\1/gs, (segment) => {
      return /lark-cli\s+/i.test(segment) && segment.length > 80 ? '""' : segment;
    });
  const text = normalizeText(title, normalizedBody);
  const titleText = String(title || "").toLowerCase();

  const hits = new Set();

  function normalizeService(svc) {
    const s = String(svc || "").toLowerCase();
    if (s === "docs") return "doc";
    return s;
  }

  // 1) Explicit domain labels in text: domain/<service>
  const explicit = new RegExp(`\\bdomain\\/(${DOMAIN_REGEX_ALTERNATION})\\b`, "gi");
  for (const match of text.matchAll(explicit)) {
    const svc = match && match[1] ? normalizeService(match[1]) : "";
    if (DOMAIN_SERVICES.includes(svc)) hits.add(svc);
  }

  // 2) Command mention: lark-cli <service> / lark cli <service>
  const cmd = new RegExp(`\\blark[-\\s]?cli\\s+(${DOMAIN_REGEX_ALTERNATION})\\b`, "gi");
  for (const match of text.matchAll(cmd)) {
    const svc = match && match[1] ? normalizeService(match[1]) : "";
    if (DOMAIN_SERVICES.includes(svc)) hits.add(svc);
  }

  // 3) Loose title match: if title contains a standalone service word.
  // This is intentionally limited to TITLE to reduce false positives.
  // NOTE: exclude `im` here because it's too common in English text (e.g. "im stuck").
  const looseServices = DOMAIN_SERVICES.filter((s) => s !== "im");
  for (const svc of looseServices) {
    const pattern = svc === "doc" ? "\\bdocs?\\b" : `\\b${svc}\\b`;
    const re = new RegExp(pattern, "i");
    if (re.test(titleText)) hits.add(svc);
  }

  // 4) Keyword heuristics (for users who don't paste the exact command)
  // Keep this conservative; add keywords only when they are strongly tied to a domain.
  const keywordMap = {
    base: [/\bbase\s*\+/i, /\bbase-token\b/i, /open-apis\/bitable\//i, /\brecords?\/(search|list)\b/i, /多维表格/],
    doc: [/\bdocx\b/i, /\bfeishu document\b/i, /\blark document\b/i, /\bdocument comments?\b/i, /飞书文档|云文档|文档/],
    drive: [/\bdrive\b/i, /\bfolder token\b/i, /create_folder/i, /drive\/v1\/files/i, /\bdrive\s*\+/i],
    sheets: [/电子表格/, /\bsheets\s*\+/i],
    calendar: [/日历/, /\bcalendar\s*\+/i],
    mail: [/邮件/, /\bmail\s*\+/i],
    task: [/任务清单/, /飞书任务/, /\btask\s*\+/i],
    wiki: [/知识库/, /\bwiki\s*\+/i],
    minutes: [/妙记/, /\bminutes\s*\+/i],
    vc: [/\bvc\s*\+/i, /飞书会议|视频会议|创建会议/],
    im: [/消息|群聊|私聊/, /\bim\s*\+/i, /im\/v1/i],
    auth: [/\bauth\s+(login|status|check|logout)\b/i, /\bkeychain\b/i, /\buser_access_token\b/i, /\buser token\b/i, /\bconsent\b/i, /授权|登录|scope authorization/],
    core: [/\bpostinstall\b/i, /\bconfig(\.json)?\b/i, /\bconfig\s+(init|show|remove)\b/i, /\bpackage\.json\b/i, /\bscripts\/install\.js\b/i, /\bbun\b/i, /\bskills?\b/i, /\btrae\b/i, /\bprofile\b/i, /\bmulti-account\b/i, /\bprivate deployment\b/i, /\bbinary release\b/i, /\bbinary fails?\b/i, /\bunsupported platform\b/i, /\bebadplatform\b/i, /\bwindows\b.*\bbinary\b|\bbinary\b.*\bwindows\b/i, /\briscv64\b.*\bsupport/i, /私有化|安装脚本|配置文件|多账号|多个应用|多用户|持久化连接|服务器端/],
  };
  for (const [svc, patterns] of Object.entries(keywordMap)) {
    if (!DOMAIN_SERVICES.includes(svc)) continue;
    for (const re of patterns) {
      if (re.test(text)) {
        hits.add(svc);
        break;
      }
    }
  }

  return [...hits].sort();
}

/**
 * Score each type label against the issue content.
 *
 * @param {string} title
 * @param {string} body
 * @returns {Record<string, number>}
 */
function scoreTypeFromText(title, body) {
  const text = normalizeText(title, body);
  const titleText = String(title || "").toLowerCase();

  const rules = {
    bug: [
      // explicit
      { re: /\bbug\b/i, w: 2 },
      // strong signals (stack traces, errors, crashes)
      { re: /\berror\b|\bexception\b|\bcrash\b|\bpanic\b|\bstack\s*trace\b|\bbroken\b|\bfails?\b|\bsigkill\b|\binvalid json\b|\bno stdout\b|\bno stderr\b|\bno output\b|\bsilently fail\w*\b|\bsilently drop\w*\b|\bdiscard\w*\b/i, w: 2 },
      // Chinese strong/medium signals
      { re: /报错|错误|异常|崩溃|闪退|卡死|死机|无输出|静默失败|截断|不生效|无法正确|不能正确|被忽略|丢失|没有给/, w: 2 },
      // Contextual "cannot/fail" patterns that are usually bugs (avoid labeling based on a bare "无法/失败").
      { re: /(无法|失败)(正常)?(.{0,16})?(使用|运行|执行|发送|创建|获取|写入|读取|安装|登录|导出|更新|上传|下载)/, w: 2 },
      // weak motivation words (do NOT label bug based on these alone)
      { re: /无法|失败|没法|不能用|不可用/, w: 1 },
    ],
    enhancement: [
      // Chinese/English explicit feature request
      { re: /功能请求|需求|\bfeature request\b|\badd support\b|\bplease add\b/i, w: 2 },
      { re: /希望支持|建议|新增|支持.*(能力|功能)/, w: 2 },
      // common Chinese ask forms that usually indicate a request
      { re: /能不能支持|能否支持|希望增加|希望新增/, w: 2 },
      // weak asks
      { re: /能否|是否可以|可否|能不能|是否能够|希望能|希望可以|请求/, w: 1 },
      { re: /\benhancement\b|\bfeature\b/i, w: 1 },
    ],
    question: [
      // comparison/usage questions
      { re: /有什么区别|有什么不同|区别是什么|\bwhat is the difference\b/i, w: 2 },
      { re: /\bhow to\b|\busage\b|\bis it possible\b|\bdoes it support\b|\bquestion\b/i, w: 2 },
      // weak question forms
      { re: /为什么/, w: 2 },
      { re: /请问|是否支持|有没有.*(支持|能力)|怎么(用|配置|接入|做)|如何(使用|配置|接入|做)|可以.*吗|能.*吗|对比/, w: 1 },
    ],
    documentation: [
      // Treat docs-related words as weaker unless paired with an explicit docs-fix signal.
      { re: /\btypo\b|\bspell(ing)?\b/i, w: 2 },
      // Avoid generic "文档" (many issues are about the document product); require a docs-fix context.
      { re: /拼写|文档(错误|修正|修复|补充|改进)|文档.*(缺失|不完整)|安装说明/, w: 2 },
      { re: /\bdocumentation\b|\breadme\b|\bexample\b|\bbest practice\b/i, w: 1 },
      { re: /示例/, w: 1 },
    ],
    performance: [
      // Avoid generic "slow" causing false positives (many issues mention slow networks).
      { re: /\bperformance\b|\bperf\b|\bhang\b|\btimeout\b|\blatency\b|\boom\b|10-100x faster|60\+ seconds/i, w: 2 },
      { re: /\bslow\b/i, w: 1 },
      { re: /慢|卡住|超时|高内存|响应慢|耗时/, w: 1 },
    ],
    security: [
      { re: /\bvuln\b|\bcve\b|\binjection\b|\btoken exposure\b|\bpermission bypass\b|\bcredential leak\b/i, w: 2 },
      { re: /凭据泄漏|注入|权限绕过|token\s*暴露|密钥泄露/, w: 2 },
    ],
  };

  const scores = {};
  for (const type of TYPE_LABELS) {
    scores[type] = 0;
    for (const rule of rules[type] || []) {
      const re = rule && rule.re;
      const w = rule && typeof rule.w === "number" ? rule.w : 1;
      if (re && re.test(text)) scores[type] += w;
    }
  }

  if (/^\s*\[bug\]/i.test(titleText) || /^\s*bug[:(]/i.test(titleText)) {
    scores.bug += 3;
  }
  if (/^\s*\[(feature|feature request)\]/i.test(titleText) || /\bfeature request\b/i.test(titleText) || /^\s*feat[:(]/i.test(titleText)) {
    scores.enhancement += 3;
  }
  if (/^\s*[【\[]\s*(feature|需求|功能)\s*[】\]]/.test(titleText)) {
    scores.enhancement += 3;
  }
  // Common Chinese feature request prefixes.
  if (/^\s*(功能请求|需求)[:：]/.test(titleText) || /\bfeature\b[:：]/i.test(titleText)) {
    scores.enhancement += 3;
  }
  if (/希望支持|能否支持|是否可以/.test(titleText)) {
    scores.enhancement += 1;
  }
  if (/^\s*\[doc\]/i.test(titleText)) {
    scores.documentation += 4;
    // If user explicitly marks it as a documentation issue, reduce the chance of mislabeling it as a bug.
    if (scores.bug > 0) scores.bug = Math.max(0, scores.bug - 3);
  }
  if (/^request\b/i.test(titleText)) {
    scores.enhancement += 3;
  }

  return scores;
}

/**
 * Choose the highest-scoring type using the configured tie breaker.
 *
 * @param {Record<string, number>} scores
 * @returns {string|null}
 */
function chooseTypeFromScores(scores) {
  const entries = TYPE_LABELS.map((t) => ({ t, v: (scores && scores[t]) || 0 }))
    .sort((a, b) => b.v - a.v);
  const top = entries[0] || { t: null, v: 0 };
  const second = entries[1] || { t: null, v: 0 };

  if (top.v < TYPE_MIN_SCORE) return null;
  // Ambiguous: top two are too close.
  if (top.v - second.v < TYPE_MIN_MARGIN) return null;

  // Preserve deterministic choice when multiple labels have same score (should be rare after margin check).
  const candidates = TYPE_LABELS.filter((t) => (scores && scores[t]) === top.v);
  if (candidates.length === 1) return candidates[0];
  for (const t of TYPE_TIE_BREAKER) {
    if (candidates.includes(t)) return t;
  }
  return candidates[0] || null;
}

/**
 * Classify issue text into one type label and zero or more domains.
 *
 * @param {string} title
 * @param {string} body
 * @returns {{type: string|null, domains: string[]}}
 */
function classifyIssueText(title, body) {
  const scores = scoreTypeFromText(title, body);
  const type = chooseTypeFromScores(scores);
  const domains = collectDomainsFromText(title, body);
  return { type, domains };
}

/**
 * Format a GitHub issue reference for logs.
 *
 * @param {string} repo
 * @param {number} number
 * @returns {string}
 */
function formatIssueRef(repo, number) {
  return `${repo}#${number}`;
}

/**
 * Minimal GitHub REST client for issue labeling operations.
 */
class GitHubClient {
  /**
   * @param {string} token
   * @param {string} repo
   */
  constructor(token, repo) {
    this.token = token;
    this.repo = repo;
  }

  /**
   * Build standard GitHub API headers.
   *
   * @param {boolean} hasBody
   * @returns {Record<string, string>}
   */
  buildHeaders(hasBody = false) {
    const headers = {
      Accept: "application/vnd.github+json",
      "X-GitHub-Api-Version": "2022-11-28",
    };
    if (this.token) headers.Authorization = `Bearer ${this.token}`;
    if (hasBody) headers["Content-Type"] = "application/json";
    return headers;
  }

  /**
   * Execute a GitHub API request with retry and rate-limit handling.
   *
   * @param {string} endpoint
   * @param {{method?: string, payload?: any, allow404?: boolean, retry?: number}} options
   * @returns {Promise<any>}
   */
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
        const waitMs = retryAfter > 0
          ? retryAfter * 1000
          : isSecondary
            ? 60_000
            : (attempt + 1) * 1000;
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

  /**
   * Search for currently unlabeled issues in the repository.
   *
   * This is intentionally a one-shot triage pass: once any label is added, the
   * issue falls out of scope for future scheduled runs.
   *
   * @param {{state?: string, maxPages?: number, maxIssues?: number}} params
   * @returns {Promise<any[]>}
   */
  async searchUnlabeledIssues(params) {
    const issues = [];
    const {
      state = "open",
      maxPages = 10,
      maxIssues = 300,
    } = params || {};

    const qualifiers = [
      `repo:${this.repo}`,
      "is:issue",
      "no:label",
      state === "all" ? "" : `state:${state}`,
    ].filter(Boolean);
    const q = qualifiers.join(" ");

    for (let page = 1; page <= maxPages; page += 1) {
      const search = new URLSearchParams({
        q,
        sort: "updated",
        order: "desc",
        per_page: "100",
        page: String(page),
      });

      const result = await this.request(`/search/issues?${search}`);
      const batch = result && Array.isArray(result.items) ? result.items : [];
      if (batch.length === 0) break;

      for (const item of batch) {
        issues.push(item);
        if (issues.length >= maxIssues) break;
      }

      if (issues.length >= maxIssues) break;
      if (batch.length < 100) break;
    }

    return issues;
  }

  /**
   * List all repository labels needed for managed-label checks.
   *
   * @returns {Promise<any[]>}
   */
  async listRepositoryLabels() {
    const labels = [];
    for (let page = 1; page <= 10; page += 1) {
      const search = new URLSearchParams({
        per_page: "100",
        page: String(page),
      });
      const batch = await this.request(`/repos/${this.repo}/labels?${search}`);
      if (!batch || batch.length === 0) break;
      labels.push(...batch);
      if (batch.length < 100) break;
    }
    return labels;
  }

  /**
   * Return managed labels that are not currently present in the repository.
   *
   * @returns {Promise<string[]>}
   */
  async listMissingManagedLabels() {
    const existing = new Set((await this.listRepositoryLabels()).map((label) => label && label.name));
    return MANAGED_LABELS.filter((name) => !existing.has(name));
  }

  /**
   * Add one or more labels to an issue.
   *
   * @param {number} issueNumber
   * @param {string[]} labels
   * @returns {Promise<void>}
   */
  async addIssueLabels(issueNumber, labels) {
    if (!labels || labels.length === 0) return;
    await this.request(`/repos/${this.repo}/issues/${issueNumber}/labels`, {
      method: "POST",
      payload: { labels },
    });
  }

  /**
   * Remove a single label from an issue.
   *
   * @param {number} issueNumber
   * @param {string} name
   * @returns {Promise<void>}
   */
  async removeIssueLabel(issueNumber, name) {
    await this.request(`/repos/${this.repo}/issues/${issueNumber}/labels/${encodeURIComponent(name)}`, {
      method: "DELETE",
      allow404: true,
    });
  }
}

/**
 * Compute label mutations for the current issue state.
 *
 * @param {{currentLabels: Set<string>|string[], desiredType: string|null, desiredDomainLabels: string[], syncDomains: boolean, overrideType: boolean}} params
 * @returns {{toAdd: string[], toRemove: string[]}}
 */
function planIssueLabelChanges(params) {
  const {
    currentLabels,
    desiredType,
    desiredDomainLabels,
    syncDomains,
    overrideType,
  } = params;

  const current = currentLabels instanceof Set ? currentLabels : new Set(currentLabels || []);
  const toAdd = new Set();
  const toRemove = new Set();

  // Type: only apply when desiredType exists.
  // Safety: by default, do NOT override existing type labels to avoid reverting manual triage.
  if (desiredType) {
    const currentType = [...current].filter((l) => TYPE_LABEL_SET.has(l));
    const shouldApplyType = overrideType || currentType.length === 0;
    if (shouldApplyType) {
      if (!current.has(desiredType)) {
        toAdd.add(desiredType);
      }
      for (const t of currentType) {
        if (t !== desiredType) toRemove.add(t);
      }
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

/**
 * Parse CLI arguments into runtime options.
 *
 * @param {string[]} argv
 * @returns {{dryRun: boolean, json: boolean, token: string, repo: string, maxPages: number, maxIssues: number, onlyMissing: boolean, syncDomains: boolean, overrideType: boolean, state: string, help?: boolean}}
 */
function parseArgs(argv) {
  const args = {
    dryRun: false,
    json: false,
    token: "",
    repo: "",
    maxPages: 10,
    maxIssues: 300,
    onlyMissing: true,
    syncDomains: false,
    overrideType: false,
    state: "open",
  };

  let i = 0;

  function readFlagValue(flag) {
    const value = argv[i + 1];
    if (value === undefined || String(value).startsWith("-")) {
      throw new Error(`missing value for ${flag}`);
    }
    i += 1;
    return String(value);
  }

  for (; i < argv.length; i += 1) {
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
      args.token = readFlagValue("--token");
      continue;
    }
    if (a === "--repo") {
      args.repo = readFlagValue("--repo");
      continue;
    }
    if (a === "--max-pages") {
      args.maxPages = toInt(readFlagValue("--max-pages"), args.maxPages);
      continue;
    }
    if (a === "--max-issues") {
      args.maxIssues = toInt(readFlagValue("--max-issues"), args.maxIssues);
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
    if (a === "--override-type") {
      args.overrideType = true;
      continue;
    }
    if (a === "--state") {
      args.state = readFlagValue("--state");
      continue;
    }
    throw new Error(`unknown argument: ${a}`);
  }

  return args;
}

/**
 * Print CLI help text.
 *
 * @returns {void}
 */
function printHelp() {
  const msg = `Usage: node scripts/issue-labels/index.js [options]

Options:
  --dry-run            Do not write labels
  --json               Output JSON (useful with --dry-run)
  --repo <owner/name>  Override GITHUB_REPOSITORY
  --token <token>      Override GITHUB_TOKEN
  --max-pages <n>      Max search result pages to scan (default: 10)
  --max-issues <n>     Max unlabeled issues to process (default: 300)
  --only-missing       Only write when changes are needed (default)
  --process-all        Evaluate all fetched unlabeled issues
  --sync-domains       Strictly sync domain/* (remove stale) when domain matched
  --override-type      Override existing type labels (default: false)
  --state open|all     Issue state to scan (default: open)
`;
  console.log(msg);
}

/**
 * Entry point for the issue labeler CLI.
 *
 * @returns {Promise<void>}
 */
async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.help) {
    printHelp();
    return;
  }

  const token = args.token || envOrFail("GITHUB_TOKEN");
  const repo = args.repo || envOrFail("GITHUB_REPOSITORY");
  const client = new GitHubClient(token, repo);
  const missingManagedLabels = new Set(await client.listMissingManagedLabels());

  const scanned = await client.searchUnlabeledIssues({
    state: args.state,
    maxPages: args.maxPages,
    maxIssues: args.maxIssues,
  });

  const results = {
    repo,
    dryRun: args.dryRun,
    query: "unlabeled issues (intentional one-shot scope)",
    scanned: 0,
    skippedPR: 0,
    skippedIssue: 0,
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
      overrideType: args.overrideType,
    });

    // If some managed labels do not exist in the repository, drop only those labels
    // (still apply the rest) instead of skipping the entire issue.
    const missingForIssue = toAdd.filter((name) => missingManagedLabels.has(name));
    const effectiveToAdd = missingForIssue.length > 0
      ? toAdd.filter((name) => !missingManagedLabels.has(name))
      : toAdd;

    if (missingForIssue.length > 0) {
      const warning = `warning: ${formatIssueRef(repo, issue.number)} missing labels in ${repo}: ${missingForIssue.join(", ")}`;
      console.warn(warning);
    }

    const hasChange = effectiveToAdd.length > 0 || toRemove.length > 0;
    // When --only-missing is enabled (default), we still want JSON output to reflect
    // issues that were "actionable" only by missing repo labels.
    if (args.onlyMissing && !hasChange) {
      if (args.json && missingForIssue.length > 0) {
        results.skippedIssue += 1;
        results.changes.push({
          issue: {
            number: issue.number,
            title: issue.title,
            url: issue.html_url,
          },
          desired: {
            type: desiredType,
            domains,
          },
          change: { toAdd: [], toRemove: [] },
          skipped: true,
          reason: "missing_managed_labels",
          missingLabels: missingForIssue,
        });
      }
      continue;
    }

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
      change: { toAdd: effectiveToAdd, toRemove },
    };

    if (missingForIssue.length > 0) {
      record.missingLabels = missingForIssue;
    }

    if (args.json) {
      results.changes.push(record);
    } else {
      console.log(`[${formatIssueRef(repo, issue.number)}] +${effectiveToAdd.join(", ") || "-"} -${toRemove.join(", ") || "-"}`);
    }

    if (!args.dryRun) {
      // Add first to avoid leaving a temporary empty state.
      if (effectiveToAdd.length > 0) {
        await client.addIssueLabels(issue.number, effectiveToAdd);
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
    console.log(`done: scanned=${results.scanned} updated=${results.updated} skipped_pr=${results.skippedPR} skipped_issue=${results.skippedIssue}`);
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
