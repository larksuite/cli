# Minutes Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).
>
> Catalog & fields live in the CLI: `event list` (the one key `minutes.minute.generated_v1`) and `event schema minutes.minute.generated_v1`. This file only covers what the schema can't: enrichment behavior and recipes. **Requires `--as user`.** Flat output (fields at `.xxx`).

## Enrichment & degradation (the gotcha)

The Process hook calls the minutes detail API to enrich `title`. **If that call fails, `title` is left empty** — the base fields (`type`, `event_id`, `timestamp`, `minute_token`, `minute_source`) are always present. So filter on `.title != ""` if you only want enriched events.

`minute_source` comes from the payload directly (survives enrichment failure) and is **only present when the minute originates from a meeting** (`source_type == "meeting"`); for recording / local-upload sources it is absent.

## jq recipes

```bash
lark-cli event consume minutes.minute.generated_v1 --as user

# title + token, skipping events where enrichment failed
lark-cli event consume minutes.minute.generated_v1 --as user \
  --jq 'select(.title != "") | {minute_token, title}'

# meeting-sourced minutes only
lark-cli event consume minutes.minute.generated_v1 --as user \
  --jq 'select(.minute_source.source_type == "meeting") | {minute_token, title}'
```
