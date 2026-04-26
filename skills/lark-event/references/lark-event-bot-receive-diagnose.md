# `lark-cli event +bot-receive-diagnose`

Diagnose why a bot is not receiving events such as `im.message.receive_v1`.

Part of the `event` module because the diagnose reuses the same
event-dispatcher / WebSocket stack as `event +subscribe`.

## What it checks

- app configuration can be resolved
- app credentials exist
- bot tenant access token can be acquired
- OpenAPI endpoint is reachable (inferred from token acquisition)
- event WebSocket startup does not return a startup error within `--timeout`
- advisory hints for event subscription, required scope (derived from `--event-type`), and bot availability

## Usage

```bash
lark-cli event +bot-receive-diagnose --as bot
```

## Common flags

```bash
# local-only checks
lark-cli event +bot-receive-diagnose --as bot --offline

# longer timeout for network / websocket startup
lark-cli event +bot-receive-diagnose --as bot --timeout 10

# diagnose a different event type
lark-cli event +bot-receive-diagnose --as bot --event-type "im.message.message_read_v1"
```

## Output

The command returns structured data with:

- `event_type`
- `summary`
- `checks`
- `next_steps`

Each check includes:

- `name`
- `status` (`pass` / `warn` / `fail` / `skip`)
- `message`
- `hint`

## Notes

- This command is read-only and does not send probe messages.
- It cannot directly read your developer-console event subscription settings or a bot's granted scopes, so subscription, scope, and availability checks are reported as actionable advisories.
- A `summary.ok` of `true` means no check failed outright — it does **not** guarantee that events are actually flowing. In particular, the WebSocket check reports `warn` (rather than `pass`) when it observes no startup error but cannot confirm readiness, because the underlying SDK may be retrying transient connection failures internally.
