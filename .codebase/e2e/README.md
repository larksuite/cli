# Internal live E2E credentials

All internal CI changes live under `.codebase/`. The public workflow, Makefile,
credential provider, and Go test harness are unchanged.

## Per-run flow

1. `ci.yaml` selects affected packages and builds the CLI before acquiring tokens.
2. `run-live.js` reuses `scripts/fetch_e2e_tat.js` inside a private temporary
   directory. Its GitHub masking output is never sent to Codebase logs.
3. `fetch-uat.js` directly calls BBQ's `POST /bbk/open_api/v1/script/run` OpenAPI.
   No BBQ CLI installation, scheduled UAT writeback, or Codebase write API is
   required in the runner.
4. The CI-owned BBQ script pins the account-pool record, internal user and tenant,
   application, application-scoped open ID, and service tenant key. It reads the
   account's maintained session, calls `mina/v2/login` for a single-use code,
   exchanges it through `oauth/v3/token`, checks the granted scopes, and verifies
   the user with `authen/v1/user_info`. No browser runs in the acquisition path.
5. The result is exported as `CI_UAT_RESULT` in `data.outputArgs`, **never printed**.
   The caller ignores `outputLog`, correlates the invocation ID, rejects stale or
   mismatched results, and requires at least 45 minutes remaining.
6. The runner validates bot and user against service endpoints, then requires
   both document create/fetch/cleanup smoke tests to PASS. A missing or failed
   user identity fails the gate; a skip cannot satisfy identity coverage.
7. The selected live packages run with gotestsum and at most two retries.
   Retries run the entire parent test so workflow setup and cleanup are preserved. JSON events, JUnit, logs, and summaries are scrubbed before publication.
   The pipeline uploads the report before returning the original live exit code.
   Preflight failures also produce `failure.json` with a fixed stage label and
   locally authored error information (including HTTP status where available).
   Raw upstream bodies and exception messages are never used as diagnostics.

Only `TEST_USER_ACCESS_TOKEN` and `TEST_TENANT_ACCESS_TOKEN` are passed to the Go
harness. The harness maps credentials per command and preserves fixtures. Never
export real `LARKSUITE_CLI_*` credentials at job scope. BBQ credentials and the
application secret are removed from the test subprocess environment.

## Platform configuration

Configure these encrypted Codebase repository variables:

| Variable | Value |
| --- | --- |
| `TEST_BOT1_APP_ID` | Same application used by the BBQ script |
| `TEST_BOT1_APP_SECRET` | Matching application secret |
| `E2E_BBQ_BASE_URL` | HTTPS origin of the BBQ service |
| `E2E_BBQ_SPACE_NODE_ID` | Owning BBQ space |
| `E2E_BBQ_SCRIPT_ID` | CI-owned script ID |
| `E2E_BBQ_ACCESS_TOKEN` | BBQ OpenAPI access token authorized for that script |

Maintain the code in `bbq-uat.sh` on the CI-owned BBQ script using bbq-cli.
Do not overwrite shared reference templates. Declare `CI_UAT_RESULT` as an output
argument with empty default. These input arguments must be configured on BBQ:

- Defaults: `BBQ_BASE_URL`, `OPEN_BASE_URL`, `SPACE_NODE_ID`, `ACCOUNT_POOL_ID`,
  `EXPECTED_USER_ID`, `EXPECTED_TENANT_ID`, `EXPECTED_APP_ID`, `EXPECTED_OPEN_ID`,
  `EXPECTED_TENANT_KEY`, `OAUTH_BASE_URL`, `UAT_SCOPES`.
- Empty defaults, supplied by each CI invocation: `APP_ID`, `APP_SECRET`,
  `REQUEST_ID`.
- `USER_TOKEN` is injected by the BBQ execution environment. It must permit
  reading the designated account-pool record. Never embed it in script source.

The account pool owns session renewal. An expired session or revoked app access
fails acquisition; it is not repaired by parallel CI runs. Reauthorize the fixed
account in BBQ when required. The pipeline does not consume or rotate a shared
OAuth refresh token. Each invocation obtains its own authorization code and UAT;
current live checks must confirm that concurrently issued UATs remain valid.

The fixed user must first authorize this application and the required scopes.
Before that authorization, the same OAuth v3 exchange may succeed with only an
identity-read scope. Token presence is therefore insufficient: `UAT_SCOPES` is
an explicit required granted set, checked on every invocation. Refreshing a
session does not replace application authorization. Permission changes or revoked
consent require updating the fixed-user authorization before CI can pass.

The script follows the reference session/code OAuth v3 flow. It does not use
`mina/loginValidate`: that legacy UAT was rejected by newer IM APIs (99991695).
OAuth `expires_in` is a duration in seconds. No refresh token is retained and
there is no fallback to legacy authorization or browser automation.

Result version 3 includes `status: ok` or `status: error`. BBQ may return a
successful execution envelope for a failed authorization; the CI must validate
the private result status and all identity/lifetime fields, not just HTTP status
or the outer `code`. Failure results contain only a stage label, never exception
text, device codes, verification URLs, session credentials or tokens.

`user_account_pool.userId` is an internal user ID. `user_info.user_id` is the
application's employee identifier; they are not interchangeable. Establish the
expected application-scoped open ID and tenant key from a verified session of the
fixed account. Do not copy an open ID from another app.

BBQ script changes can take time to reach execution workers. The request ID and
result version guard against cached results, while credential and identity checks
fail closed if an incompatible script is executed. Verify a newly published script
before updating dependent pipeline code.

## Validation

Run without credentials:

```bash
make script-test
node --test .codebase/e2e/*.test.js
```

After configuring the variables and running `make build`, set `LARK_CLI_BIN` to
that binary and `LARKSUITE_CLI_BRAND=feishu`. For the two cleanable identity tests:

```bash
LIVE_PKGS=./tests/cli_e2e/docs E2E_LIVE_SMOKE_ONLY=1 node .codebase/e2e/run-live.js
```

For the selected or complete live suite, omit `E2E_LIVE_SMOKE_ONLY` and supply the
space-separated package list from `scripts/e2e_domains.js` in `LIVE_PKGS`.
The output directory is `e2e-live-report`; do not commit it. Inspect
`identity-summary.json` for `bot=1,user=1` and review every remaining skip in
`live.jsonl`. A successful preflight alone is not evidence that user tests ran.

## Business-domain selection

The pipeline currently selects domains from the branch diff through
`scripts/e2e_domains.js`. An IM-only change selects the IM package; shared/runtime
or `.codebase/` changes select the complete suite. The internal manual trigger
currently has no domain-selection input. Even a selected domain runs the mandatory
bot/user document smoke first, and the standard prerequisite CI jobs still run.

For an already configured standalone runner, `LIVE_PKGS=./tests/cli_e2e/im`
selects the IM suite when invoking `run-live.js`; the pipeline itself computes
and overwrites this variable from the diff.
