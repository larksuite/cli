# lark-wiki +node-list

List wiki nodes in a space or under a specific parent node. **Default fetches a single page** (large knowledge bases can have thousands of nodes — opt into `--page-all` explicitly with an eye on `--page-limit`).

## Usage

```bash
# Default: single page of root nodes
lark-cli wiki +node-list --space-id <SPACE_ID>

# Drill into a sub-directory (still single page by default)
lark-cli wiki +node-list --space-id <SPACE_ID> --parent-node-token <NODE_TOKEN>

# Drill with a wiki URL (CLI normalizes /wiki/<token> to node_token)
lark-cli wiki +node-list --space-id <SPACE_ID> --parent-node-token "https://feishu.cn/wiki/wikcn_xxx"

# Personal document library (user identity only)
lark-cli wiki +node-list --space-id my_library --as user

# Walk every page (capped by --page-limit, default 10)
lark-cli wiki +node-list --space-id <SPACE_ID> --page-all

# Walk every page with a higher cap
lark-cli wiki +node-list --space-id <SPACE_ID> --page-all --page-limit 30

# Resume from a cursor
lark-cli wiki +node-list --space-id <SPACE_ID> --page-token <TOKEN>

# Pretty / table output
lark-cli wiki +node-list --space-id <SPACE_ID> --format pretty
```

## Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--space-id` | string | **Yes** | — | Numeric wiki space ID. Use `my_library` for personal document library (user only) |
| `--parent-node-token` | string | No | — | Parent wiki node token, or a `/wiki/<token>` URL; omit to list the space root |
| `--page-size` | int | No | 50 | Page size, 1-50 |
| `--page-token` | string | No | — | Page cursor; implies single-page fetch (no auto-pagination) |
| `--page-all` | bool | No | `false` | Automatically paginate through all pages (capped by `--page-limit`) |
| `--page-limit` | int | No | 10 | Max pages with `--page-all` (0 = unlimited) |
| `--format` | enum | No | `json` | `json` / `pretty` / `table` / `csv` / `ndjson` |
| `--as` | enum | No | `auto` | Identity `user`/`bot`; wiki is user-centric → pass `--as user` (`my_library` requires `--as user`) |

## Output

```json
{
  "ok": true,
  "data": {
    "nodes": [
      {
        "space_id": "6946843325487912356",
        "node_token": "wikcn_EXAMPLE_TOKEN",
        "obj_token": "doccn_EXAMPLE_TOKEN",
        "obj_type": "docx",
        "parent_node_token": "",
        "node_type": "origin",
        "title": "Getting Started",
        "has_child": true
      }
    ],
    "has_more": false,
    "page_token": ""
  },
  "meta": { "count": 1 }
}
```

When the default single-page fetch (or `--page-all` capped by `--page-limit`) does not exhaust the upstream cursor, `has_more=true` and `page_token=<cursor>` so the caller can resume via `--page-token` or by increasing `--page-limit`.

## Traverse the wiki tree

To list all content recursively, call `+node-list` again with each node's `node_token` as `--parent-node-token` when `has_child` is `true`.

```bash
# Step 1: list root nodes
lark-cli wiki +node-list --space-id 6946843325487912356

# Step 2: drill into a node that has children
lark-cli wiki +node-list --space-id 6946843325487912356 --parent-node-token wikcn_EXAMPLE_TOKEN
```

## Notes

- `--space-id my_library` is a per-user alias and only valid with `--as user`. The shortcut will refuse `--as bot` with `my_library` upfront.
- `--space-id` is a numeric wiki `space_id`. Do not pass a wiki URL, wiki node token, document token, or title. Use `lark-cli wiki +space-list --as user` to discover it.
- `--parent-node-token` must resolve to a wiki node token. If you have a docx/sheet/base/file URL, first run `lark-cli wiki +node-get --node-token <url>` and use the returned `node_token`.
- Treat `invalid_parameters` (`space_id is not int`, `invalid page_token`), `not_found` (`node not found by parent node token`), and `permission_denied` as terminal for the current arguments. Fix the argument or permission before retrying.
- For `rate_limit`, stop immediate retries and retry later with exponential backoff or a smaller `--page-limit`.

## Required Scope

`wiki:node:retrieve`
