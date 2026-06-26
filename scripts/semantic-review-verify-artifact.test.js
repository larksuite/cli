// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");
const childProcess = require("node:child_process");
const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const zlib = require("node:zlib");

const { MAX_FACTS_BYTES, extractEntryFromBuffer, verifyArtifactDigest, verifyZipEntries, writeVerifiedFacts } = require("./semantic-review-verify-artifact.js");

describe("verifyZipEntries", () => {
  it("rejects path traversal and symlink entries", () => {
    const badEntries = [
      { fileName: "../facts.json", externalFileAttributes: 0, compressedSize: 10, uncompressedSize: 10 },
      { fileName: "facts.json", externalFileAttributes: 0o120000 << 16, compressedSize: 10, uncompressedSize: 10 },
      { fileName: "facts.json", externalFileAttributes: 0o040000 << 16, compressedSize: 10, uncompressedSize: 10 },
    ];
    for (const entry of badEntries) {
      assert.throws(() => verifyZipEntries([entry]));
    }
  });

  it("rejects multi-file and oversized artifacts", () => {
    const entry = { fileName: "facts.json", externalFileAttributes: 0o100644 << 16, compressedSize: 100, uncompressedSize: 100 };
    assert.throws(() => verifyZipEntries([entry, entry]));
    assert.throws(() => verifyZipEntries([{ ...entry, uncompressedSize: MAX_FACTS_BYTES + 1 }]));
  });

  it("rejects suspicious compression ratios", () => {
    const entry = { fileName: "facts.json", externalFileAttributes: 0o100644 << 16, compressedSize: 1, uncompressedSize: 1000 };
    assert.throws(() => verifyZipEntries([entry]), /compression ratio/);
  });

  it("accepts exactly one regular facts file", () => {
    const entry = { fileName: "facts.json", externalFileAttributes: 0o100644 << 16, compressedSize: 100, uncompressedSize: 100 };
    assert.equal(verifyZipEntries([entry]), entry);
  });

  it("validates artifact sha256 digest when provided", () => {
    const buf = Buffer.from("artifact");
    const digest = crypto.createHash("sha256").update(buf).digest("hex");
    assert.throws(() => verifyArtifactDigest(buf, ""), /artifact digest is required/);
    assert.doesNotThrow(() => verifyArtifactDigest(buf, `sha256:${digest}`));
    assert.throws(() => verifyArtifactDigest(buf, `sha256:${"0".repeat(64)}`), /digest mismatch/);
    assert.throws(() => verifyArtifactDigest(buf, "md5:bad"), /unsupported artifact digest/);
  });

  it("caps deflated facts extraction before zip size mismatch checks", () => {
    const header = Buffer.alloc(30);
    header.writeUInt32LE(0x04034b50, 0);
    header.writeUInt16LE(8, 8);
    const compressed = zlib.deflateRawSync(Buffer.alloc(MAX_FACTS_BYTES + 1, "x"));
    const entry = {
      localHeaderOffset: 0,
      compressedSize: compressed.length,
      uncompressedSize: MAX_FACTS_BYTES,
    };

    assert.throws(() => extractEntryFromBuffer(Buffer.concat([header, compressed]), entry));
  });

  it("extracts facts from a real zip buffer", () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-zip-"));
    const zipPath = path.join(dir, "facts.zip");
    const outPath = path.join(dir, "facts.json");
    const restrictedScope = "pri" + "vate";
    const facts = Buffer.from(JSON.stringify({
      schema_version: 1,
      public_content: [
        {
          rule: "public_content_semantic_candidate",
          action: "WARNING",
          file: "pull_request_metadata",
          line: 1,
          source: "metadata",
          excerpt: "public release notes mention an internal rollout plan",
          message: "public contribution may contain sensitive implementation detail",
          suggestion: "move internal detail to " + restrictedScope + " discussion",
        },
        {
          rule: "public_content_change_id_trailer",
          action: "REJECT",
          file: "commit:1234abc",
          line: 3,
          source: "commit",
        },
        {
          rule: "public_content_automation_branch",
          action: "WARNING",
          file: "branch",
          line: 1,
          source: "branch",
        },
        {
          rule: "public_content_" + "pri" + "vate_ipv4",
          action: "WARNING",
          file: "docs/public-network.md",
          line: 7,
          source: "file",
        },
      ],
    }) + "\n");
    const zip = makeZip([{ fileName: "facts.json", data: facts, mode: 0o100644 }]);
    fs.writeFileSync(zipPath, zip);

    writeVerifiedFacts(zipPath, outPath, digestFor(zip));

    assert.equal(fs.readFileSync(outPath, "utf8"), facts.toString("utf8"));
  });

  it("rejects malformed zip boundaries with a controlled error", () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-zip-"));
    const zipPath = path.join(dir, "facts.zip");
    const outPath = path.join(dir, "facts.json");
    const zip = Buffer.from([0x50, 0x4b, 0x05, 0x06]);
    fs.writeFileSync(zipPath, zip);

    assert.throws(
      () => writeVerifiedFacts(zipPath, outPath, digestFor(zip)),
      /zip end of central directory|zip central directory|zip bounds/,
    );
  });

  it("rejects invalid facts JSON shape", () => {
    for (const [name, facts, want] of [
      ["not-json", Buffer.from("{"), /facts JSON is invalid/],
      ["array", Buffer.from("[]"), /facts JSON must be an object/],
      ["wrong-schema", Buffer.from('{"schema_version":2}'), /schema_version/],
      ["non-array-skills", Buffer.from('{"schema_version":1,"skills":{}}'), /skills must be an array/],
      ["bad-skill-path", Buffer.from('{"schema_version":1,"skills":[{"source_file":"../x","line":1,"raw":"x","references_invalid_command":true}]}'), /source_file/],
      ["bad-skill-line", Buffer.from('{"schema_version":1,"skills":[{"source_file":"skills/lark-doc/SKILL.md","line":"3","raw":"x","references_invalid_command":true}]}'), /line/],
      ["bad-command-item", Buffer.from('{"schema_version":1,"commands":["not-object"]}'), /commands\[0\]/],
      ["bad-command-flags", Buffer.from('{"schema_version":1,"commands":[{"path":"docs +fetch","source":"shortcut","flags":["ok",1]}]}'), /commands\[0\]\.flags\[1\]/],
      ["bad-skill-quality-path", Buffer.from('{"schema_version":1,"skill_quality":[{"source_file":"/tmp/SKILL.md","word_count":1,"critical_count":0,"description_length":10}]}'), /skill_quality\[0\]\.source_file/],
      ["bad-error-path", Buffer.from('{"schema_version":1,"errors":[{"file":"../x.go","line":1,"boundary":true,"uses_structured_error":false,"has_hint":false,"hint_action_count":0,"required_hint":true,"retryable":false}]}'), /errors\[0\]\.file/],
      ["bad-example-dry-run", Buffer.from('{"schema_version":1,"examples":[{"raw":"lark-cli docs +fetch","source_file":"skills/lark-doc/SKILL.md","line":3,"executable":true,"dry_run":{"method":"GET","url":"/open-apis/docx","query":{"page_size":["20",1]}}}]}'), /examples\[0\]\.dry_run\.query\.page_size\[1\]/],
      ["bad-output-field", Buffer.from(JSON.stringify({ schema_version: 1, outputs: [{ command: "drive files list", fields: ["ok", "x".repeat(9000)] }] })), /outputs\[0\]\.fields\[1\]/],
      ["non-array-public-content", Buffer.from('{"schema_version":1,"public_content":{}}'), /public_content must be an array/],
      ["bad-public-content-item", Buffer.from('{"schema_version":1,"public_content":["not-object"]}'), /public_content\[0\]/],
      ["bad-public-content-action", Buffer.from('{"schema_version":1,"public_content":[{"rule":"public_content_semantic_candidate","action":"BLOCK","file":"pull_request_metadata","line":1}]}'), /public_content\[0\]\.action/],
      ["bad-public-content-path", Buffer.from('{"schema_version":1,"public_content":[{"rule":"public_content_semantic_candidate","action":"WARNING","file":"../x","line":1}]}'), /public_content\[0\]\.file/],
      ["dot-slash-public-content-path", Buffer.from('{"schema_version":1,"public_content":[{"rule":"public_content_semantic_candidate","action":"WARNING","file":"./foo","line":1}]}'), /public_content\[0\]\.file/],
      ["empty-public-content-path", Buffer.from('{"schema_version":1,"public_content":[{"rule":"public_content_semantic_candidate","action":"WARNING","file":"","line":1}]}'), /public_content\[0\]\.file/],
      ["dot-public-content-path", Buffer.from('{"schema_version":1,"public_content":[{"rule":"public_content_semantic_candidate","action":"WARNING","file":".","line":1}]}'), /public_content\[0\]\.file/],
      ["url-public-content-path", Buffer.from('{"schema_version":1,"public_content":[{"rule":"public_content_semantic_candidate","action":"WARNING","file":"https://example.invalid/x","line":1}]}'), /public_content\[0\]\.file/],
      ["dotgit-public-content-path", Buffer.from('{"schema_version":1,"public_content":[{"rule":"public_content_semantic_candidate","action":"WARNING","file":".git/config","line":1}]}'), /public_content\[0\]\.file/],
      ["windows-public-content-path", Buffer.from('{"schema_version":1,"public_content":[{"rule":"public_content_semantic_candidate","action":"WARNING","file":"C:\\\\tmp\\\\x","line":1}]}'), /public_content\[0\]\.file/],
      ["bad-public-content-commit-ref", Buffer.from('{"schema_version":1,"public_content":[{"rule":"public_content_change_id_trailer","action":"REJECT","file":"commit:notasha","line":1}]}'), /public_content\[0\]\.file/],
      ["bad-public-content-line", Buffer.from('{"schema_version":1,"public_content":[{"rule":"public_content_semantic_candidate","action":"WARNING","file":"pull_request_metadata","line":"1"}]}'), /public_content\[0\]\.line/],
      ["zero-public-content-line", Buffer.from('{"schema_version":1,"public_content":[{"rule":"public_content_semantic_candidate","action":"WARNING","file":"pull_request_metadata","line":0}]}'), /public_content\[0\]\.line/],
      ["bad-diagnostic-action", Buffer.from('{"schema_version":1,"diagnostics":[{"rule":"r","action":"BLOCK","file":"x.go","line":1,"message":"m"}]}'), /diagnostics.*action/],
      ["long-message", Buffer.from(JSON.stringify({ schema_version: 1, diagnostics: [{ rule: "r", action: "REJECT", file: "x.go", line: 1, message: "x".repeat(9000) }] })), /too long/],
    ]) {
      const dir = fs.mkdtempSync(path.join(os.tmpdir(), `facts-shape-${name}-`));
      const zipPath = path.join(dir, "facts.zip");
      const outPath = path.join(dir, "facts.json");
      const zip = makeZip([{ fileName: "facts.json", data: facts, mode: 0o100644 }]);
      fs.writeFileSync(zipPath, zip);
      assert.throws(() => writeVerifiedFacts(zipPath, outPath, digestFor(zip)), want);
    }
  });

  it("rejects invalid entries through real zip parsing", () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-zip-"));
    for (const [name, zip] of [
      ["duplicate", makeZip([
        { fileName: "facts.json", data: Buffer.from("{}"), mode: 0o100644 },
        { fileName: "facts.json", data: Buffer.from("{}"), mode: 0o100644 },
      ])],
      ["path-traversal", makeZip([{ fileName: "../facts.json", data: Buffer.from("{}"), mode: 0o100644 }])],
      ["symlink", makeZip([{ fileName: "facts.json", data: Buffer.from("target"), mode: 0o120000 }])],
    ]) {
      const zipPath = path.join(dir, `${name}.zip`);
      fs.writeFileSync(zipPath, zip);
      assert.throws(() => writeVerifiedFacts(zipPath, path.join(dir, `${name}.json`), digestFor(zip)), /artifact|path|symlink|regular/);
    }
  });

  it("writes an infrastructure decision when CLI verification fails", () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "semantic-review-zip-"));
    const zipPath = path.join(dir, "facts.zip");
    const outPath = path.join(dir, "facts.json");
    const decisionPath = path.join(dir, "decision.json");
    const zip = makeZip([{ fileName: "../facts.json", data: Buffer.from("{}"), mode: 0o100644 }]);
    fs.writeFileSync(zipPath, zip);

    const result = childProcess.spawnSync(process.execPath, [path.join(__dirname, "semantic-review-verify-artifact.js"), zipPath, outPath, digestFor(zip)], {
      env: {
        ...process.env,
        SEMANTIC_REVIEW_BLOCK: "true",
        SEMANTIC_REVIEW_DECISION_OUT: decisionPath,
      },
      encoding: "utf8",
    });

    assert.equal(result.status, 1);
    assert.match(result.stderr, /invalid artifact path/);
    const decision = JSON.parse(fs.readFileSync(decisionPath, "utf8"));
    assert.equal(decision.block_mode, true);
    assert.equal(decision.infrastructure_failure, true);
    assert.match(decision.system_warnings[0].message, /invalid artifact path/);
  });
});

function digestFor(buf) {
  const digest = crypto.createHash("sha256").update(buf).digest("hex");
  return `sha256:${digest}`;
}

function makeZip(entries) {
  const locals = [];
  const centrals = [];
  let offset = 0;
  for (const entry of entries) {
    const name = Buffer.from(entry.fileName);
    const data = Buffer.from(entry.data);
    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50, 0);
    local.writeUInt16LE(20, 4);
    local.writeUInt16LE(0, 6);
    local.writeUInt16LE(0, 8);
    local.writeUInt32LE(0, 10);
    local.writeUInt32LE(0, 14);
    local.writeUInt32LE(data.length, 18);
    local.writeUInt32LE(data.length, 22);
    local.writeUInt16LE(name.length, 26);
    local.writeUInt16LE(0, 28);
    locals.push(local, name, data);

    const central = Buffer.alloc(46);
    central.writeUInt32LE(0x02014b50, 0);
    central.writeUInt16LE(0x0314, 4);
    central.writeUInt16LE(20, 6);
    central.writeUInt16LE(0, 8);
    central.writeUInt16LE(0, 10);
    central.writeUInt32LE(0, 12);
    central.writeUInt32LE(0, 16);
    central.writeUInt32LE(data.length, 20);
    central.writeUInt32LE(data.length, 24);
    central.writeUInt16LE(name.length, 28);
    central.writeUInt16LE(0, 30);
    central.writeUInt16LE(0, 32);
    central.writeUInt16LE(0, 34);
    central.writeUInt16LE(0, 36);
    central.writeUInt32LE((entry.mode || 0o100644) * 0x10000, 38);
    central.writeUInt32LE(offset, 42);
    centrals.push(central, name);

    offset += local.length + name.length + data.length;
  }
  const centralOffset = offset;
  const centralDirectory = Buffer.concat(centrals);
  const eocd = Buffer.alloc(22);
  eocd.writeUInt32LE(0x06054b50, 0);
  eocd.writeUInt16LE(0, 4);
  eocd.writeUInt16LE(0, 6);
  eocd.writeUInt16LE(entries.length, 8);
  eocd.writeUInt16LE(entries.length, 10);
  eocd.writeUInt32LE(centralDirectory.length, 12);
  eocd.writeUInt32LE(centralOffset, 16);
  eocd.writeUInt16LE(0, 20);
  return Buffer.concat([...locals, centralDirectory, eocd]);
}
