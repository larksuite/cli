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
