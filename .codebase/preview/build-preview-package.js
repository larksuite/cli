#!/usr/bin/env node
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Packs the preview package for one merge request from binaries that
// .codebase/preview/build.sh has already cross-compiled.
//
// The layout is the internal release package's, reused through stageTree:
// one binary per platform under bin/, the same launcher and postinstall, the
// shipped root files. Four things differ, and each one is a decision:
//
// - The binaries come from a plain `go build` per platform rather than
//   GoReleaser. A preview needs two platforms and neither a changelog nor
//   checksums, and GoReleaser would cost minutes for artifacts nobody reads.
// - The version is a prerelease naming the merge request and the commit, so
//   `lark-cli --version` identifies the build under test.
// - `private: true` and no publishConfig. npm refuses to publish a private
//   package, so a token that reaches this job cannot turn a preview into a
//   release; the launcher reads the absent publishConfig as "no channel to
//   update along" and says so instead of resolving some other build.
// - skills/ travels inside the package. The binary embeds its own copy for
//   `lark-cli skills read`, but an agent needs the files on disk, and the
//   internal network has no git host `npx skills add` can clone from.
//
// Usage:
//   node .codebase/preview/build-preview-package.js --bin-dir <dir> --out preview-package \
//     --name @scope/name --version 1.2.3-mr7.gabc1234 --label mr7 --commit abc1234 \
//     [--release-package @scope/cli --release-registry https://registry.example]
//
// Writes <out>/<name>-<version>.tgz and <out>/manifest.json, and prints the
// manifest to stdout.

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

const { stageTree } = require("../release/build-registry-package.js");

const PACKAGE_NAME_PATTERN = /^@[a-z0-9][a-z0-9._-]*\/[a-z0-9][a-z0-9._-]*$/;
const BINARY_PATTERN = /^lark-cli-(darwin|linux|windows)-(amd64|arm64|riscv64)(\.exe)?$/;
const PRERELEASE_VERSION_PATTERN =
  /^(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)$/;
const PACKAGE_DIRS = ["skills"];

/**
 * A preview version must be a prerelease, so it can never be mistaken for a
 * release, and every identifier must be one npm accepts: a numeric identifier
 * carrying a leading zero is invalid semver and npm rejects the package at
 * pack time with a message that does not mention the version.
 */
function validatePreviewVersion(version) {
  const match = PRERELEASE_VERSION_PATTERN.exec(String(version));
  if (!match) {
    throw new Error(`Preview version must be X.Y.Z-<prerelease>, got ${JSON.stringify(version)}`);
  }
  for (const identifier of match[1].split(".")) {
    if (/^\d+$/.test(identifier) && identifier.length > 1 && identifier.startsWith("0")) {
      throw new Error(`Preview version identifier ${JSON.stringify(identifier)} has a leading zero; npm rejects it`);
    }
  }
  return version;
}

function derivePreviewPackageJson(base, { name, version, label, commit, releasePackage, releaseRegistry }) {
  if (typeof name !== "string" || !PACKAGE_NAME_PATTERN.test(name)) {
    throw new Error(`Package name must be a scoped npm name, got ${JSON.stringify(name)}`);
  }
  if (name === base.name) {
    throw new Error(`Preview package must not reuse the source package name ${JSON.stringify(name)}`);
  }
  validatePreviewVersion(version);
  for (const [field, value] of [["label", label], ["commit", commit]]) {
    if (typeof value !== "string" || !value) throw new Error(`preview ${field} is required`);
  }
  for (const field of ["description", "license"]) {
    if (typeof base[field] !== "string" || !base[field]) {
      throw new Error(`source package.json lacks ${field}`);
    }
  }

  // What the launcher needs to tell someone how to get back to a release
  // build. Omitted rather than guessed when the caller names no release.
  const larkPreview = { label, commit };
  if (releasePackage !== undefined || releaseRegistry !== undefined) {
    if (typeof releasePackage !== "string" || !PACKAGE_NAME_PATTERN.test(releasePackage)) {
      throw new Error(`Release package must be a scoped npm name, got ${JSON.stringify(releasePackage)}`);
    }
    let parsed;
    try {
      parsed = new URL(releaseRegistry);
    } catch (_) {
      parsed = null;
    }
    if (!parsed || parsed.protocol !== "https:") {
      throw new Error(`Release registry must be an https URL, got ${JSON.stringify(releaseRegistry)}`);
    }
    larkPreview.releasePackage = releasePackage;
    larkPreview.releaseRegistry = parsed.origin + parsed.pathname.replace(/\/+$/, "");
  }

  return {
    name,
    version,
    description: `${base.description} (preview build of ${label} at ${commit})`,
    bin: { "lark-cli": "scripts/run.js" },
    scripts: { postinstall: "node scripts/postinstall.js" },
    os: base.os,
    cpu: base.cpu,
    engines: base.engines,
    license: base.license,
    // Never published: npm refuses `publish` on a private package, and the
    // launcher treats a missing publishConfig as "this build cannot update".
    private: true,
    larkPreview,
    files: ["bin/", "scripts/run.js", "scripts/postinstall.js", "skills/", "CHANGELOG.md"],
  };
}

/**
 * Every file in the binary directory must be a platform binary this package
 * can dispatch to. An unexpected name is an error rather than something to
 * skip: it means the build wrote somewhere the launcher will not look.
 */
function collectBinaries(binDir) {
  if (!fs.existsSync(binDir)) throw new Error(`binary directory ${binDir} does not exist`);
  const binaries = [];
  for (const entry of fs.readdirSync(binDir).sort()) {
    const match = BINARY_PATTERN.exec(entry);
    if (!match) {
      throw new Error(`${binDir} holds ${entry}, which is not a lark-cli-<goos>-<goarch> binary`);
    }
    if ((match[1] === "windows") !== Boolean(match[3])) {
      throw new Error(`${entry} has the wrong extension for ${match[1]}`);
    }
    binaries.push({ target: entry, source: path.join(binDir, entry) });
  }
  if (binaries.length === 0) throw new Error(`${binDir} holds no binaries; build them before packaging`);
  return binaries;
}

function stagePreviewPackage({ repoRoot, binDir, stageDir, ...identity }) {
  const binaries = collectBinaries(binDir);
  const base = JSON.parse(fs.readFileSync(path.join(repoRoot, "package.json"), "utf8"));
  const packageJson = derivePreviewPackageJson(base, identity);
  return stageTree({ repoRoot, stageDir, binaries, packageJson, extraDirs: PACKAGE_DIRS });
}

function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i += 2) {
    const key = argv[i];
    const value = argv[i + 1];
    if (!key.startsWith("--") || value === undefined) {
      throw new Error(`Expected --key value pairs, got ${JSON.stringify(argv)}`);
    }
    args[key.slice(2).replace(/-([a-z])/g, (_, c) => c.toUpperCase())] = value;
  }
  for (const required of ["binDir", "out", "name", "version", "label", "commit"]) {
    if (!args[required]) throw new Error(`--${required.replace(/[A-Z]/g, (c) => "-" + c.toLowerCase())} is required`);
  }
  return args;
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  const repoRoot = path.resolve(__dirname, "..", "..");
  const outDir = path.resolve(args.out);
  const stageDir = path.join(outDir, "package");

  const staged = stagePreviewPackage({
    repoRoot,
    binDir: path.resolve(args.binDir),
    stageDir,
    name: args.name,
    version: args.version,
    label: args.label,
    commit: args.commit,
    releasePackage: args.releasePackage,
    releaseRegistry: args.releaseRegistry,
  });

  const packs = JSON.parse(
    execFileSync("npm", ["pack", "--ignore-scripts", "--json", "--pack-destination", outDir], {
      cwd: stageDir,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "inherit"],
    })
  );
  if (!Array.isArray(packs) || packs.length !== 1) {
    throw new Error("npm pack did not produce exactly one package");
  }
  const pack = packs[0];
  if (pack.name !== staged.packageJson.name || pack.version !== staged.packageJson.version) {
    throw new Error(
      `npm pack produced ${pack.name}@${pack.version}, expected ${staged.packageJson.name}@${staged.packageJson.version}`
    );
  }
  // The staging tree duplicates every binary and the whole skills tree; only
  // the tarball travels on to the artifact.
  fs.rmSync(stageDir, { recursive: true, force: true });

  const manifest = {
    name: pack.name,
    version: pack.version,
    label: staged.packageJson.larkPreview.label,
    commit: staged.packageJson.larkPreview.commit,
    // Carried so the job can print how to get back to a released build
    // without knowing the release identity itself.
    releasePackage: staged.packageJson.larkPreview.releasePackage || null,
    filename: pack.filename,
    integrity: pack.integrity,
    shasum: pack.shasum,
    size: pack.size,
    binaries: staged.binaries,
  };
  fs.writeFileSync(path.join(outDir, "manifest.json"), JSON.stringify(manifest, null, 2) + "\n");
  process.stdout.write(JSON.stringify(manifest, null, 2) + "\n");
}

module.exports = { collectBinaries, derivePreviewPackageJson, stagePreviewPackage, validatePreviewVersion };

if (require.main === module) {
  try {
    main();
  } catch (err) {
    console.error(`build-preview-package: ${err.message}`);
    process.exit(1);
  }
}
