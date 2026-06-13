# VC Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).

## Key catalog (4)

| EventKey | Purpose |
|---|---|
| `vc.meeting.participant_meeting_started_v1` | A meeting the current user participates in has started |
| `vc.meeting.participant_meeting_joined_v1` | The current user has joined a meeting |
| `vc.meeting.participant_meeting_ended_v1` | A meeting the current user participates in has ended |
| `vc.note.generated_v1` | A note has been generated (meeting, recording, upload, etc.) |

All four keys use a **Custom schema** (flat output) and carry a **PreConsume hook** that auto-subscribes / unsubscribes via OAPI on first / last consumer. All require `--as user`.

## Scopes & auth

| EventKey | Scope | Auth |
|---|---|---|
| `vc.meeting.participant_meeting_started_v1` | `vc:meeting.meetingevent:read` | user |
| `vc.meeting.participant_meeting_joined_v1` | `vc:meeting.meetingevent:read` | user |
| `vc.meeting.participant_meeting_ended_v1` | `vc:meeting.meetingevent:read` | user |
| `vc.note.generated_v1` | `vc:note:read` | user |

---

## Meeting participant events

Covered keys:

- `vc.meeting.participant_meeting_started_v1`
- `vc.meeting.participant_meeting_joined_v1`
- `vc.meeting.participant_meeting_ended_v1`

### Output fields

| Field | Type | Description |
|---|---|---|
| `type` | string | Event type; one of the covered meeting participant EventKeys |
| `event_id` | string | Globally unique event ID; safe for deduplication |
| `timestamp` | string (timestamp_ms) | Event delivery time (ms timestamp string) |
| `meeting_id` | string | Meeting ID |
| `topic` | string | Meeting topic |
| `meeting_no` | string | Meeting number |
| `start_time` | string | Meeting start time in RFC3339, converted to the local timezone |
| `calendar_event_id` | string | Calendar event ID associated with the meeting |
| `end_time` | string | Meeting end time in RFC3339, converted to the local timezone; only present for `vc.meeting.participant_meeting_ended_v1` |

### Gotchas

- `start_time` / `end_time` are **not** the raw unix-seconds from OAPI — the Process hook converts them to local-timezone RFC3339. If the raw value is empty or non-numeric, the field is left empty. `end_time` is emitted only for `vc.meeting.participant_meeting_ended_v1`.
- No detail API call is made; all fields come from the event payload itself.

### Example

```bash
lark-cli event consume vc.meeting.participant_meeting_started_v1 --as user
lark-cli event consume vc.meeting.participant_meeting_joined_v1 --as user
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
