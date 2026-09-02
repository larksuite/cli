# Example: read-only policy

A policy plugin that installs a `Rule` allowing only `docs/*` and
`im/*` read commands. Any denied command produces a structured
`failed_precondition` envelope with policy diagnostics.

## Build & run

```sh
cd extension/platform/examples/readonly-policy
go build -o readonly-cli .

./readonly-cli config policy show
# {
#   "source": "plugin",
#   "source_name": "readonly",
#   "denied_paths": N,
#   "rules": [{
#     "name": "agent-readonly",
#     "allow": ["docs/**", "im/**"],
#     "deny": [],
#     "max_risk": "read",
#     "identities": [],
#     "allow_unannotated": false
#   }]
# }

./readonly-cli docs +update --doc X --content Y
# {"ok":false,"error":{"type":"validation","subtype":"failed_precondition",
#   "hint":"denied by policy policy (source plugin:readonly, ... reason_code write_not_allowed); ..."}}

./readonly-cli docs +fetch --doc-token X
# Normal read response (assuming credentials)
```

## Key points

- `Restrict(&Rule{...})` is the only call needed — the Builder
  flips Capabilities to `Restricts=true, FailurePolicy=FailClosed`
  automatically. A policy plugin that silently fails to install
  would erase the security boundary, so FailClosed is enforced.
- `MaxRisk: platform.RiskRead` rejects any command annotated
  write / high-risk-write.
- `AllowUnannotated` is left default (false): unannotated commands
  are denied with `risk_not_annotated`. Set it to true if you need
  a gradual-adoption window for the lark-cli main tree.
- A fork that wants denied commands to present as absent can opt in from
  `main` with `cmd.ExecuteWithOptions(cmd.ConcealRestrictedCommands(...))`.

## Caveats

- A binary may have **only one** plugin calling `Restrict()`. Two
  policy plugins is a deliberate `plugin_conflict` configuration
  error.
- This Rule shadows any `~/.lark-cli/policy.yml` — plugin Rule
  wins per the resolver precedence.
