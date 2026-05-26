# lint/

Source-level static checks that guard lark-cli conventions golangci-lint
cannot express. Each lint domain is a sibling Go package under `lint/`;
the top-level `lint/main.go` aggregates results and emits a single
exit code.

`lint/` is its own Go module so its `golang.org/x/tools/go/packages`
dependency does not leak into the shipped `lark-cli` binary's module
graph.

## Layout

```
lint/
├── go.mod              # module github.com/larksuite/cli/lint
├── go.sum
├── main.go             # package main — dispatches to every registered domain
├── lintapi/            # shared types every domain returns
│   └── violation.go    # Violation, Action, ActionReject / ActionLabel / ActionWarning
└── errscontract/       # first domain: typed-error contract guards
    ├── scan.go         # ScanRepo(root) ([]lintapi.Violation, error)  ← public entry
    ├── runner.go
    ├── typecheck.go
    ├── violation.go    # local type aliases to lintapi
    ├── rule_problem_embed.go
    ├── rule_no_registrar.go
    ├── rule_adhoc_subtype.go
    ├── rule_declared_subtype.go
    ├── rule_subtype_classifier.go
    ├── rule_typed_error_completeness.go
    └── *_test.go
```

## Running

```bash
# from the repo root (one level above lint/)
go run -C lint . ..
```

`-C lint` switches Go's working directory to `lint/`; the `..` argument
is the repo root to scan (relative to `lint/`).

CI: `.github/workflows/ci.yml` step `Run errs/ lint guards (lintcheck)`.

Exit codes follow `lint/main.go`:

| Code | Meaning |
|------|---------|
| 0 | no REJECT diagnostics (LABEL / WARNING are advisory) |
| 1 | one or more REJECT diagnostics |
| 2 | a domain's `ScanRepo` returned an error |

## Adding a new lint domain

1. Create a sibling package: `lint/<domain>/`. Pick a name that reads
   like a category, not a list of rules (`errscontract/` covers many
   error-contract rules; `flagnaming/` would cover many flag-related
   rules).

2. Inside the new package, expose one public entry:

   ```go
   package <domain>

   import "github.com/larksuite/cli/lint/lintapi"

   // ScanRepo walks root and returns every violation produced by this
   // domain's checks. Domains MUST return []lintapi.Violation so the
   // top-level dispatcher can aggregate uniformly.
   func ScanRepo(root string) ([]lintapi.Violation, error) { ... }
   ```

3. Per-rule files are named `rule_<name>.go` with sibling
   `rule_<name>_test.go`. Each rule function returns
   `[]lintapi.Violation`. `runner.go` (or `scan.go`) composes the rules.

4. Register the domain in `lint/main.go`:

   ```go
   var scanners = []scanner{
       {name: "errscontract", fn: errscontract.ScanRepo},
       {name: "<domain>",     fn: <domain>.ScanRepo},  // ← add here
   }
   ```

5. Verify locally:

   ```bash
   go test  -C lint ./...      # all domains' tests
   go run   -C lint . ..       # full scan against the repo
   ```

6. Document the rules. If they enforce a contract that already has a
   spec (e.g. `errs/ERROR_CONTRACT.md`), add the lint entry to that
   contract's "CI guards" table. Otherwise create a short spec
   alongside the package.

## Rule severity conventions (`lintapi.Action`)

| Action | Effect | When to use |
|--------|--------|-------------|
| `ActionReject` | exit 1, fails CI | a contract violation that must be fixed before merge |
| `ActionLabel`  | stderr only; CI can grep for `[needs-taxonomy-decision]` and label the PR | governance signal that asks a human to choose (e.g. `ad_hoc_*` subtype needs a taxonomy decision) |
| `ActionWarning`| stderr only | advisory hint surfaced to reviewers (typed scope unavailable, fallback to AST-only, etc.) — never gates merges |

Only `ActionReject` contributes to a nonzero exit code; `ActionLabel`
and `ActionWarning` are reviewer signal only.
