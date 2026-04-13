// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");
const os = require("os");

const VERSION = require("../package.json").version.replace(/-.*$/, "");
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

const platform = PLATFORM_MAP[process.platform];
const arch = ARCH_MAP[process.arch];

if (!platform || !arch) {
  console.error(
    `Unsupported platform: ${process.platform}-${process.arch}`
  );
  process.exit(1);
}

const isWindows = process.platform === "win32";
const ext = isWindows ? ".zip" : ".tar.gz";
const archiveName = `${NAME}-${VERSION}-${platform}-${arch}${ext}`;
const GITHUB_URL = `https://github.com/${REPO}/releases/download/v${VERSION}/${archiveName}`;
const MIRROR_URL = `https://registry.npmmirror.com/-/binary/lark-cli/v${VERSION}/${archiveName}`;

const binDir = path.join(__dirname, "..", "bin");
const dest = path.join(binDir, NAME + (isWindows ? ".exe" : ""));

fs.mkdirSync(binDir, { recursive: true });

function download(url, destPath) {
  const args = [
    "--fail", "--location", "--silent", "--show-error",
    "--connect-timeout", "10", "--max-time", "120",
    "--output", destPath,
  ];
  // --ssl-revoke-best-effort: on Windows (Schannel), avoid CRYPT_E_REVOCATION_OFFLINE
  // errors when the certificate revocation list server is unreachable
  if (isWindows) args.unshift("--ssl-revoke-best-effort");
  args.push(url);
  execFileSync("curl", args, { stdio: ["ignore", "ignore", "pipe"] });
}

function install() {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "lark-cli-"));
  const archivePath = path.join(tmpDir, archiveName);

  try {
    try {
      download(GITHUB_URL, archivePath);
    } catch (err) {
      download(MIRROR_URL, archivePath);
    }

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
