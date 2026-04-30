# IM Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).
>
> **Heads-up for AI agents**: this key's `.content` is **NOT** the raw OAPI payload shape your training data may suggest. `lark-cli` runs a Process hook (`convertlib`) that flattens the V2 envelope and **pre-renders** `.content` to human-readable text for `text` / `post` / `image` / `file` / `audio` / etc. Only `interactive` (cards) keeps the raw JSON string. Don't blindly `fromjson`.

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

**`.content` shape depends on `message_type`** (this key uses a flat Custom schema; see [`events/im/message_receive.go`](../../../events/im/message_receive.go)):

| message_type | `.content` shape | How to read |
|---|---|---|
| `text` / `post` / `image` / `file` / `audio` / `sticker` / `share_chat` / `share_user` / `media` / `system` | Human-readable text (convertlib-processed; `@mentions` resolved to display names) | Use `.content` directly |
| `interactive` (card) | Raw card JSON string (structured actions can't be losslessly flattened) | `.content \| fromjson` to get the card object |

**Do not blindly `fromjson`** — for non-interactive messages it fails with `jq: fromjson cannot be applied to "hello"` because `.content` isn't JSON-encoded.

```bash
# text: .content is plain text — no fromjson needed
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.message_type=="text") | .content'

# interactive: .content is a JSON string — fromjson to parse
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.message_type=="interactive") | .content | fromjson'
```

## On-demand filter recipes

> **Default = no `--jq`.** Run `lark-cli event consume im.message.receive_v1 --as bot` to see every message. The recipes below are only for cases where the user has asked to narrow the stream.

### 1. Filter by chat type (p2p vs group)

`chat_type` is an enum with values `p2p` / `group`.

```bash
# p2p only (direct messages)
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.chat_type=="p2p") | {from: .sender_id, msg: .content}'

# group only
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.chat_type=="group") | {chat: .chat_id, from: .sender_id, msg: .content}'
```

### 2. Filter by message type

```bash
# text only — content is plain human-readable text
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.message_type=="text") | .content'

# interactive (card) only — parse the card body
lark-cli event consume im.message.receive_v1 --as bot \
  --jq 'select(.message_type=="interactive") | .content | fromjson'
```

### 3. Filter by sender (only one user's messages)

```bash
# example: only messages from the given open_id
lark-cli event consume im.message.receive_v1 --as bot\
  --jq 'select(.sender_id=="ou_xxxxxxxxxxxxxxxxxxxxxxxxxx") | {msg_id: .message_id, text: .content}'
```

Get your own open_id via `lark-cli contact +get-user --as user`; other users' via `lark-cli contact +search-user`.