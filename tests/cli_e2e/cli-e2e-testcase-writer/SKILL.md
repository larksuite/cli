---
name: cli-e2e-testcase-writer
description: Write scenario-based end-to-end Go testcases for the compiled `lark-cli` binary under `tests/cli_e2e`. Use when adding or updating a CLI testcase that should autonomously explore help and schema output, build a self-contained lifecycle with `clie2e.RunCmd`, organize steps with `t.Run`, clean up with `t.Cleanup`, and assert JSON output with `testify/assert` and `gjson`.
metadata:
  requires:
    bins: ["lark-cli"]
---

# CLI E2E Testcase Writer

Write testcase code, not framework code. `tests/cli_e2e/core.go` already provides the harness, and `tests/cli_e2e/demo/task_lifecycle_test.go` is the reference example only. Unless the user explicitly asks for framework work, add or update testcase files only.

## What a good testcase looks like

A good cli e2e testcase here is:
- scenario-based, not a loose smoke test
- self-contained and data-consistent
  create the resource you later read, update, search, or delete
- broad enough to prove the workflow
  usually create plus one or more follow-up reads or mutations plus teardown
- scoped to one feature or one workflow
  do not turn one testcase into the entire domain
- written with normal Go testing primitives

This is different from traditional API test suites where usage docs live elsewhere. Here, the command contract is discoverable from `lark-cli --help`, domain help, subcommand help, and schema output, and the agent is expected to explore and verify it autonomously.

## File organization

Put real domain testcases under:

```text
tests/cli_e2e/{domain}/
```

Examples:
- `tests/cli_e2e/task/task_status_workflow_test.go`
- `tests/cli_e2e/task/task_comment_workflow_test.go`

Treat `tests/cli_e2e/demo/` as reference material, not as the place to accumulate real coverage.

## How to split cases

Split by feature or workflow, not by API surface inventory.

Good splits:
- one file for task status flow: `create -> complete -> get -> reopen -> get`
- one file for task comment flow
- one file for task reminder flow
- one file for tasklist association flow

Bad split:
- one giant `task_test.go` that creates a task, updates it, comments it, reminds it, assigns it, adds followers, attaches tasklists, and queries everything in one lifecycle

Prefer:
- one top-level test per workflow
- one file per workflow or per closely related feature
- small shared helpers in the same domain test package when setup/cleanup logic truly repeats

## Explore before writing

Do not guess command names, flags, or payload fields from memory. Discover them:

```bash
lark-cli --help
lark-cli <domain> --help
lark-cli <domain> +<shortcut> -h
lark-cli <domain> <resource> <method> -h
lark-cli schema <domain>.<resource>.<method>
```

Use this exploration loop repeatedly while writing the testcase:
1. find the right domain and command path
2. decide whether the scenario should use a shortcut or a resource method
3. inspect the exact `--params` and `--data` shape
4. run the draft testcase
5. inspect failures, then go back to help or schema and refine

Also inspect environmental constraints before finalizing coverage:
- whether the current test environment supports `bot`, `user`, or both
- whether the scenario needs external identities, preexisting groups, documents, chats, or other remote fixtures
- whether the command path is actually executable in CI-like conditions

## Use the harness directly

Call `clie2e.RunCmd` with `clie2e.Request`.

```go
result, err := clie2e.RunCmd(ctx, clie2e.Request{
	Args: []string{"task", "tasks", "get"},
	Params: map[string]any{
		"task_guid": taskGUID,
	},
})
require.NoError(t, err)
result.AssertExitCode(t, 0)
result.AssertStdoutStatus(t, 0)
```

Use `Request` like this:
- `Args`: command path and plain flags
- `Params`: JSON for `--params`
- `Data`: JSON for `--data`
- `BinaryPath`, `DefaultAs`, `Format`: only when the testcase must override defaults

## Default testcase shape

Use one top-level test per workflow. Break the workflow into substeps with `t.Run`.

```go
func TestDomain_Scenario(t *testing.T) {
	parentT := t
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)

	suffix := time.Now().UTC().Format("20060102-150405")
	var resourceID string

	t.Run("create", func(t *testing.T) {
		result, err := clie2e.RunCmd(ctx, clie2e.Request{...})
		require.NoError(t, err)
		result.AssertExitCode(t, 0)
		result.AssertStdoutStatus(t, true)
		resourceID = gjson.Get(result.Stdout, "data.guid").String()
		require.NotEmpty(t, resourceID)

		parentT.Cleanup(func() {
			// best-effort delete
		})
	})

	t.Run("get", func(t *testing.T) {
		require.NotEmpty(t, resourceID)
	})
}
```

Use this shape because:
- `t.Run` makes reports readable
- `parentT.Cleanup` keeps created resources alive for later substeps
- one testcase owns one full resource lifecycle

## Data self-consistency

Prefer workflows whose data can be created and cleaned up entirely within the testcase.

Good:
- create a task, then get/update/comment/delete that same task
- create a tasklist, then add a task created by the testcase

Be explicit when the data is not self-consistent:
- if a testcase needs a real user open_id, preexisting chat, existing document, or tenant-specific fixture, do not invent one
- call out the missing prerequisite to the user
- if you still want to leave a reference testcase in code, write it with `t.Skip()` and a short reason

Example:

```go
func TestTask_AssignWorkflow_UserOnly(t *testing.T) {
	t.Skip("requires a real user open_id and user-capable test environment")
}
```

Do not silently hardcode made-up IDs, fake URLs, or guessed remote resources just to make the testcase look complete.

## Environment constraints

Assume the current local/CI-like environment may support only `bot` identity by default.

Implications:
- do not assume `--as user` works
- commands or workflows that require user identity may be unsupported in the current environment
- confirm this by checking help, running the command, or using known repo guidance before writing the final testcase set

When `--as user` is unavailable:
- still implement bot-compatible workflows normally
- for user-only workflows, either stop and tell the user what prerequisite is missing, or leave a skipped testcase with `t.Skip()`

Typical risky areas:
- `+get-my-tasks`
- commands that require current-user profile or self identity lookup
- workflows that need a real user open_id for assign/follower/member mutations

## Go testing rules

- Use `t.Run` for lifecycle steps such as `create`, `update`, `get`, `list`, `delete`.
- Use `t.Cleanup` for teardown and shared cleanup.
- Use `t.Helper()` in local helpers when the same setup or assertion logic really repeats.
- Use table-driven tests only when the same scenario shape repeats across multiple inputs. Do not force table-driven style onto a single live workflow.
- Use `require.NoError` for command execution and prerequisites.
- Use `assert` for returned field values after the command has succeeded.
- Use `gjson.Get(result.Stdout, "...")` for JSON field extraction.

## Output conventions

- shortcut-style commands often return `{"ok": true, ...}` and should use `result.AssertStdoutStatus(t, true)`
- service-style commands often return `{"code": 0, "data": ...}` and should use `result.AssertStdoutStatus(t, 0)`

Then assert the business fields with `gjson`.

## Common mistakes

- Do not modify `tests/cli_e2e/core.go` just because one testcase wants a convenience wrapper.
- Do not write a testcase that depends on preexisting remote data.
- Do not put agent, model, or vendor brand names into task summaries, comments, tasklist names, fixture IDs, or other visible remote test data; use neutral prefixes such as `lark-cli-e2e-` or `<domain>-e2e-`.
- Do not attach cleanup to the create subtest if later subtests still need the resource.
- Do not place new real coverage under `tests/cli_e2e/demo/`.
- Do not dump all domain behaviors into one file or one testcase.
- Do not hardcode obvious defaults unless the command really needs explicit flags.
- Do not guess `Params` or `Data` fields when schema output can tell you the exact shape.
- Do not fabricate prerequisite data when the scenario needs real external fixtures.
- Do not force a user-only workflow to run in a bot-only environment; use `t.Skip()` with a concrete reason.
- Do not stop after the first draft. Run, inspect, explore again, and improve the testcase.

## Validation

- Run `go test ./tests/cli_e2e/... -count=1`.
- Rerun the touched package directly when the testcase is live and slow.
- If behavior is unclear, go back to help and schema before changing the testcase.
