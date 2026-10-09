// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
'use strict';
const { CredentialError, requestJSON } = require('./fetch-uat');

function credentials(env) {
  const appID = env.E2E_AAT_APP_ID;
  const appSecret = env.E2E_AAT_APP_SECRET;
  if (!appID || !appSecret) throw new CredentialError('Missing dedicated AAT application credentials');
  if (appID === env.TEST_BOT1_APP_ID) throw new CredentialError('AAT application must be separate from UAT/TAT');
  return { client_id: appID, client_secret: appSecret };
}
async function introspectAAT(env, token, expectedOpenID, fetchImpl = fetch, now = Date.now) {
  const result = await requestJSON('https://accounts.feishu.cn/oauth/v1/introspect', {
    method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({ ...credentials(env), token }).toString(),
  }, fetchImpl, 30000);
  if (result.code && result.code !== 0 || result.error || result.active !== true ||
      result.client_id !== env.E2E_AAT_APP_ID || result.user_type !== 66 ||
      typeof result.open_id !== 'string' || !result.open_id ||
      !Number.isSafeInteger(result.exp) || result.exp <= Math.floor(now() / 1000) + 60 ||
      expectedOpenID && result.open_id !== expectedOpenID) {
    throw new CredentialError('AAT identity, application or lifetime verification failed');
  }
  return { app_id: result.client_id, open_id: result.open_id, principal_type: 'agent_employee', credential_type: 'aat', expires_at: result.exp };
}
async function fetchAAT(env = process.env, fetchImpl = fetch, now = Date.now) {
  const result = await requestJSON('https://accounts.feishu.cn/oauth/v3/token', {
    method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    body: new URLSearchParams({ ...credentials(env), grant_type: 'client_credentials' }).toString(),
  }, fetchImpl, 30000);
  if (result.code !== 0 || typeof result.access_token !== 'string' || !result.access_token || /\s/.test(result.access_token)) {
    throw new CredentialError('AAT token acquisition failed');
  }
  const identity = await introspectAAT(env, result.access_token, undefined, fetchImpl, now);
  if (identity.expires_at - Math.floor(now() / 1000) < 15 * 60) throw new CredentialError('Insufficient AAT lifetime for selected workflows');
  return { token: result.access_token, identity };
}
module.exports = { fetchAAT, introspectAAT };
