#!/usr/bin/env node
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Assembles the internal npm package from a GoReleaser dist directory and
// packs it.
//
// The public package downloads its binary at install time from the GitHub
// release. The internal package cannot — the registry it lives on has no
// release page, and a download would hand out the public build — so every
// binary GoReleaser produced travels inside the package under bin/, and the
// launcher in run.js picks the one for the host. Nothing in the source tree is
// edited: the package.json is derived here from the public one, and the
// launcher and postinstall live next to this script.
//
// Usage:
//   node .codebase/release/build-registry-package.js --dist dist --out release-candidate \
//     --name @scope/name --registry https://registry.example
//
// Writes <out>/<name>-<version>.tgz and <out>/manifest.json, and prints the
// manifest to stdout.

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

const PACKAGE_NAME_PATTERN = /^@[a-z0-9][a-z0-9._-]*\/[a-z0-9][a-z0-9._-]*$/;
const SHIPPED_ROOT_FILES = ["CHANGELOG.md", "LICENSE", "README.md"];

function derivePackageJson(base, { name, registry }) {
  if (typeof name !== "string" || !PACKAGE_NAME_PATTERN.test(name)) {
    throw new Error(`Package name must be a scoped npm name, got ${JSON.stringify(name)}`);
  }
  let parsed;
  try {
    parsed = new URL(registry);
  } catch (_) {
    parsed = null;
  }
  if (!parsed || parsed.protocol !== "https:") {
    throw new Error(`Registry must be an https URL, got ${JSON.stringify(registry)}`);
  }
  for (const field of ["version", "description", "license"]) {
    if (typeof base[field] !== "string" || !base[field]) {
      throw new Error(`source package.json lacks ${field}`);
    }
  }
  // Only what the internal package needs. No dependencies: the install wizard
  // is not shipped, and no postinstall download exists to configure.
  return {
    name,
    version: base.version,
    description: base.description,
    bin: { "lark-cli": "scripts/run.js" },
    scripts: { postinstall: "node scripts/postinstall.js" },
    os: base.os,
    cpu: base.cpu,
    engines: base.engines,
    license: base.license,
    publishConfig: { registry: parsed.origin + parsed.pathname.replace(/\/+$/, "") },
    files: ["bin/", "scripts/run.js", "scripts/postinstall.js", "CHANGELOG.md"],
  };
}

// GoReleaser's dist/artifacts.json lists every artifact it produced; the
// binaries are the entries of type "Binary".
function selectBinaries(artifacts) {
  if (!Array.isArray(artifacts)) throw new Error("artifacts.json must be an array");
  const binaries = [];
  const seen = new Set();
  for (const artifact of artifacts) {
    if (artifact.type !== "Binary") continue;
    const { goos, goarch, path: relPath } = artifact;
    if (!goos || !goarch || !relPath) {
      throw new Error(`binary artifact lacks goos/goarch/path: ${JSON.stringify(artifact)}`);
    }
    const key = `${goos}-${goarch}`;
    if (seen.has(key)) throw new Error(`more than one binary for ${key}`);
    seen.add(key);
    binaries.push({ goos, goarch, path: relPath, target: `lark-cli-${key}${goos === "windows" ? ".exe" : ""}` });
  }
  if (binaries.length === 0) throw new Error("artifacts.json lists no binaries");
  return binaries.sort((a, b) => a.target.localeCompare(b.target));
}

// Lays out a package directory: one binary per platform under bin/, the
// launcher and its postinstall under scripts/, the shipped root files, any
// whole directory the package carries, and the package.json. The preview
// package in .codebase/preview builds on the same layout, so this stays
// independent of where the binaries came from.
function stageTree({ repoRoot, stageDir, binaries, packageJson, launcherDir = __dirname, extraDirs = [] }) {
  for (const binary of binaries) {
    if (!fs.existsSync(binary.source)) {
      throw new Error(`binary for ${binary.target} is missing at ${binary.source}`);
    }
  }

  fs.rmSync(stageDir, { recursive: true, force: true });
  fs.mkdirSync(path.join(stageDir, "bin"), { recursive: true });
  fs.mkdirSync(path.join(stageDir, "scripts"), { recursive: true });
  for (const binary of binaries) {
    const dest = path.join(stageDir, "bin", binary.target);
    fs.copyFileSync(binary.source, dest);
    fs.chmodSync(dest, 0o755);
  }
  for (const file of ["run.js", "postinstall.js"]) {
    fs.copyFileSync(path.join(launcherDir, file), path.join(stageDir, "scripts", file));
  }
  for (const file of SHIPPED_ROOT_FILES) {
    fs.copyFileSync(path.join(repoRoot, file), path.join(stageDir, file));
  }
  for (const dir of extraDirs) {
    const source = path.join(repoRoot, dir);
    if (!fs.existsSync(source)) throw new Error(`package directory ${dir} is missing`);
    fs.cpSync(source, path.join(stageDir, dir), { recursive: true });
  }
  fs.writeFileSync(path.join(stageDir, "package.json"), JSON.stringify(packageJson, null, 2) + "\n");

  return { packageJson, binaries: binaries.map((b) => b.target) };
}

function stagePackage({ repoRoot, distDir, stageDir, name, registry, launcherDir = __dirname }) {
  const artifactsPath = path.join(distDir, "artifacts.json");
  if (!fs.existsSync(artifactsPath)) {
    throw new Error(`${artifactsPath} not found; run GoReleaser before packaging`);
  }
  const binaries = selectBinaries(JSON.parse(fs.readFileSync(artifactsPath, "utf8")));
  for (const binary of binaries) {
    // GoReleaser records paths relative to the directory it ran in.
    binary.source = path.isAbsolute(binary.path) ? binary.path : path.join(path.dirname(distDir), binary.path);
    if (!fs.existsSync(binary.source)) {
      throw new Error(`artifacts.json lists ${binary.path} but it is missing`);
    }
  }
  const base = JSON.parse(fs.readFileSync(path.join(repoRoot, "package.json"), "utf8"));
  const packageJson = derivePackageJson(base, { name, registry });

  return stageTree({ repoRoot, stageDir, binaries, packageJson, launcherDir });
}

function parseArgs(argv) {
  const args = {};
  for (let i = 0; i < argv.length; i += 2) {
    const key = argv[i];
    const value = argv[i + 1];
    if (!key.startsWith("--") || value === undefined) {
      throw new Error(`Expected --key value pairs, got ${JSON.stringify(argv)}`);
    }
    args[key.slice(2)] = value;
  }
  for (const required of ["dist", "out", "name", "registry"]) {
    if (!args[required]) throw new Error(`--${required} is required`);
  }
  return args;
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  const repoRoot = path.resolve(__dirname, "..", "..");
  const distDir = path.resolve(args.dist);
  const outDir = path.resolve(args.out);
  const stageDir = path.join(outDir, "package");

  const staged = stagePackage({ repoRoot, distDir, stageDir, name: args.name, registry: args.registry });

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
    throw new Error(`npm pack produced ${pack.name}@${pack.version}, expected ${staged.packageJson.name}@${staged.packageJson.version}`);
  }
  // The staging tree duplicates every binary; only the tarball travels on.
  fs.rmSync(stageDir, { recursive: true, force: true });

  const manifest = {
    name: pack.name,
    version: pack.version,
    registry: staged.packageJson.publishConfig.registry,
    filename: pack.filename,
    integrity: pack.integrity,
    shasum: pack.shasum,
    size: pack.size,
    binaries: staged.binaries,
  };
  fs.writeFileSync(path.join(outDir, "manifest.json"), JSON.stringify(manifest, null, 2) + "\n");
  process.stdout.write(JSON.stringify(manifest, null, 2) + "\n");
}

module.exports = { derivePackageJson, selectBinaries, stagePackage, stageTree };

if (require.main === module) {
  try {
    main();
  } catch (err) {
    console.error(`build-registry-package: ${err.message}`);
    process.exit(1);
  }
}
