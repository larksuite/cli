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
make build
go test ./tests/cli_e2e/... -count=1
```

## Internal CI: UAT/TAT and AAT

The internal `e2e_live` job selects business packages from the diff. User commands
use UAT and bot commands use TAT; adding a normal test to a selected package does
not require changing CI.

For a self-contained user workflow whose **setup, assertions and cleanup** also
support Agent Employee AAT, name the top-level test
`TestXxxAsUser_AATCompatible`. The suffix is an explicit compatibility declaration,
not a different CLI identity: both UAT and AAT use `--as user`.

The AAT runner automatically discovers this exact suffix, including tests in new
packages, and unions those tests with its existing explicit allowlist. Duplicates
run once. Business contributors add the test and update their `coverage.md`;
new suffixed tests require no CI YAML or runner changes. Confirm AAT permissions
and product capability with a live run; do not mark bot-dependent, dry-run or
uncleanable workflows. Missing, skipped or failed selected tests fail the AAT gate.

See [internal live CI documentation](../../.codebase/e2e/README.md#adding-aat-compatible-cases)
for legacy explicit entries, discovery, reports and local validation.
