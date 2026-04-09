# CLI E2E Tests

This directory contains end-to-end tests for `lark-cli`.

The purpose of this module is to verify real CLI workflows from a user-facing perspective: run the compiled binary, execute commands end to end, and catch regressions that are not obvious from unit tests alone.

## What Is Here

- `core.go`, `core_test.go`: the shared E2E test harness and its own tests
- `demo/`: reference testcase(s)
- `cli-e2e-testcase-writer/`: the local skill for adding or updating testcase files in this module

## For Contributors

When writing or updating testcases under `tests/cli_e2e`, install and use this skill first:

```bash
npx skills add ./tests/cli_e2e/cli-e2e-testcase-writer
```

Then follow `tests/cli_e2e/cli-e2e-testcase-writer/SKILL.md`.

Example prompt:

```text
Use $cli-e2e-testcase-writer to write lark-cli xxx domain related testcases.
Put them under tests/cli_e2e/xxx.
```

## Run

```bash
make e2e-test
```

JUnit report output:

```text
tests/cli_e2e/.artifacts/cli-e2e-report.xml
```

## Local User E2E Credentials

For `--as user` E2E runs, you can inject a portable credential file instead of
going through browser login.

Set `LARK_CLI_CREDENTIALS_FILE` to a JSON file like this:

```json
{
  "appId": "cli_xxx",
  "appSecret": "xxxx",
  "brand": "lark",
  "userOpenId": "ou_xxx",
  "userName": "e2e user",
  "accessToken": "",
  "refreshToken": "u-xxxx",
  "expiresAt": 0,
  "refreshExpiresAt": 0,
  "scope": "task:task:readonly",
  "grantedAt": 0
}
```

When this env var is present, the CLI E2E harness will:

- create an isolated temporary HOME + `config.json`
- point child `lark-cli` processes at that temp config directory
- let the CLI read refresh/access token data from the same credentials file
- remove the temporary files after each command run

Example:

```bash
export LARK_CLI_CREDENTIALS_FILE=/tmp/lark-cli-user-creds.json
go test ./tests/cli_e2e/task -count=1
```

For GitHub Actions, store the same JSON content as a base64-encoded repository
secret such as `TEST_USER_CREDENTIALS_B64`, decode it into a temporary file at
runtime, export `LARK_CLI_CREDENTIALS_FILE`, and remove the file in an
`if: always()` cleanup step.

## Browser Auth E2E (Playwright)

`tests/cli_e2e/auth` contains config/auth entry-chain tests:

- `auth login --no-wait --json` -> browser authorization -> `auth login --device-code`
- `config init --new` -> parse verification URL from process output -> browser authorization -> `config show`

Playwright files live in `tests/cli_e2e/browser`.

Run locally:

```bash
cd tests/cli_e2e/browser
npm install

cd ../../..
export LARK_E2E_ENABLE_BROWSER_AUTH=1
go test ./tests/cli_e2e/auth -count=1 -v
```

If your OAuth page redirects to Feishu login (QR/password), provide an
authenticated Playwright storage state:

```bash
cd tests/cli_e2e/browser
npx playwright codegen https://open.feishu.cn --save-storage=.auth/state.json
```

Then run E2E with:

```bash
export PLAYWRIGHT_STORAGE_STATE=/Users/bytedance/cli/tests/cli_e2e/browser/.auth/state.json
export LARK_E2E_ENABLE_BROWSER_AUTH=1
go test ./tests/cli_e2e/auth -count=1 -v
```

When enabled, tests write artifacts to a temporary directory and print its path:

- `cli.stdout.log`
- `cli.stderr.log`
- `playwright.stdout.log`
- `playwright.stderr.log`
