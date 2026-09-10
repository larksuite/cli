#!/usr/bin/env node
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Launcher shipped as scripts/run.js of the internal package. The package
// carries one prebuilt binary per platform under bin/, so nothing is
// downloaded at install time; this picks the right one and hands over.
//
// `lark-cli update` is handled here instead of by the binary. The binary is
// the unmodified public build, whose updater knows only the public
// @larksuite/cli and would install it over this package. The launcher does the
// same three steps against this package's own registry: read the `latest`
// version, `npm install -g` it, check `lark-cli --version`. Afterwards it
// syncs the public skills bundle exactly like the public updater. Only npm
// installs are updated automatically; a pnpm install is told the command to
// run. The public update and skills notifiers are switched off unless the
// caller set them, because they would point at the public updater.

const fs = require("fs");
const http = require("http");
const https = require("https");
const path = require("path");
const { spawnSync } = require("child_process");

const PLATFORMS = { darwin: "darwin", linux: "linux", win32: "windows" };
const ARCHES = { x64: "amd64", arm64: "arm64", riscv64: "riscv64" };
const SKILLS_SOURCE = "https://open.feishu.cn/lark-cli/skills/regular";
const SKILLS_SOURCE_FALLBACK = "larksuite/cli";
const FETCH_TIMEOUT_MS = 15000;
const INSTALL_TIMEOUT_MS = 10 * 60 * 1000;
const SKILLS_TIMEOUT_MS = 5 * 60 * 1000;

/**
 * File name of the binary for a platform, or null when the package has no
 * build for it.
 */
function binaryName(platform = process.platform, arch = process.arch) {
  const goos = PLATFORMS[platform];
  const goarch = ARCHES[arch];
  if (!goos || !goarch) return null;
  return `lark-cli-${goos}-${goarch}${goos === "windows" ? ".exe" : ""}`;
}

/** Orders X.Y.Z and X.Y.Z-<pre> versions; a prerelease sorts before its release. */
function compareVersions(a, b) {
  const parse = (v) => {
    const m = /^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?$/.exec(String(v).trim());
    if (!m) return null;
    return { core: [Number(m[1]), Number(m[2]), Number(m[3])], pre: m[4] ? m[4].split(".") : null };
  };
  const pa = parse(a);
  const pb = parse(b);
  if (!pa || !pb) throw new Error(`cannot compare versions ${JSON.stringify(a)} and ${JSON.stringify(b)}`);
  for (let i = 0; i < 3; i++) if (pa.core[i] !== pb.core[i]) return pa.core[i] < pb.core[i] ? -1 : 1;
  if (!pa.pre && !pb.pre) return 0;
  if (!pa.pre) return 1;
  if (!pb.pre) return -1;
  for (let i = 0; i < Math.max(pa.pre.length, pb.pre.length); i++) {
    const x = pa.pre[i];
    const y = pb.pre[i];
    if (x === y) continue;
    if (x === undefined) return -1;
    if (y === undefined) return 1;
    const nx = /^\d+$/.test(x) ? Number(x) : null;
    const ny = /^\d+$/.test(y) ? Number(y) : null;
    if (nx !== null && ny !== null) return nx < ny ? -1 : 1;
    if (nx !== null) return -1;
    if (ny !== null) return 1;
    return x < y ? -1 : 1;
  }
  return 0;
}

/** A pnpm install lives in a `.pnpm` virtual store or under `pnpm/store`. */
function isPnpmInstall(dir) {
  const parts = dir.split(/[\\/]+/);
  return parts.some((part, i) => part === ".pnpm" || (part === "pnpm" && parts[i + 1] === "store"));
}

function fetchLatestVersion(registry, name) {
  const url = `${registry}/${name}/latest`;
  const client = url.startsWith("http:") ? http : https;
  return new Promise((resolve, reject) => {
    const req = client.get(url, { timeout: FETCH_TIMEOUT_MS }, (res) => {
      let body = "";
      res.setEncoding("utf8");
      res.on("data", (chunk) => { if (body.length < 1 << 20) body += chunk; });
      res.on("end", () => {
        if (res.statusCode !== 200) return reject(new Error(`registry returned HTTP ${res.statusCode} for ${url}`));
        try {
          const version = JSON.parse(body).version;
          if (typeof version !== "string" || !version) throw new Error("no version field");
          resolve(version);
        } catch (err) {
          reject(new Error(`registry response for ${url} is not a version manifest: ${err.message}`));
        }
      });
    });
    req.on("timeout", () => req.destroy(new Error(`registry request to ${url} timed out`)));
    req.on("error", reject);
  });
}

function parseUpdateArgs(args) {
  const opts = { check: false, json: false };
  for (const arg of args) {
    if (arg === "--check") opts.check = true;
    else if (arg === "--json") opts.json = true;
    else throw new Error(`unknown flag ${arg}; lark-cli update accepts --check and --json`);
  }
  return opts;
}

function runCommand(command, args, timeout) {
  return spawnSync(command, args, { encoding: "utf8", timeout, shell: process.platform === "win32" });
}

/**
 * Reinstall instructions for a preview build, from the release identity its
 * package.json carries. Generic text when the package names none.
 */
function leavePreviewHint(preview) {
  const name = preview && preview.releasePackage;
  const registry = preview && preview.releaseRegistry;
  if (!name || !registry) return "Reinstall the released package to leave this preview build.";
  return `To leave this preview build:\n  npm install -g ${name}@latest --registry ${registry}`;
}

async function update(args, pkg, packageDir) {
  const opts = parseUpdateArgs(args);
  const registry = pkg.publishConfig && pkg.publishConfig.registry;
  const emit = (out) => {
    if (opts.json) process.stdout.write(JSON.stringify(out, null, 2) + "\n");
    else if (out.message) process.stderr.write(out.message + "\n");
  };

  // A preview build has no publish target. It was packed for one merge
  // request and installed from a tarball, so there is no channel to move
  // along: `latest` of any package is a different build, not a newer one.
  if (!registry) {
    emit({
      ok: false,
      action: "preview_build",
      package: pkg.name,
      current_version: pkg.version,
      message: `${pkg.name}@${pkg.version} is a preview build and does not update itself.\n${leavePreviewHint(pkg.larkPreview)}`,
    });
    return 1;
  }

  const manual = `npm install -g ${pkg.name}@latest --registry ${registry}`;

  if (isPnpmInstall(packageDir)) {
    emit({ ok: false, action: "manual_required", message: `${pkg.name} was installed with pnpm; update it with:\n  pnpm add -g ${pkg.name}@latest --registry ${registry}` });
    return 1;
  }

  const latest = await fetchLatestVersion(registry, pkg.name);
  const current = pkg.version;
  const base = { ok: true, current_version: current, latest_version: latest, package: pkg.name, registry };
  if (compareVersions(latest, current) <= 0) {
    emit({ ...base, action: "already_up_to_date", message: `lark-cli ${current} is up to date.` });
    return 0;
  }
  if (opts.check) {
    emit({ ...base, action: "update_available", message: `Update available: ${current} -> ${latest}\nRun: lark-cli update` });
    return 0;
  }

  process.stderr.write(`Updating lark-cli ${current} -> ${latest} via npm ...\n`);
  const install = runCommand("npm", ["install", "-g", `${pkg.name}@${latest}`, "--registry", registry, "--no-audit", "--no-fund"], INSTALL_TIMEOUT_MS);
  if (install.status !== 0) {
    process.stderr.write((install.stdout || "") + (install.stderr || ""));
    emit({ ...base, ok: false, action: "update_error", message: `npm install failed${install.error ? `: ${install.error.message}` : ""}; run by hand:\n  ${manual}` });
    return 1;
  }

  const check = runCommand("lark-cli", ["--version"], 30000);
  const reported = (check.stdout || "").trim().split(/\s+/).pop() || "";
  if (check.status !== 0 || reported.replace(/^v/, "") !== latest) {
    emit({ ...base, ok: false, action: "update_error", message: `installed lark-cli reports ${JSON.stringify(check.stdout || "")}, expected ${latest}` });
    return 1;
  }

  // Same skills bundle the public updater syncs; a failure here leaves a
  // working, updated CLI, so it is reported but does not fail the update.
  let skills = "updated";
  const sync = (source) => runCommand("npx", ["-y", "skills", "add", source, "-y", "-g"], SKILLS_TIMEOUT_MS);
  if (sync(SKILLS_SOURCE).status !== 0 && sync(SKILLS_SOURCE_FALLBACK).status !== 0) {
    skills = "failed";
    process.stderr.write(`Skills sync failed; run by hand: npx skills add ${SKILLS_SOURCE_FALLBACK} -y -g\n`);
  }
  emit({ ...base, action: "updated", skills_action: skills, message: `Successfully updated lark-cli from ${current} to ${latest}` });
  return 0;
}

function main() {
  const pkg = require("../package.json");
  const packageDir = path.resolve(__dirname, "..");
  const args = process.argv.slice(2);

  if (args[0] === "update") {
    update(args.slice(1), pkg, packageDir)
      .then((code) => process.exit(code))
      .catch((err) => {
        console.error(`lark-cli update failed: ${err.message}`);
        process.exit(1);
      });
    return;
  }

  const name = binaryName();
  const binary = name && path.join(packageDir, "bin", name);
  if (!binary || !fs.existsSync(binary)) {
    console.error(`${pkg.name} has no binary for ${process.platform}-${process.arch}`);
    process.exit(1);
  }

  const env = {
    LARKSUITE_CLI_NO_UPDATE_NOTIFIER: "1",
    LARKSUITE_CLI_NO_SKILLS_NOTIFIER: "1",
    ...process.env,
  };
  const result = spawnSync(binary, args, { stdio: "inherit", env });
  if (result.error) {
    console.error(`Failed to start ${binary}: ${result.error.message}`);
    process.exit(1);
  }
  process.exit(result.status === null ? 1 : result.status);
}

module.exports = { binaryName, compareVersions, isPnpmInstall, leavePreviewHint, parseUpdateArgs, update };

if (require.main === module) main();
