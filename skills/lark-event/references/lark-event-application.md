# Lark Application Events

This page covers Application-domain EventKeys supported by `lark-cli event`.

## `application.bot.menu_v6`

Triggered when a user clicks a custom bot menu item whose response action is configured as a push event.

Listen as the bot identity:

```bash
lark-cli event consume application.bot.menu_v6 --as bot
```

Filter a specific menu event key:

```bash
lark-cli event consume application.bot.menu_v6 --as bot --jq 'select(.event_key == "start_eval")'
```

Output is flattened at the top level:

| Field | Meaning |
|---|---|
| `type` | Event type, always `application.bot.menu_v6` |
| `event_id` | Globally unique event ID from the event header |
| `timestamp` | Event delivery time, preferring `header.create_time` |
| `app_id` | App ID from the event header |
| `tenant_key` | Tenant key from the event header |
| `event_key` | Developer-defined menu event key, for example `start_eval` |
| `menu_timestamp` | Menu click timestamp from the event body |
| `operator_id` | Operator open_id alias |
| `operator_open_id` | Operator open_id |
| `operator_union_id` | Operator union_id |
| `operator_user_id` | Operator user_id |
| `operator_name` | Operator display name |

This EventKey has no `--param`; use `--jq` to filter by `event_key` or operator fields.
