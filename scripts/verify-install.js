#!/usr/bin/env node

// One-time manual verification script for install.js.
// Downloads real checksums.txt from GitHub Release and runs install.js
// against it using LARK_CLI_TEST_* env vars for isolation.
//
// Prerequisites: gh CLI authenticated, network access to GitHub.
// Usage: node scripts/verify-install.js

const fs = require("fs");
const path = require("path");
const os = require("os");
const { execFileSync } = require("child_process");

const VERSION = require("../package.json").version;
const REPO = "larksuite/cli";

const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "verify-install-"));
const outputDir = path.join(tmpDir, "bin");

function cleanup() {
  fs.rmSync(tmpDir, { recursive: true, force: true });
}

try {
  console.log(`Verifying install.js against GitHub Release v${VERSION}...`);
  console.log(`Working directory: ${tmpDir}`);

  // Download checksums.txt from the real release
  execFileSync("gh", [
    "release", "download", `v${VERSION}`,
    "--repo", REPO,
    "--pattern", "checksums.txt",
    "--dir", tmpDir,
  ], { stdio: "inherit" });

  const checksumPath = path.join(tmpDir, "checksums.txt");
  if (!fs.existsSync(checksumPath)) {
    throw new Error("checksums.txt not found after download");
  }

  // Run install.js with test mode env vars
  fs.mkdirSync(outputDir, { recursive: true });
  execFileSync(process.execPath, [path.join(__dirname, "install.js")], {
    env: {
      ...process.env,
      LARK_CLI_TEST_MODE: "1",
      LARK_CLI_TEST_CHECKSUM_PATH: checksumPath,
      LARK_CLI_TEST_OUTPUT_DIR: outputDir,
    },
    stdio: "inherit",
  });

  // Verify binary exists
  const binaryName = "lark-cli" + (process.platform === "win32" ? ".exe" : "");
  const binaryPath = path.join(outputDir, binaryName);

  if (!fs.existsSync(binaryPath)) {
    throw new Error(`Binary not found at ${binaryPath}`);
  }

  // Verify binary runs
  const versionOutput = execFileSync(binaryPath, ["--version"], {
    encoding: "utf-8",
    timeout: 10000,
  }).trim();
  console.log(`Binary version output: ${versionOutput}`);

  cleanup();
  console.log("\nPASS: install.js verification succeeded");
  process.exit(0);
} catch (err) {
  cleanup();
  console.error(`\nFAIL: ${err.message}`);
  process.exit(1);
}
