# AGENTS.md

## Goal (pick one per PR)

- Make CLI better: improve UX, error messages, help text, flags, and output clarity.
- Improve reliability: fix bugs, edge cases, and regressions with tests.
- Improve developer velocity: simplify code paths, reduce complexity, keep behavior explicit.
- Improve quality gates: strengthen tests/lint/checks without adding heavy process.

## Build & Test

```bash
make build          # Build (runs fetch_meta first)
make unit-test      # Required before PR (runs with -race)
make test           # Full: vet + unit + integration
```

## Pre-PR Checks (match CI gates)

1. `make unit-test`
2. `go mod tidy` — must not change `go.mod`/`go.sum`
3. `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6 run --new-from-rev=origin/main`
4. If dependencies changed: `go run github.com/google/go-licenses/v2@v2.0.1 check ./... --disallowed_types=forbidden,restricted,reciprocal,unknown`

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

`RunE` functions must return `output.Errorf` / `output.ErrWithHint` — never bare `fmt.Errorf`. AI agents parse stderr as JSON; bare errors break this contract.

### stdout is data, stderr is everything else

Program output (JSON envelopes) goes to stdout. Progress, warnings, hints go to stderr. Mixing them corrupts pipe chains.

### Use `vfs.*` instead of `os.*`

All filesystem access goes through `internal/vfs`. This enables test mocking.

### Validate paths before reading

CLI arguments are untrusted (they come from AI agents). Call `validate.SafeInputPath` before any file I/O.

### Tests

- Every behavior change needs a test alongside the change.
- `cmdutil.TestFactory(t, config)` for test factories.
- `t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())` to isolate config state.
