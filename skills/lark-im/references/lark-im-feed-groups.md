# im feed.groups

> **Prerequisite:** Before executing this command, ensure [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) has been read once in the current task for authentication, global parameters, and safety rules. Do not reread it if already loaded.

Maps to `lark-cli im feed.groups <method>` (six raw methods, structured input via `--params` / `--data`) plus three typed read shortcuts. **Run `lark-cli schema im.feed.groups.<method> --format pretty` for the authoritative parameters and field shapes, and `lark-cli im +feed-group-list --help` (also `+feed-group-list-item`, `+feed-group-query-item`) for the shortcut flags.** This file covers only what those cannot.

## CLI surface split

Six methods are raw-only, three are shortcut-only — and the shortcut-only ones have **no raw schema at all**, so `lark-cli schema im.feed.groups.list` fails rather than returning a definition:

| Method | CLI surface |
|---|---|
| `create` `update` `delete` `batch_query` `batch_add_item` `batch_remove_item` | raw command + `schema` |
| `list` | **only** [`+feed-group-list`](lark-im-feed-group-list.md) |
| `list_item` / `feed.groups.list_item` | **only** [`+feed-group-list-item`](lark-im-feed-group-list-item.md) |
| `batch_query_item` / `feed.groups.batch_query_item` | **only** [`+feed-group-query-item`](lark-im-feed-group-query-item.md) |

**Picking a read method:** `batch_query` / `+feed-group-query-item` are lightweight ID lookups; `+feed-group-list` / `+feed-group-list-item` paginate the whole set and are much heavier. When you already hold the IDs (`group_id` from `create`, the `feed_id`s you passed to `batch_add_item`), prefer the lightweight lookup. Reserve the list methods for discovering IDs you don't have.

## Common Notes

- `feed_id` is the identifier of one feed card inside a group. Because only the `chat` feed card type exists in v1 (see `feed_card_type` below), `feed_id` is currently always a chat ID such as `oc_xxx` — there is no separate feed-card ID space to look up.
- Read methods return **two parallel lists**: a live list (`groups[]` or `items[]`) and a soft-deleted list (`deleted_groups[]` or `deleted_items[]`). Consumers tracking incremental sync must consume **both** — reading only the live list silently misses removals.
- Rule-based groups (`type=rule`) auto-populate from `feed_group_creator.rules`. Normal groups (`type=normal`) are managed explicitly via `batch_add_item` / `batch_remove_item`.

> **Choose the simplest group that fits** — it keeps `create` / `update` fast and predictable. Apply these in order:
> 1. **Prefer `type=normal`.** When the target chats are known up front, set membership explicitly with `batch_add_item` / `batch_remove_item`. Use `type=rule` only when membership must be derived automatically.
> 2. **Keep the rule set smallest.** Use the fewest `rules[]` and `condition_items[]` that express the intent (one condition is ideal). This outranks the style rules below — never split a rule or add conditions just to satisfy them (e.g. one `match_any` rule beats two single-condition rules for "A or B").
> 3. **Within that, make each condition precise.** Prefer positive, specific conditions (`is`, or `contain` with a distinctive keyword) over exclusion (`is_not`, `not_contain`) or broad keywords, which capture more than intended. For a multi-condition rule, prefer `match_all` (narrower) over `match_any` (wider).

## Gotchas

- **`items[].feed_id` is required in practice but not in the schema.** The API definition leaves it optional; every element of `items` must still set it, or the entry is unusable. Always pass `{"feed_id":"oc_xxx","feed_type":"chat"}` per item — for both `batch_add_item` and `batch_remove_item`.
- **`feed_type` is not pre-validated by the CLI.** It is wire-typed as an open string, so a wrong value reaches the server and is rejected at runtime rather than failing locally.
- **`batch_add_item` / `batch_remove_item` fail partially.** Success is not all-or-nothing — check `failed_items[]`; absent or empty means everything succeeded.
- **`update_fields` takes integers, not names.** See the enum below; the string forms are rejected by the server, not by the CLI.

## `feed_group_rules`

The same nested object is used in `feed_group_creator.rules` (create), `feed_group_updater.rules` (update), and read responses under `groups[].rules`:

```json
{
  "rules": [
    {
      "condition": {
        "match_type": "match_all",
        "condition_items": [
          { "type": "chat_type", "operator": "is", "chat_type": "group" },
          { "type": "keyword",   "operator": "contain", "keyword": "release" }
        ]
      },
      "action": "add"
    }
  ]
}
```

Per-`type` required-field legend — the sibling field consulted depends on `type`, and the others are ignored:

- `type=keyword` → `keyword` is required; `user_id` and `chat_type` are ignored.
- `type=chatter` → `user_id` is required; the request's `user_id_type` query parameter tells the server how to interpret it.
- `type=chat_type` → `chat_type` is required.

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

### Enum values

All values below are exhaustive. None appear in `--help` or `schema` output.

**`feed_group_type`** — `feed_group_creator.type` and response `groups[].type`:

- `normal` — empty group; members managed explicitly via `batch_add_item` / `batch_remove_item`.
- `rule` — auto-populated; `feed_group_creator.rules` must be supplied.

**`feed_card_type`** — `items[].feed_type` everywhere a feed card appears:

- `chat` — the only value the v1 OAPI service accepts. `feed_id` is therefore a chat ID such as `oc_xxx`.

**`feed_group_rule_action`** — `feed_group_rules.rules[].action`:

- `add` — when the condition matches, add the matching feed into this group.
- `remove` — when the condition matches, remove the matching feed from this group.

**`feed_group_rule_cond_match_type`** — `feed_group_rules.rules[].condition.match_type`:

- `match_all` — every condition item must match.
- `match_any` — at least one condition item must match.

**`feed_group_rule_cond_item_type`** — `condition_items[].type`; determines which sibling field is consulted:

- `keyword` — match against a keyword; consult the `keyword` field.
- `chatter` — match against a user; consult the `user_id` field (interpreted per the request's `user_id_type`).
- `chat_type` — match against a chat type; consult the `chat_type` field.

**`feed_group_rule_cond_item_operator`** — `condition_items[].operator`, typically paired with the relevant `type`:

- `contain` — substring match; typically paired with `keyword`.
- `not_contain` — substring non-match; typically paired with `keyword`.
- `is` — equality; typically paired with `chatter` or `chat_type`.
- `is_not` — non-equality; typically paired with `chatter` or `chat_type`.

**`feed_group_rule_cond_item_chat_type`** — `condition_items[].chat_type` when `type=chat_type`:

- `p2p`
- `group`
- `thread_group`
- `helpdesk`
- `bot`
- `mute`
- `flag`
- `cross_tenant`
- `any`

**`update_fields`** — `feed_group_updater.update_fields`; multiple values may be listed:

- `1` — update name only.
- `2` — update rules only.

Wire form is integers. The server rejects the lowercase string forms (`"name"`, `"rules"`) with `9499 Invalid parameter value`. Omit the array (or pass an empty array) to make no field updates.

### Scope requirements

`--help` prints no scope information, and the two enriching shortcuts need a second scope that is declared only in Go source:

- `+feed-group-list` — `im:feed_group_v1:read`
- `+feed-group-list-item` / `+feed-group-query-item` — `im:feed_group_v1:read` **plus** `im:chat:read`, because they always resolve `chat_name` through a follow-up `chats/batch_query`
- The six raw methods — `im:feed_group_v1:write` for `create` / `update` / `delete` / `batch_add_item` / `batch_remove_item`, `im:feed_group_v1:read` for `batch_query`

When a scope is missing the CLI surfaces a hint such as `lark-cli auth login --scope "im:feed_group_v1:write"`.

## References

- [lark-im](../SKILL.md) — all IM commands
