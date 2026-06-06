# im +chat-messages-list

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first to understand authentication, global parameters, and safety rules.

Fetch the message list for a conversation. Supports both group chats and direct messages.

By default the response carries a `reactions` block (counts + details from `im.reactions.batch_query`) on every message that has reactions, and `update_time` on messages that were actually edited. Thread replies expanded via auto-`thread_replies` participate in the same batched enrichment. Pass `--no-reactions` to skip the extra round-trip. Pass `--download-resources` to additionally download message resources (image/file/audio/video/media + post-embedded, excluding stickers) into `./lark-im-resources/` and attach a `resources` block — off by default. See [message enrichment](lark-im-message-enrichment.md) for the full contract.

This skill maps to the shortcut: `lark-cli im +chat-messages-list` (internally calls `GET /open-apis/im/v1/messages`, and automatically resolves the p2p chat_id when needed).

## Commands

```bash
# Get group chat messages (json output by default)
lark-cli im +chat-messages-list --chat-id oc_xxx

# Get direct messages with a user (pass open_id and resolve p2p chat_id automatically)
lark-cli im +chat-messages-list --user-id ou_xxx

# Specify a time range (ISO 8601)
lark-cli im +chat-messages-list --chat-id oc_xxx --start "2026-03-10T00:00:00+08:00" --end "2026-03-11T00:00:00+08:00"

# Specify a time range (date only)
lark-cli im +chat-messages-list --chat-id oc_xxx --start 2026-03-10 --end 2026-03-11

# Control sort order and page size (max 50)
lark-cli im +chat-messages-list --chat-id oc_xxx --order asc --page-size 20

# Pagination
lark-cli im +chat-messages-list --chat-id oc_xxx --page-token "xxx"

# JSON output
lark-cli im +chat-messages-list --chat-id oc_xxx --format json
```

## Parameters

| Parameter | Required | Description |
|------|------|------|
| `--chat-id <id>` | One of two | Specify the conversation by its chat_id directly (e.g., group chat `oc_xxx`) |
| `--user-id <id>` | One of two | Specify a DM conversation by the other user's open_id (`ou_xxx`); p2p chat_id is resolved automatically. Requires user identity (`--as user`); not supported with bot identity |
| `--start <time>` | No | Start time (ISO 8601 or date only) |
| `--end <time>` | No | End time (ISO 8601 or date only) |
| `--order <order>` | No | Sort order: `asc` / `desc` (default `desc`) |
| `--page-size <n>` | No | Page size (default 50, max 50) |
| `--page-token <token>` | No | Pagination token |
| `--no-reactions` | No | Skip auto-fetching the `reactions` block |
| `--download-resources` | No | Download message resources (image/file/audio/video/media + post-embedded, excluding stickers) into `./lark-im-resources/` and attach a `resources` block. Off by default; no extra requests when omitted |

> Rule: `--chat-id` and `--user-id` are mutually exclusive. You must provide exactly one of them.

> **CAUTION:** `--order` is the only sort axis — messages are always ordered by creation time, `asc` or `desc`. There is no field axis: the command cannot sort by sender or any other field, so do **not** attempt `--sort sender` or similar (it is rejected). If the user asks to group or sort by sender, fetch with `--order` and aggregate client-side, and tell them this is local post-processing, not a CLI/API sort capability.

## Resource Rendering

Messages are rendered into human-readable text for inspection. Image messages are shown as placeholders such as `[Image: img_xxx]`; files, audio, and videos are rendered with resource keys in the content (e.g. `<audio key="file_xxx" duration="Xs"/>`). By default resource binaries are **not** downloaded.

Two ways to get the binaries:
- **In one pass:** add `--download-resources` to this command — every eligible resource (image/file/audio/video/media + post-embedded, excluding stickers) is downloaded into `./lark-im-resources/` and a `resources` block (`{message_id, key, type, local_path, size_bytes}`) is attached to each message. See [message enrichment](lark-im-message-enrichment.md#resource-auto-download---download-resources-opt-in).
- **One at a time:** use [lark-im-messages-resources-download](lark-im-messages-resources-download.md).

| Resource Type | Marker in Content | Behavior |
|---------|-------------|------|
| Image | `[Image: img_xxx]` | `--download-resources`, or manually `im +messages-resources-download --type image` |
| File | `<file key="file_xxx" .../>` | `--download-resources`, or manually `im +messages-resources-download --type file` |
| Audio | `<audio key="file_xxx" duration="Xs"/>` | `--download-resources`, or manually `im +messages-resources-download --type file` |
| Video | `<video key="file_xxx" .../>` | `--download-resources`, or manually `im +messages-resources-download --type file` |
| Sticker | `[Sticker]` | Not downloadable (Feishu does not support fetching sticker resources) |

## Thread Expansion (`thread_id`)

In JSON output, a message may contain a `thread_id` (`omt_xxx`) field, which means the message has replies in a thread. Use [`im +threads-messages-list`](lark-im-threads-messages-list.md) to inspect replies in that thread:

```bash
lark-cli im +threads-messages-list --thread omt_xxx
```

| Scenario | Recommendation |
|------|------|
| You need context | Call `im +threads-messages-list --order desc --page-size 10` for the discovered thread_id to inspect recent replies |
| The user asks for the "full discussion" | Use `im +threads-messages-list --order asc --page-size 50`, then paginate if needed |
| You only need an overview | Skip thread expansion |

## Output Fields

| Field | Description |
|------|------|
| `messages` | Message array |
| `total` | Number of messages in the current page |
| `has_more` | Whether additional pages are available |
| `page_token` | Pagination token for the next page |

Each message contains:

| Field | Description |
|------|------|
| `message_id` | Message ID |
| `msg_type` | Message type: `text`, `image`, `file`, `interactive`, `post`, `audio`, `video`, `system`, etc. |
| `create_time` | Creation time |
| `sender` | Sender information (includes `name` for user senders) |
| `content` | Message content |
| `deleted` | Whether the message has been recalled (always present, `true` = recalled) |
| `updated` | Whether the message has been edited after sending |
| `mentions` | Array of @mentions in the message; each item contains `{id, key, name}`. Present only when the message contains @mentions |
| `thread_id` | Thread ID (`omt_xxx`) if the message has replies in a thread. Present only when replies exist |

## Pagination (`has_more` / `page_token`)

`im +chat-messages-list` returns `has_more` and `page_token` when more data is available. Use `--page-token` to continue:

```bash
lark-cli im +chat-messages-list --chat-id oc_xxx --page-token <PAGE_TOKEN>
```

You can also fall back to the generic API:

```bash
lark-cli api GET /open-apis/im/v1/messages \
  --params 'container_id_type=chat&container_id=oc_xxx&page_size=50&page_token=<PAGE_TOKEN>'
```

## Common Errors and Troubleshooting

| Symptom | Root Cause | Solution |
|---------|---------|---------|
| `specify --chat-id <chat_id> or --user-id <open_id>` | Neither `--chat-id` nor `--user-id` was provided | You must provide exactly one |
| `--chat-id and --user-id cannot be specified together` | Both parameters were provided | Use only one |
| `--user-id requires user identity (--as user); use --chat-id when calling with bot identity` | `--user-id` was used with bot identity | The p2p resolution endpoint requires user identity. Either pass `--as user` or look up the p2p `chat_id` separately and pass it via `--chat-id` |
| `P2P chat not found for this user` | `--user-id` was used but no p2p chat exists for the current identity and that user | Confirm the target direct-message relationship exists for the current identity |
| `--start: invalid time format` | Invalid time format | Use ISO 8601 or date-only format such as `2026-03-10` |
| Permission denied | Message read permissions are missing | Ensure the app has `im:message:readonly` and `im:chat:read` enabled |

## AI Usage Guidance

1. **Resolving chat_id from a chat name:** When the user refers to a chat by name and you don't have the `chat_id`, use [`+chat-search`](lark-im-chat-search.md) first:
   ```bash
   # Find chat_id by name, then list messages
   lark-cli im +chat-search --query "<chat name keyword>" --format json
   lark-cli im +chat-messages-list --chat-id <chat_id>
   ```
   **Do not use `im chats search` or `+chat-list` — always use the `+chat-search` shortcut.**
2. **Prefer `--chat-id` when available:** if the chat_id is already known, use it directly to avoid extra API calls.
3. **For direct messages:** use `--user-id` to resolve the p2p chat automatically instead of looking it up manually. This requires user identity (`--as user`); with bot identity, resolve the p2p `chat_id` yourself and pass it via `--chat-id`.
4. **For time ranges:** both ISO 8601 and date-only inputs are supported. Date-only is usually simpler.
5. **For full content:** table output truncates content. Use `--format json` when you need the complete message body.
6. **For sender info:** the command already resolves sender names, so you do not need a separate lookup.
7. **Application/bot identity + named group history:** If the user says "使用应用身份/以 bot 身份" and asks to list or read historical messages for a named group, use bot identity for both steps:
   ```bash
   lark-cli im +chat-search --as bot --query "<chat name keyword>" --format json
   lark-cli im +chat-messages-list --as bot --chat-id <chat_id> --page-size 50 --format json
   ```
   Do not use `im +messages-search --as bot`; `+messages-search` is user-only. Continue with `--page-token` if `has_more=true`.

## References

- [lark-im](../SKILL.md) - all IM commands
- [lark-shared](../../lark-shared/SKILL.md) - authentication and global parameters
