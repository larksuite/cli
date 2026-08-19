// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const test = require("node:test");
const { pathToFileURL } = require("node:url");

const runScript = path.join(__dirname, "run.js");
const unixOnly = { skip: process.platform === "win32" };

function writeExecutable(file, content) {
  fs.writeFileSync(file, content, { mode: 0o755 });
}

function makeFixture(t, cliExitCode = 0) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "install-wizard-"));
  const fakeBin = path.join(root, "fake-bin");
  const prefix = path.join(root, "npm-prefix");
  const commandLog = path.join(root, "commands.log");
  const promptsStub = path.join(root, "prompts-stub.mjs");
  const loader = path.join(root, "prompts-loader.mjs");
  fs.mkdirSync(fakeBin, { recursive: true });
  fs.mkdirSync(path.join(prefix, "bin"), { recursive: true });

  fs.writeFileSync(promptsStub, `
export const log = { info() {}, success() {}, warn() {}, error() {}, step() {} };
export function spinner() { return { start() {}, stop(message) { console.error(message); } }; }
export function cancel(message) { console.error(message); }
export function isCancel() { return false; }
`);
  fs.writeFileSync(loader, `
const stub = new URL("./prompts-stub.mjs", import.meta.url).href;
export async function resolve(specifier, context, nextResolve) {
  if (specifier === "@clack/prompts") return { url: stub, shortCircuit: true };
  return nextResolve(specifier, context);
}
`);

  writeExecutable(path.join(fakeBin, "npm"), `#!/bin/sh
case "$1" in
  list) printf '%s\n' '@larksuite/cli@1.0.0' ;;
  view) printf '%s\n' '1.0.0' ;;
  prefix) printf '%s\n' "$FAKE_NPM_PREFIX" ;;
  *) exit 97 ;;
esac
`);
  writeExecutable(path.join(fakeBin, "npx"), `#!/bin/sh
printf 'npx:%s\n' "$*" >> "$COMMAND_TEST_LOG"
printf '%s\n' 'lark-existing'
`);
  writeExecutable(path.join(prefix, "bin", "lark-cli"), `#!/bin/sh
printf 'lark-cli:%s\n' "$*" >> "$COMMAND_TEST_LOG"
exit "$LARK_CLI_EXIT_CODE"
`);

  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  return {
    commandLog,
    env: {
      ...process.env,
      PATH: `${fakeBin}${path.delimiter}${process.env.PATH}`,
      FAKE_NPM_PREFIX: prefix,
      LARK_CLI_EXIT_CODE: String(cliExitCode),
      COMMAND_TEST_LOG: commandLog,
      NO_COLOR: "1",
      NODE_OPTIONS: `--no-warnings --experimental-loader=${pathToFileURL(loader).href}`,
    },
  };
}

function runInstall(args, env) {
  return spawnSync(process.execPath, [runScript, "install", "--lang", "en", ...args], {
    cwd: path.join(__dirname, ".."),
    encoding: "utf8",
    env,
  });
}

function readIfPresent(file) {
  return fs.existsSync(file) ? fs.readFileSync(file, "utf8") : "";
}

for (const [name, args, layout] of [
  ["suite layout", ["--skills-layout", "suite"], "suite"],
  ["equals form for separate layout", ["--skills-layout=separate"], "separate"],
]) {
  test(`install applies ${name} even when skills already exist`, unixOnly, (t) => {
    const fixture = makeFixture(t);
    const result = runInstall(args, fixture.env);

    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.equal(readIfPresent(fixture.commandLog), `lark-cli:update --skills-layout ${layout}\n`);
  });
}

test("install without a layout keeps the existing skills skip behavior", unixOnly, (t) => {
  const fixture = makeFixture(t);
  const result = runInstall([], fixture.env);

  assert.equal(result.status, 0, result.stdout + result.stderr);
  assert.equal(readIfPresent(fixture.commandLog), "npx:-y skills ls -g\n");
});

test("install rejects an unsupported skills layout before changing skills", unixOnly, (t) => {
  const fixture = makeFixture(t);
  const result = runInstall(["--skills-layout", "hybrid"], fixture.env);

  assert.equal(result.status, 1);
  assert.match(result.stdout + result.stderr, /--skills-layout must be one of separate or suite/);
  assert.equal(readIfPresent(fixture.commandLog), "");
});

test("install reports the layout-specific retry command when synchronization fails", unixOnly, (t) => {
  const fixture = makeFixture(t, 9);
  const result = runInstall(["--skills-layout", "suite"], fixture.env);

  assert.equal(result.status, 1);
  assert.match(result.stdout + result.stderr, /lark-cli update --skills-layout suite/);
  assert.equal(readIfPresent(fixture.commandLog), "lark-cli:update --skills-layout suite\n");
});
