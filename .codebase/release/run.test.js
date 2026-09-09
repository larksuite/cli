// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("fs");
const http = require("http");
const os = require("os");
const path = require("path");
const { spawn, spawnSync } = require("child_process");

const { binaryName, compareVersions, isPnpmInstall, parseUpdateArgs } = require("./run.js");
const { prune } = require("./postinstall.js");

const POSIX = process.platform !== "win32";
const NAME = "@example/larksuite-cli";

// A package tree as npm would lay it out, with this platform's binary replaced
// by a shell script that reports what it received, plus fake npm / npx /
// lark-cli commands on PATH that record their calls.
function makePackage({ version = "1.0.93", registry = "https://registry.example", root = fs.mkdtempSync(path.join(os.tmpdir(), "launcher-")) } = {}) {
  const pkgDir = path.join(root, "node_modules", ...NAME.split("/"));
  fs.mkdirSync(path.join(pkgDir, "scripts"), { recursive: true });
  fs.mkdirSync(path.join(pkgDir, "bin"));
  for (const f of ["run.js", "postinstall.js"]) fs.copyFileSync(path.join(__dirname, f), path.join(pkgDir, "scripts", f));
  fs.writeFileSync(path.join(pkgDir, "package.json"), JSON.stringify({ name: NAME, version, publishConfig: { registry } }));
  fs.writeFileSync(path.join(pkgDir, "bin", binaryName()), '#!/bin/sh\necho "args:$*"\necho "notifier:$LARKSUITE_CLI_NO_UPDATE_NOTIFIER"\nexit 3\n', { mode: 0o755 });
  fs.writeFileSync(path.join(pkgDir, "bin", "lark-cli-windows-arm64.exe"), "other platform");

  const fakeBin = path.join(root, "fakebin");
  fs.mkdirSync(fakeBin);
  const log = path.join(root, "calls.log");
  fs.writeFileSync(path.join(fakeBin, "npm"), `#!/bin/sh\nprintf 'npm %s\\n' "$*" >> "${log}"\n[ "\${FAKE_NPM_FAIL:-}" = 1 ] && { echo "npm ERR! boom" >&2; exit 1; }\nexit 0\n`, { mode: 0o755 });
  fs.writeFileSync(path.join(fakeBin, "npx"), `#!/bin/sh\nprintf 'npx %s\\n' "$*" >> "${log}"\n[ "\${FAKE_NPX_FAIL:-}" = 1 ] && exit 1\nexit 0\n`, { mode: 0o755 });
  fs.writeFileSync(path.join(fakeBin, "lark-cli"), `#!/bin/sh\necho "lark-cli version \${FAKE_REPORT:-0.0.0}"\n`, { mode: 0o755 });
  return { root, pkgDir, fakeBin, log, calls: () => (fs.existsSync(log) ? fs.readFileSync(log, "utf8").trim().split("\n").filter(Boolean) : []) };
}

function launch(pkg, args, env = {}) {
  return spawnSync(process.execPath, [path.join(pkg.pkgDir, "scripts", "run.js"), ...args], {
    encoding: "utf8",
    env: { ...process.env, PATH: `${pkg.fakeBin}:${process.env.PATH}`, ...env },
  });
}

// update talks to a registry served from this process, so the child must run
// asynchronously or the server could never answer.
function launchAsync(pkg, args, env = {}) {
  return new Promise((resolve) => {
    const child = spawn(process.execPath, [path.join(pkg.pkgDir, "scripts", "run.js"), ...args], {
      env: { ...process.env, PATH: `${pkg.fakeBin}:${process.env.PATH}`, ...env },
    });
    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (d) => (stdout += d));
    child.stderr.on("data", (d) => (stderr += d));
    child.on("close", (status) => resolve({ status, stdout, stderr }));
  });
}

async function withRegistry(latest, fn) {
  const server = http.createServer((req, res) => {
    if (req.url === `/${NAME}/latest` && latest) {
      res.setHeader("content-type", "application/json");
      res.end(JSON.stringify({ name: NAME, version: latest }));
    } else {
      res.statusCode = 404;
      res.end("{}");
    }
  });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  try {
    return await fn(`http://127.0.0.1:${server.address().port}`);
  } finally {
    server.close();
  }
}

const json = (r) => JSON.parse(r.stdout);

describe("binaryName", () => {
  it("maps Node platform names to the GoReleaser build names", () => {
    assert.equal(binaryName("darwin", "arm64"), "lark-cli-darwin-arm64");
    assert.equal(binaryName("win32", "x64"), "lark-cli-windows-amd64.exe");
    assert.equal(binaryName("linux", "riscv64"), "lark-cli-linux-riscv64");
    assert.equal(binaryName("freebsd", "x64"), null);
    assert.equal(binaryName("linux", "ia32"), null);
  });
});

describe("compareVersions", () => {
  it("orders releases and prereleases like npm does", () => {
    assert.equal(compareVersions("1.0.93", "1.0.94"), -1);
    assert.equal(compareVersions("1.0.93-beta.1", "1.0.93"), -1);
    assert.equal(compareVersions("1.0.93-beta.2", "1.0.93-beta.1"), 1);
    assert.equal(compareVersions("v1.0.93", "1.0.93"), 0);
    assert.equal(compareVersions("1.10.0", "1.9.9"), 1);
    assert.throws(() => compareVersions("1.0", "1.0.0"), /cannot compare/);
  });
});

describe("isPnpmInstall", () => {
  it("recognises pnpm's virtual store and global store, not directories merely named pnpm", () => {
    assert.equal(isPnpmInstall("/x/node_modules/.pnpm/@example+larksuite-cli@1.0.93/node_modules/@example/larksuite-cli"), true);
    assert.equal(isPnpmInstall("/Users/x/Library/pnpm/store/v11/links/@example/larksuite-cli"), true);
    assert.equal(isPnpmInstall("/opt/pnpm/lib/node_modules/@example/larksuite-cli"), false);
    assert.equal(isPnpmInstall("/usr/local/lib/node_modules/@example/larksuite-cli"), false);
  });
});

describe("parseUpdateArgs", () => {
  it("accepts --check and --json only", () => {
    assert.deepEqual(parseUpdateArgs(["--check", "--json"]), { check: true, json: true });
    assert.throws(() => parseUpdateArgs(["--force"]), /unknown flag --force/);
  });
});

describe("run.js launcher", { skip: !POSIX && "shell-script stand-ins need a POSIX shell" }, () => {
  it("forwards arguments, propagates the exit code and silences the public notifiers", () => {
    const result = launch(makePackage(), ["docs", "+fetch", "--doc", "x"]);
    assert.equal(result.status, 3);
    assert.match(result.stdout, /^args:docs \+fetch --doc x$/m);
    assert.match(result.stdout, /^notifier:1$/m);
  });

  it("leaves a notifier setting the caller made alone", () => {
    assert.match(launch(makePackage(), [], { LARKSUITE_CLI_NO_UPDATE_NOTIFIER: "" }).stdout, /^notifier:$/m);
  });

  it("fails clearly when the package has no binary for the host", () => {
    const pkg = makePackage();
    fs.rmSync(path.join(pkg.pkgDir, "bin", binaryName()));
    const result = launch(pkg, ["--version"]);
    assert.equal(result.status, 1);
    assert.match(result.stderr, /has no binary for/);
  });
});

describe("run.js update", { skip: !POSIX && "shell-script stand-ins need a POSIX shell" }, () => {
  it("reports an up-to-date install without touching npm", () =>
    withRegistry("1.0.93", async (registry) => {
      const pkg = makePackage({ registry });
      const result = await launchAsync(pkg, ["update", "--check", "--json"]);
      assert.equal(result.status, 0);
      assert.equal(json(result).action, "already_up_to_date");
      assert.deepEqual(pkg.calls(), []);
    }));

  it("--check reports the available version from the package's own registry and changes nothing", () =>
    withRegistry("1.0.94", async (registry) => {
      const pkg = makePackage({ registry });
      const result = await launchAsync(pkg, ["update", "--check", "--json"]);
      assert.equal(result.status, 0);
      assert.deepEqual(json(result), {
        ok: true, current_version: "1.0.93", latest_version: "1.0.94", package: NAME, registry,
        action: "update_available", message: "Update available: 1.0.93 -> 1.0.94\nRun: lark-cli update",
      });
      assert.deepEqual(pkg.calls(), []);
    }));

  it("installs the new version from its registry, verifies it and syncs the public skills", () =>
    withRegistry("1.0.94", async (registry) => {
      const pkg = makePackage({ registry });
      const result = await launchAsync(pkg, ["update", "--json"], { FAKE_REPORT: "1.0.94" });
      assert.equal(result.status, 0, result.stderr);
      assert.equal(json(result).action, "updated");
      assert.equal(json(result).skills_action, "updated");
      assert.deepEqual(pkg.calls(), [
        `npm install -g ${NAME}@1.0.94 --registry ${registry} --no-audit --no-fund`,
        "npx -y skills add https://open.feishu.cn/lark-cli/skills/regular -y -g",
      ]);
    }));

  it("fails when the installed CLI does not report the new version", () =>
    withRegistry("1.0.94", async (registry) => {
      const pkg = makePackage({ registry });
      const result = await launchAsync(pkg, ["update", "--json"], { FAKE_REPORT: "1.0.93" });
      assert.equal(result.status, 1);
      assert.equal(json(result).action, "update_error");
      assert.equal(pkg.calls().filter((c) => c.startsWith("npx")).length, 0, "skills are not synced after a failed update");
    }));

  it("fails with npm's output and the manual command when the install fails", () =>
    withRegistry("1.0.94", async (registry) => {
      const pkg = makePackage({ registry });
      const result = await launchAsync(pkg, ["update"], { FAKE_NPM_FAIL: "1" });
      assert.equal(result.status, 1);
      assert.match(result.stderr, /npm ERR! boom/);
      assert.match(result.stderr, new RegExp(`npm install -g ${NAME}@latest --registry ${registry}`));
    }));

  it("keeps a successful update when the skills sync fails, and says so", () =>
    withRegistry("1.0.94", async (registry) => {
      const pkg = makePackage({ registry });
      const result = await launchAsync(pkg, ["update", "--json"], { FAKE_REPORT: "1.0.94", FAKE_NPX_FAIL: "1" });
      assert.equal(result.status, 0);
      assert.equal(json(result).skills_action, "failed");
      assert.deepEqual(pkg.calls().filter((c) => c.startsWith("npx")), [
        "npx -y skills add https://open.feishu.cn/lark-cli/skills/regular -y -g",
        "npx -y skills add larksuite/cli -y -g",
      ]);
    }));

  it("tells a pnpm install to update by hand", () =>
    withRegistry("1.0.94", async (registry) => {
      const root = fs.mkdtempSync(path.join(os.tmpdir(), "launcher-pnpm-"));
      const pkg = makePackage({ registry, root: path.join(root, "node_modules", ".pnpm", "x") });
      const result = await launchAsync(pkg, ["update", "--json"]);
      assert.equal(result.status, 1);
      assert.equal(json(result).action, "manual_required");
      assert.match(json(result).message, /pnpm add -g/);
      assert.deepEqual(pkg.calls(), []);
    }));

  it("fails clearly when the registry has no such package", () =>
    withRegistry(null, async (registry) => {
      const result = await launchAsync(makePackage({ registry }), ["update", "--check"]);
      assert.equal(result.status, 1);
      assert.match(result.stderr, /registry returned HTTP 404/);
    }));

  it("rejects flags it does not implement", () => {
    const result = launch(makePackage(), ["update", "--force"]);
    assert.equal(result.status, 1);
    assert.match(result.stderr, /unknown flag --force/);
  });
});

describe("postinstall prune", () => {
  it("keeps this platform's binary and removes the others", () => {
    const pkg = makePackage();
    assert.deepEqual(prune(path.join(pkg.pkgDir, "bin")), ["lark-cli-windows-arm64.exe"]);
    assert.deepEqual(fs.readdirSync(path.join(pkg.pkgDir, "bin")), [binaryName()]);
  });

  it("never throws when there is nothing to prune", () => {
    assert.deepEqual(prune(path.join(os.tmpdir(), "no-such-bin-" + process.pid)), []);
  });
});
