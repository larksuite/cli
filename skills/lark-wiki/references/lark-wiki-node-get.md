# lark-wiki +node-get

Get a wiki node's details by `node_token`, `obj_token`, or a Lark URL. Use this as the "what am I about to touch?" step before `+move` / `+node-copy` / `+node-delete`.

## Usage

```bash
lark-cli wiki +node-get \
  --node-token <node_token | obj_token | Lark URL> \
  [--space-id <space_id>] \
  [--format json|pretty|table|csv|ndjson] \
  [--as user|bot]
```

## Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--node-token` | string | **Yes** | — | `node_token`, cloud-doc `obj_token`, or a Lark URL embedding one (e.g. `https://feishu.cn/wiki/<token>` or `https://feishu.cn/docx/<token>`). Matches the `--node-token` naming used by sibling `+node-delete` / `+node-copy` / `+move`. |
| `--token` | string | — (deprecated) | — | Deprecated original name; still accepted for backward compatibility but emits a `Flag --token has been deprecated, use --node-token instead` warning on stderr. New scripts should use `--node-token`. |
| `--space-id` | string | No | — | Optional cross-check: fail if the resolved node does not live in this space |
| `--format` | enum | No | `json` | `json` / `pretty` / `table` / `csv` / `ndjson` |
| `--as` | enum | No | `auto` | Identity `user`/`bot`; wiki is user-centric → pass `--as user` |

## Output

```json
{
  "space_id": "7160145948494381236",
  "node_token": "wikcnEXAMPLE",
  "obj_token": "docxEXAMPLE",
  "obj_type": "docx",
  "node_type": "origin",
  "parent_node_token": "wikcnPARENT",
  "origin_node_token": "",
  "title": "Design Spec",
  "has_child": true,
  "creator": "ou_xxx",
  "owner": "ou_yyy",
  "obj_edit_time": "1700000000",
  "obj_create_time": "1690000000",
  "node_create_time": "1690000001",
  "updated_at": "2023-11-14T22:13:20Z"
}
```

## Notes

- The underlying API is `GET /open-apis/wiki/v2/spaces/node_by_token`. Only `token` is sent; the server detects whether it is a Wiki or document token and validates its length. The CLI still requires a nonempty token and validates URL syntax.
- `--obj-type` is deprecated and hidden. Legacy scripts may still pass it; its value is ignored without a warning or any extra stdout/stderr output. URL paths are used only to extract tokens, not to assert the returned object type. `--space-id` remains a response cross-check.
- `creator` falls back to `creator` when `node_creator` is absent. `updated_at` is `obj_edit_time` formatted as RFC3339.
- The shortcut preserves its existing output fields and does not emit or synthesize a `url`. Use `node_token` / `obj_token` as the identifiers.

## Terminal business errors

These HTTP 200 responses carry a non-zero business code and are not retryable with the same input:

| Code | Meaning | Required action |
|------|---------|-----------------|
| `131005` | The Wiki node does not exist | Check the token or obtain a current Wiki link |
| `131006` | The current user or app/bot identity lacks access to the Wiki node or space | This is resource access, not app scope authorization. Do not retry the same request, reauthorize, or switch identity as trial and error; ask the node owner or wiki administrator to grant read access, or use an accessible resource |
| `131012` | The Wiki node has been deleted | Do not retry the same node token; rediscover the node or ask for a current Wiki link |
| `131013` | The resource token is invalid | Do not switch identity or reauthorize; correct the URL/token |
| `131014` | The document is not mounted in Wiki | Stop Wiki resolution; use the corresponding docs/sheets/base/drive command, or provide a Wiki URL/node_token |
| `131016` | The token is too short | Provide the complete token or document URL; do not retry the same input |

HTTP 200 alone does not mean success: non-zero business codes still produce a CLI failure. `131001` (invalid request) and gateway errors may still return HTTP 4xx/5xx.

## Rate limiting

For `99991400` / `rate_limit`: Do not retry immediately. Wait `retry_after_seconds`, or use exponential backoff with jitter. Stop after 3 total attempts (1 initial + 2 retries).

## Required Scope

`wiki:node:retrieve`
