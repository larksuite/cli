# Minutes Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).

## Key catalog (1)

| EventKey | Purpose |
|---|---|
| `minutes.minute.generated_v1` | A minute (妙记) has been generated |

This key uses a **Custom schema** (flat output at `.xxx`) and carries a **PreConsume hook** that auto-subscribes / unsubscribes via OAPI on first / last consumer.

## Scopes & auth

| EventKey | Scope | Auth |
|---|---|---|
| `minutes.minute.generated_v1` | `minutes:minutes.basic:read` | user |

Requires `--as user`.

## `minutes.minute.generated_v1`

### Output fields

| Field | Type | Description |
|---|---|---|
| `type` | string | Event type; always `minutes.minute.generated_v1` |
| `event_id` | string | Globally unique event ID; safe for deduplication |
| `timestamp` | string (timestamp_ms) | Event delivery time (ms timestamp string) |
| `minute_token` | string | Minute token |
| `title` | string | Minute title (enriched via detail API) |
| `minute_source` | object | Minute source metadata; only present when the source is a meeting |
| `minute_source.source_type` | string | Source type; only present when the source is a meeting (value: `meeting`) |
| `minute_source.source_entity_id` | string | Source entity ID (meeting ID); only present when the source is a meeting |

### Enrichment & degradation

The Process hook calls `GET /open-apis/minutes/v1/minutes/{minute_token}` to enrich `title`. If the detail API fails, this field is left empty — the base fields (`type`, `event_id`, `timestamp`, `minute_token`, `minute_source`) are always present.

`minute_source` is populated from the event payload directly (not the detail API), so it survives enrichment failures. Note: `minute_source` is only present when the minute originates from a meeting; for other sources (e.g. recording, local upload) this field is absent.

### Example

```bash
lark-cli event consume minutes.minute.generated_v1 --as user

# Project title and token only (skip events where enrichment failed)
lark-cli event consume minutes.minute.generated_v1 --as user \
  --jq 'select(.title != "") | {minute_token, title}'

# Filter by source type
lark-cli event consume minutes.minute.generated_v1 --as user \
  --jq 'select(.minute_source.source_type == "meeting") | {minute_token, title}'
```
