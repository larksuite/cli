// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const runner = path.join(__dirname, 'run-live.js');
for (const mode of ['success', 'acquire-failure', 'verify-failure', 'smoke-skip', 'live-failure']) {
  test(`lifecycle ${mode}: cleanup, exit status and sanitized evidence`, () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(),'ci-lifecycle-'));
    try {
      fs.mkdirSync(path.join(root,'scripts')); fs.mkdirSync(path.join(root,'bin')); fs.mkdirSync(path.join(root,'private'));
      fs.writeFileSync(path.join(root,'scripts/fetch_e2e_tat.js'), `const fs=require('fs');fs.writeFileSync(process.env.RUNNER_TEMP+'/e2e-live-tat','fake-tat-secret');console.log('::add-mask::fake-tat-secret');`);
      fs.writeFileSync(path.join(root,'preload.js'), `const Module=require('module');const original=Module._load;Module._load=function(id,...args){if(id==='./fetch-uat')return {
        fetchUAT:async()=>{if(process.env.MODE==='acquire-failure')throw Error('fake-bbq-secret upstream body');return {access_token:'fake-uat-secret',expires_at:Math.floor(Date.now()/1000)+7200}},
        verifyIdentities:async()=>{if(process.env.MODE==='verify-failure')throw Error('fake-bbq-secret upstream body')}
      };return original.call(this,id,...args)};`);
      fs.writeFileSync(path.join(root,'bin/go'), `#!${process.execPath}
const fs=require('fs');const args=process.argv.slice(2);const smoke=args.some(a=>a.startsWith('-run='));const mode=process.env.MODE;
if(!args.includes('--rerun-fails-run-root-test'))process.exit(89);
if(smoke){const pattern=new RegExp(args.find(a=>a.startsWith('-run=')).slice(5));if(!['TestDocs_CreateAndFetchWorkflowAsBot','TestDocs_CreateAndFetchWorkflowAsUser_AATCompatible'].every(name=>pattern.test(name)))process.exit(90);}
fs.appendFileSync('executions',smoke?'smoke\\n':'live\\n');
if(process.env.E2E_BBQ_ACCESS_TOKEN || process.env.TEST_BOT1_APP_SECRET || process.env.LARKSUITE_CLI_USER_ACCESS_TOKEN)process.exit(88);
const xml=args.find(a=>a.startsWith('--junitfile=')).split('=')[1];const json=args.find(a=>a.startsWith('--jsonfile=')).split('=')[1];
const ev=['Bot','User'].map(as=>JSON.stringify({Package:'docs',Test:'TestDocs_CreateAndFetchWorkflowAs'+as+(as==='User'?'_AATCompatible':''),Action:mode==='smoke-skip'&&as==='User'?'skip':'pass'})).join('\\n');
fs.writeFileSync(json,ev);fs.writeFileSync(xml,'<testsuite>fake-uat-secret</testsuite>');console.log('fake-uat-secret fake-tat-secret');process.exit(!smoke&&mode==='live-failure'?7:0);
`, {mode:0o700});
      const result = spawnSync(process.execPath,['--require',path.join(root,'preload.js'),runner], {cwd:root,encoding:'utf8',env:{...process.env,MODE:mode,PATH:path.join(root,'bin')+path.delimiter+process.env.PATH,RUNNER_TEMP:path.join(root,'private'),LIVE_PKGS:'github.com/larksuite/cli/tests/cli_e2e/docs',TEST_BOT1_APP_ID:'cli_test',TEST_BOT1_APP_SECRET:'fake-app-secret',E2E_BBQ_ACCESS_TOKEN:'fake-bbq-secret'}});
      assert.equal(result.status,mode==='success'?0:mode==='live-failure'?7:1,result.stderr);
      assert.deepEqual(fs.readdirSync(path.join(root,'private')),[]);
      assert.doesNotMatch(result.stdout+result.stderr,/fake-(uat|tat|app|bbq)-secret/);
      for(const name of fs.readdirSync(path.join(root,'e2e-live-report'))) assert.doesNotMatch(fs.readFileSync(path.join(root,'e2e-live-report',name),'utf8'),/fake-(uat|tat|app|bbq)-secret/);
      const executed=fs.existsSync(path.join(root,'executions'))?fs.readFileSync(path.join(root,'executions'),'utf8'):'';
      assert.equal(executed,mode.endsWith('failure')&&mode!=='live-failure'?'':mode==='smoke-skip'?'smoke\n':'smoke\nlive\n');
      if(mode==='acquire-failure' || mode==='verify-failure') {
        const failure=JSON.parse(fs.readFileSync(path.join(root,'e2e-live-report/failure.json'),'utf8'));
        assert.equal(failure.stage,mode==='acquire-failure'?'user_token':'identity_verification');
        assert.equal(failure.reason,'Execution failed');
      }
      if(mode==='live-failure')assert.ok(fs.existsSync(path.join(root,'e2e-live-report/live.xml')));
    } finally {fs.rmSync(root,{recursive:true,force:true});}
  });
}
