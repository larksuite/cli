# Drive Events

> **Prerequisite:** Read [`../SKILL.md`](../SKILL.md) first for the `event consume` essentials (commands, subprocess contract, jq usage).

## Key catalog (3)

| EventKey | Purpose |
|---|---|
| `drive.notice.comment_add_v1` | A cloud document comment or reply produced a notice for the current user or bot (for example an @mention) |
| `drive.file.bitable_field_changed_v1` | A field of a subscribed Base (bitable) was added, deleted or edited |
| `drive.file.bitable_record_changed_v1` | A record of a subscribed Base (bitable) was added, deleted or edited |

`drive.notice.comment_add_v1` uses a **Processed schema** (flat output; root path `.`); the two Base keys use a **Native schema** (V2 envelope; output rooted at `.event`). All three carry a **PreConsume hook** that registers the subscription before listening.

The two subscription models differ and are easy to confuse:

| | `drive.notice.comment_add_v1` | the two `drive.file.bitable_*` keys |
|---|---|---|
| Subscription record | per (app, identity, event type) | per (app, identity, **file**) |
| Takes a param | no | `-p file_token=<base_token>` (required) |
| Covers | that one event type, every document | **every cloud-document file event** on that file |
| Unsubscribed on exit | no (shared by other consumers) | only when this consumer created it |

## Scopes & auth

| EventKey | Scope | Auth |
|---|---|---|
| `drive.notice.comment_add_v1` | `docs:document.comment:read` | user, bot |
| `drive.file.bitable_field_changed_v1` | `base:field:read` | user, bot |
| `drive.file.bitable_record_changed_v1` | `base:record:read` | user, bot |

The platform accepts any one of several scopes per row — `base:field:read`, `bitable:app` or `drive:drive` for the field key, `base:record:read`, `bitable:app` or `drive:drive` for the record key — but `event consume` prechecks exactly the one in the table, so grant that one rather than a broader equivalent.

Subscribing the file needs a scope of its own, separate from the one that reads the event: any one of `docs:event:subscribe`, `drive:drive`, `docs:doc`, `sheets:spreadsheet`. A scope held for the event itself (for example `base:field:read`) does not grant it, and no precheck covers it — the subscribe call fails at startup instead, with `99991679` as user (the envelope carries `missing_scopes` and an `auth login --scope` hint) or `99991672` as bot (the hint carries the console grant link).

The app must also enable the EventKey's event type under event subscriptions in the developer console (long connection mode) and publish a version; otherwise `event consume` fails up-front and prints a console link in `error.hint`.

- `--as bot`: receive notices addressed to the bot, such as a comment or reply that @mentions the bot, and replies in comment threads the bot takes part in (these arrive with `is_mentioned: false`).
- `--as user`: receive the comment notices the authorized user gets, such as being @mentioned or replies in their comment threads.

`to_open_id` is the notice receiver (the bot or the subscribed user); `from_open_id` is the comment author.

## `drive.notice.comment_add_v1`

### Subscription behavior

On startup, `event consume` calls, with the selected identity:

```text
POST /open-apis/drive/v1/user/subscription
{"event_type": "drive.notice.comment_add_v1"}
```

The server keys this subscription by app, identity, and event type, not by document, so there is no `--param`. It is a durable relation: graceful exit makes no unsubscribe call, and re-running the consumer repeats the idempotent subscribe call. To stop receiving notices entirely, cancel it explicitly with the same `--as` identity the consumer used:

```bash
lark-cli drive user remove_subscription --event-type drive.notice.comment_add_v1 --as bot   # or --as user
```

### Examples

```bash
# Comments or replies that @mention the bot
lark-cli event consume drive.notice.comment_add_v1 --as bot --jq 'select(.is_mentioned)'

# Then read the comment the notice points to
lark-cli drive +batch-query-comments --token <file_token> --type <file_type> --comment-ids <comment_id> --as bot
```

The event carries identifiers only, not the comment text. Read the content with `drive +batch-query-comments` (root comment) or `drive +list-replies --comment-id <comment_id>` (replies), using `file_token` / `file_type` from the event. To answer, use `drive +add-reply --comment-id <comment_id>`; full-document comments reject API replies (error `1069302`), so answer those with a new `drive +add-comment` instead.

### Output fields (flat; root path `.`)

| Field | Type | Description |
|---|---|---|
| `type` | string | Always `drive.notice.comment_add_v1` |
| `event_id` | string | Globally unique event ID; safe for deduplication |
| `timestamp` | string (timestamp_ms) | Event delivery time from `header.create_time` |
| `app_id` / `tenant_key` | string | From the event header |
| `file_token` | string | Token of the commented document |
| `file_type` | string | Document type, for example `docx`, `doc`, `sheet`, `file` |
| `comment_id` | string | Comment ID; for a reply this is the parent comment |
| `reply_id` | string | Reply that triggered the notice; for `add_comment` it is the comment's first reply, which holds the comment body |
| `notice_type` | string enum | `add_comment` or `add_reply` |
| `is_mentioned` | bool | Whether the notice receiver was @mentioned; always present |
| `visibility` | string | Comment visibility reported by the server |
| `from_open_id` / `from_union_id` / `from_user_id` | string | Comment author IDs |
| `from_user_type` | string | Comment author type, for example `user` or `bot` |
| `to_open_id` / `to_union_id` / `to_user_id` | string | Notice receiver IDs |

When consuming as the bot, drop the bot's own comments with `--jq 'select(.from_open_id != "<bot_open_id>")'` to avoid reply loops.

## `drive.file.bitable_field_changed_v1` / `drive.file.bitable_record_changed_v1`

### Per-file subscription

Both keys subscribe **per Base**: `event consume` calls `POST /open-apis/drive/v1/files/{file_token}/subscribe?file_type=bitable` on startup with the token passed via `-p file_token=<base_token>`. Missing the param fails validation up-front, before any subscription happens.

That one record covers every cloud-document file event on the file, not just the key being consumed, so two consequences follow:

- Subscribing for field changes also starts record changes, and one unsubscribe stops both.
- A subscription that already existed when the consumer started is **left in place on exit** — tearing it down would cut off whoever created it. Only a subscription this consumer created is removed.

The subscription is also what makes delivery happen at all. With no subscription on the file, editing it produces no event, however the app is configured.

### Which Base you receive

The app's stream carries every Base subscribed under it, so each consumer filters to its own `file_token`. Events for other Bases never reach stdout.

### Output fields (V2 envelope; root path `.event`)

Common to both:

| Field | Type | Description |
|---|---|---|
| `.event.file_token` | string | Base app token |
| `.event.file_type` | string | Always `bitable` |
| `.event.table_id` | string | Table the change happened in |
| `.event.revision` | int | Base revision this change produced |
| `.event.update_time` | int | When the change was applied, in seconds |
| `.event.operator_id.{open_id,union_id,user_id}` | string | Who made the change; `user_id` is null unless the app holds the user_id contact scope |
| `.event.subscriber_id_list[]` | object | Subscribers this delivery was produced for |

`drive.file.bitable_field_changed_v1` adds `.event.action_list[]` with `action` (`field_added` / `field_deleted` / `field_edited`), `field_id`, and `before_value` / `after_value` as `{id, name, type, description, property}` where `type` is the numeric Base field type and `property` is a type-dependent object (`options`, `formula_expression`, `formatter`, …). `property` is delivered on stdout like every other field — output is a verbatim passthrough — but `event schema` does not describe its keys, so read them off a real event rather than the schema. One UI edit can produce several entries: adding a field arrives as `field_added` followed by `field_edited` for the same `field_id`.

`drive.file.bitable_record_changed_v1` adds `.event.action_list[]` with `record_id`, `action` (`record_added` / `record_deleted` / `record_edited`), and `before_value` / `after_value` as a list of `{field_id, field_value}` covering **every field of the table**, not only the changed ones. `field_value` is a string: empty when the cell has no value, and itself JSON for structured types such as text runs. A change to a formula field's computed value does not produce an event.

### Examples

```bash
# Stream record changes on one Base
lark-cli event consume drive.file.bitable_record_changed_v1 \
    -p file_token=<base_token> --as user

# Sample one field change for payload inspection
lark-cli event consume drive.file.bitable_field_changed_v1 \
    -p file_token=<base_token> --as user --max-events 1 --timeout 2m

# Project to "who touched which record"
lark-cli event consume drive.file.bitable_record_changed_v1 \
    -p file_token=<base_token> --as user \
    --jq '{table: .event.table_id, by: .event.operator_id.union_id, records: (.event.action_list | map({(.action): .record_id}))}'

# Then read the changed record back
lark-cli base +record-get --base-token <base_token> --table-id <table_id> \
    --record-id <record_id> --as user
```
