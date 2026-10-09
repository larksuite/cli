// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
'use strict';
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawn } = require('node:child_process');
const { fetchAAT, introspectAAT } = require('./fetch-aat');
const { CredentialError } = require('./fetch-uat');
const { redact } = require('./run-live');
const { selectWorkflows } = require('./aat-selection');

const workflows = {
  'github.com/larksuite/cli/tests/cli_e2e/calendar': ['TestCalendar_PersonalEventWorkflowAsUser_AATCompatible'],
  'github.com/larksuite/cli/tests/cli_e2e/docs': ['TestDocs_CreateAndFetchWorkflowAsUser_AATCompatible'],
  'github.com/larksuite/cli/tests/cli_e2e/sheets': ['TestSheets_FindReplaceWorkflowAsUser_AATCompatible', 'TestSheets_CreateWorkflowAsUser_AATCompatible'],
};
function aatEnv(env, token, configDir) {
  const clean = { ...env };
  for (const key of Object.keys(clean)) {
    if (key.startsWith('LARKSUITE_CLI_') || key.startsWith('TEST_') || key.startsWith('E2E_') ||
        ['OPENCLAW_HOME', 'HERMES_HOME'].includes(key)) delete clean[key];
  }
  return { ...clean, LARKSUITE_CLI_CONFIG_DIR: configDir, LARKSUITE_CLI_DATA_DIR: configDir, LARKSUITE_CLI_BRAND: 'feishu',
    LARKSUITE_CLI_APP_ID: env.E2E_AAT_APP_ID, LARKSUITE_CLI_AGENT_ACCESS_TOKEN: token,
    LARKSUITE_CLI_DEFAULT_AS: 'user' };
}
function checkResults(events, selected = workflows) {
  const final = new Map();
  let failed = false;
  for (const line of events.split('\n').filter(Boolean)) {
    const event = JSON.parse(line);
    if (event.Action === 'fail') failed = true;
    if (event.Test && ['pass', 'fail', 'skip'].includes(event.Action)) final.set(`${event.Package}/${event.Test}`, event.Action);
  }
  const results = Object.entries(selected).flatMap(([pkg, names]) => names.map(name => ({ test: `${pkg}/${name}`, status: final.get(`${pkg}/${name}`) || 'missing' })));
  const skipped = [...final.values()].filter(x => x === 'skip').length;
  return { passed: results.length > 0 && !failed && skipped === 0 && results.every(x => x.status === 'pass'), skipped, results };
}
async function runAAT(env = process.env) {
  const privateDir = fs.mkdtempSync(path.join(env.RUNNER_TEMP || os.tmpdir(), 'cli-e2e-aat-'));
  fs.chmodSync(privateDir, 0o700);
  const reportDir = path.resolve('e2e-aat-report');
  fs.mkdirSync(reportDir, { recursive: true });
  for (const name of fs.readdirSync(reportDir)) {
    if (/^live-\d+\.(log|jsonl|xml)$/.test(name) || ['live.log', 'live.jsonl', 'live.xml', 'discovery.log', 'discovery.jsonl', 'selection.json', 'summary.json', 'identity-summary.json', 'failure.json'].includes(name)) fs.rmSync(path.join(reportDir, name), { force: true });
  }
  const configDir = path.join(privateDir, 'config');
  fs.mkdirSync(configDir, { mode: 0o700 });
  let child;
  let interrupted = false;
  const stop = () => { interrupted = true; if (child?.pid) { try { process.kill(-child.pid, 'SIGTERM'); } catch {} } };
  process.on('SIGTERM', stop); process.on('SIGINT', stop);
  const secrets = [env.E2E_AAT_APP_SECRET];
  let stage = 'aat_selection';
  function execute(command, args, childEnv, name, stderrName) {
    if (interrupted) throw new CredentialError('AAT run interrupted');
    return new Promise((resolve, reject) => {
      const fd = fs.openSync(path.join(privateDir, name), 'w', 0o600);
      const stderrFD = stderrName ? fs.openSync(path.join(privateDir, stderrName), 'w', 0o600) : fd;
      child = spawn(command, args, { env: childEnv, detached: true, stdio: ['ignore', fd, stderrFD] });
      fs.closeSync(fd);
      if (stderrName) fs.closeSync(stderrFD);
      child.once('error', () => reject(new CredentialError('Could not start AAT subprocess')));
      child.once('close', code => { child = undefined; resolve(code ?? 1); });
    });
  }
  try {
    // Enumerate before minting a token so compilation cannot consume its lifetime.
    // The discovery child receives neither app secrets nor live credentials.
    const discoveryRC = await execute('go', ['test', '-json', '-list=^Test', '-run=^$', '-p=1', './tests/cli_e2e/...'],
      aatEnv(env, '', configDir), 'discovery.jsonl', 'discovery.log');
    for (const name of ['discovery.jsonl', 'discovery.log']) {
      fs.writeFileSync(path.join(reportDir, name), redact(fs.readFileSync(path.join(privateDir, name), 'utf8'), secrets));
    }
    if (discoveryRC !== 0) throw new CredentialError('AAT test discovery failed');
    const selected = selectWorkflows(fs.readFileSync(path.join(privateDir, 'discovery.jsonl'), 'utf8'), workflows);
    fs.writeFileSync(path.join(reportDir, 'selection.json'), JSON.stringify(selected, null, 2));
    stage = 'aat_acquisition';
    const { token, identity } = await fetchAAT(env);
    secrets.push(token);
    const childEnv = aatEnv(env, token, configDir);
    stage = 'aat_cli_identity';
    const binary = env.LARK_CLI_BIN || path.resolve('lark-cli');
    if (await execute(binary, ['whoami', '--as', 'user'], childEnv, 'whoami.json') !== 0) throw new CredentialError('CLI AAT identity verification failed');
    const whoami = JSON.parse(fs.readFileSync(path.join(privateDir, 'whoami.json'), 'utf8'));
    if (whoami.principal !== 'agent_employee' || whoami.credential !== 'aat' || whoami.identity !== 'user' || whoami.appId !== env.E2E_AAT_APP_ID || whoami.available !== true) throw new CredentialError('CLI did not resolve Agent Employee AAT');
    stage = 'aat_workflows';
    let rc = 0;
    let events = '';
    let logs = '';
    let index = 0;
    // Keep explicit selections package-scoped: the same test name in a different
    // package must not run unless that package opts in too.
    for (const [pkg, names] of Object.entries(selected)) {
      const prefix = `live-${++index}`;
      const packageRC = await execute('go', ['run', 'gotest.tools/gotestsum@v1.12.3', '--format=standard-verbose',
        `--junitfile=${path.join(privateDir, `${prefix}.xml`)}`, `--jsonfile=${path.join(privateDir, `${prefix}.jsonl`)}`, '--',
        pkg, '-p=1', '-count=1', '-timeout=5m', `-run=^(${names.join('|')})$`], childEnv, `${prefix}.log`);
      if (packageRC !== 0) rc = packageRC;
      for (const extension of ['log', 'jsonl', 'xml']) {
        const name = `${prefix}.${extension}`;
        const file = path.join(privateDir, name);
        if (!fs.existsSync(file)) continue;
        const clean = redact(fs.readFileSync(file, 'utf8'), secrets);
        fs.writeFileSync(path.join(reportDir, name), clean);
        if (extension === 'jsonl') events += clean + '\n';
        if (extension === 'log') logs += clean + '\n';
      }
    }
    fs.writeFileSync(path.join(reportDir, 'live.jsonl'), events);
    fs.writeFileSync(path.join(reportDir, 'live.log'), logs);
    const summary = checkResults(events, selected);
    fs.writeFileSync(path.join(reportDir, 'summary.json'), JSON.stringify(summary, null, 2));
    stage = 'aat_post_verification';
    const after = await introspectAAT(env, token, identity.open_id);
    if (await execute(binary, ['whoami', '--as', 'user'], childEnv, 'whoami-after.json') !== 0) throw new CredentialError('CLI AAT post-verification failed');
    const finalIdentity = JSON.parse(fs.readFileSync(path.join(privateDir, 'whoami-after.json'), 'utf8'));
    if (finalIdentity.principal !== 'agent_employee' || finalIdentity.credential !== 'aat' || finalIdentity.identity !== 'user' || finalIdentity.appId !== identity.app_id || finalIdentity.available !== true) throw new CredentialError('CLI AAT identity changed');
    fs.writeFileSync(path.join(reportDir, 'identity-summary.json'), JSON.stringify({ before: identity, after }, null, 2));
    console.log(`AAT workflows: ${summary.results.map(x => `${x.test.split('/').pop()}=${x.status}`).join(', ')}, skipped=${summary.skipped}`);
    return interrupted ? 1 : rc || (summary.passed ? 0 : 1);
  } catch (error) {
    const reason = error instanceof CredentialError ? error.message : 'AAT execution failed';
    fs.writeFileSync(path.join(reportDir, 'failure.json'), JSON.stringify({ stage, reason }, null, 2));
    console.error(`AAT E2E failed at ${stage}: ${reason}`);
    return 1;
  } finally {
    process.removeListener('SIGTERM', stop); process.removeListener('SIGINT', stop);
    fs.rmSync(privateDir, { recursive: true, force: true });
  }
}
if (require.main === module) runAAT().then(code => { process.exitCode = code; });
module.exports = { aatEnv, checkResults, runAAT, workflows };
