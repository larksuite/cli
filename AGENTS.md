# AGENTS.md

## Purpose

`lark-cli` is the official Lark/Feishu CLI for humans and AI agents. Optimize
for predictable machine-readable behavior without making the human CLI worse.

Keep each PR focused on one goal: CLI UX, reliability, simpler explicit code,
or a useful gate. Done means the correct implementation surface, preserved
contracts unless a break was requested, and reported checks.

Public behavior, tests, lint, and CI define current contracts. Existing code is
evidence, but a legacy exception is not precedent; `Proposal` documents are
direction only. Do not mix unrelated cleanup, hand-edit generated files, weaken
gates, or broaden allowlists to make CI pass.

## Implementation Discipline

Use this sequence for product behavior or enforcement changes:

1. Establish the current contract and owner before choosing a solution. Trace the
   flow from the command or shortcut through runtime and internal owners to wire
   output; inspect affected callers and tests, then choose the surface below.
2. Apply YAGNI to scope. Implement the behavior required now; do not add
   speculative modes, configuration, compatibility paths, extension points, or
   scaffolding.
3. Reuse the project's existing machinery before creating a parallel path.
   Prefer the owning generic, runtime, or internal surface, then the Go standard
   library or an existing dependency, while preserving the contracts below.
4. Fix the root cause at the narrowest cohesive boundary shared by affected
   callers. Keep command and domain policy at the caller unless there is a real
   cross-command invariant for an internal owner to enforce.
5. Ship the smallest complete change: implementation, a revert-failing regression
   test for behavior or enforcement, and required contract or guidance updates.

Optimize for minimum owned complexity, not minimum diff or line count. A correct
root-cause fix may touch the owner, callers, tests, and docs; a one-line workaround
at the wrong layer is not simpler. Prefer deletion and boring explicit code, and
avoid one-use abstractions or new dependencies for straightforward behavior.

YAGNI never overrides explicit requirements or output, error, path, security,
compatibility, and data-safety contracts. If a deliberately limited solution has
a real ceiling, document the ceiling and the concrete condition for expanding it
at the decision point.

## Build

Run `make build` for the canonical local build; it writes `./lark-cli` with
version metadata and the embedded service catalog. It, `make vet`,
`make unit-test`, `make live-skills-test`, and dependent targets first run
`python3 scripts/fetch_meta.py`, so Python 3 is required. On a clean checkout,
the ignored `internal/registry/meta_data.json` is absent and the first run needs
access to `open.feishu.cn`; a valid existing file is reused.
`LARKSUITE_CLI_REMOTE_META=off` does not disable this build-time fetch.
`make live-skills-test` requires working `npx` and network access.

If the fetch fails before Go starts, report the missing Python or network
prerequisite instead of changing product code or generated metadata. `go build .`
may compile against the tracked empty fallback metadata, but that is only a
degraded compile check without the full service catalog.

## Choose the Correct Surface

| Need | Implement in | Rule |
|------|--------------|------|
| Agent/human-friendly workflow, composition, or smart defaults | `shortcuts/<domain>/` via `common.Shortcut` | Must add UX or workflow value beyond exposing one endpoint. |
| One-to-one supported OpenAPI method | Upstream service metadata + generic `cmd/service/` machinery | Verify it with `schema` after the canonical metadata fetch. `internal/registry/meta_data.json` is generated and ignored; never hand-edit it or add a shortcut merely to expose a missing catalog method. |
| Arbitrary OpenAPI endpoint | Generic `cmd/api/` machinery | Keep it endpoint-agnostic. |
| Auth, config, profile, update, or CLI lifecycle | `cmd/<area>/` plus the owning shared/internal package | Keep new Cobra code as wiring when a lower owner exists. |
| EventKey, payload shape, or domain projection | `events/<domain>/` | Shared event mechanics stay in `internal/event/`; CLI assembly stays in `cmd/event/`. |
| Command-independent mechanism or cross-command invariant | Owning `internal/<area>/` package | Keep UX/domain policy at the caller; use a cohesive owner, not a generic utils package. Test the owner and affected caller contracts. |
| Public plugin or host integration | `extension/` | Exported symbols are compatibility commitments; orchestration stays internal. |
| Per-command decision guidance: when, avoid, prerequisites, tips, or examples | `affordance/<domain>.md` | Enrich `--help` and `schema` without restating command descriptions, flags, or field schemas. |
| Domain routing, concepts, safety, or cross-command agent workflow | `skills/<name>/SKILL.md` and `references/` | Keep always-needed decisions in `SKILL.md`, conditional HOW in references, and link commands from affordance. |

Do not duplicate one command surface inside another. Register new shortcuts in
the domain's `Shortcuts()`; declare risk, identities/scopes, flags, and dry-run.

## Hard Contracts

Read the [JSON output contract](README.md#json-output-contract) before changing
wire output. Read the [source guard guide](lint/README.md) and `.golangci.yml`
before changing or waiving enforcement.

- Command/flag semantics, help/schema metadata, output placement and shapes,
  errors, exit codes, risk/identity, and exported APIs are compatibility contracts.
  When forwarding or echoing accepted input, preserve it verbatim unless its
  contract defines normalization; never silently substitute another behavior.
- Success data goes to stdout; typed failure envelopes, progress, warnings, and
  hints go to stderr. Predicate/self-contained results and partial failures are
  documented exceptions whose complete result remains on stdout.
- Keep new or touched Cobra code as wiring. Lark/Feishu API calls in shortcuts go
  through `*common.RuntimeContext`; direct HTTP is only for non-gateway protocols
  such as presigned storage and requires a precise `//nolint:forbidigo` reason.
- Keep user/workspace FileIO invocation-scoped: use `runtime.FileIO()`,
  `runtime.ValidatePath()`, and `runtime.ResolveSavePath()` so portable commands do
  not assume a local host or process working directory.
- Shortcuts do not use `internal/vfs` for user/workspace files. A narrow
  `//nolint:depguard` waiver is allowed only for CLI-owned state or explicitly
  CLI-managed host configuration; explain that ownership boundary. Other internal
  filesystem code uses `internal/vfs` and validates paths.
- If FileIO lacks a host-local tree operation, prefer an owning `internal/` package
  or optional capability. Direct `os` calls require a stated local-only boundary,
  validated and bounded paths, and a precise `//nolint:forbidigo` reason.
- Do not hardcode resolver-owned hosts. At new API boundaries, or when changed
  behavior consumes fields from a loose map, project that shape into a typed
  struct before downstream use. Extend published interfaces through optional
  interfaces rather than breaking external implementations.
- Source guards enforce raw HTTP/os/vfs, resolver-host, and migrated-error
  constructs; other semantics rely on tests and review. Every exemption must be
  narrow, local, and explain why the safe path does not apply.

## Structured Errors

Before changing a command failure, taxonomy, or error wire field, read the
[error contract](errs/ERROR_CONTRACT.md); it owns constructor selection, wrapping,
extension fields, stability, and CI guards.

- Command-facing failures use typed `errs.*` unless the error contract defines
  an output-control exception. Never return a final plain `fmt.Errorf` /
  `errors.New` or ad hoc envelope; pass typed errors through and preserve causes.
- Lark API failures use a domain typed wrapper, `runtime.CallAPITyped`, or
  `runtime.DoAPIJSONTyped`; raw callers use `runtime.ClassifyAPIResponse` or
  `errclass.BuildAPIError`.
- `param` names only failing user input; recovery belongs in `hint`. Populate
  `missing_scopes`, `log_id`, and similar fields only from known runtime evidence.
- Error tests assert typed metadata and cause preservation, not message text alone.

## Affordance and Skills

Before editing command guidance, read the [affordance guide](affordance/README.md).
For plugin distributions, read [Ship skills and command guidance](extension/platform/README.md#ship-skills-and-command-guidance).

- Go metadata/schema owns WHAT; affordance owns command-level WHEN; `SKILL.md`
  owns domain routing, concepts, safety, and cross-command workflows; `references/`
  owns detailed or conditional HOW.
- Do not duplicate canonical descriptions, schemas, or generic error taxonomy.
  Keep workflow-specific recovery in a skill when it changes agent behavior;
  affordance examples remain runnable, current, and safe.
- Skill frontmatter `description` is a concise WHAT/WHEN/NOT routing trigger. Keep
  always-needed decisions in `SKILL.md`; move conditional detail to `references/`.
- Skill names and reference paths are public pointers. Every path reachable from
  shipped docs must ship. Before referencing a new content directory, update
  `content_embed.go` and add an embedded-FS reachability test; `assets/` and
  `scripts/` stay source-only unless the distribution contract changes.

## Tests

- Every behavior change needs a nearby test that fails if the implementation is
  reverted; assert fields, requests, typed errors, or side effects directly.
- Command/shortcut tests needing a Factory use `cmdutil.TestFactory(t, config)`;
  isolate config with
  `t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())`.
- Tests must not depend on developer profiles, keychains, home directories,
  execution order, or real credentials unless explicitly live.
- Live E2E flows are self-contained: create, use, and clean up even after failure.

| Shortcut change | Dry-run E2E | Live E2E |
|-----------------|:-----------:|:--------:|
| New shortcut | Required | Required |
| Flags or request params | Required | Required if behavior changes |
| Bug fix | Required | Required when risk reaches the API boundary |
| Internal refactor, no behavior change | Not needed | Not needed |

Dry-run tests use placeholder credentials and assert method, URL, params, and
body without a real API call. Confirm contracts with `--help` and `schema` first.
If no deterministic, cleanable live flow exists, do not leak tenant state or add
a flaky test; document the blocker, fixture conditions, and substitute evidence.

## Validation

Run the narrowest useful check while iterating, then broaden with risk:

| Change | Checks |
|--------|--------|
| Go package | `go test ./path/to/package/...`, then the pre-PR Go checks below |
| Broad/cross-cutting | `make test` |
| Committed command/help/schema surface | `make quality-gate` |
| Skills | `node scripts/skill-format-check/index.js`, then `make quality-gate` after committing the change |
| Affordance | Add/update `internal/affordance/*_source_test.go`; run `go test ./internal/affordance ./cmd/service ./internal/schema` |
| Make-covered scripts/workflows | `make script-test` |
| Public plugin SDK | `make examples-build` plus relevant `tests/plugin_e2e` |
| Auth sidecar | `make sidecar-test` |
| Skills sync behavior | `make live-skills-test` |
| Dependencies | `go mod tidy` plus the CI `go-licenses` check |

`make quality-gate` and diff-scoped linters compare the base with `HEAD`; they
ignore staged, unstaged, and untracked changes, so run them after committing.
Some skill-quality signals are warnings; frontmatter has the separate hard check
above. The gate does not validate affordance examples or references in this file:
run the focused source test and verify changed paths/symbols directly.

`make test` currently leaves root binaries `audit-observer` and `readonly-policy`; do not stage them.

Before a Go PR, run `make unit-test`, `make vet`, and `make fmt-check`;
`go mod tidy` must leave module files unchanged. Set
`QUALITY_GATE_CHANGED_FROM` to the PR base before diff-scoped checks. Pinned
`go run module@version` commands need module access unless cached.

```bash
go mod tidy
git diff --exit-code -- go.mod go.sum
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6 run --new-from-rev="$QUALITY_GATE_CHANGED_FROM"
go run -C lint . --changed-from "$QUALITY_GATE_CHANGED_FROM" ..
go test -C lint ./... -count=1
go run github.com/google/go-licenses/v2@v2.0.1 check ./... --disallowed_types=forbidden,restricted,reciprocal,unknown
```

CI is authoritative; state exactly which relevant checks were not run.

## Maintaining This File

Add a root rule only when it is repository-specific, non-obvious, actionable,
and prevents a recurring failure or high-impact contract breach. Prefer code,
tests, or CI for mechanizable constraints; keep only rationale and safe exceptions
here. Update named references in the same PR, and delete or move stale, obvious,
redundant, task-local, or fully enforced rules. Each edit should reduce ambiguity,
not only add text.

## Commit and PR

Use English Conventional Commits/PR titles, complete the PR template, and never
commit secrets, tokens, internal endpoints, or sensitive test data.
