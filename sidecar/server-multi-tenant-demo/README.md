# Multi-Tenant Sidecar Server Demo

> ⚠️ **This is a demo.** For production deployment, implement your own sidecar
> server conforming to the wire protocol in `github.com/larksuite/cli/sidecar`.

## Problem

Organizations often manage **multiple Lark/Feishu apps** (e.g. one per
department, one per product line), each with its own `app_id` and `app_secret`.
These credentials must never be exposed to end-user environments (CI runners,
developer sandboxes, containers). At the same time, when multiple users share
the same sidecar infrastructure, their Feishu identities must be strictly
isolated — user A must never accidentally operate as user B.

Additionally, in containerized or sandboxed deployments the sidecar's `keys/`
directory is frequently a **read-only host mount**. Pre-creating a `.key` file
for every new client before it starts is a manual bottleneck that prevents
fully automated provisioning and elastic scaling.

The single-tenant [server-demo](../server-demo/) solves the credential-hiding
problem for **one app with one user**. This multi-tenant demo extends it to
support:

1. **Multiple organizations / apps** — run one sidecar instance per app; each
   instance holds its own `app_id` / `app_secret` and listens on a separate
   port. Clients choose which app (organization) to use by pointing
   `LARKSUITE_CLI_AUTH_PROXY` to the corresponding port.
2. **Per-client identity isolation** — each client environment gets a unique
   HMAC key. The sidecar identifies request origin by matching the HMAC
   signature and injects the correct user's token. No fallback to other
   users' tokens.
3. **Client key self-registration** — clients generate their own HMAC key
   locally and register it with the sidecar via `/_sidecar/keys/register`.
   The server operator only distributes the shared `proxy.key` (read-only);
   no per-client files need to be pre-created on the server.
4. **Self-service user login** — management endpoints let each client initiate
   an OAuth device-flow login to bind their own Feishu identity, without
   exposing `app_secret` to the client.

With these capabilities, `lark-cli` users in multi-tenant environments can
use all skills (docs, sheets, im, calendar, etc.) and auth features without
any changes to `lark-cli` itself — the isolation is entirely server-side.

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
    │  LARKSUITE_CLI_PROXY_KEY=<per-client key>    │
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
- Client keys can be **self-registered** via the `/_sidecar/keys/register`
  API — no server-side pre-provisioning required (see [Client setup](#client-setup) below).

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
- **Management plane** (login / key registration): all clients use the shared
  `proxy.key`. This allows any client to register its own key and initiate
  login without needing individual key files pre-provisioned by an admin.
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

The server needs only a `proxy.key` (auto-generated on first run). Client
keys are registered on demand via the API — no manual file creation needed.

```text
keys/
└── proxy.key          # shared management-plane key (auto-generated on first run)
```

If you prefer to pre-provision keys manually (e.g. for offline environments):

```bash
openssl rand -hex 32 > keys/alice.key
openssl rand -hex 32 > keys/bob.key
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
server-side. Any `lark-cli` skill (`lark doc`, `lark sheets`, `lark im`,
`lark calendar`, `lark api`, etc.) works transparently once the client is
configured.

### Overview

The client-side flow has three steps, typically handled by an **init script**
that runs once when the client environment (container, sandbox, CI runner)
starts:

1. **Generate** a client-specific HMAC key (stored locally in a writable path, never shared)
2. **Register** the key with the sidecar via `/_sidecar/keys/register`
3. **Login** once to bind the client's Feishu/Lark identity

After that, all `lark-cli` commands work without further configuration.

> **Why self-registration?** In environments where the `keys/` directory is
> a read-only host mount (common in container orchestrators), clients cannot
> write key files directly. The registration API lets each client generate
> its key in a writable location (e.g. `$HOME/.lark-sidecar/client.key`) and
> register it with the server over the management-plane HMAC channel.
> The server persists the key to disk and hot-loads it — no restart needed.

### Required environment variables

```bash
# Point to the sidecar instance for the desired app
export LARKSUITE_CLI_AUTH_PROXY="http://127.0.0.1:16384"

# Client-specific HMAC key (data-plane identity) — generated locally
export LARKSUITE_CLI_PROXY_KEY="<64-char hex string>"

# Must match the app configured on the sidecar instance
export LARKSUITE_CLI_APP_ID="cli_xxx"

# feishu or lark
export LARKSUITE_CLI_BRAND="feishu"
```

### Client init script

Integrators should add an init script to their client environment startup
(e.g. container entrypoint, CI pipeline setup, sandbox bootstrap). The
following reference implementation can be adapted. It is **idempotent**:
re-running it (e.g. on restart) is safe.

```bash
#!/usr/bin/env bash
# client-init.sh — run once at container/sandbox startup

SIDECAR_HOST="127.0.0.1"          # or host.docker.internal inside Docker
SIDECAR_PORT="16384"               # port of the sidecar instance for your app
MGMT_KEY_FILE="/path/to/shared/proxy.key"   # read-only shared key from the server operator
CLIENT_KEY_FILE="$HOME/.lark-sidecar/client.key"
CLIENT_ID="$(hostname)"            # unique name for this client environment

# ── Step 1: generate a client-specific key if not yet present ──────────────
mkdir -p "$(dirname "$CLIENT_KEY_FILE")"
if [ ! -f "$CLIENT_KEY_FILE" ]; then
  python3 -c "import secrets; print(secrets.token_hex(32))" > "$CLIENT_KEY_FILE"
  chmod 600 "$CLIENT_KEY_FILE"
fi
CLIENT_KEY="$(cat "$CLIENT_KEY_FILE")"

# ── Step 2: register the key with the sidecar (idempotent) ─────────────────
hmac_sign() {
  local method="$1" path="$2" body="$3"
  local ts body_sha canonical sig
  ts=$(date +%s)
  body_sha=$(printf '%s' "$body" | sha256sum | cut -d' ' -f1)
  canonical="sidecar-mgmt\n${method}\n${path}\n${ts}\n${body_sha}"
  sig=$(printf '%b' "$canonical" \
    | openssl dgst -sha256 -hmac "$(cat "$MGMT_KEY_FILE")" -hex 2>/dev/null \
    | sed 's/.* //')
  echo "$ts $body_sha $sig"
}

call_sidecar() {
  local method="$1" path="$2" body="${3:-}"
  read -r ts body_sha sig <<< "$(hmac_sign "$method" "$path" "$body")"
  curl -sf --max-time 10 -X "$method" \
    "http://${SIDECAR_HOST}:${SIDECAR_PORT}${path}" \
    -H "Content-Type: application/json" \
    -H "X-Sidecar-Timestamp: $ts" \
    -H "X-Sidecar-Body-SHA256: $body_sha" \
    -H "X-Sidecar-Signature: $sig" \
    ${body:+-d "$body"}
}

REGISTER_BODY=$(python3 -c "
import json
print(json.dumps({'client_id': '$CLIENT_ID', 'key_hex': '$CLIENT_KEY'}))
")
REGISTER_RESULT=$(call_sidecar POST /_sidecar/keys/register "$REGISTER_BODY" 2>&1)
echo "[init] key registration: $REGISTER_RESULT"

# ── Step 3: export env vars for this session ───────────────────────────────
export LARKSUITE_CLI_AUTH_PROXY="http://${SIDECAR_HOST}:${SIDECAR_PORT}"
export LARKSUITE_CLI_PROXY_KEY="$CLIENT_KEY"
export LARKSUITE_CLI_APP_ID="cli_xxx"    # fill in your app_id
export LARKSUITE_CLI_BRAND="feishu"
```

### Multi-organization switching (multiple sidecar instances)

When an operator manages multiple organizations (each with its own Lark app),
one sidecar instance runs per organization. Register the client key with
**every instance** during init. At runtime, the user switches organizations
by changing `LARKSUITE_CLI_AUTH_PROXY` and `LARKSUITE_CLI_APP_ID` — the same
client key works across all instances because each independently verifies it.

```bash
# Register with all organization instances during init
for PORT in 16384 16385; do
  call_sidecar POST /_sidecar/keys/register "$REGISTER_BODY" "$SIDECAR_HOST" "$PORT"
done

# Switch to Organization A at runtime
export LARKSUITE_CLI_AUTH_PROXY="http://127.0.0.1:16384"
export LARKSUITE_CLI_APP_ID="cli_aaa"

# Switch to Organization B at runtime
export LARKSUITE_CLI_AUTH_PROXY="http://127.0.0.1:16385"
export LARKSUITE_CLI_APP_ID="cli_bbb"
```

A helper script can present these as a menu (e.g. "Select organization"),
reading from a config file that maps organization names to ports. The sidecar
itself does not implement organization selection — it is one instance per
app by design.

### User login flow

After key registration, the client authenticates once to bind their Feishu
identity. A helper script (or manual `curl`) calls:

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

### Example: full end-to-end workflow

```bash
# ── Server side (one-time setup, on the trusted host) ─────────────────────
./sidecar-multi-tenant-demo \
  --listen 0.0.0.0:16384 \
  --key-file /srv/lark-sidecar/keys/proxy.key

# Distribute proxy.key to clients (read-only; used for management-plane signing)

# ── Client side: init (runs on every container/sandbox start) ─────────────
# (see "Client init script" section above for the full reference script)
./client-init.sh
# → generates ~/.lark-sidecar/client.key  (if not present)
# → registers key with sidecar: {"ok":true,"status":"registered"}
# → exports LARKSUITE_CLI_* env vars

# ── Client side: first-time login (once per user per sidecar instance) ────
# Use a helper script or call the management endpoints directly:
curl -X POST ... /_sidecar/auth/login   # → returns device_code + verification URL
# → user opens URL in browser and authorizes the app
curl -X POST ... /_sidecar/auth/poll    # → blocks until authorization completes
# → sidecar stores: client_id → feishu open_id

# ── Client side: daily usage (all lark-cli skills work transparently) ─────
lark api GET /open-apis/authen/v1/user_info --as user
# → sidecar matches client key → injects user's token → returns user's info

lark doc fetch <doc_token>
# → sidecar injects user's token → fetches document content

lark api POST /open-apis/im/v1/messages \
  --body '{"receive_id":"...","msg_type":"text","content":"{\"text\":\"hello\"}"}' \
  --as bot
# → sidecar injects app-level token → sends message as the app bot
```

## Management endpoints

| Endpoint | Method | Body | Purpose |
| --- | --- | --- | --- |
| `/_sidecar/keys/register` | POST | `{"client_id": "...", "key_hex": "..."}` | Register a per-client HMAC key |
| `/_sidecar/auth/login` | POST | `{"client_id": "...", "domains": [...]}` | Start OAuth device-flow |
| `/_sidecar/auth/poll` | POST | `{"device_code": "...", "client_id": "..."}` | Poll for completion |
| `/_sidecar/auth/status` | POST | `{"client_id": "..."}` | Query status and mapping |

All management requests require HMAC signing with the shared `proxy.key`.
The HMAC covers method, path, timestamp, and body SHA-256 — see
`verifyManagementHMAC` in `auth_bridge.go` for the canonical string format.

### Key registration details

`/_sidecar/keys/register` is **idempotent**:

| Case | HTTP status | Response body |
| --- | --- | --- |
| New key | 200 | `{"ok":true,"status":"registered"}` |
| Same key already registered | 200 | `{"ok":true,"status":"already_registered"}` |
| Different key already registered | 409 | `{"ok":false,"error":"key_conflict: ..."}` |

On conflict (409), the client should retain the original key and retry
registration with the existing key, or contact the server operator to resolve
the conflict.

## Design decisions

1. **HMAC key as client identity** — the key is the existing trust anchor.
   Using it for identification introduces no new trust assumptions and
   prevents a malicious client from spoofing another client's identity
   (unlike a header-based approach).

2. **No fallback on unmapped clients** — this is authentication. Silently
   falling back to another user's token is a security violation. Unmapped
   clients receive an explicit error prompting them to log in.

3. **Client self-registration** — clients generate their own key locally and
   register it with the sidecar. The server operator distributes only
   `proxy.key` (read-only), eliminating manual per-client provisioning.

4. **One sidecar instance per app (organization)** — keeps `app_secret`
   scoping simple and avoids cross-app token confusion. Multi-organization
   support is achieved by running multiple instances on different ports.

5. **Proxy.key reuse across restarts** — when multiple sidecar instances start
   concurrently, they all write to the same key file. The last writer wins,
   leaving other instances with stale in-memory keys. Reusing the existing
   key eliminates this race.

## Source layout

| File | Purpose |
| --- | --- |
| `main.go` | Entry point: flag parsing, key loading, server lifecycle |
| `handler.go` | `proxyHandler.ServeHTTP` — multi-key HMAC verification, hot-load, and request forwarding |
| `auth_bridge.go` | Management endpoints: key registration, login, poll, status, user mapping persistence |
| `forward.go` | Forwarding HTTP client + proxy-header filter |
| `allowlist.go` | Target host / identity allowlists |
| `audit.go` | Log path/error sanitization |
| `handler_test.go` | Unit tests |

## See also

- [server-demo](../server-demo/) — single-tenant minimal implementation
- [`sidecar` package](https://pkg.go.dev/github.com/larksuite/cli/sidecar) — wire protocol
