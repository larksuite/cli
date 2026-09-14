#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
# Deployed to the CI-owned BBQ script. Output parameters are separate from logs.
set +x
set -euo pipefail
CI_UAT_RESULT="$(python3 - <<'PY'
import json
import os
import time
import requests

stage = 'configuration'
try:
    def required(name):
        value = os.environ.get(name, '')
        if not value:
            raise ValueError('missing configuration')
        return value

    app_id = required('APP_ID')
    if app_id != required('EXPECTED_APP_ID'):
        raise ValueError('application mismatch')
    account_id = required('ACCOUNT_POOL_ID')
    user_id = required('EXPECTED_USER_ID')
    tenant_id = required('EXPECTED_TENANT_ID')
    request_id = required('REQUEST_ID')
    session = requests.Session()

    def call(method, base, path, **kwargs):
        if not base.startswith('https://'):
            raise ValueError('HTTPS required')
        response = session.request(method, base.rstrip('/') + path,
                                   timeout=30, allow_redirects=False, **kwargs)
        if response.status_code != 200:
            raise ValueError('HTTP failure')
        value = response.json()
        if not isinstance(value, dict):
            raise ValueError('invalid response')
        return value

    stage = 'account_pool'
    accounts = call('GET', required('BBQ_BASE_URL'), '/bbq/open_api/v1/user_account_pool/all',
                    headers={'x-access-token': required('USER_TOKEN')},
                    params={'spaceNodeID': required('SPACE_NODE_ID'),
                            'userId': user_id, 'notMask': 'true'})
    if accounts.get('code') != 0:
        raise ValueError('account lookup failed')
    matches = [row for row in accounts['data'] if str(row.get('id')) == account_id
               and str(row.get('userId')) == user_id
               and str(row.get('tenantId')) == tenant_id]
    if len(matches) != 1 or not matches[0].get('sessionKey'):
        raise ValueError('fixed account missing')
    stage = 'session_code'
    login = call('POST', required('OPEN_BASE_URL'), '/open-apis/mina/v2/login',
                 json={'appid': app_id, 'sessionid': matches[0]['sessionKey'], 'anonymousid': ''})
    if login.get('error') != 0 or not login.get('data', {}).get('code'):
        raise ValueError('session authorization failed')
    stage = 'token_exchange'
    issued_at = int(time.time())
    token = call('POST', required('OAUTH_BASE_URL'), '/oauth/v3/token',
                 json={'client_id': app_id, 'client_secret': required('APP_SECRET'),
                       'code': login['data']['code'], 'grant_type': 'authorization_code'})
    if token.get('code') != 0:
        raise ValueError('token exchange failed')
    access_token = token.get('access_token')
    expires_in = token.get('expires_in')
    if not isinstance(access_token, str) or not access_token or not isinstance(expires_in, int) or expires_in < 2700:
        raise ValueError('invalid token or lifetime')
    expires_at = issued_at + expires_in
    stage = 'scope_verification'
    if not set(required('UAT_SCOPES').split()).issubset(set(token.get('scope', '').split())):
        raise ValueError('requested scopes not granted')
    stage = 'user_verification'
    info = call('GET', required('OPEN_BASE_URL'), '/open-apis/authen/v1/user_info',
                headers={'Authorization': 'Bearer ' + access_token})
    user = info.get('data') or {}
    if info.get('code') != 0 or not user.get('open_id') or user.get('open_id') != required('EXPECTED_OPEN_ID') or user.get('tenant_key') != required('EXPECTED_TENANT_KEY'):
        raise ValueError('user mismatch')
    # user_info tenant_key is a different identifier from account-pool tenantId.
    if token.get('open_id') and token['open_id'] != user['open_id']:
        raise ValueError('open ID mismatch')
    print(json.dumps({'version': 3, 'status': 'ok', 'request_id': request_id, 'app_id': app_id,
                      'user_id': str(user['user_id']), 'open_id': user['open_id'],
                      'issued_at': issued_at, 'expires_at': expires_at,
                      'access_token': access_token}, separators=(',', ':')))
except Exception:
    # Never emit upstream bodies, exceptions, sessions, codes, or tokens.
    print(json.dumps({'version': 3, 'status': 'error', 'request_id': os.environ.get('REQUEST_ID', ''), 'stage': stage}))
PY
)" || exit 1
[ -n "$CI_UAT_RESULT" ] || exit 1
export CI_UAT_RESULT
echo 'UAT request completed; result is in the private output parameter'
