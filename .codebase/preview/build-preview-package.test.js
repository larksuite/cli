// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("fs");
const os = require("os");
const path = require("path");

const {
  collectBinaries,
  derivePreviewPackageJson,
  stagePreviewPackage,
  validatePreviewVersion,
} = require("./build-preview-package.js");

const BASE = {
  name: "@larksuite/cli",
  version: "1.2.3",
  description: "The official CLI",
  bin: { "lark-cli": "scripts/run.js" },
  scripts: { postinstall: "node scripts/install.js" },
  os: ["darwin", "linux", "win32"],
  cpu: ["x64", "arm64", "riscv64"],
  engines: { node: ">=16" },
  license: "MIT",
  files: ["scripts/install.js", "scripts/run.js", "checksums.txt", "CHANGELOG.md"],
  dependencies: { "@clack/prompts": "^1.2.0" },
};
const IDENTITY = {
  name: "@example/larksuite-cli-preview",
  version: "1.2.3-mr7.gabc1234",
  label: "mr7",
  commit: "abc1234",
};
const RELEASE = { releasePackage: "@example/larksuite-cli", releaseRegistry: "https://registry.example" };

describe("validatePreviewVersion", () => {
  it("requires a prerelease, so a preview can never read as a release", () => {
    assert.equal(validatePreviewVersion("1.2.3-mr7.gabc1234"), "1.2.3-mr7.gabc1234");
    assert.throws(() => validatePreviewVersion("1.2.3"), /X\.Y\.Z-<prerelease>/);
    assert.throws(() => validatePreviewVersion("1.2.3-"), /X\.Y\.Z-<prerelease>/);
  });

  it("rejects the numeric identifier npm would reject", () => {
    // Why the g prefix on the commit exists: a short hash of digits alone
    // becomes a numeric identifier, and a leading zero is invalid semver.
    assert.throws(() => validatePreviewVersion("1.2.3-mr7.0123456"), /leading zero/);
    assert.equal(validatePreviewVersion("1.2.3-mr7.g0123456"), "1.2.3-mr7.g0123456");
  });
});

describe("derivePreviewPackageJson", () => {
  it("builds a private, unpublishable package that carries skills", () => {
    const pkg = derivePreviewPackageJson(BASE, { ...IDENTITY, ...RELEASE });
    assert.deepEqual(pkg, {
      name: "@example/larksuite-cli-preview",
      version: "1.2.3-mr7.gabc1234",
      description: "The official CLI (preview build of mr7 at abc1234)",
      bin: { "lark-cli": "scripts/run.js" },
      scripts: { postinstall: "node scripts/postinstall.js" },
      os: BASE.os,
      cpu: BASE.cpu,
      engines: BASE.engines,
      license: "MIT",
      private: true,
      larkPreview: {
        label: "mr7",
        commit: "abc1234",
        releasePackage: "@example/larksuite-cli",
        releaseRegistry: "https://registry.example",
      },
      files: ["bin/", "scripts/run.js", "scripts/postinstall.js", "skills/", "CHANGELOG.md"],
    });
    assert.equal(pkg.publishConfig, undefined, "no publish target: the launcher reads this as do-not-update");
  });

  it("omits the release identity when the caller names none", () => {
    assert.deepEqual(derivePreviewPackageJson(BASE, IDENTITY).larkPreview, { label: "mr7", commit: "abc1234" });
  });

  it("refuses to reuse the source package name", () => {
    assert.throws(
      () => derivePreviewPackageJson(BASE, { ...IDENTITY, name: BASE.name }),
      /must not reuse the source package name/
    );
  });

  it("rejects a bad name, a half-given or non-https release identity and an incomplete source", () => {
    assert.throws(() => derivePreviewPackageJson(BASE, { ...IDENTITY, name: "preview cli" }), /scoped npm name/);
    assert.throws(() => derivePreviewPackageJson(BASE, { ...IDENTITY, releaseRegistry: "https://r.example" }), /Release package/);
    assert.throws(
      () => derivePreviewPackageJson(BASE, { ...IDENTITY, ...RELEASE, releaseRegistry: "http://r.example" }),
      /https/
    );
    assert.throws(() => derivePreviewPackageJson(BASE, { ...IDENTITY, label: "" }), /label is required/);
    assert.throws(() => derivePreviewPackageJson({ ...BASE, description: undefined }, IDENTITY), /lacks description/);
  });

  it("strips a trailing slash from the release registry", () => {
    const pkg = derivePreviewPackageJson(BASE, { ...IDENTITY, ...RELEASE, releaseRegistry: "https://registry.example/" });
    assert.equal(pkg.larkPreview.releaseRegistry, "https://registry.example");
  });
});

function makeBinDir(names) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "preview-bin-"));
  for (const name of names) fs.writeFileSync(path.join(dir, name), `bytes of ${name}`);
  return dir;
}

describe("collectBinaries", () => {
  it("names every binary by platform, in a stable order", () => {
    const dir = makeBinDir(["lark-cli-darwin-arm64", "lark-cli-darwin-amd64", "lark-cli-windows-amd64.exe"]);
    assert.deepEqual(collectBinaries(dir).map((b) => b.target), [
      "lark-cli-darwin-amd64",
      "lark-cli-darwin-arm64",
      "lark-cli-windows-amd64.exe",
    ]);
  });

  it("fails on a stray file rather than shipping a package the launcher cannot dispatch", () => {
    assert.throws(() => collectBinaries(makeBinDir(["lark-cli-darwin-arm64", "notes.txt"])), /not a lark-cli-/);
    assert.throws(() => collectBinaries(makeBinDir(["lark-cli-windows-amd64"])), /wrong extension for windows/);
    assert.throws(() => collectBinaries(makeBinDir([])), /no binaries/);
    assert.throws(() => collectBinaries(path.join(os.tmpdir(), "preview-bin-absent")), /does not exist/);
  });
});

function makeRepo() {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "preview-repo-"));
  for (const f of ["CHANGELOG.md", "LICENSE", "README.md"]) fs.writeFileSync(path.join(root, f), `${f}\n`);
  fs.writeFileSync(path.join(root, "package.json"), JSON.stringify(BASE));
  fs.mkdirSync(path.join(root, "skills", "lark-base", "references"), { recursive: true });
  fs.writeFileSync(path.join(root, "skills", "lark-base", "SKILL.md"), "# base\n");
  fs.writeFileSync(path.join(root, "skills", "lark-base", "references", "read.md"), "# read\n");
  return root;
}

describe("stagePreviewPackage", () => {
  it("stages the binaries, the release launcher and the whole skills tree", () => {
    const root = makeRepo();
    const binDir = makeBinDir(["lark-cli-darwin-amd64", "lark-cli-darwin-arm64"]);
    const stageDir = path.join(root, "preview-package", "package");

    const staged = stagePreviewPackage({ repoRoot: root, binDir, stageDir, ...IDENTITY, ...RELEASE });

    assert.deepEqual(staged.binaries, ["lark-cli-darwin-amd64", "lark-cli-darwin-arm64"]);
    assert.equal(fs.readFileSync(path.join(stageDir, "bin", "lark-cli-darwin-arm64"), "utf8"), "bytes of lark-cli-darwin-arm64");
    if (process.platform !== "win32") {
      assert.equal(fs.statSync(path.join(stageDir, "bin", "lark-cli-darwin-arm64")).mode & 0o111, 0o111, "binaries are executable");
    }
    // The launcher is the release one, unmodified: one implementation of
    // platform dispatch, and one place where `update` is decided.
    const releaseDir = path.join(__dirname, "..", "release");
    for (const file of ["run.js", "postinstall.js"]) {
      assert.equal(
        fs.readFileSync(path.join(stageDir, "scripts", file), "utf8"),
        fs.readFileSync(path.join(releaseDir, file), "utf8"),
        `${file} is the release launcher`
      );
    }
    // skills/ travels inside: the intranet has no git host to clone from.
    assert.equal(fs.readFileSync(path.join(stageDir, "skills", "lark-base", "SKILL.md"), "utf8"), "# base\n");
    assert.ok(fs.existsSync(path.join(stageDir, "skills", "lark-base", "references", "read.md")), "references staged");
    for (const f of ["CHANGELOG.md", "LICENSE", "README.md"]) assert.ok(fs.existsSync(path.join(stageDir, f)), `${f} staged`);

    const written = JSON.parse(fs.readFileSync(path.join(stageDir, "package.json"), "utf8"));
    assert.equal(written.private, true);
    assert.equal(written.version, IDENTITY.version);
  });

  it("fails when the repository has no skills tree to ship", () => {
    const root = makeRepo();
    fs.rmSync(path.join(root, "skills"), { recursive: true });
    assert.throws(
      () =>
        stagePreviewPackage({
          repoRoot: root,
          binDir: makeBinDir(["lark-cli-darwin-arm64"]),
          stageDir: path.join(root, "preview-package", "package"),
          ...IDENTITY,
        }),
      /skills is missing/
    );
  });
});
