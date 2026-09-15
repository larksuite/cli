// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("fs");
const os = require("os");
const path = require("path");

const { deriveLauncherPackageJson, derivePlatformPackageJson, selectBinaries, stagePackages, stageTree } = require("./build-registry-package.js");

const BASE = {
  name: "@larksuite/cli",
  version: "1.2.3",
  description: "The official CLI",
  bin: { "lark-cli": "scripts/run.js" },
  scripts: { postinstall: "node scripts/install.js", "release:check": "node scripts/release-preflight.js" },
  os: ["darwin", "linux", "win32"],
  cpu: ["x64", "arm64", "riscv64"],
  engines: { node: ">=16" },
  license: "MIT",
  files: ["scripts/install.js", "scripts/run.js", "checksums.txt", "CHANGELOG.md"],
  dependencies: { "@clack/prompts": "^1.2.0" },
};
const TARGET = { name: "@example/larksuite-cli", registry: "https://registry.example" };
const LINUX = { goos: "linux", goarch: "amd64", target: "lark-cli-linux-amd64" };
const WINDOWS = { goos: "windows", goarch: "arm64", target: "lark-cli-windows-arm64.exe" };

describe("deriveLauncherPackageJson", () => {
  it("builds the launcher package from the public one, depending optionally on every platform package", () => {
    const pkg = deriveLauncherPackageJson(BASE, { ...TARGET, binaries: [LINUX, WINDOWS] });
    assert.deepEqual(pkg, {
      name: "@example/larksuite-cli",
      version: "1.2.3",
      description: "The official CLI",
      bin: { "lark-cli": "scripts/run.js" },
      os: BASE.os,
      cpu: BASE.cpu,
      engines: BASE.engines,
      license: "MIT",
      publishConfig: { registry: "https://registry.example" },
      optionalDependencies: {
        "@example/larksuite-cli-linux-amd64": "1.2.3",
        "@example/larksuite-cli-windows-arm64": "1.2.3",
      },
      files: ["scripts/run.js", "CHANGELOG.md"],
    });
    assert.equal(pkg.scripts, undefined, "no postinstall: nothing to prune");
  });

  it("strips a trailing slash from the registry", () => {
    assert.equal(deriveLauncherPackageJson(BASE, { ...TARGET, registry: "https://registry.example/", binaries: [LINUX] }).publishConfig.registry, "https://registry.example");
  });

  it("rejects an unscoped name, a non-https registry, an incomplete source and no platforms", () => {
    assert.throws(() => deriveLauncherPackageJson(BASE, { name: "lark cli", registry: TARGET.registry, binaries: [LINUX] }), /scoped npm name/);
    assert.throws(() => deriveLauncherPackageJson(BASE, { ...TARGET, registry: "http://registry.example", binaries: [LINUX] }), /https/);
    assert.throws(() => deriveLauncherPackageJson({ ...BASE, version: undefined }, { ...TARGET, binaries: [LINUX] }), /lacks version/);
    assert.throws(() => deriveLauncherPackageJson(BASE, { ...TARGET, binaries: [] }), /at least one platform/);
  });
});

describe("derivePlatformPackageJson", () => {
  it("restricts the package to its platform in npm's terms and gives it no bin", () => {
    const pkg = derivePlatformPackageJson(BASE, { ...TARGET, binary: WINDOWS });
    assert.deepEqual(pkg, {
      name: "@example/larksuite-cli-windows-arm64",
      version: "1.2.3",
      description: "The official CLI (windows/arm64 binary for @example/larksuite-cli)",
      os: ["win32"],
      cpu: ["arm64"],
      engines: BASE.engines,
      license: "MIT",
      publishConfig: { registry: "https://registry.example" },
      files: ["bin/"],
    });
    assert.deepEqual(derivePlatformPackageJson(BASE, { ...TARGET, binary: LINUX }).os, ["linux"]);
    assert.deepEqual(derivePlatformPackageJson(BASE, { ...TARGET, binary: LINUX }).cpu, ["x64"]);
  });

  it("refuses a platform the launcher cannot name", () => {
    assert.throws(() => derivePlatformPackageJson(BASE, { ...TARGET, binary: { goos: "freebsd", goarch: "amd64" } }), /no npm os\/cpu name for freebsd\/amd64/);
  });
});

const ARTIFACTS = [
  { name: "lark-cli", path: "dist/lark-cli_linux_amd64_v1/lark-cli", goos: "linux", goarch: "amd64", type: "Binary" },
  { name: "lark-cli.exe", path: "dist/lark-cli_windows_arm64_v8.0/lark-cli.exe", goos: "windows", goarch: "arm64", type: "Binary" },
  { name: "lark-cli-1.2.3-linux-amd64.tar.gz", path: "dist/lark-cli-1.2.3-linux-amd64.tar.gz", goos: "linux", goarch: "amd64", type: "Archive" },
  { name: "checksums.txt", path: "dist/checksums.txt", type: "Checksum" },
];

describe("selectBinaries", () => {
  it("keeps only binaries, named by platform, in a stable order", () => {
    assert.deepEqual(selectBinaries(ARTIFACTS).map((b) => b.target), ["lark-cli-linux-amd64", "lark-cli-windows-arm64.exe"]);
  });

  it("rejects an empty set and two binaries for one platform", () => {
    assert.throws(() => selectBinaries(ARTIFACTS.filter((a) => a.type !== "Binary")), /no binaries/);
    assert.throws(() => selectBinaries([ARTIFACTS[0], { ...ARTIFACTS[0], path: "dist/other/lark-cli" }]), /more than one binary for linux-amd64/);
  });
});

function makeRepo(artifacts, { skipBinary } = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "registry-package-repo-"));
  for (const f of ["CHANGELOG.md", "LICENSE", "README.md"]) fs.writeFileSync(path.join(root, f), `${f}\n`);
  fs.writeFileSync(path.join(root, "package.json"), JSON.stringify(BASE));
  const dist = path.join(root, "dist");
  fs.mkdirSync(dist);
  for (const a of artifacts) {
    if (a.type !== "Binary" || a.path === skipBinary) continue;
    fs.mkdirSync(path.join(root, path.dirname(a.path)), { recursive: true });
    fs.writeFileSync(path.join(root, a.path), `bytes of ${a.path}`);
  }
  fs.writeFileSync(path.join(dist, "artifacts.json"), JSON.stringify(artifacts));
  return { root, dist, out: path.join(root, "release-candidate", "stage") };
}

const readJson = (file) => JSON.parse(fs.readFileSync(file, "utf8"));

describe("stagePackages", () => {
  it("stages one tree per platform and the launcher last", () => {
    const { root, dist, out } = makeRepo(ARTIFACTS);
    const staged = stagePackages({ repoRoot: root, distDir: dist, stageRoot: out, ...TARGET });

    assert.deepEqual(
      staged.map((t) => t.packageJson.name),
      ["@example/larksuite-cli-linux-amd64", "@example/larksuite-cli-windows-arm64", "@example/larksuite-cli"],
      "platform packages first, launcher last"
    );
    assert.deepEqual(staged.map((t) => t.kind), ["platform", "platform", "launcher"], "each tree says what it is");

    const linux = path.join(out, "linux-amd64");
    assert.equal(fs.readFileSync(path.join(linux, "bin", "lark-cli"), "utf8"), "bytes of dist/lark-cli_linux_amd64_v1/lark-cli");
    if (process.platform !== "win32") {
      assert.equal(fs.statSync(path.join(linux, "bin", "lark-cli")).mode & 0o111, 0o111, "binaries are executable");
    }
    assert.ok(fs.existsSync(path.join(linux, "LICENSE")), "platform package carries the license");
    assert.deepEqual(fs.readdirSync(linux).sort(), ["LICENSE", "bin", "package.json"], "nothing else travels with a binary");
    assert.equal(readJson(path.join(linux, "package.json")).name, "@example/larksuite-cli-linux-amd64");

    assert.ok(fs.existsSync(path.join(out, "windows-arm64", "bin", "lark-cli.exe")), "windows binary keeps its extension");

    const launcher = path.join(out, "launcher");
    assert.equal(fs.readFileSync(path.join(launcher, "scripts", "run.js"), "utf8"), fs.readFileSync(path.join(__dirname, "run.js"), "utf8"));
    assert.ok(!fs.existsSync(path.join(launcher, "bin")), "the launcher ships no binary");
    assert.ok(!fs.existsSync(path.join(launcher, "scripts", "postinstall.js")), "the launcher ships no postinstall");
    for (const f of ["CHANGELOG.md", "LICENSE", "README.md"]) assert.ok(fs.existsSync(path.join(launcher, f)), `${f} staged`);
    const launcherJson = readJson(path.join(launcher, "package.json"));
    assert.equal(launcherJson.name, TARGET.name);
    assert.deepEqual(Object.keys(launcherJson.optionalDependencies), ["@example/larksuite-cli-linux-amd64", "@example/larksuite-cli-windows-arm64"]);
  });

  it("fails when artifacts.json lists a binary that is missing", () => {
    const { root, dist, out } = makeRepo(ARTIFACTS, { skipBinary: "dist/lark-cli_windows_arm64_v8.0/lark-cli.exe" });
    assert.throws(() => stagePackages({ repoRoot: root, distDir: dist, stageRoot: out, ...TARGET }), /windows_arm64.*missing/);
  });

  it("fails without artifacts.json", () => {
    const { root, dist, out } = makeRepo(ARTIFACTS);
    fs.rmSync(path.join(dist, "artifacts.json"));
    assert.throws(() => stagePackages({ repoRoot: root, distDir: dist, stageRoot: out, ...TARGET }), /artifacts\.json/);
  });
});

// The preview package still bundles its binaries; this is the layout it
// stages through.
describe("stageTree", () => {
  it("assembles bin/, the launcher, postinstall and the root files", () => {
    const { root, out } = makeRepo(ARTIFACTS);
    const binaries = selectBinaries(ARTIFACTS).map((b) => ({ ...b, source: path.join(root, b.path) }));
    const staged = stageTree({ repoRoot: root, stageDir: out, binaries, packageJson: { name: "@example/preview", version: "1.2.3-x.g1" } });

    assert.deepEqual(staged.binaries, ["lark-cli-linux-amd64", "lark-cli-windows-arm64.exe"]);
    assert.equal(fs.readFileSync(path.join(out, "bin", "lark-cli-linux-amd64"), "utf8"), "bytes of dist/lark-cli_linux_amd64_v1/lark-cli");
    assert.equal(fs.readFileSync(path.join(out, "scripts", "postinstall.js"), "utf8"), fs.readFileSync(path.join(__dirname, "postinstall.js"), "utf8"));
    for (const f of ["CHANGELOG.md", "LICENSE", "README.md"]) assert.ok(fs.existsSync(path.join(out, f)), `${f} staged`);
    assert.equal(readJson(path.join(out, "package.json")).name, "@example/preview");
  });

  it("fails when a binary is missing", () => {
    const { root, out } = makeRepo(ARTIFACTS, { skipBinary: "dist/lark-cli_windows_arm64_v8.0/lark-cli.exe" });
    const binaries = selectBinaries(ARTIFACTS).map((b) => ({ ...b, source: path.join(root, b.path) }));
    assert.throws(() => stageTree({ repoRoot: root, stageDir: out, binaries, packageJson: {} }), /windows-arm64.*missing/);
  });
});
