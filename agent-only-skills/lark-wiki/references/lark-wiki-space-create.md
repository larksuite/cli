# lark-wiki +space-create

Create a wiki space. OpenAPI: `POST /open-apis/wiki/v2/spaces`. This is the project-initialization entry point — the alternative is hand-writing `wiki spaces create --params '{...}'`.

> The underlying `spaces.create` API is flagged `danger: true` in the schema browser, but it is **not** confirmation-gated (no `--yes`). A space created by mistake is recoverable via `wiki +delete-space`.

## Usage

```bash
lark-cli wiki +space-create \
  --name <space_name> \
  [--description <text>] \
  [--as user]
```

## Flags

| Flag | Type | Required | Default | Description |
|------|------|----------|---------|-------------|
| `--name` | string | **Yes** | — | Wiki space name. Blank/whitespace is rejected (an unnamed space is almost always an accident) |
| `--description` | string | No | — | Wiki space description |
| `--as` | enum | No | `user` | Use the AAT user lane |

## Output

```json
{
  "space_id": "7160145948494381236",
  "name": "Engineering Wiki",
  "description": "team docs",
  "space_type": "team",
  "visibility": "private",
  "open_sharing": "closed"
}
```

There is no `url` field — the create API does not return one.

## Notes

- Use the AAT `--as user` lane consistently.
- `--dry-run` previews the `POST /open-apis/wiki/v2/spaces` request (and surfaces the blank-name validation error early).

## Required Scope

`wiki:space:write_only`
