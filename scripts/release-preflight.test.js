// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const assert = require("node:assert/strict");
const { describe, it } = require("node:test");

const { validateReleasePreflight } = require("./release-preflight");

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
        },
      },
    );
  });

  it("rejects non-stable or inconsistent package metadata", () => {
    const prerelease = metadata("1.2.3-beta.1");
    const topLevelMismatch = metadata();
    topLevelMismatch.packageLockJson.version = "1.2.4";
    const rootMismatch = metadata();
    rootMismatch.packageLockJson.packages[""].version = "1.2.4";

    for (const { packageJson, packageLockJson } of [
      prerelease,
      topLevelMismatch,
      rootMismatch,
    ]) {
      assertRejected(validateReleasePreflight(packageJson, packageLockJson));
    }
  });

  it("rejects an invalid or mismatched release tag", () => {
    const { packageJson, packageLockJson } = metadata();

    for (const tag of ["1.2.3", "v1.2.3-beta.1", "v1.2.4"]) {
      assertRejected(validateReleasePreflight(packageJson, packageLockJson, tag));
    }
  });
});
