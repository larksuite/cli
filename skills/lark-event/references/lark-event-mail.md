# Mail Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).

## Key catalog (1)

| EventKey | Purpose |
|---|---|
| `mail.user_mailbox.event.message_received_v1` | New email arrived in a user mailbox |

**Identity:** `--as user` only. Bot identity is rejected at param-validation time. The current user must have console event `mail.user_mailbox.event.message_received_v1` subscribed in the developer console (and the app must be published) before `event consume` will run.

## Params

| Name | Role | Default | Description |
|---|---|---|---|
| `mailbox` | subscription identity | `me` | Email address (or `me` for the current user). Different mailboxes get **independent** server-side subscriptions; running two `event consume` processes with different `-p mailbox=...` is supported and each gets its own subscribe/cleanup lifecycle. |
| `folders` | filter | — | Comma-separated folder IDs. Events whose mail is not in any of these folders are dropped (triggers one metadata fetch per event). |
| `labels` | filter | — | Comma-separated label IDs. Events whose mail does **not carry all** of these labels are dropped (AND semantics). |
| `msg-format` | output enrichment | `event` | Controls which fields are populated. See output schema below. |

`event` schema shows `SUB-KEY=yes` on `mailbox` (the only subscription identity param). The other three are filter / process params; changing their values does **not** open a new subscription.

## Output schema (union, by `msg-format`)

The output struct is a union — fields populate progressively as `msg-format` increases. Fields absent at a lower level are omitted from JSON (omitempty).

| Field | event | metadata | plain_text_full | full |
|---|:-:|:-:|:-:|:-:|
| `message_id` | ✅ | ✅ | ✅ | ✅ |
| `mail_address` | ✅ | ✅ | ✅ | ✅ |
| `mailbox_type` | ✅ | ✅ | ✅ | ✅ |
| `subscriber.user_ids[].{user_id,open_id,union_id}` | ✅ | ✅ | ✅ | ✅ |
| `from` | — | ✅ | ✅ | ✅ |
| `subject` | — | ✅ | ✅ | ✅ |
| `snippet` | — | ✅ | ✅ | ✅ |
| `folder_id` | — | ✅ | ✅ | ✅ |
| `label_ids` | — | ✅ | ✅ | ✅ |
| `body_text` | — | — | ✅ | ✅ |
| `body_html` | — | — | — | ✅ |
| `attachments` | — | — | — | ✅ |

**Picking a level**:
- `event` (default): no per-event API call. Use when you only need `message_id` to fetch on demand via `mail +message`.
- `metadata`: 1 GET per event. Use for triage / notification — gives subject + from + snippet.
- `plain_text_full`: 1 GET per event with `format=plain_text`. Use when you need the body for content analysis but don't care about HTML/attachments.
- `full`: 1 GET per event with `format=full`. Use when you need attachments metadata or the HTML body.

Run `lark-cli event schema mail.user_mailbox.event.message_received_v1` for the live field reference (descriptions per field include the conditional, e.g. `"Sender email address (msg-format >= metadata)"`).

## Pipeline: receive then fetch on demand

For long-running agents, default to `msg-format=event` and fetch only when needed. Cheaper, lower latency, and the message API gives you everything `msg-format=full` would.

```bash
lark-cli event consume mail.user_mailbox.event.message_received_v1 --as user \
  --jq '{id: .message_id, addr: .mail_address}' \
| while IFS= read -r evt; do
    msg_id=$(echo "$evt" | jq -r '.id')
    lark-cli mail +message --message-id "$msg_id" --as user
  done
```

## Multi-mailbox

Each `-p mailbox=...` value (after normalize) yields a distinct `SubscriptionID`. The bus daemon dedups PreConsume/cleanup per `SubscriptionID`, so the following two processes run **independent** subscriptions and event streams against the same Feishu app:

```bash
# Terminal A
lark-cli event consume mail.user_mailbox.event.message_received_v1 \
  -p mailbox=alice@x.com --as user

# Terminal B
lark-cli event consume mail.user_mailbox.event.message_received_v1 \
  -p mailbox=bob@x.com --as user
```

`event status` shows both as separate rows under a single EventKey, distinguished by the `SUB` column (the fingerprint suffix).

## `me` alias resolution

`-p mailbox=me` (the default) is resolved to the current user's real primary email at startup via `GET /open-apis/mail/v1/user_mailboxes/me/profile`. After resolution, the real email is what flows through fingerprint / PreConsume / Match / Process. So `me` and the explicit email of the same user produce the same `SubscriptionID` (no accidental duplicate subscriptions).

If your user has no mailbox provisioned (e.g. enterprise email not yet enabled), this call returns an error and `event consume` exits with a `normalize params for ...: resolve mailbox 'me': ...` message.

## Filter recipes

### 1. New mail to a specific folder only

```bash
lark-cli event consume mail.user_mailbox.event.message_received_v1 \
  -p mailbox=me -p folders=INBOX -p msg-format=metadata --as user \
  --jq '{from, subject, snippet}'
```

### 2. Flagged (starred) mail across all folders

```bash
lark-cli event consume mail.user_mailbox.event.message_received_v1 \
  -p mailbox=me -p labels=FLAGGED -p msg-format=metadata --as user
```

Use `lark-cli mail labels list` to discover label IDs.

### 3. Drop mail from a specific sender

`folders` / `labels` are positive filters; for sender exclusion use `--jq`:

```bash
lark-cli event consume mail.user_mailbox.event.message_received_v1 \
  -p mailbox=me -p msg-format=metadata --as user \
  --jq 'select(.from != "noreply@example.com")'
```

## Cleanup

On graceful exit (`--max-events`/`--timeout` reached, or SIGTERM/stdin EOF), the last consumer for a `SubscriptionID` runs cleanup: a POST to `unsubscribe`. Two stderr outcomes:

- Success: `[event] cleanup done.`
- Failure: `WARN: cleanup failed: <reason> (server-side subscribe is idempotent — residual record will be overwritten on next subscribe)`

The Feishu server-side subscribe record is `(app, user, event_type)`-keyed and idempotent, so a failed unsubscribe leaks at most one stale record per `(app, user)` until the next `event consume` for that key — which silently overwrites it. Agents **MUST NOT** manually call the unsubscribe API as a recovery action: the server has no reference counting, so a stray unsubscribe will silently kill another co-living consumer's stream.
