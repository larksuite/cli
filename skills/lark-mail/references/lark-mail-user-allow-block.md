# User allow and block senders

Use `user_mailbox.allow_sender` and `user_mailbox.blocked_sender` to manage the current user's trusted and blocked sender lists. These are native Meta API commands, not mail shortcuts.

## Commands

Always inspect the schema before calling an API:

```bash
lark-cli schema mail.user_mailbox.allow_sender.list
lark-cli schema mail.user_mailbox.allow_sender.batch_create
lark-cli schema mail.user_mailbox.allow_sender.batch_remove
lark-cli schema mail.user_mailbox.blocked_sender.list
lark-cli schema mail.user_mailbox.blocked_sender.batch_create
lark-cli schema mail.user_mailbox.blocked_sender.batch_remove
```

List and search use the same `list` method. Omit `keyword` to list records; pass `keyword` to search by sender address or domain prefix.

```bash
lark-cli mail user_mailbox.allow_sender list --as user \
  --params '{"user_mailbox_id":"me","page_size":20}'

lark-cli mail user_mailbox.blocked_sender list --as user \
  --params '{"user_mailbox_id":"me","keyword":"example.com","page_size":20}'
```

Batch set semantics are implemented by `batch_create`: it incrementally adds entries and does not replace the whole list.

```bash
lark-cli mail user_mailbox.blocked_sender batch_create --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"items":[{"sender":"spam@example.com","sender_type":1},{"sender":"bad.example.com","sender_type":2}]}'
```

Batch delete semantics are implemented by `batch_remove`:

```bash
lark-cli mail user_mailbox.allow_sender batch_remove --as user \
  --params '{"user_mailbox_id":"me"}' \
  --data '{"senders":["trusted@example.com","trusted.example.com"]}'
```

## Fields

- `user_mailbox_id`: use `"me"` for the current user mailbox, or pass a full mailbox address / `open_id`.
- `items[].sender`: email address or domain.
- `items[].sender_type`: `1` for email address, `2` for domain.
- `senders[]`: email addresses or domains to remove.
- `keyword`: optional list/search prefix filter.
- `page_size`: 1-100, default 20.
- `page_token`: pass the previous response token when continuing pagination.

## Scopes

- `list`: `mail:user_mailbox:readonly`
- `batch_create` / `batch_remove`: `mail:user_mailbox`

Server-side rules enforce allow/block mutual exclusion, a maximum of 100 items per batch, a combined 2000-item limit per user, and self-address / tenant-domain protection.
