# Mail allow/block set

Use `mail +allow-block-set` to add sender addresses or domains to the current user's personal allow or block list.

## Command

```bash
lark-cli mail +allow-block-set --as user --type allow --address partner@example.com
lark-cli mail +allow-block-set --as user --type block --address spam.example.com --address bad.example.org
```

## Flags

| Flag | Required | Notes |
|---|---:|---|
| `--mailbox` | no | Mailbox address. Defaults to `me`. With `--as bot`, pass an explicit mailbox address. |
| `--type` | yes | `allow` or `block`. `all` is not supported for writes. |
| `--address` | yes | Repeatable or comma-separated; accepts up to 100 addresses/domains. |
| `--scene` | no | `sender` or `web_image`; defaults to `sender`. |

## Output

The result includes `requested`, `success_count`, `failed_items`, and the raw API `response`. If `failed_items` is non-empty, the command still succeeds and prints a warning to stderr so callers can inspect the partial server-side filtering.

## Recovery

- Self address/domain errors: do not add your own mailbox address or internal tenant domain.
- Permission errors: re-authorize with the scope shown in the typed error hint.
- `--as bot --mailbox me`: pass an explicit mailbox address.
