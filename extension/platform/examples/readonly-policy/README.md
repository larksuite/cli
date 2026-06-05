# Example: read-only policy

A policy plugin that installs a `Rule` allowing only `docs/*` and
`im/*` read commands. Any write command produces a structured
`command_denied` envelope.

## Build & run

```sh
cd extension/platform/examples/readonly-policy
go build -o readonly-cli .

./readonly-cli config policy show
# {
#   "source": "plugin",
#   "source_name": "readonly",
#   "denied_paths": N,
#   "rule": {
#     "name": "agent-readonly",
#     "allow": ["docs/**", "im/**"],
#     "deny": [],
#     "max_risk": "read",
#     "identities": [],
#     "allow_unannotated": false
#   }
# }

./readonly-cli docs +update --doc-token X --content Y
# {"ok":false,"error":{
#   "type":"command_denied",
#   "detail":{
#     "layer":"policy",
#     "policy_source":"plugin:readonly",
#     "rule_name":"agent-readonly",
#     "reason_code":"write_not_allowed"
#   }
# }}

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

## Caveats

- A binary may have **only one** plugin calling `Restrict()`. Two
  policy plugins is a deliberate `plugin_conflict` configuration
  error.
- This Rule shadows any `~/.lark-cli/policy.yml` — plugin Rule
  wins per the resolver precedence.
