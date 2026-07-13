#!/usr/bin/env node
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const fs = require("node:fs");
const path = require("node:path");
const { execFileSync } = require("node:child_process");
const {
  e2eDomainsForPath,
  findPathMapping,
  isSkippablePath,
  matchesFullFallback,
  normalizeRepoPath,
} = require("./domain-map");

const ROOT = process.env.E2E_DOMAINS_ROOT || path.join(__dirname, "..");
process.chdir(ROOT);

function execLines(command, args) {
  return execFileSync(command, args, { encoding: "utf8" })
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean);
}

function modulePath() {
  return execLines("go", ["list", "-m"])[0];
}

function rootPackage(moduleName) {
  return `${moduleName}/tests/cli_e2e`;
}

function allLivePackages(moduleName) {
  return execLines("go", ["list", "./tests/cli_e2e/..."])
    .filter((pkg) => pkg !== rootPackage(moduleName))
    .filter((pkg) => !pkg.endsWith("/demo"));
}

function allDryPackages(moduleName) {
  return allLivePackages(moduleName);
}

const domainExistsCache = new Map();

function domainExists(domain) {
  if (domainExistsCache.has(domain)) {
    return domainExistsCache.get(domain);
  }
  let exists = false;
  try {
    execFileSync("go", ["list", `./tests/cli_e2e/${domain}`], { stdio: "ignore" });
    exists = true;
  } catch {
    exists = false;
  }
  domainExistsCache.set(domain, exists);
  return exists;
}

function readChangedFiles() {
  const changedFilesPath = process.env.E2E_DOMAIN_CHANGED_FILES;
  if (changedFilesPath) {
    return fs.readFileSync(changedFilesPath, "utf8")
      .split(/\r?\n/)
      .map(normalizeRepoPath)
      .filter(Boolean);
  }

  if (process.env.GITHUB_EVENT_NAME !== "pull_request") {
    return null;
  }

  const baseRef = process.env.GITHUB_BASE_REF || "main";
  try {
    execFileSync("git", ["rev-parse", "--verify", `origin/${baseRef}`], { stdio: "ignore" });
    return execLines("git", ["diff", "--name-only", `origin/${baseRef}...HEAD`]).map(normalizeRepoPath);
  } catch {
    return null;
  }
}

function addDomain(domains, domain) {
  if (domain && domainExists(domain)) {
    domains.add(domain);
    return true;
  }
  return false;
}

function classifyPath(filePath, domains) {
  const normalized = normalizeRepoPath(filePath);
  if (!normalized) return { matched: false };

  const e2eMatch = normalized.match(/^tests\/cli_e2e\/([^/]+)\//);
  if (e2eMatch) {
    const domain = e2eMatch[1];
    if (domain === "demo") return { matched: false };
    if (domainExists(domain)) {
      addDomain(domains, domain);
      return { matched: true };
    }
    if (isSkippablePath(normalized)) return { matched: false };
    return { fullReason: `unknown CLI E2E domain path: ${normalized}` };
  }

  if (normalized.startsWith("tests/cli_e2e/")) {
    return { fullReason: `shared CLI E2E harness changed: ${normalized}` };
  }

  if (matchesFullFallback(normalized)) {
    return { fullReason: `shared/runtime path changed: ${normalized}` };
  }

  const mappedDomains = e2eDomainsForPath(normalized);
  if (mappedDomains.length > 0) {
    const missingDomains = [];
    for (const domain of mappedDomains) {
      if (!addDomain(domains, domain)) missingDomains.push(domain);
    }
    if (missingDomains.length > 0) {
      return { fullReason: `mapped CLI E2E domain has no package: ${missingDomains.join(",")} (${normalized})` };
    }
    return { matched: true };
  }

  if (findPathMapping(normalized)) {
    return { fullReason: `mapped path has no CLI E2E package: ${normalized}` };
  }

  if (normalized.match(/^shortcuts\/[^/]+\//) || normalized.match(/^skills\/lark-[^/]+\//)) {
    return { fullReason: `unmapped CLI E2E domain path: ${normalized}` };
  }

  if (isSkippablePath(normalized)) return { matched: false };

  return { fullReason: `unclassified path changed: ${normalized}` };
}

function resolveDomains(changedFiles) {
  const moduleName = modulePath();
  const rootDryPackage = rootPackage(moduleName);
  if (changedFiles === null) {
    return {
      mode: "full",
      reason: "non-pull_request run or unavailable diff",
      domains: ["all"],
      dryRootPackage: rootDryPackage,
      dryPackages: allDryPackages(moduleName),
      livePackages: allLivePackages(moduleName),
    };
  }

  const domains = new Set();
  let matchedRelevant = false;
  let fullReason = "";

  for (const file of changedFiles) {
    const result = classifyPath(file, domains);
    if (result.matched) matchedRelevant = true;
    if (result.fullReason && !fullReason) fullReason = result.fullReason;
  }

  if (fullReason) {
    return {
      mode: "full",
      reason: fullReason,
      domains: ["all"],
      dryRootPackage: rootDryPackage,
      dryPackages: allDryPackages(moduleName),
      livePackages: allLivePackages(moduleName),
    };
  }

  if (matchedRelevant && domains.size > 0) {
    const sortedDomains = [...domains].sort();
    const packages = sortedDomains.map((domain) => `${moduleName}/tests/cli_e2e/${domain}`);
    return {
      mode: "subset",
      reason: "business domain changes",
      domains: sortedDomains,
      dryRootPackage: rootDryPackage,
      dryPackages: packages,
      livePackages: packages,
    };
  }

  return {
    mode: "skip",
    reason: "docs-only or no live CLI E2E impact",
    domains: [],
    dryRootPackage: "",
    dryPackages: [],
    livePackages: [],
  };
}

function emit(resolved) {
  const values = {
    mode: resolved.mode,
    reason: resolved.reason,
    domains: resolved.domains.join(","),
    dry_root_package: resolved.dryRootPackage,
    dry_packages: resolved.dryPackages.join(" "),
    live_packages: resolved.livePackages.join(" "),
  };

  const lines = Object.entries(values).map(([key, value]) => `${key}=${value}`);
  console.log(lines.join("\n"));

  if (process.env.GITHUB_OUTPUT) {
    fs.appendFileSync(process.env.GITHUB_OUTPUT, `${lines.join("\n")}\n`);
  }
}

if (require.main === module) {
  emit(resolveDomains(readChangedFiles()));
}

module.exports = {
  classifyPath,
  readChangedFiles,
  resolveDomains,
};
