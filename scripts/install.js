const fs = require("fs");
const path = require("path");
const { execSync, execFileSync } = require("child_process");
const os = require("os");

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
  riscv64: "riscv64",
};

const platform = PLATFORM_MAP[process.platform];
const arch = ARCH_MAP[process.arch];

if (!platform || !arch) {
  console.error(`Unsupported platform: ${process.platform}-${process.arch}`);
  process.exit(1);
}

const isWindows = process.platform === "win32";
const ext = isWindows ? ".zip" : ".tar.gz";
const archiveName = `${NAME}-${VERSION}-${platform}-${arch}${ext}`;
const GITHUB_URL = `https://github.com/${REPO}/releases/download/v${VERSION}/${archiveName}`;
const MIRROR_URL = `https://registry.npmmirror.com/-/binary/lark-cli/v${VERSION}/${archiveName}`;

const binDir = path.join(__dirname, "..", "bin");
const dest = path.join(binDir, NAME + (isWindows ? ".exe" : ""));

const DEFAULT_BUILD_DATE = new Date().toISOString().slice(0, 10);

function sanitizeBuildVersion(input) {
  const value = String(input || "").trim();
  if (!value) {
    return VERSION;
  }
  return /^[A-Za-z0-9._-]+$/.test(value) ? value : VERSION;
}

function sanitizeBuildDate(input) {
  const value = String(input || "").trim();
  if (!value) {
    return DEFAULT_BUILD_DATE;
  }
  return /^\d{4}-\d{2}-\d{2}$/.test(value) ? value : DEFAULT_BUILD_DATE;
}

const BUILD_VERSION = sanitizeBuildVersion(process.env.LARK_CLI_BUILD_VERSION);
const BUILD_DATE = sanitizeBuildDate(process.env.LARK_CLI_BUILD_DATE);

fs.mkdirSync(binDir, { recursive: true });

function isMissingBinaryError(err) {
  const msg = String((err && err.message) || "");
  const stderr = String((err && err.stderr) || "");
  const stdout = String((err && err.stdout) || "");
  const status = Number((err && (err.status ?? err.statusCode)) || 0);

  if (status === 22) {
    return true;
  }

  if (/404|not found|unsupported platform|unsupported architecture/i.test(msg)) {
    return true;
  }

  if (/404|not found/i.test(stderr) || /404|not found/i.test(stdout)) {
    return true;
  }

  return false;
}

function download(url, destPath) {
  const sslFlag = isWindows ? "--ssl-revoke-best-effort " : "";
  execSync(
    `curl ${sslFlag}--fail --location --silent --show-error --connect-timeout 10 --max-time 120 --output "${destPath}" "${url}"`,
    { stdio: ["ignore", "ignore", "pipe"] }
  );
}

function extractArchive(archivePath, tmpDir) {
  if (isWindows) {
    execSync(
      `powershell -Command "Expand-Archive -Path '${archivePath}' -DestinationPath '${tmpDir}'"`,
      { stdio: "ignore" }
    );
  } else {
    execSync(`tar -xzf "${archivePath}" -C "${tmpDir}"`, {
      stdio: "ignore",
    });
  }
}

function installFromRelease() {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "lark-cli-"));
  const archivePath = path.join(tmpDir, archiveName);

  try {
    try {
      download(GITHUB_URL, archivePath);
    } catch (err) {
      download(MIRROR_URL, archivePath);
    }

    extractArchive(archivePath, tmpDir);

    const binaryName = NAME + (isWindows ? ".exe" : "");
    const extractedBinary = path.join(tmpDir, binaryName);

    fs.copyFileSync(extractedBinary, dest);
    fs.chmodSync(dest, 0o755);
    console.log(`${NAME} v${VERSION} installed successfully`);
    return true;
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

function installFromSource() {
  if (isWindows) {
    throw new Error("source fallback is not supported on Windows yet");
  }

  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "lark-cli-src-"));

  try {
    execFileSync("go", ["version"], { stdio: "ignore" });

    execFileSync("git", ["clone", "--depth", "1", "--branch", `v${VERSION}`, `https://github.com/${REPO}.git`, tmpDir], {
      stdio: "ignore",
    });

    const ldflags = [
      "-s",
      "-w",
      `-X github.com/larksuite/cli/internal/build.Version=${BUILD_VERSION}`,
      `-X github.com/larksuite/cli/internal/build.Date=${BUILD_DATE}`,
    ].join(" ");

    execFileSync("go", ["build", "-trimpath", "-ldflags", ldflags, "-o", dest, "."], {
      cwd: tmpDir,
      env: { ...process.env, CGO_ENABLED: "0" },
      stdio: "ignore",
    });

    fs.chmodSync(dest, 0o755);
    console.log(`${NAME} v${VERSION} installed from source fallback`);
    return true;
  } finally {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  }
}

function install() {
  try {
    return installFromRelease();
  } catch (err) {
    const isMissingBinary = isMissingBinaryError(err);

    if (isMissingBinary || process.arch === "riscv64") {
      console.warn(
        `Prebuilt binary unavailable for ${process.platform}-${process.arch}, attempting source fallback...`
      );
      return installFromSource();
    }

    throw err;
  }
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
  if (process.arch === "riscv64") {
    console.error(
      `\nFor riscv64, ensure 'go' and 'git' are installed to enable source fallback build.`
    );
  }
  process.exit(1);
}
