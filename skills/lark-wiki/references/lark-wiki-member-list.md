# lark-wiki +member-list

List the members of a wiki space. OpenAPI: `GET /open-apis/wiki/v2/spaces/:space_id/members`. **Default fetches a single page** (matches `+space-list` / `+node-list`); pass `--page-all` to walk every page.

## Usage

```bash
# Default: single page
lark-cli wiki +member-list --space-id <space_id>

# Walk every page (capped by --page-limit, default 10)
lark-cli wiki +member-list --space-id <space_id> --page-all

# Walk every page, no cap
lark-cli wiki +member-list --space-id <space_id> --page-all --page-limit 0

# Resume from a specific cursor (single-page fetch regardless of --page-all)
lark-cli wiki +member-list --space-id <space_id> --page-token <TOKEN>

# Personal library
lark-cli wiki +member-list --space-id my_library --as user

# Pretty / table / csv / ndjson output
lark-cli wiki +member-list --space-id <space_id> --format pretty
lark-cli wiki +member-list --space-id <space_id> --format table
```

## Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--space-id` | string | **Yes** | — | Wiki space ID; use `my_library` for the personal document library (user only) |
| `--page-size` | int | No | 50 | Page size, 1-50 |
| `--page-token` | string | No | — | Page cursor; implies single-page fetch (no auto-pagination) |
| `--page-all` | bool | No | `false` | Automatically paginate through all pages (capped by `--page-limit`) |
| `--page-limit` | int | No | 10 | Max pages with `--page-all` (0 = unlimited) |
| `--format` | enum | No | `json` | `json` / `pretty` / `table` / `csv` / `ndjson` |
| `--as` | enum | No | `auto` | Identity `user`/`bot`; wiki is user-centric → pass `--as user` |

## Output

```json
{
  "ok": true,
  "data": {
    "space_id": "7160145948494381236",
    "members": [
      {
        "member_id": "ou_449b53ad6aee526f7ed311b216aabcef",
        "member_type": "openid",
        "member_role": "admin"
      },
      {
        "member_id": "ou_67e5ecb64ce1c0bd94612c17999db411",
        "member_type": "openid",
        "member_role": "member"
      }
    ],
    "has_more": false,
    "page_token": ""
  },
  "meta": { "count": 2 }
}
```

`type` (`user` / `chat` / `department`) is included when the server returns it. When the default single-page fetch (or `--page-all` capped by `--page-limit`) does not exhaust the upstream cursor, `has_more=true` and `page_token=<cursor>` so the caller can resume.

## Notes

- **Bot + `my_library` is rejected upfront** — pass an explicit `--space-id` when `--as bot`.
- Use `member_id` from the output as `--member-id` for [`+member-remove`](lark-wiki-member-remove.md); `member_type` and `member_role` must be passed exactly as listed to remove a grant.
- `--dry-run` previews 2 steps when `--space-id my_library` (resolve → list), 1 step otherwise.

## Required Scope

`wiki:member:retrieve`
