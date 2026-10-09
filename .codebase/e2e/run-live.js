// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
'use strict';
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawn } = require('node:child_process');
const { CredentialError, fetchUAT, verifyIdentities } = require('./fetch-uat');

function redact(text, secrets) {
  for (const secret of secrets.filter(Boolean).sort((a, b) => b.length - a.length)) {
    for (const value of new Set([secret, encodeURIComponent(secret), JSON.stringify(secret).slice(1, -1)])) {
      text = text.split(value).join('[REDACTED]');
    }
  }
  return text;
}
function testEnv(env, uat, tat) {
  const result = { ...env };
  for (const key of Object.keys(result)) {
    if (key.startsWith('E2E_BBQ_') || key === 'TEST_BOT1_APP_SECRET') delete result[key];
  }
  // These standard names activate the provider globally, bypassing fixture tests.
  for (const key of ['LARKSUITE_CLI_APP_ID', 'LARKSUITE_CLI_APP_SECRET',
    'LARKSUITE_CLI_USER_ACCESS_TOKEN', 'LARKSUITE_CLI_TENANT_ACCESS_TOKEN']) delete result[key];
  result.TEST_USER_ACCESS_TOKEN = uat;
  result.TEST_TENANT_ACCESS_TOKEN = tat;
  return result;
}
function coverage(events) {
  const counts = { passed: 0, failed: 0, skipped: 0, bot: 0, user: 0 };
  // gotestsum appends reruns. Count the final result of each test once.
  const final = new Map();
  for (const line of events.split('\n').filter(Boolean)) {
    const event = JSON.parse(line);
    if (event.Test && ['pass', 'fail', 'skip'].includes(event.Action)) final.set(`${event.Package}/${event.Test}`, event);
  }
  for (const event of final.values()) {
    counts[{ pass: 'passed', fail: 'failed', skip: 'skipped' }[event.Action]]++;
    if (event.Action === 'pass' && event.Test === 'TestDocs_CreateAndFetchWorkflowAsBot') counts.bot++;
    if (event.Action === 'pass' && event.Test === 'TestDocs_CreateAndFetchWorkflowAsUser_AATCompatible') counts.user++;
  }
  return counts;
}
async function runLive(env = process.env) {
  const packages = (env.LIVE_PKGS || '').split(/\s+/).filter(Boolean);
  if (!packages.length || packages.some(p => !/^(?:\.\/|github\.com\/larksuite\/cli\/)tests\/cli_e2e(?:\/[A-Za-z0-9_./-]+)?$/.test(p))) {
    throw new Error('Invalid live package selection');
  }
  const privateDir = fs.mkdtempSync(path.join(env.RUNNER_TEMP || os.tmpdir(), 'cli-e2e-'));
  fs.chmodSync(privateDir, 0o700);
  const reportDir = path.resolve('e2e-live-report');
  fs.mkdirSync(reportDir, { recursive: true });
  for (const name of ['smoke.log', 'smoke.xml', 'smoke.jsonl', 'live.log', 'live.xml', 'live.jsonl', 'identity-summary.json', 'summary.json', 'failure.json']) {
    fs.rmSync(path.join(reportDir, name), { force: true });
  }
  let child;
  let interrupted = false;
  const stop = () => {
    interrupted = true;
    if (child?.pid) { try { process.kill(-child.pid, 'SIGTERM'); } catch { /* already exited */ } }
  };
  process.on('SIGTERM', stop);
  process.on('SIGINT', stop);
  const secrets = [env.TEST_BOT1_APP_SECRET, env.E2E_BBQ_ACCESS_TOKEN];
  function execute(command, args, childEnv, logPath) {
    return new Promise((resolve, reject) => {
      if (interrupted) return reject(new Error('Live run interrupted'));
      const fd = fs.openSync(logPath, 'w', 0o600);
      child = spawn(command, args, { env: childEnv, detached: true, stdio: ['ignore', fd, fd] });
      fs.closeSync(fd);
      child.once('error', () => reject(new Error('Could not start live subprocess')));
      child.once('close', code => { child = undefined; resolve(code ?? 1); });
    });
  }
  let stage = 'tenant_token';
  try {
    const tatLog = path.join(privateDir, 'tat.log');
    const tatRC = await execute(process.execPath, ['scripts/fetch_e2e_tat.js'], {
      ...env, RUNNER_TEMP: privateDir, LARKSUITE_CLI_APP_ID: env.TEST_BOT1_APP_ID,
    }, tatLog);
    // Never relay the upstream helper's GitHub masking directive or raw errors.
    if (tatRC !== 0) throw new Error('Tenant token acquisition failed');
    const tat = fs.readFileSync(path.join(privateDir, 'e2e-live-tat'), 'utf8').trim();
    if (!tat) throw new Error('Tenant token missing');
    secrets.push(tat);
    fs.rmSync(tatLog);
    fs.rmSync(path.join(privateDir, 'e2e-live-tat'));
    stage = 'user_token';
    const uat = await fetchUAT(env);
    secrets.push(uat.access_token);
    stage = 'identity_verification';
    await verifyIdentities({ ...env, TEST_TENANT_ACCESS_TOKEN: tat }, uat);
    console.log('Bot and fixed-user service verification passed; UAT lifetime >= 45 minutes');
    const cleanEnv = testEnv(env, uat.access_token, tat);
    // Always prove both identities with self-contained, cleanable document tests,
    // including changes whose selected packages do not contain docs.
    const commonArgs = ['run', 'gotest.tools/gotestsum@v1.12.3', '--rerun-fails=2', '--rerun-fails-run-root-test', '--rerun-fails-max-failures=20', '--format=standard-verbose'];
    const smokeLog = path.join(privateDir, 'smoke.log');
    const smokeEvents = path.join(privateDir, 'smoke.jsonl');
    stage = 'identity_smoke';
    const smokeRC = await execute('go', [...commonArgs, '--packages=./tests/cli_e2e/docs',
      `--junitfile=${path.join(privateDir, 'smoke.xml')}`, `--jsonfile=${smokeEvents}`, '--', '-count=1', '-timeout=5m',
      '-run=^TestDocs_CreateAndFetchWorkflowAs(Bot|User_AATCompatible)$'], cleanEnv, smokeLog);
    publish('smoke.log'); publish('smoke.xml'); publish('smoke.jsonl');
    const smoke = coverage(fs.existsSync(smokeEvents) ? fs.readFileSync(smokeEvents, 'utf8') : '');
    fs.writeFileSync(path.join(reportDir, 'identity-summary.json'), JSON.stringify(smoke, null, 2));
    console.log(`Identity smoke: bot=${smoke.bot}, user=${smoke.user}, skipped=${smoke.skipped}`);
    if (smokeRC !== 0 || smoke.bot !== 1 || smoke.user !== 1) return smokeRC || 1;
    if (interrupted) return 1;
    if (env.E2E_LIVE_SMOKE_ONLY === '1') return 0;
    // Recheck after smoke so the remaining 20-minute test budget is covered.
    if (uat.expires_at - Math.floor(Date.now() / 1000) < 25 * 60) throw new Error('Insufficient UAT lifetime for live suite');
    stage = 'live_suite';
    const rc = await execute('go', [...commonArgs, `--packages=${packages.join(' ')}`,
      `--junitfile=${path.join(privateDir, 'live.xml')}`, `--jsonfile=${path.join(privateDir, 'live.jsonl')}`,
      '--', '-count=1', '-timeout=20m'], cleanEnv, path.join(privateDir, 'live.log'));
    publish('live.log'); publish('live.xml'); publish('live.jsonl');
    const summary = coverage(fs.existsSync(path.join(privateDir, 'live.jsonl')) ? fs.readFileSync(path.join(privateDir, 'live.jsonl'), 'utf8') : '');
    fs.writeFileSync(path.join(reportDir, 'summary.json'), JSON.stringify(summary, null, 2));
    console.log(`Live results: passed=${summary.passed}, failed=${summary.failed}, skipped=${summary.skipped}`);
    return interrupted ? 1 : rc;
  } catch (error) {
    const reason = typeof CredentialError === 'function' && error instanceof CredentialError
      ? error.message : 'Execution failed';
    fs.writeFileSync(path.join(reportDir, 'failure.json'), JSON.stringify({ stage, reason }, null, 2));
    console.error(`Live E2E failed at ${stage}: ${reason}`);
    throw error;
  } finally {
    process.removeListener('SIGTERM', stop);
    process.removeListener('SIGINT', stop);
    fs.rmSync(privateDir, { recursive: true, force: true });
  }
  function publish(name) {
    const filename = path.join(privateDir, name);
    if (!fs.existsSync(filename)) return;
    const clean = redact(fs.readFileSync(filename, 'utf8'), secrets);
    fs.writeFileSync(path.join(reportDir, name), clean);
    if (name.endsWith('.log')) process.stdout.write(clean);
  }
}
if (require.main === module) {
  runLive().then(code => { process.exitCode = code; }).catch(() => {
    console.error('Live E2E failed during credential acquisition, verification, or report handling');
    process.exitCode = 1;
  });
}
module.exports = { runLive, redact, testEnv, coverage };
