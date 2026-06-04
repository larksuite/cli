# Whiteboard Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).

## Key catalog (1)

| EventKey | Purpose |
|---|---|
| `board.whiteboard.updated_v1` | A whiteboard has been edited |

This key uses a **Native schema** (V2 envelope; output rooted at `.event`) and carries a **PreConsume hook** that auto-subscribes / unsubscribes via OAPI on first / last consumer.

## Scopes & auth

| EventKey | Scope | Auth |
|---|---|---|
| `board.whiteboard.updated_v1` | `board:whiteboard:node:read` | user, bot |

Supports `--as user` or `--as bot`. The caller must have **manage** access to the target whiteboard, otherwise the subscribe OAPI returns 403 and `event consume` exits with an auth error before listening.

## `board.whiteboard.updated_v1`

### Per-whiteboard subscription

Unlike global event keys (e.g. minutes / im), this key subscribes **per whiteboard**: `event consume` calls `POST /open-apis/board/v1/whiteboards/{whiteboard_id}/subscribe` on startup with the `whiteboard_id` you pass via `-p`. **Required parameter**: `-p whiteboard_id=<whiteboard_token>`. Missing this param fails param validation up-front with `required param "whiteboard_id" missing for EventKey board.whiteboard.updated_v1` before any subscription happens.

Whiteboard token can be obtained via the docs OAPI [list document blocks](https://open.feishu.cn/document/ukTMukTMukTM/uUDN04SN0QjL1QDN/document-docx/docx-v1/document-block/list): the block whose `block_type=43` is a whiteboard, and `block.token` is the whiteboard token.

### Output fields (V2 envelope; root path `.event`)

| Field | Type | Description |
|---|---|---|
| `.event.whiteboard_id` | string (kind=whiteboard_id) | Whiteboard token |
| `.event.operator_ids[].open_id` | string (kind=open_id) | Editor's open_id (`ou_` prefix) |
| `.event.operator_ids[].union_id` | string (kind=union_id) | Editor's union_id |
| `.event.operator_ids[].user_id` | string (kind=user_id) | Editor's user_id (only present when the caller's app has the user_id-related contact scope granted by the OAPI side) |

`operator_ids` is an array — multi-user collaborative editing within one tick collapses into a single event with multiple entries.

### Subscription lifecycle

| Phase | Behavior |
|---|---|
| Startup | `event consume` calls `subscribe` OAPI; on success stderr emits `[event] consuming as ...`, `[event] running pre-consume setup...`, `[event] listening for events (key=board.whiteboard.updated_v1)...`, then the AI-facing ready marker `[event] ready event_key=board.whiteboard.updated_v1` |
| Running | Edits to the whiteboard stream as NDJSON to stdout |
| Graceful exit (Ctrl+C / SIGTERM / `--max-events` / `--timeout` / stdin EOF) | `event consume` calls `unsubscribe` OAPI |
| `kill -9` | **Skips unsubscribe → server-side subscription leaks**, may cause `subscription already exists` or duplicate delivery on next consume. See SKILL.md "Never `kill -9`". |

### Example

```bash
# Stream every edit on whiteboard <token> until Ctrl+C
lark-cli event consume board.whiteboard.updated_v1 \
    -p whiteboard_id=<whiteboard_token> \
    --as user

# Sample one event for payload inspection
lark-cli event consume board.whiteboard.updated_v1 \
    -p whiteboard_id=<whiteboard_token> \
    --as user --max-events 1 --timeout 2m

# Project to "edit summary": who edited which whiteboard
lark-cli event consume board.whiteboard.updated_v1 \
    -p whiteboard_id=<whiteboard_token> \
    --as user \
    --jq '{whiteboard: .event.whiteboard_id, editors: (.event.operator_ids | map(.open_id))}'
```
