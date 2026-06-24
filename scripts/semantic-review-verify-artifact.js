// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const fs = require("fs");
const crypto = require("crypto");
const zlib = require("zlib");

const MAX_FACTS_BYTES = 4 * 1024 * 1024;
const MAX_COMPRESSION_RATIO = 100;
const MAX_ARRAY_ITEMS = 5000;
const MAX_STRING_BYTES = 8192;
const VALID_ACTIONS = new Set(["REJECT", "LABEL", "WARNING"]);
const MAX_OBJECT_KEYS = 1000;
const MAX_JSON_DEPTH = 12;

function isSymlink(entry) {
  return ((entry.externalFileAttributes >>> 16) & 0o170000) === 0o120000;
}

function isRegularOrUnspecified(entry) {
  const fileType = (entry.externalFileAttributes >>> 16) & 0o170000;
  return fileType === 0 || fileType === 0o100000;
}

function verifyZipEntries(entries) {
  if (entries.length !== 1) {
    throw new Error(`expected exactly one artifact file, got ${entries.length}`);
  }
  const entry = entries[0];
  if (entry.fileName !== "facts.json" || entry.fileName.startsWith("/") || entry.fileName.includes("..")) {
    throw new Error(`invalid artifact path: ${entry.fileName}`);
  }
  if (isSymlink(entry)) {
    throw new Error("facts artifact must not contain symlinks");
  }
  if (!isRegularOrUnspecified(entry)) {
    throw new Error("facts artifact must contain a regular file");
  }
  if (entry.uncompressedSize <= 0 || entry.uncompressedSize > MAX_FACTS_BYTES) {
    throw new Error(`invalid facts size: ${entry.uncompressedSize}`);
  }
  if (entry.compressedSize > 0 && entry.uncompressedSize / entry.compressedSize > MAX_COMPRESSION_RATIO) {
    throw new Error("facts artifact compression ratio is too high");
  }
  return entry;
}

function readZipEntries(zipPath) {
  return readZipEntriesFromBuffer(fs.readFileSync(zipPath));
}

function readZipEntriesFromBuffer(buf) {
  const eocdOffset = findEndOfCentralDirectory(buf);
  requireBufferRange(buf, eocdOffset, 22, "zip end of central directory");
  const entriesTotal = buf.readUInt16LE(eocdOffset + 10);
  const centralDirectorySize = buf.readUInt32LE(eocdOffset + 12);
  const centralDirectoryOffset = buf.readUInt32LE(eocdOffset + 16);
  requireBufferRange(buf, centralDirectoryOffset, centralDirectorySize, "zip central directory");
  if (centralDirectoryOffset + centralDirectorySize > eocdOffset) {
    throw new Error("zip central directory overlaps end of central directory");
  }
  const entries = [];
  let offset = centralDirectoryOffset;
  for (let i = 0; i < entriesTotal; i++) {
    requireBufferRange(buf, offset, 46, "zip central directory entry");
    if (buf.readUInt32LE(offset) !== 0x02014b50) {
      throw new Error("invalid zip central directory");
    }
    const compressionMethod = buf.readUInt16LE(offset + 10);
    const compressedSize = buf.readUInt32LE(offset + 20);
    const uncompressedSize = buf.readUInt32LE(offset + 24);
    const fileNameLength = buf.readUInt16LE(offset + 28);
    const extraLength = buf.readUInt16LE(offset + 30);
    const commentLength = buf.readUInt16LE(offset + 32);
    const externalFileAttributes = buf.readUInt32LE(offset + 38);
    const localHeaderOffset = buf.readUInt32LE(offset + 42);
    const fileNameStart = offset + 46;
    requireBufferRange(buf, fileNameStart, fileNameLength + extraLength + commentLength, "zip central directory name");
    const fileName = buf.toString("utf8", fileNameStart, fileNameStart + fileNameLength);
    entries.push({
      fileName,
      externalFileAttributes,
      uncompressedSize,
      compressedSize,
      compressionMethod,
      localHeaderOffset,
    });
    offset = fileNameStart + fileNameLength + extraLength + commentLength;
  }
  if (offset > centralDirectoryOffset + centralDirectorySize) {
    throw new Error("zip central directory entry exceeds declared size");
  }
  return entries;
}

function findEndOfCentralDirectory(buf) {
  if (buf.length < 22) {
    throw new Error("zip end of central directory not found");
  }
  const minOffset = Math.max(0, buf.length - 0xffff - 22);
  for (let offset = buf.length - 22; offset >= minOffset; offset--) {
    if (buf.readUInt32LE(offset) === 0x06054b50) {
      return offset;
    }
  }
  throw new Error("zip end of central directory not found");
}

function requireBufferRange(buf, offset, length, label) {
  if (!Number.isInteger(offset) || !Number.isInteger(length) || offset < 0 || length < 0 || offset + length > buf.length) {
    throw new Error(`${label} is outside artifact bounds`);
  }
}

function extractEntryFromBuffer(buf, entry) {
  const offset = entry.localHeaderOffset;
  requireBufferRange(buf, offset, 30, "zip local file header");
  if (buf.readUInt32LE(offset) !== 0x04034b50) {
    throw new Error("invalid zip local file header");
  }
  const compressionMethod = buf.readUInt16LE(offset + 8);
  const fileNameLength = buf.readUInt16LE(offset + 26);
  const extraLength = buf.readUInt16LE(offset + 28);
  const dataStart = offset + 30 + fileNameLength + extraLength;
  requireBufferRange(buf, offset + 30, fileNameLength + extraLength, "zip local file name");
  requireBufferRange(buf, dataStart, entry.compressedSize, "zip local file data");
  const compressed = buf.subarray(dataStart, dataStart + entry.compressedSize);
  let out;
  if (compressionMethod === 0) {
    out = Buffer.from(compressed);
  } else if (compressionMethod === 8) {
    out = zlib.inflateRawSync(compressed, { maxOutputLength: MAX_FACTS_BYTES });
  } else {
    throw new Error(`unsupported zip compression method: ${compressionMethod}`);
  }
  if (out.length !== entry.uncompressedSize) {
    throw new Error(`facts size mismatch: ${out.length} != ${entry.uncompressedSize}`);
  }
  return out;
}

function verifyArtifactDigest(buf, expectedDigest) {
  if (!expectedDigest) {
    throw new Error("artifact digest is required");
  }
  const match = /^sha256:([a-f0-9]{64})$/i.exec(expectedDigest);
  if (!match) {
    throw new Error(`unsupported artifact digest: ${expectedDigest}`);
  }
  const got = crypto.createHash("sha256").update(buf).digest("hex");
  if (got.toLowerCase() !== match[1].toLowerCase()) {
    throw new Error("facts artifact digest mismatch");
  }
}

function requireArray(facts, key) {
  if (!(key in facts)) {
    return [];
  }
  if (!Array.isArray(facts[key])) {
    throw new Error(`facts JSON ${key} must be an array`);
  }
  if (facts[key].length > MAX_ARRAY_ITEMS) {
    throw new Error(`facts JSON ${key} has too many items`);
  }
  return facts[key];
}

function requireObject(value, path) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new Error(`facts JSON ${path} must be an object`);
  }
  return value;
}

function requireString(value, path, { optional = false } = {}) {
  if (value === undefined || value === null) {
    if (optional) {
      return "";
    }
    throw new Error(`facts JSON ${path} must be a string`);
  }
  if (typeof value !== "string") {
    throw new Error(`facts JSON ${path} must be a string`);
  }
  if (Buffer.byteLength(value, "utf8") > MAX_STRING_BYTES) {
    throw new Error(`facts JSON ${path} is too long`);
  }
  return value;
}

function requireBoolean(value, path, { optional = false } = {}) {
  if (value === undefined || value === null) {
    if (optional) {
      return false;
    }
    throw new Error(`facts JSON ${path} must be a boolean`);
  }
  if (typeof value !== "boolean") {
    throw new Error(`facts JSON ${path} must be a boolean`);
  }
  return value;
}

function requireInteger(value, path, { optional = false, min = 0, max = 1000000 } = {}) {
  if (value === undefined || value === null) {
    if (optional) {
      return 0;
    }
    throw new Error(`facts JSON ${path} must be an integer`);
  }
  if (!Number.isInteger(value) || value < min || value > max) {
    throw new Error(`facts JSON ${path} must be an integer between ${min} and ${max}`);
  }
  return value;
}

function requireLine(value, path) {
  if (!Number.isInteger(value) || value < 0 || value > 1000000) {
    throw new Error(`facts JSON ${path} must be a valid line number`);
  }
}

function requireSafePath(value, path) {
  const file = requireString(value, path);
  if (file.startsWith("/") || file.includes("..") || file.includes("\0")) {
    throw new Error(`facts JSON ${path} must be a repository-relative path`);
  }
  return file;
}

function requirePublicContentFile(value, path) {
  const file = requireString(value, path);
  if (file === "branch" || file === "pull_request_metadata" || /^commit:[0-9a-f]{7,40}$/.test(file)) {
    return file;
  }
  if (file.startsWith("commit:")) {
    throw new Error(`facts JSON ${path} must be a valid public content location`);
  }
  requireSafePath(file, path);
  if (
    file === "" ||
    file === "." ||
    file.startsWith("./") ||
    file.includes("\\") ||
    file.includes("\0") ||
    file.split("/").includes(".git") ||
    /^[A-Za-z][A-Za-z0-9+.-]*:/.test(file)
  ) {
    throw new Error(`facts JSON ${path} must be a repository-relative path`);
  }
  return file;
}

function requirePositiveLine(value, path) {
  requireLine(value, path);
  if (value === 0) {
    throw new Error(`facts JSON ${path} must be a positive line number`);
  }
}

function requireStringArray(value, path, { optional = false } = {}) {
  if (value === undefined || value === null) {
    if (optional) {
      return [];
    }
    throw new Error(`facts JSON ${path} must be an array`);
  }
  if (!Array.isArray(value)) {
    throw new Error(`facts JSON ${path} must be an array`);
  }
  if (value.length > MAX_ARRAY_ITEMS) {
    throw new Error(`facts JSON ${path} has too many items`);
  }
  for (const [i, item] of value.entries()) {
    requireString(item, `${path}[${i}]`);
  }
  return value;
}

function requireJSONValue(value, path, depth = 0) {
  if (depth > MAX_JSON_DEPTH) {
    throw new Error(`facts JSON ${path} is too deeply nested`);
  }
  if (value === null || typeof value === "boolean" || typeof value === "number") {
    return;
  }
  if (typeof value === "string") {
    requireString(value, path);
    return;
  }
  if (Array.isArray(value)) {
    if (value.length > MAX_ARRAY_ITEMS) {
      throw new Error(`facts JSON ${path} has too many items`);
    }
    for (const [i, item] of value.entries()) {
      requireJSONValue(item, `${path}[${i}]`, depth + 1);
    }
    return;
  }
  if (typeof value === "object") {
    const entries = Object.entries(value);
    if (entries.length > MAX_OBJECT_KEYS) {
      throw new Error(`facts JSON ${path} has too many keys`);
    }
    for (const [key, item] of entries) {
      requireString(key, `${path} key`);
      requireJSONValue(item, `${path}.${key}`, depth + 1);
    }
    return;
  }
  throw new Error(`facts JSON ${path} must be a JSON value`);
}

function requireStringArrayMap(value, path, { optional = false } = {}) {
  if (value === undefined || value === null) {
    if (optional) {
      return;
    }
    throw new Error(`facts JSON ${path} must be an object`);
  }
  const obj = requireObject(value, path);
  const entries = Object.entries(obj);
  if (entries.length > MAX_OBJECT_KEYS) {
    throw new Error(`facts JSON ${path} has too many keys`);
  }
  for (const [key, values] of entries) {
    requireString(key, `${path} key`);
    requireStringArray(values, `${path}.${key}`);
  }
}

function verifyDryRunRequest(value, path) {
  if (value === undefined || value === null) {
    return;
  }
  const item = requireObject(value, path);
  requireString(item.method, `${path}.method`);
  requireString(item.url, `${path}.url`);
  requireStringArrayMap(item.query, `${path}.query`, { optional: true });
  if (item.params !== undefined && item.params !== null) {
    requireObject(item.params, `${path}.params`);
    requireJSONValue(item.params, `${path}.params`);
  }
  if (item.body !== undefined && item.body !== null) {
    requireJSONValue(item.body, `${path}.body`);
  }
}

function verifyCommandExample(value, path) {
  const item = requireObject(value, path);
  requireString(item.raw, `${path}.raw`);
  requireSafePath(item.source_file, `${path}.source_file`);
  requireLine(item.line, `${path}.line`);
  requireString(item.command_path, `${path}.command_path`, { optional: true });
  requireString(item.domain, `${path}.domain`, { optional: true });
  requireString(item.source, `${path}.source`, { optional: true });
  requireBoolean(item.changed, `${path}.changed`, { optional: true });
  requireBoolean(item.executable, `${path}.executable`, { optional: true });
  requireString(item.skip_reason, `${path}.skip_reason`, { optional: true });
  requireInteger(item.exit_code, `${path}.exit_code`, { optional: true, min: 0, max: 255 });
  requireInteger(item.stdout_bytes, `${path}.stdout_bytes`, { optional: true });
  requireInteger(item.api_call_count, `${path}.api_call_count`, { optional: true });
  verifyDryRunRequest(item.expected_request, `${path}.expected_request`);
  verifyDryRunRequest(item.dry_run, `${path}.dry_run`);
}

function verifyFactsJSON(data) {
  let facts;
  try {
    facts = JSON.parse(data.toString("utf8"));
  } catch (err) {
    throw new Error(`facts JSON is invalid: ${err.message}`);
  }
  if (!facts || typeof facts !== "object" || Array.isArray(facts)) {
    throw new Error("facts JSON must be an object");
  }
  if (facts.schema_version !== 1) {
    throw new Error("facts JSON schema_version must be 1");
  }
  for (const [i, value] of requireArray(facts, "commands").entries()) {
    const item = requireObject(value, `commands[${i}]`);
    requireString(item.path, `commands[${i}].path`);
    requireString(item.canonical_path, `commands[${i}].canonical_path`, { optional: true });
    requireString(item.domain, `commands[${i}].domain`, { optional: true });
    requireBoolean(item.changed, `commands[${i}].changed`, { optional: true });
    requireString(item.source, `commands[${i}].source`);
    requireBoolean(item.generated, `commands[${i}].generated`, { optional: true });
    requireStringArray(item.flags, `commands[${i}].flags`, { optional: true });
    for (const [j, example] of requireArray(item, "examples").entries()) {
      verifyCommandExample(example, `commands[${i}].examples[${j}]`);
    }
    requireBoolean(item.legacy_naming, `commands[${i}].legacy_naming`, { optional: true });
    requireBoolean(item.name_conflicts_existing, `commands[${i}].name_conflicts_existing`, { optional: true });
    requireBoolean(item.flag_alias_conflict, `commands[${i}].flag_alias_conflict`, { optional: true });
  }
  for (const [i, value] of requireArray(facts, "skills").entries()) {
    const item = requireObject(value, `skills[${i}]`);
    requireSafePath(item.source_file, `skills[${i}].source_file`);
    requireLine(item.line, `skills[${i}].line`);
    requireString(item.raw, `skills[${i}].raw`);
    requireString(item.command_path, `skills[${i}].command_path`, { optional: true });
    requireString(item.domain, `skills[${i}].domain`, { optional: true });
    requireBoolean(item.changed, `skills[${i}].changed`, { optional: true });
    requireString(item.source, `skills[${i}].source`, { optional: true });
    requireBoolean(item.references_invalid_command, `skills[${i}].references_invalid_command`, { optional: true });
    requireBoolean(item.destructive_without_guard, `skills[${i}].destructive_without_guard`, { optional: true });
    requireBoolean(item.scope_conflict, `skills[${i}].scope_conflict`, { optional: true });
  }
  for (const [i, value] of requireArray(facts, "skill_quality").entries()) {
    const item = requireObject(value, `skill_quality[${i}]`);
    requireSafePath(item.source_file, `skill_quality[${i}].source_file`);
    requireString(item.domain, `skill_quality[${i}].domain`, { optional: true });
    requireBoolean(item.changed, `skill_quality[${i}].changed`, { optional: true });
    requireInteger(item.word_count, `skill_quality[${i}].word_count`, { optional: true });
    requireInteger(item.critical_count, `skill_quality[${i}].critical_count`, { optional: true });
    requireInteger(item.description_length, `skill_quality[${i}].description_length`, { optional: true });
    requireBoolean(item.critical_over_budget, `skill_quality[${i}].critical_over_budget`, { optional: true });
  }
  for (const [i, value] of requireArray(facts, "errors").entries()) {
    const item = requireObject(value, `errors[${i}]`);
    requireSafePath(item.file, `errors[${i}].file`);
    requireLine(item.line, `errors[${i}].line`);
    requireString(item.command, `errors[${i}].command`, { optional: true });
    requireString(item.command_path, `errors[${i}].command_path`, { optional: true });
    requireString(item.domain, `errors[${i}].domain`, { optional: true });
    requireBoolean(item.changed, `errors[${i}].changed`, { optional: true });
    requireString(item.source, `errors[${i}].source`, { optional: true });
    requireBoolean(item.boundary, `errors[${i}].boundary`, { optional: true });
    requireBoolean(item.uses_structured_error, `errors[${i}].uses_structured_error`, { optional: true });
    requireBoolean(item.has_hint, `errors[${i}].has_hint`, { optional: true });
    requireInteger(item.hint_action_count, `errors[${i}].hint_action_count`, { optional: true });
    requireBoolean(item.required_hint, `errors[${i}].required_hint`, { optional: true });
    requireString(item.code, `errors[${i}].code`, { optional: true });
    requireString(item.message, `errors[${i}].message`, { optional: true });
    requireString(item.hint, `errors[${i}].hint`, { optional: true });
    requireBoolean(item.retryable, `errors[${i}].retryable`, { optional: true });
  }
  for (const [i, value] of requireArray(facts, "outputs").entries()) {
    const item = requireObject(value, `outputs[${i}]`);
    requireString(item.command, `outputs[${i}].command`);
    requireString(item.domain, `outputs[${i}].domain`, { optional: true });
    requireBoolean(item.changed, `outputs[${i}].changed`, { optional: true });
    requireString(item.source, `outputs[${i}].source`, { optional: true });
    requireStringArray(item.fields, `outputs[${i}].fields`, { optional: true });
    requireBoolean(item.is_list, `outputs[${i}].is_list`, { optional: true });
    requireBoolean(item.has_default_limit, `outputs[${i}].has_default_limit`, { optional: true });
    requireBoolean(item.has_field_selector, `outputs[${i}].has_field_selector`, { optional: true });
    requireBoolean(item.has_decision_field, `outputs[${i}].has_decision_field`, { optional: true });
  }
  for (const [i, value] of requireArray(facts, "examples").entries()) {
    verifyCommandExample(value, `examples[${i}]`);
  }
  for (const [i, value] of requireArray(facts, "public_content").entries()) {
    const item = requireObject(value, `public_content[${i}]`);
    requireString(item.rule, `public_content[${i}].rule`);
    const action = requireString(item.action, `public_content[${i}].action`);
    if (!VALID_ACTIONS.has(action)) {
      throw new Error(`facts JSON public_content[${i}].action is invalid`);
    }
    requirePublicContentFile(item.file, `public_content[${i}].file`);
    requirePositiveLine(item.line, `public_content[${i}].line`);
    requireString(item.source, `public_content[${i}].source`, { optional: true });
    requireString(item.excerpt, `public_content[${i}].excerpt`, { optional: true });
    requireString(item.message, `public_content[${i}].message`, { optional: true });
    requireString(item.suggestion, `public_content[${i}].suggestion`, { optional: true });
  }
  for (const [i, value] of requireArray(facts, "diagnostics").entries()) {
    const item = requireObject(value, `diagnostics[${i}]`);
    requireString(item.rule, `diagnostics[${i}].rule`);
    const action = requireString(item.action, `diagnostics[${i}].action`);
    if (!VALID_ACTIONS.has(action)) {
      throw new Error(`facts JSON diagnostics[${i}].action is invalid`);
    }
    requireSafePath(item.file, `diagnostics[${i}].file`);
    requireLine(item.line, `diagnostics[${i}].line`);
    requireString(item.message, `diagnostics[${i}].message`);
    requireString(item.suggestion, `diagnostics[${i}].suggestion`, { optional: true });
    requireString(item.subject_type, `diagnostics[${i}].subject_type`, { optional: true });
    requireString(item.command_path, `diagnostics[${i}].command_path`, { optional: true });
    requireString(item.flag_name, `diagnostics[${i}].flag_name`, { optional: true });
  }
}

function writeVerifiedFacts(zipPath, outPath, expectedDigest = "") {
  const buf = fs.readFileSync(zipPath);
  verifyArtifactDigest(buf, expectedDigest);
  const entry = verifyZipEntries(readZipEntriesFromBuffer(buf));
  const data = extractEntryFromBuffer(buf, entry);
  verifyFactsJSON(data);
  fs.writeFileSync(outPath, data);
  return entry;
}

function verificationFailureDecision(message, blockMode) {
  return {
    block_mode: blockMode,
    degraded: true,
    infrastructure_failure: true,
    system_warnings: [{
      severity: "critical",
      message: `quality-gate facts artifact verification failed: ${message}`,
      suggested_action: "inspect the semantic-review workflow artifact verification logs and rerun CI after the artifact issue is resolved",
    }],
    blockers: [],
    warnings: [],
  };
}

function writeFailureDecisionFromEnv(err) {
  const decisionOut = process.env.SEMANTIC_REVIEW_DECISION_OUT || "";
  if (!decisionOut) {
    return;
  }
  const blockMode = process.env.SEMANTIC_REVIEW_BLOCK === "true";
  const message = err && err.message ? err.message : String(err || "unknown error");
  const decision = verificationFailureDecision(message, blockMode);
  fs.writeFileSync(decisionOut, JSON.stringify(decision, null, 2) + "\n", "utf8");
  const markdownOut = process.env.SEMANTIC_REVIEW_MARKDOWN_OUT || "";
  if (markdownOut) {
    fs.writeFileSync(markdownOut, [
      "## Semantic Review",
      "",
      "The semantic review system could not produce a fully trusted result.",
      "",
      `- ${decision.system_warnings[0].message}`,
      `- Action: ${decision.system_warnings[0].suggested_action}`,
      "",
    ].join("\n"), "utf8");
  }
}

if (require.main === module) {
  const [zipPath, outPath = "facts.json", expectedDigest = ""] = process.argv.slice(2);
  if (!zipPath) {
    console.error("usage: node scripts/semantic-review-verify-artifact.js <artifact.zip> [facts.json] [sha256:<digest>]");
    process.exit(2);
  }
  try {
    writeVerifiedFacts(zipPath, outPath, expectedDigest);
  } catch (err) {
    console.error(`semantic-review artifact verifier: ${err.message}`);
    try {
      writeFailureDecisionFromEnv(err);
    } catch (writeErr) {
      console.error(`semantic-review artifact verifier: failed to write infrastructure decision: ${writeErr.message}`);
    }
    process.exit(1);
  }
}

module.exports = { MAX_FACTS_BYTES, verifyArtifactDigest, verifyZipEntries, verifyFactsJSON, readZipEntries, extractEntryFromBuffer, writeVerifiedFacts, verificationFailureDecision };
