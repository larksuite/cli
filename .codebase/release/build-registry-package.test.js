// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("fs");
const os = require("os");
const path = require("path");

const { derivePackageJson, selectBinaries, stagePackage } = require("./build-registry-package.js");

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

describe("derivePackageJson", () => {
  it("builds the internal package from the public one", () => {
    const pkg = derivePackageJson(BASE, TARGET);
    assert.deepEqual(pkg, {
      name: "@example/larksuite-cli",
      version: "1.2.3",
      description: "The official CLI",
      bin: { "lark-cli": "scripts/run.js" },
      scripts: { postinstall: "node scripts/postinstall.js" },
      os: BASE.os,
      cpu: BASE.cpu,
      engines: BASE.engines,
      license: "MIT",
      publishConfig: { registry: "https://registry.example" },
      files: ["bin/", "scripts/run.js", "scripts/postinstall.js", "CHANGELOG.md"],
    });
  });

  it("strips a trailing slash from the registry", () => {
    assert.equal(derivePackageJson(BASE, { ...TARGET, registry: "https://registry.example/" }).publishConfig.registry, "https://registry.example");
  });

  it("rejects an unscoped name, a non-https registry and an incomplete source", () => {
    assert.throws(() => derivePackageJson(BASE, { name: "lark cli", registry: TARGET.registry }), /scoped npm name/);
    assert.throws(() => derivePackageJson(BASE, { name: TARGET.name, registry: "http://registry.example" }), /https/);
    assert.throws(() => derivePackageJson({ ...BASE, version: undefined }, TARGET), /lacks version/);
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
  return { root, dist, out: path.join(root, "release-candidate", "package") };
}

describe("stagePackage", () => {
  it("assembles bin/, the launcher, postinstall and the root files", () => {
    const { root, dist, out } = makeRepo(ARTIFACTS);
    const staged = stagePackage({ repoRoot: root, distDir: dist, stageDir: out, ...TARGET });

    assert.deepEqual(staged.binaries, ["lark-cli-linux-amd64", "lark-cli-windows-arm64.exe"]);
    assert.equal(fs.readFileSync(path.join(out, "bin", "lark-cli-linux-amd64"), "utf8"), "bytes of dist/lark-cli_linux_amd64_v1/lark-cli");
    if (process.platform !== "win32") {
      assert.equal(fs.statSync(path.join(out, "bin", "lark-cli-linux-amd64")).mode & 0o111, 0o111, "binaries are executable");
    }
    assert.equal(fs.readFileSync(path.join(out, "scripts", "run.js"), "utf8"), fs.readFileSync(path.join(__dirname, "run.js"), "utf8"));
    assert.equal(fs.readFileSync(path.join(out, "scripts", "postinstall.js"), "utf8"), fs.readFileSync(path.join(__dirname, "postinstall.js"), "utf8"));
    for (const f of ["CHANGELOG.md", "LICENSE", "README.md"]) assert.ok(fs.existsSync(path.join(out, f)), `${f} staged`);
    assert.equal(JSON.parse(fs.readFileSync(path.join(out, "package.json"), "utf8")).name, TARGET.name);
  });

  it("fails when artifacts.json lists a binary that is missing", () => {
    const { root, dist, out } = makeRepo(ARTIFACTS, { skipBinary: "dist/lark-cli_windows_arm64_v8.0/lark-cli.exe" });
    assert.throws(() => stagePackage({ repoRoot: root, distDir: dist, stageDir: out, ...TARGET }), /windows_arm64.*missing/);
  });

  it("fails without artifacts.json", () => {
    const { root, dist, out } = makeRepo(ARTIFACTS);
    fs.rmSync(path.join(dist, "artifacts.json"));
    assert.throws(() => stagePackage({ repoRoot: root, distDir: dist, stageDir: out, ...TARGET }), /artifacts\.json/);
  });
});
