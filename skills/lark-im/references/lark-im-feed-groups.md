# im feed.groups

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first to understand authentication, global parameters, and safety rules.

This reference covers the IM feed-group (tag) APIs. **There is no resolvable `--help` or `schema` for the six raw `feed.groups.*` write methods** (see [Schema gap](#schema-gap)), so the request/response shapes, enums, and rule guidance below are the only source of truth and are kept in full. The three read methods are exposed only as typed `+` shortcuts — for those, see the sibling references and their `--help` rather than this file.

> **All `feed.groups.*` methods are user-only** — they require `user_access_token`. Run with `--as user`; bot/tenant tokens are rejected.

## Schema gap

> Report this when you hit it: `lark-cli schema im.feed.groups.<method>` does **not** resolve — there is no `feed` service and no `im.feed*` resource in the schema registry (`lark-cli schema im` lists only `chat.members, chats, images, messages, pins, reactions, threads`). Do **not** send users to `schema` for feed-group methods. Until the CLI registers these methods, this reference is the authority for the six raw methods, and the three shortcuts' `--help` is the authority for the read methods.

## Routing: pick the right method

- **Read paths are shortcut-only** — `list`, `list_item`, `batch_query_item` have **no** raw command. Use the typed `+` shortcuts:
  - [`+feed-group-list`](lark-im-feed-group-list.md) — list your feed groups (`--page-all` merges live + soft-deleted). No enrichment. Scope: `im:feed_group_v1:read`.
  - [`+feed-group-list-item`](lark-im-feed-group-list-item.md) — list feed cards in a group, enriched with `chat_name`. Scopes: `im:feed_group_v1:read` + `im:chat:read`.
  - [`+feed-group-query-item`](lark-im-feed-group-query-item.md) — look up feed cards by ID, enriched with `chat_name`. Scopes: `im:feed_group_v1:read` + `im:chat:read`.
- **Lightweight lookup vs. heavy list**: when you already hold the IDs (`group_id` from `create`, the `feed_id`s you passed to `batch_add_item`), prefer the lightweight ID lookups (`batch_query` / `+feed-group-query-item`) over the paginated list methods (`+feed-group-list` / `+feed-group-list-item`), which are much heavier. Reserve the list methods for discovering IDs you don't have.
- **`type=normal` vs `type=rule`**: a `normal` group's membership is managed explicitly via `batch_add_item` / `batch_remove_item`; a `rule` group auto-populates from `feed_group_creator.rules`. See [rule guidance](#choosing-a-group-shape).

## Choosing a group shape

> **Choose the simplest group that fits** — it keeps `create` / `update` fast and predictable. Apply in order:
> 1. **Prefer `type=normal`.** When the target chats are known up front, set membership explicitly with `batch_add_item` / `batch_remove_item`. Use `type=rule` only when membership must be derived automatically.
> 2. **Keep the rule set smallest.** Use the fewest `rules[]` and `condition_items[]` that express the intent (one condition is ideal). This outranks the precision rule below — never split a rule or add conditions just to satisfy it (e.g. one `match_any` rule beats two single-condition rules for "A or B").
> 3. **Within that, make each condition precise.** Prefer positive, specific conditions (`is`, or `contain` with a distinctive keyword) over exclusion (`is_not`, `not_contain`) or broad keywords, which capture more than intended. For a multi-condition rule, prefer `match_all` (narrower) over `match_any` (wider).

## Identity / ID conventions (shared)

- `feed_group_id` — the feed-group identifier returned by `create`, formatted as `ofg_xxx`.
- `feed_id` — the identifier of one feed card inside a group. In v1 only the `chat` feed card type is supported, so `feed_id` is currently a chat ID such as `oc_xxx`.
- **Read APIs return two parallel lists** — a live list (`groups[]` or `items[]`) and a soft-deleted list (`deleted_groups[]` or `deleted_items[]`). Consumers tracking incremental sync must consume both.

---

# HELP-GAP — raw `feed.groups.*` write methods (no `--help`/schema; keep until CLI adds it)

> Everything from here down documents the six raw methods that take `--params '<json>'` / `--data '<json>'`. None of it is expressible via `--help` or `schema` today (the methods are unregistered — see [Schema gap](#schema-gap)). Once the CLI registers these methods, replace this section with a pointer to schema.

## create

Create a new feed group. Returns the new `group_id` on success.

```bash
# Normal (empty) group
lark-cli im feed.groups create --as user \
  --data '{"feed_group_creator":{"type":"normal","name":"Releases"}}'

# Rule-based group: auto-add p2p chats with "release" in their name
lark-cli im feed.groups create --as user \
  --data '{
    "feed_group_creator":{
      "type":"rule",
      "name":"Auto: release chats",
      "rules":{
        "rules":[
          {
            "condition":{
              "match_type":"match_all",
              "condition_items":[
                {"type":"chat_type","operator":"is","chat_type":"p2p"},
                {"type":"keyword","operator":"contain","keyword":"release"}
              ]
            },
            "action":"add"
          }
        ]
      }
    }
  }'
```

- `--params`: `user_id_type` (optional) — `open_id` | `union_id` | `user_id`; used when the body contains `user_id` references inside rules.
- `--data`: `feed_group_creator.type` (required: `normal` | `rule`) · `feed_group_creator.name` (required) · `feed_group_creator.rules` (required when `type=rule`; see [feed_group_rules](#feed_group_rules)).
- Response: `{"group_id":"ofg_xxx"}`.

## update

Update a feed group's name and/or rules. The `update_fields` array tells the server which fields to apply.

> **Scope each update to what actually changed.** To rename only, pass `update_fields:[1]` so rules are left untouched. When you do change rules, the [group-shape guidance](#choosing-a-group-shape) applies to the resulting set.

```bash
# Rename only
lark-cli im feed.groups update --as user \
  --params '{"feed_group_id":"ofg_xxx"}' \
  --data '{"feed_group_updater":{"name":"测试标签名称","update_fields":[1]}}'

# Replace rules only
lark-cli im feed.groups update --as user \
  --params '{"feed_group_id":"ofg_xxx"}' \
  --data '{"feed_group_updater":{"rules":{"rules":[]},"update_fields":[2]}}'
```

- `--params`: `feed_group_id` (required, path) · `user_id_type` (optional, for `user_id` fields inside `rules`).
- `--data`: `feed_group_updater.name` (optional) · `feed_group_updater.rules` (optional; same shape as `create`) · `feed_group_updater.update_fields` (optional integer markers: `1`=name, `2`=rules — server applies only the listed fields).
- Response: empty body on success — inspect the CLI exit code for status.

> **`update_fields` takes integers, not strings.** The server rejects the lowercase string forms (`"name"`, `"rules"`) with `9499 Invalid parameter value`. Use `[1]` / `[2]`. Omit the array (or pass `[]`) to make no field updates.

## delete

```bash
lark-cli im feed.groups delete --as user --params '{"feed_group_id":"ofg_xxx"}'
```

- `--params`: `feed_group_id` (required, path). Response: empty body on success.

## batch_query

Look up feed groups by an explicit ID list. Returns both live and soft-deleted matches.

```bash
lark-cli im feed.groups batch_query --as user \
  --params '{"user_id_type":"open_id"}' \
  --data '{"group_ids":["ofg_xxx","ofg_yyy"]}'
```

- `--params`: `user_id_type` (optional) — interprets `user_id` references inside `groups[].rules`.
- `--data`: `group_ids` (required array).
- Response: `{"groups":[...],"deleted_groups":[...]}`; each element carries `group_id`, `type`, `name`, and (when defined) `rules` (the [feed_group_rules](#feed_group_rules) shape). `deleted_groups[]` is the soft-deleted list returned for incremental-sync clients.

## batch_add_item

Add feed cards (chats) into one feed group. Partial failures are reported in `failed_items`.

```bash
lark-cli im feed.groups batch_add_item --as user \
  --params '{"feed_group_id":"ofg_xxx"}' \
  --data '{"items":[{"feed_id":"oc_xxx","feed_type":"chat"},{"feed_id":"oc_yyy","feed_type":"chat"}]}'
```

- `--params`: `feed_group_id` (required, path).
- `--data`: `items[]` (required array). Each item: `feed_id` (the chat ID, e.g. `oc_xxx`) and `feed_type` (`"chat"` only).
- Response: `{"failed_items":[{"item":{...},"error_code":<int>,"error_message":"..."}]}`. **`failed_items` absent or empty means all succeeded** — check it for partial failure; it lists the original `{feed_id, feed_type}` plus a numeric `error_code` and human-readable `error_message`.

> **`items[].feed_id` is a trap.** Although the meta marks it `Required: No`, every element of `items` must set it — a missing `feed_id` yields an unusable entry. Always pass `{"feed_id":"oc_xxx","feed_type":"chat"}` per item.

## batch_remove_item

Remove feed cards from one feed group. **Same request and response shape as `batch_add_item`** — same `feed_group_id` path param, same `items[]` body, same `failed_items[]` response, and the same `feed_id`-required-in-practice caveat.

```bash
lark-cli im feed.groups batch_remove_item --as user \
  --params '{"feed_group_id":"ofg_xxx"}' \
  --data '{"items":[{"feed_id":"oc_xxx","feed_type":"chat"}]}'
```

## Enums (raw methods)

Sourced from the internal datasync IDL (`lark.im.datasync.open.thrift`). Values listed are exhaustive.

- **`feed_group_type`** (`feed_group_creator.type`, response `groups[].type`): `normal` (empty; managed via `batch_add_item`/`batch_remove_item`) · `rule` (auto-populated; requires `feed_group_creator.rules`).
- **`feed_card_type`** (`items[].feed_type`; wire alias `FeedCardTypeV1`): `chat` is the **only** value the v1 OAPI service accepts, so `feed_id` is a chat ID (`oc_xxx`). **The CLI does not pre-validate this** — anything other than `chat` reaches the server and is rejected at runtime. Treat `chat` as effectively required.
- **`feed_group_rule_action`** (`rules[].action`): `add` · `remove`.
- **`feed_group_rule_cond_match_type`** (`rules[].condition.match_type`): `match_all` (every item must match) · `match_any` (at least one).
- **`feed_group_rule_cond_item_type`** (`condition_items[].type` — determines which sibling field is consulted): `keyword` (→ `keyword` field) · `chatter` (→ `user_id` field, interpreted per `user_id_type`) · `chat_type` (→ `chat_type` field).
- **`feed_group_rule_cond_item_operator`** (`condition_items[].operator`): `contain` / `not_contain` (substring, with `keyword`) · `is` / `is_not` (equality, with `chatter` or `chat_type`).
- **`feed_group_rule_cond_item_chat_type`** (`condition_items[].chat_type` when `type=chat_type`): `p2p` · `group` · `thread_group` · `helpdesk` · `bot` · `mute` · `flag` · `cross_tenant` · `any`.
- **`update_fields`** (`feed_group_updater.update_fields`): integers `1` = name, `2` = rules (multiple may be listed). See the string-vs-integer trap under [update](#update).

## feed_group_rules

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

Per-`type` required-field legend:

- `type=keyword` → `keyword` required; `user_id` and `chat_type` ignored.
- `type=chatter` → `user_id` required; the request's `user_id_type` query parameter tells the server how to interpret it.
- `type=chat_type` → `chat_type` required.

## Permissions (raw write methods)

- `create` / `update` / `delete` / `batch_add_item` / `batch_remove_item` → `im:feed_group_v1:write`.
- `batch_query` → `im:feed_group_v1:read`.
- Read shortcuts: scopes listed under [Routing](#routing-pick-the-right-method) and in each shortcut doc.

If a required scope is missing, the CLI surfaces a hint such as `lark-cli auth login --scope "im:feed_group_v1:write"`.

## References

- [lark-im](../SKILL.md) — all IM commands
- [lark-shared](../../lark-shared/SKILL.md) — authentication and global parameters
