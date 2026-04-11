---
name: cli-e2e-testcase-writer
description: Use when adding or updating Go CLI E2E coverage for one `tests/cli_e2e/{domain}` domain of the compiled `lark-cli`, especially when the work requires live `--help` or `schema` exploration, scenario-based `clie2e.RunCmd` workflows, and per-domain `coverage.md` maintenance.
metadata:
  requires:
    bins: ["lark-cli"]
---

# CLI E2E Testcase Writer

Work on one domain per run. Produce exactly two artifacts for that domain:
- workflow testcase files under `tests/cli_e2e/{domain}/`
- `tests/cli_e2e/{domain}/coverage.md`

Focus on domain testcase files. Do not change shared E2E support code such as `tests/cli_e2e/core.go` unless the user explicitly asks. Treat `tests/cli_e2e/demo/` as reference only.

## Core standard

- Make the testcase scenario-based and self-contained.
- Prove one workflow end to end: create plus follow-up read, or mutate plus teardown.
- Prefer one file per workflow or one closely related feature.
- For mutable flows, prove persisted state with read-after-write assertions, not just exit code.
- Leave prerequisite-heavy paths uncovered when they cannot be proven, and explain why in `coverage.md`.

## Workflow

### 1. Explore the live CLI before writing code

```bash
lark-cli --help
lark-cli <domain> --help
lark-cli <domain> +<shortcut> -h
lark-cli <domain> <group> --help
lark-cli <domain> <group> <method> -h
lark-cli schema <domain>.<group>.<method>
```

### 2. Count leaf commands for the denominator

- A leaf command is one that executes an action — it has no further subcommands.
- If `lark-cli <domain> <group> --help` lists no subcommands, `<group>` itself is the leaf.
- Count `task +create` as one leaf and `task tasks get` as one leaf.
- Do not count parameter combinations.
- Reuse coverage already present under `tests/cli_e2e/{domain}/`. Do not count `tests/cli_e2e/demo/`.

### 3. Choose the proof surface before editing

Identify the provable risks for the touched workflow: invalid input, missing prerequisite, identity or permission, state transition, output shape, cleanup safety. If only the happy path is testable, document the blocked risk areas in `coverage.md`.

### 4. Add or update the workflow testcase

- Use `clie2e.RunCmd(ctx, clie2e.Request{...})`.
- Put command path and plain flags in `Args`; put JSON in `Params` (URL/path parameters) and `Data` (request body).
- Prefer one top-level test per workflow with `t.Run` substeps.
- Register teardown on `parentT.Cleanup` so it survives subtest failures.
- When touching an existing command, verify the JSON response shape is stable: assert status type, field paths, and identifiers consumed by later steps before changing assertions.

### 5. Run and iterate

Run `go test ./tests/cli_e2e/{domain} -count=1` while iterating and before finishing. If command shape or behavior is unclear, re-check help or schema (step 1) before changing assertions.

### 6. Refresh the domain outputs

- Update the workflow testcase files.
- Update `coverage.md`: recompute the denominator from live help output, mark each command as `shortcut` or `api`, and keep one command table for the whole domain.

## Testcase rules

- Override `BinaryPath`, `DefaultAs`, or `Format` on `clie2e.Request` only when the testcase truly needs it.
- Use `require.NoError`, `result.AssertExitCode`, `result.AssertStdoutStatus`, `assert`, and `gjson`.
- Shortcut responses (`{ok: bool}`) assert `true`; API responses (`{code: int}`) assert `0`.
- Use `t.Helper()` only for setup or assertion helpers that are called from multiple tests.
- Use table-driven tests only when the scenario shape repeats across inputs.
- For expected failures, assert stderr content and exit code when the environment makes them deterministic.
- If identity or external fixtures cannot be proven, leave the command uncovered and document the prerequisite rather than faking confidence.

## coverage.md

Keep `coverage.md` brief and mechanical. Include:
- a domain-specific H1 title
- a metrics section with denominator, covered count, and coverage rate
- a summary section restating each `Test...` workflow, key `t.Run(...)` proof points, and main blockers
- one command table for all commands

Recommended structure:

```markdown
# <Domain> CLI E2E Coverage

## Metrics
- Denominator: N leaf commands
- Covered: N
- Coverage: N%

## Summary
- TestXxx: ... key `t.Run(...)` proof points ...
- Blocked area: ...

## Command Table
| Status | Cmd | Type | Testcase | Key parameter shapes | Notes / uncovered reason |
| --- | --- | --- | --- | --- | --- |
| ✓ | task +create | shortcut | task_status_workflow_test.go::TestTask_StatusWorkflow | basic create; create with due | |
| ✕ | task +assign | shortcut |  | none | requires real user open_id |
```

- Mark each command `shortcut` or `api`.
- Write testcase entries in `go test -run` friendly form.
- Commands only exercised in `parentT.Cleanup` teardown are not counted as covered.
- Do not split covered and uncovered commands into separate sections.

## Guardrails

- Run as bot identity only; do not assume `--as user` works.
- Do not place new real coverage under `tests/cli_e2e/demo/`.
- Do not depend on preexisting remote data.
- Do not fabricate open_ids, chats, docs, or other remote fixtures.
- Prefer deterministic negative cases over tenant-dependent assertions.
- Do not guess `Params` or `Data` fields when help or schema can tell you the exact shape.
- Do not hardcode obvious defaults unless the command truly requires explicit flags.
- Do not put agent, model, or vendor brand names in visible remote test data; use neutral prefixes such as `lark-cli-e2e-` or `<domain>-e2e-`.
- A command is covered only when the testcase asserts returned fields or persisted state, not just exit code.
- Cleanup-only execution is not primary coverage, except `delete` in the same workflow that created the resource.
