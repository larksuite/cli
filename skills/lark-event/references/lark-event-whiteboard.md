# Whiteboard Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).
>
> One key: `board.whiteboard.updated_v1` (run `event schema` for fields). Supports `--as user` **or** `--as bot`. Output is V2-enveloped — fields at `.event.xxx`.

## Per-whiteboard subscription (the gotcha)

Unlike global keys, this one subscribes **per whiteboard**. **Required param: `-p whiteboard_id=<whiteboard_token>`** — omitting it fails param validation up-front (`required param "whiteboard_id" missing ...`) before any subscription.

- Get the token via the docs OAPI [list document blocks](https://open.feishu.cn/document/ukTMukTMukTM/uUDN04SN0QjL1QDN/document-docx/docx-v1/document-block/list): the block with `block_type=43` is a whiteboard; its `block.token` is the whiteboard token.
- The caller must have **manage** access to that whiteboard, otherwise the subscribe OAPI returns 403 and `event consume` exits with an auth error **before** listening.
- `.event.operator_ids` is an **array** — multiple collaborators editing in one tick collapse into a single event with multiple entries.
- `.event.operator_ids[].user_id` is present **only when the app has been granted the user_id-related contact scope**; otherwise only `open_id` / `union_id` appear.

## jq recipes

```bash
# stream every edit on the whiteboard until Ctrl+C
lark-cli event consume board.whiteboard.updated_v1 -p whiteboard_id=<whiteboard_token> --as user

# sample one event to inspect the payload
lark-cli event consume board.whiteboard.updated_v1 -p whiteboard_id=<whiteboard_token> --as user --max-events 1 --timeout 2m

# edit summary: who edited which whiteboard
lark-cli event consume board.whiteboard.updated_v1 -p whiteboard_id=<whiteboard_token> --as user \
  --jq '{whiteboard: .event.whiteboard_id, editors: (.event.operator_ids | map(.open_id))}'
```
