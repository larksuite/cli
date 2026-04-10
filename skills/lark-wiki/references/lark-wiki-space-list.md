# lark-wiki +space-list

List all wiki spaces accessible to the caller. Automatically paginates through all pages.

## Usage

```bash
lark-cli wiki +space-list [--as user|bot]
```

## Flags

| Flag | Required | Description |
|------|----------|-------------|
| `--as` | No | Identity: `user` or `bot` (default: `user`) |

## Output

```json
{
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
  "total": 1
}
```

## Notes

- Returns all spaces via automatic pagination; the command may issue multiple API requests under the hood.
- Use `space_id` from the output as `--space-id` for `+node-list` or `+node-copy`.
- `my_library` is the personal document library; its `space_id` is returned as a numeric ID in results.

## Required Scope

`wiki:space:retrieve`
