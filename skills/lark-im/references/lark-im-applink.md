# im +applink

> **Prerequisite:** Read [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first to understand authentication and identity selection.

Use AppLinks to navigate to a conversation that the person opening the link can already access. These links do not grant membership or replace group invitation/share links from `im chats link`.

## Choose the input

When chat and thread IDs are already known, assemble navigation links without an API request:

```bash
# A joined conversation
lark-cli im +applink --chat-id oc_xxx

# A thread container, even when no message position is available
lark-cli im +applink --chat-id oc_xxx --thread-id omt_xxx
```

`--chat` and `--thread` are aliases for `--chat-id` and `--thread-id`. A thread ID requires the chat ID and must start with `omt_`.

When only a message ID is known, resolve it with one message read request:

```bash
lark-cli im +applink --message-id om_xxx

# Preview the request without reading the message
lark-cli im +applink --message-id om_xxx --dry-run
```

Use this mode for an `om_` root message too. It returns a thread link if the response includes a thread ID and chat ID. Do not combine `--message-id` with the chat/thread flags. No reactions, sender profiles, or thread replies are fetched.

Both modes use the configured profile's Feishu/Lark brand and URL rewrite extensions. Choose `--as user` or `--as bot` according to the intended identity. Message resolution needs the corresponding message-read permission and visibility; local generation does not verify that the chat or thread exists or that the opener can access it.

## Output

The normal JSON success envelope contains `data` with the supplied or resolved IDs and available links:

| Field | Availability |
|-------|--------------|
| `chat_id` | Supplied locally or returned by the message API |
| `thread_id` | Supplied locally or returned by the message API |
| `message_id` | Returned by message resolution |
| `chat_app_link` | A chat ID is available |
| `thread_app_link` | Both chat and thread IDs are available; opens the thread container |
| `message_app_link` | The API supplies a message link, or enough position metadata to assemble one |

Unavailable fields are omitted. A server-provided message link takes precedence. Missing message positions never become a guessed anchor; use `thread_app_link` or `chat_app_link` when only container navigation is available.

```bash
# Extract a locally assembled thread link
lark-cli im +applink --chat-id oc_xxx --thread-id omt_xxx --jq '.data.thread_app_link'
```

The thread route reuses the CLI's existing client navigation protocol. Whether it opens a separate view or a side panel depends on the installed client; this command does not select or guarantee a side-panel presentation.

`+threads-messages-list` also includes a top-level `thread_app_link` in JSON output when the returned messages identify the chat. `+messages-reply` returns the available thread ID and thread/message links from its existing reply response. Neither enhancement adds a message read request.
