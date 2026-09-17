# Example: Agent Employee credential provider

A credential provider whose Agent Employee credential comes from an external
system rather than the local keychain. It implements the long-standing
`Provider` interface plus the optional `AccessProvider`, which is what lets a
credential arrive together with the subject it belongs to.

## Why AccessProvider

The legacy `ResolveToken` returns a token string. That is enough for a user or
tenant token, whose subject is already established elsewhere — by an interactive
login, or by the tenant the app is installed in. An Agent Employee credential
carries its own user principal, and the CLI refuses to send one it cannot match
against the principal the account declared. A string alone leaves nothing to
match, so an account declaring `CredentialAAT` is only served through
`ResolveAccess`; a provider that declares one without implementing the interface
is rejected rather than quietly falling back to some other credential.

## What the CLI checks

The provider performs its own verification. The CLI does not repeat it — it
checks consistency:

| Declared in `ResolveAccount` | Returned by `ResolveAccess` | Result on mismatch |
|-|-|-|
| `CredentialKind: CredentialAAT` | `Kind: CredentialAAT` | request fails |
| `CredentialKind: CredentialAAT` | — | `--as bot` fails before any request: an Agent Employee has no tenant lane |
| `OpenID: ou_xxx` | `OpenID: ou_xxx` | request fails |
| `OpenID: ou_xxx` | `ExpiresAt` set and in the future | request fails |

The last two rows are what declaring `OpenID` buys: it asks the CLI to hold the
credential to that principal and to a stated expiry. A provider that declares no
`OpenID` owns the credential's lifecycle itself — the CLI has nothing to compare
against, so it uses what `ResolveAccess` returns and checks neither field. This
example declares one, and is checked on both.

Nothing this provider returns is written to the local keychain, config, logs or
dry-run output.

## Build & run

```sh
cd extension/credential/examples/agent-employee-provider
go build -o agent-cli .

export EXAMPLE_AGENT_APP_ID=cli_xxx
export EXAMPLE_AGENT_OPEN_ID=ou_xxx
export EXAMPLE_AGENT_TOKEN=<token minted by your system>

./agent-cli whoami
# {"identity":"user","credential":"aat","principal":"agent_employee",...}

./agent-cli api GET /open-apis/im/v1/chats
# the Agent Employee credential travels the user lane

./agent-cli api GET /open-apis/im/v1/chats --as bot
# refused before any request: an Agent Employee has no tenant lane

./agent-cli auth status --verify
# verificationSource=provider_managed — the provider vouched for the subject,
# which is a weaker statement than this CLI having verified it directly
```

Unset `EXAMPLE_AGENT_APP_ID` or `EXAMPLE_AGENT_OPEN_ID` and the provider steps
aside, letting the built-in keychain path answer as usual.
