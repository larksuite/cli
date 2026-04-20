#!/usr/bin/env node
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const fs = require("fs");
const path = require("path");
const { execFileSync, execFile } = require("child_process");
const p = require("@clack/prompts");

const PKG = "@larksuite/cli";
const SKILLS_REPO = "https://open.feishu.cn";
const SKILLS_REPO_FALLBACK = "larksuite/cli";
const isWindows = process.platform === "win32";

// ---------------------------------------------------------------------------
// i18n
// ---------------------------------------------------------------------------

const messages = {
  zh: {
    setup:          "正在设置 Feishu/Lark CLI...",
    step1:          "正在安装 %s...",
    step1Upgrade:   "正在升级 %s (v%s → v%s)...",
    step1Skip:      "已安装 (v%s)，跳过",
    step1Done:      "已全局安装",
    step1Upgraded:  "已升级到 v%s",
    step1Fail:      "全局安装失败。运行以下命令重试: npm install -g %s",
    step2:          "安装 AI Skills",
    step2Skip:      "已安装，跳过",
    step2Spinner:   "正在安装 Skills...",
    step2Done:      "Skills 已安装",
    step2Fail:      "Skills 安装失败。运行以下命令重试: npx skills add %s -y -g",
    step3:          "正在配置应用...",
    step3NotFound:  "未找到 lark-cli，终止",
    step3Found:     "发现已配置应用 (App ID: %s)，继续使用？",
    step3Skip:      "跳过应用配置",
    step3Done:      "应用已配置",
    step3Fail:      "应用配置失败。运行以下命令重试: lark-cli config init --new",
    step4:          "授权",
    step4NotFound:  "未找到 lark-cli，跳过授权",
    step4Confirm:   "是否允许 AI 访问你个人的消息、文档、日历等飞书 / Lark 数据，并以你的名义执行操作？",
    step4Skip:      "跳过授权。后续运行 lark-cli auth login 完成授权",
    step4Done:      "授权完成",
    step4Fail:      "授权失败。运行以下命令重试: lark-cli auth login",
    done:           "安装完成！\n可以和你的 AI 工具（如 Claude Code、Trae等）说：\"飞书/Lark CLI 能帮我做什么？结合我的情况推荐一下从哪里开始\"",
    cancelled:      "安装已取消",
  },
  en: {
    setup:          "Setting up Feishu/Lark CLI...",
    step1:          "Installing %s globally...",
    step1Upgrade:   "Upgrading %s (v%s → v%s)...",
    step1Skip:      "Already installed (v%s). Skipped",
    step1Done:      "Installed globally",
    step1Upgraded:  "Upgraded to v%s",
    step1Fail:      "Failed to install globally. Run manually: npm install -g %s",
    step2:          "Install AI skills",
    step2Skip:      "Already installed. Skipped",
    step2Spinner:   "Installing skills...",
    step2Done:      "Skills installed",
    step2Fail:      "Failed to install skills. Run manually: npx skills add %s -y -g",
    step3:          "Configuring app...",
    step3NotFound:  "lark-cli not found. Aborting",
    step3Found:     "Found existing app (App ID: %s). Use this app?",
    step3Skip:      "Skipped app configuration",
    step3Done:      "App configured",
    step3Fail:      "Failed to configure app. Run manually: lark-cli config init --new",
    step4:          "Authorization",
    step4NotFound:  "lark-cli not found. Skipping authorization",
    step4Confirm:   "Allow the AI to access your messages, documents, calendar, and more in Feishu/Lark, and perform actions on your behalf?",
    step4Skip:      "Skipped. Run lark-cli auth login to authorize later",
    step4Done:      "Authorization complete",
    step4Fail:      "Failed to authorize. Run lark-cli auth login to retry",
    done:           "You are all set!\nNow try asking your AI tool (Claude Code, Trae, etc.): \"What can Feishu/Lark CLI help me with, and where should I start?\"",
    cancelled:      "Installation cancelled",
  },
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function handleCancel(value, msg) {
  if (p.isCancel(value)) {
    p.cancel(msg.cancelled);
    process.exit(0);
  }
  return value;
}

function execCmd(cmd, args, opts) {
  if (isWindows) {
    return execFileSync("cmd.exe", ["/c", cmd, ...args], opts);
  }
  return execFileSync(cmd, args, opts);
}

function run(cmd, args, opts = {}) {
  execCmd(cmd, args, { stdio: "inherit", ...opts });
}

function runSilent(cmd, args, opts = {}) {
  return execCmd(cmd, args, {
    stdio: ["ignore", "pipe", "pipe"],
    ...opts,
  });
}

function runSilentAsync(cmd, args, opts = {}) {
  const actualCmd = isWindows ? "cmd.exe" : cmd;
  const actualArgs = isWindows ? ["/c", cmd, ...args] : args;
  return new Promise((resolve, reject) => {
    execFile(actualCmd, actualArgs, {
      stdio: ["ignore", "pipe", "pipe"],
      ...opts,
    }, (err, stdout) => {
      if (err) reject(err);
      else resolve(stdout);
    });
  });
}

function fmt(template, ...values) {
  let i = 0;
  return template.replace(/%s/g, () => values[i++] ?? "");
}

/** Resolve the path of globally installed lark-cli (skip npx temp copies). */
function whichLarkCli() {
  try {
    const prefix = execFileSync("npm", ["prefix", "-g"], {
      stdio: ["ignore", "pipe", "pipe"],
    }).toString().trim();
    const bin = isWindows
      ? path.join(prefix, "lark-cli.cmd")
      : path.join(prefix, "bin", "lark-cli");
    if (fs.existsSync(bin)) return bin;
  } catch (_) {
    // fall through
  }
  // Fallback to which/where if npm prefix lookup fails.
  try {
    const cmd = isWindows ? "where" : "which";
    return execFileSync(cmd, ["lark-cli"], { stdio: ["ignore", "pipe", "pipe"] })
      .toString()
      .split("\n")[0]
      .trim();
  } catch (_) {
    return null;
  }
}

/** Get the latest version of @larksuite/cli from the registry. Returns version or null. */
function getLatestVersion() {
  try {
    const out = runSilent("npm", ["view", PKG, "version"], { timeout: 15000 });
    const ver = out.toString().trim();
    return /^\d+\.\d+\.\d+/.test(ver) ? ver : null;
  } catch (_) {
    return null;
  }
}

/** Compare two semver strings. Returns true if a < b. */
function semverLessThan(a, b) {
  const pa = a.replace(/-.*$/, "").split(".").map(Number);
  const pb = b.replace(/-.*$/, "").split(".").map(Number);
  for (let i = 0; i < 3; i++) {
    if ((pa[i] || 0) < (pb[i] || 0)) return true;
    if ((pa[i] || 0) > (pb[i] || 0)) return false;
  }
  return false;
}

/** Check whether @larksuite/cli is truly installed in npm global prefix. Returns version or null. */
function getGloballyInstalledVersion() {
  try {
    const out = runSilent("npm", ["list", "-g", PKG], { timeout: 15000 });
    const match = out.toString().match(/@(\d+\.\d+\.\d+[^\s]*)/);
    return match ? match[1] : "unknown";
  } catch (_) {
    return null;
  }
}

/** Check whether lark-cli config already exists. Returns app ID or null. */
function getExistingAppId(binPath) {
  try {
    const out = runSilent(binPath, ["config", "show"], { timeout: 10000 });
    const json = JSON.parse(out.toString());
    return json.appId || null;
  } catch (_) {
    return null;
  }
}

/** Parse --lang from process.argv, returns "zh", "en", or null. */
function parseLangArg() {
  const args = process.argv.slice(2);
  for (let i = 0; i < args.length; i++) {
    if (args[i] === "--lang" && args[i + 1]) {
      const val = args[i + 1].toLowerCase();
      if (val === "zh" || val === "en") return val;
    }
    if (args[i].startsWith("--lang=")) {
      const val = args[i].split("=")[1].toLowerCase();
      if (val === "zh" || val === "en") return val;
    }
  }
  return null;
}

// ---------------------------------------------------------------------------
// Steps
// ---------------------------------------------------------------------------

async function stepSelectLang() {
  const fromArg = parseLangArg();
  if (fromArg) return fromArg;

  const lang = await p.select({
    message: "请选择语言 / Select language",
    options: [
      { value: "zh", label: "中文" },
      { value: "en", label: "English" },
    ],
  });
  return handleCancel(lang, messages.zh);
}

async function stepInstallGlobally(msg) {
  const installedVer = getGloballyInstalledVersion();
  const latestVer = getLatestVersion();
  const needsUpgrade = installedVer && latestVer && semverLessThan(installedVer, latestVer);

  if (installedVer && !needsUpgrade) {
    p.log.info(fmt(msg.step1Skip, installedVer));
    return false;
  }

  const s = p.spinner();
  if (needsUpgrade) {
    s.start(fmt(msg.step1Upgrade, PKG, installedVer, latestVer));
  } else {
    s.start(fmt(msg.step1, PKG));
  }
  try {
    await runSilentAsync("npm", ["install", "-g", PKG], { timeout: 120000 });
    s.stop(needsUpgrade ? fmt(msg.step1Upgraded, latestVer) : msg.step1Done);
    return needsUpgrade;
  } catch (_) {
    s.stop(fmt(msg.step1Fail, PKG));
    process.exit(1);
  }
}

async function skillsAlreadyInstalled() {
  try {
    const out = await runSilentAsync("npx", ["-y", "skills", "ls", "-g"], {
      timeout: 120000,
    });
    return /^lark-/m.test(out.toString());
  } catch (_) {
    return false;
  }
}

async function stepInstallSkills(msg) {
  const s = p.spinner();
  s.start(msg.step2Spinner);
  try {
    if (await skillsAlreadyInstalled()) {
      s.stop(msg.step2Skip);
      return;
    }
    try {
      await runSilentAsync("npx", ["-y", "skills", "add", SKILLS_REPO, "-y", "-g"], {
        timeout: 120000,
      });
    } catch (_) {
      await runSilentAsync("npx", ["-y", "skills", "add", SKILLS_REPO_FALLBACK, "-y", "-g"], {
        timeout: 120000,
      });
    }
    s.stop(msg.step2Done);
  } catch (_) {
    s.stop(fmt(msg.step2Fail, SKILLS_REPO_FALLBACK));
    process.exit(1);
  }
}

async function stepConfigInit(msg, lang) {
  const s = p.spinner();
  s.start(msg.step3);

  const larkCli = whichLarkCli();
  if (!larkCli) {
    s.stop(msg.step3NotFound);
    process.exit(1);
  }

  const appId = getExistingAppId(larkCli);
  s.stop(msg.step3);

  if (appId) {
    const reuse = await p.confirm({
      message: fmt(msg.step3Found, appId),
    });
    if (handleCancel(reuse, msg) && reuse) {
      p.log.info(msg.step3Skip);
      return;
    }
  }

  try {
    run(larkCli, ["config", "init", "--new", "--lang", lang]);
    p.log.success(msg.step3Done);
  } catch (_) {
    p.log.error(msg.step3Fail);
    process.exit(1);
  }
}

async function stepAuthLogin(msg) {
  const larkCli = whichLarkCli();
  if (!larkCli) {
    p.log.warn(msg.step4NotFound);
    return;
  }

  const yes = await p.confirm({
    message: msg.step4Confirm,
  });
  if (p.isCancel(yes)) {
    p.cancel(msg.cancelled);
    process.exit(0);
  }
  if (!yes) {
    p.log.info(msg.step4Skip);
    return;
  }

  p.log.step(msg.step4);
  try {
    run(larkCli, ["auth", "login"]);
    p.log.success(msg.step4Done);
  } catch (_) {
    p.log.warn(msg.step4Fail);
  }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

async function main() {
  const lang = await stepSelectLang();
  const msg = messages[lang];

  p.intro(msg.setup);

  await stepInstallGlobally(msg);
  await stepInstallSkills(msg);
  await stepConfigInit(msg, lang);
  await stepAuthLogin(msg);

  p.outro(msg.done);
}

main().catch((err) => {
  p.cancel("Unexpected error: " + (err.message || err));
  process.exit(1);
});
