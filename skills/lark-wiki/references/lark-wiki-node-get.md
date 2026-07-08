# lark-wiki +node-get

Get a wiki node's details by `node_token`, `obj_token`, or a Lark URL. Use this as the "what am I about to touch?" step before `+move` / `+node-copy` / `+node-delete`.

## Usage

```bash
lark-cli wiki +node-get \
  --node-token <node_token | obj_token | Lark URL> \
  [--obj-type <doc|docx|sheet|bitable|mindnote|slides|file>] \
  [--space-id <space_id>] \
  [--format json|pretty|table|csv|ndjson] \
  [--as user|bot]
```

## Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--node-token` | string | **Yes** | — | `node_token`, cloud-doc `obj_token`, or a Lark URL embedding one (e.g. `https://feishu.cn/wiki/<token>` or `https://feishu.cn/docx/<token>`). Matches the `--node-token` naming used by sibling `+node-delete` / `+node-copy` / `+move`. |
| `--token` | string | — (deprecated) | — | Deprecated original name; still accepted for backward compatibility but emits a `Flag --token has been deprecated, use --node-token instead` warning on stderr. New scripts should use `--node-token`. |
| `--obj-type` | enum | No | — | Needed when `--node-token` is a raw `obj_token`; auto-inferred from typed Lark URLs. If omitted for a raw token, the shortcut treats it as a wiki `node_token`. |
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

- The underlying API is `GET /open-apis/wiki/v2/spaces/get_node`. For a `node_token` no `obj_type` is sent; for an `obj_token` the `obj_type` (explicit or URL-inferred) is required.
- `creator` falls back to `creator` when `node_creator` is absent. `updated_at` is `obj_edit_time` formatted as RFC3339.
- No `url` is returned: `get_node` does not provide one and a synthesized `www.feishu.cn/wiki/<node_token>` link is non-canonical/misleading for a read command. Use `node_token` / `obj_token` as the identifiers.

## Required Scope

`wiki:node:retrieve`
