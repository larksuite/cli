# im message read status

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first to understand authentication and global parameters.

Use these commands for message read-status queries:

- `im messages batch_get_read_status` queries whether the current user has read up to 50 messages.
- `im messages read_users` lists users who have read one message and supports both user and bot identities.

This is a raw Meta API command backed by `POST /open-apis/im/v1/messages/batch_query_read_status`.

## Identity and scope

- User identity only: always pass `--as user` or use a profile whose default identity is user.
- Required scope: `im:message.read_status:readonly`.
- Bot or tenant access tokens are not supported.
- The request accepts at most 50 message IDs, and results are limited to messages visible to the current user.

For `read_users`:

- User identity requires `im:message:get_as_user`; the user can query only messages they sent within the last 7 days.
- Bot identity uses an existing bot message scope such as `im:message:readonly`; the bot must be in the chat and can query only messages it sent within the last 7 days.

## Commands

```bash
# Inspect the exact local schema first
lark-cli schema im.messages.batch_get_read_status --format json

# Preview without calling the API
lark-cli im messages batch_get_read_status \
  --data '{"message_ids":["om_xxx","om_yyy"]}' \
  --as user \
  --dry-run \
  --json

# Execute the query
lark-cli im messages batch_get_read_status \
  --data '{"message_ids":["om_xxx","om_yyy"]}' \
  --as user \
  --json
```

For a long list, put the request body in a relative file and pass `--data @request.json`.

To list the users who have read one message as the current user:

```bash
lark-cli im messages read_users \
  --params '{"message_id":"om_xxx","user_id_type":"open_id"}' \
  --as user \
  --json
```

Use `--as bot` for the existing bot flow. The command is paginated with optional `page_size` and `page_token` parameters.

## Request

| Field | Required | Limit | Description |
|------|------|------|------|
| `message_ids` | Yes | 1–50 | OpenAPI message IDs in `om_xxx` format |

## Response

The response contains `read_statuses`:

```json
{
  "read_statuses": [
    {
      "message_id": "om_xxx",
      "read_status": "read"
    },
    {
      "message_id": "om_yyy",
      "read_status": "unexpected",
      "unexpected_reason": "no_permission"
    }
  ]
}
```

| Field | Meaning |
|------|------|
| `message_id` | Input OpenAPI message ID |
| `read_status` | `read`, `unread`, or `unexpected` |
| `unexpected_reason` | Reason when status is `unexpected`, such as `invalid`, `no_permission`, or `not_support` |

## AI usage guidance

1. Never treat `unexpected` as `unread`; it means the service could not determine a read state.
2. Do not retry the same inaccessible message repeatedly when `unexpected_reason` is `no_permission`.
3. Preserve the returned `message_id` when correlating results with the input batch.
4. Use `--dry-run` first when constructing the JSON body programmatically.

## References

- [lark-im](../SKILL.md) — all IM commands
- [lark-shared](../../lark-shared/SKILL.md) — authentication and global parameters
