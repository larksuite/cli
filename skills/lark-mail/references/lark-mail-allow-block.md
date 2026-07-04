# User-level allow/block sender lists

Manage the current user's sender allowlist and blocklist. These commands use the user mailbox APIs under:

- `GET /open-apis/mail/v1/user_mailboxes/:user_mailbox_id/allow_senders`
- `GET /open-apis/mail/v1/user_mailboxes/:user_mailbox_id/blocked_senders`
- `POST /open-apis/mail/v1/user_mailboxes/:user_mailbox_id/allow_senders/batch_create`
- `POST /open-apis/mail/v1/user_mailboxes/:user_mailbox_id/blocked_senders/batch_create`
- `DELETE /open-apis/mail/v1/user_mailboxes/:user_mailbox_id/allow_senders/batch_delete`
- `DELETE /open-apis/mail/v1/user_mailboxes/:user_mailbox_id/blocked_senders/batch_delete`

Do not use or invent a `sender_allow_blocks` API.

## List

```bash
lark-cli mail +allow-block-list --as user --mailbox me --type all
lark-cli mail +allow-block-list --as user --type allow --page-size 50
```

`--type` accepts `allow`, `block`, or `all`. `--page-size` must be 1-100.

## Search

```bash
lark-cli mail +allow-block-search --as user --type all --query vendor
lark-cli mail +allow-block-search --as user --type block --query spam@example.com
```

`--query` is required and must be at most 255 bytes. If the backend search cache is still warming, retry later or run `+allow-block-list` first.

## Add Senders

```bash
lark-cli mail +allow-block-set --as user --type allow --address trusted@example.com,partner.com
lark-cli mail +allow-block-set --as user --type block --address blocked@example.com,spam.example
```

`--type` must be `allow` or `block`; `all` is not valid for writes. `--address` supports comma-separated addresses or domains. `--address-file` is a relative file path with one sender per line. The merged sender count must be 1-100.

Adding a sender to one list removes it from the opposite list server-side.

## Delete Senders

```bash
lark-cli mail +allow-block-delete --as user --type block --address MixedCase@Example.COM
```

Delete sends addresses exactly as provided, including casing, so legacy mixed-case records can be removed.

## Safety

Before set/delete, preview the target list, sender count, and sender values, then ask the user to confirm. Do not add the user's own address, aliases, or internal tenant domains. If the API rejects any entry, surface the rejected values, show the adjusted final sender list and count, and ask the user to confirm again before retrying.
