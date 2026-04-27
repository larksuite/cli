# Sidecar Server Reference Implementation

> ⚠️ **This is a demo.** For production deployment, implement your own sidecar
> server conforming to the wire protocol in `github.com/larksuite/cli/sidecar`.

This example shows how to implement a sidecar auth proxy server that receives
HMAC-signed requests from lark-cli sandbox clients and forwards them to the
Lark/Feishu API with real credentials injected.

## What this demo shows

- HMAC-SHA256 request verification (timestamp drift, body digest, signature)
- Target host allowlist + https-only target validation (anti-SSRF / anti-downgrade)
- Identity-based token resolution (UAT for user, TAT for bot)
- Auth-header allowlist: real token may only be injected into `Authorization`
  / `X-Lark-MCP-UAT` / `X-Lark-MCP-TAT`, rejecting attempts to smuggle it into
  `Cookie`, `User-Agent`, or other intermediate-logged headers
- Audit logging with path ID-segment sanitization and upstream error truncation
- Safe request forwarding (strips client-supplied auth headers)

## What this demo does NOT handle

- **TAT refresh** — the shared `DefaultTokenProvider` caches the TAT via
  `sync.Once`, which never refreshes. A long-running server will return an
  expired TAT after 2 hours. Production implementations should maintain a
  TTL-based cache with early renewal.
- High availability / load balancing / hot key rotation
- TLS termination
- Rate limiting / per-identity quotas

## Both sides need the right build tags

Sidecar is split into **two separate binaries** with **different build tags**:

| Side | Binary | Build tag | How to build |
| --- | --- | --- | --- |
| Sandbox (client) | `lark-cli` | `authsidecar` | `go build -tags authsidecar -o lark-cli .` |
| Trusted (server) | `sidecar-server-demo` | `authsidecar_demo` | `go build -tags authsidecar_demo -o sidecar-server-demo ./sidecar/server-demo/` |

If the sandbox runs a standard `lark-cli` **without** `-tags authsidecar`, the
`LARKSUITE_CLI_AUTH_PROXY` env var is ignored and requests bypass the sidecar
entirely — real credentials (if any) leak to the sandbox.

## Prerequisites

The demo reuses the lark-cli credential pipeline, so the trusted machine must
have an app configured:

```bash
lark-cli config init --new   # configure app_id / app_secret (required)
lark-cli auth login          # store user refresh_token in keychain
                              # (only required if sandbox will use --as user)
```

`auth login` is **only required for user identity**. If the server will only
serve bot requests (TAT), `config init` alone is enough because the TAT is
minted from `app_id + app_secret`.

Also, the server process **must not** inherit `LARKSUITE_CLI_AUTH_PROXY` — if
it does, the sidecar credential provider would activate inside the server and
return sentinel tokens instead of real ones. The demo rejects this at startup
with a clear error, but you should make sure to `unset LARKSUITE_CLI_AUTH_PROXY`
in the server shell before launching.

## Run

```bash
./sidecar-server-demo \
  --listen 127.0.0.1:16384 \
  --key-file <HOME>/.lark-sidecar/proxy.key \
  --log-file <HOME>/.lark-sidecar/audit.log
```

### Flags

| Flag | Default | Purpose |
| --- | --- | --- |
| `--listen` | `127.0.0.1:16384` | Address to bind the HTTP listener |
| `--key-file` | `<HOME>/.lark-sidecar/proxy.key` | Path to write the generated HMAC key (mode 0600) |
| `--log-file` | *(empty, stderr)* | Audit log output path |
| `--profile` | *(empty, active profile)* | lark-cli profile name for credential lookup |

### Startup output

```
Auth sidecar listening on http://127.0.0.1:16384
HMAC key prefix: a3b2c1d4
Full key written to /Users/alice/.lark-sidecar/proxy.key (mode 0600)

Set in sandbox:
  export LARKSUITE_CLI_AUTH_PROXY="http://127.0.0.1:16384"
  export LARKSUITE_CLI_PROXY_KEY="<read from /Users/alice/.lark-sidecar/proxy.key>"
  export LARKSUITE_CLI_APP_ID="cli_xxx"
  export LARKSUITE_CLI_BRAND="feishu"
```

The `key-file` path is printed exactly as passed on the command line (relative
paths stay relative). The `HMAC key prefix` is the first 8 characters for
identification without revealing the full key.

### Sandbox env vars (complete list)

The startup banner only prints the *required* variables. Two more are
optional:

```bash
export LARKSUITE_CLI_AUTH_PROXY="http://..."       # required (see constraints below)
export LARKSUITE_CLI_PROXY_KEY="..."               # required
export LARKSUITE_CLI_APP_ID="cli_xxx"              # required
export LARKSUITE_CLI_BRAND="feishu"                # required (feishu | lark)
export LARKSUITE_CLI_DEFAULT_AS="user"             # optional: force default identity
export LARKSUITE_CLI_STRICT_MODE="user"            # optional: lock sandbox to one identity
```

**`LARKSUITE_CLI_AUTH_PROXY` constraints** — validated by the CLI on startup:

- Scheme must be `http://` (or bare `host:port`). `https://` is rejected
  today because the interceptor does not yet perform TLS; a future PR that
  wires up real TLS will relax this.
- Host must be loopback (`127.0.0.1`, `::1`) or one of the recognized
  same-host aliases: `localhost`, `host.docker.internal`,
  `host.containers.internal`, `host.lima.internal`, `gateway.docker.internal`.
  The sidecar pattern is inherently same-machine; cross-machine deployment
  is a different product (auth broker / STS) with different security
  requirements (mTLS, cert rotation, per-client keys) and is not supported
  by this feature.
- No path, query, fragment, or `user:pass@` in the URL.

**How auto identity detection works in sidecar mode**: on every invocation the
CLI asks the sidecar to look up the logged-in user's `open_id` via
`/open-apis/authen/v1/user_info`. If that succeeds, `--as` defaults to `user`;
if it fails (trusted side has no valid user login, or the call errors out),
it falls back to `bot`. Setting `LARKSUITE_CLI_DEFAULT_AS=user` lets you
short-circuit this and always default to user regardless of the lookup
result; set it to `bot` for the opposite.

**Note**: `LARKSUITE_CLI_STRICT_MODE` and the server's identity allowlist are
two separate enforcement points:
- `STRICT_MODE` is interpreted locally by the sandbox CLI — it rejects
  `--as` values the sandbox itself disallows, before any request goes out.
- The server's allowlist is built from the **trusted-side** config's
  `SupportedIdentities` (`sidecar/server-demo/allowlist.go`). The sandbox
  cannot override it.

A well-configured deployment aligns both (e.g. both set to `user` when the
app only supports user tokens), but they are computed independently.

### Graceful shutdown

Send `SIGINT` (`Ctrl+C`) or `SIGTERM` to stop the server. The demo drains
in-flight requests with a 5-second timeout before exiting.

## Wire protocol

See the [`sidecar` package on pkg.go.dev](https://pkg.go.dev/github.com/larksuite/cli/sidecar)
for protocol constants, HMAC signing/verification, and address validation utilities.

Headers (client → server):

| Header | Purpose |
| --- | --- |
| `X-Lark-Proxy-Version` | Wire-protocol version (currently `"v1"`). Server rejects unknown values with 400. |
| `X-Lark-Proxy-Target` | Original target **scheme + host only** (e.g. `https://open.feishu.cn`). Must be `https://`; any path/query/fragment/userinfo in this header is rejected. The path and query come from the request line itself; the server reconstructs the upstream URL as `https://<host> + requestURI`. |
| `X-Lark-Proxy-Identity` | `"user"` or `"bot"`. Covered by the signature. |
| `X-Lark-Proxy-Auth-Header` | Which header the server should inject real token into. Covered by the signature. |
| `X-Lark-Proxy-Signature` | hex-encoded HMAC-SHA256 |
| `X-Lark-Proxy-Timestamp` | Unix seconds (drift ≤ 60s) |
| `X-Lark-Body-SHA256` | hex-encoded SHA-256 of the request body |

Signing material (newline-separated, in order):

```text
version
method
host
pathAndQuery
bodySHA256
timestamp
identity
authHeader
```

Every field above is part of the canonical string. In particular, `identity`
and `authHeader` are covered so a captured request cannot be replayed with
its identity flipped (bot↔user) or its auth-header redirected (e.g. into
`Cookie`) inside the 60s drift window.

## Source layout

| File | Purpose |
| --- | --- |
| `main.go` | Entry point: flag parsing, server lifecycle |
| `handler.go` | `proxyHandler.ServeHTTP` — main request flow |
| `forward.go` | Forwarding HTTP client + proxy-header filter |
| `allowlist.go` | Target host / identity allowlists |
| `audit.go` | Log path/error sanitization |
| `handler_test.go` | Unit tests for all of the above |
