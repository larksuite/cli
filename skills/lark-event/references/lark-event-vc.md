# VC Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).

## Key catalog

| EventKey | Purpose |
|---|---|
| `vc.meeting.participant_meeting_ended_v1` | A meeting the current user participates in has ended |
| `vc.note.generated_v1` | A note has been generated (meeting, recording, upload, etc.) |
| `vc.bot.meeting_invited_v1` | The bot is invited to a meeting |
| `vc.bot.meeting_event_v1` | The bot observes meeting activity |
| `vc.bot.meeting_ended_v1` | A meeting observed by the bot has ended |

The user VC keys use a **Custom schema** (flat output) and carry a **PreConsume hook** that auto-subscribes / unsubscribes via OAPI on first / last consumer. They require `--as user`.

The `vc.bot.*` keys are bot-observed events. They require `--as bot`, keep the original payload in `raw_event`, and do not call the user-side VC meeting subscription / unsubscription APIs.

## Scopes & auth

| EventKey | Scope | Auth |
|---|---|---|
| `vc.meeting.participant_meeting_ended_v1` | `vc:meeting.meetingevent:read` | user |
| `vc.note.generated_v1` | `vc:note:read` | user |
| `vc.bot.meeting_invited_v1` | App event subscription in the Developer Console | bot |
| `vc.bot.meeting_event_v1` | App event subscription in the Developer Console | bot |
| `vc.bot.meeting_ended_v1` | App event subscription in the Developer Console | bot |

---

## `vc.meeting.participant_meeting_ended_v1`

### Output fields

| Field | Type | Description |
|---|---|---|
| `type` | string | Event type; always `vc.meeting.participant_meeting_ended_v1` |
| `event_id` | string | Globally unique event ID; safe for deduplication |
| `timestamp` | string (timestamp_ms) | Event delivery time (ms timestamp string) |
| `meeting_id` | string | Meeting ID |
| `topic` | string | Meeting topic |
| `meeting_no` | string | Meeting number |
| `start_time` | string | Meeting start time in RFC3339, converted to the local timezone |
| `end_time` | string | Meeting end time in RFC3339, converted to the local timezone |
| `calendar_event_id` | string | Calendar event ID associated with the meeting |

### Gotchas

- `start_time` / `end_time` are **not** the raw unix-seconds from OAPI — the Process hook converts them to local-timezone RFC3339. If the raw value is empty or non-numeric, the field is left empty.
- No detail API call is made; all fields come from the event payload itself.

### Example

```bash
lark-cli event consume vc.meeting.participant_meeting_ended_v1 --as user

# Project meeting topic and end time only
lark-cli event consume vc.meeting.participant_meeting_ended_v1 --as user \
  --jq '{meeting: .meeting_id, topic: .topic, ended: .end_time}'
```

---

## `vc.note.generated_v1`

Fires when a note is generated — not just from meetings, but also from realtime recordings and local file uploads.

### Output fields

| Field | Type | Description |
|---|---|---|
| `type` | string | Event type; always `vc.note.generated_v1` |
| `event_id` | string | Globally unique event ID; safe for deduplication |
| `timestamp` | string (timestamp_ms) | Event delivery time (ms timestamp string) |
| `note_id` | string | Note ID |
| `note_token` | string | Note document token; may be empty if detail is not yet available |
| `verbatim_token` | string | Verbatim document token; may be empty if detail is not yet available |
| `note_source` | object | Source metadata; only present when source is a meeting |
| `note_source.source_type` | string | Source type; only present when source is a meeting (value: `meeting`) |
| `note_source.source_entity_id` | string | Source entity ID (meeting ID); only present when source is a meeting |

### Source type semantics

| `source_type` | Trigger |
|---|---|
| `meeting` | Note generated from a meeting |

`note_source` (and its sub-fields) are only populated when `source_type` is `meeting`. For other sources the field is absent.

### Example

```bash
lark-cli event consume vc.note.generated_v1 --as user

# Only notes with enriched tokens, skip incomplete ones
lark-cli event consume vc.note.generated_v1 --as user \
  --jq 'select(.note_token != "") | {note_id, note_token, verbatim_token}'

# Filter to meeting-sourced notes only
lark-cli event consume vc.note.generated_v1 --as user \
  --jq 'select(.note_source.source_type == "meeting") | {note_id, meeting_id: .note_source.source_entity_id}'
```

---

## Bot-observed VC events

Use bot identity for all `vc.bot.*` keys:

```bash
lark-cli event consume vc.bot.meeting_invited_v1 --as bot
lark-cli event consume vc.bot.meeting_event_v1 --as bot
lark-cli event consume vc.bot.meeting_ended_v1 --as bot
```

These keys model what the bot observes. Do not treat them as aliases for:

| Bot event | Not the same as |
|---|---|
| `vc.bot.meeting_invited_v1` | Meeting start events, participant join events, or IM meeting cards |
| `vc.bot.meeting_event_v1` | User-side `vc +meeting-events` open meeting activity queries |
| `vc.bot.meeting_ended_v1` | `vc.meeting.participant_meeting_ended_v1` or open meeting resource ended events |

### Output fields

| Field | Type | Description |
|---|---|---|
| `type` | string | Event type; one of the supported `vc.bot.*` keys |
| `event_id` | string | Globally unique event ID; safe for deduplication |
| `timestamp` | string (timestamp_ms) | Event delivery time from `header.create_time` when present |
| `call_id` | string | Invitation call ID; pass through to VC agent join when present |
| `meeting_no` | string | Meeting number when present in the payload |
| `activity_event_type` | string | Meeting activity subtype when present |
| `chat_emoji_types` | string[] | Feishu post emotion `emoji_type` values extracted from `vc.bot.meeting_event_v1` payloads |
| `raw_event` | object | Original bot event payload; authoritative for fields not normalized by `lark-cli` |

Malformed or evolving payloads are not over-normalized. If a payload cannot be parsed, `event consume` passes the raw payload through; if a field is not recognized, read `raw_event`.

### Post emotion forwarding

`lark-cli event consume` does not send IM messages automatically. When `vc.bot.meeting_event_v1` exposes `chat_emoji_types`, an agent can explicitly forward the emotion by sending a Feishu `post` message whose content uses `tag: "emotion"` and `emoji_type` from `chat_emoji_types`.

```bash
lark-cli im +messages-send \
  --chat-id <chat_id> \
  --msg-type post \
  --content '{
    "zh_cn": {
      "title": "Meeting reaction",
      "content": [[
        {"tag": "text", "text": "Reaction: "},
        {"tag": "emotion", "emoji_type": "JIAYI"}
      ]]
    }
  }' \
  --as bot
```

Use the exact key from `.chat_emoji_types[]` as `emoji_type`; do not convert it to a system emoji or synthesize another value.
