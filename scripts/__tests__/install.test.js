const { describe, it, before, after, beforeEach } = require("node:test");
const assert = require("node:assert/strict");
const https = require("node:https");
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const os = require("node:os");
const { execFileSync } = require("node:child_process");

// Enable test mode before requiring install.js
process.env.LARK_CLI_TEST_MODE = "1";
process.env.LARK_CLI_TEST_ALLOWED_HOSTS = "localhost,127.0.0.1";
process.env.NODE_TLS_REJECT_UNAUTHORIZED = "0";

const installModule = require("../install.js");
const { isAllowedHost, download, getExpectedChecksum, verifyChecksum, MAX_REDIRECTS } = installModule;

const FIXTURES_DIR = path.join(__dirname, "fixtures");
const cert = fs.readFileSync(path.join(FIXTURES_DIR, "test-cert.pem"));
const key = fs.readFileSync(path.join(FIXTURES_DIR, "test-key.pem"));

let server;
let serverPort;
let routeHandler;

function setRoute(handler) {
  routeHandler = handler;
}

function serverUrl(urlPath) {
  return `https://localhost:${serverPort}${urlPath || "/"}`;
}

describe("install.js E2E tests", () => {
  before(async () => {
    server = https.createServer({ cert, key }, (req, res) => {
      if (routeHandler) {
        routeHandler(req, res);
      } else {
        res.writeHead(404);
        res.end("No route configured");
      }
    });
    await new Promise((resolve) => {
      server.listen(0, "localhost", resolve);
    });
    serverPort = server.address().port;
  });

  after(async () => {
    await new Promise((resolve) => server.close(resolve));
  });

  beforeEach(() => {
    routeHandler = null;
  });

  // Test #1: Happy path — download, verify checksum, extract, binary exists
  it("should download, verify checksum, extract, and produce binary", async () => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "install-test-"));
    const outputDir = path.join(tmpDir, "bin");
    const archiveDir = path.join(tmpDir, "archive");
    const binaryName = "lark-cli";

    try {
      // Create a fake binary
      fs.mkdirSync(archiveDir, { recursive: true });
      const fakeBinaryPath = path.join(archiveDir, binaryName);
      fs.writeFileSync(fakeBinaryPath, "#!/bin/sh\necho fake-lark-cli-v1.0.0\n");
      fs.chmodSync(fakeBinaryPath, 0o755);

      // Create .tar.gz archive
      const pkg = require("../../package.json");
      const platformName = process.platform === "win32" ? "windows" : process.platform;
      const archMap = { x64: "amd64", arm64: "arm64" };
      const archiveName = `lark-cli-${pkg.version}-${platformName}-${archMap[process.arch]}.tar.gz`;
      const archivePath = path.join(tmpDir, archiveName);

      execFileSync("tar", ["-czf", archivePath, "-C", archiveDir, binaryName]);

      // Compute SHA256
      const archiveBuffer = fs.readFileSync(archivePath);
      const sha256 = crypto.createHash("sha256").update(archiveBuffer).digest("hex");

      // Write checksums.txt
      const checksumPath = path.join(tmpDir, "checksums.txt");
      fs.writeFileSync(checksumPath, `${sha256}  ${archiveName}\n`);

      // Configure mock server to serve the archive
      const archiveContent = fs.readFileSync(archivePath);
      setRoute((req, res) => {
        if (req.url.endsWith(archiveName)) {
          res.writeHead(200, {
            "Content-Type": "application/octet-stream",
            "Content-Length": archiveContent.length,
          });
          res.end(archiveContent);
        } else {
          res.writeHead(404);
          res.end("Not found");
        }
      });

      // Test download
      const downloadedPath = path.join(tmpDir, "downloaded-" + archiveName);
      await download(serverUrl(`/${archiveName}`), downloadedPath);

      // Test checksum verification
      process.env.LARK_CLI_TEST_CHECKSUM_PATH = checksumPath;
      const expectedHash = getExpectedChecksum(archiveName);
      assert.equal(expectedHash, sha256);
      verifyChecksum(downloadedPath, expectedHash);

      // Test extraction
      fs.mkdirSync(outputDir, { recursive: true });
      execFileSync("tar", ["-xzf", downloadedPath, "-C", outputDir]);

      // Verify binary exists and is executable
      const installedBinary = path.join(outputDir, binaryName);
      assert.ok(fs.existsSync(installedBinary), "Binary should exist at output path");
      const result = execFileSync(installedBinary, { encoding: "utf-8" });
      assert.match(result, /fake-lark-cli/);
    } finally {
      delete process.env.LARK_CLI_TEST_CHECKSUM_PATH;
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  // Test #2: HTTPS enforcement — redirect to http:// rejected
  it("should reject redirect to non-HTTPS URL", async () => {
    setRoute((req, res) => {
      res.writeHead(302, { Location: "http://evil.com/malware.tar.gz" });
      res.end();
    });

    const tmpFile = path.join(os.tmpdir(), "test-download-" + Date.now());
    await assert.rejects(
      () => download(serverUrl("/archive.tar.gz"), tmpFile),
      (err) => {
        assert.match(err.message, /non-HTTPS URL rejected/);
        return true;
      }
    );
  });

  // Test #3: Host allowlist — redirect to untrusted host
  it("should reject redirect to untrusted host", async () => {
    setRoute((req, res) => {
      res.writeHead(302, { Location: "https://evil.com/malware.tar.gz" });
      res.end();
    });

    const tmpFile = path.join(os.tmpdir(), "test-download-" + Date.now());
    await assert.rejects(
      () => download(serverUrl("/archive.tar.gz"), tmpFile),
      (err) => {
        assert.match(err.message, /untrusted host/);
        return true;
      }
    );
  });

  // Test #4: Redirect depth — too many redirects
  it("should reject after too many redirects", async () => {
    let redirectCount = 0;
    setRoute((req, res) => {
      redirectCount++;
      res.writeHead(302, { Location: serverUrl(`/redirect-${redirectCount}`) });
      res.end();
    });

    const tmpFile = path.join(os.tmpdir(), "test-download-" + Date.now());
    await assert.rejects(
      () => download(serverUrl("/start"), tmpFile),
      (err) => {
        assert.match(err.message, /Too many redirects/);
        return true;
      }
    );
  });

  // Test #5: Checksum mismatch
  it("should throw on checksum mismatch", () => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "checksum-test-"));
    try {
      const filePath = path.join(tmpDir, "test-file");
      fs.writeFileSync(filePath, "real content");
      const wrongHash = "0000000000000000000000000000000000000000000000000000000000000000";

      assert.throws(
        () => verifyChecksum(filePath, wrongHash),
        (err) => {
          assert.match(err.message, /Checksum verification failed/);
          return true;
        }
      );
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  // Test #6: Checksum file missing
  it("should throw when checksums.txt is missing", () => {
    const originalPath = process.env.LARK_CLI_TEST_CHECKSUM_PATH;
    try {
      process.env.LARK_CLI_TEST_CHECKSUM_PATH = "/nonexistent/checksums.txt";
      assert.throws(
        () => getExpectedChecksum("any-file.tar.gz"),
        (err) => {
          assert.match(err.message, /Checksum file not found/);
          return true;
        }
      );
    } finally {
      if (originalPath !== undefined) {
        process.env.LARK_CLI_TEST_CHECKSUM_PATH = originalPath;
      } else {
        delete process.env.LARK_CLI_TEST_CHECKSUM_PATH;
      }
    }
  });

  // Test #7: Redirect with no Location header
  it("should reject redirect without Location header", async () => {
    setRoute((req, res) => {
      res.writeHead(302);
      res.end();
    });

    const tmpFile = path.join(os.tmpdir(), "test-download-" + Date.now());
    await assert.rejects(
      () => download(serverUrl("/no-location"), tmpFile),
      (err) => {
        assert.match(err.message, /no Location header/i);
        return true;
      }
    );
  });

  // Test #8: Non-200 status code
  it("should reject on non-200 status code", async () => {
    setRoute((req, res) => {
      res.writeHead(404);
      res.end("Not Found");
    });

    const tmpFile = path.join(os.tmpdir(), "test-download-" + Date.now());
    await assert.rejects(
      () => download(serverUrl("/missing"), tmpFile),
      (err) => {
        assert.match(err.message, /Download failed with status 404/);
        return true;
      }
    );
  });

  // Test #9: Checksum entry missing for target archive
  it("should throw when checksums.txt lacks entry for target archive", () => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "checksum-test-"));
    try {
      const checksumPath = path.join(tmpDir, "checksums.txt");
      fs.writeFileSync(checksumPath, "abc123  some-other-file.tar.gz\n");

      const originalPath = process.env.LARK_CLI_TEST_CHECKSUM_PATH;
      process.env.LARK_CLI_TEST_CHECKSUM_PATH = checksumPath;
      try {
        assert.throws(
          () => getExpectedChecksum("nonexistent-archive.tar.gz"),
          (err) => {
            assert.match(err.message, /No checksum entry for/);
            return true;
          }
        );
      } finally {
        if (originalPath !== undefined) {
          process.env.LARK_CLI_TEST_CHECKSUM_PATH = originalPath;
        } else {
          delete process.env.LARK_CLI_TEST_CHECKSUM_PATH;
        }
      }
    } finally {
      fs.rmSync(tmpDir, { recursive: true, force: true });
    }
  });

  // Test #10: Network error (socket destroyed)
  it("should reject on network error", async () => {
    setRoute((req, res) => {
      req.socket.destroy();
    });

    const tmpFile = path.join(os.tmpdir(), "test-download-" + Date.now());
    await assert.rejects(
      () => download(serverUrl("/destroy"), tmpFile),
      (err) => {
        // ECONNRESET or similar network error
        assert.ok(err.message || err.code, "Should have error message or code");
        return true;
      }
    );
  });
});
