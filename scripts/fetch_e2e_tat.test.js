// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

const assert = require("node:assert/strict");
const fs = require("node:fs");
const http = require("node:http");
const os = require("node:os");
const path = require("node:path");
const { spawn } = require("node:child_process");
const test = require("node:test");

const scriptPath = path.join(__dirname, "fetch_e2e_tat.js");

function startServer(handler) {
  const server = http.createServer((req, res) => {
    let body = "";
    req.on("data", (chunk) => {
      body += chunk;
    });
    req.on("end", () => {
      handler(req, res, body);
    });
  });
  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => {
      const port = server.address().port;
      resolve({ server, port });
    });
  });
}

function abortResponse(res) {
  res.writeHead(200, {
    "Content-Type": "application/json",
    "Content-Length": "100",
  });
  res.write('{"code":0');
  setImmediate(() => res.destroy());
}

function runScript(envOverrides) {
  const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), "fetch-e2e-tat-"));
  const githubOutput = path.join(tmpDir, "github-output");
  const env = {
    ...process.env,
    LARKSUITE_CLI_APP_ID: "test_app_id",
    TEST_BOT1_APP_SECRET: "test-secret",
    RUNNER_TEMP: tmpDir,
    GITHUB_OUTPUT: githubOutput,
    E2E_TAT_RETRY_BASE_MS: "10",
    ...envOverrides,
  };

  return new Promise((resolve) => {
    const child = spawn(process.execPath, [scriptPath], {
      cwd: path.join(__dirname, ".."),
      env,
    });

    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (data) => {
      stdout += data;
    });
    child.stderr.on("data", (data) => {
      stderr += data;
    });
    child.on("close", (code) => {
      const output = fs.existsSync(githubOutput)
        ? fs.readFileSync(githubOutput, "utf8")
        : "";
      resolve({ tmpDir, stdout, stderr, output, exitCode: code });
    });
  });
}

test("encodeForm encodes form parameters", () => {
  const { encodeForm } = require(scriptPath);
  const result = encodeForm({
    grant_type: "client_credentials",
    client_id: "abc&def",
    client_secret: "test-secret",
    note: "x=y",
  });
  const params = new URLSearchParams(result);
  assert.equal(params.get("grant_type"), "client_credentials");
  assert.equal(params.get("client_id"), "abc&def");
  assert.equal(params.get("client_secret"), "test-secret");
  assert.equal(params.get("note"), "x=y");
});

test("exits with error when app id is missing", async () => {
  const result = await runScript({ LARKSUITE_CLI_APP_ID: "" });
  assert.notEqual(result.exitCode, 0);
  assert.match(result.stderr, /Missing required environment variable: LARKSUITE_CLI_APP_ID/);
});

test("exits with error when app secret is missing", async () => {
  const result = await runScript({ TEST_BOT1_APP_SECRET: "" });
  assert.notEqual(result.exitCode, 0);
  assert.match(result.stderr, /Missing required environment variable: TEST_BOT1_APP_SECRET/);
});

test("fetches token and writes it to a private file", async () => {
  const { server, port } = await startServer((req, res, body) => {
    assert.equal(req.method, "POST");
    const params = new URLSearchParams(body);
    assert.equal(params.get("grant_type"), "client_credentials");
    assert.equal(params.get("client_id"), "test_app_id");
    assert.equal(params.get("client_secret"), "test-secret");
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ code: 0, access_token: "test-token" }));
  });

  try {
    const result = await runScript({
      E2E_TAT_ENDPOINT: `http://127.0.0.1:${port}/token`,
    });

    assert.equal(result.exitCode, 0, `stderr: ${result.stderr}`);
    assert.ok(result.stdout.includes("::add-mask::test-token"));
    assert.ok(result.stdout.includes("Prepared shared live E2E tenant token"));

    const tatPath = path.join(result.tmpDir, "e2e-live-tat");
    assert.ok(fs.existsSync(tatPath), "token file should exist");

    const stat = fs.statSync(tatPath);
    assert.equal(stat.mode & 0o777, 0o600, "token file should be owner-only");
    assert.equal(fs.readFileSync(tatPath, "utf8"), "test-token");

    assert.ok(
      result.output.includes(`path=${tatPath}`),
      "should write path to GITHUB_OUTPUT",
    );
  } finally {
    server.close();
  }
});

test("retries an interrupted response and then succeeds", async () => {
  let requestCount = 0;
  const { server, port } = await startServer((req, res) => {
    requestCount++;
    if (requestCount === 1) {
      abortResponse(res);
      return;
    }
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ code: 0, access_token: "test-token" }));
  });

  try {
    const result = await runScript({
      E2E_TAT_ENDPOINT: `http://127.0.0.1:${port}/token`,
    });

    assert.equal(result.exitCode, 0, `stderr: ${result.stderr}`);
    assert.equal(requestCount, 2);
  } finally {
    server.close();
  }
});

test("fails after every interrupted response is retried", async () => {
  let requestCount = 0;
  const { server, port } = await startServer((req, res) => {
    requestCount++;
    abortResponse(res);
  });

  try {
    const result = await runScript({
      E2E_TAT_ENDPOINT: `http://127.0.0.1:${port}/token`,
    });

    assert.notEqual(result.exitCode, 0);
    assert.equal(requestCount, 4);
    assert.match(result.stderr, /Failed to fetch tenant access token/);
  } finally {
    server.close();
  }
});

test("exits with error after all retries fail", async () => {
  let requestCount = 0;
  const { server, port } = await startServer((req, res) => {
    requestCount++;
    res.writeHead(500, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ code: 500, error: "server error" }));
  });

  try {
    const result = await runScript({
      E2E_TAT_ENDPOINT: `http://127.0.0.1:${port}/token`,
    });

    assert.notEqual(result.exitCode, 0);
    assert.equal(requestCount, 4);
    assert.match(result.stderr, /Failed to fetch tenant access token/);
  } finally {
    server.close();
  }
});
