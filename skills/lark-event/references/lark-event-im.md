# IM Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).
>
> **The catalog lives in the CLI, not here.** `lark-cli event list` lists all 11 IM EventKeys; `lark-cli event schema <key>` gives any key's fields / types / enums. This file only covers what the schema can't: payload-shape gotchas and ready-to-use jq recipes.

## Shape: flat vs enveloped

`im.message.receive_v1` is the only **flat** key (fields at `.xxx`). The other 10 IM keys are **V2-enveloped** — fields live at `.event.xxx` (e.g. `.event.chat_id`). `event schema <key>` confirms it (its Output Schema nests everything under `event`).

## `.content` is pre-rendered — do NOT blindly `fromjson` (`im.message.receive_v1`)

`lark-cli` runs a Process hook that **pre-renders `.content` to human-readable text** for every `message_type` except `interactive` (`@mentions` resolved to display names). Only `interactive` (cards) keeps the raw JSON string.

| message_type | `.content` | How to read |
|---|---|---|
| everything except `interactive` | plain text | use `.content` directly |
| `interactive` (card) | raw card JSON string | `.content \| fromjson` |

Applying `fromjson` to a non-interactive message errors per event (`jq: fromjson cannot be applied to "hello"`) and the consumer **silently drops** it — looks alive, emits nothing.

**`sender_id` is `open_id` only** — the payload carries no display name; resolve via the contact API if you need one.

## jq recipes (`im.message.receive_v1`)

> Default = no `--jq` (stream every message). Use these only when asked to narrow the stream.

```bash
# group chats only (chat_type enum: p2p | group)
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.chat_type=="group") | {chat: .chat_id, from: .sender_id, msg: .content}'

# text messages only — .content is plain text
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.message_type=="text") | .content'

# interactive cards only — parse the card body
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.message_type=="interactive") | .content | fromjson'

# one sender's messages only
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.sender_id=="ou_xxxx") | {msg_id: .message_id, text: .content}'
```

Get your own open_id via `lark-cli contact +get-user --as user`; others' via `lark-cli contact +search-user`.
