#!/usr/bin/env node
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Assembles the internal npm packages from a GoReleaser dist directory and
// packs them.
//
// The public package downloads its binary at install time from the GitHub
// release. The internal package cannot — the registry it lives on has no
// release page, and a download would hand out the public build — so the
// binaries travel through the registry instead, one package per platform:
//
//   <name>                     the launcher (scripts/run.js), no binary. Names
//                              every platform package as an optionalDependency
//                              pinned to the same version.
//   <name>-<goos>-<goarch>     bin/lark-cli for one platform, restricted to it
//                              by the package.json os/cpu fields.
//
// npm installs the launcher plus exactly the platform package the host can
// run, and skips the others without downloading them. A cold install therefore
// costs one binary, not seven — the shape esbuild and swc ship in — and a
// sandbox that has no npm can fetch the platform tarball by URL and untar it.
// Nothing in the source tree is edited: the package.json files are derived
// here from the public one, and the launcher lives next to this script.
//
// Usage:
//   node .codebase/release/build-registry-package.js --dist dist --out release-candidate \
//     --name @scope/name --registry https://registry.example
//
// Writes one <out>/<file>.tgz per package and <out>/manifest.json, and prints
// the manifest to stdout. The manifest describes the launcher package at the
// top level — the identity later stages check — and lists every platform
// package under `platform_packages`.

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");

const { ARCHES, PLATFORMS, bundledBinaryFile, platformBinaryFile } = require("./run.js");

const PACKAGE_NAME_PATTERN = /^@[a-z0-9][a-z0-9._-]*\/[a-z0-9][a-z0-9._-]*$/;
const SHIPPED_ROOT_FILES = ["CHANGELOG.md", "LICENSE", "README.md"];

// GoReleaser's GOOS/GOARCH back to the names npm's os/cpu fields use, from the
// same tables the launcher resolves the host with.
const NODE_OS = Object.fromEntries(Object.entries(PLATFORMS).map(([node, go]) => [go, node]));
const NODE_CPU = Object.fromEntries(Object.entries(ARCHES).map(([node, go]) => [go, node]));

function parseRegistry(registry) {
  let parsed;
  try {
    parsed = new URL(registry);
  } catch (_) {
    parsed = null;
  }
  if (!parsed || parsed.protocol !== "https:") {
    throw new Error(`Registry must be an https URL, got ${JSON.stringify(registry)}`);
  }
  return parsed.origin + parsed.pathname.replace(/\/+$/, "");
}

function checkPackageName(name) {
  if (typeof name !== "string" || !PACKAGE_NAME_PATTERN.test(name)) {
    throw new Error(`Package name must be a scoped npm name, got ${JSON.stringify(name)}`);
  }
}

function checkSource(base) {
  for (const field of ["version", "description", "license"]) {
    if (typeof base[field] !== "string" || !base[field]) {
      throw new Error(`source package.json lacks ${field}`);
    }
  }
}

/** Name of the platform package for one GoReleaser binary. */
function platformPackageName(name, binary) {
  return `${name}-${binary.goos}-${binary.goarch}`;
}

/**
 * The launcher package. It carries no binary and no postinstall: the platform
 * package is an optionalDependency pinned to this exact version, so what npm
 * installs next to it is always the matching build, and there is nothing to
 * prune afterwards.
 */
function deriveLauncherPackageJson(base, { name, registry, binaries }) {
  checkPackageName(name);
  const publishRegistry = parseRegistry(registry);
  checkSource(base);
  if (!Array.isArray(binaries) || binaries.length === 0) {
    throw new Error("the launcher package needs at least one platform package to depend on");
  }
  const optionalDependencies = {};
  for (const binary of binaries) optionalDependencies[platformPackageName(name, binary)] = base.version;
  return {
    name,
    version: base.version,
    description: base.description,
    bin: { "lark-cli": "scripts/run.js" },
    os: base.os,
    cpu: base.cpu,
    engines: base.engines,
    license: base.license,
    publishConfig: { registry: publishRegistry },
    optionalDependencies,
    files: ["scripts/run.js", "CHANGELOG.md"],
  };
}

/**
 * One platform package: the binary under bin/, installable only where it
 * runs. No `bin` field — the launcher package owns the `lark-cli` command, and
 * a second link to the same name would make the two packages fight over it.
 */
function derivePlatformPackageJson(base, { name, registry, binary }) {
  checkPackageName(name);
  const publishRegistry = parseRegistry(registry);
  checkSource(base);
  const os = NODE_OS[binary.goos];
  const cpu = NODE_CPU[binary.goarch];
  if (!os || !cpu) {
    throw new Error(`no npm os/cpu name for ${binary.goos}/${binary.goarch}; add it to the launcher's tables`);
  }
  return {
    name: platformPackageName(name, binary),
    version: base.version,
    description: `${base.description} (${binary.goos}/${binary.goarch} binary for ${name})`,
    os: [os],
    cpu: [cpu],
    engines: base.engines,
    license: base.license,
    publishConfig: { registry: publishRegistry },
    files: ["bin/"],
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
    binaries.push({ goos, goarch, path: relPath, target: bundledBinaryFile(goos, goarch) });
  }
  if (binaries.length === 0) throw new Error("artifacts.json lists no binaries");
  return binaries.sort((a, b) => a.target.localeCompare(b.target));
}

function copyBinary(source, dest, label) {
  if (!fs.existsSync(source)) throw new Error(`binary for ${label} is missing at ${source}`);
  fs.copyFileSync(source, dest);
  fs.chmodSync(dest, 0o755);
}

// Lays out a bundled package directory: one binary per platform under bin/,
// the launcher and its postinstall under scripts/, the shipped root files, any
// whole directory the package carries, and the package.json. The preview
// package in .codebase/preview is built this way, because it is installed
// from a tarball and cannot depend on published platform packages.
function stageTree({ repoRoot, stageDir, binaries, packageJson, launcherDir = __dirname, extraDirs = [] }) {
  for (const binary of binaries) {
    if (!fs.existsSync(binary.source)) {
      throw new Error(`binary for ${binary.target} is missing at ${binary.source}`);
    }
  }

  fs.rmSync(stageDir, { recursive: true, force: true });
  fs.mkdirSync(path.join(stageDir, "bin"), { recursive: true });
  fs.mkdirSync(path.join(stageDir, "scripts"), { recursive: true });
  for (const binary of binaries) copyBinary(binary.source, path.join(stageDir, "bin", binary.target), binary.target);
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

/** The launcher package tree: scripts/run.js, the root files and package.json. */
function stageLauncher({ repoRoot, stageDir, packageJson, launcherDir = __dirname }) {
  fs.rmSync(stageDir, { recursive: true, force: true });
  fs.mkdirSync(path.join(stageDir, "scripts"), { recursive: true });
  fs.copyFileSync(path.join(launcherDir, "run.js"), path.join(stageDir, "scripts", "run.js"));
  for (const file of SHIPPED_ROOT_FILES) {
    fs.copyFileSync(path.join(repoRoot, file), path.join(stageDir, file));
  }
  fs.writeFileSync(path.join(stageDir, "package.json"), JSON.stringify(packageJson, null, 2) + "\n");
  return { kind: "launcher", packageJson };
}

/** One platform package tree: bin/lark-cli[.exe], LICENSE and package.json. */
function stagePlatform({ repoRoot, stageDir, binary, packageJson }) {
  fs.rmSync(stageDir, { recursive: true, force: true });
  fs.mkdirSync(path.join(stageDir, "bin"), { recursive: true });
  copyBinary(binary.source, path.join(stageDir, "bin", platformBinaryFile(binary.goos)), binary.target);
  fs.copyFileSync(path.join(repoRoot, "LICENSE"), path.join(stageDir, "LICENSE"));
  fs.writeFileSync(path.join(stageDir, "package.json"), JSON.stringify(packageJson, null, 2) + "\n");
  return { kind: "platform", packageJson, binary: binary.target };
}

/**
 * Stages every package of the release under <stageRoot>/<dir>: the launcher in
 * `launcher/`, each platform package in `<goos>-<goarch>/`. Returns the staged
 * trees in publish order — platform packages first, the launcher last, so
 * the launcher's optionalDependencies are resolvable from the moment it is
 * visible on the registry.
 */
function stagePackages({ repoRoot, distDir, stageRoot, name, registry, launcherDir = __dirname }) {
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

  const staged = [];
  for (const binary of binaries) {
    const packageJson = derivePlatformPackageJson(base, { name, registry, binary });
    const stageDir = path.join(stageRoot, `${binary.goos}-${binary.goarch}`);
    staged.push({ ...stagePlatform({ repoRoot, stageDir, binary, packageJson }), stageDir, goos: binary.goos, goarch: binary.goarch });
  }
  const launcher = deriveLauncherPackageJson(base, { name, registry, binaries });
  staged.push({ ...stageLauncher({ repoRoot, stageDir: path.join(stageRoot, "launcher"), packageJson: launcher, launcherDir }), stageDir: path.join(stageRoot, "launcher") });
  return staged;
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

function pack(stageDir, outDir, expected) {
  const packs = JSON.parse(
    execFileSync("npm", ["pack", "--ignore-scripts", "--json", "--pack-destination", outDir], {
      cwd: stageDir,
      encoding: "utf8",
      stdio: ["ignore", "pipe", "inherit"],
    })
  );
  if (!Array.isArray(packs) || packs.length !== 1) {
    throw new Error(`npm pack of ${expected.name} did not produce exactly one package`);
  }
  const result = packs[0];
  if (result.name !== expected.name || result.version !== expected.version) {
    throw new Error(`npm pack produced ${result.name}@${result.version}, expected ${expected.name}@${expected.version}`);
  }
  return { filename: result.filename, integrity: result.integrity, shasum: result.shasum, size: result.size };
}

function main() {
  const args = parseArgs(process.argv.slice(2));
  const repoRoot = path.resolve(__dirname, "..", "..");
  const distDir = path.resolve(args.dist);
  const outDir = path.resolve(args.out);
  const stageRoot = path.join(outDir, "stage");

  const staged = stagePackages({ repoRoot, distDir, stageRoot, name: args.name, registry: args.registry });
  const packed = staged.map((tree) => ({ ...tree, ...pack(tree.stageDir, outDir, tree.packageJson) }));
  // The staging trees duplicate every binary; only the tarballs travel on.
  fs.rmSync(stageRoot, { recursive: true, force: true });

  // The manifest's top level is the launcher's identity; which tree that is
  // comes from the tree itself, not from where stagePackages happened to put it.
  const launchers = packed.filter((p) => p.kind === "launcher");
  if (launchers.length !== 1) throw new Error(`expected exactly one launcher package, staged ${launchers.length}`);
  const launcher = launchers[0];
  const manifest = {
    name: launcher.packageJson.name,
    version: launcher.packageJson.version,
    registry: launcher.packageJson.publishConfig.registry,
    filename: launcher.filename,
    integrity: launcher.integrity,
    shasum: launcher.shasum,
    size: launcher.size,
    platform_packages: packed
      .filter((p) => p.kind === "platform")
      .map((p) => ({ name: p.packageJson.name, goos: p.goos, goarch: p.goarch, filename: p.filename, integrity: p.integrity, shasum: p.shasum, size: p.size })),
  };
  fs.writeFileSync(path.join(outDir, "manifest.json"), JSON.stringify(manifest, null, 2) + "\n");
  process.stdout.write(JSON.stringify(manifest, null, 2) + "\n");
}

module.exports = { deriveLauncherPackageJson, derivePlatformPackageJson, platformPackageName, selectBinaries, stagePackages, stageTree };

if (require.main === module) {
  try {
    main();
  } catch (err) {
    console.error(`build-registry-package: ${err.message}`);
    process.exit(1);
  }
}
