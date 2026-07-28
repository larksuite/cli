#!/usr/bin/env node
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const crypto = require("node:crypto");
const fs = require("node:fs");
const path = require("node:path");
const { isDeepStrictEqual } = require("node:util");

const MANIFEST_NAME = "candidate-manifest.json";
const RELEASE_VERSION_PATTERN =
  /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-beta\.(0|[1-9][0-9]*))?$/;
const SHA_PATTERN = /^[0-9a-fA-F]{40}$/;
const SHA256_PATTERN = /^[0-9a-fA-F]{64}$/;

function fail(message) {
  throw new Error(message);
}

function hasOwn(value, key) {
  return Object.prototype.hasOwnProperty.call(value, key);
}

function assertObject(value, label) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) {
    fail(`${label} must be an object`);
  }
}

function assertExactKeys(value, expectedKeys, label) {
  assertObject(value, label);
  const expected = new Set(expectedKeys);
  const unexpected = Object.keys(value).filter((key) => !expected.has(key)).sort();
  const missing = expectedKeys.filter((key) => !hasOwn(value, key));
  if (unexpected.length > 0) {
    fail(`${label} has unexpected field: ${unexpected.join(", ")}`);
  }
  if (missing.length > 0) {
    fail(`${label} is missing required field: ${missing.join(", ")}`);
  }
}

function normalizeSourceSha(sourceSha, label = "sourceSha") {
  if (typeof sourceSha !== "string" || !SHA_PATTERN.test(sourceSha)) {
    fail(`${label} must be exactly 40 hexadecimal characters`);
  }
  return sourceSha.toLowerCase();
}

function parseReleaseVersion(version) {
  const match = typeof version === "string" ? RELEASE_VERSION_PATTERN.exec(version) : null;
  if (!match) {
    fail(
      "release version must use Stable X.Y.Z or Beta X.Y.Z-beta.N; "
      + "other prerelease labels, build metadata, and leading zeros are not allowed",
    );
  }
  return {
    version,
    channel: match[4] === undefined ? "stable" : "beta",
    major: Number(match[1]),
    minor: Number(match[2]),
    patch: Number(match[3]),
    beta: match[4] === undefined ? null : Number(match[4]),
  };
}

function validateChannel(version, channel, label = "metadata") {
  const parsed = parseReleaseVersion(version);
  if (channel !== "stable" && channel !== "beta") {
    fail(`${label}.channel must be stable or beta`);
  }
  if (parsed.channel !== channel) {
    fail(`${label}.version requires channel ${parsed.channel}, received ${channel}`);
  }
  return parsed;
}

function assertSafeFilename(name, label) {
  if (
    typeof name !== "string"
    || name.length === 0
    || name === "."
    || name === ".."
    || name.includes("/")
    || name.includes("\\")
    || path.basename(name) !== name
  ) {
    fail(`${label} must be a safe basename without path separators`);
  }
}

function filePath(directory, name, label) {
  assertSafeFilename(name, label);
  return path.join(directory, name);
}

function assertRegularFile(directory, name, label) {
  const target = filePath(directory, name, label);
  let stat;
  try {
    stat = fs.lstatSync(target);
  } catch (error) {
    fail(`${label} ${name} could not be inspected: ${error.message}`);
  }
  if (stat.isSymbolicLink() || !stat.isFile()) {
    fail(`${label} ${name} must be a regular file, not a symlink or other file type`);
  }
  return target;
}

function listRegularFiles(directory) {
  let stat;
  try {
    stat = fs.lstatSync(directory);
  } catch (error) {
    fail(`candidate directory could not be inspected: ${error.message}`);
  }
  if (stat.isSymbolicLink() || !stat.isDirectory()) {
    fail("candidate directory must be a directory and must not be a symlink");
  }

  let names;
  try {
    names = fs.readdirSync(directory);
  } catch (error) {
    fail(`candidate directory could not be read: ${error.message}`);
  }
  names.sort();
  for (const name of names) {
    assertRegularFile(directory, name, "candidate entry");
  }
  return names;
}

function hashFile(target, algorithms) {
  const hashes = algorithms.map((algorithm) => crypto.createHash(algorithm));
  const buffer = Buffer.allocUnsafe(64 * 1024);
  let descriptor;
  try {
    descriptor = fs.openSync(target, "r");
    for (;;) {
      const length = fs.readSync(descriptor, buffer, 0, buffer.length, null);
      if (length === 0) break;
      const chunk = buffer.subarray(0, length);
      for (const hash of hashes) hash.update(chunk);
    }
  } catch (error) {
    fail(`could not hash ${path.basename(target)}: ${error.message}`);
  } finally {
    if (descriptor !== undefined) fs.closeSync(descriptor);
  }
  return hashes.map((hash) => hash.digest());
}

function sha256File(target) {
  return hashFile(target, ["sha256"])[0].toString("hex");
}

function npmDigests(target) {
  const [sha256, sha512] = hashFile(target, ["sha256", "sha512"]);
  return {
    sha256: sha256.toString("hex"),
    integrity: `sha512-${sha512.toString("base64")}`,
  };
}

function validateCreateMetadata(metadata) {
  assertObject(metadata, "metadata");
  const sourceSha = normalizeSourceSha(metadata.sourceSha, "metadata.sourceSha");
  if (typeof metadata.version !== "string") {
    fail("metadata.version must be a string");
  }
  validateChannel(metadata.version, metadata.channel);
  assertSafeFilename(metadata.npmPackage, "metadata.npmPackage");
  if (!metadata.npmPackage.endsWith(".tgz")) {
    fail("metadata.npmPackage must designate an npm .tgz file");
  }
  return {
    sourceSha,
    version: metadata.version,
    channel: metadata.channel,
    npmPackage: metadata.npmPackage,
  };
}

function createCandidateManifest(directory, metadata) {
  const normalized = validateCreateMetadata(metadata);
  const names = listRegularFiles(directory);
  if (!names.includes(normalized.npmPackage)) {
    fail(`designated npm package is missing: ${normalized.npmPackage}`);
  }

  const releaseAssetNames = names.filter(
    (name) => name !== MANIFEST_NAME && name !== normalized.npmPackage,
  );
  const releaseAssets = releaseAssetNames.map((name) => ({
    name,
    sha256: sha256File(assertRegularFile(directory, name, "release asset")),
  }));
  const npmTarget = assertRegularFile(
    directory,
    normalized.npmPackage,
    "designated npm package",
  );

  return {
    schemaVersion: 1,
    sourceSha: normalized.sourceSha,
    version: normalized.version,
    channel: normalized.channel,
    releaseAssets,
    npmPackage: {
      name: normalized.npmPackage,
      ...npmDigests(npmTarget),
    },
  };
}

function validateSha256(value, label) {
  if (typeof value !== "string" || !SHA256_PATTERN.test(value)) {
    fail(`${label} must be exactly 64 hexadecimal characters`);
  }
  return value.toLowerCase();
}

function validateIntegrity(value, label) {
  if (typeof value !== "string" || !value.startsWith("sha512-") || value.length === 7) {
    fail(`${label} must be a sha512 SRI string`);
  }
  const encoded = value.slice(7);
  if (!/^[A-Za-z0-9+/]+={0,2}$/.test(encoded)) {
    fail(`${label} must be a sha512 SRI string`);
  }
  const decoded = Buffer.from(encoded, "base64");
  if (decoded.length !== 64 || decoded.toString("base64") !== encoded) {
    fail(`${label} must contain one canonical SHA-512 digest`);
  }
  return value;
}

function validateManifest(manifest) {
  assertExactKeys(
    manifest,
    [
      "schemaVersion",
      "sourceSha",
      "version",
      "channel",
      "releaseAssets",
      "npmPackage",
    ],
    "manifest",
  );
  if (manifest.schemaVersion !== 1) {
    fail("manifest.schemaVersion must be 1");
  }
  const sourceSha = normalizeSourceSha(manifest.sourceSha, "manifest.sourceSha");
  if (typeof manifest.version !== "string") {
    fail("manifest.version must be a string");
  }
  validateChannel(manifest.version, manifest.channel, "manifest");
  if (!Array.isArray(manifest.releaseAssets)) {
    fail("manifest.releaseAssets must be an array");
  }

  const seen = new Set();
  let previousName = null;
  const releaseAssets = manifest.releaseAssets.map((asset, index) => {
    const label = `manifest.releaseAssets[${index}]`;
    assertExactKeys(asset, ["name", "sha256"], label);
    assertSafeFilename(asset.name, `${label}.name`);
    if (seen.has(asset.name)) {
      fail(`manifest contains duplicate release asset: ${asset.name}`);
    }
    if (previousName !== null && previousName >= asset.name) {
      fail("manifest.releaseAssets must be sorted by name");
    }
    seen.add(asset.name);
    previousName = asset.name;
    return {
      name: asset.name,
      sha256: validateSha256(asset.sha256, `${label}.sha256`),
    };
  });

  assertExactKeys(manifest.npmPackage, ["name", "sha256", "integrity"], "manifest.npmPackage");
  assertSafeFilename(manifest.npmPackage.name, "manifest.npmPackage.name");
  if (seen.has(manifest.npmPackage.name)) {
    fail(`release asset ${manifest.npmPackage.name} duplicates npm package`);
  }
  if (!manifest.npmPackage.name.endsWith(".tgz")) {
    fail("manifest.npmPackage.name must designate an npm .tgz file");
  }

  return {
    sourceSha,
    version: manifest.version,
    channel: manifest.channel,
    releaseAssets,
    npmPackage: {
      name: manifest.npmPackage.name,
      sha256: validateSha256(
        manifest.npmPackage.sha256,
        "manifest.npmPackage.sha256",
      ),
      integrity: validateIntegrity(
        manifest.npmPackage.integrity,
        "manifest.npmPackage.integrity",
      ),
    },
  };
}

function validateExpectedMetadata(expectedMetadata) {
  assertObject(expectedMetadata, "expected metadata");
  const sourceSha = normalizeSourceSha(
    expectedMetadata.sourceSha,
    "expected metadata.sourceSha",
  );
  if (typeof expectedMetadata.version !== "string") {
    fail("expected metadata.version must be a string");
  }
  validateChannel(expectedMetadata.version, expectedMetadata.channel, "expected metadata");
  return {
    sourceSha,
    version: expectedMetadata.version,
    channel: expectedMetadata.channel,
  };
}

function describeSetMismatch(label, expected, actual) {
  const expectedSet = new Set(expected);
  const actualSet = new Set(actual);
  const missing = expected.filter((name) => !actualSet.has(name));
  const unexpected = actual.filter((name) => !expectedSet.has(name));
  if (missing.length === 0 && unexpected.length === 0) return;
  fail(
    `${label} set does not match manifest `
    + `(missing: ${missing.length > 0 ? missing.join(", ") : "none"}; `
    + `unexpected: ${unexpected.length > 0 ? unexpected.join(", ") : "none"})`,
  );
}

function verifyCandidateManifest(directory, manifest, expectedMetadata, scope) {
  if (scope !== "artifact" && scope !== "release") {
    fail("verification scope must be artifact or release");
  }
  const validated = validateManifest(manifest);
  const expected = validateExpectedMetadata(expectedMetadata);
  if (validated.sourceSha !== expected.sourceSha) {
    fail(
      `manifest sourceSha does not match expected sourceSha `
      + `(${validated.sourceSha} != ${expected.sourceSha})`,
    );
  }
  if (validated.version !== expected.version) {
    fail(
      `manifest version does not match expected version `
      + `(${validated.version} != ${expected.version})`,
    );
  }
  if (validated.channel !== expected.channel) {
    fail(
      `manifest channel does not match expected channel `
      + `(${validated.channel} != ${expected.channel})`,
    );
  }

  const actualNames = listRegularFiles(directory);
  const releaseNames = validated.releaseAssets.map((asset) => asset.name);
  const expectedNames = scope === "artifact"
    ? [...releaseNames, validated.npmPackage.name, MANIFEST_NAME].sort()
    : [...releaseNames].sort();
  describeSetMismatch(
    scope === "artifact" ? "artifact file" : "release asset",
    expectedNames,
    actualNames,
  );
  if (scope === "artifact") {
    const inDirectoryManifest = readManifest(
      filePath(directory, MANIFEST_NAME, "candidate manifest"),
    );
    if (!isDeepStrictEqual(inDirectoryManifest, manifest)) {
      fail(
        "in-directory candidate manifest does not match the manifest supplied for verification",
      );
    }
  }

  for (const asset of validated.releaseAssets) {
    const actual = sha256File(assertRegularFile(directory, asset.name, "release asset"));
    if (actual !== asset.sha256) {
      fail(
        `SHA-256 mismatch for release asset ${asset.name}: `
        + `expected ${asset.sha256}, observed ${actual}`,
      );
    }
  }

  if (scope === "artifact") {
    const target = assertRegularFile(
      directory,
      validated.npmPackage.name,
      "npm package",
    );
    const actual = npmDigests(target);
    if (actual.sha256 !== validated.npmPackage.sha256) {
      fail(
        `npm package SHA-256 mismatch for ${validated.npmPackage.name}: `
        + `expected ${validated.npmPackage.sha256}, observed ${actual.sha256}`,
      );
    }
    if (actual.integrity !== validated.npmPackage.integrity) {
      fail(
        `npm package integrity mismatch for ${validated.npmPackage.name}: `
        + `expected ${validated.npmPackage.integrity}, observed ${actual.integrity}`,
      );
    }
  }
  return true;
}

function compareReleaseVersions(left, right) {
  const leftMatch = RELEASE_VERSION_PATTERN.exec(left);
  const rightMatch = RELEASE_VERSION_PATTERN.exec(right);
  const leftParts = leftMatch.slice(1, 4).map((part) => BigInt(part));
  const rightParts = rightMatch.slice(1, 4).map((part) => BigInt(part));
  for (let index = 0; index < leftParts.length; index += 1) {
    if (leftParts[index] < rightParts[index]) return -1;
    if (leftParts[index] > rightParts[index]) return 1;
  }
  if (leftMatch[4] === undefined && rightMatch[4] === undefined) return 0;
  const leftBeta = BigInt(leftMatch[4]);
  const rightBeta = BigInt(rightMatch[4]);
  if (leftBeta < rightBeta) return -1;
  if (leftBeta > rightBeta) return 1;
  return 0;
}

function validateObservedDistTags(distTags) {
  if (distTags === undefined) return {};
  assertObject(distTags, "observed.distTags");
  for (const [distTag, channel] of [["latest", "stable"], ["beta", "beta"]]) {
    if (!hasOwn(distTags, distTag)) continue;
    let parsed;
    try {
      parsed = parseReleaseVersion(distTags[distTag]);
    } catch {
      fail(`observed dist-tag ${distTag} must contain a valid ${channel} version`);
    }
    if (parsed.channel !== channel) {
      fail(`observed dist-tag ${distTag} must contain a valid ${channel} version`);
    }
  }
  return distTags;
}

function assertNpmIntegrity(value, label) {
  if (typeof value !== "string" || !value.startsWith("sha512-") || value.length === 7) {
    fail(`${label} must be a non-empty sha512 integrity string`);
  }
}

function evaluateNpmState(target, observed) {
  assertExactKeys(target, ["version", "channel", "integrity"], "target");
  assertObject(observed, "observed");
  if (typeof target.version !== "string") {
    fail("target.version must be a string");
  }
  validateChannel(target.version, target.channel, "target");
  assertNpmIntegrity(target.integrity, "target.integrity");
  if (hasOwn(observed, "versionPresent") && typeof observed.versionPresent !== "boolean") {
    fail("observed.versionPresent must be a boolean");
  }
  if (
    hasOwn(observed, "publishedVersion")
    && observed.publishedVersion !== undefined
    && observed.publishedVersion !== target.version
  ) {
    fail("observed.publishedVersion must equal target.version when provided");
  }

  const distTags = validateObservedDistTags(observed.distTags);
  const distTag = target.channel === "stable" ? "latest" : "beta";
  const hasPublishedIntegrity =
    typeof observed.publishedIntegrity === "string"
    && observed.publishedIntegrity.length > 0;
  const versionPresent =
    observed.versionPresent === true
    || observed.publishedVersion === target.version
    || hasPublishedIntegrity;

  if (observed.versionPresent === false && hasPublishedIntegrity) {
    fail("observed npm state is inconsistent: version is absent but integrity is present");
  }
  if (versionPresent) {
    if (!hasPublishedIntegrity) {
      fail(`npm version ${target.version} is present but published integrity is missing`);
    }
    assertNpmIntegrity(observed.publishedIntegrity, "observed.publishedIntegrity");
    if (observed.publishedIntegrity !== target.integrity) {
      fail(`npm version ${target.version} already exists with different integrity`);
    }
    return { distTag, action: "reuse" };
  }

  if (hasOwn(distTags, distTag)) {
    const comparison = compareReleaseVersions(distTags[distTag], target.version);
    if (comparison > 0) {
      fail(
        `npm dist-tag ${distTag} must not move backwards from `
        + `${distTags[distTag]} to ${target.version}`,
      );
    }
    if (comparison === 0) {
      fail(
        `npm dist-tag ${distTag} already points to target version ${target.version}, `
        + "but the registry reports that version absent",
      );
    }
  }
  return { distTag, action: "publish" };
}

function parseCliOptions(args, allowedOptions) {
  const allowed = new Set(allowedOptions);
  const options = {};
  for (let index = 0; index < args.length; index += 2) {
    const option = args[index];
    const value = args[index + 1];
    if (typeof option !== "string" || !option.startsWith("--") || !allowed.has(option)) {
      fail(`unknown option: ${option === undefined ? "(missing)" : option}`);
    }
    if (value === undefined || value.startsWith("--")) {
      fail(`option ${option} requires a value`);
    }
    if (hasOwn(options, option)) {
      fail(`option ${option} must not be repeated`);
    }
    options[option] = value;
  }
  for (const option of allowedOptions) {
    if (!hasOwn(options, option)) fail(`required option is missing: ${option}`);
  }
  return options;
}

function readManifest(manifestPath) {
  let stat;
  try {
    stat = fs.lstatSync(manifestPath);
  } catch (error) {
    fail(`manifest could not be inspected: ${error.message}`);
  }
  if (stat.isSymbolicLink() || !stat.isFile()) {
    fail("manifest must be a regular file and must not be a symlink");
  }
  try {
    return JSON.parse(fs.readFileSync(manifestPath, "utf8"));
  } catch (error) {
    fail(`manifest must contain valid JSON: ${error.message}`);
  }
}

function writeSuccess(value) {
  process.stdout.write(`${JSON.stringify(value)}\n`);
}

function writeFailure(error) {
  process.stderr.write(`${JSON.stringify({
    ok: false,
    error: {
      type: "release_candidate",
      message: error.message,
    },
  })}\n`);
  process.exitCode = 1;
}

function main() {
  try {
    const [command, ...args] = process.argv.slice(2);
    if (command !== "create" && command !== "verify") {
      fail("command must be create or verify");
    }
    if (command === "create") {
      const options = parseCliOptions(args, [
        "--directory",
        "--manifest",
        "--source-sha",
        "--version",
        "--channel",
        "--npm-package",
      ]);
      const directory = path.resolve(options["--directory"]);
      const manifestPath = path.resolve(options["--manifest"]);
      const expectedManifestPath = path.join(directory, MANIFEST_NAME);
      if (manifestPath !== expectedManifestPath) {
        fail(`--manifest must be ${expectedManifestPath} for create`);
      }
      const manifest = createCandidateManifest(directory, {
        sourceSha: options["--source-sha"],
        version: options["--version"],
        channel: options["--channel"],
        npmPackage: options["--npm-package"],
      });
      try {
        fs.writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
      } catch (error) {
        fail(`candidate manifest could not be written: ${error.message}`);
      }
      writeSuccess({ ok: true, manifest });
      return;
    }

    const options = parseCliOptions(args, [
      "--directory",
      "--manifest",
      "--scope",
      "--source-sha",
      "--version",
      "--channel",
    ]);
    const directory = path.resolve(options["--directory"]);
    const manifestPath = path.resolve(options["--manifest"]);
    const scope = options["--scope"];
    if (scope === "artifact" && manifestPath !== path.join(directory, MANIFEST_NAME)) {
      fail(
        `--manifest must be ${path.join(directory, MANIFEST_NAME)} `
        + "inside --directory for artifact scope",
      );
    }
    const manifest = readManifest(manifestPath);
    verifyCandidateManifest(
      directory,
      manifest,
      {
        sourceSha: options["--source-sha"],
        version: options["--version"],
        channel: options["--channel"],
      },
      scope,
    );
    writeSuccess({
      ok: true,
      scope,
      version: manifest.version,
      channel: manifest.channel,
      sourceSha: normalizeSourceSha(manifest.sourceSha),
    });
  } catch (error) {
    writeFailure(error);
  }
}

module.exports = {
  createCandidateManifest,
  verifyCandidateManifest,
  evaluateNpmState,
  parseReleaseVersion,
};

if (require.main === module) main();
