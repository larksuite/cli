# External credential platform integration

The Extended edition of `lark-cli` supports temporary sandboxes whose user
identity and Feishu/Lark credentials are managed by an external platform. The
Standard npm/npx edition does not contain the credential process, proxy
Transport, or external-platform integration.

## Modes

| Mode | Request path | Credential in the sandbox |
| --- | --- | --- |
| `platform_proxy` | CLI → external platform → Feishu/Lark | None. The platform authenticates the sandbox outside the CLI request. |
| `credential_proxy` | CLI → external platform → Feishu/Lark | A short-lived external-platform bearer. Real UAT/TAT values stay remote. |
| `direct` | CLI → Feishu/Lark | The external program returns UAT/TAT to the current CLI process. |

Proxy modes cover OpenAPI, HTTP streams, and opaque file handles.
`direct` is an environment-variable replacement, not credential isolation:
another process with the same sandbox permissions may invoke the external
program directly.

Real-time event consumption is intentionally unavailable with externally
managed credentials in protocol v1. `event consume`, the hidden event bus,
legacy `event +subscribe`, and `mail +watch` fail closed for all three modes;
read-only event schema/list/status and local bus cleanup remain available.

## Configuration

An administrator installs one closed-schema system file:

| OS | Path |
| --- | --- |
| Linux | `/etc/lark-cli/external-credential.json` |
| macOS | `/Library/Application Support/lark-cli/external-credential.json` |
| Windows | `%ProgramData%\lark-cli\external-credential.json` |

The user Profile remains ordinary application selection:

```json
{
  "currentApp": "sandbox",
  "apps": [{
    "name": "sandbox",
    "appId": "cli_xxx",
    "brand": "feishu",
    "defaultAs": "user",
    "users": []
  }]
}
```

The Profile must not contain `appSecret`, logged-in users, or an
`externalCredential` field. The selected `brand` and `appId` must appear in
the system file. The deploying integrator provisions this secretless selector;
credential-backed `profile add`, login, and migration commands are not an
external-platform bootstrap mechanism.

While the managed runtime is active, the deploying integrator also owns
persistent Profile selection and identity settings. `profile add`, `profile
use`, `profile rename`, `profile remove`, `config default-as`, and `config
strict-mode` therefore fail before changing local state. `profile list`
remains available for local inspection. Workspace policy is a separate
boundary: `config risk-control`, `config policy`, and `config plugins` remain
locally available because they neither select application identity nor manage
credentials.

### Platform-authenticated proxy

```json
{
  "version": 1,
  "mode": "platform_proxy",
  "remoteEndpoint": "https://credentials.example",
  "applications": [{"brand": "feishu", "appId": "cli_xxx"}]
}
```

This mode has no external program and sends no `Authorization` header to the
proxy. It is valid only when the runtime platform can identify and authorize
the sandbox allocation outside request-controlled data.

### Short-lived credential proxy

```json
{
  "version": 1,
  "mode": "credential_proxy",
  "remoteEndpoint": "https://credentials.example",
  "program": {
    "executable": "/opt/platform/bin/lark-credential",
    "arguments": ["get"],
    "sha256": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "protocolVersion": 1,
    "timeoutSeconds": 5
  },
  "applications": [{"brand": "feishu", "appId": "cli_xxx"}]
}
```

The program returns a short-lived bearer accepted only by the external
platform. The proxy injects the real Feishu/Lark credential after validating
the bearer and the requested identity.

### Direct credential

```json
{
  "version": 1,
  "mode": "direct",
  "program": {
    "executable": "/opt/platform/bin/lark-credential",
    "arguments": ["get"],
    "sha256": "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "protocolVersion": 1,
    "timeoutSeconds": 5
  },
  "applications": [{"brand": "feishu", "appId": "cli_xxx"}]
}
```

`direct` must not configure `remoteEndpoint`.

## External program protocol v1

The CLI executes the configured absolute path directly, without a shell or
`PATH` lookup. It writes one UTF-8 JSON value to stdin and reads one JSON value
from stdout. The executable must be a native binary, must not be a symlink or
interpreter script, and must match the configured SHA-256 before every
execution.

The helper starts in the executable's administrator-controlled parent
directory and does not inherit the caller's environment. Linux and macOS pass
an empty environment; Windows passes only `SYSTEMROOT` and `WINDIR` values
resolved through the operating-system API. Helper dependencies must be
expressed through administrator-controlled arguments or files, not `PATH`,
loader, proxy, CA, home, temporary-directory, or language environment values.

The system file, executable, and their parent directories must be controlled
by root on Linux/macOS or LocalSystem/Administrators on Windows. The CLI
rejects writable paths and unsafe Windows ACLs. The external program must not
wait for a TTY or write credentials to arguments, stderr, logs, or error text.

### Direct UAT/TAT

CLI request:

```json
{"version":1,"mode":"direct","credential_type":"access_token","app_id":"cli_xxx","brand":"feishu","identity":"user"}
```

Program response:

```json
{"version":1,"credential":{"token_type":"uat","access_token":"u-example","expires_at":"2030-01-01T10:15:00Z","scopes":["docx:document"]}}
```

For `identity=bot`, `token_type` must be `tat`.

### Short-lived proxy bearer

CLI request:

```json
{"version":1,"mode":"credential_proxy","credential_type":"proxy_access_token","app_id":"cli_xxx","brand":"feishu","identity":"user","remote_endpoint":"https://credentials.example"}
```

Program response:

```json
{"version":1,"credential":{"scheme":"bearer","access_token":"proxy-example","expires_at":"2030-01-01T10:15:00Z"}}
```

Both timestamp examples assume the request is made at
`2030-01-01T10:10:00Z`. Producers calculate `expires_at` from the actual
request time.

Every `expires_at` uses RFC 3339 and must be more than 60 seconds in the
future. Only the short-lived `credential_proxy` bearer is capped at one hour;
direct UAT/TAT credentials are not subject to that proxy-specific upper bound.
The CLI caches credentials only in the current process and refreshes them 60
seconds before expiry. Cross-command caching belongs to the external program
or platform.

### Program error

On failure the program exits non-zero and may return:

```json
{"version":1,"error":{"code":"temporarily_unavailable","message":"identity service is unavailable"}}
```

Protocol v1 accepts `temporarily_unavailable`, `access_denied`,
`invalid_request`, and `unsupported_identity`. Responses are limited to
64 KiB. Unknown versions, fields, credential types, or mixed
`credential`/`error` responses are rejected without falling back to another
credential source.

## Proxy protocol v1

OpenAPI and HTTP-stream requests are forwarded with the original method,
query, body, and target recorded in `X-Lark-CLI-Original-Target`. The proxy
endpoint is:

```text
/lark-cli/v1/openapi/open-apis/...
```

Every proxy request includes:

```text
X-Lark-Proxy-Version: 1
X-Lark-CLI-App-ID: cli_xxx
X-Lark-CLI-Identity: user|bot
X-Lark-CLI-Request-ID: <uuid>
```

`credential_proxy` also includes
`Authorization: Bearer <short-lived-proxy-credential>`.
`platform_proxy` explicitly removes credential headers and does not add an
Authorization header. App ID and identity are requested values; the external
platform must validate them against its own trusted sandbox/session binding.

File APIs return opaque same-origin handles:

```text
https://credentials.example/lark-cli/v1/files/<handle>
```

Raw presigned object-storage URLs are rejected in proxy modes. The platform
binds each handle to the authorized application, identity, session, operation,
and expiry.

Marked proxy failures use `X-Lark-CLI-Proxy-Error: 1` and:

```json
{
  "version": 1,
  "error": {
    "code": "access_denied",
    "stage": "policy",
    "message": "blocked by tenant policy",
    "request_id": "proxy_req_123",
    "upstream_started": false
  }
}
```

The CLI emits the platform request id as `proxy_request_id`. Numeric `code` and
`log_id` are populated only from a real Feishu/Lark response. Redirects,
unmarked malformed responses, unsupported request surfaces, and direct
requests to Feishu/Lark hosts fail closed.

## Credential-source isolation

When the system file exists, all three modes reject Profile secrets and users,
credential or identity environment variables, keychain/OAuth fallback, and
additional compile-time credential providers. In `credential_proxy` and
`platform_proxy`, the CLI additionally rejects non-empty
`LARKSUITE_CLI_PROXY_*`/`LARKSUITE_CLI_CA_PATH` overrides, an enabled
`proxy_config.json`, and compile-time Transport extensions so they cannot form
a competing application data plane. `direct` replaces credential resolution
only: it retains the ordinary local Transport chain, including those CLI proxy
settings and Transport extensions, and does not provide managed data-plane
isolation. Operating-system networking such as `HTTP(S)_PROXY` and the system
certificate trust store remains part of the base Transport in every mode; the
proxy-mode boundary guarantees application routing to the managed endpoint,
not isolation from every local network intermediary.

In an Extended binary with a valid managed configuration, login, logout, local
credential-list commands, and the Profile/identity mutations listed above are
blocked. Read-only status, check, scopes, doctor, `config show`, `profile list`,
and QR-code commands remain available. Extended `config show` reports the
managed credential source and sanitized runtime mode; it never projects local
secrets or users.

A Standard binary treats the presence of `external-credential.json` only as a
fail-closed edition sentinel; it does not parse or implement this protocol.
Commands that require an effective credential or runtime configuration,
including `config show`, return the typed `failed_precondition` error requiring
the Extended edition. Purely local inspection and workspace-policy surfaces
such as `profile list`, `config risk-control`, `config policy`, and `config
plugins` remain available, while `doctor` reports the edition mismatch as the
active configuration failure.

`platform_proxy` is the only mode that avoids a bearer credential in the
sandbox. `credential_proxy` keeps real Feishu/Lark credentials remote but a
same-permission process may still invoke the external program and reuse its
short-lived bearer. `direct` additionally exposes the returned Feishu/Lark
credential to the CLI process. The external platform remains responsible for
session-to-user binding, authorization, expiry, revocation, rate limits,
audit, file-handle binding, and service reliability. The CLI is responsible
for not leaking credentials, routing proxy-mode requests fail closed, and
launching the configured helper within the isolation described above. It does
not claim to validate the adopter's sandbox boundary or out-of-band
`platform_proxy` client authentication.

## Extended edition

Extended archives are published in the same GitHub Release as Standard
archives:

```text
lark-cli-extended-<version>-<os>-<arch>.tar.gz
lark-cli-extended-<version>-windows-<arch>.zip
```

Both editions install the command name `lark-cli`. `lark-cli version --json`
reports the immutable `edition` and compiled `capabilities`. The Extended
installer and `lark-cli update` download only `lark-cli-extended` assets,
verify `checksums.txt`, verify the candidate edition/version, and never switch
to the npm/npx Standard artifact.
