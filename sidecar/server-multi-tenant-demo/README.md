# Multi-Tenant Sidecar Server Demo

> ⚠️ **This is a demo.** For production deployment, implement your own sidecar
> server conforming to the wire protocol in `github.com/larksuite/cli/sidecar`.

This example extends the single-tenant [server-demo](../server-demo/) with
**per-client identity isolation**, allowing multiple CLI sandbox environments to
share one sidecar process while maintaining separate Feishu/Lark user identities.

## Why a separate demo?

The [server-demo](../server-demo/) shows the minimal sidecar implementation:
one shared HMAC key, one set of credentials. This multi-tenant demo adds:

- **Per-client HMAC key isolation** — each client gets a unique `.key` file;
  the sidecar identifies request origin by matching the HMAC signature.
- **OAuth device-flow login bridge** — management endpoints (`login`, `poll`,
  `status`) let each client bind their own Feishu user identity.
- **Persistent client → user mapping** — survives sidecar restarts.
- **Strict identity enforcement** — unmapped clients receive an explicit error
  instead of silently falling back to another user's token.

## Architecture

```
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
- **Data plane** (API proxy): each client uses its own `{name}.key` for HMAC
  signing; the sidecar identifies the client by matching which key verifies.

## Build

```bash
go build -tags authsidecar_multi_tenant_demo \
  -o sidecar-multi-tenant-demo \
  ./sidecar/server-multi-tenant-demo/
```

## Run

```bash
./sidecar-multi-tenant-demo \
  --listen 127.0.0.1:16384 \
  --key-file /path/to/keys/proxy.key \
  --keys-dir /path/to/keys/ \
  --log-file /path/to/audit.log
```

### Flags (additions over server-demo)

| Flag | Default | Purpose |
| --- | --- | --- |
| `--keys-dir` | *(parent of `--key-file`)* | Directory containing per-client `*.key` files for identity isolation |

All other flags are identical to [server-demo](../server-demo/README.md#flags).

### Key directory layout

```
keys/
├── proxy.key          # shared key (management plane)
├── alice.key          # client "alice" (data plane)
├── bob.key            # client "bob" (data plane)
└── charlie.key        # client "charlie" (data plane)
```

- Filename stem (without `.key`) becomes the client identity.
- `proxy.key` is excluded from client key scanning.
- Keys are auto-rescanned on cache miss (no restart needed for new clients).
- Duplicate key values and shared-key collisions are rejected with a log warning.

## Client setup

**Data plane** (normal API requests):
```bash
export LARKSUITE_CLI_PROXY_KEY="$(cat /path/to/keys/alice.key)"
```

**Management plane** (login/status via auth bridge script):
- Uses `proxy.key` for HMAC signing.
- Passes `{"client_id": "alice"}` in the request body.

## Management endpoints

| Endpoint | Method | Purpose |
| --- | --- | --- |
| `/_sidecar/auth/login` | POST | Start OAuth device-flow login |
| `/_sidecar/auth/poll` | POST | Poll for login completion |
| `/_sidecar/auth/status` | POST | Query auth status and user mapping |

All management requests require HMAC signing with the shared `proxy.key`.

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
