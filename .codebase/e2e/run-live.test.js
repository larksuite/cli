// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { redact, testEnv, coverage, runLive } = require('./run-live');
test('test child only receives TEST tokens; fixture configuration is preserved', () => {
  const value = testEnv({ TEST_BOT1_APP_ID: 'cli_test', TEST_BOT1_APP_SECRET: 'secret', E2E_BBQ_ACCESS_TOKEN: 'secret', LARKSUITE_CLI_APP_ID: 'bad', LARKSUITE_CLI_USER_ACCESS_TOKEN: 'bad', LARKSUITE_CLI_BRAND: 'feishu', FIXTURE: 'fixture' }, 'uat', 'tat');
  assert.deepEqual(value, { TEST_BOT1_APP_ID: 'cli_test', LARKSUITE_CLI_BRAND: 'feishu', FIXTURE: 'fixture', TEST_USER_ACCESS_TOKEN: 'uat', TEST_TENANT_ACCESS_TOKEN: 'tat' });
});
test('redacts raw and encoded credentials', () => {
  assert.equal(redact('a+b a%2Bb token-long token', ['a+b', 'token', 'token-long', '']), '[REDACTED] [REDACTED] [REDACTED] [REDACTED]');
});
test('identity evidence requires PASS; final rerun wins', () => {
  const event = (Test, Action) => JSON.stringify({ Package: 'docs', Test, Action });
  const s = coverage([event('TestDocs_CreateAndFetchWorkflowAsUser','run'),event('TestDocs_CreateAndFetchWorkflowAsUser','skip'),event('TestDocs_CreateAndFetchWorkflowAsBot','pass'),event('Other','fail'),event('Other','pass')].join('\n'));
  assert.deepEqual(s, { passed: 2, failed: 0, skipped: 1, bot: 1, user: 0 });
});
test('unsafe package selection fails before acquiring tokens', async () => {
  await assert.rejects(runLive({ LIVE_PKGS: '' }), /Invalid live package/);
  await assert.rejects(runLive({ LIVE_PKGS: '--exec=bad' }), /Invalid live package/);
});
test('internal pipeline owns its implementation and tests', () => {
  const yaml = fs.readFileSync(path.join(__dirname, '../pipelines/ci.yaml'), 'utf8');
  const live = yaml.split('\n  e2e_live:')[1].split('\n  # The tarball')[0];
  assert.match(live, /node \.codebase\/e2e\/run-live\.js/);
  assert.match(yaml, /node --test \.codebase\/e2e\/\*\.test\.js/);
  assert.match(live, /E2E_BBQ_ACCESS_TOKEN: \$\{\{ E2E_BBQ_ACCESS_TOKEN \}\}/);
  assert.doesNotMatch(live, /LARKSUITE_CLI_(APP_ID|USER_ACCESS_TOKEN|TENANT_ACCESS_TOKEN):/);
  assert.doesNotMatch(live, /export TEST_(USER|TENANT)_ACCESS_TOKEN|whoami --as bot|TEST_USER_ACCESS_TOKEN:/);
  assert.match(live, /Preserve live E2E exit status/); assert.match(live, /path: e2e-live-report/);
});
