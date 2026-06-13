# lark-cli Error Contract

`errs/` defines a typed, RFC 7807–aligned error taxonomy for the CLI. Three
audiences depend on it: **AI agents and shell scripts** parsing the JSON
envelope on stderr; **protocol adapters** mapping CLI errors into MCP /
OAuth shapes; and **framework + business code** producing errors. This file
is the single source of truth for all three.

Something off in production? See **Troubleshooting**.

## Invariants

1. Every error belongs to exactly one **Category**. The set is closed
   (`errs/category.go`); adding a member requires deliberate review.
2. Every typed error has a **Subtype** — a stable
   lowercase-with-underscores identifier declared in `errs/subtypes*.go`.
   Undeclared subtypes fail CI. Every error path constructs a typed
   `*errs.*` error at its origin, so the constraint applies uniformly.
3. **`Category` + `Subtype`** are wire-stable identifiers consumers may
   branch on. Renaming either is a breaking change.
4. `Code` is the upstream numeric code when known (e.g. Lark API code).
   It is `omitempty` and never carries CLI-internal meaning.
5. Every typed error embeds `errs.Problem`. `CheckProblemEmbed` rejects
   exported `*Error` structs that do not.
6. Wrapping is idempotent: re-wrapping an already-typed error returns it
   unchanged across the `errors.As` / `errors.Unwrap` chain.
7. For the typed-envelope path, exit codes derive from `Category` only
   via `output.ExitCodeForCategory` — including `SecurityPolicyError`,
   which exits `6` via `CategoryPolicy`. `output.ErrBare(code)` is the
   exception: it constructs an `*output.BareError`, a deliberate
   silent-exit signal (stdout already carries the answer) that bypasses
   the envelope (see **Predicate commands** below).

## Wire format

Typed errors render to **stderr** as one JSON object per process exit:

```json
{
  "ok": false,
  "identity": "user",
  "error": {
    "type": "authorization",
    "subtype": "missing_scope",
    "code": 99991679,
    "message": "missing scope `calendar:event:create` for app cli_xxx",
    "hint": "run lark-cli auth login --scope calendar:event:create",
    "log_id": "20260520-0a1b2c3d",
    "missing_scopes": ["calendar:event:create"],
    "console_url": "https://open.feishu.cn/app/cli_xxx/auth?q=..."
  }
}
```

| Field | Stability | Notes |
|-------|-----------|-------|
| `ok` | wire-stable | always `false` for errors |
| `identity` | wire-stable | `user` \| `bot` — caller identity; omitted when not resolved |
| `error.type` | **wire-stable** | one of the 9 Categories |
| `error.subtype` | **wire-stable** | declared Subtype constant |
| `error.code` | wire-stable | upstream numeric code, omitted when zero |
| `error.message` | informational | not safe to branch on |
| `error.hint` | informational | actionable recovery guidance |
| `error.log_id` | informational | upstream request id (server-side trace) |
| `error.retryable` | wire-stable | `true` when present; omitted when `false` |
| `error.param` | per-Subtype-stable | single offending parameter (`ValidationError`); see **Validation parameters** |
| `error.params` | per-Subtype-stable | per-parameter validation detail array (`ValidationError`); see **Validation parameters** |
| per-Subtype extension fields | per-Subtype-stable | e.g. `missing_scopes`, `console_url`, `challenge_url` |

`SecurityPolicyError` renders through the same typed envelope as every
other category. `error.type` is `"policy"`, `error.subtype` is one of
`challenge_required` / `access_denied`, and process exit is `6` via
`CategoryPolicy`.

## Categories

| Category | When | Exit | Typed struct |
|----------|------|------|--------------|
| `validation` | malformed user input | 2 | `ValidationError` |
| `authentication` | no valid token / login required | 3 | `AuthenticationError` |
| `authorization` | token lacks scope / app permission denied | 3 | `PermissionError` |
| `config` | local config missing / unbound | 3 | `ConfigError` |
| `network` | DNS, refused, timeout, transport | 4 | `NetworkError` |
| `api` | server-side Lark error w/o specific bucket | 1 | `APIError` |
| `policy` | content safety / security challenge | 6 | `SecurityPolicyError`, `ContentSafetyError` |
| `internal` | SDK contract violation / decode failure | 5 | `InternalError` |
| `confirmation` | high-risk action needs `--yes` | 10 | `ConfirmationRequiredError` |

Canonical mapping: `internal/output/exitcode.go` `ExitCodeForCategory`.

> **Note on the `authorization` / `PermissionError` asymmetry.** The wire
> `type` field uses the RFC 7807 / taxonomy-formal name `"authorization"`,
> but the Go type is named `PermissionError`. This is deliberate, following
> the gRPC / Google APIs convention (`codes.Unauthenticated` +
> `codes.PermissionDenied`): each name is chosen to be **maximally
> distinct and readable on its own**, not to be perfectly symmetric.
> `AuthenticationError` and `AuthorizationError` differ visually only at
> the 5th character and are easy to confuse in code review;
> `AuthenticationError` and `PermissionError` cannot be confused. The wire
> field stays formal because it is the protocol-level taxonomy; the Go
> type favors call-site readability.

## Flow

```
  call site
     │ constructs typed error (e.g. *errs.ValidationError)
     ▼
  command runE returns err
     │
     ▼
  cmd/root.go handleRootError dispatches:
     ├─ typed (errs.ProblemOf)    → typed JSON envelope; exit = ExitCodeOf(err)
     │     (includes *errs.SecurityPolicyError → policy envelope, exit 6;
     │      *errs.ConfigError, constructed typed at origin)
     ├─ *output.PartialFailureError → no stderr envelope (ok:false result already on stdout); exit = code
     ├─ *output.BareError         → no envelope (stdout already written); exit = code
     └─ Cobra usage error         → typed validation envelope (invalid_argument); exit 2
```

The dispatcher emits a JSON envelope on stderr for both the typed branch and
residual Cobra usage errors (missing required flag, unknown command,
argument validation): the latter are classified into a typed validation
envelope (`invalid_argument`) and exit `2`, matching the explicit flag and
subcommand guards.

### Predicate commands (`output.BareError`)

A small class of commands is **predicates**: they answer a yes/no
question and signal the answer through the shell exit code so callers
can write `if cmd; then ... fi`. `lark-cli auth check` is the canonical
example — its `README` contract is `exit 0 = ok, 1 = missing`.

These commands deliberately:

1. write a structured JSON answer to **stdout** themselves, and
2. return `output.ErrBare(exitCode)` — an `*output.BareError` — to
   communicate the exit code to the dispatcher without producing a
   `stderr` envelope.

`*output.BareError` is **not** an error in the typed-envelope sense — it
carries no category, subtype, or message, only an exit code. It is a
one-bit output-control signal that lives outside the contract for the
same reason `grep -q` / `diff` / `systemctl is-active` set non-zero exit
codes without printing anything to stderr: pollution of stderr by a
predicate's negative answer would break `2>/dev/null` log hygiene in
caller scripts.

A second class also uses `ErrBare`: a command that emits its own complete
structured result envelope on **stdout** under `--json` (e.g. `update`, whose
`{ok:false, error:{type, message}}` is its established output shape) and needs
only the exit code conveyed, with no `stderr` envelope. Like a predicate, its
answer is already on stdout; `ErrBare` carries the exit code alone.

New code should not reach for `ErrBare` unless the command's full answer is
already on stdout — a predicate's yes/no, or a self-contained result envelope
as above. Anything whose error content must reach the caller on `stderr`
belongs in a typed `*errs.XxxError` — or, for a batch result, in the
partial-failure outcome below.

### Partial failure (batch / multi-status)

A batch command (e.g. `drive +push` / `+pull` / `+sync`) that processes
many items can finish in a third state, neither full success nor a single
error: some items succeeded and some failed. Its primary output is the
per-item result, so it does **not** belong in a `stderr` error envelope.

Such a command returns `runtime.OutPartialFailure(data, meta)`, which:

1. writes the full result to **stdout** as an `ok:false` envelope — the
   summary and every per-item outcome (succeeded *and* failed) stay
   machine-readable, exactly as a successful `Out(...)` would carry them,
   but with `ok` honestly reporting failure; and
2. returns `*output.PartialFailureError`, a typed exit signal the
   dispatcher maps to a non-zero exit code while writing nothing further
   to `stderr`.

This is distinct from `ErrBare` (a predicate's one-bit answer) and from a
typed `*errs.XxxError` (a `stderr` error envelope): a partial failure is a
*result*, reported on stdout, that also failed. Consumers branch on
`ok == false` and then read `data.summary` / `data.items[]`.

## Consumers

### Go (in-process)

```go
var pe *errs.PermissionError
if errors.As(err, &pe) {
    fmt.Println("missing:", pe.MissingScopes)
}
```

Predicates cover the common categories (`errs/predicates.go`):

```go
if errs.IsAuthentication(err)       { ... }
if errs.IsPermission(err) { ... }
if errs.IsValidation(err) { ... }
```

Type-agnostic field access:

```go
if p, ok := errs.ProblemOf(err); ok {
    log.Printf("cat=%s subtype=%s retryable=%t", p.Category, p.Subtype, p.Retryable)
}
exitCode := output.ExitCodeOf(err) // ExitInternal for non-typed errors
```

### Shell / AI

```bash
out=$(lark-cli ... 2>&1)
code=$?

# Defensive guard: tolerate any non-JSON output before parsing with jq.
if ! jq -e . >/dev/null 2>&1 <<<"$out"; then
    printf '%s\n' "$out" >&2
    exit "$code"
fi

case "$(jq -r '.error.type // empty' <<<"$out")" in
  authorization) jq -r '.error.missing_scopes[]' <<<"$out" ;;
  network)       echo "transport failure, safe to retry" ;;
  internal)      echo "bug — file an issue with log_id $(jq -r '.error.log_id // "n/a"' <<<"$out")" ;;
esac
```

Unknown fields are forward-compatible additions: ignore, don't fail.
Branch only on `type`, `subtype`, `code`, `retryable`, and declared
extension fields — `message` is human-readable prose that may be
reworded without notice.

## Producers

### Quick reference

The canonical producer surface is the **builder API in `errs/types.go`** (per type: struct + `NewXxxError` + chained `WithX` setters live in one place):
each `NewXxxError(subtype, format, args...)` locks `Category` at the
constructor name, requires `Subtype` + `Message` positionally, and exposes
optional fields via chained `.WithX(...)` setters. Struct literals remain
legal for framework dynamic paths (e.g. classifier fanout) but the lint
`CheckTypedErrorCompleteness` still requires `Category` + `Subtype` +
`Message` on any literal it sees.

| Situation | Use |
|-----------|-----|
| Bad user input | `errs.NewValidationError(subtype, msg).WithParam("--flag")` |
| Login required | `errs.NewAuthenticationError(errs.SubtypeTokenMissing, msg)` |
| Token lacks scope | `errclass.BuildAPIError(resp, ctx)` |
| Local config missing | `errs.NewConfigError(errs.SubtypeNotConfigured, msg)` |
| Transport failure | `errs.NewNetworkError(errs.SubtypeNetworkTimeout, msg).WithCause(err)` (subtype: `timeout` / `tls` / `dns` / `server_error` / `transport`) |
| Lark API error | `errclass.BuildAPIError(resp, ctx)` |
| SDK / decode bug | `errs.NewInternalError(errs.SubtypeSDKError, msg).WithCause(err)` |
| Policy block | `errs.NewSecurityPolicyError(subtype, msg).WithChallengeURL(url)` or `errs.NewContentSafetyError(subtype, msg).WithRules(...)` |
| Needs `--yes` | `errs.NewConfirmationRequiredError(risk, action, msg)` |

### Authoring discipline

Five rules every producer follows. Some are enforced by `lint/errscontract`
AST guards (`go run -C lint . ..`); the rest by code review.

#### Propagate typed errors unchanged

A function that receives an error already carrying `errs.Problem`
returns it as-is up the stack. Reclassification at non-boundary frames
(e.g., wrapping a `*ValidationError` into `*InternalError`) defeats the
single-source taxonomy and silently downgrades typed signals.

Conforming:

```go
_, err := runtime.DoAPI(req, opts)
if err != nil {
    return err // already typed by the framework boundary
}
```

Non-conforming:

```go
return fmt.Errorf("calling /open-apis: %v", err)  // %v strips the typed shape
return &errs.InternalError{Cause: err}            // re-decides category
```

#### Never return a typed-nil pointer

A typed-nil pointer (`var pe *errs.PermissionError; return pe`) wraps as
a non-nil interface — `errors.As` matches and `.Error()` may panic.
Return interface `nil` literally.

Non-conforming:

```go
var e *errs.ValidationError  // nil pointer
return e                     // non-nil interface holding nil pointer
```

#### Let `Category` derive the exit code

Do not pick exit codes by hand in new typed producers — `ExitCodeForCategory`
maps `Category` to the shell code. A new exit-code requirement means a
new `Category`, not a one-off override at the call site.

(The only exits not derived from `Category` are the
`*output.BareError` and the `*output.PartialFailureError` signals, which
carry their own code by design and sit outside the typed-envelope contract —
see **Predicate commands**.)

#### Split `Message`, `Hint`, and `Cause`

Each field carries a distinct role:

| Field | Carries | Style |
|-------|---------|-------|
| `Message` | What is wrong | Direct, lowercase first letter, no trailing period |
| `Hint` | What to do next | Imperative ("run `lark-cli auth login`", "use `--as user`") |
| `Cause` | The wrapped upstream `error`, not a stringified copy | Typed; serialized as `json:"-"` |

`Hint` must not be merged into `Message`. AI agents and humans read them
on separate channels; merging defeats both.

`Cause` must be a real `error`. If the upstream returned an `error`,
place it in `Cause` so `errors.Is` and `errors.Unwrap` walk the chain —
do not inline its `.Error()` into `Message`.

Conforming:

```go
return errs.NewNetworkError(errs.SubtypeNetworkTransport,
    "request to /open-apis failed after 3 retries").
    WithHint("check connectivity and retry; set --log-level debug if it persists").
    WithCause(ioErr)
```

Non-conforming:

```go
Message: fmt.Sprintf("request failed: %v — retry later", ioErr)
// conflates what + what-to-do + cause into one string
```

#### Validation parameters: `Param` and `Params`

`ValidationError` carries two additive parameter fields. Both are
optional; a producer sets whichever fits the failure.

**`Param string` (wire `param`)** — the single offending parameter. When a
`*ValidationError` originates from a flag value, `Param` holds the flag
name with leading dashes (`"--priority"`, not `"priority"`). AI agents
grep this field literally to surface "the bad flag was `--X`". For
positional arguments, use the canonical name without dashes
(`"target_user_id"`).

**`Params []InvalidParam` (wire `params`)** — per-parameter validation
detail, for failures that need to report *which* parameters failed and
*why*, one entry each. Each `errs.InvalidParam` is
`{Name, Reason string, Suggestions []string}`: `Name` identifies the
parameter, `Reason` states why it failed, and the optional `Suggestions`
(wire `suggestions`, omitted when empty) carries ranked candidate
corrections an agent can retry with — the did-you-mean candidates for an
unknown flag or subcommand — without parsing the human-facing `hint`. This
is the CLI's rendering of the RFC 7807 `invalid-params` extension member
(RFC 7807 §3.1). The wire key is `params`, not `invalid_params`: the
enclosing envelope already carries `type:"validation"`, so the `invalid_`
qualifier would be redundant on the wire.

`Param` and `Params` are independent additive fields, not alternates of a
single representation. Use `Param` for the common single-parameter error;
use `Params` when one failure spans several parameters or needs a
per-parameter reason. Set with `.WithParam("--flag")` / `.WithParams(...)`.

A `params` wire example (multiple parameters each carrying a reason):

```json
{
  "ok": false,
  "identity": "user",
  "error": {
    "type": "validation",
    "subtype": "invalid_argument",
    "message": "2 parameters failed validation",
    "params": [
      { "name": "--start", "reason": "expected RFC3339, got \"yesterday\"" },
      { "name": "--end", "reason": "must be after --start" }
    ]
  }
}
```

### Constructing typed errors

Prefer the **builder API**. The constructor pins `Category` + `Subtype` +
`Message`, the chained setters fill optional fields, and the resulting
value retains its concrete `*XxxError` pointer through the chain so
type-specific setters remain reachable to the end:

```go
return errs.NewValidationError(errs.SubtypeInvalidArgument,
    "--data must be a valid JSON object: %v", parseErr).
    WithParam("--data")
```

Why builder over struct literal:

- `Category` is locked at the function name — caller cannot mis-specify it
- `Subtype` and `Message` are positional arguments — `go build` rejects
  the call site if either is missing
- The chain reads top-down: required identity first, optional fields after
- Message is `fmt.Sprintf`-formatted from `(format, args...)`, matching
  `fmt.Errorf` muscle memory and avoiding a separate `Sprintf` line

Struct literals remain legal — `CheckTypedErrorCompleteness` continues to
enforce `Category` + `Subtype` + `Message` on any literal it sees — and
the framework classifier (`internal/errclass/classify.go`) still uses
them on the dynamic dispatch path where a `Problem` value is composed
once and wrapped per Category branch. Outside that pattern, new code
should reach for the builder.

When the validation logic outgrows a single range check — multiple flags,
format parsing, conditional rules — extract it into a helper that also returns
the typed `*errs.ValidationError`; the helper, not `Execute`, sets `Param` (a
helper bound to one shortcut is normal in this codebase; see `parseTimeRange`
in `shortcuts/calendar/calendar_agenda.go`).

### Wrapping upstream errors

When a producer receives an error from a function it called, four cases
cover the decision:

| Source | Decision | Example |
|--------|----------|---------|
| Helper returned a typed `*errs.*Error` | Return unchanged | `return err` |
| Helper returned an untyped error tied to user input (`strconv.Atoi`, `json.Unmarshal`, …) | Construct a typed error; put the untyped error in `Cause` | `return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --data: %v", jsonErr).WithCause(jsonErr)` |
| SDK call via `runtime.DoAPI` failed | Return unchanged — the framework boundary already wrapped it | `return err` |
| Invariant broken (must-not-happen state) | Lift with `errs.WrapInternal`, set a `Message` describing the invariant | `return errs.WrapInternal(fmt.Errorf("identity resolver returned nil: %w", err))` |

Prefer the `Cause` field over `fmt.Errorf("ctx: %w", err)` when
attaching an upstream error to a typed one. `Cause` is the chain
`errs.UnwrapTypedError` walks and the chain consumer code expects;
`fmt.Errorf("...: %w", err)` only affects `.Error()` output, which the
wire envelope does not surface.

#### Boundary helpers (framework-internal)

These helpers are called from framework boundaries, not from domain
code:

- `errs.WrapInternal(err)` — lifts an untyped error to `*InternalError`;
  already-typed errors pass through unchanged.
- `client.WrapDoAPIError(err)` — classifies SDK transport / decode
  failures into `*errs.NetworkError` / `*errs.InternalError` at the SDK
  boundary.
- `client.WrapJSONResponseParseError(body, err)` — lifts response-layer
  JSON parse failures to `*errs.InternalError`.

If you find yourself reaching for `WrapDoAPIError` from a `shortcuts/**`
package, you are probably calling the SDK at the wrong layer — go
through `runtime.DoAPI`.

### Extending the taxonomy

#### Add a Subtype

1. Add a constant in `errs/subtypes.go` under the right Category block.
   Subtypes are framework-shared — service-specific Subtypes are an
   anti-pattern (the wire `code` field already identifies the source
   service; Subtype encodes cross-service semantics like `not_found`,
   `quota_exceeded`).
2. If it maps from a Lark code, register the mapping in
   `internal/errclass/codemeta_<service>.go`.
3. Add a dispatch test in `internal/errclass/classify_test.go`.
4. Reference the constant from a producer.
5. `go run -C lint . ..` — `CheckDeclaredSubtype` fails until the
   constant is wired through.

`ad_hoc_*` subtypes are a temporary unblocker that label a value for
follow-up, not a permanent identifier. Resolve any `ad_hoc_*` to a
declared constant within one week of introduction; `CheckAdHocSubtype`
emits a warning to keep them visible.

#### Add a typed Error struct

Rare; the existing structs cover the 9 Categories with room. If you must:

1. In `errs/types.go`, add a new section with: the struct embedding `errs.Problem`, a nil-receiver-safe `Unwrap()` if it carries `Cause`, a `NewXxxError(subtype, format, args...)` constructor, and one chained `WithX` setter per extension field.
2. Add an `IsXxx` predicate in `errs/predicates.go`.
3. Add a wire-format pin in `errs/marshal_test.go` and a builder-chain pin in `errs/types_test.go`.

`CheckProblemEmbed` enforces the `Problem` embed at lint time. New
top-level wire fields are forbidden — per-Subtype data goes into the
typed struct as a documented extension field, not into the envelope's
top level.

## CI guards

Two golangci-lint rules and the custom `errscontract` AST module enforce the
contract; CI runs all three on every PR.

**golangci-lint** — scopes are defined in `.golangci.yml` (not duplicated here,
so this spec cannot drift from the lint config):

| Rule | Enforces |
|------|----------|
| forbidigo `errs-no-bare-wrap` | a command / wire-boundary final error must be typed (`errs.NewXxxError`), never a bare `fmt.Errorf` / `errors.New`; a genuine intermediate wrap opts out with `//nolint:forbidigo` + a reason |
| errorlint | every error wrap uses `%w` and every comparison uses `errors.Is` / `errors.As` — interior wraps stay legal but cannot break the `errors.Unwrap` chain the typed boundary relies on |

**errscontract** (`lint/errscontract/`, a separate Go module so its
`golang.org/x/tools` dependency stays out of the shipped binary; run locally
with `go run -C lint . ..`):

| Check | Enforces |
|-------|----------|
| `CheckNoLegacyEnvelopeLiteral` / `CheckNoLegacyCommonHelperCall` / `CheckNoLegacyRuntimeAPICall` | the removed `output.*` legacy error surface cannot be reintroduced anywhere |
| `CheckProblemEmbed` | every exported `*Error` embeds `errs.Problem` |
| `CheckDeclaredSubtype` | every `Subtype:` value is a declared constant (or `ad_hoc_*`) |
| `CheckTypedErrorCompleteness` | every typed-error struct literal sets `Category`, `Subtype`, and `Message` |
| `CheckAdHocSubtype` | `ad_hoc_*` Subtypes flagged for promotion (warning) |
| `CheckNoRegistrar` | no `mergeCodeMeta` / `RegisterServiceMap` from service code |

`errscontract` also carries framework-internal invariants (nil-safe `Unwrap`,
builder immutability, unwrap symmetry); see `lint/errscontract/` for the full
set and `lint/README.md` for adding a new lint domain.

## Stability

| Tier | Surface | Change policy |
|------|---------|---------------|
| Wire-stable | `error.type`, `error.subtype`, `error.code`, `error.retryable`, declared extension fields, `Category` enum values | breaking change ⇒ semver major; deprecation window required |
| Additive | new Category, new declared Subtype, new extension field on an existing struct | minor release; consumers ignore unknown fields by contract |
| Experimental | `ad_hoc_*` Subtypes; fields documented as such in `errs/types.go` | may change or be promoted/removed within one release |

## Troubleshooting

**Envelope shows `type=api subtype=unknown` for what should be a more
specific category.** The Lark code is unknown to `LookupCodeMeta` and fell
through to the generic bucket (`internal/errclass/classify.go`). Add the
code to `internal/errclass/codemeta_<service>.go` with the right Category
and Subtype, plus a dispatch test in `internal/errclass/classify_test.go`.

**Envelope shows `type=internal subtype=sdk_error`.** Origin is
`client.WrapDoAPIError` taking the non-transport branch
(`internal/client/api_errors.go`). Check: did the SDK fail to decode the
response (look for `subtype=invalid_response` in the wrapped chain)? Was the
transport detection too narrow for this error (e.g. a `*url.Error` with an
inner that does not satisfy `net.Error`)? Either widen the transport
predicate or add an explicit typed wrap upstream.

**`CheckDeclaredSubtype` rejects my Subtype.** The constant must be
declared in `errs/subtypes*.go` *and* referenced from the dispatch path.
Bare string literals trip `CheckDeclaredSubtype` unless they match the
`ad_hoc_*` prefix; `ad_hoc_*` then trips `CheckAdHocSubtype` as a
follow-up warning.

**`errors.As(&typedErr)` panics with a nil-pointer receiver.** A typed-nil
slipped through. All typed errors define nil-safe `Unwrap()`, but
returning a typed-nil pointer up the stack still defeats `errors.As`.
Return interface `nil` from constructors, never a typed-nil pointer.

**Exit code is 5 (internal) when I expected 3 (auth).** The error was not
typed before reaching `handleRootError`. Wrap at the boundary
(`client.WrapDoAPIError` or a typed constructor) — the bare `error.Error()`
string cannot be classified retroactively.

## Security & privacy

- `log_id` is a server-side trace token. Safe to surface; it does not
  carry user content.
- `missing_scopes` is app configuration, not user data.
- `Message` and `Hint` must not contain tokens, JWTs, or personally
  identifying values. CI does not catch this — producer responsibility.
- Wrapped `Cause` is **not** serialized to the wire (`json:"-"`). It is
  retained for in-process `errors.Is` / `errors.Unwrap` traversal and
  optional debug logging only.

## Pointers (task-driven)

- *Which struct to construct?* → **Producers / Quick reference**
- *Add a new condition?* → **Add a Subtype**
- *Consume from a shell script?* → **Consumers / Shell / AI**
- *Understand or fix a CI failure?* → **CI guards**
- *Read source.* → `errs/doc.go` → `errs/category.go` → `errs/types.go`
  → `errs/predicates.go` → `internal/errclass/` →
  `cmd/root.go` `handleRootError`.
