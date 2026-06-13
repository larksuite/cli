# VC Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).
>
> Catalog & fields live in the CLI: `event list` shows the VC keys (meeting-ended, note-generated, recording started / ended / transcript-generated); `event schema <key>` shows each one's fields. This file only covers what the schema can't: behavior gotchas and recipes. **All VC keys require `--as user`.** Flat output (fields at `.xxx`).

## Behavior gotchas

- **`participant_meeting_ended_v1`**: `start_time` / `end_time` are **not** raw unix seconds — the Process hook converts them to local-timezone RFC3339. If the raw value is empty/non-numeric, the field is left empty. No detail API call; all fields come from the payload.
- **`note.generated_v1`**: fires for meetings *and* recordings/uploads. `note_token` / `verbatim_token` may be empty if detail isn't ready yet. `note_source` (and `note_source.source_entity_id` = meeting ID) is **only present when `source_type == "meeting"`**.
- **`recording.*`**: only fire on Feishu-connected software. `recording_started`/`recording_ended` share `unique_key` (pairs a start with its end). `recording_transcript_generated` carries `transcript_items` as an **array, delivered in batches** — expect multiple events per recording.

## jq recipes

```bash
# meeting ended: topic + end time
lark-cli event consume vc.meeting.participant_meeting_ended_v1 --as user \
  --jq '{meeting: .meeting_id, topic: .topic, ended: .end_time}'

# notes: meeting-sourced only, with enriched tokens
lark-cli event consume vc.note.generated_v1 --as user \
  --jq 'select(.note_source.source_type == "meeting" and .note_token != "") | {note_id, note_token, meeting_id: .note_source.source_entity_id}'

# recording transcript: stream speaker + text per line
lark-cli event consume vc.recording.recording_transcript_generated_v1 --as user \
  --jq '.transcript_items[] | {speaker: .speaker_name, text}'
```
