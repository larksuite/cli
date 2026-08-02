# Mail User Allow/Block

Manage the current user's trusted and blocked mail senders.

## Commands

```bash
lark-cli mail user-allow-block list --type allow|block|all [--page-size 20] [--cursor <page_token>]
lark-cli mail user-allow-block search <keyword> --type allow|block|all [--page-size 20] [--cursor <page_token>]
lark-cli mail user-allow-block get <address-or-domain> [--allow|--block]
lark-cli mail user-allow-block add --allow|--block <address-or-domain>...
lark-cli mail user-allow-block delete --allow|--block <address-or-domain>...
```

Use `--mailbox me` by default. Records can be full addresses such as `alice@example.com` or domains such as `example.com`.

## Examples

```bash
# List both lists.
lark-cli mail user-allow-block list --type all --page-size 50

# Search both lists.
lark-cli mail user-allow-block search suspicious.example --type all

# Add trusted senders.
lark-cli mail user-allow-block add --allow alice@example.com example.com

# Block senders.
lark-cli mail user-allow-block add --block spammer@example.com bad.example

# Delete from the block list.
lark-cli mail user-allow-block delete --block bad.example

# Check one record.
lark-cli mail user-allow-block get alice@example.com
```

## Notes

- `add` and `delete` require exactly one of `--allow` or `--block`.
- A single add/delete request accepts at most 100 records.
- Search depends on the server-side mail allow/block cache. If the API reports that the cache is not ready, retry later or run `list` without a keyword.
- The server rejects attempts to add your own address, an alias, or an internal tenant domain. Remove that record and retry.
- Rate limit errors usually recover by retrying later with a smaller `--page-size` or fewer records.
