# IM sync-to-chat relation

When replying inside a thread, a sender can also send that reply to the main
chat. The server then stores two messages with the same content but different
message IDs: the original thread reply and a chat-level copy. Message-reading
shortcuts may include `sync_to_chat_info` to connect those two messages.

The field is absent when the message has no usable synchronization relation;
absence does not mean the message request failed.

| Field | Description |
|------|------|
| `type` | Role of the current message. `1` (`SYNC_TO_CHAT_TARGET_MESSAGE`) means this message is the chat-level copy; `2` (`SYNC_TO_CHAT_SOURCE_MESSAGE`) means this message is the original thread reply. |
| `related_message_id` | Message ID on the other side: the original thread reply for type `1`, or the chat-level copy for type `2`. |
| `thread_id` | Root thread ID of the original reply. Normally present on the type `1` chat-level copy. |

When the current message is the synchronized chat message (`type: 1`):

```json
{
  "sync_to_chat_info": {
    "type": 1,
    "thread_id": "omt_root",
    "related_message_id": "om_source_reply"
  }
}
```

When the current message is the source thread reply (`type: 2`):

```json
{
  "sync_to_chat_info": {
    "type": 2,
    "related_message_id": "om_synchronized_message"
  }
}
```

## Agent rules

- **Deduplicate summaries and counts.** The chat-level copy and original reply
  carry the same content and may both appear in one result, including when the
  original is nested under `thread_replies`. Treat a type `1` message as a copy
  of its `related_message_id` unless the separate chat position or reactions
  matter to the task.
- **Choose where to reply.** Reply to the type `2` original with
  `+messages-reply --reply-in-thread` to stay in the thread. Reply to the type
  `1` copy without `--reply-in-thread` to answer in the main chat.
- **Do not assume missing means failure.** Ordinary messages omit
  `sync_to_chat_info`.

The CLI exposes only these documented fields. Unknown upstream relation fields
are ignored until they receive an explicit public contract.
