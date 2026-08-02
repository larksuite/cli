# im +chat-messages-list

> **Prerequisite:** Before executing this command, ensure [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) has been read once in the current task for authentication, global parameters, and safety rules. Do not reread it if already loaded.

**Run `lark-cli im +chat-messages-list --help` for the authoritative flags, defaults, sort/pagination controls, and completeness rule.** This file covers only what `--help` cannot.

## Automatic enrichment

`--help` names the flags that turn enrichment off; it does not say what arrives when they are left on:

- A `reactions` block (counts + details from `im.reactions.batch_query`) on every message that has reactions — so **do not call the reactions API separately** for messages you already pulled here.
- `update_time` on messages that were actually edited (absent otherwise).
- Thread replies expanded via auto-`thread_replies` participate in the same batched enrichment.

See [message enrichment](lark-im-message-enrichment.md) for the full contract.

## Resource Rendering

Message content is rendered into inspectable text, and the markers it uses are not documented anywhere else — an agent parsing `content` needs them:

| Resource | Marker inside `content` | Retrieval |
|---------|-------------|------|
| Image | `![Image](img_xxx)` | `--download-resources`, or `im +messages-resources-download --type image` |
| File | `<file key="file_xxx" .../>` | `--download-resources`, or `im +messages-resources-download --type file` |
| Audio | `<audio key="file_xxx" duration="Xs"/>` | same as File |
| Video | `<video key="file_xxx" .../>` | same as File |
| Sticker | `[Sticker]` | **Not retrievable at all** — Feishu exposes no endpoint for sticker bytes |

With `--download-resources`, each message additionally carries a `resources` block shaped
`{message_id, key, type, local_path, size_bytes}` — see
[message enrichment](lark-im-message-enrichment.md#resource-auto-download---download-resources-opt-in).

## Thread Expansion (`thread_id`)

A message carrying `thread_id` (`omt_xxx`) has replies in a thread; the field is **absent** when it does not. The replies are not in this response — fetch them with [`im +threads-messages-list`](lark-im-threads-messages-list.md).

| Situation | What to do |
|------|------|
| You need surrounding context | Read recent replies for the discovered `thread_id` |
| The user asks for the "full discussion" | Read the thread in chronological order, and require its own completeness signal |
| You only need an overview | Skip thread expansion entirely |

## Gotchas

- **`--user-id` requires user identity.** The p2p-resolution endpoint is user-only, so with bot identity you must resolve the p2p `chat_id` yourself and pass `--chat-id`. `--help` states the flags are mutually exclusive but not this identity constraint.
- **`msg_type` includes `system`** (join/leave/rename events) alongside user content. Summaries that should ignore housekeeping must filter it out — no flag does this.
- **`deleted` is always present**, `updated` and `mentions` only appear when applicable. A `jq` path assuming they exist will break on ordinary messages.
- **Table output truncates `content`.** Use `--format json` whenever the full message body matters.
- **Sender names are already resolved** — no separate contact lookup is needed.

## AI Usage Guidance

**Resolving `chat_id` from a chat name:** use [`+chat-search`](lark-im-chat-search.md) first. **Do not use `im chats search` or `+chat-list`** — always the `+chat-search` shortcut.

**Bot identity with a named group:** when the user says "使用应用身份 / 以 bot 身份" and asks for a named group's history, keep bot identity for both steps — `+chat-search --as bot` then `+chat-messages-list --as bot --chat-id`. Do **not** reach for `im +messages-search --as bot`; that command is user-only.

**Prefer `--chat-id` when you already hold it** — passing `--user-id` costs an extra resolution round-trip.

## Common Errors and Troubleshooting

Only errors whose cause or fix is not evident from `--help`:

| Symptom | Root cause | Solution |
|---------|---------|---------|
| `--user-id requires user identity (--as user); use --chat-id when calling with bot identity` | p2p resolution endpoint is user-only | Pass `--as user`, or look up the p2p `chat_id` separately and pass `--chat-id` |
| `P2P chat not found for this user` | No p2p chat exists between the **current identity** and that user | Confirm the direct-message relationship exists for the identity you are using — it is identity-scoped, not global |
| Permission denied | Read scopes missing | App needs `im:message:readonly` **and** `im:chat:read` |

## References

- [lark-im](../SKILL.md) — all IM commands
