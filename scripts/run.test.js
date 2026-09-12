// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("fs");
const os = require("os");
const path = require("path");
const { spawnSync, spawn, execFileSync } = require("child_process");

const MARKER = "failed to launch the native binary";
const IS_WINDOWS = process.platform === "win32";
const IS_ROOT =
  !IS_WINDOWS && typeof process.getuid === "function" && process.getuid() === 0;
const BIN_NAME = "lark-cli" + (IS_WINDOWS ? ".exe" : "");
const SENTINEL = "--sentinel=LEAKCANARY";
const SENTINEL_TOKEN = "LEAKCANARY";
// Single-line, easily-flippable platform gate for the win32 NTSTATUS tests
// below (spec 6.2 row 1b). To confirm their assertions are real (not a
// vacuous skip), temporarily hardcode `true` here on a non-Windows host and
// run the suite: the test then executes for real, and it MUST fail, because
// run.js still checks the real process.platform (so its crash branch stays
// dark) and this OS truncates process.exit() codes to 8 bits before Node
// ever sees them (for example, 0xC0000005 -> 5). Restore this line to
// `IS_WINDOWS` before committing.
const IS_WIN32_CRASH_TESTABLE = IS_WINDOWS;

// run.js resolves the native binary relative to its own directory
// (<dir>/../bin/lark-cli), so a fixture needs a full scripts/ + bin/ layout.
// Building that under fs.mkdtempSync keeps the developer's real ./bin/lark-cli
// untouched and keeps the location unpredictable. Never replace this with a
// fixed path such as %TEMP%\lark-cli-test — dropping a copy of the node
// executable into a predictable user-writable directory changes Windows' DLL
// search order.
function makeSandbox(t) {
  // fs.realpathSync canonicalises the temp root. assertLaunchFailure checks
  // res.stderr.includes(sandbox.bin), but the path run.js prints comes from
  // __dirname, and Node canonicalises a main module's path. On macOS the
  // difference is /var -> /private/var, a prefix extension, so substring
  // containment still holds and the tests pass unnoticed. On Windows it is
  // NOT a prefix extension: libuv's realpath uses GetFinalPathNameByHandle
  // with FILE_NAME_NORMALIZED, which expands 8.3 short names, and
  // GitHub-hosted Windows runners set TEMP to
  // C:\Users\RUNNER~1\AppData\Local\Temp while the profile directory is
  // runneradmin. Short-name expansion is a substitution, not a prefix
  // extension, so includes() returns false and every launch-failure case
  // fails on the one platform the shim-test-windows job exists to cover. Do
  // not "simplify" this back to the bare mkdtempSync result.
  const root = fs.realpathSync(
    fs.mkdtempSync(path.join(os.tmpdir(), "lark-cli-run-"))
  );
  t.after(() =>
    fs.rmSync(root, {
      recursive: true,
      force: true,
      maxRetries: 5,
      retryDelay: 100,
    })
  );
  fs.mkdirSync(path.join(root, "scripts"));
  fs.mkdirSync(path.join(root, "bin"));
  fs.copyFileSync(
    path.join(__dirname, "run.js"),
    path.join(root, "scripts", "run.js")
  );
  // run.js runs install.js when the binary is missing. A stub that exits 0
  // without producing one lets the "installer succeeded but produced nothing"
  // path reach the launch attempt instead of stopping earlier.
  const installJs = path.join(root, "scripts", "install.js");
  fs.writeFileSync(installJs, "process.exit(0);\n");
  return {
    root,
    bin: path.join(root, "bin", BIN_NAME),
    installJs,
    runJs: path.join(root, "scripts", "run.js"),
  };
}

function runShim(sandbox, args) {
  return spawnSync(process.execPath, [sandbox.runJs, ...args], {
    encoding: "utf8",
  });
}

// A fixture that actually launches: a copy of the running node executable.
// A #!/bin/sh script cannot be started as lark-cli.exe on Windows, which would
// make every "the binary ran" case inert there.
function installRunnableBinary(sandbox) {
  fs.copyFileSync(process.execPath, sandbox.bin);
  if (!IS_WINDOWS) {
    fs.chmodSync(sandbox.bin, 0o755);
  }
}

// `--` stops node from claiming the sentinel as one of its own options; without
// it node bails out with "bad option" and exits 9, which would silently
// invalidate the fixture instead of exercising the intended exit status.
function runRanBinary(sandbox, code) {
  return runShim(sandbox, ["-e", code, "--", SENTINEL]);
}

function assertBinaryRan(res) {
  assert.ok(
    !res.stderr.includes(MARKER),
    `launch-failure diagnostic on a binary that ran: ${res.stderr}`
  );
  assert.ok(
    !res.stderr.includes(SENTINEL_TOKEN),
    `caller arguments leaked into stderr: ${res.stderr}`
  );
}

function assertLaunchFailure(res, sandbox) {
  assert.equal(res.status, 1);
  assert.equal(res.stdout, "", `stdout should stay empty: ${res.stdout}`);
  assert.ok(
    res.stderr.includes(MARKER),
    `missing diagnostic, stderr was: ${res.stderr}`
  );
  assert.ok(
    res.stderr.includes(sandbox.bin),
    `diagnostic omits the binary path: ${res.stderr}`
  );
  const line = res.stderr.match(/^ {2}error: (.+)$/m);
  assert.ok(line, `diagnostic omits an error line: ${res.stderr}`);
  const reason = line[1].trim();
  assert.notEqual(reason, "", "error line is empty");
  // Keep failure output factual and local. A launch failure is not necessarily
  // a lark-cli bug, and prompting users to paste local paths into an issue adds
  // noise and may expose machine-specific information.
  assert.ok(
    !res.stderr.includes("github.com/larksuite/cli/issues"),
    `diagnostic should not prompt issue creation: ${res.stderr}`
  );
  assert.ok(
    !res.stderr.includes("Please include"),
    `diagnostic should not ask users to share local details: ${res.stderr}`
  );
  // The sentinel check for launch-failure cases is not redundant with the
  // check in assertBinaryRan: when the launch fails, e.message contains only
  // the errno, not the full argv (measured: String(e) is
  // "Error: spawnSync <path> ENOENT" and e.path is just the bin path --
  // neither carries argv). However, the error object also carries
  // e.spawnargs, which holds the complete argument list even on launch
  // failure. This assertion catches any future edit that prints e.spawnargs
  // or the whole error object.
  assert.ok(
    !res.stderr.includes(SENTINEL_TOKEN),
    `caller arguments leaked into stderr: ${res.stderr}`
  );
  return reason;
}

describe("run.js launch-failure diagnostics", () => {
  for (const [name, content] of [
    ["zero-byte", Buffer.alloc(0)],
    ["whitespace-only", Buffer.from("    \n")],
  ]) {
    it(`rejects a ${name} binary instead of silently succeeding`, (t) => {
      const sandbox = makeSandbox(t);
      fs.writeFileSync(sandbox.bin, content);
      if (!IS_WINDOWS) {
        fs.chmodSync(sandbox.bin, 0o755);
      }

      const res = runShim(sandbox, ["--version", SENTINEL]);

      assert.equal(assertLaunchFailure(res, sandbox), "INVALID_BINARY");
    });
  }

  it("rejects a non-file binary path", (t) => {
    const sandbox = makeSandbox(t);
    fs.mkdirSync(sandbox.bin);

    const res = runShim(sandbox, ["--version", SENTINEL]);

    assert.equal(assertLaunchFailure(res, sandbox), "INVALID_BINARY");
  });

  it(
    "reports a binary that cannot be executed",
    {
      skip: IS_WINDOWS
        ? "POSIX execute bit"
        : IS_ROOT
        ? "mode 0000 does not deny exec for uid 0, so this fixture cannot " +
          "produce EACCES when running as root"
        : false,
    },
    (t) => {
      const sandbox = makeSandbox(t);
      fs.writeFileSync(sandbox.bin, "#!/bin/sh\nexit 0\n");
      fs.chmodSync(sandbox.bin, 0o000);

      const res = runShim(sandbox, ["--version", SENTINEL]);

      assert.equal(assertLaunchFailure(res, sandbox), "EACCES");
    }
  );

  it("reports a missing binary when the installer produced nothing", (t) => {
    const sandbox = makeSandbox(t);

    const res = runShim(sandbox, ["--version", SENTINEL]);

    assert.equal(assertLaunchFailure(res, sandbox), "ENOENT");
  });

  it("reports an auto-install process failure", (t) => {
    const sandbox = makeSandbox(t);
    fs.writeFileSync(sandbox.installJs, "process.exit(7);\n");

    const res = runShim(sandbox, ["--version", SENTINEL]);

    assert.equal(res.status, 1);
    assert.equal(res.stdout, "", `stdout should stay empty: ${res.stdout}`);
    assert.ok(
      res.stderr.includes("Failed to auto-install lark-cli binary"),
      `missing auto-install diagnostic: ${res.stderr}`
    );
    assert.ok(
      res.stderr.includes(sandbox.installJs),
      `auto-install diagnostic omits the install path: ${res.stderr}`
    );
    assert.ok(
      !res.stderr.includes(SENTINEL_TOKEN),
      `caller arguments leaked into stderr: ${res.stderr}`
    );
  });
});

describe("run.js pass-through when the binary runs", () => {
  it("forwards a non-zero exit status unchanged", (t) => {
    const sandbox = makeSandbox(t);
    installRunnableBinary(sandbox);

    const res = runRanBinary(sandbox, "process.exit(7)");

    assert.equal(res.status, 7);
    assertBinaryRan(res);
  });

  for (const signal of ["SIGINT", "SIGTERM"]) {
    it(
      `exits 1 without a diagnostic when the binary receives ${signal}`,
      { skip: IS_WINDOWS ? "POSIX signals" : false },
      (t) => {
        const sandbox = makeSandbox(t);
        installRunnableBinary(sandbox);

        const res = runRanBinary(
          sandbox,
          `process.kill(process.pid, '${signal}')`
        );

        assert.equal(res.status, 1);
        assert.equal(res.stdout, "", `stdout should stay empty: ${res.stdout}`);
        assert.equal(res.stderr, "", `stderr should stay empty: ${res.stderr}`);
        assertBinaryRan(res);
      }
    );
  }

  it(
    "reports the signal when the binary crashes",
    { skip: IS_WINDOWS ? "POSIX signals" : false },
    (t) => {
      const sandbox = makeSandbox(t);
      installRunnableBinary(sandbox);

      const res = runRanBinary(sandbox, "process.kill(process.pid, 'SIGKILL')");

      const expected =
        `\nlark-cli: the native binary was terminated by signal SIGKILL.\n` +
        `  path:  ${sandbox.bin}\n`;
      assert.equal(res.status, 1);
      assert.equal(res.stdout, "", `stdout should stay empty: ${res.stdout}`);
      assert.equal(res.stderr, expected);
      assert.ok(
        !res.stderr.includes(MARKER),
        `crash signal was misreported as a launch failure: ${res.stderr}`
      );
      assert.ok(
        !res.stderr.includes(SENTINEL_TOKEN),
        `caller arguments leaked into stderr: ${res.stderr}`
      );
    }
  );

  for (const [name, status] of [
    ["STATUS_FATAL_APP_EXIT", 0x40000015],
    ["STATUS_BREAKPOINT", 0x80000003],
    ["STATUS_ACCESS_VIOLATION", 0xc0000005],
  ]) {
    it(
      `reports ${name} and preserves its win32 NTSTATUS`,
      {
        skip: IS_WIN32_CRASH_TESTABLE
          ? false
          : "win32-only: POSIX truncates process exit codes to 8 bits, " +
            "so NTSTATUS passthrough cannot be reproduced outside win32",
      },
      (t) => {
        const sandbox = makeSandbox(t);
        installRunnableBinary(sandbox);

        const res = runRanBinary(sandbox, `process.exit(${status})`);

        assert.equal(res.status, status, "the raw NTSTATUS must be preserved");
        assert.equal(res.stdout, "", `stdout should stay empty: ${res.stdout}`);
        assert.equal(
          res.stderr,
          `\nlark-cli: the native binary crashed (status 0x${status.toString(16)}).\n` +
            `  path:  ${sandbox.bin}\n`
        );
        assert.ok(
          !res.stderr.includes(MARKER),
          `win32 crash was misreported as a launch failure: ${res.stderr}`
        );
        assert.ok(
          !res.stderr.includes(SENTINEL_TOKEN),
          `caller arguments leaked into stderr: ${res.stderr}`
        );
      }
    );
  }

  it(
    "keeps STATUS_CONTROL_C_EXIT quiet and preserves its win32 NTSTATUS",
    {
      skip: IS_WIN32_CRASH_TESTABLE
        ? false
        : "win32-only: POSIX truncates NTSTATUS values to 8 bits",
    },
    (t) => {
      const sandbox = makeSandbox(t);
      installRunnableBinary(sandbox);

      const status = 0xc000013a;
      const res = runRanBinary(sandbox, `process.exit(${status})`);

      assert.equal(res.status, status, "the raw NTSTATUS must be preserved");
      assert.equal(res.stdout, "", `stdout should stay empty: ${res.stdout}`);
      assert.equal(res.stderr, "", `stderr should stay empty: ${res.stderr}`);
      assertBinaryRan(res);
    }
  );

  it("passes stdout and stderr through and exits 0", (t) => {
    const sandbox = makeSandbox(t);
    installRunnableBinary(sandbox);

    const res = runRanBinary(
      sandbox,
      "console.log('shim-passthrough'); console.error('shim-stderr')"
    );

    assert.equal(res.status, 0);
    assert.match(res.stdout, /shim-passthrough/);
    // A shim that swallowed child stderr would be a variant of the very bug
    // this branch repairs (issue #2053: launch failures with no evidence).
    // This does not collide with assertBinaryRan's checks below: it asserts
    // MARKER and the argv sentinel are absent from stderr, and "shim-stderr"
    // is neither.
    assert.match(res.stderr, /shim-stderr/);
    assertBinaryRan(res);
  });
});

// Regression for the flush-safety fix: `process.exitCode = 1` + natural
// return (not `process.exit(1)`) after the diagnostic console.error() calls.
// Writes to a piped stderr are asynchronous on POSIX; `process.exit()` can
// tear the process down before that write reaches the pipe. This is the
// exact scenario an AI agent / log wrapper hits: it captures the shim's
// stderr through a pipe, the child fills it, and the diagnostic must still
// arrive.
//
// To make this deterministic, the fixture is killed from OUTSIDE (by this
// test, via its discovered pid) while it is still blocked mid-write on a
// completely unread pipe, instead of self-signalling. Measured: if the
// fixture kills itself after its own blocking write loop returns, that
// return is proof room just opened up, so the diagnostic that follows
// almost always finds room too and the bug never reproduces -- on both
// Linux and macOS, self-signalling passed 100% regardless of
// process.exit(1) vs process.exitCode, i.e. it never actually exercised the
// backpressure path. An external kill has no such tell: the fixture can be
// stopped while genuinely full, so run.js's own diagnostic write is
// guaranteed to need the async path this fix protects.
//
// Linux-only: this exact harness reproduces the drop 100% reliably on Linux
// (matching the script-test CI job, ubuntu-latest, and the AI-agent/
// container use case this fix targets). On macOS the underlying run.js fix
// is not in question -- a separate check confirmed the fixture genuinely
// blocks on a full, unread pipe through run.js's own execFileSync/"inherit"
// chain, same as Linux -- but child_process's "close" event was observed to
// fire as soon as the shim process exits, before this test's reader had
// drained (or in some runs, before it had even attached), independent of
// process.exit(1) vs process.exitCode. That is a difference in how Node's
// child_process "close" event interacts with unread pipe backlog once the
// writer has exited (epoll vs kqueue), not evidence about run.js's
// correctness, so it is not treated as a platform for this regression test.
const BACKPRESSURE_TEST_OPTIONS = {
  skip:
    process.platform === "linux"
      ? false
      : "Linux-only: this harness's determinism relies on Linux pipe/close semantics",
  timeout: 15000,
};

// Bigger than any realistic OS pipe buffer (measured ~256KB on the Linux CI
// container). The child is killed while blocked in this write, before it can
// return and create room for the shim's diagnostic.
const BLOCKING_CHILD_CODE =
  "const fs = require('fs');" +
  "const buf = Buffer.alloc(5000000, 0x78);" +
  "let off = 0;" +
  "while (off < buf.length) { off += fs.writeSync(2, buf, off, buf.length - off); }" +
  "console.error('unreachable: should have been killed first');";

function expectDiagnosticUnderBackpressure(
  t,
  sandbox,
  shimArgs,
  expectedDiagnostic,
  expectedPath
) {
  return new Promise((resolve, reject) => {
    const shim = spawn(process.execPath, [sandbox.runJs, ...shimArgs], {
      stdio: ["ignore", "ignore", "pipe"],
      // A separate process group lets the cleanup hook stop both the shim and
      // its blocked child on every rejection or timeout path.
      detached: true,
    });
    t.after(() => {
      try {
        process.kill(-shim.pid, "SIGKILL");
      } catch (err) {
        if (err.code !== "ESRCH") {
          throw err;
        }
      }
    });

    // Attach up front: a broken implementation may exit before the reader is
    // attached, which is precisely the failure this helper must observe.
    const closed = new Promise((res) => shim.on("close", res));
    shim.on("error", reject);

    setTimeout(() => {
      let fixturePid = null;
      try {
        const out = execFileSync("pgrep", ["-P", String(shim.pid)])
          .toString()
          .trim();
        fixturePid = out ? Number(out.split("\n")[0]) : null;
      } catch (err) {
        reject(new Error(`could not discover the fixture pid: ${err.message}`));
        return;
      }
      if (!fixturePid) {
        reject(new Error("fixture pid not found under the shim"));
        return;
      }
      process.kill(fixturePid, "SIGKILL");

      // Keep the pipe unread while run.js queues its diagnostic, then drain it.
      setTimeout(() => {
        let stderr = "";
        shim.stderr.on("data", (chunk) => {
          stderr += chunk;
        });
        closed.then((code) => {
          try {
            assert.equal(code, 1);
            assert.ok(
              stderr.includes(expectedDiagnostic),
              `diagnostic dropped under stderr backpressure ` +
                `(captured ${stderr.length} bytes): ` +
                JSON.stringify(stderr.slice(-300))
            );
            assert.ok(
              stderr.includes(expectedPath),
              `diagnostic omits its relevant path: ${stderr.slice(-300)}`
            );
            assert.ok(
              !stderr.includes(SENTINEL_TOKEN),
              "caller arguments leaked into stderr under backpressure"
            );
            resolve();
          } catch (err) {
            reject(err);
          }
        }, reject);
      }, 300);
    }, 300);
  });
}

describe("run.js flush safety under stderr backpressure", () => {
  it(
    "keeps the crash-signal diagnostic when stderr is a slow-draining pipe",
    BACKPRESSURE_TEST_OPTIONS,
    (t) => {
      const sandbox = makeSandbox(t);
      installRunnableBinary(sandbox);

      return expectDiagnosticUnderBackpressure(
        t,
        sandbox,
        ["-e", BLOCKING_CHILD_CODE, "--", SENTINEL],
        "the native binary was terminated by signal SIGKILL",
        sandbox.bin
      );
    }
  );

  it(
    "keeps the auto-install diagnostic when stderr is a slow-draining pipe",
    BACKPRESSURE_TEST_OPTIONS,
    (t) => {
      const sandbox = makeSandbox(t);
      fs.writeFileSync(sandbox.installJs, BLOCKING_CHILD_CODE);

      return expectDiagnosticUnderBackpressure(
        t,
        sandbox,
        ["--version", SENTINEL],
        "Failed to auto-install lark-cli binary",
        sandbox.installJs
      );
    }
  );
});
