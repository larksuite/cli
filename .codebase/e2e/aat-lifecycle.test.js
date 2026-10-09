// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const { workflows } = require('./run-aat');
const runner = path.join(__dirname, 'run-aat.js');

for (const mode of ['pass', 'skip']) {
  test(`AAT lifecycle ${mode}: new package discovery, deduplication and required results`, () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), 'aat-lifecycle-'));
    try {
      fs.mkdirSync(path.join(root, 'bin')); fs.mkdirSync(path.join(root, 'private'));
      const addedPackage = 'github.com/larksuite/cli/tests/cli_e2e/newdomain';
      const addedTest = 'TestNewAsUser_AATCompatible';
      const catalog = { ...workflows, [addedPackage]: [addedTest, 'TestOrdinaryUserWorkflow'] };
      fs.writeFileSync(path.join(root, 'catalog.json'), JSON.stringify(catalog));
      fs.writeFileSync(path.join(root, 'preload.js'), `const Module=require('module'); const original=Module._load;
Module._load=function(id,...args){if(id==='./fetch-aat') {const identity={app_id:'cli_agent',open_id:'ou_agent'};return {fetchAAT:async()=>({token:'fake-aat-secret',identity}),introspectAAT:async()=>identity};}return original.call(this,id,...args)};`);
      fs.writeFileSync(path.join(root, 'bin/cli'), `#!${process.execPath}
console.log(JSON.stringify({principal:'agent_employee',credential:'aat',identity:'user',appId:'cli_agent',available:true}));`, { mode: 0o700 });
      fs.writeFileSync(path.join(root, 'bin/go'), `#!${process.execPath}
const fs=require('fs');const args=process.argv.slice(2);const catalog=JSON.parse(fs.readFileSync('catalog.json','utf8'));
if(process.env.E2E_AAT_APP_SECRET || process.env.TEST_USER_ACCESS_TOKEN || process.env.TEST_TENANT_ACCESS_TOKEN)process.exit(88);
if(args[0]==='test'){
 if(process.env.LARKSUITE_CLI_AGENT_ACCESS_TOKEN)process.exit(89);
 for(const [Package,names] of Object.entries(catalog)) {for(const name of names)console.log(JSON.stringify({Action:'output',Package,Output:name+'\\n'}));console.log(JSON.stringify({Action:'pass',Package}));}
 process.exit(0);
}
if(process.env.LARKSUITE_CLI_AGENT_ACCESS_TOKEN!=='fake-aat-secret')process.exit(90);
const pkg=args[args.indexOf('--')+1];const pattern=new RegExp(args.find(a=>a.startsWith('-run=')).slice(5));
const names=catalog[pkg].filter(name=>pattern.test(name));
fs.appendFileSync('executions',JSON.stringify({pkg,names})+'\\n');
const events=names.map(Test=>JSON.stringify({Package:pkg,Test,Action:process.env.MODE==='skip'&&Test==='${addedTest}'?'skip':'pass'})).join('\\n');
fs.writeFileSync(args.find(a=>a.startsWith('--jsonfile=')).slice(11),events);
fs.writeFileSync(args.find(a=>a.startsWith('--junitfile=')).slice(12),'<testsuite>fake-aat-secret</testsuite>');
console.log('fake-aat-secret');`, { mode: 0o700 });
      const result = spawnSync(process.execPath, ['--require', path.join(root, 'preload.js'), runner], {
        cwd: root, encoding: 'utf8', env: { ...process.env, MODE: mode, PATH: path.join(root, 'bin') + path.delimiter + process.env.PATH,
          RUNNER_TEMP: path.join(root, 'private'), LARK_CLI_BIN: path.join(root, 'bin/cli'),
          E2E_AAT_APP_ID: 'cli_agent', E2E_AAT_APP_SECRET: 'fake-app-secret', TEST_USER_ACCESS_TOKEN: 'fake-uat-secret' },
      });
      assert.equal(result.status, mode === 'pass' ? 0 : 1, result.stderr);
      const report = path.join(root, 'e2e-aat-report');
      const selected = JSON.parse(fs.readFileSync(path.join(report, 'selection.json'), 'utf8'));
      assert.deepEqual(selected[addedPackage], [addedTest]);
      const executions = fs.readFileSync(path.join(root, 'executions'), 'utf8').trim().split('\n').map(JSON.parse);
      assert.equal(executions.length, Object.keys(selected).length);
      for (const { pkg, names } of executions) assert.deepEqual(names.sort(), selected[pkg]);
      const summary = JSON.parse(fs.readFileSync(path.join(report, 'summary.json'), 'utf8'));
      assert.equal(summary.results.length, Object.values(workflows).flat().length + 1);
      assert.equal(summary.passed, mode === 'pass');
      assert.deepEqual(fs.readdirSync(path.join(root, 'private')), []);
      for (const name of fs.readdirSync(report)) assert.doesNotMatch(fs.readFileSync(path.join(report, name), 'utf8'), /fake-(aat|app|uat)-secret/);
    } finally { fs.rmSync(root, { recursive: true, force: true }); }
  });
}
