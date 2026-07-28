// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const assert = require("node:assert/strict");
const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const { afterEach, describe, it } = require("node:test");

const {
  createCandidateManifest,
  evaluateNpmState,
  parseReleaseVersion,
  verifyCandidateManifest,
} = require("./release-candidate");

const SOURCE_SHA = "ABCDEF0123456789ABCDEF0123456789ABCDEF01";
const tempDirectories = [];

function tempDirectory() {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "release-candidate-"));
  tempDirectories.push(directory);
  return directory;
}

function writeCandidate(version = "1.2.3", channel = "stable") {
  const directory = tempDirectory();
  const npmPackage = `larksuite-cli-${version}.tgz`;
  fs.writeFileSync(path.join(directory, "z-checksums.txt"), "checksums\n");
  fs.writeFileSync(path.join(directory, `lark-cli-${version}-linux-amd64.tar.gz`), "linux\n");
  fs.writeFileSync(path.join(directory, npmPackage), "npm package\n");
  return {
    directory,
    metadata: { sourceSha: SOURCE_SHA, version, channel, npmPackage },
  };
}

function sha256(value) {
  return crypto.createHash("sha256").update(value).digest("hex");
}

function sha512Integrity(value) {
  return `sha512-${crypto.createHash("sha512").update(value).digest("base64")}`;
}

function writeManifest(directory, manifest) {
  fs.writeFileSync(
    path.join(directory, "candidate-manifest.json"),
    `${JSON.stringify(manifest, null, 2)}\n`,
  );
}

function clone(value) {
  return JSON.parse(JSON.stringify(value));
}

function copyReleaseAssets(sourceDirectory, manifest) {
  const releaseDirectory = tempDirectory();
  for (const asset of manifest.releaseAssets) {
    fs.copyFileSync(
      path.join(sourceDirectory, asset.name),
      path.join(releaseDirectory, asset.name),
    );
  }
  return releaseDirectory;
}

afterEach(() => {
  while (tempDirectories.length > 0) {
    fs.rmSync(tempDirectories.pop(), { recursive: true, force: true });
  }
});

describe("parseReleaseVersion", () => {
  it("accepts exact stable and beta versions", () => {
    assert.deepEqual(parseReleaseVersion("1.2.3"), {
      version: "1.2.3",
      channel: "stable",
      major: 1,
      minor: 2,
      patch: 3,
      beta: null,
    });
    assert.deepEqual(parseReleaseVersion("1.2.3-beta.4"), {
      version: "1.2.3-beta.4",
      channel: "beta",
      major: 1,
      minor: 2,
      patch: 3,
      beta: 4,
    });
  });

  it("rejects unsupported labels, build metadata, and leading zeros", () => {
    for (const version of [
      "1.2.3-alpha.1",
      "1.2.3-rc.1",
      "1.2.3-beta",
      "1.2.3-beta.01",
      "1.2.3+build.1",
      "01.2.3",
      "1.02.3",
      "1.2.03",
    ]) {
      assert.throws(() => parseReleaseVersion(version), /Stable X\.Y\.Z or Beta X\.Y\.Z-beta\.N/);
    }
  });
});

describe("candidate manifest", () => {
  it("creates a normalized, sorted stable manifest with exact digests", () => {
    const { directory, metadata } = writeCandidate();

    const manifest = createCandidateManifest(directory, metadata);

    assert.deepEqual(manifest, {
      schemaVersion: 1,
      sourceSha: SOURCE_SHA.toLowerCase(),
      version: "1.2.3",
      channel: "stable",
      releaseAssets: [
        {
          name: "lark-cli-1.2.3-linux-amd64.tar.gz",
          sha256: sha256("linux\n"),
        },
        {
          name: "z-checksums.txt",
          sha256: sha256("checksums\n"),
        },
      ],
      npmPackage: {
        name: "larksuite-cli-1.2.3.tgz",
        sha256: sha256("npm package\n"),
        integrity: sha512Integrity("npm package\n"),
      },
    });
  });

  it("creates and verifies a beta artifact candidate", () => {
    const candidate = writeCandidate("2.0.0-beta.7", "beta");
    const manifest = createCandidateManifest(candidate.directory, candidate.metadata);
    writeManifest(candidate.directory, manifest);

    assert.equal(manifest.channel, "beta");
    assert.equal(
      verifyCandidateManifest(
        candidate.directory,
        manifest,
        candidate.metadata,
        "artifact",
      ),
      true,
    );
  });

  it("verifies a release directory containing only release assets", () => {
    const candidate = writeCandidate();
    const manifest = createCandidateManifest(candidate.directory, candidate.metadata);
    const releaseDirectory = copyReleaseAssets(candidate.directory, manifest);

    assert.equal(
      verifyCandidateManifest(releaseDirectory, manifest, candidate.metadata, "release"),
      true,
    );
  });

  it("rejects a modified candidate file", () => {
    const candidate = writeCandidate();
    const manifest = createCandidateManifest(candidate.directory, candidate.metadata);
    writeManifest(candidate.directory, manifest);
    fs.appendFileSync(
      path.join(candidate.directory, manifest.releaseAssets[0].name),
      "tampered",
    );

    assert.throws(
      () => verifyCandidateManifest(
        candidate.directory,
        manifest,
        candidate.metadata,
        "artifact",
      ),
      /SHA-256 mismatch/,
    );
  });

  it("rejects modified npm package content and integrity", () => {
    const candidate = writeCandidate();
    const manifest = createCandidateManifest(candidate.directory, candidate.metadata);
    writeManifest(candidate.directory, manifest);
    fs.appendFileSync(path.join(candidate.directory, manifest.npmPackage.name), "tampered");

    assert.throws(
      () => verifyCandidateManifest(
        candidate.directory,
        manifest,
        candidate.metadata,
        "artifact",
      ),
      /npm package SHA-256 mismatch/,
    );

    fs.writeFileSync(
      path.join(candidate.directory, manifest.npmPackage.name),
      "npm package\n",
    );
    manifest.npmPackage.integrity = sha512Integrity("different npm package");
    assert.throws(
      () => verifyCandidateManifest(
        candidate.directory,
        manifest,
        candidate.metadata,
        "artifact",
      ),
      /npm package integrity mismatch/,
    );
  });

  it("rejects missing and unexpected release assets", () => {
    const candidate = writeCandidate();
    const manifest = createCandidateManifest(candidate.directory, candidate.metadata);
    const releaseDirectory = copyReleaseAssets(candidate.directory, manifest);
    fs.rmSync(path.join(releaseDirectory, manifest.releaseAssets[0].name));

    assert.throws(
      () => verifyCandidateManifest(releaseDirectory, manifest, candidate.metadata, "release"),
      /release asset set does not match.*missing:/,
    );

    fs.copyFileSync(
      path.join(candidate.directory, manifest.releaseAssets[0].name),
      path.join(releaseDirectory, manifest.releaseAssets[0].name),
    );
    fs.writeFileSync(path.join(releaseDirectory, "unexpected.zip"), "unexpected");
    assert.throws(
      () => verifyCandidateManifest(releaseDirectory, manifest, candidate.metadata, "release"),
      /release asset set does not match.*unexpected:/,
    );
  });

  it("rejects symlinks and unsafe designated package names", () => {
    const candidate = writeCandidate();
    fs.symlinkSync(
      path.join(candidate.directory, candidate.metadata.npmPackage),
      path.join(candidate.directory, "linked.tgz"),
    );
    assert.throws(
      () => createCandidateManifest(candidate.directory, candidate.metadata),
      /linked\.tgz.*regular file/,
    );

    fs.rmSync(path.join(candidate.directory, "linked.tgz"));
    for (const npmPackage of ["../package.tgz", "nested/package.tgz", "nested\\package.tgz", ".", ".."]) {
      assert.throws(
        () => createCandidateManifest(
          candidate.directory,
          { ...candidate.metadata, npmPackage },
        ),
        /safe basename/,
      );
    }
  });

  it("rejects path traversal and duplicate manifest entries", () => {
    const candidate = writeCandidate();
    const manifest = createCandidateManifest(candidate.directory, candidate.metadata);
    const releaseDirectory = copyReleaseAssets(candidate.directory, manifest);

    const traversing = clone(manifest);
    traversing.releaseAssets[0].name = "../outside";
    assert.throws(
      () => verifyCandidateManifest(releaseDirectory, traversing, candidate.metadata, "release"),
      /safe basename/,
    );

    const duplicate = clone(manifest);
    duplicate.releaseAssets.push({ ...duplicate.releaseAssets[0] });
    assert.throws(
      () => verifyCandidateManifest(releaseDirectory, duplicate, candidate.metadata, "release"),
      /duplicate release asset/,
    );

    const collision = clone(manifest);
    collision.npmPackage.name = collision.releaseAssets[0].name;
    assert.throws(
      () => verifyCandidateManifest(releaseDirectory, collision, candidate.metadata, "release"),
      /duplicates npm package/,
    );
  });

  it("rejects metadata, schema, and channel mismatches", () => {
    const candidate = writeCandidate();
    const manifest = createCandidateManifest(candidate.directory, candidate.metadata);
    const releaseDirectory = copyReleaseAssets(candidate.directory, manifest);

    assert.throws(
      () => verifyCandidateManifest(
        releaseDirectory,
        manifest,
        { ...candidate.metadata, sourceSha: "0".repeat(40) },
        "release",
      ),
      /sourceSha does not match/,
    );
    assert.throws(
      () => verifyCandidateManifest(
        releaseDirectory,
        manifest,
        { ...candidate.metadata, version: "1.2.4" },
        "release",
      ),
      /version does not match/,
    );
    assert.throws(
      () => createCandidateManifest(
        candidate.directory,
        { ...candidate.metadata, channel: "beta" },
      ),
      /version requires channel stable/,
    );

    assert.throws(
      () => verifyCandidateManifest(
        releaseDirectory,
        { ...manifest, schemaVersion: 2 },
        candidate.metadata,
        "release",
      ),
      /schemaVersion must be 1/,
    );
    assert.throws(
      () => verifyCandidateManifest(
        releaseDirectory,
        { ...manifest, unexpected: true },
        candidate.metadata,
        "release",
      ),
      /unexpected field/,
    );
  });

  it("requires the exact artifact directory set", () => {
    const candidate = writeCandidate();
    const manifest = createCandidateManifest(candidate.directory, candidate.metadata);

    assert.throws(
      () => verifyCandidateManifest(
        candidate.directory,
        manifest,
        candidate.metadata,
        "artifact",
      ),
      /artifact file set does not match.*missing: candidate-manifest\.json/,
    );

    writeManifest(candidate.directory, manifest);
    fs.writeFileSync(path.join(candidate.directory, "unexpected"), "unexpected");
    assert.throws(
      () => verifyCandidateManifest(
        candidate.directory,
        manifest,
        candidate.metadata,
        "artifact",
      ),
      /artifact file set does not match.*unexpected: unexpected/,
    );
  });
});

describe("evaluateNpmState", () => {
  it("publishes stable and beta versions to their fixed dist-tags", () => {
    assert.deepEqual(
      evaluateNpmState(
        { version: "1.2.3", channel: "stable", integrity: "sha512-YQ==" },
        { distTags: { latest: "1.2.2", beta: "1.3.0-beta.1" } },
      ),
      { distTag: "latest", action: "publish" },
    );
    assert.deepEqual(
      evaluateNpmState(
        { version: "1.3.0-beta.2", channel: "beta", integrity: "sha512-Yg==" },
        { distTags: { latest: "1.2.3", beta: "1.3.0-beta.1" } },
      ),
      { distTag: "beta", action: "publish" },
    );
  });

  it("reuses the same published version with identical integrity", () => {
    assert.deepEqual(
      evaluateNpmState(
        { version: "1.2.3", channel: "stable", integrity: "sha512-YQ==" },
        {
          versionPresent: true,
          publishedIntegrity: "sha512-YQ==",
          distTags: { latest: "1.2.3" },
        },
      ),
      { distTag: "latest", action: "reuse" },
    );
  });

  it("rejects same npm version with different or missing integrity", () => {
    const target = { version: "1.2.3", channel: "stable", integrity: "sha512-YQ==" };

    assert.throws(
      () => evaluateNpmState(target, {
        versionPresent: true,
        publishedIntegrity: "sha512-Yg==",
      }),
      /different integrity/,
    );
    assert.throws(
      () => evaluateNpmState(target, { versionPresent: true }),
      /published integrity is missing/,
    );
  });

  it("does not move latest or beta backwards or overwrite an absent equal version", () => {
    assert.throws(
      () => evaluateNpmState(
        { version: "1.2.3", channel: "stable", integrity: "sha512-YQ==" },
        { distTags: { latest: "1.2.4" } },
      ),
      /must not move backwards/,
    );
    assert.throws(
      () => evaluateNpmState(
        { version: "1.2.3-beta.2", channel: "beta", integrity: "sha512-YQ==" },
        { distTags: { beta: "1.2.3-beta.3" } },
      ),
      /must not move backwards/,
    );
    assert.throws(
      () => evaluateNpmState(
        { version: "1.2.3", channel: "stable", integrity: "sha512-YQ==" },
        { distTags: { latest: "1.2.3" } },
      ),
      /already points to target version.*registry reports that version absent/,
    );
  });

  it("rejects malformed and cross-channel dist-tags", () => {
    const target = { version: "1.2.3", channel: "stable", integrity: "sha512-YQ==" };
    for (const distTags of [
      { latest: "1.2.3-beta.1" },
      { beta: "1.2.3" },
      { latest: "v1.2.2" },
      { beta: "1.2.3-rc.1" },
    ]) {
      assert.throws(
        () => evaluateNpmState(target, { distTags }),
        /dist-tag (latest|beta).*must contain a valid (stable|beta) version/,
      );
    }
  });
});

describe("CLI", () => {
  it("creates and verifies an artifact manifest with JSON stdout", () => {
    const candidate = writeCandidate("3.0.0-beta.1", "beta");
    const script = path.join(__dirname, "release-candidate.js");
    const manifestPath = path.join(candidate.directory, "candidate-manifest.json");
    const createResult = spawnSync(
      process.execPath,
      [
        script,
        "create",
        "--directory", candidate.directory,
        "--manifest", manifestPath,
        "--source-sha", SOURCE_SHA,
        "--version", candidate.metadata.version,
        "--channel", candidate.metadata.channel,
        "--npm-package", candidate.metadata.npmPackage,
      ],
      { encoding: "utf8" },
    );

    assert.equal(createResult.status, 0, createResult.stderr);
    assert.equal(createResult.stderr, "");
    const createOutput = JSON.parse(createResult.stdout);
    assert.equal(createOutput.ok, true);
    assert.equal(createOutput.manifest.channel, "beta");
    assert.deepEqual(JSON.parse(fs.readFileSync(manifestPath, "utf8")), createOutput.manifest);

    const verifyResult = spawnSync(
      process.execPath,
      [
        script,
        "verify",
        "--directory", candidate.directory,
        "--manifest", manifestPath,
        "--scope", "artifact",
        "--source-sha", SOURCE_SHA,
        "--version", candidate.metadata.version,
        "--channel", candidate.metadata.channel,
      ],
      { encoding: "utf8" },
    );

    assert.equal(verifyResult.status, 0, verifyResult.stderr);
    assert.equal(verifyResult.stderr, "");
    assert.deepEqual(JSON.parse(verifyResult.stdout), {
      ok: true,
      scope: "artifact",
      version: "3.0.0-beta.1",
      channel: "beta",
      sourceSha: SOURCE_SHA.toLowerCase(),
    });
  });

  it("writes deterministic CLI failures to stderr", () => {
    const script = path.join(__dirname, "release-candidate.js");
    const result = spawnSync(process.execPath, [script, "unknown"], { encoding: "utf8" });

    assert.equal(result.status, 1);
    assert.equal(result.stdout, "");
    assert.deepEqual(JSON.parse(result.stderr), {
      ok: false,
      error: {
        type: "release_candidate",
        message: "command must be create or verify",
      },
    });
  });
});
