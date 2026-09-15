// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("fs");
const http = require("http");
const os = require("os");
const path = require("path");
const { spawn, spawnSync } = require("child_process");

const { binaryName, compareVersions, isPnpmInstall, leavePreviewHint, parseUpdateArgs, platformPackageName, resolveBinary } = require("./run.js");
const { prune } = require("./postinstall.js");

const POSIX = process.platform !== "win32";
const NAME = "@example/larksuite-cli";

const FAKE_BINARY = '#!/bin/sh\necho "args:$*"\necho "notifier:$LARKSUITE_CLI_NO_UPDATE_NOTIFIER"\nexit 3\n';

// A package tree as npm would lay it out, with this platform's binary replaced
// by a shell script that reports what it received, plus fake npm / npx /
// lark-cli commands on PATH that record their calls. By default the binary
// is bundled under bin/ as the preview package ships it; `layout: "platform"`
// puts it in a sibling platform package instead, as the released package
// installs it.
function makePackage({ version = "1.0.93", registry = "https://registry.example", root = fs.mkdtempSync(path.join(os.tmpdir(), "launcher-")), packageJson, layout = "bundled" } = {}) {
  const pkgDir = path.join(root, "node_modules", ...NAME.split("/"));
  fs.mkdirSync(path.join(pkgDir, "scripts"), { recursive: true });
  fs.mkdirSync(path.join(pkgDir, "bin"));
  for (const f of ["run.js", "postinstall.js"]) fs.copyFileSync(path.join(__dirname, f), path.join(pkgDir, "scripts", f));
  fs.writeFileSync(path.join(pkgDir, "package.json"), JSON.stringify(packageJson || { name: NAME, version, publishConfig: { registry } }));
  if (layout === "platform") {
    const platformDir = path.join(root, "node_modules", ...platformPackageName(NAME).split("/"));
    fs.mkdirSync(path.join(platformDir, "bin"), { recursive: true });
    fs.writeFileSync(path.join(platformDir, "package.json"), JSON.stringify({ name: platformPackageName(NAME), version }));
    fs.writeFileSync(path.join(platformDir, "bin", "lark-cli"), FAKE_BINARY, { mode: 0o755 });
  } else {
    fs.writeFileSync(path.join(pkgDir, "bin", binaryName()), FAKE_BINARY, { mode: 0o755 });
    fs.writeFileSync(path.join(pkgDir, "bin", "lark-cli-windows-arm64.exe"), "other platform");
  }

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

describe("platformPackageName", () => {
  it("derives the platform package from the launcher package name", () => {
    assert.equal(platformPackageName("@example/larksuite-cli", "darwin", "arm64"), "@example/larksuite-cli-darwin-arm64");
    assert.equal(platformPackageName("@example/larksuite-cli", "win32", "x64"), "@example/larksuite-cli-windows-amd64");
    assert.equal(platformPackageName("@example/larksuite-cli", "freebsd", "x64"), null);
  });
});

describe("resolveBinary", () => {
  const pkg = {
    name: NAME,
    version: "1.0.93",
    publishConfig: { registry: "https://registry.example" },
    optionalDependencies: { [platformPackageName(NAME)]: "1.0.93" },
  };

  it("prefers a bundled binary", () => {
    const { pkgDir } = makePackage();
    assert.equal(resolveBinary(pkg, pkgDir).binary, path.join(pkgDir, "bin", binaryName()));
  });

  it("falls back to the installed platform package", () => {
    const { root, pkgDir } = makePackage({ layout: "platform" });
    // require.resolve returns the real path; macOS puts tmpdir behind a symlink.
    assert.equal(fs.realpathSync(resolveBinary(pkg, pkgDir).binary), fs.realpathSync(path.join(root, "node_modules", ...platformPackageName(NAME).split("/"), "bin", "lark-cli")));
  });

  it("names both places it looked and the install command when neither has the binary", () => {
    const { pkgDir } = makePackage({ layout: "platform" });
    fs.rmSync(path.join(pkgDir, "..", path.basename(platformPackageName(NAME))), { recursive: true });
    const { binary, reason } = resolveBinary(pkg, pkgDir);
    assert.equal(binary, null);
    assert.match(reason, new RegExp(`bin/${binaryName()}`));
    assert.match(reason, new RegExp(`npm install -g ${platformPackageName(NAME)}@1.0.93 --registry https://registry.example`));
  });

  it("refuses a platform package of another version and names the install command", () => {
    const { root, pkgDir } = makePackage({ layout: "platform" });
    const manifest = path.join(root, "node_modules", ...platformPackageName(NAME).split("/"), "package.json");
    fs.writeFileSync(manifest, JSON.stringify({ name: platformPackageName(NAME), version: "1.0.92" }));
    const { binary, reason } = resolveBinary(pkg, pkgDir);
    assert.equal(binary, null);
    assert.match(reason, /needs @example\/larksuite-cli-.*@1\.0\.93, but 1\.0\.92 is installed/);
    assert.match(reason, new RegExp(`npm install -g ${platformPackageName(NAME)}@1.0.93`));
  });

  it("does not tell a preview build, which declares no platform packages, to install one", () => {
    const { pkgDir } = makePackage();
    fs.rmSync(path.join(pkgDir, "bin", binaryName()));
    const preview = { name: NAME, version: "1.0.93-mr7.gabc1234", larkPreview: {} };
    const { reason } = resolveBinary(preview, pkgDir);
    assert.match(reason, /is not in the package/);
    assert.doesNotMatch(reason, /npm install|platform package/);
  });

  it("says so when there is no build for the platform at all", () => {
    const { pkgDir } = makePackage();
    assert.match(resolveBinary(pkg, pkgDir, "freebsd", "x64").reason, /no build for freebsd-x64/);
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

  it("runs the binary from the platform package when none is bundled", () => {
    const result = launch(makePackage({ layout: "platform" }), ["whoami"]);
    assert.equal(result.status, 3);
    assert.match(result.stdout, /^args:whoami$/m);
  });

  it("fails clearly when the package has no binary for the host", () => {
    const pkg = makePackage({ packageJson: { name: NAME, version: "1.0.93", publishConfig: { registry: "https://registry.example" }, optionalDependencies: { [platformPackageName(NAME)]: "1.0.93" } } });
    fs.rmSync(path.join(pkg.pkgDir, "bin", binaryName()));
    const result = launch(pkg, ["--version"]);
    assert.equal(result.status, 1);
    assert.match(result.stderr, /has no binary for/);
    assert.match(result.stderr, /npm install -g @example\/larksuite-cli-.*@1\.0\.93 --registry https:\/\/registry\.example/);
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

describe("leavePreviewHint", () => {
  it("names the exact reinstall command when the package carries a release identity", () => {
    assert.equal(
      leavePreviewHint({ releasePackage: "@example/cli", releaseRegistry: "https://registry.example" }),
      "To leave this preview build:\n  npm install -g @example/cli@latest --registry https://registry.example"
    );
  });

  it("stays generic rather than guessing when the package names no release", () => {
    for (const preview of [undefined, {}, { releasePackage: "@example/cli" }]) {
      assert.match(leavePreviewHint(preview), /^Reinstall the released package/);
    }
  });
});

// A preview package is packed by .codebase/preview and installed from its
// tarball. It has no publishConfig, and the launcher must read that as "there
// is no channel to move along" rather than resolving `latest` of some other
// build — every preview is a different merge request, not a newer version.
describe("run.js update on a preview build", { skip: !POSIX && "shell-script stand-ins need a POSIX shell" }, () => {
  const previewPackageJson = {
    name: "@example/larksuite-cli-preview",
    version: "1.0.93-mr7.gabc1234",
    private: true,
    larkPreview: { label: "mr7", commit: "abc1234", releasePackage: NAME, releaseRegistry: "https://registry.example" },
  };

  it("refuses to update and says how to get back to a release", async () => {
    const pkg = makePackage({ packageJson: previewPackageJson });
    const result = await launchAsync(pkg, ["update", "--json"]);
    assert.equal(result.status, 1);
    assert.deepEqual(json(result), {
      ok: false,
      action: "preview_build",
      package: "@example/larksuite-cli-preview",
      current_version: "1.0.93-mr7.gabc1234",
      message:
        "@example/larksuite-cli-preview@1.0.93-mr7.gabc1234 is a preview build and does not update itself.\n" +
        `To leave this preview build:\n  npm install -g ${NAME}@latest --registry https://registry.example`,
    });
    assert.deepEqual(pkg.calls(), [], "no npm or npx call is made");
  });

  it("refuses --check too, instead of reporting some other build as available", async () => {
    const pkg = makePackage({ packageJson: previewPackageJson });
    const result = await launchAsync(pkg, ["update", "--check", "--json"]);
    assert.equal(result.status, 1);
    assert.equal(json(result).action, "preview_build");
  });

  it("still dispatches every other command to the binary", () => {
    const pkg = makePackage({ packageJson: previewPackageJson });
    const result = launch(pkg, ["whoami", "--as", "bot"]);
    assert.equal(result.status, 3);
    assert.match(result.stdout, /args:whoami --as bot/);
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
