// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { execFileSync } = require("node:child_process");
const test = require("node:test");

const scriptPath = path.join(__dirname, "e2e_domains.js");

function parseOutput(raw) {
  const result = {};
  for (const line of raw.trim().split(/\r?\n/)) {
    const idx = line.indexOf("=");
    if (idx === -1) continue;
    result[line.slice(0, idx)] = line.slice(idx + 1);
  }
  return result;
}

function runDomains(files) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "e2e-domains-"));
  const file = path.join(dir, "changed.txt");
  fs.writeFileSync(file, `${files.join("\n")}\n`);
  try {
    return parseOutput(execFileSync(process.execPath, [scriptPath], {
      cwd: path.join(__dirname, ".."),
      encoding: "utf8",
      env: { ...process.env, E2E_DOMAIN_CHANGED_FILES: file },
    }));
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

test("maps shortcut changes to one business domain package", () => {
  const output = runDomains(["shortcuts/im/messages/send.go"]);
  assert.equal(output.mode, "subset");
  assert.equal(output.domains, "im");
  assert.match(output.dry_root_package, /github\.com\/larksuite\/cli\/tests\/cli_e2e$/);
  assert.match(output.live_packages, /github\.com\/larksuite\/cli\/tests\/cli_e2e\/im/);
  assert.doesNotMatch(output.live_packages, /github\.com\/larksuite\/cli\/tests\/cli_e2e\/drive/);
});

test("maps doc shortcuts to docs package", () => {
  const output = runDomains(["shortcuts/doc/update.go"]);
  assert.equal(output.mode, "subset");
  assert.equal(output.domains, "docs");
  assert.match(output.live_packages, /github\.com\/larksuite\/cli\/tests\/cli_e2e\/docs/);
});

test("maps direct e2e domain package changes", () => {
  const output = runDomains(["tests/cli_e2e/drive/helpers.go"]);
  assert.equal(output.mode, "subset");
  assert.equal(output.domains, "drive");
  assert.match(output.live_packages, /github\.com\/larksuite\/cli\/tests\/cli_e2e\/drive/);
});

test("falls back to full for shared e2e harness changes", () => {
  const output = runDomains(["tests/cli_e2e/core.go"]);
  assert.equal(output.mode, "full");
  assert.equal(output.domains, "all");
  assert.match(output.reason, /shared CLI E2E harness changed/);
});

test("falls back to full for runtime changes", () => {
  const output = runDomains(["cmd/root.go"]);
  assert.equal(output.mode, "full");
  assert.equal(output.domains, "all");
  assert.match(output.reason, /shared\/runtime path changed/);
});

test("skips docs-only changes", () => {
  const output = runDomains(["docs/usage.md", "README.md"]);
  assert.equal(output.mode, "skip");
  assert.equal(output.domains, "");
  assert.equal(output.dry_root_package, "");
  assert.equal(output.live_packages, "");
});

test("uses shared map for skill domain changes", () => {
  const output = runDomains(["skills/lark-sheets/SKILL.md"]);
  assert.equal(output.mode, "subset");
  assert.equal(output.domains, "sheets");
  assert.match(output.live_packages, /github\.com\/larksuite\/cli\/tests\/cli_e2e\/sheets/);
});

test("falls back to full when a mapped path has no e2e package", () => {
  const output = runDomains(["shortcuts/whiteboard/export.go"]);
  assert.equal(output.mode, "full");
  assert.match(output.reason, /unmapped CLI E2E domain path/);
});
