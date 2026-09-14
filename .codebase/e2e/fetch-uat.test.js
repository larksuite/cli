// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { fetchUAT, verifyIdentities, requestJSON, baseURL } = require('./fetch-uat');
const env = { E2E_BBQ_BASE_URL: 'https://bbq.example.test', E2E_BBQ_ACCESS_TOKEN: 'test-bbq-credential', E2E_BBQ_SPACE_NODE_ID: '97', E2E_BBQ_SCRIPT_ID: '123', TEST_BOT1_APP_ID: 'cli_test', TEST_BOT1_APP_SECRET: 'test-app-secret' };
const now = () => 2000000000000;
function mock(mutate = () => {}) {
  return async (url, options) => {
    assert.equal(url, 'https://bbq.example.test/bbk/open_api/v1/script/run');
    assert.equal(options.method, 'POST'); assert.equal(options.redirect, 'error');
    assert.equal(options.headers['x-access-token'], env.E2E_BBQ_ACCESS_TOKEN);
    const body = JSON.parse(options.body);
    assert.equal(body.spaceNodeID, 97); assert.equal(body.id, 123);
    assert.equal(body.inputArgs.APP_ID, env.TEST_BOT1_APP_ID); assert.equal(body.inputArgs.APP_SECRET, env.TEST_BOT1_APP_SECRET);
    const value = { version: 3, status: 'ok', request_id: body.inputArgs.REQUEST_ID, app_id: 'cli_test', user_id: 'employee', open_id: 'ou_test', access_token: 'test-uat-value', issued_at: 2000000000, expires_at: 2000007200 };
    mutate(value);
    return new Response(JSON.stringify({ code: 0, data: { outputLog: 'never-relay-this-secret', outputArgs: { CI_UAT_RESULT: JSON.stringify(value) } } }));
  };
}
test('calls OpenAPI with this request ID and returns private output', async () => {
  assert.equal((await fetchUAT(env, mock(), now)).access_token, 'test-uat-value');
});
for (const [label, change] of Object.entries({ expired: { expires_at: 2000000000 }, short: { expires_at: 2000002699 }, stale: { issued_at: 1999999800 }, future: { issued_at: 2000000070 }, application: { app_id: 'another-app' }, invocation: { request_id: 'another-run' }, empty: { access_token: '' }, whitespace: { access_token: 'invalid\ntoken' }, wrongType: { expires_at: '2000007200' }, missingUser: { user_id: '' }, missingOpenID: { open_id: '' }, version: { version: 4 } })) {
  test(`rejects ${label} response`, async () => { await assert.rejects(fetchUAT(env, mock(v => Object.assign(v, change)), now), /identity, freshness or lifetime/); });
}
test('never parses or relays outputLog on execution failure', async () => {
  for (const response of [{ code: 1, message: 'secret' }, { code: 0, data: { outputLog: 'secret', outputArgs: { CI_UAT_RESULT: '' } } }]) {
    await assert.rejects(fetchUAT(env, async () => new Response(JSON.stringify(response)), now), e => !e.message.includes('secret'));
  }
});
test('concurrent calls have distinct invocation IDs', async () => {
  const ids = []; const f = mock();
  const wrapped = async (url, opts) => { ids.push(JSON.parse(opts.body).inputArgs.REQUEST_ID); return f(url, opts); };
  await Promise.all([fetchUAT(env, wrapped, now), fetchUAT(env, wrapped, now)]); assert.notEqual(ids[0], ids[1]);
});
test('requires HTTPS origin and resource IDs before network calls', async () => {
  for (const base of ['http://bbq.example.test', 'https://user:password@bbq.example.test', 'https://bbq.example.test/other']) assert.throws(() => baseURL(base));
  for (const changes of [{ E2E_BBQ_ACCESS_TOKEN: '' }, { E2E_BBQ_SCRIPT_ID: '0' }, { E2E_BBQ_SCRIPT_ID: '1e3' }]) {
    await assert.rejects(fetchUAT({ ...env, ...changes }, () => assert.fail('must not call network')));
  }
});
test('HTTP, JSON and network errors never disclose upstream data', async () => {
  for (const fn of [async () => new Response('secret', { status: 500 }), async () => new Response('secret'), async () => { throw new Error('secret'); }]) {
    await assert.rejects(requestJSON('https://example.test', {}, fn), error => /^Credential service request failed(?: \(HTTP 500\))?$/.test(error.message));
  }
});
test('request has an abort deadline', async () => {
  const timer = setTimeout(() => {}, 1000);
  try { await assert.rejects(requestJSON('https://example.test', {}, async (_, opts) => new Promise((resolve, reject) => {
    opts.signal.addEventListener('abort', () => reject(new Error('secret')), { once: true });
  }), 5), /Credential service request failed/); } finally { clearTimeout(timer); }
});
test('both service identities require successful business responses', async () => {
  const uat = { access_token: 'uat', user_id: 'employee', open_id: 'ou_user' }; const calls = [];
  await verifyIdentities({ TEST_TENANT_ACCESS_TOKEN: 'tat' }, uat, async (url, options) => {
    calls.push(options.headers.Authorization);
    return new Response(JSON.stringify(url.endsWith('/info') ? { code: 0, bot: { open_id: 'ou_bot' } } : { code: 0, data: { user_id: 'employee', open_id: 'ou_user' } }));
  });
  assert.deepEqual(calls, ['Bearer tat', 'Bearer uat']);
  await assert.rejects(verifyIdentities({ TEST_TENANT_ACCESS_TOKEN: 'tat' }, uat, async () => new Response(JSON.stringify({ code: 9999, bot: { open_id: 'ou_bot' } }))), /Bot service/);
});

test('successful execution with a failed private authorization result is rejected', async () => {
  await assert.rejects(fetchUAT(env, mock(value => { value.status = 'error'; }), now), /BBQ UAT authorization failed/);
});

test('HTTP diagnostics include only status, never upstream text', async () => {
  await assert.rejects(requestJSON('https://example.test', {}, async () => new Response('SECRET_BODY', { status: 403 })), { message: 'Credential service request failed (HTTP 403)' });
});

test('diagnostic fields are allowlisted and cannot inject secrets', async () => {
  await assert.rejects(fetchUAT(env, mock(v => { v.status='error'; v.stage='SECRET'; }), now), { message:'BBQ UAT authorization failed (unknown stage)' });
  await assert.rejects(requestJSON('https://example.test', {}, async () => { throw new Error('SECRET', { cause:{ code:'ENOTFOUND' } }); }), { message:'Credential service request failed (ENOTFOUND)' });
  await assert.rejects(requestJSON('https://example.test', {}, async () => { throw new Error('SECRET', { cause:{ code:'SECRET' } }); }), { message:'Credential service request failed' });
});
