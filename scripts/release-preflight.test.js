// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const assert = require("node:assert/strict");
const { describe, it } = require("node:test");

const { validateReleasePreflight, validateReleaseSourcePolicy } = require("./release-preflight");

function metadata(version = "1.2.3") {
  return {
    packageJson: { version },
    packageLockJson: {
      version,
      packages: { "": { version } },
    },
  };
}

function assertRejected(result) {
  assert.equal(result.ok, false);
  assert.equal(result.error.type, "release_preflight");
  assert.equal(typeof result.error.message, "string");
}

describe("validateReleasePreflight", () => {
  it("accepts matching stable package, lock, and tag versions", () => {
    const { packageJson, packageLockJson } = metadata();

    assert.deepEqual(
      validateReleasePreflight(packageJson, packageLockJson, "v1.2.3"),
      {
        ok: true,
        data: {
          packageVersion: "1.2.3",
          lockVersion: "1.2.3",
          lockRootVersion: "1.2.3",
          tagVersion: "1.2.3",
          releaseChannel: "stable",
        },
      },
    );
  });

  it("accepts matching beta package, lock, and tag versions", () => {
    const { packageJson, packageLockJson } = metadata("1.2.3-beta.4");

    assert.deepEqual(
      validateReleasePreflight(packageJson, packageLockJson, "v1.2.3-beta.4"),
      {
        ok: true,
        data: {
          packageVersion: "1.2.3-beta.4",
          lockVersion: "1.2.3-beta.4",
          lockRootVersion: "1.2.3-beta.4",
          tagVersion: "1.2.3-beta.4",
          releaseChannel: "beta",
        },
      },
    );
  });

  it("derives the release channel when no tag is provided", () => {
    const { packageJson, packageLockJson } = metadata("1.2.3-beta.0");

    assert.deepEqual(
      validateReleasePreflight(packageJson, packageLockJson),
      {
        ok: true,
        data: {
          packageVersion: "1.2.3-beta.0",
          lockVersion: "1.2.3-beta.0",
          lockRootVersion: "1.2.3-beta.0",
          tagVersion: null,
          releaseChannel: "beta",
        },
      },
    );
  });

  it("rejects unsupported or invalid package versions with an actionable hint", () => {
    for (const version of [
      "1.2.3-alpha.1",
      "1.2.3-rc.1",
      "1.2.3-beta",
      "1.2.3-beta.01",
      "1.2.3+build.1",
      "1.2.3-beta.1+build.1",
      "01.2.3",
      "1.02.3",
      "1.2.03",
    ]) {
      const { packageJson, packageLockJson } = metadata(version);
      const result = validateReleasePreflight(packageJson, packageLockJson);

      assertRejected(result);
      assert.match(result.error.hint, /stable X\.Y\.Z or beta X\.Y\.Z-beta\.N/i);
    }
  });

  it("rejects inconsistent package metadata", () => {
    const topLevelMismatch = metadata();
    topLevelMismatch.packageLockJson.version = "1.2.4";
    const rootMismatch = metadata();
    rootMismatch.packageLockJson.packages[""].version = "1.2.4";
    const channelMismatch = metadata("1.2.3-beta.1");
    channelMismatch.packageLockJson.version = "1.2.3";

    for (const { packageJson, packageLockJson } of [
      topLevelMismatch,
      rootMismatch,
      channelMismatch,
    ]) {
      assertRejected(validateReleasePreflight(packageJson, packageLockJson));
    }
  });

  it("rejects an invalid or mismatched release tag", () => {
    const { packageJson, packageLockJson } = metadata();

    for (const tag of [
      "1.2.3",
      "v1.2.3-alpha.1",
      "v1.2.3-beta.01",
      "v1.2.3+build.1",
      "v1.2.3-beta.1",
      "v1.2.4",
    ]) {
      assertRejected(validateReleasePreflight(packageJson, packageLockJson, tag));
    }
  });

  it("rejects a beta tag that does not match beta package metadata", () => {
    const { packageJson, packageLockJson } = metadata("1.2.3-beta.2");

    for (const tag of ["v1.2.3-beta.1", "v1.2.3"]) {
      assertRejected(validateReleasePreflight(packageJson, packageLockJson, tag));
    }
  });
});

describe("validateReleaseSourcePolicy", () => {
  const mainSha = "a".repeat(40);
  const betaSha = "b".repeat(40);

  it("accepts a stable release commit contained in main", () => {
    assert.deepEqual(
      validateReleaseSourcePolicy("stable", betaSha, mainSha, true),
      { ok: true, data: { warning: null } },
    );
  });

  it("rejects a stable release commit outside main", () => {
    const result = validateReleaseSourcePolicy("stable", betaSha, mainSha, false);

    assertRejected(result);
    assert.match(result.error.message, /contained in origin\/main/);
  });

  it("accepts a beta release from a separate branch without a warning", () => {
    assert.deepEqual(
      validateReleaseSourcePolicy("beta", betaSha, mainSha, false),
      { ok: true, data: { warning: null } },
    );
  });

  it("warns but accepts a beta release tag pointing to the current main head", () => {
    const result = validateReleaseSourcePolicy("beta", mainSha, mainSha, true);

    assert.equal(result.ok, true);
    assert.match(result.data.warning, /unintended beta version was merged into main/);
  });
});
