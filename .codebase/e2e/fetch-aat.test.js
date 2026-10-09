// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
const { test } = require('node:test');
const assert = require('node:assert/strict');
const { fetchAAT, introspectAAT } = require('./fetch-aat');
const env = { E2E_AAT_APP_ID: 'cli_dedicated', E2E_AAT_APP_SECRET: 'private-test-secret' };
const now = () => 2000000000000;
const valid = { active: true, client_id: 'cli_dedicated', user_type: 66, open_id: 'ou_agent', exp: 2000007200 };
function mock(change = {}) {
  return async (url, opts) => {
    assert.equal(opts.redirect, 'error');
    const fields = new URLSearchParams(opts.body);
    assert.equal(fields.get('client_id'), env.E2E_AAT_APP_ID);
    assert.equal(fields.get('client_secret'), env.E2E_AAT_APP_SECRET);
    if (url.endsWith('/token')) {
      assert.equal(fields.get('grant_type'), 'client_credentials');
      assert.equal(fields.has('scope'), false);
      return new Response(JSON.stringify({ code: 0, access_token: 'private-aat-token' }));
    }
    assert.equal(fields.get('token'), 'private-aat-token');
    return new Response(JSON.stringify({ ...valid, ...change }));
  };
}
test('dedicated app mints and verifies an Agent Employee token', async () => {
  const result = await fetchAAT(env, mock(), now);
  assert.equal(result.token, 'private-aat-token');
  assert.equal(result.identity.credential_type, 'aat');
});
for (const [name, change] of Object.entries({ tenant: { user_type: 0 }, user: { user_type: 1 }, inactive: { active: false }, application: { client_id: 'other' }, missingPrincipal: { open_id: '' }, expired: { exp: 1999999999 }, shortLifetime: { exp: 2000000600 } })) {
  test(`reject ${name}`, async () => assert.rejects(fetchAAT(env, mock(change), now), /AAT/));
}
test('postflight rejects changed agent principal', async () => {
  await assert.rejects(introspectAAT(env, 'private-aat-token', 'ou_other', mock(), now), /verification failed/);
});
test('missing or shared credentials fail before network access', async () => {
  for (const changes of [{ E2E_AAT_APP_SECRET: '' }, { TEST_BOT1_APP_ID: 'cli_dedicated' }]) {
    await assert.rejects(fetchAAT({ ...env, ...changes }, () => assert.fail('no network')), /credentials|separate/);
  }
});
test('upstream errors cannot disclose credentials', async () => {
  await assert.rejects(fetchAAT(env, async () => new Response('private-test-secret', { status: 403 }), now), { message: 'Credential service request failed (HTTP 403)' });
});
