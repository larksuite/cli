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
    ├── scan.go         # ScanRepoWithOptions(root, opts)  ← public entry
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
└── domaincontract/     # resolver ownership + approved public hostname policy
    ├── scan.go         # ScanRepoWithOptions(root, opts)  ← public entry
    ├── unapproved.go   # Go AST/type-aware hostname extraction
    ├── policy.go       # exact public/fixture allowlist validation
    ├── diff.go         # added-line attribution
    └── *_test.go
```

## Endpoint domain contract (`domaincontract`)

`domaincontract` contains two complementary Go source guards.

The resolver-ownership guard rejects:

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

The approved-domain guard parses every Git-tracked Go file in full. In CI,
unapproved-host findings are limited to values whose expressions intersect an
added line; policy validation and unused-entry checks remain repository-wide.
It rejects an exact hostname unless it is present in one of:

- `internal/qualitygate/config/allowlists/public-domains.txt`, for production
  and test code; or
- `internal/qualitygate/config/allowlists/fixture-domains.txt`, only for
  `*_test.go`, the repository-root `tests/`, and any `testdata/` (never
  `skills/`).

RFC 2606 example/test names are accepted independently of those lists. This
includes the reserved `.test`, `.example`, `.invalid`, and `.localhost`
namespaces and the exact names `example.com`, `example.net`, and `example.org`;
they are safe placeholders rather than supported public endpoints.

High-confidence evidence is deliberately limited to static string expressions
assigned to `host`, `hostname`, or `domain` semantic names (including common
case/plural forms and collections), plus static strings whose entire value is
an absolute `http`, `https`, `ws`, or `wss` URL. It supports Go literals,
escapes, compile-time concatenation, constant references, grouped declarations,
multi-value assignments, and multiline expressions. Bare domain-shaped strings
without hostname semantics are not blocked.

Sequence values are scanned individually. For a hostname-semantic map, a key or
value is evidence only when it is the sole hostname-shaped side of that entry;
ambiguous string-to-string entries are not guessed. Struct fields use Go type
information so known non-network `Host` / `Domain` fields do not become hostname
evidence merely because an enum or command category contains a dot.

Allowlist matching is lowercase and exact: there are no wildcard, suffix, DNS,
or public-suffix exceptions. Entries must be sorted and unique, use ASCII
hostnames, and have a current in-scope use. See
`internal/qualitygate/config/README.md` for admission and approval rules.

This is not a general outbound-URL or cross-language data-flow analyzer. It does
not inspect non-Go assets or dynamically constructed values.

To add or change a resolver-owned Feishu/Lark endpoint, edit the resolver rather
than hardcoding the host elsewhere.

## Running

```bash
# PR-scoped scan from the repo root (one level above lint/)
go run -C lint . --changed-from <base-revision> ..

# Full inventory (also reports historical unapproved hostnames)
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

   type ScanOptions struct {
       ChangedFrom string
   }

   // ScanRepoWithOptions walks root and returns every violation produced
   // by this domain's checks. Domains MUST return []lintapi.Violation so
   // the top-level dispatcher can aggregate uniformly.
   func ScanRepoWithOptions(root string, opts ScanOptions) ([]lintapi.Violation, error) { ... }
   ```

3. Per-rule files are named `rule_<name>.go` with sibling
   `rule_<name>_test.go`. Each rule function returns
   `[]lintapi.Violation`. `runner.go` (or `scan.go`) composes the rules.

4. Register the domain in `lint/main.go`:

   ```go
   var scanners = []scanner{
       {name: "errscontract", fn: errscontract.ScanRepoWithOptions},
       {name: "<domain>", fn: func(root string, opts errscontract.ScanOptions) ([]lintapi.Violation, error) {
           return <domain>.ScanRepoWithOptions(root, <domain>.ScanOptions{
               ChangedFrom: opts.ChangedFrom,
           })
       }},
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
