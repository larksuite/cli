# VC Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).

## Key catalog (1)

| EventKey | Purpose |
|---|---|
| `vc.meeting.participant_meeting_ended_v1` | A meeting the current user participates in has ended |

This key uses a **Custom schema** (flat output at `.xxx`) and carries a **PreConsume hook** that auto-subscribes / unsubscribes via OAPI on first / last consumer.

## Scopes & auth

| EventKey | Scope | Auth |
|---|---|---|
| `vc.meeting.participant_meeting_ended_v1` | `vc:meeting.meetingevent:read` | user |

Requires `--as user`.

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
