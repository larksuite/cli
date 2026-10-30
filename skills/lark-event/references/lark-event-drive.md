# Drive Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).

## Key catalog (1)

| EventKey | Purpose |
|---|---|
| `drive.notice.comment_add_v1` | A cloud document comment or reply produced a notice for the current user or bot (for example an @mention) |

This key uses a **Processed schema** (flat output; root path `.`) and carries a **PreConsume hook** that registers the comment notice subscription before listening.

## Scopes & auth

| EventKey | Scope | Auth |
|---|---|---|
| `drive.notice.comment_add_v1` | `docs:document.comment:read` | user, bot |

The app must also enable `drive.notice.comment_add_v1` under event subscriptions in the developer console (long connection mode) and publish a version; otherwise `event consume` fails up-front and prints a console link in `error.hint`.

- `--as bot`: receive notices addressed to the bot, such as a comment or reply that @mentions the bot, and replies in comment threads the bot takes part in (these arrive with `is_mentioned: false`).
- `--as user`: receive the comment notices the authorized user gets, such as being @mentioned or replies in their comment threads.

`to_open_id` is the notice receiver (the bot or the subscribed user); `from_open_id` is the comment author.

## `drive.notice.comment_add_v1`

### Subscription behavior

On startup, `event consume` calls, with the selected identity:

```text
POST /open-apis/drive/v1/user/subscription
{"event_type": "drive.notice.comment_add_v1"}
```

The server keys this subscription by app, identity, and event type, not by document, so there is no `--param`. It is a durable relation: graceful exit makes no unsubscribe call, and re-running the consumer repeats the idempotent subscribe call. To stop receiving notices entirely, cancel it explicitly with the same `--as` identity the consumer used:

```bash
lark-cli drive user remove_subscription --event-type drive.notice.comment_add_v1 --as bot   # or --as user
```

### Examples

```bash
# Comments or replies that @mention the bot
lark-cli event consume drive.notice.comment_add_v1 --as bot --jq 'select(.is_mentioned)'

# Then read the comment the notice points to
lark-cli drive +batch-query-comments --token <file_token> --type <file_type> --comment-ids <comment_id> --as bot
```

The event carries identifiers only, not the comment text. Read the content with `drive +batch-query-comments` (root comment) or `drive +list-replies --comment-id <comment_id>` (replies), using `file_token` / `file_type` from the event. To answer, use `drive +add-reply --comment-id <comment_id>`; full-document comments reject API replies (error `1069302`), so answer those with a new `drive +add-comment` instead.

### Output fields (flat; root path `.`)

| Field | Type | Description |
|---|---|---|
| `type` | string | Always `drive.notice.comment_add_v1` |
| `event_id` | string | Globally unique event ID; safe for deduplication |
| `timestamp` | string (timestamp_ms) | Event delivery time from `header.create_time` |
| `app_id` / `tenant_key` | string | From the event header |
| `file_token` | string | Token of the commented document |
| `file_type` | string | Document type, for example `docx`, `doc`, `sheet`, `file` |
| `comment_id` | string | Comment ID; for a reply this is the parent comment |
| `reply_id` | string | Reply that triggered the notice; for `add_comment` it is the comment's first reply, which holds the comment body |
| `notice_type` | string enum | `add_comment` or `add_reply` |
| `is_mentioned` | bool | Whether the notice receiver was @mentioned; always present |
| `visibility` | string | Comment visibility reported by the server |
| `from_open_id` / `from_union_id` / `from_user_id` | string | Comment author IDs |
| `from_user_type` | string | Comment author type, for example `user` or `bot` |
| `to_open_id` / `to_union_id` / `to_user_id` | string | Notice receiver IDs |

When consuming as the bot, drop the bot's own comments with `--jq 'select(.from_open_id != "<bot_open_id>")'` to avoid reply loops.
