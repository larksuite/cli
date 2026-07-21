# AGENTS.md

## Goal (pick one per PR)

- Make CLI better: improve UX, error messages, help text, flags, and output clarity.
- Improve reliability: fix bugs, edge cases, and regressions with tests.
- Improve developer velocity: simplify code paths, reduce complexity, keep behavior explicit.
- Improve quality gates: strengthen tests/lint/checks without adding heavy process.

## Build & Test

```bash
make build             # Build (runs fetch_meta first)
make unit-test         # Required before PR (runs with -race where supported, e.g. amd64/arm64)
make live-skills-test  # Opt-in real Skills CLI tests; runs with isolated user directories
make test              # Full: vet + unit + integration
```

## Notification Opt-Outs

`lark-cli` emits two notice types into JSON envelope `_notice` to nudge AI agents toward fixes:

- `_notice.update` — a newer binary is available on npm
- `_notice.skills` — locally installed skills are out of sync with the running binary

To suppress them in non-CI scripts (CI envs are auto-skipped):

| Env var | Effect |
|---------|--------|
| `LARKSUITE_CLI_NO_UPDATE_NOTIFIER=1` | Suppress `_notice.update` |
| `LARKSUITE_CLI_NO_SKILLS_NOTIFIER=1` | Suppress `_notice.skills` |

Both notices recommend the same fix command: `lark-cli update`. The skills notice's `current` field is `""` when skills have never been synced (cold start) and a version string when synced for an older binary (drift).

## Pre-PR Checks (match CI gates)

1. `make unit-test`
2. `go vet ./...`
3. `gofmt -l .` — must produce no output
4. `go mod tidy` — must not change `go.mod`/`go.sum`
5. `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6 run --new-from-rev=origin/main`
6. If dependencies changed: `go run github.com/google/go-licenses/v2@v2.0.1 check ./... --disallowed_types=forbidden,restricted,reciprocal,unknown`

## Commit & PR

- Conventional Commits in English: `feat:`, `fix:`, `docs:`, `test:`, `refactor:`, `chore:`, `ci:`
- PR title in the same format. Fill `.github/pull_request_template.md` completely.
- Never commit secrets, tokens, or internal sensitive data.

## Source Layout

| Path | What it does |
|------|-------------|
| `cmd/root.go` | Entry point, command registration, strict mode pruning |
| `cmd/profile/` | Multi-profile management (add/list/use/rename/remove) |
| `cmd/config/` | Config init, show, strict-mode |
| `cmd/service/` | Auto-registered API commands from embedded metadata |
| `shortcuts/common/runner.go` | Shortcut execution pipeline, Flag.Input (@file/stdin) resolution |
| `shortcuts/` | Domain-specific shortcut implementations |
| `internal/cmdutil/factory.go` | Factory pattern — identity resolution, credential, config |
| `internal/cmdutil/factory_default.go` | Production factory wiring |
| `internal/credential/` | Credential provider chain (extension → default) |
| `extension/credential/` | Plugin-facing credential interfaces and env provider |
| `internal/client/client.go` | APIClient: DoSDKRequest, DoStream |
| `internal/core/config.go` | Multi-profile config loading/saving |
| `internal/vfs/` | Filesystem abstraction (use `vfs.*` instead of `os.*`) |
| `internal/validate/path.go` | Path safety validation |

## Who Uses This CLI

This CLI's primary consumers include AI agents (Claude Code, Cursor, Gemini CLI). Your code is read by machines — error messages, output format, and flag design all directly affect agent success rates.

The one rule to internalize: **every error message you write will be parsed by an AI to decide its next action.** Make errors structured, actionable, and specific.

## Code Conventions

### Structured errors in commands

Command-facing failures must be typed `errs.*` errors — never the legacy `output.Err*` helpers and never a final bare `fmt.Errorf`. AI agents parse the stderr envelope's `type` / `subtype` / `param` / `hint` fields to decide their next action; the full taxonomy lives in `errs/ERROR_CONTRACT.md`.

Picking a constructor:

| Failure | Constructor |
|---------|-------------|
| User flag/arg fails validation | `errs.NewValidationError(errs.SubtypeInvalidArgument, ...).WithParam("--flag")` |
| Valid request, wrong system state | `errs.NewValidationError(errs.SubtypeFailedPrecondition, ...).WithHint(...)` |
| Lark API returned `code != 0` | `runtime.CallAPITyped` (shortcuts) / `errclass.BuildAPIError` (raw responses) — never hand-build |
| Network / transport failure | `errs.NewNetworkError(errs.SubtypeNetworkTransport, ...)` |
| Local file I/O failure | `errs.NewInternalError(errs.SubtypeFileIO, ...)` — validate the path first (`validate.SafeInputPath` / `SafeOutputPath`) and use `vfs.*` |
| Unclassified lower-layer error as final | `errs.NewInternalError(errs.SubtypeUnknown, ...).WithCause(err)` |
| Lower layer already returned a typed error | pass it through unchanged — re-wrapping downgrades its classification |

Signatures that are easy to guess wrong:

- `runtime.CallAPITyped(method, url string, params map[string]interface{}, data interface{}) (map[string]interface{}, error)` — it performs the HTTP request itself and classifies `code != 0` into a typed error; just return the error it gives you.
- Typed pass-through check: `if _, ok := errs.ProblemOf(err); ok { return err }` — `ProblemOf` returns `(*errs.Problem, bool)`, not a nilable pointer.
- `.WithParam` exists only on `*errs.ValidationError`. `InternalError` / `NetworkError` have no param field — file or endpoint context goes in the message or `.WithHint(...)`.

`forbidigo` + `lint/errscontract` reject the legacy `output.Err*` helpers, bare final `fmt.Errorf` / `errors.New`, and legacy envelope literals on migrated paths. Beyond what lint catches, three authoring conventions apply:

- Preserve the underlying error with `.WithCause(err)` so `errors.Is` / `errors.Unwrap` keep working.
- `param` names only the user input that actually failed. Recovery guidance goes in `.WithHint(...)`; machine-readable recovery fields (`missing_scopes`, `log_id`) carry server/system ground truth only — never caller-side guesses.
- Error-path tests assert typed metadata via `errs.ProblemOf` (`category` / `subtype` / `param`) and cause preservation, not message substrings alone.

### stdout is data, stderr is everything else

Program output (JSON envelopes) goes to stdout. Progress, warnings, hints go to stderr. Mixing them corrupts pipe chains.

### Typed data over loose maps

Parse `map[string]interface{}` into a typed struct at the boundary — one projection function per shape — and let everything downstream consume struct fields, not string keys. A typo'd map key compiles fine and fails at runtime, which an agent then debugs blind.

Use distinct types when two values could be swapped silently: see `internal/meta.Token` — a bare string compiles on either side of a string/string signature, a distinct type does not.

Legacy loose-map code exists in older paths. Match its call sites when touching it, but do not copy the pattern into new code.

### Transcribe faithfully — no silent fallbacks

When code echoes input onward (request previews, transformations, proxies), transcribe verbatim. A `default:` branch that coerces unrecognized input into a plausible value ("unknown HTTP verb → GET") makes the output lie, and an agent reasons from the lie.

The same rule applies to flag combinations and internal wiring: if a requested option cannot be honored, return a typed validation error — never silently substitute another behavior and exit 0. Silent guesses (defaulting a missing identity, discarding writes on a nil writer) are bugs even when every current caller happens to avoid them.

### Use `vfs.*` instead of `os.*`

All filesystem access goes through `internal/vfs`. This enables test mocking.

### Validate paths before reading

CLI arguments are untrusted (they come from AI agents). Call `validate.SafeInputPath` before any file I/O.

### Tests

- Every behavior change needs a test alongside the change.
- A contract test must fail if the implementation is reverted. If you can undo the code change and the suite stays green, the contract is not pinned — assert the new field/behavior directly, not a happy-path substring.
- `cmdutil.TestFactory(t, config)` for test factories.
- `t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())` to isolate config state.

### E2E Testing

**Dry-run E2E (required for every shortcut change)**
- Validates request structure without calling real APIs
- Place in `tests/cli_e2e/dryrun/` or the corresponding domain directory
- Set env vars `LARKSUITE_CLI_APP_ID`/`APP_SECRET`/`BRAND`, use `--dry-run`, assert method/URL/params
- No secrets needed — runs on fork PRs
- Explore correct params with `lark-cli <domain> --help` and `lark-cli schema` first

**Live E2E (required for new flows or behavior changes)**
- Validates real API round-trips
- Place in `tests/cli_e2e/<domain>/`
- Must be self-contained: create -> use -> cleanup
- Needs bot credentials (CI secrets, skipped on fork PRs)
- Reference: `tests/cli_e2e/task/task_status_workflow_test.go`

| Change | Dry-run E2E | Live E2E |
|--------|:-----------:|:--------:|
| New shortcut | Required | Required |
| Modify shortcut flags/params | Required | If behavior changes |
| Shortcut bug fix | Required | If regression risk |
| Internal refactor (no shortcut impact) | Not needed | Not needed |
