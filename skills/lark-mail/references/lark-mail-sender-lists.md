# Mail Sender Lists

Manage user mailbox trusted and blocked senders with the sender-list shortcuts.

## Shortcuts

```bash
lark-cli mail +sender-list --as user --type allow --mailbox me --page-size 50
lark-cli mail +sender-search --as user --type block --keyword example.com --page-token "<cursor>"
lark-cli mail +sender-set --as user --type allow --sender alice@example.com --sender example.org
lark-cli mail +sender-delete --as user --type block --sender Alice@Example.COM
```

Use `--type allow` for trusted senders and `--type block` for blocked senders. `--mailbox` defaults to `me`; `--user-mailbox-id` is accepted as a hidden compatibility alias.

## Input Rules

- Sender values may be email addresses (`alice@example.com`) or domains (`example.org`).
- Write paths trim inputs, reject whitespace/control/dangerous Unicode, validate shape, and lowercase valid values before adding.
- When at least one valid sender remains, invalid inputs are filtered and returned in `invalid_senders` instead of blocking the whole batch.
- When deleting mixed-case input, the shortcut sends both lowercase and original-case variants so historical case-sensitive entries can be removed.
- A batch can include at most 100 valid entries.

## API Mapping

| Shortcut | API |
|----------|-----|
| `+sender-list --type allow` | `GET /open-apis/mail/v1/user_mailboxes/{mailbox}/allow_senders` |
| `+sender-list --type block` | `GET /open-apis/mail/v1/user_mailboxes/{mailbox}/blocked_senders` |
| `+sender-search` | Same list API with `keyword` query |
| `+sender-set --type allow` | `POST .../allow_senders/batch_create` |
| `+sender-set --type block` | `POST .../blocked_senders/batch_create` |
| `+sender-delete --type allow` | `POST .../allow_senders/batch_remove` |
| `+sender-delete --type block` | `POST .../blocked_senders/batch_remove` |

List and search use `mail:user_mailbox.message:readonly` and can also work when the token has modify scope. Set and delete use `mail:user_mailbox.message:modify`. Backend conflict or permission errors are returned as CLI errors with the operation context attached.
