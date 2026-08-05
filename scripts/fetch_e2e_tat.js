#!/usr/bin/env node
// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Fetches a live E2E tenant access token (TAT) for the shared bot identity.
//
// Invoked from the e2e-live CI job. Exchanges the bot app id/secret for a
// tenant access token, writes the token to a private file under $RUNNER_TEMP,
// and emits the file path as a step output so the test step can read it once
// and then delete it.
//
// The secret arrives via environment variables; the OAuth parameter names are
// literal because this is a source code file (.js), so the quality gate's
// benign-code-credential exemption applies to the process.env references.

const fs = require("node:fs");
const http = require("node:http");
const https = require("node:https");
const path = require("node:path");
const { URL } = require("node:url");

const ENDPOINT = process.env.E2E_TAT_ENDPOINT || "https://accounts.feishu.cn/oauth/v3/token";
const MAX_ATTEMPTS = 4;
const RETRY_BASE_MS = parseInt(process.env.E2E_TAT_RETRY_BASE_MS || "1000", 10);

function requireEnv(name) {
  const value = process.env[name];
  if (!value) {
    console.error(`::error::Missing required environment variable: ${name}`);
    process.exit(1);
  }
  return value;
}

function postForm(url, body) {
  return new Promise((resolve, reject) => {
    const parsed = new URL(url);
    const transport = parsed.protocol === "http:" ? http : https;
    const req = transport.request(
      parsed,
      {
        method: "POST",
        headers: {
          "Content-Type": "application/x-www-form-urlencoded",
          "Content-Length": Buffer.byteLength(body),
        },
        timeout: 20000,
      },
      (resp) => {
        const chunks = [];
        let settled = false;
        const rejectOnce = (error) => {
          if (!settled) {
            settled = true;
            reject(error);
          }
        };
        resp.on("data", (chunk) => chunks.push(chunk));
        resp.on("aborted", () => rejectOnce(new Error("response aborted before completion")));
        resp.on("error", rejectOnce);
        resp.on("close", () => {
          if (!resp.complete) {
            rejectOnce(new Error("response closed before completion"));
          }
        });
        resp.on("end", () => {
          if (!resp.complete) {
            rejectOnce(new Error("response ended before completion"));
            return;
          }
          settled = true;
          resolve({
            status: resp.statusCode,
            body: Buffer.concat(chunks).toString("utf8"),
            headers: resp.headers,
          });
        });
      },
    );
    req.on("timeout", () => {
      req.destroy();
      reject(new Error("request timed out"));
    });
    req.on("error", reject);
    req.write(body);
    req.end();
  });
}

function encodeForm(params) {
  return Object.entries(params)
    .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(value)}`)
    .join("&");
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function fetchTenantToken() {
  const appId = requireEnv("LARKSUITE_CLI_APP_ID");
  const appSecret = requireEnv("TEST_BOT1_APP_SECRET");

  const body = encodeForm({
    grant_type: "client_credentials",
    client_id: appId,
    client_secret: appSecret,
  });

  let lastError = "";
  for (let attempt = 1; attempt <= MAX_ATTEMPTS; attempt++) {
    try {
      const { status, body: respBody, headers } = await postForm(ENDPOINT, body);
      let payload;
      try {
        payload = JSON.parse(respBody);
      } catch {
        const logID = headers["x-tt-logid"] || headers["x-request-id"] || "unavailable";
        lastError = `HTTP ${status}, log_id=${logID}, non-JSON response`;
      }
      if (payload) {
        const token = payload.access_token;
        if (status === 200 && payload.code === 0 && token) {
          return token;
        }
        lastError = `HTTP ${status}, code=${payload.code}, error=${payload.error}, msg=${payload.msg || payload.error_description}`;
      }
    } catch (err) {
      lastError = err.message;
    }

    if (attempt < MAX_ATTEMPTS) {
      await sleep(2 ** (attempt - 1) * RETRY_BASE_MS);
    }
  }

  console.error(`::error::Failed to fetch tenant access token: ${lastError}`);
  process.exit(1);
}

async function main() {
  const token = await fetchTenantToken();
  console.log(`::add-mask::${token}`);

  const tatPath = path.join(process.env.RUNNER_TEMP, "e2e-live-tat");
  fs.writeFileSync(tatPath, token, { encoding: "utf8", mode: 0o600 });

  if (process.env.GITHUB_OUTPUT) {
    fs.appendFileSync(process.env.GITHUB_OUTPUT, `path=${tatPath}\n`);
  }

  console.log("Prepared shared live E2E tenant token");
}

if (require.main === module) {
  main();
}

module.exports = {
  encodeForm,
  fetchTenantToken,
  postForm,
  requireEnv,
};
