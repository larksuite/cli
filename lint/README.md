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
└── domaincontract/     # endpoint domain contract: no hardcoded resolver hosts
    ├── scan.go         # ScanRepo(root) ([]lintapi.Violation, error)  ← public entry
    └── scan_test.go
```

## Endpoint domain contract (`domaincontract`)

`domaincontract` is a syntax-level regression guard for the resolver-owned
Open, Accounts, MCP, and AppLink hosts used by the Go CLI. In production `.go`
files it rejects:

- string literals containing a resolver-owned host FQDN
  (`{open,accounts,mcp,applink}.{feishu.cn,larksuite.com}`), and
- direct references to the SDK base-URL globals (`FeishuBaseUrl` / `LarkBaseUrl`)
  selected off an import of the SDK root package, which pick a host without
  going through the resolver. Unrelated identifiers sharing the name are not
  flagged.

Host literals are permitted only inside the resolver's `ResolveEndpoints`
function body (`internal/core/types.go`) and in this rule's own host list
(`lint/domaincontract/scan.go`); a helper elsewhere in the resolver file
returning a hardcoded host is still rejected. Comments and `_test.go` files
are not scanned. Literals are unquoted before matching (escape sequences
cannot hide a host) and match case-insensitively, and dot-imports of the SDK
root package are rejected outright (they would hide the globals from this
parse-level guard). The forbidden-host list is bound to the resolver source by
`TestForbiddenHostsMatchResolver`, so adding a resolver domain without updating
the guard fails the lint module's tests.

This is not a general outbound-URL or data-flow analyzer. It does not inspect
non-Go assets, hosts assembled from string fragments, SDK constructor option
flow, or previously unknown Feishu/Lark hosts. The literal rule and code review
remain the backstop for those cases.

To add or change an outbound endpoint, edit the resolver — never hardcode a host.

## Running

```bash
# from the repo root (one level above lint/)
go run -C lint . ..
```

`-C lint` switches Go's working directory to `lint/`; the `..` argument
is the repo root to scan (relative to `lint/`).

CI: `.github/workflows/ci.yml` step `Run source-contract lint guards (lintcheck)`.

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
