# lark-cli im +messages-card-update

Update a sent interactive card message.

Use this when an agent sent a shared interactive card and needs to replace card content, such as fixing a button URL or replacing a progress card with a final card.

This is different from [`+messages-update`](lark-im-messages-update.md), which edits `text` and `post` messages.

## Limits

- Only interactive card messages are supported.
- The original card and replacement card must be shared cards with `config.update_multi=true`.
- The message must not be recalled.
- Feishu platform limits still apply, including the supported update window and exclusions for batch-sent cards.
- The caller identity must match an identity allowed to update the card. Use `--as bot` for app-sent cards and `--as user` for user-sent cards when permitted.

## Examples

```bash
lark-cli im +messages-card-update \
  --message-id om_xxx \
  --content '{"config":{"update_multi":true},"elements":[{"tag":"div","text":{"tag":"plain_text","content":"Done"}}]}' \
  --as bot
```

To update a button link, send the complete replacement card JSON, not a partial patch:

```bash
lark-cli im +messages-card-update \
  --message-id om_xxx \
  --content '{"config":{"update_multi":true},"elements":[{"tag":"action","actions":[{"tag":"button","text":{"tag":"plain_text","content":"Open"},"url":"https://example.com/new"}]}]}' \
  --as bot
```

Preview the request without executing it:

```bash
lark-cli im +messages-card-update \
  --message-id om_xxx \
  --content '{"config":{"update_multi":true},"elements":[{"tag":"div","text":{"tag":"plain_text","content":"Preview"}}]}' \
  --as bot \
  --dry-run
```
