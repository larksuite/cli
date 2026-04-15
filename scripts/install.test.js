// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { test } = require("node:test");
const assert = require("node:assert");
const fs = require("fs");
const os = require("os");
const path = require("path");
const crypto = require("crypto");
const {
  verifyChecksum,
  getExpectedChecksum,
  ChecksumError,
  PackageIntegrityError,
} = require("./install.js");

function mktmpdir() {
  return fs.mkdtempSync(path.join(os.tmpdir(), "install-test-"));
}

test("verifyChecksum: correct hash resolves", async () => {
  const dir = mktmpdir();
  try {
    const filePath = path.join(dir, "data.bin");
    const bytes = Buffer.from("hello world");
    fs.writeFileSync(filePath, bytes);
    const correctHash = crypto.createHash("sha256").update(bytes).digest("hex");

    await verifyChecksum(filePath, correctHash);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test("verifyChecksum: mismatched hash throws ChecksumError", async () => {
  const dir = mktmpdir();
  try {
    const filePath = path.join(dir, "data.bin");
    fs.writeFileSync(filePath, "hello world");
    const wrongHash = "0".repeat(64);

    await assert.rejects(
      () => verifyChecksum(filePath, wrongHash),
      (err) => err instanceof ChecksumError,
    );
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test("getExpectedChecksum: returns hash for listed archive", () => {
  const dir = mktmpdir();
  try {
    const checksumsPath = path.join(dir, "checksums.txt");
    const knownHash = "a".repeat(64);
    fs.writeFileSync(
      checksumsPath,
      `${knownHash}  lark-cli-1.0.0-linux-amd64.tar.gz\n`
    );

    const result = getExpectedChecksum(
      "lark-cli-1.0.0-linux-amd64.tar.gz",
      checksumsPath,
    );
    assert.strictEqual(result, knownHash);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test("getExpectedChecksum: throws PackageIntegrityError (not ChecksumError) when checksums.txt file is absent", () => {
  const dir = mktmpdir();
  try {
    const missingPath = path.join(dir, "does-not-exist.txt");

    assert.throws(
      () => getExpectedChecksum("lark-cli-1.0.0-linux-amd64.tar.gz", missingPath),
      (err) =>
        err instanceof PackageIntegrityError &&
        !(err instanceof ChecksumError),
    );
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});

test("getExpectedChecksum: throws ChecksumError when entry missing", () => {
  const dir = mktmpdir();
  try {
    const checksumsPath = path.join(dir, "checksums.txt");
    fs.writeFileSync(
      checksumsPath,
      `${"a".repeat(64)}  some-other-archive.tar.gz\n`
    );

    assert.throws(
      () => getExpectedChecksum("nonexistent-archive.tar.gz", checksumsPath),
      (err) => err instanceof ChecksumError,
    );
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
});
