# CLI E2E Tests

This directory contains end-to-end tests for `lark-cli`.

The purpose of this module is to verify real CLI workflows from a user-facing perspective: run the compiled binary, execute commands end to end, and catch regressions that are not obvious from unit tests alone.

## Excel compatibility round trip

`sheets/TestSheets_ExcelCompatRoundTripWorkflow` is an opt-in workflow for
Excel compatibility validation:

```text
local XLSX -> Lark Sheet -> exported XLSX -> Lark Sheet
```

It captures four per-sheet screenshot stages:

- `source` and `exported` use a deterministic local workbook renderer.
- `imported` and `reimported` require a real Lark Sheet screenshot adapter.

Set the following environment variables before running the live workflow:

| Variable | Meaning |
|---|---|
| `LARK_CLI_E2E_EXCEL_COMPAT_SOURCE` | Absolute path to the source XLSX |
| `LARK_CLI_E2E_EXCEL_COMPAT_OUTPUT_DIR` | Absolute artifact directory; defaults to a test temp directory |
| `LARK_CLI_E2E_EXCEL_COMPAT_PROFILE` | Optional isolated lark-cli profile |
| `LARK_CLI_E2E_EXCEL_COMPAT_HOME` | Required with a profile; absolute isolated HOME containing that profile |
| `LARK_CLI_E2E_EXCEL_COMPAT_CONFIG_DIR` | Required with a profile; absolute isolated lark-cli config directory |
| `LARK_CLI_E2E_EXCEL_COMPAT_RENDER_COMMAND` | JSON argv array for local rendering; supports `{workbook}`, `{stage}`, `{output_dir}` |
| `LARK_CLI_E2E_EXCEL_COMPAT_SCREENSHOT_COMMAND` | JSON argv array for online screenshots; supports `{token}`, `{stage}`, `{output_dir}` |

Both adapter executables must be absolute paths. `{output_dir}` always means the
root artifact directory, and both adapters must write
`<output_dir>/<stage>/manifest.json` plus one non-empty PNG per sheet. Offline
manifests use `provenance.kind=synthetic_preview` and bind the exact
`workbook`; online manifests use `provenance.kind=lark_sheet` and bind the exact
`token`. The workflow clears each stage directory before capture, imports
serially, records the exported XLSX, reimports it, and verifies deletion of both
temporary spreadsheets. It fails if cleanup cannot be verified so leaked test
documents cannot be mistaken for a clean run.

Use the repository-local development binary and an isolated profile/config.
When targeting BOE/PRE/PPE, switch that binary with the
`lark-cli-env-switch` workflow first; do not replace the globally installed
stable lark-cli. When no profile is supplied, set an explicit bot app ID and
tenant access token; the workflow never falls back to the default global
profile.

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
make build
go test ./tests/cli_e2e/... -count=1
```
