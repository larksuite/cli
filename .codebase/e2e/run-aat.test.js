// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { aatEnv, checkResults, workflows } = require('./run-aat');
test('AAT child uses only the dedicated token and isolated configuration', () => {
  const e = aatEnv({ PATH: '/bin', E2E_AAT_APP_ID: 'cli_agent', E2E_AAT_APP_SECRET: 'secret', TEST_USER_ACCESS_TOKEN: 'uat', TEST_TENANT_ACCESS_TOKEN: 'tat', TEST_BOT1_APP_ID: 'old', E2E_BBQ_ACCESS_TOKEN: 'bbq', LARKSUITE_CLI_APP_SECRET: 'secret', LARKSUITE_CLI_USER_ACCESS_TOKEN: 'uat', LARKSUITE_CLI_TENANT_ACCESS_TOKEN: 'tat', LARKSUITE_CLI_CONFIG_DIR: '/personal' }, 'aat', '/isolated');
  assert.deepEqual(e, { PATH: '/bin', LARKSUITE_CLI_CONFIG_DIR: '/isolated', LARKSUITE_CLI_DATA_DIR: '/isolated', LARKSUITE_CLI_BRAND: 'feishu', LARKSUITE_CLI_APP_ID: 'cli_agent', LARKSUITE_CLI_AGENT_ACCESS_TOKEN: 'aat', LARKSUITE_CLI_DEFAULT_AS: 'user' });
});
function events(action = 'pass') { return Object.entries(workflows).flatMap(([Package, tests]) => tests.map(Test => JSON.stringify({ Package, Test, Action: action }))).join('\n'); }
test('all workflows must pass, missing or skipped tests cannot make the gate green', () => {
  assert.equal(checkResults(events()).passed, true);
  for (const e of ['', events('skip'), events('fail'), events().split('\n')[0], events()+'\n'+JSON.stringify({ Action: 'fail', Package: 'calendar' }), events()+'\n'+JSON.stringify({ Action: 'skip', Package: 'calendar', Test: 'subtest' })]) assert.equal(checkResults(e).passed, false);
});
test('AAT pipeline uploads report and returns real status using dedicated variables', () => {
  const yaml = fs.readFileSync(path.join(__dirname, '../pipelines/ci.yaml'), 'utf8');
  const job = yaml.split('\n  e2e_aat:')[1].split('\n  # The tarball')[0];
  assert.match(job, /E2E_AAT_APP_SECRET: \$\{\{ E2E_AAT_APP_SECRET \}\}/);
  assert.doesNotMatch(job, /TEST_BOT|E2E_BBQ|USER_ACCESS_TOKEN|TENANT_ACCESS_TOKEN/);
  assert.match(job, /node \.codebase\/e2e\/run-aat\.js/);
  assert.match(job, /path: e2e-aat-report/);
  assert.match(job, /exit "\$\(cat e2e-aat-report\/exit-code\)"/);
});

test('every selected root is required, including multiple tests in one package', () => {
  const lines = events().split('\n');
  for (let i = 0; i < lines.length; i++) {
    const summary = checkResults(lines.filter((_, n) => n !== i).join('\n'));
    assert.equal(summary.passed, false);
    assert.equal(summary.results.filter(x => x.status === 'missing').length, 1);
  }
  assert.equal(checkResults(events()).results.length, Object.values(workflows).flat().length);
});
