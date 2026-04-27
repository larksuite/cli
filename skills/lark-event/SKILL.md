---
name: lark-event
version: 1.0.0
description: "Lark/Feishu real-time event listening / subscribing / consuming: stream events as NDJSON via `lark-cli event consume <EventKey>` (covers IM message receive, reactions, chat member changes, etc.). Use for Lark bots, real-time message processing, long-running subscribers, streaming webhook/push handlers. Supports `--max-events` / `--timeout` bounded runs and a stderr ready-marker contract — designed for AI agents running as subprocesses."
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli event --help"
---

# Lark Events

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) first for authentication, `--as user/bot` switching, `Permission denied` handling, and safety rules.

## Core commands

| Command | Purpose |
|------|------|
| `lark-cli event list [--json]` | List all subscribable EventKeys |
| `lark-cli event schema <EventKey> [--json]` | Show an EventKey's params and output schema |
| `lark-cli event consume <EventKey> [flags]` | Blocking consume; events → stdout NDJSON |
| `lark-cli event status [--json] [--fail-on-orphan]` | Inspect the local bus daemon status |
| `lark-cli event stop [--all] [--force]` | Stop the bus daemon |


## Common flags

| Flag | Description |
|---|---|
| `--param key=value` / `-p` | Business params (repeatable; comma-separated for multi-value). Unknown keys fail with valid names listed inline |
| `--jq <expr>` | jq expression to filter / transform each event; empty output skips the event |
| `--max-events N` | Exit after N events. Default 0 = unlimited |
| `--timeout D` | Exit after duration D (e.g. `30s`, `2m`). Default 0 = no timeout. Whichever of `--max-events` / `--timeout` fires first wins |
| `--output-dir <dir>` | Write each event as a file (relative paths only; prevents traversal) |
| `--quiet` | Suppress stderr diagnostics. **AI should not use this** — it silences the ready marker |
| `--as user\|bot\|auto` | Identity for the session (see lark-shared) |


## Examples

```bash
# Default: stream every event for the key (no filter, no projection)
lark-cli event consume im.message.receive_v1 --as bot

# Grab one sample event to inspect payload shape
lark-cli event consume im.message.receive_v1 --max-events 1 --timeout 30s --as bot

# Run for 10 minutes then auto-exit
lark-cli event consume im.message.receive_v1 --timeout 10m --as bot

# Consume multiple EventKeys concurrently (one shape per process, no dispatcher)
lark-cli event consume im.message.receive_v1          --as bot > receive.ndjson &
lark-cli event consume im.message.reaction.created_v1 --as bot > reaction.ndjson &
wait

```

## Call flow

1. `lark-cli event list --json` → pick a legal key
2. `lark-cli event schema <key> --json` → read `resolved_output_schema` + `jq_root_path` to determine field paths
3. `lark-cli event consume <key> [--jq '<expr>']` → consume

## Subprocess contract

### Ready marker

`event consume`'s stderr emits a fixed line `[event] ready event_key=<key>`. **Parent processes should block on stderr until this line appears, then start reading stdout.** Do not fall back to `sleep`.

### stdin EOF = graceful exit

`event consume` treats stdin close as a shutdown signal (wired for AI subprocess callers). `< /dev/null` / `nohup` / systemd's default `StandardInput=null` will cause an immediate graceful exit (stderr `reason: signal`). To keep running:

- Feed stdin a source that never EOFs: `< <(tail -f /dev/null)`
- Or run bounded: `--max-events N` / `--timeout D`

### Exit codes & reason

On exit, the last stderr line is `[event] exited — received N event(s) in Xs (reason: ...)`.

| exit code | reason | Trigger |
|---|---|---|
| 0 | `reason: limit` | `--max-events` reached |
| 0 | `reason: timeout` | `--timeout` reached |
| 0 | `reason: signal` | Ctrl+C / SIGTERM / stdin EOF |
| non-0 | `Error: ...` (no `exited` line) | Startup / runtime failure (permissions, network, params, config) |

Orchestrators should treat `reason: limit/timeout/signal` (all exit 0) as "business completion" and non-zero as "failure".

### Never `kill -9`

**Avoid `kill -9` on consume processes**: for EventKeys with a **PreConsume hook** (those that register server-side subscriptions via OAPI), `kill -9` skips the OAPI unsubscribe and leaks server-side subscriptions (symptoms: "subscription already exists" on restart, duplicate event delivery). Prefer SIGTERM or closing stdin.

### One consume, one EventKey (multi-key = multi-shell)

The command takes exactly one positional argument; `k1,k2` and wildcards are unsupported. Listening to N keys means N subprocesses — this is **intentional**:

- One shape per process stdout; no dispatcher logic required in the AI
- Fault isolation (one key failing doesn't affect others)
- Independent `--as` / `--jq` / `--max-events` / `--timeout` per key

All N consumers share a single bus daemon (UDS local IPC), so the overhead is small

## Writing jq via schema

`event schema <key> --json` is the source of truth for writing `--jq`. Four things to look at:

**(1) Where fields start** — see `jq_root_path`

- Value `"."` → fields are at the top level, write `.chat_id`
- Value `".event"` → fields are inside a V2 envelope, write `.event.chat_id`

**(2) Field list and types** — see `resolved_output_schema.properties.<name>`

Each field carries `type` / `description`, and some also have `format`. Snippet (from `event schema im.message.receive_v1 --json`):

```json
{
  "chat_id":     {"type":"string", "format":"chat_id",      "description":"Chat ID, prefixed with oc_"},
  "sender_id":   {"type":"string", "format":"open_id",      "description":"Sender open_id, prefixed with ou_"},
  "create_time": {"type":"string", "format":"timestamp_ms", "description":"Send time as ms-epoch string"}
}
```

**(3) Field semantics** — see the `format` tag

Lark-defined semantic tags (**not** JSON Schema's standard `format`). Common values: `open_id` / `chat_id` / `message_id` / `timestamp_ms` / `email`. Purpose: distinguish "same string type, different meanings" fields so you can reverse-lookup via API or convert formats.

**(4) Decoded state** — read the field's `description`

`event consume` runs Process hooks that may pre-decode some payload fields (flattening V2 envelopes, rendering `.content` to plain text, etc.) — behavior differs from raw OAPI. **Always read the field's `description` before writing jq**, especially for generic field names like `content` / `data` / `body` / `payload`.

**Why it matters**: blindly applying `fromjson` to an already-decoded text field makes jq error on every event and silently drop it — the consumer looks alive but emits nothing, with only a single `WARN` line buried on stderr. (This is the general behavior: any jq runtime error skips the event with a one-line WARN; the loop does not abort.)

**Don't shortcut the schema**: when projecting `event schema --json` with jq, do not strip `.description` from `properties` — that's the field that tells you whether a field is already decoded. Dump the full property objects, not just keys.

---

**Aside**: `--param`'s valid parameters also live in the schema — the `params` section lists `name` / `type` / `required` / `enum` / `default` / `description`; **section missing = this key accepts no `--param`**.

## Topic index

| Topic | Reference | Coverage |
|---|---|---|
| IM | [`references/lark-event-im.md`](references/lark-event-im.md) | Catalog of 11 IM EventKeys + shape notes (flat vs V2 envelope) + `im.message.receive_v1` field gotchas (`sender_id` is open_id only; `.content` is plain text except for `interactive` cards) + common jq recipes (filter by chat_type / message_type / sender) |
