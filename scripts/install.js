// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");
const os = require("os");
const crypto = require("crypto");

class ChecksumError extends Error {}
class NetworkError extends Error {}
class PackageIntegrityError extends Error {}

const VERSION = require("../package.json").version;
const REPO = "larksuite/cli";
const NAME = "lark-cli";

const PLATFORM_MAP = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

const isWindows = process.platform === "win32";
const ext = isWindows ? ".zip" : ".tar.gz";

const ALLOWED_INITIAL_HOSTS = new Set([
  "github.com",
  "registry.npmmirror.com",
]);

const CURL_CONNECT_TIMEOUT_SEC = 10;
const CURL_MAX_TIME_SEC        = 120;
const CURL_MAX_REDIRS          = 5;

const DEFAULT_CHECKSUM_PATH = path.join(__dirname, "..", "checksums.txt");

// Defensive: escape single quotes for PowerShell literal-string embedding.
// tmpDir comes from mkdtempSync so is controlled, but this hardens against
// future refactors that route external input into the script.
function escapeSingleQuotes(s) {
  return s.replace(/'/g, "''");
}

const binDir = path.join(__dirname, "..", "bin");
const dest = path.join(binDir, NAME + (isWindows ? ".exe" : ""));

function download(url, destPath) {
  // JS-layer pre-check: initial URL must be https and in allowlist.
  // Redirect targets are NOT host-checked; we rely on curl's
  // --proto-redir =https + --max-redirs + SHA256 verify for safety.
  const parsed = new URL(url);
  if (parsed.protocol !== "https:") {
    throw new NetworkError(`Non-HTTPS URL rejected: ${url}`);
  }
  if (!ALLOWED_INITIAL_HOSTS.has(parsed.hostname)) {
    throw new NetworkError(`Untrusted initial host: ${parsed.hostname}`);
  }

  const args = [
    "--fail",                                     // HTTP 4xx/5xx -> non-zero exit
    "--location",                                 // follow redirects
    "--proto",       "=https",                    // initial URL: https only
    "--proto-redir", "=https",                    // redirect targets: https only
    "--max-redirs",  String(CURL_MAX_REDIRS),
    "--tlsv1.2",                                  // minimum TLS 1.2
    "--connect-timeout", String(CURL_CONNECT_TIMEOUT_SEC),
    "--max-time",        String(CURL_MAX_TIME_SEC),
    "--silent", "--show-error",
    "--output", destPath,
  ];

  if (isWindows) {
    // Schannel CRL check hard-fails when the CRL server is unreachable;
    // this flag was in the original install.js and is preserved to
    // avoid regression for users in corporate networks.
    args.unshift("--ssl-revoke-best-effort");
  }

  // URL is always the last positional arg.
  args.push(url);

  try {
    execFileSync("curl", args, {
      stdio: ["ignore", "ignore", "pipe"],
    });
  } catch (err) {
    if (err.code === "ENOENT") {
      // ENOENT is NOT a NetworkError: another source won't help (curl
      // is missing). Throw plain Error so the fallback loop re-raises
      // instead of silently trying the next URL.
      throw new Error(
        "curl is required for installation but was not found in PATH. " +
        "Install curl or manually download the binary from " +
        `https://github.com/${REPO}/releases/tag/v${VERSION}`
      );
    }
    const stderr = err.stderr ? err.stderr.toString().trim() : "";
    const exitCode = err.status != null ? err.status : "unknown";
    throw new NetworkError(
      `curl exited with code ${exitCode}${stderr ? ": " + stderr : ""}`
    );
  }
}

function downloadWithFallback(urls, destPath) {
  const attempts = [];
  for (const url of urls) {
    try {
      download(url, destPath);
      return url;
    } catch (err) {
      if (err instanceof NetworkError) {
        attempts.push({ url, error: err.message });
        continue;
      }
      // ChecksumError, plain Error (ENOENT), or any other type:
      // re-raise immediately without trying the next source.
      throw err;
    }
  }
  const detail = attempts
    .map((a) => `  - ${a.url}\n      ${a.error}`)
    .join("\n");
  throw new NetworkError(`All download sources failed:\n${detail}`);
}

function extract(archivePath, tmpDir) {
  if (isWindows) {
    const script =
      `$ErrorActionPreference = 'Stop'\n` +
      `Expand-Archive -LiteralPath '${escapeSingleQuotes(archivePath)}' ` +
      `-DestinationPath '${escapeSingleQuotes(tmpDir)}' -Force\n`;

    const scriptPath = path.join(tmpDir, "extract.ps1");
    fs.writeFileSync(scriptPath, script, { encoding: "utf-8" });

    execFileSync("powershell", [
      "-NoProfile",
      "-NonInteractive",
      "-ExecutionPolicy", "Bypass",
      "-File", scriptPath,
    ], { stdio: "ignore" });
  } else {
    execFileSync("tar", ["-xzf", archivePath, "-C", tmpDir], {
      stdio: "ignore",
    });
  }
}

function verifyChecksum(filePath, expectedHash) {
  return new Promise((resolve, reject) => {
    const hash = crypto.createHash("sha256");
    const stream = fs.createReadStream(filePath);
    stream.on("error", reject);
    stream.on("data", (chunk) => hash.update(chunk));
    stream.on("end", () => {
      const actual = hash.digest("hex");
      const expected = expectedHash.toLowerCase();
      if (actual !== expected) {
        reject(new ChecksumError(
          `SHA256 mismatch for ${path.basename(filePath)}\n` +
          `  expected: ${expected}\n` +
          `  actual:   ${actual}`
        ));
        return;
      }
      resolve();
    });
  });
}

function getExpectedChecksum(archiveFilename, checksumPath = DEFAULT_CHECKSUM_PATH) {
  if (!fs.existsSync(checksumPath)) {
    // Packaging bug, not a tamper signal — routed separately.
    throw new PackageIntegrityError("checksums.txt missing from package");
  }

  const contents = fs.readFileSync(checksumPath, "utf-8");
  const lineRegex = /^([0-9a-fA-F]{64})\s+\*?(.+)$/;

  for (const rawLine of contents.split("\n")) {
    const line = rawLine.trim();
    if (line === "" || line.startsWith("#")) continue;

    const match = line.match(lineRegex);
    if (!match) continue;

    const [, hash, filename] = match;
    if (filename.trim() === archiveFilename) {
      return hash.toLowerCase();
    }
  }

  throw new ChecksumError(`No checksum entry for ${archiveFilename}`);
}

async function install() {
  const platform = PLATFORM_MAP[process.platform];
  const arch = ARCH_MAP[process.arch];
  if (!platform || !arch) {
    throw new Error(
      `Unsupported platform: ${process.platform}-${process.arch}. ` +
      `Download manually from ` +
      `https://github.com/${REPO}/releases/tag/v${VERSION}`
    );
  }
  const archiveName = `${NAME}-${VERSION}-${platform}-${arch}${ext}`;
  const sources = [
    `https://github.com/${REPO}/releases/download/v${VERSION}/${archiveName}`,
    `https://registry.npmmirror.com/-/binary/lark-cli/v${VERSION}/${archiveName}`,
  ];

  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "lark-cli-"));
  const archivePath = path.join(tmpDir, archiveName);

  try {
    // 1. Early fail: if the bundled checksums.txt is broken,
    //    report now before spending bandwidth.
    const expectedHash = getExpectedChecksum(archiveName);

    // 2. Multi-source download; only NetworkError triggers fallback.
    const sourceUrl = downloadWithFallback(sources, archivePath);

    // 3. Integrity check outside the fallback loop. Mismatch aborts
    //    the entire install, does NOT try the next source.
    await verifyChecksum(archivePath, expectedHash);

    // 4. Extract (safe: bytes match the official release).
    extract(archivePath, tmpDir);

    // 5. Copy binary into place and chmod.
    const binaryName = NAME + (isWindows ? ".exe" : "");
    const extractedBinary = path.join(tmpDir, binaryName);
    fs.mkdirSync(path.dirname(dest), { recursive: true });
    fs.copyFileSync(extractedBinary, dest);
    fs.chmodSync(dest, 0o755);

    console.log(
      `${NAME} v${VERSION} installed successfully ` +
      `(from ${new URL(sourceUrl).hostname})`
    );
  } finally {
    // 6. Always clean up the temp directory.
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

if (require.main === module) {
  install().catch((err) => {
    if (err instanceof PackageIntegrityError) {
      console.error(`\n${NAME} install aborted: the installed package looks broken.\n`);
      console.error(err.message);
      console.error(
        `\nRe-install the package; if the issue persists, please report it:\n` +
        `  https://github.com/${REPO}/issues\n`
      );
    } else if (err instanceof ChecksumError) {
      console.error(`\n[SECURITY] ${NAME} install aborted due to integrity check failure:\n`);
      console.error(err.message);
      console.error(
        `\nRetry the install; if it persists, report it and download manually:\n` +
        `  https://github.com/${REPO}/releases/tag/v${VERSION}\n`
      );
    } else if (err instanceof NetworkError) {
      console.error(`\n${NAME} install failed due to network errors:\n`);
      console.error(err.message);
      console.error(
        `\nIf you are behind a firewall or on a restricted network, try configuring a proxy:\n` +
        `  export https_proxy=http://your-proxy:port\n` +
        `  npm install -g @larksuite/cli\n`
      );
    } else {
      console.error(`\n${NAME} install failed:\n${err.stack || err.message}`);
    }
    process.exit(1);
  });
}

module.exports = {
  verifyChecksum,
  getExpectedChecksum,
  ChecksumError,
  NetworkError,
  PackageIntegrityError,
};
