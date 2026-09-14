// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const script = path.join(__dirname, 'bbq-uat.sh');
const mockRequests = `import os,time
time.sleep=lambda _:None
class Response:
    status_code=200
    def __init__(self,data):self.data=data
    def json(self):return self.data
class Session:
    def request(self,method,url,**kw):
        assert kw['timeout']==30 and kw['allow_redirects'] is False
        mode=os.environ['MODE']
        if mode=='transport':raise Exception('SECRET_SESSION must not leak')
        if url.endswith('/user_account_pool/all'):
            assert kw['params']['userId']=='internal-user'
            assert kw['headers']['x-access-token']=='SECRET_BBQ'
            return Response({'code':0,'data':[{'id':42,'userId':'wrong' if mode=='account' else 'internal-user','tenantId':'tenant','sessionKey':'SECRET_SESSION'}]})
        if url.endswith('/mina/v2/login'):
            assert kw['json']=={'appid':'cli_test','sessionid':'SECRET_SESSION','anonymousid':''}
            return Response({'error':1 if mode=='session' else 0,'data':{'code':'SECRET_CODE'}})
        if url.endswith('/oauth/v3/token'):
            assert kw['json']=={'client_id':'cli_test','client_secret':'SECRET_APP','code':'SECRET_CODE','grant_type':'authorization_code'}
            return Response({'code':1 if mode=='exchange' else 0,'access_token':'SECRET_UAT','expires_in':10 if mode=='expiry' else 7200,'scope':'drive:drive' if mode=='scope' else 'drive:drive im:message'})
        assert url.endswith('/authen/v1/user_info')
        assert kw['headers']['Authorization']=='Bearer SECRET_UAT'
        return Response({'code':0,'data':{'user_id':'employee-id','open_id':'wrong' if mode=='identity' else 'ou_user','tenant_key':'wrong' if mode=='tenant' else 'tenant-key'}})
`;
for (const mode of ['ok','transport','account','session','exchange','expiry','scope','identity','tenant','application']) {
  test(`BBQ OAuth script ${mode}: fixed identity, scope and private output`, () => {
    const dir=fs.mkdtempSync(path.join(os.tmpdir(),'bbq-uat-test-'));
    try {
      fs.writeFileSync(path.join(dir,'requests.py'),mockRequests);
      const result=spawnSync('bash',['-c','source "$1"; printf "RESULT=%s\\n" "$CI_UAT_RESULT"','test',script],{encoding:'utf8',env:{...process.env,PYTHONPATH:dir,MODE:mode,APP_ID:'cli_test',EXPECTED_APP_ID:mode==='application'?'other':'cli_test',APP_SECRET:'SECRET_APP',USER_TOKEN:'SECRET_BBQ',SPACE_NODE_ID:'97',ACCOUNT_POOL_ID:'42',EXPECTED_USER_ID:'internal-user',EXPECTED_TENANT_ID:'tenant',EXPECTED_OPEN_ID:'ou_user',EXPECTED_TENANT_KEY:'tenant-key',REQUEST_ID:'request',BBQ_BASE_URL:'https://bbq.example.test',OPEN_BASE_URL:'https://open.example.test',OAUTH_BASE_URL:'https://accounts.example.test',UAT_SCOPES:'drive:drive im:message'}});
      assert.doesNotMatch(result.stdout.split('\n').filter(s=>!s.startsWith('RESULT=')).join('\n')+result.stderr,/SECRET_/);
      if(mode==='ok'){
        assert.equal(result.status,0,result.stderr);
        const value=JSON.parse(result.stdout.split('\n').find(s=>s.startsWith('RESULT=')).slice(7));
        assert.equal(value.version,3);assert.equal(value.access_token,'SECRET_UAT');assert.equal(value.user_id,'employee-id');assert.equal(value.request_id,'request');assert.equal(value.expires_at-value.issued_at,7200);
      }else{assert.equal(result.status,0);assert.doesNotMatch(result.stdout,/SECRET_UAT/);const error=JSON.parse(result.stdout.split('\n').find(s=>s.startsWith('RESULT=')).slice(7));assert.equal(error.status,'error');assert.ok(error.stage);}
    }finally{fs.rmSync(dir,{recursive:true,force:true});}
  });
}
