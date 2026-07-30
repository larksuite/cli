// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const RELEASE_VERSION_PATTERN = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-beta\.(0|[1-9][0-9]*))?$/;

function parseReleaseVersion(version) {
  const match = typeof version === "string" ? RELEASE_VERSION_PATTERN.exec(version) : null;
  if (!match) return null;
  return {
    major: Number(match[1]),
    minor: Number(match[2]),
    patch: Number(match[3]),
    beta: match[4] === undefined ? null : Number(match[4]),
  };
}

function compareReleaseVersions(left, right) {
  const a = parseReleaseVersion(left);
  const b = parseReleaseVersion(right);
  if (!a || !b) throw new Error("Both versions must be stable or beta release versions.");

  for (const field of ["major", "minor", "patch"]) {
    if (a[field] !== b[field]) return a[field] < b[field] ? -1 : 1;
  }
  if (a.beta === b.beta) return 0;
  if (a.beta === null) return 1;
  if (b.beta === null) return -1;
  return a.beta < b.beta ? -1 : 1;
}

function isNpmVersionMissing(errorOutput) {
  return typeof errorOutput === "string" && /(?:^|\s)E404(?:\s|$)/m.test(errorOutput);
}

function decideNpmPublish({ version, versionExists, integrityMatches, channelVersion }) {
  if (!parseReleaseVersion(version)) {
    throw new Error("version must be a stable or beta release version.");
  }
  if (channelVersion !== null && !parseReleaseVersion(channelVersion)) {
    throw new Error("channelVersion must be null or a stable or beta release version.");
  }

  if (!versionExists) {
    if (channelVersion !== null && compareReleaseVersions(channelVersion, version) >= 0) {
      return { action: "reject", reason: "The npm channel already points to this or a newer version." };
    }
    return { action: "publish" };
  }

  if (!integrityMatches) {
    return { action: "reject", reason: "The existing npm version has different package integrity." };
  }
  if (channelVersion === null || compareReleaseVersions(channelVersion, version) < 0) {
    return { action: "advance-tag" };
  }
  return { action: "verify" };
}

module.exports = { compareReleaseVersions, decideNpmPublish, isNpmVersionMissing };
