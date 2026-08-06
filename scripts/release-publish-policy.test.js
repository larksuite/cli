// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const assert = require("node:assert/strict");
const { describe, it } = require("node:test");

const { compareReleaseVersions, decideNpmPublish, isNpmVersionMissing } = require("./release-publish-policy");

describe("compareReleaseVersions", () => {
  it("orders beta versions before their stable release", () => {
    assert.equal(compareReleaseVersions("1.2.3-beta.4", "1.2.3"), -1);
    assert.equal(compareReleaseVersions("1.2.4-beta.0", "1.2.3"), 1);
  });
});

describe("isNpmVersionMissing", () => {
  it("accepts only npm's explicit not-found error", () => {
    assert.equal(isNpmVersionMissing("npm error code E404\nnpm error 404 Not Found"), true);
    assert.equal(isNpmVersionMissing("npm error code ETIMEDOUT"), false);
    assert.equal(isNpmVersionMissing(""), false);
  });
});

describe("decideNpmPublish", () => {
  it("publishes a missing version only when it cannot move a channel backwards", () => {
    assert.deepEqual(
      decideNpmPublish({ version: "1.2.3", versionExists: false, integrityMatches: false, channelVersion: "1.2.2" }),
      { action: "publish" },
    );
    assert.deepEqual(
      decideNpmPublish({ version: "1.2.3", versionExists: false, integrityMatches: false, channelVersion: "1.2.4" }),
      { action: "reject", reason: "The npm channel already points to this or a newer version." },
    );
  });

  it("keeps an already advanced channel and advances only an older channel", () => {
    assert.deepEqual(
      decideNpmPublish({ version: "1.2.3-beta.4", versionExists: true, integrityMatches: true, channelVersion: "1.2.3-beta.5" }),
      { action: "verify" },
    );
    assert.deepEqual(
      decideNpmPublish({ version: "1.2.3-beta.4", versionExists: true, integrityMatches: true, channelVersion: "1.2.3-beta.3" }),
      { action: "advance-tag" },
    );
  });

  it("rejects an existing version with different integrity", () => {
    assert.deepEqual(
      decideNpmPublish({ version: "1.2.3", versionExists: true, integrityMatches: false, channelVersion: "1.2.3" }),
      { action: "reject", reason: "The existing npm version has different package integrity." },
    );
  });
});
