# lark-wiki +space-list

List wiki spaces accessible to the caller. **Default fetches a single page** (matches the rest of the CLI's list shortcuts); pass `--page-all` to walk every page.

## Usage

```bash
# Default: single page (first up to --page-size items)
lark-cli wiki +space-list

# Walk every page (capped by --page-limit, default 10)
lark-cli wiki +space-list --page-all

# Walk every page, no cap (use with care if you have many spaces)
lark-cli wiki +space-list --page-all --page-limit 0

# Resume from a specific cursor (single-page fetch regardless of --page-all)
lark-cli wiki +space-list --page-token <TOKEN>

# Pretty / table / csv / ndjson output
lark-cli wiki +space-list --format pretty
lark-cli wiki +space-list --format table
```

## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--page-size` | int | 50 | Page size, 1-50 |
| `--page-token` | string | — | Page cursor; implies single-page fetch (no auto-pagination) |
| `--page-all` | bool | `false` | Automatically paginate through all pages (capped by `--page-limit`) |
| `--page-limit` | int | 10 | Max pages with `--page-all` (0 = unlimited) |
| `--format` | enum | `json` | `json` / `pretty` / `table` / `csv` / `ndjson` |
| `--as` | enum | `user` | Identity: `user` or `bot` |

## Output

```json
{
  "ok": true,
  "data": {
    "spaces": [
      {
        "space_id": "6946843325487912356",
        "name": "Engineering Wiki",
        "description": "...",
        "space_type": "team",
        "visibility": "private",
        "open_sharing": "closed"
      }
    ],
    "has_more": false,
    "page_token": ""
  },
  "meta": { "count": 1 }
}
```

When the default single-page fetch (or `--page-all` capped by `--page-limit`) does not exhaust the upstream cursor, `has_more=true` and `page_token=<cursor>` so the caller can resume via `--page-token` or by increasing `--page-limit`.

## Notes

- **The underlying API never returns the my_library personal library**; resolve it via `lark-cli wiki spaces get --params '{"space_id":"my_library"}'`.
- Use `space_id` from the output as `--space-id` for `+node-list` or `+node-copy`.

## Required Scope

`wiki:space:retrieve`
