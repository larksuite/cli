// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const { describe, it } = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const { pathToFileURL } = require("node:url");

const runScript = path.join(__dirname, "run.js");
const unixOnly = { skip: process.platform === "win32" && "fake npm/npx are POSIX shell scripts" };

// The wizard runs in non-interactive mode when stdin is not a TTY, which is
// the case under spawnSync. Fake npm reports the CLI as already installed at
// the latest version so step 1 is skipped, fake npx records every invocation
// so the test can prove whether the skills step ran, and @clack/prompts is
// replaced through a module hook because the script-test job never runs
// npm install.
function makeFixture(t) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "install-wizard-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));

  const fakeBin = path.join(root, "fake-bin");
  const commandLog = path.join(root, "commands.log");
  fs.mkdirSync(fakeBin, { recursive: true });

  const writeExecutable = (file, content) => fs.writeFileSync(file, content, { mode: 0o755 });
  writeExecutable(path.join(fakeBin, "npm"), `#!/bin/sh
case "$1" in
  list) printf '%s\\n' '@larksuite/cli@1.0.0' ;;
  view) printf '%s\\n' '1.0.0' ;;
  prefix) printf '%s\\n' "$FAKE_NPM_PREFIX" ;;
  *) exit 97 ;;
esac
`);
  writeExecutable(path.join(fakeBin, "npx"), `#!/bin/sh
printf 'npx %s\\n' "$*" >> "$COMMAND_TEST_LOG"
printf '%s\\n' 'lark-existing'
`);

  const promptsStub = path.join(root, "prompts-stub.mjs");
  fs.writeFileSync(promptsStub, `
const out = (message) => { if (message) console.log(message); };
export const log = { info: out, success: out, error: out, warn: out, step: out, message: out };
export function spinner() { return { start() {}, stop(message) { out(message); }, message() {} }; }
export function intro(message) { out(message); }
export function outro(message) { out(message); }
export function cancel(message) { out(message); }
export function isCancel() { return false; }
export async function select() { return "en"; }
export async function confirm() { return false; }
`);
  const hooks = path.join(root, "prompts-hooks.mjs");
  fs.writeFileSync(hooks, `
const stub = ${JSON.stringify(pathToFileURL(promptsStub).href)};
export async function resolve(specifier, context, nextResolve) {
  if (specifier === "@clack/prompts") return { url: stub, shortCircuit: true };
  return nextResolve(specifier, context);
}
`);
  const loader = path.join(root, "prompts-loader.mjs");
  fs.writeFileSync(loader, `
import { register } from "node:module";
register(${JSON.stringify(pathToFileURL(hooks).href)});
`);

  return {
    commandLog,
    loader,
    env: {
      ...process.env,
      PATH: `${fakeBin}${path.delimiter}${process.env.PATH}`,
      COMMAND_TEST_LOG: commandLog,
      FAKE_NPM_PREFIX: root,
    },
  };
}

function runInstall(fixture, args) {
  return spawnSync(
    process.execPath,
    ["--import", pathToFileURL(fixture.loader).href, runScript, "install", ...args],
    { env: fixture.env, encoding: "utf8", input: "" }
  );
}

function readCommandLog(fixture) {
  return fs.existsSync(fixture.commandLog) ? fs.readFileSync(fixture.commandLog, "utf8") : "";
}

describe("install wizard skills step", () => {
  it("installs skills by default", unixOnly, (t) => {
    const fixture = makeFixture(t);
    const result = runInstall(fixture, ["--lang", "en"]);

    assert.equal(result.status, 0, result.stderr);
    assert.match(readCommandLog(fixture), /^npx -y skills ls -g$/m);
    assert.match(result.stdout, /Already installed\. Skipped/);
  });

  it("skips skills with --no-skills and never invokes npx", unixOnly, (t) => {
    const fixture = makeFixture(t);
    const result = runInstall(fixture, ["--lang", "en", "--no-skills"]);

    assert.equal(result.status, 0, result.stderr);
    assert.equal(readCommandLog(fixture), "");
    assert.match(result.stdout, /Skipped skills installation \(--no-skills\)/);
    assert.match(result.stdout, /npx skills add larksuite\/cli -y -g/);
  });

  it("localizes the --no-skills notice", unixOnly, (t) => {
    const fixture = makeFixture(t);
    const result = runInstall(fixture, ["--no-skills", "--lang=zh"]);

    assert.equal(result.status, 0, result.stderr);
    assert.equal(readCommandLog(fixture), "");
    assert.match(result.stdout, /已按 --no-skills 跳过 Skills 安装/);
  });
});
