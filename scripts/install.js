const fs = require("fs");
const path = require("path");
const https = require("https");
const crypto = require("crypto");
const { execFileSync } = require("child_process");
const os = require("os");

const VERSION = require("../package.json").version;
const REPO = "larksuite/cli";
const NAME = "lark-cli";

const MAX_REDIRECTS = 5;

function isTestMode() {
  return process.env.LARK_CLI_TEST_MODE === '1';
}

function isAllowedHost(hostname) {
  if (hostname === 'github.com' || hostname.endsWith('.githubusercontent.com')) {
    return true;
  }
  if (isTestMode() && process.env.LARK_CLI_TEST_ALLOWED_HOSTS) {
    const extraHosts = process.env.LARK_CLI_TEST_ALLOWED_HOSTS.split(',').map(h => h.trim());
    return extraHosts.includes(hostname);
  }
  return false;
}

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

function getDownloadUrl() {
  if (isTestMode() && process.env.LARK_CLI_TEST_BASE_URL) {
    return `${process.env.LARK_CLI_TEST_BASE_URL}/${archiveName}`;
  }
  return `https://github.com/${REPO}/releases/download/v${VERSION}/${archiveName}`;
}

function getBinDir() {
  if (isTestMode() && process.env.LARK_CLI_TEST_OUTPUT_DIR) {
    return process.env.LARK_CLI_TEST_OUTPUT_DIR;
  }
  return path.join(__dirname, "..", "bin");
}

function getDest() {
  return path.join(getBinDir(), NAME + (isWindows ? ".exe" : ""));
}

function download(downloadUrl, destPath, redirectCount = 0) {
  return new Promise((resolve, reject) => {
    if (redirectCount > MAX_REDIRECTS) {
      return reject(new Error("Too many redirects."));
    }

    const parsed = new URL(downloadUrl);

    if (parsed.protocol !== "https:") {
      return reject(new Error(`Redirect to non-HTTPS URL rejected: ${downloadUrl}`));
    }

    if (!isAllowedHost(parsed.hostname)) {
      return reject(new Error(`Redirect to untrusted host: ${parsed.hostname}`));
    }

    https
      .get(downloadUrl, (res) => {
        if (res.statusCode === 301 || res.statusCode === 302) {
          const location = res.headers.location;
          if (!location) {
            return reject(new Error("Redirect with no Location header."));
          }
          return download(location, destPath, redirectCount + 1).then(resolve, reject);
        }
        if (res.statusCode !== 200) {
          return reject(new Error(`Download failed with status ${res.statusCode}: ${downloadUrl}`));
        }
        const file = fs.createWriteStream(destPath);
        res.pipe(file);
        file.on("finish", () => {
          file.close();
          resolve();
        });
      })
      .on("error", reject);
  });
}

function getExpectedChecksum(archiveFilename) {
  const checksumPath = isTestMode() && process.env.LARK_CLI_TEST_CHECKSUM_PATH
    ? process.env.LARK_CLI_TEST_CHECKSUM_PATH
    : path.join(__dirname, "..", "checksums.txt");
  if (!fs.existsSync(checksumPath)) {
    throw new Error("Checksum file not found. Package may be corrupted.");
  }

  const content = fs.readFileSync(checksumPath, "utf-8");
  const lines = content.trim().split(/\r?\n/);
  for (const line of lines) {
    // goreleaser format: "<sha256hex>  <filename>"
    const separatorIndex = line.indexOf("  ");
    if (separatorIndex === -1) continue;
    const hash = line.substring(0, separatorIndex);
    const filename = line.substring(separatorIndex + 2).trim();
    if (filename === archiveFilename) {
      return hash;
    }
  }

  throw new Error(`No checksum entry for ${archiveFilename}.`);
}

function verifyChecksum(filePath, expectedHash) {
  const fileBuffer = fs.readFileSync(filePath);
  const actualHash = crypto.createHash("sha256").update(fileBuffer).digest("hex");
  if (actualHash !== expectedHash) {
    throw new Error(
      `Checksum verification failed!\n` +
      `  Expected: ${expectedHash}\n` +
      `  Actual:   ${actualHash}\n` +
      `This may indicate a corrupted download or tampered binary. Please retry or install from source.`
    );
  }
}

async function install() {
  const url = getDownloadUrl();
  const binDir = getBinDir();
  const dest = getDest();

  fs.mkdirSync(binDir, { recursive: true });

  const expectedHash = getExpectedChecksum(archiveName);
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "lark-cli-"));
  const archivePath = path.join(tmpDir, archiveName);

  try {
    await download(url, archivePath);
    verifyChecksum(archivePath, expectedHash);

    if (isWindows) {
      const script =
        `Expand-Archive -LiteralPath '${archivePath.replace(/'/g, "''")}' -DestinationPath '${tmpDir.replace(/'/g, "''")}'`;
      const scriptPath = path.join(tmpDir, "extract.ps1");
      fs.writeFileSync(scriptPath, script);
      execFileSync("powershell", ["-ExecutionPolicy", "Bypass", "-File", scriptPath], {
        stdio: "ignore",
      });
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

if (require.main === module) {
  install().catch((err) => {
    console.error(`Failed to install ${NAME}:`, err.message);
    process.exit(1);
  });
}

module.exports = {
  isAllowedHost,
  download,
  getExpectedChecksum,
  verifyChecksum,
  install,
  MAX_REDIRECTS,
};
