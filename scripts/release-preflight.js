#!/usr/bin/env node
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const fs = require("node:fs");
const path = require("node:path");

const STABLE_VERSION_PATTERN = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/;

function isStableVersion(value) {
  return typeof value === "string" && STABLE_VERSION_PATTERN.test(value);
}

function releaseError(message, observed, hint) {
  return { ok: false, error: { type: "release_preflight", message, observed, hint } };
}

function validateReleasePreflight(packageJson, packageLockJson, tag) {
  const packageVersion = packageJson?.version;
  const lockVersion = packageLockJson?.version;
  const lockRootVersion = packageLockJson?.packages?.[""]?.version;
  const observed = {
    packageVersion: packageVersion ?? null,
    lockVersion: lockVersion ?? null,
    lockRootVersion: lockRootVersion ?? null,
    tagVersion: null,
  };

  for (const [field, value] of [
    ["package.json.version", packageVersion],
    ["package-lock.json.version", lockVersion],
    ['package-lock.json.packages[""].version', lockRootVersion],
  ]) {
    if (!isStableVersion(value)) {
      return releaseError(
        `${field} must be a stable release version in X.Y.Z form`,
        observed,
        "Use the same stable X.Y.Z version in all package fields; prerelease and build metadata are not allowed for production releases.",
      );
    }
  }

  if (packageVersion !== lockVersion || packageVersion !== lockRootVersion) {
    return releaseError(
      "Package version fields do not match",
      observed,
      "Synchronize package.json.version and both package-lock.json version fields.",
    );
  }

  if (tag === undefined) {
    return { ok: true, data: observed };
  }
  if (typeof tag !== "string" || !tag.startsWith("v") || !isStableVersion(tag.slice(1))) {
    return releaseError(
      "--tag must use the stable release form vX.Y.Z",
      { ...observed, tag },
      `Use --tag v${packageVersion}; prerelease and build metadata are not allowed for production releases.`,
    );
  }

  const tagVersion = tag.slice(1);
  if (tagVersion !== packageVersion) {
    return releaseError(
      "Tag version does not match the package version",
      { ...observed, tagVersion, tag },
      `Use --tag v${packageVersion}.`,
    );
  }
  return { ok: true, data: { ...observed, tagVersion } };
}

function writeResult(result) {
  (result.ok ? process.stdout : process.stderr).write(`${JSON.stringify(result)}\n`);
  if (!result.ok) process.exitCode = 1;
}

function main() {
  const args = process.argv.slice(2);
  let tag;
  if (args.length === 2 && args[0] === "--tag") {
    tag = args[1];
  } else if (args.length !== 0) {
    writeResult(releaseError(
      "Expected no arguments or --tag vX.Y.Z",
      { arguments: args },
      "Run release:check without arguments or pass exactly one --tag value.",
    ));
    return;
  }

  const repoRoot = path.resolve(__dirname, "..");
  try {
    const packageJson = JSON.parse(fs.readFileSync(path.join(repoRoot, "package.json"), "utf8"));
    const packageLockJson = JSON.parse(fs.readFileSync(path.join(repoRoot, "package-lock.json"), "utf8"));
    writeResult(validateReleasePreflight(packageJson, packageLockJson, tag));
  } catch (error) {
    writeResult(releaseError(
      "Could not read release package metadata",
      { reason: error.message },
      "Ensure package.json and package-lock.json exist and contain valid JSON.",
    ));
  }
}

module.exports = { validateReleasePreflight };

if (require.main === module) main();
