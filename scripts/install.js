// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");
const os = require("os");
const crypto = require("crypto");

const VERSION = require("../package.json").version.replace(/-.*$/, "");
const REPO = "larksuite/cli";
const NAME = "lark-cli";
// Allowlist gates the *initial* request URL only. curl --location follows
// redirects (capped by --max-redirs 3) without re-checking the target host.
// This is acceptable because checksum verification is the primary integrity
// control; the allowlist is defense-in-depth to reject obviously wrong URLs.
const ALLOWED_HOSTS = [
  "github.com",
  "objects.githubusercontent.com",
  "registry.npmmirror.com",
];

const PLATFORM_MAP = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

const platform = PLATFORM_MAP[process.platform];
const arch = ARCH_MAP[process.arch];

const isWindows = process.platform === "win32";
const ext = isWindows ? ".zip" : ".tar.gz";
const archiveName = `${NAME}-${VERSION}-${platform}-${arch}${ext}`;
const GITHUB_URL = `https://github.com/${REPO}/releases/download/v${VERSION}/${archiveName}`;
const MIRROR_URL = `https://registry.npmmirror.com/-/binary/lark-cli/v${VERSION}/${archiveName}`;

const binDir = path.join(__dirname, "..", "bin");
const dest = path.join(binDir, NAME + (isWindows ? ".exe" : ""));

function assertAllowedHost(url) {
  const { hostname } = new URL(url);
  if (!ALLOWED_HOSTS.includes(hostname)) {
    throw new Error(`Download host not allowed: ${hostname}`);
  }
}

function download(url, destPath) {
  assertAllowedHost(url);
  const args = [
    "--fail", "--location", "--silent", "--show-error",
    "--connect-timeout", "10", "--max-time", "120",
    "--max-redirs", "3",
    "--output", destPath,
  ];
  // --ssl-revoke-best-effort: on Windows (Schannel), avoid CRYPT_E_REVOCATION_OFFLINE
  // errors when the certificate revocation list server is unreachable
  if (isWindows) args.unshift("--ssl-revoke-best-effort");
  args.push(url);
  execFileSync("curl", args, { stdio: ["ignore", "ignore", "pipe"] });
}

function install() {
  fs.mkdirSync(binDir, { recursive: true });

  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "lark-cli-"));
  const archivePath = path.join(tmpDir, archiveName);

  try {
    try {
      download(GITHUB_URL, archivePath);
    } catch (err) {
      download(MIRROR_URL, archivePath);
    }

    const expectedHash = getExpectedChecksum(archiveName);
    verifyChecksum(archivePath, expectedHash);

    if (isWindows) {
      execFileSync("powershell", [
        "-Command",
        `Expand-Archive -Path '${archivePath}' -DestinationPath '${tmpDir}'`,
      ], { stdio: "ignore" });
    } else {
      execFileSync("tar", ["-xzf", archivePath, "-C", tmpDir], {
        stdio: "ignore",
      });
    }

    const binaryName = NAME + (isWindows ? ".exe" : "");
    const extractedBinary = path.join(tmpDir, binaryName);

    fs.copyFileSync(extractedBinary, dest);
    fs.chmodSync(dest, 0o755);
    console.log(`${NAME} v${VERSION} installed successfully`);
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

function getExpectedChecksum(archiveName, checksumsDir) {
  const dir = checksumsDir || path.join(__dirname, "..");
  const checksumsPath = path.join(dir, "checksums.txt");

  if (!fs.existsSync(checksumsPath)) {
    console.error(
      "[WARN] checksums.txt not found, skipping checksum verification"
    );
    return null;
  }

  const content = fs.readFileSync(checksumsPath, "utf8");
  for (const line of content.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed) continue;
    const idx = trimmed.indexOf("  ");
    if (idx === -1) continue;
    const hash = trimmed.slice(0, idx);
    const name = trimmed.slice(idx + 2);
    if (name === archiveName) return hash;
  }

  throw new Error(`Checksum entry not found for ${archiveName}`);
}

function verifyChecksum(archivePath, expectedHash) {
  if (expectedHash === null) return;

  // Stream the file to avoid loading the entire archive into memory.
  // Archives can be 10-100MB; streaming keeps RSS constant.
  const hash = crypto.createHash("sha256");
  const fd = fs.openSync(archivePath, "r");
  try {
    const buf = Buffer.alloc(64 * 1024);
    let bytesRead;
    while ((bytesRead = fs.readSync(fd, buf, 0, buf.length, null)) > 0) {
      hash.update(buf.subarray(0, bytesRead));
    }
  } finally {
    fs.closeSync(fd);
  }
  const actual = hash.digest("hex");

  if (actual.toLowerCase() !== expectedHash.toLowerCase()) {
    throw new Error(
      `[SECURITY] Checksum mismatch for ${path.basename(archivePath)}: expected ${expectedHash} but got ${actual}`
    );
  }
}

if (require.main === module) {
  if (!platform || !arch) {
    console.error(
      `Unsupported platform: ${process.platform}-${process.arch}`
    );
    process.exit(1);
  }

  // When triggered as a postinstall hook under npx, skip the binary download.
  // The "install" wizard doesn't need it, and run.js calls install.js directly
  // (with LARK_CLI_RUN=1) for other commands that do need the binary.
  const isNpxPostinstall =
    process.env.npm_command === "exec" && !process.env.LARK_CLI_RUN;

  if (isNpxPostinstall) {
    process.exit(0);
  }

  try {
    install();
  } catch (err) {
    console.error(`Failed to install ${NAME}:`, err.message);
    console.error(
      `\nIf you are behind a firewall or in a restricted network, try setting a proxy:\n` +
      `  export https_proxy=http://your-proxy:port\n` +
      `  npm install -g @larksuite/cli`
    );
    process.exit(1);
  }
}

module.exports = { getExpectedChecksum, verifyChecksum, assertAllowedHost };
