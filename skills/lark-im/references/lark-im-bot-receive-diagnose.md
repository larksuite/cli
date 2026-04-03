# `lark-cli im +bot-receive-diagnose`

Diagnose why a bot is not receiving IM message events such as `im.message.receive_v1`.

## What it checks

- app configuration can be resolved
- app credentials exist
- bot tenant access token can be acquired
- OpenAPI endpoint is reachable
- event WebSocket startup does not fail immediately
- next-step hints for event subscription, receive scope, and bot availability

## Usage

```bash
lark-cli im +bot-receive-diagnose --as bot
```

## Common flags

```bash
# local-only checks
lark-cli im +bot-receive-diagnose --as bot --offline

# longer timeout for network / websocket startup
lark-cli im +bot-receive-diagnose --as bot --timeout 10

# diagnose a different event type
lark-cli im +bot-receive-diagnose --as bot --event-type "im.message.message_read_v1"
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
- It cannot directly read your developer-console event subscription settings, so subscription and availability checks are reported as actionable hints.
- A passing result means the bot receive chain does not fail immediately from the CLI side; it does not guarantee that a target chat or business handler is already configured correctly.
