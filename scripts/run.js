#!/usr/bin/env node
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { execFileSync } = require("child_process");
const fs = require("fs");
const path = require("path");

const ext = process.platform === "win32" ? ".exe" : "";
const bin = path.join(__dirname, "..", "bin", "lark-cli" + ext);

const MACHO_MAGICS = new Set([
  0xfeedface,
  0xfeedfacf,
  0xcefaedfe,
  0xcffaedfe,
  0xcafebabe,
  0xbebafeca,
  0xcafebabf,
  0xbfbafeca,
]);

// Return null only for a non-empty native executable for this platform.
// In particular, Linux/glibc treats an empty or shell-like executable as an
// empty /bin/sh script after ENOEXEC, which would otherwise make the shim
// silently exit 0 even though the native binary never ran.
function binaryProblem(filePath) {
  let stat;
  try {
    stat = fs.statSync(filePath);
  } catch (err) {
    return typeof err.code === "string" ? err.code : "UNKNOWN";
  }
  if (!stat.isFile() || stat.size === 0) {
    return "INVALID_BINARY";
  }

  const header = Buffer.alloc(4);
  let fd;
  let bytesRead;
  try {
    fd = fs.openSync(filePath, "r");
    bytesRead = fs.readSync(fd, header, 0, header.length, 0);
  } catch (err) {
    return typeof err.code === "string" ? err.code : "UNKNOWN";
  } finally {
    if (fd !== undefined) {
      fs.closeSync(fd);
    }
  }

  if (process.platform === "win32") {
    return bytesRead >= 2 && header[0] === 0x4d && header[1] === 0x5a
      ? null
      : "INVALID_BINARY";
  }
  if (process.platform === "linux") {
    const isELF =
      bytesRead === 4 &&
      header.equals(Buffer.from([0x7f, 0x45, 0x4c, 0x46]));
    return isELF ? null : "INVALID_BINARY";
  }
  if (process.platform === "darwin") {
    return bytesRead === 4 && MACHO_MAGICS.has(header.readUInt32BE(0))
      ? null
      : "INVALID_BINARY";
  }
  return "INVALID_BINARY";
}

function reportLaunchFailure(reason) {
  console.error(
    `\nlark-cli: failed to launch the native binary.\n` +
    `  path:  ${bin}\n` +
    `  error: ${reason}`
  );
  process.exitCode = 1;
}

// On Windows, a crashed self-update may have left the binary renamed to .old.
// Recover it before proceeding so the CLI remains functional.
const oldBin = bin + ".old";
function restoreOldBinary() {
  try {
    if (fs.existsSync(bin)) {
      fs.rmSync(bin, { force: true });
    }
    fs.renameSync(oldBin, bin);
    return true;
  } catch (_) {
    return false;
  }
}

if (process.platform === "win32" && fs.existsSync(oldBin)) {
  if (!fs.existsSync(bin)) {
    restoreOldBinary();
  } else {
    try {
      execFileSync(bin, ["--version"], { stdio: "ignore", timeout: 10000 });
      try {
        fs.rmSync(oldBin, { force: true });
      } catch (_) {
        // Best-effort cleanup; keep running the healthy binary.
      }
    } catch (_) {
      restoreOldBinary();
    }
  }
}

// Intercept "install" subcommand — run the setup wizard directly,
// bypassing the native binary (which may not exist yet under npx).
const args = process.argv.slice(2);
if (args[0] === "install") {
  require("./install-wizard.js");
} else {
  // Auto-download a missing or invalid binary (e.g. npx skipped postinstall,
  // or an interrupted install left a partial destination behind).
  let problem = binaryProblem(bin);
  if (problem === "ENOENT" || problem === "INVALID_BINARY") {
    try {
      execFileSync(process.execPath, [path.join(__dirname, "install.js")], {
        stdio: "inherit",
        env: { ...process.env, LARK_CLI_RUN: "true" },
      });
    } catch (_) {
      console.error(
        `\nFailed to auto-install lark-cli binary.\n` +
        `To fix, run the install script manually:\n` +
        `  node "${path.join(__dirname, "install.js")}"\n`
      );
      process.exitCode = 1;
      return;
    }

    problem = binaryProblem(bin);
    if (problem !== null) {
      reportLaunchFailure(problem);
      return;
    }
  }

  try {
    execFileSync(bin, args, { stdio: "inherit" });
  } catch (e) {
    // Every branch below that prints a diagnostic returns naturally instead
    // of calling `process.exit(1)`. On POSIX, writes to a piped stderr are asynchronous
    // (unlike a TTY); `process.exit()` tears the process down immediately and
    // can drop a write that has not finished flushing yet. That is exactly
    // what happens when the child has just filled a piped stderr (the
    // AI-agent/log-wrapper case this CLI is built for): `process.exit(1)`
    // right after `console.error(...)` can lose the entire diagnostic.
    // Leaving `process.exitCode` set and returning naturally lets the event
    // loop drain pending writes before the process actually exits, so the
    // diagnostic survives. The silent branches print nothing, so they keep
    // using `process.exit` directly. Numeric statuses can be normal exits or,
    // on Windows, an NTSTATUS; diagnose exceptional NTSTATUS values and
    // preserve every numeric status unchanged.
    if (typeof e.status === "number") {
      // Windows has no signals: native process termination surfaces as an
      // NTSTATUS number here. Any non-zero severity bits (0x40000000+) identify
      // an informational/warning/error termination status outside lark-cli's
      // documented small exit-code range. Exclude STATUS_CONTROL_C_EXIT to stay
      // symmetric with the quiet SIGINT/SIGTERM allowlist.
      const windowsStatus = e.status >>> 0;
      if (
        process.platform === "win32" &&
        windowsStatus >= 0x40000000 &&
        windowsStatus !== 0xc000013a
      ) {
        console.error(
          `\nlark-cli: the native binary crashed (status 0x${windowsStatus.toString(16)}).\n` +
          `  path:  ${bin}`
        );
        // Preserve the raw NTSTATUS for wrappers that use %ERRORLEVEL% to
        // distinguish crash types, while returning naturally to flush stderr.
        process.exitCode = e.status;
        return;
      }
      process.exit(e.status);
    }
    // SIGINT and SIGTERM are the explicit quiet allowlist for intentional
    // interruption (Ctrl+C during `auth login`, for one). Other signals are
    // crash evidence worth surfacing, but do not prove the binary failed to
    // launch. Only print e.signal and the known bin path: e.message and related
    // error fields can contain the caller's full argv.
    if (e.signal) {
      if (e.signal === "SIGINT" || e.signal === "SIGTERM") {
        process.exit(1);
      }
      console.error(
        `\nlark-cli: the native binary was terminated by signal ${e.signal}.\n` +
        `  path:  ${bin}`
      );
      process.exitCode = 1;
      return;
    }
    // Neither: the launch itself failed. Report only what is actually known —
    // permissions, file format, CPU architecture and endpoint policy all land
    // here, and the errno is the only evidence. Print e.code and never
    // e.message: when the child does run, e.message carries the full argv,
    // which can contain values the caller passed on the command line.
    reportLaunchFailure(typeof e.code === "string" ? e.code : "UNKNOWN");
  }
}
