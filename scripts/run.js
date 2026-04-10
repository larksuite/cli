#!/usr/bin/env node
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { execFileSync } = require("child_process");
const fs = require("fs");
const path = require("path");

const ext = process.platform === "win32" ? ".exe" : "";
const bin = path.join(__dirname, "..", "bin", "lark-cli" + ext);

// On Windows, a crashed self-update may have left the binary renamed to .old.
// Recover it before proceeding so the CLI remains functional.
const oldBin = bin + ".old";
if (!fs.existsSync(bin) && fs.existsSync(oldBin)) {
  try {
    fs.renameSync(oldBin, bin);
  } catch (_) {
    // Best-effort; fall through to the normal "not found" error below.
  }
}

if (!fs.existsSync(bin)) {
  console.error(
    `Error: lark-cli binary not found at ${bin}\n\n` +
    `This usually means the postinstall script was skipped.\n` +
    `Common causes:\n` +
    `  - npm is configured with ignore-scripts=true\n` +
    `  - The postinstall download failed\n\n` +
    `To fix, run the install script manually:\n` +
    `  node "${path.join(__dirname, "install.js")}"\n`
  );
  process.exit(1);
}

try {
  execFileSync(bin, process.argv.slice(2), { stdio: "inherit" });
} catch (e) {
  process.exit(e.status || 1);
}
