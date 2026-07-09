// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const fs = require("node:fs");
const path = require("node:path");

const DOMAIN_MAP_PATH = path.join(__dirname, "domain-map.json");
const domainMap = JSON.parse(fs.readFileSync(DOMAIN_MAP_PATH, "utf8"));

function normalizeRepoPath(input) {
  return String(input || "").trim().replace(/\\/g, "/").replace(/^\.\//, "").toLowerCase();
}

const pathMappingsBySpecificity = (domainMap.pathMappings || [])
  .map((entry) => ({ ...entry, prefix: normalizeRepoPath(entry.prefix) }))
  .sort((a, b) => b.prefix.length - a.prefix.length);

function findPathMapping(filePath) {
  const normalized = normalizeRepoPath(filePath);
  return pathMappingsBySpecificity.find((entry) => normalized.startsWith(entry.prefix));
}

function labelDomainsForPath(filePath) {
  const mapping = findPathMapping(filePath);
  return mapping ? [...(mapping.labelDomains || [])] : [];
}

function e2eDomainsForPath(filePath) {
  const mapping = findPathMapping(filePath);
  return mapping ? [...(mapping.e2eDomains || [])] : [];
}

function matchesFullFallback(filePath) {
  const normalized = normalizeRepoPath(filePath);
  return (domainMap.fullFallbackPrefixes || []).some((prefix) => normalized.startsWith(prefix));
}

function isSkippablePath(filePath) {
  const normalized = normalizeRepoPath(filePath);
  const basename = path.posix.basename(normalized);
  return (domainMap.skipPrefixes || []).some((prefix) => normalized.startsWith(prefix))
    || (domainMap.skipSuffixes || []).some((suffix) => normalized.endsWith(suffix))
    || (domainMap.skipFilenames || []).includes(basename);
}

module.exports = {
  domainMap,
  e2eDomainsForPath,
  findPathMapping,
  isSkippablePath,
  labelDomainsForPath,
  matchesFullFallback,
  normalizeRepoPath,
};
