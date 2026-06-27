# Mail allow/block list

Use `mail +allow-block-list` to list or search the current user's personal sender allow/block lists.

## Command

```bash
lark-cli mail +allow-block-list --as user --type all
lark-cli mail +allow-block-list --as user --type block --query spam.example.com
```

## Flags

| Flag | Required | Notes |
|---|---:|---|
| `--mailbox` | no | Mailbox address. Defaults to `me`. With `--as bot`, pass an explicit mailbox address. |
| `--type` | no | `allow`, `block`, or `all`. Defaults to `all`; `all` calls both resources and merges the result. |
| `--query` | no | Optional address/domain keyword. Omit for list mode. |
| `--page-size` | no | 1-100, default 50. |
| `--page-token` | no | Cursor from a previous response. |

## Output

The result contains `items[]`, each tagged with `type` (`allow` or `block`), plus pagination fields. When `--type all` is used, `allow` and `block` pagination metadata are returned separately.

## Recovery

- `456` / cache building: retry later, or remove `--query` and page through the list.
- Permission errors: re-authorize with the scope shown in the typed error hint.
- `--as bot --mailbox me`: pass an explicit mailbox address.
