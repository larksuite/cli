// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("fs");
const path = require("path");
const os = require("os");

const { getExpectedChecksum, verifyChecksum } = require("./install.js");

describe("getExpectedChecksum", () => {
  function makeTmpChecksums(content) {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "checksum-test-"));
    fs.writeFileSync(path.join(dir, "checksums.txt"), content, "utf8");
    return dir;
  }

  it("returns correct hash from standard-format checksums.txt", () => {
    const dir = makeTmpChecksums(
      "abc123def456  lark-cli-1.0.0-darwin-arm64.tar.gz\n"
    );
    const hash = getExpectedChecksum(
      "lark-cli-1.0.0-darwin-arm64.tar.gz",
      dir
    );
    assert.equal(hash, "abc123def456");
  });

  it("returns correct entry when multiple entries exist", () => {
    const dir = makeTmpChecksums(
      "aaaa  lark-cli-1.0.0-linux-amd64.tar.gz\n" +
      "bbbb  lark-cli-1.0.0-darwin-arm64.tar.gz\n" +
      "cccc  lark-cli-1.0.0-windows-amd64.zip\n"
    );
    const hash = getExpectedChecksum(
      "lark-cli-1.0.0-darwin-arm64.tar.gz",
      dir
    );
    assert.equal(hash, "bbbb");
  });

  it("throws Error when archiveName is not found", () => {
    const dir = makeTmpChecksums(
      "aaaa  lark-cli-1.0.0-linux-amd64.tar.gz\n"
    );
    assert.throws(
      () => getExpectedChecksum("nonexistent.tar.gz", dir),
      { message: /Checksum entry not found for nonexistent\.tar\.gz/ }
    );
  });

  it("returns null when checksums.txt does not exist", () => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "checksum-test-"));
    // No checksums.txt in dir
    const result = getExpectedChecksum("anything.tar.gz", dir);
    assert.equal(result, null);
  });
});
