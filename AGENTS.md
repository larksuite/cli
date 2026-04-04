# AGENTS.md
Concise maintainer/developer guide for building, testing, and opening high-quality PRs in this repo.

## Goal (pick one per PR)
- Make CLI better: improve UX, error messages, help text, flags, and output clarity.
- Improve reliability: fix bugs, edge cases, and regressions with tests.
- Improve developer velocity: simplify code paths, reduce complexity, keep behavior explicit.
- Improve quality gates: strengthen tests/lint/checks without adding heavy process.

## Fast Dev Loop
1. `make build` (runs `python3 scripts/fetch_meta.py` first)
2. `make unit-test` (required before PR)
3. Run changed command(s) manually via `./lark-cli ...`

## Pre-PR Checks (match CI gates)
1. `make unit-test`
2. `go mod tidy` (must not change `go.mod`/`go.sum`)
3. `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6 run --new-from-rev=origin/main`
4. If dependencies changed: `go run github.com/google/go-licenses/v2@v2.0.1 check ./... --disallowed_types=forbidden,restricted,reciprocal,unknown`
5. Optional full local suite: `make test` (vet + unit + integration)

## Test/Check Commands
- Unit: `make unit-test`
- Integration: `make integration-test`
- Full: `make test`
- Vet only: `make vet`
- Coverage (local): `go test -race -coverprofile=coverage.txt -covermode=atomic ./...`

## Commit/PR Rules
- Use Conventional Commits in English: `feat: ...`, `fix: ...`, `docs: ...`, `ci: ...`, `test: ...`, `chore: ...`, `refactor: ...`
- Keep PR title in the same Conventional Commit format (squash merge keeps it).
- Before opening a real PR, draft/fill description from `.github/pull_request_template.md` and ensure Summary/Changes/Test Plan are complete.
- Never commit secrets/tokens/internal sensitive data.
