# lark-cli im +messages-update

Update a sent text or post message.

Use this when an agent sends a placeholder/progress message and later needs to replace it with the final result, without adding another chat message.

## Limits

- Bot identity only.
- Only messages sent by the current app can be edited.
- Only `text` and `post` messages are supported.
- Interactive card messages use a different card update API and are not handled by this shortcut.
- Feishu tenant admin settings and platform limits still apply, including edit windows and edit count limits.

## Examples

```bash
lark-cli im +messages-update \
  --message-id om_xxx \
  --text "Done: the report is ready" \
  --as bot
```

```bash
lark-cli im +messages-update \
  --message-id om_xxx \
  --markdown "**Done**\n\n- item 1\n- item 2" \
  --as bot
```

For raw content JSON:

```bash
lark-cli im +messages-update \
  --message-id om_xxx \
  --msg-type text \
  --content '{"text":"updated text"}' \
  --as bot
```
