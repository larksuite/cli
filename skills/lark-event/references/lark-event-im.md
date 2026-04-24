# IM Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).
>
> **Example convention**: every command below includes `--as bot --max-events 1 --timeout 30s` as a runnable bounded skeleton (grabs one event then auto-exits; works in AI / non-TTY environments out of the box). For long-running listeners, drop `--max-events` and either keep stdin alive with `< <(tail -f /dev/null)` or rely on `--timeout`.

## Key catalog (11)

| EventKey | Purpose |
|---|---|
| `im.message.receive_v1` | Receive IM messages |
| `im.message.message_read_v1` | User read a bot's **p2p** message (group messages don't fire this) |
| `im.message.reaction.created_v1` | Reaction added to a message |
| `im.message.reaction.deleted_v1` | Reaction removed from a message |
| `im.chat.updated_v1` | Chat settings changed (owner, avatar, name, permissions, etc.) |
| `im.chat.disbanded_v1` | Chat disbanded |
| `im.chat.member.bot.added_v1` | Bot added to a chat |
| `im.chat.member.bot.deleted_v1` | Bot removed from a chat |
| `im.chat.member.user.added_v1` | User joined a chat (including topic chats) |
| `im.chat.member.user.deleted_v1` | User left voluntarily **or** was removed |
| `im.chat.member.user.withdrawn_v1` | Pending chat invite withdrawn (inviter canceled; user never actually joined) |

> **Shape**: `im.message.receive_v1` is the only flat key (fields at `.xxx`); the other 10 are V2-enveloped (fields at `.event.xxx`).

## Gotchas (`im.message.receive_v1`)

**sender_id is open_id only**: the event payload carries no display name. Call the contact API separately if you need the sender's name.

**`.content` is a raw JSON string for `text` / `post` / `interactive` messages** — you must `fromjson` first:

```bash
# Wrong: .content is a JSON string (not an object); indexing a string with .text makes jq error "Cannot index string with string" and the event is skipped
lark-cli event consume im.message.receive_v1 --as bot --max-events 1 --timeout 30s \
  --jq '.content.text'

# Right: fromjson parses it into an object, then pick fields based on the actual shape
lark-cli event consume im.message.receive_v1 --as bot --max-events 1 --timeout 30s \
  --jq '.content | fromjson'

```

## Common jq recipes

### 1. Filter by chat type (p2p vs group)

`chat_type` is an enum with values `p2p` / `group`.

```bash
# p2p only
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.chat_type=="p2p") | {from: .sender_id, msg: .content}'

# group only
lark-cli event consume im.message.receive_v1 --as bot  \
  --jq 'select(.chat_type=="group") | {chat: .chat_id, from: .sender_id, msg: .content}'
```

### 2. Filter by message type

```bash
# text only (skip cards, images, etc.)
lark-cli event consume im.message.receive_v1 --as bot  \
  --jq 'select(.message_type=="text") | .content'
```
