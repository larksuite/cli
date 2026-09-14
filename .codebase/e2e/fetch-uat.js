// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
'use strict';
const { randomUUID } = require('node:crypto');

// Only locally authored messages may cross the CI logging boundary.
class CredentialError extends Error {}

function required(env, name) {
  if (typeof env[name] !== 'string' || !env[name].trim()) throw new CredentialError(`Missing ${name}`);
  return env[name];
}
function positiveInteger(value) {
  if (!/^[1-9][0-9]*$/.test(value) || !Number.isSafeInteger(Number(value))) throw new CredentialError('Invalid BBQ resource ID');
  return Number(value);
}
function baseURL(value) {
  let url;
  try { url = new URL(value); } catch { throw new CredentialError('Expected an HTTPS origin'); }
  if (url.protocol !== 'https:' || url.username || url.password || url.search || url.hash || url.pathname !== '/') {
    throw new CredentialError('Expected an HTTPS origin');
  }
  return url.origin;
}
async function requestJSON(url, options, fetchImpl = fetch, timeoutMs = 240000) {
  let httpStatus;
  try {
    const response = await fetchImpl(url, { ...options, redirect: 'error', signal: AbortSignal.timeout(timeoutMs) });
    if (!response.ok) {
      if (Number.isInteger(response.status)) httpStatus = response.status;
      throw new CredentialError('HTTP failure');
    }
    const chunks = [];
    let bytes = 0;
    for await (const chunk of response.body) {
      bytes += chunk.length;
      if (bytes > 1024 * 1024) throw new CredentialError('Response too large');
      chunks.push(Buffer.from(chunk));
    }
    const body = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    if (!body || typeof body !== 'object' || Array.isArray(body)) throw new CredentialError('Invalid response');
    return body;
  } catch (error) {
    // Fetch/JSON errors can contain URLs, upstream bodies and credentials.
    const networkCode = error?.cause?.code;
    const safeCodes = ['ENOTFOUND', 'ECONNREFUSED', 'ECONNRESET', 'ETIMEDOUT', 'UND_ERR_CONNECT_TIMEOUT', 'CERT_HAS_EXPIRED', 'UNABLE_TO_VERIFY_LEAF_SIGNATURE'];
    const detail = httpStatus ? `HTTP ${httpStatus}` : safeCodes.includes(networkCode) ? networkCode : '';
    throw new CredentialError(`Credential service request failed${detail ? ` (${detail})` : ''}`);
  }
}
async function fetchUAT(env = process.env, fetchImpl = fetch, now = Date.now) {
  const base = baseURL(required(env, 'E2E_BBQ_BASE_URL'));
  const token = required(env, 'E2E_BBQ_ACCESS_TOKEN');
  const appID = required(env, 'TEST_BOT1_APP_ID');
  const appSecret = required(env, 'TEST_BOT1_APP_SECRET');
  const requestID = randomUUID();
  const result = await requestJSON(`${base}/bbk/open_api/v1/script/run`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'x-access-token': token },
    body: JSON.stringify({
      spaceNodeID: positiveInteger(required(env, 'E2E_BBQ_SPACE_NODE_ID')),
      id: positiveInteger(required(env, 'E2E_BBQ_SCRIPT_ID')),
      inputArgs: { APP_ID: appID, APP_SECRET: appSecret, REQUEST_ID: requestID },
    }),
  }, fetchImpl);
  // The platform's outputLog is untrusted and can contain sensitive diagnostics.
  // Only consume the declared, private output parameter from this invocation.
  if (result.code !== 0 || typeof result.data?.outputArgs?.CI_UAT_RESULT !== 'string') {
    throw new CredentialError(`BBQ UAT execution failed${Number.isInteger(result.code) ? ` (code ${result.code})` : ''}`);
  }
  let value;
  try { value = JSON.parse(result.data.outputArgs.CI_UAT_RESULT); } catch { throw new CredentialError('BBQ UAT result missing or invalid'); }
  if (value?.status !== 'ok') {
    const stages = ['configuration', 'account_pool', 'session_code', 'token_exchange', 'scope_verification', 'user_verification'];
    const stage = stages.includes(value?.stage) ? value.stage : 'unknown stage';
    throw new CredentialError(`BBQ UAT authorization failed (${stage})`);
  }
  const seconds = Math.floor(now() / 1000);
  if (value?.version !== 3 || value.request_id !== requestID || value.app_id !== appID ||
      typeof value.user_id !== 'string' || !value.user_id || typeof value.open_id !== 'string' || !value.open_id ||
      typeof value.access_token !== 'string' || !value.access_token || /\s/.test(value.access_token) ||
      !Number.isSafeInteger(value.issued_at) || !Number.isSafeInteger(value.expires_at) ||
      value.issued_at > seconds + 60 || value.issued_at < seconds - 180 || value.expires_at - seconds < 2700) {
    throw new CredentialError('BBQ UAT identity, freshness or lifetime check failed');
  }
  return value;
}
async function verifyIdentities(env, uat, fetchImpl = fetch) {
  const base = 'https://open.feishu.cn';
  const bot = await requestJSON(`${base}/open-apis/bot/v3/info`, {
    headers: { Authorization: `Bearer ${required(env, 'TEST_TENANT_ACCESS_TOKEN')}` },
  }, fetchImpl, 30000);
  if (bot.code !== 0 || !bot.bot?.open_id) throw new CredentialError('Bot service verification failed');
  const user = await requestJSON(`${base}/open-apis/authen/v1/user_info`, {
    headers: { Authorization: `Bearer ${uat.access_token}` },
  }, fetchImpl, 30000);
  if (user.code !== 0 || user.data?.open_id !== uat.open_id || String(user.data?.user_id) !== uat.user_id) {
    throw new CredentialError('User service verification failed');
  }
}
module.exports = { CredentialError, fetchUAT, verifyIdentities, requestJSON, baseURL };
