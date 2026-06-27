# Mail allow/block delete

Use `mail +allow-block-delete` to remove sender addresses or domains from the current user's personal allow or block list.

## Command

```bash
lark-cli mail +allow-block-delete --as user --type allow --address partner@example.com
lark-cli mail +allow-block-delete --as user --type block --address spam.example.com --address bad.example.org
```

## Flags

| Flag | Required | Notes |
|---|---:|---|
| `--mailbox` | no | Mailbox address. Defaults to `me`. With `--as bot`, pass an explicit mailbox address. |
| `--type` | yes | `allow` or `block`. `all` is not supported for writes. |
| `--address` | yes | Repeatable or comma-separated; accepts up to 100 addresses/domains. |

## Output

The result includes `requested`, `success_count`, and the raw API `response`.

## Recovery

- Permission errors: re-authorize with the scope shown in the typed error hint.
- `--as bot --mailbox me`: pass an explicit mailbox address.
