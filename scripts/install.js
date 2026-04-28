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
const DEFAULT_MIRROR_HOST = "https://registry.npmmirror.com";
// Allowlist gates the *initial* request URL only. curl --location follows
// redirects (capped by --max-redirs 3) without re-checking the target host.
// This is acceptable because checksum verification is the primary integrity
// control; the allowlist is defense-in-depth to reject obviously wrong URLs.
const ALLOWED_HOSTS = new Set([
  "github.com",
  "objects.githubusercontent.com",
  "registry.npmmirror.com",
]);

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

const binDir = path.join(__dirname, "..", "bin");
const dest = path.join(binDir, NAME + (isWindows ? ".exe" : ""));

// Build the ordered list of binary mirror URLs to try. Resolution rules:
//   1. LARK_CLI_DOWNLOAD_HOST  — explicit override; returned as the SOLE
//                                URL so we never silently fall through to
//                                a host the user did not authorize.
//   2. npm_config_registry     — when the user has set a non-default
//                                registry (npmmirror clone, corp Verdaccio,
//                                Artifactory, …), include the derived path
//                                first. Many of these proxies don't actually
//                                host /-/binary/<pkg>/..., so we ALWAYS
//                                append the public npmmirror as a final
//                                fallback so the install does not regress
//                                from the previous behavior of "GitHub →
//                                npmmirror".
//   3. registry.npmmirror.com  — public China mirror, always tried last
//                                unless an explicit override was set.
// The default public npmjs registry is skipped in step 2 because it does not
// host binaries under /-/binary/...
//
// LARK_CLI_DOWNLOAD_HOST is constrained to HTTPS URLs with a real hostname.
// This prevents `file://` (would let curl read local paths and pass the empty
// hostname through assertAllowedHost), `ftp://`, or `http://` — which would
// silently downgrade transport security for the binary download.
function resolveMirrorUrls(env, archive, version) {
  const binaryPath = `/-/binary/lark-cli/v${version}/${archive}`;
  const defaultUrl = joinUrl(DEFAULT_MIRROR_HOST, binaryPath);

  const override = (env.LARK_CLI_DOWNLOAD_HOST || "").trim();
  if (override) {
    // User explicitly opted in — fail loudly on bad input rather than fall
    // through to a different host than they asked for. No additional fallback
    // either; the user picked this host on purpose.
    const base = parseDownloadBase(override, "LARK_CLI_DOWNLOAD_HOST");
    return [joinUrl(base.origin + base.pathname, binaryPath)];
  }

  const urls = [];
  const registry = (env.npm_config_registry || "").trim();
  if (registry && !isDefaultNpmjsRegistry(registry) && isValidDownloadBase(registry)) {
    const base = new URL(registry);
    urls.push(joinUrl(base.origin + base.pathname, binaryPath));
  }
  if (!urls.includes(defaultUrl)) urls.push(defaultUrl);
  return urls;
}

function joinUrl(base, suffix) {
  return base.replace(/\/+$/, "") + suffix;
}

function parseDownloadBase(raw, source) {
  let parsed;
  try {
    parsed = new URL(raw);
  } catch (_) {
    throw new Error(`${source} is not a valid URL: ${raw}`);
  }
  if (parsed.protocol !== "https:" || !parsed.hostname) {
    throw new Error(
      `${source} must be an https:// URL with a hostname (got ${raw})`
    );
  }
  return parsed;
}

function isValidDownloadBase(raw) {
  try {
    const parsed = new URL(raw);
    return parsed.protocol === "https:" && !!parsed.hostname;
  } catch (_) {
    return false;
  }
}

function isDefaultNpmjsRegistry(url) {
  try {
    const { hostname } = new URL(url);
    return hostname === "registry.npmjs.org";
  } catch (_) {
    return false;
  }
}

function assertAllowedHost(url) {
  const { hostname } = new URL(url);
  if (!ALLOWED_HOSTS.has(hostname)) {
    throw new Error(`Download host not allowed: ${hostname}`);
  }
}

// Resolve the mirror URL chain and admit each host. Called from install() so
// resolution errors (e.g. invalid LARK_CLI_DOWNLOAD_HOST) propagate into the
// guarded try/catch and surface the recovery guidance instead of a raw
// module-load stack trace.
function getMirrorUrls(env) {
  const urls = resolveMirrorUrls(env, archiveName, VERSION);
  for (const u of urls) ALLOWED_HOSTS.add(new URL(u).hostname);
  return urls;
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
  // Resolve the mirror URL chain up front so a bad LARK_CLI_DOWNLOAD_HOST
  // throws here (inside the guarded path) and gets the friendly error help
  // below, not a raw module-load stack trace.
  const mirrorUrls = getMirrorUrls(process.env);

  fs.mkdirSync(binDir, { recursive: true });

  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "lark-cli-"));
  const archivePath = path.join(tmpDir, archiveName);

  try {
    // Try GitHub first, then walk the mirror chain in order. Stop at the
    // first success. This preserves the "GitHub → npmmirror" safety net
    // even when an unrelated npm_config_registry was set globally and its
    // /-/binary/ path doesn't actually serve our archive.
    let lastErr;
    let downloaded = false;
    for (const url of [GITHUB_URL, ...mirrorUrls]) {
      try {
        download(url, archivePath);
        downloaded = true;
        break;
      } catch (e) {
        lastErr = e;
      }
    }
    if (!downloaded) throw lastErr;

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
      `\nIf you are behind a firewall or in a restricted network, try one of:\n` +
      `  # 1. Use a proxy:\n` +
      `  export https_proxy=http://your-proxy:port\n` +
      `  npm install -g @larksuite/cli\n\n` +
      `  # 2. Point to a corporate npm mirror that proxies /-/binary/lark-cli/...:\n` +
      `  npm install -g @larksuite/cli --registry=https://your-corp-mirror/\n\n` +
      `  # 3. Override the binary download host directly:\n` +
      `  LARK_CLI_DOWNLOAD_HOST=https://your-host npm install -g @larksuite/cli`
    );
    process.exit(1);
  }
}

module.exports = { getExpectedChecksum, verifyChecksum, assertAllowedHost, resolveMirrorUrls };
