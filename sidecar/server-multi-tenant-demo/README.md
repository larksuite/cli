# Multi-Tenant Sidecar Server Demo

> ⚠️ **This is a demo.** For production deployment, implement your own sidecar
> server conforming to the wire protocol in `github.com/larksuite/cli/sidecar`.

## Problem

Organizations often manage **multiple Lark/Feishu apps** (e.g. one per
department, one per product line), each with its own `app_id` and `app_secret`.
These credentials must never be exposed to end-user environments (CI runners,
developer sandboxes, containerized workspaces). At the same time, when multiple
users share the same sidecar infrastructure, their Feishu identities must be
strictly isolated — user A must never accidentally operate as user B.

The single-tenant [server-demo](../server-demo/) solves the credential-hiding
problem for **one app with one user**. This multi-tenant demo extends it to
support:

1. **Multiple apps** — run one sidecar instance per app; each instance holds
   its own `app_id` / `app_secret` and listens on a separate port. Clients
   choose which app to use by pointing `LARKSUITE_CLI_AUTH_PROXY` to the
   corresponding port.
2. **Per-client identity isolation** — each client environment gets a unique
   HMAC key. The sidecar identifies request origin by matching the HMAC
   signature and injects the correct user's token. No fallback to other
   users' tokens.
3. **Self-service user login** — management endpoints let each client initiate
   an OAuth device-flow login to bind their own Feishu identity, without
   exposing `app_secret` to the client.

## Typical deployment

```text
                    Trusted Host
    ┌──────────────────────────────────────────────┐
    │  sidecar instance A (port 16384)             │
    │    app_id=cli_aaa  app_secret=***            │
    │    keys/proxy.key  keys/alice.key  keys/bob… │
    │                                              │
    │  sidecar instance B (port 16385)             │
    │    app_id=cli_bbb  app_secret=***            │
    │    keys/proxy.key  keys/charlie.key  ...     │
    └─────────────┬────────────────────────────────┘
                  │ same machine (loopback / docker bridge)
    ┌─────────────┴────────────────────────────────┐
    │  Client sandbox (container / CI runner)       │
    │                                              │
    │  LARKSUITE_CLI_AUTH_PROXY=http://host:16384   │
    │  LARKSUITE_CLI_PROXY_KEY=<contents of         │
    │                           alice.key>          │
    │  LARKSUITE_CLI_APP_ID=cli_aaa                │
    │  LARKSUITE_CLI_BRAND=feishu                  │
    │                                              │
    │  $ lark api GET /open-apis/... --as user     │
    │    → sidecar matches alice.key               │
    │    → injects alice's Feishu user token       │
    └──────────────────────────────────────────────┘
```

**Key points:**

- `app_id` and `app_secret` live only on the trusted host — clients only
  know `app_id` (needed for the CLI's credential pipeline) and their own
  HMAC key.
- Each sidecar instance binds one app. Multiple apps = multiple instances
  on different ports.
- Clients select which app to use by choosing which sidecar port to connect
  to (via `LARKSUITE_CLI_AUTH_PROXY`).

## Architecture

```text
┌──────────────────────────────────────────────────────┐
│                    Sidecar Server                     │
│                                                      │
│  ┌─────────────┐  ┌──────────────────────────────┐  │
│  │ Shared Key   │  │ Per-Client Keys              │  │
│  │ (proxy.key)  │  │ alice.key, bob.key, ...      │  │
│  └──────┬──────┘  └──────────────┬───────────────┘  │
│         │ management plane       │ data plane        │
│         ▼                        ▼                   │
│  ┌─────────────┐  ┌──────────────────────────────┐  │
│  │ Auth Bridge  │  │ Proxy Handler                │  │
│  │ login/poll/  │  │ HMAC verify → identify       │  │
│  │ status       │  │ client → inject user token   │  │
│  └─────────────┘  └──────────────────────────────┘  │
└──────────────────────────────────────────────────────┘
```

**Dual-key design:**
- **Management plane** (login flow): all clients use the shared `proxy.key`.
  This allows any client to initiate login and query status without needing
  individual key files pre-provisioned.
- **Data plane** (API proxy): each client uses its own `{name}.key` for HMAC
  signing. The sidecar identifies the client by matching which key verifies
  the request signature, then injects that client's bound user token.

## Build

```bash
go build -tags authsidecar_multi_tenant_demo \
  -o sidecar-multi-tenant-demo \
  ./sidecar/server-multi-tenant-demo/
```

## Server setup

### 1. Configure the Lark app (trusted side only)

```bash
lark-cli config init --new   # set app_id / app_secret
```

### 2. Prepare the keys directory

```text
keys/
├── proxy.key          # shared key (auto-generated on first run)
├── alice.key          # client "alice" — generate with: openssl rand -hex 32 > alice.key
├── bob.key            # client "bob"
└── charlie.key        # client "charlie"
```

- Each file contains a 64-character hex string (32 bytes).
- Filename stem (without `.key`) becomes the client identity.
- `proxy.key` is excluded from client key scanning.
- Keys are auto-rescanned on cache miss — add a new `.key` file and the next
  unrecognized request will trigger a rescan; no restart needed.
- Duplicate key values and shared-key collisions are rejected with a warning.

### 3. Start the server

```bash
./sidecar-multi-tenant-demo \
  --listen 127.0.0.1:16384 \
  --key-file /path/to/keys/proxy.key \
  --keys-dir /path/to/keys/ \
  --log-file /path/to/audit.log
```

| Flag | Default | Purpose |
| --- | --- | --- |
| `--listen` | `127.0.0.1:16384` | Address to bind the HTTP listener |
| `--key-file` | `~/.lark-sidecar/proxy.key` | Shared HMAC key path (created if absent) |
| `--keys-dir` | *(parent of `--key-file`)* | Directory containing per-client `*.key` files |
| `--log-file` | *(stderr)* | Audit log output path |
| `--profile` | *(active profile)* | lark-cli profile name for credential lookup |

## Client setup

**No changes to `lark-cli` itself are required.** The standard sidecar env
vars are all that's needed — the multi-tenant isolation is entirely
server-side.

### Required environment variables

```bash
# Point to the sidecar instance for the desired app
export LARKSUITE_CLI_AUTH_PROXY="http://127.0.0.1:16384"

# Client-specific HMAC key (data-plane identity)
export LARKSUITE_CLI_PROXY_KEY="$(cat /path/to/keys/alice.key)"

# Must match the app configured on the sidecar instance
export LARKSUITE_CLI_APP_ID="cli_xxx"

# feishu or lark
export LARKSUITE_CLI_BRAND="feishu"
```

### Multi-app switching (multiple sidecar instances)

When the server operator runs multiple sidecar instances (one per app), clients
switch between apps by changing `LARKSUITE_CLI_AUTH_PROXY` to point to the
appropriate port:

```bash
# App A (e.g. "Marketing" app)
export LARKSUITE_CLI_AUTH_PROXY="http://127.0.0.1:16384"
export LARKSUITE_CLI_APP_ID="cli_marketing_app"

# App B (e.g. "Engineering" app)
export LARKSUITE_CLI_AUTH_PROXY="http://127.0.0.1:16385"
export LARKSUITE_CLI_APP_ID="cli_engineering_app"
```

A client-side helper script can present these as a menu (e.g. "Select
company"), reading from a local config file that maps app names to ports.
The sidecar itself does not implement app selection — it is one instance per
app by design.

### User login flow

Once the env vars are set, the client authenticates via the management
endpoints. A helper script (or manual `curl`) calls:

1. **Login**: `POST /_sidecar/auth/login` with `{"client_id": "alice"}` →
   returns a device code and verification URL.
2. **User opens the URL in a browser** and authorizes the app.
3. **Poll**: `POST /_sidecar/auth/poll` with `{"device_code": "...", "client_id": "alice"}` →
   blocks until authorization completes.
4. **Status**: `POST /_sidecar/auth/status` with `{"client_id": "alice"}` →
   returns the bound user name and token status.

All management requests are signed with the **shared `proxy.key`** (not the
client-specific key). The `client_id` in the body tells the sidecar which
client→user mapping to update.

After login, `lark-cli` commands (`lark api ...`, `lark doc ...`, etc.) work
immediately — the sidecar injects the correct user token based on the
client's HMAC key, with no additional configuration needed.

### Example: end-to-end workflow

```bash
# 1. Server operator generates a key for a new client
openssl rand -hex 32 > /path/to/keys/alice.key

# 2. Client environment is configured (e.g. in .bashrc or container init)
export LARKSUITE_CLI_AUTH_PROXY="http://host.docker.internal:16384"
export LARKSUITE_CLI_PROXY_KEY="$(cat /path/to/keys/alice.key)"
export LARKSUITE_CLI_APP_ID="cli_xxx"
export LARKSUITE_CLI_BRAND="feishu"

# 3. Client logs in (one-time)
#    (using a helper script that calls the management endpoints)
lark-auth login

# 4. Client uses lark-cli as normal — identity is automatically resolved
lark api GET /open-apis/authen/v1/user_info --as user
# → returns alice's Feishu identity, not another user's
```

## Management endpoints

| Endpoint | Method | Body | Purpose |
| --- | --- | --- | --- |
| `/_sidecar/auth/login` | POST | `{"client_id": "...", "domains": [...]}` | Start OAuth device-flow |
| `/_sidecar/auth/poll` | POST | `{"device_code": "...", "client_id": "..."}` | Poll for completion |
| `/_sidecar/auth/status` | POST | `{"client_id": "..."}` | Query status and mapping |

All management requests require HMAC signing with the shared `proxy.key`.
The HMAC covers method, path, timestamp, and body SHA-256 — see
`verifyManagementHMAC` in `auth_bridge.go` for the canonical string format.

## Design decisions

1. **HMAC key as client identity** — the key is the existing trust anchor.
   Using it for identification introduces no new trust assumptions and
   prevents a malicious client from spoofing another client's identity
   (unlike a header-based approach).

2. **No fallback on unmapped clients** — this is authentication. Silently
   falling back to another user's token is a security violation. Unmapped
   clients receive an explicit error prompting them to log in.

3. **One sidecar instance per app** — keeps `app_secret` scoping simple and
   avoids cross-app token confusion. Multi-app support is achieved by running
   multiple instances on different ports.

4. **Proxy.key reuse across restarts** — when multiple sidecar instances start
   concurrently, they all write to the same key file. The last writer wins,
   leaving other instances with stale in-memory keys. Reusing the existing
   key eliminates this race.

## Source layout

| File | Purpose |
| --- | --- |
| `main.go` | Entry point: flag parsing, key loading, server lifecycle |
| `handler.go` | `proxyHandler.ServeHTTP` — multi-key HMAC verification and request forwarding |
| `auth_bridge.go` | Management endpoints: login, poll, status, user mapping persistence |
| `forward.go` | Forwarding HTTP client + proxy-header filter |
| `allowlist.go` | Target host / identity allowlists |
| `audit.go` | Log path/error sanitization |
| `handler_test.go` | Unit tests |

## See also

- [server-demo](../server-demo/) — single-tenant minimal implementation
- [`sidecar` package](https://pkg.go.dev/github.com/larksuite/cli/sidecar) — wire protocol
