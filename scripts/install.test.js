// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("fs");
const path = require("path");
const os = require("os");

const crypto = require("crypto");

const { getExpectedChecksum, verifyChecksum, assertAllowedHost, resolveMirrorUrls } = require("./install.js");

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

  it("skips malformed lines and still finds valid entry", () => {
    const dir = makeTmpChecksums(
      "garbage line without separator\n" +
      "\n" +
      "abc123  lark-cli-1.0.0-darwin-arm64.tar.gz\n" +
      "also garbage\n"
    );
    const hash = getExpectedChecksum(
      "lark-cli-1.0.0-darwin-arm64.tar.gz",
      dir
    );
    assert.equal(hash, "abc123");
  });

  it("skips tab-separated lines (only double-space is valid)", () => {
    const dir = makeTmpChecksums(
      "wrong\tlark-cli-1.0.0-darwin-arm64.tar.gz\n" +
      "correct  lark-cli-1.0.0-darwin-arm64.tar.gz\n"
    );
    const hash = getExpectedChecksum(
      "lark-cli-1.0.0-darwin-arm64.tar.gz",
      dir
    );
    assert.equal(hash, "correct");
  });
});

describe("verifyChecksum", () => {
  function makeTmpFile(content) {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "checksum-test-"));
    const filePath = path.join(dir, "archive.tar.gz");
    fs.writeFileSync(filePath, content);
    return filePath;
  }

  function sha256(content) {
    return crypto.createHash("sha256").update(content).digest("hex");
  }

  it("returns normally when hash matches", () => {
    const content = "binary content here";
    const filePath = makeTmpFile(content);
    const hash = sha256(content);
    // Should not throw
    verifyChecksum(filePath, hash);
  });

  it("matches case-insensitively", () => {
    const content = "case test";
    const filePath = makeTmpFile(content);
    const hash = sha256(content).toUpperCase();
    // Should not throw
    verifyChecksum(filePath, hash);
  });

  it("throws [SECURITY]-prefixed Error on mismatch", () => {
    const filePath = makeTmpFile("real content");
    assert.throws(
      () => verifyChecksum(filePath, "0000000000000000000000000000000000000000000000000000000000000000"),
      (err) => {
        assert.match(err.message, /^\[SECURITY\]/);
        assert.match(err.message, /Checksum mismatch/);
        return true;
      }
    );
  });
});

describe("assertAllowedHost", () => {
  it("accepts github.com", () => {
    assertAllowedHost("https://github.com/larksuite/cli/releases/download/v1.0.0/archive.tar.gz");
  });

  it("accepts objects.githubusercontent.com", () => {
    assertAllowedHost("https://objects.githubusercontent.com/some/path");
  });

  it("accepts registry.npmmirror.com", () => {
    assertAllowedHost("https://registry.npmmirror.com/-/binary/lark-cli/v1.0.0/archive.tar.gz");
  });

  it("rejects unknown host", () => {
    assert.throws(
      () => assertAllowedHost("https://evil.example.com/payload"),
      { message: /Download host not allowed: evil\.example\.com/ }
    );
  });

  it("normalizes hostname to lowercase", () => {
    // URL constructor lowercases hostnames per spec
    assertAllowedHost("https://GitHub.COM/larksuite/cli/releases/download/v1.0.0/a.tar.gz");
  });

  it("ignores port when matching hostname", () => {
    // URL.hostname does not include port
    assertAllowedHost("https://github.com:443/larksuite/cli/releases/download/v1.0.0/a.tar.gz");
  });

  it("throws on invalid URL", () => {
    assert.throws(
      () => assertAllowedHost("not-a-url"),
      TypeError
    );
  });
});

describe("resolveMirrorUrls", () => {
  const ARCHIVE = "lark-cli-1.0.0-linux-amd64.tar.gz";
  const VERSION = "1.0.0";
  const DEFAULT = "https://registry.npmmirror.com/-/binary/lark-cli/v1.0.0/lark-cli-1.0.0-linux-amd64.tar.gz";

  it("returns only the default mirror when no env vars are set", () => {
    assert.deepEqual(resolveMirrorUrls({}, ARCHIVE, VERSION), [DEFAULT]);
  });

  it("does not derive from the default npmjs registry", () => {
    // The public npmjs registry doesn't host /-/binary/<pkg>/..., so we must
    // not point downloads at it.
    assert.deepEqual(
      resolveMirrorUrls(
        { npm_config_registry: "https://registry.npmjs.org/" },
        ARCHIVE,
        VERSION
      ),
      [DEFAULT]
    );
  });

  it("derives from non-default npm_config_registry AND keeps default as fallback", () => {
    // Critical: a corporate npm proxy (Verdaccio/Artifactory/Nexus) often
    // doesn't actually serve /-/binary/<pkg>/..., so we must keep the
    // public npmmirror as a final fallback or installs regress vs. the
    // pre-PR "GitHub → npmmirror" behavior.
    assert.deepEqual(
      resolveMirrorUrls(
        { npm_config_registry: "https://corp.example.com/repository/npm-public/" },
        ARCHIVE,
        VERSION
      ),
      [
        "https://corp.example.com/repository/npm-public/-/binary/lark-cli/v1.0.0/lark-cli-1.0.0-linux-amd64.tar.gz",
        DEFAULT,
      ]
    );
  });

  it("derived URL appears before the default in the chain", () => {
    const urls = resolveMirrorUrls(
      { npm_config_registry: "https://corp.example.com/" },
      ARCHIVE,
      VERSION
    );
    assert.equal(urls.length, 2);
    assert.match(urls[0], /^https:\/\/corp\.example\.com\//);
    assert.equal(urls[1], DEFAULT);
  });

  it("does not duplicate the default if the registry already points at it", () => {
    // If npm_config_registry happens to be the public npmmirror, we still
    // want a single entry, not two identical ones.
    assert.deepEqual(
      resolveMirrorUrls(
        { npm_config_registry: "https://registry.npmmirror.com/" },
        ARCHIVE,
        VERSION
      ),
      [DEFAULT]
    );
  });

  it("strips trailing slashes from the registry URL", () => {
    assert.deepEqual(
      resolveMirrorUrls(
        { npm_config_registry: "https://corp.example.com///" },
        ARCHIVE,
        VERSION
      ),
      [
        "https://corp.example.com/-/binary/lark-cli/v1.0.0/lark-cli-1.0.0-linux-amd64.tar.gz",
        DEFAULT,
      ]
    );
  });

  it("ignores empty/whitespace npm_config_registry", () => {
    assert.deepEqual(
      resolveMirrorUrls(
        { npm_config_registry: "" },
        ARCHIVE,
        VERSION
      ),
      [DEFAULT]
    );
  });

  it("silently falls back when npm_config_registry is non-https", () => {
    // Implicit feature: don't break installs whose npm registry is plain http.
    // The user didn't opt into binary-mirror behavior, so just use the default.
    assert.deepEqual(
      resolveMirrorUrls(
        { npm_config_registry: "http://internal.example.com/" },
        ARCHIVE,
        VERSION
      ),
      [DEFAULT]
    );
  });

  it("silently falls back when npm_config_registry is file://", () => {
    assert.deepEqual(
      resolveMirrorUrls(
        { npm_config_registry: "file:///tmp" },
        ARCHIVE,
        VERSION
      ),
      [DEFAULT]
    );
  });
});
