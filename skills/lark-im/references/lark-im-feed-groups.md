# im feed.groups

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first to understand authentication, global parameters, and safety rules.

This reference is the shared annotation target for the IM feed-group (tag) APIs:

- `feed.groups.create`
- `feed.groups.update`
- `feed.groups.delete`
- `feed.groups.batch_query`
- `feed.groups.list`
- `feed.groups.batch_add_item`
- `feed.groups.batch_remove_item`
- `feed.groups.batch_query_item`
- `feed.groups.list_item`

It focuses on:

- What each method does
- The request / response shape you need when calling the raw API commands
- The enum surface (group types, rule conditions, feed card types) used in payloads

> **Important:** All `feed.groups.*` methods are user-only (`user_access_token`) and accept structured input through `--params '<json>'` and `--data '<json>'`. They do not expose typed flags such as `--feed-group-id` directly. There is no `+` shortcut wrapper.

## Command Overview

| Method | HTTP | Path | Purpose |
|---|---|---|---|
| `feed.groups.create` | `POST` | `/open-apis/im/v1/groups` | Create a new feed group (tag) |
| `feed.groups.update` | `PUT` | `/open-apis/im/v1/groups/{feed_group_id}` | Update a feed group's name and/or rules |
| `feed.groups.delete` | `DELETE` | `/open-apis/im/v1/groups/{feed_group_id}` | Delete one feed group |
| `feed.groups.batch_query` | `POST` | `/open-apis/im/v1/groups/batch_query` | Look up feed groups by ID list |
| `feed.groups.list` | `GET` | `/open-apis/im/v1/groups` | List the caller's feed groups with optional time-range filter |
| `feed.groups.batch_add_item` | `POST` | `/open-apis/im/v1/groups/{feed_group_id}/batch_add_item` | Add feed cards (chats) into a feed group |
| `feed.groups.batch_remove_item` | `POST` | `/open-apis/im/v1/groups/{feed_group_id}/batch_remove_item` | Remove feed cards from a feed group |
| `feed.groups.batch_query_item` | `POST` | `/open-apis/im/v1/groups/{feed_group_id}/batch_query_item` | Look up feed cards inside a group by ID list |
| `feed.groups.list_item` | `GET` | `/open-apis/im/v1/groups/{feed_group_id}/list_item` | List feed cards inside one feed group |

## Common Notes

- `feed_group_id` is the feed-group identifier returned by `create`, typically formatted as `ofg_xxx`. In meta examples it appears as a string; on the wire it is the group's stable ID.
- `feed_id` is the identifier of one feed card inside a group. In v1 only the `chat` feed card type is supported (see `feed_card_type` below), so `feed_id` is currently a chat ID such as `oc_xxx`.
- All `feed.groups.*` methods require `user_access_token`. Run with `--as user`; bot/tenant tokens are rejected.
- Read APIs (`batch_query`, `list`, `batch_query_item`, `list_item`) return **two parallel lists**: a live list (`groups[]` or `items[]`) and a soft-deleted list (`deleted_groups[]` or `deleted_items[]`). Consumers tracking incremental sync should consume both.
- Time-range fields (`start_time`, `end_time`, `update_time`) are Unix timestamps **in milliseconds**, encoded as decimal strings (e.g. `1767196800000`).
- Rule-based feed groups (`type=rule`) auto-populate from the rules declared in `feed_group_creator.rules`. Normal feed groups (`type=normal`) are managed explicitly via `batch_add_item` / `batch_remove_item`.

## Inspect Schema

```bash
lark-cli schema im.feed.groups
lark-cli schema im.feed.groups.create --format pretty
lark-cli schema im.feed.groups.list --format pretty
lark-cli schema im.feed.groups.batch_add_item --format pretty
lark-cli schema im.feed.groups.list_item --format pretty
```

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

### Request

#### `--params`

| Parameter | Required | Description |
|---|---|---|
| `user_id_type` | No | ID type used when the request body contains `user_id` references inside rules. One of `open_id`, `union_id`, `user_id` |

#### `--data`

| Field | Required | Description |
|---|---|---|
| `feed_group_creator.type` | Yes | `normal` (empty group) or `rule` (auto-populated by rules) |
| `feed_group_creator.name` | Yes | Display name, e.g. `"标签名称测试"` |
| `feed_group_creator.rules` | No | Rule object (required when `type=rule`). See `feed_group_rules` section below |

### Response

```json
{
  "group_id": "ofg_xxx"
}
```

## update

Update a feed group's name and/or rules. The `update_fields` array tells the server which fields are being updated.

```bash
# Rename only
lark-cli im feed.groups update --as user \
  --params '{"feed_group_id":"ofg_xxx"}' \
  --data '{"feed_group_updater":{"name":"测试标签名称","update_fields":["name"]}}'

# Replace rules only (rules array uses the feed_group_rules shape — see that section)
lark-cli im feed.groups update --as user \
  --params '{"feed_group_id":"ofg_xxx"}' \
  --data '{
    "feed_group_updater":{
      "rules":{"rules":[]},
      "update_fields":["rules"]
    }
  }'
```

### Request

#### `--params`

| Parameter | Required | Description |
|---|---|---|
| `feed_group_id` | Yes | Path parameter — the feed group to update |
| `user_id_type` | No | ID type for any `user_id` fields inside `rules` |

#### `--data`

| Field | Required | Description |
|---|---|---|
| `feed_group_updater.name` | No | New display name |
| `feed_group_updater.rules` | No | Replacement rule object. Same structure as `create.feed_group_creator.rules` |
| `feed_group_updater.update_fields` | No | Array of update markers: `name`, `rules`. Server applies only the listed fields |

### Response

Empty body on success. Inspect the CLI exit code for status.

## delete

Delete one feed group.

```bash
lark-cli im feed.groups delete --as user \
  --params '{"feed_group_id":"ofg_xxx"}'
```

### Request

| Parameter | Required | Description |
|---|---|---|
| `feed_group_id` | Yes | Path parameter — the feed group to delete |

### Response

Empty body on success.

## batch_query

Look up feed groups by an explicit list of IDs. Returns both live and soft-deleted matches.

```bash
lark-cli im feed.groups batch_query --as user \
  --params '{"user_id_type":"open_id"}' \
  --data '{"group_ids":["ofg_xxx","ofg_yyy"]}'
```

### Request

#### `--params`

| Parameter | Required | Description |
|---|---|---|
| `user_id_type` | No | ID type used when the response includes `user_id` references inside `groups[].rules` |

#### `--data`

| Field | Required | Description |
|---|---|---|
| `group_ids` | Yes | Array of feed group IDs to look up |

### Response

```json
{
  "groups": [
    {
      "group_id": "ofg_xxx",
      "type": "normal",
      "name": "test",
      "rules": { "rules": [] }
    }
  ],
  "deleted_groups": [
    {
      "group_id": "ofg_yyy",
      "type": "rule",
      "name": "test",
      "rules": { "rules": [] }
    }
  ]
}
```

Each `rules.rules[]` element follows the `feed_group_rules` shape — see that section for the full structure.

### Top-Level Fields

| Field | Type | Meaning |
|---|---|---|
| `groups` | `array<object>` | Live feed groups for the requested IDs |
| `deleted_groups` | `array<object>` | Soft-deleted matches, returned for incremental-sync clients |

Each element carries `group_id`, `type`, `name`, and (when defined) `rules`.

## list

List the caller's feed groups, optionally filtered by an update-time window and paginated.

```bash
# First page
lark-cli im feed.groups list --as user

# With pagination + time range
lark-cli im feed.groups list --as user \
  --params '{
    "page_size":50,
    "page_token":"<PAGE_TOKEN>",
    "start_time":"1767196800000",
    "end_time":"1767200000000",
    "user_id_type":"open_id"
  }'
```

### Request (`--params`)

| Parameter | Required | Description |
|---|---|---|
| `page_token` | No | Continuation token returned by the previous page |
| `page_size` | No | Records per page |
| `start_time` | No | Update-time window start (Unix milliseconds as a decimal string) |
| `end_time` | No | Update-time window end (Unix milliseconds as a decimal string) |
| `user_id_type` | No | ID type used when the response includes `user_id` references inside `groups[].rules` |

### Response

```json
{
  "groups": [
    {
      "group_id": "ofg_xxx",
      "type": "normal",
      "name": "test",
      "rules": { "rules": [] }
    }
  ],
  "deleted_groups": [
    { "group_id": "ofg_yyy", "type": "rule", "name": "test", "rules": { "rules": [] } }
  ],
  "page_token": "YhljsPiGfUgnVAg9urvRFd-BvSqRLxxxx",
  "has_more": true
}
```

`rules.rules[]` elements follow the `feed_group_rules` shape — see that section.

| Field | Type | Meaning |
|---|---|---|
| `groups` | `array<object>` | Live feed groups in this page |
| `deleted_groups` | `array<object>` | Soft-deleted feed groups in this page |
| `page_token` | `string` | Token for the next page when `has_more=true` |
| `has_more` | `boolean` | Whether more pages exist |

## batch_add_item

Add feed cards (chats) into one feed group. Partial failures are reported in `failed_items`.

```bash
lark-cli im feed.groups batch_add_item --as user \
  --params '{"feed_group_id":"ofg_xxx"}' \
  --data '{
    "items":[
      {"feed_id":"oc_xxx","feed_type":"chat"},
      {"feed_id":"oc_yyy","feed_type":"chat"}
    ]
  }'
```

### Request

| Source | Field | Required | Description |
|---|---|---|---|
| `--params` | `feed_group_id` | Yes | Path parameter — the target feed group |
| `--data` | `items[]` | Yes | Array of feed cards to add |
| `--data` | `items[].feed_id` | No | The chat ID to add (e.g. `oc_xxx`) |
| `--data` | `items[].feed_type` | Yes (`"chat"` only) | Wire-typed as an open string. v1 OAPI service accepts only `chat`; anything else is rejected at runtime. See the Enums section. |

> Note: `items[].feed_id` is marked `Required: No` in the meta but every element of `items` must set it — a missing field yields an unusable entry. Always pass `{"feed_id": "oc_xxx", "feed_type": "chat"}` per item.

### Response

```json
{
  "failed_items": [
    {
      "item": { "feed_id": "oc_xxx", "feed_type": "chat" },
      "error_code": 240001,
      "error_message": "feed_id is invalid"
    }
  ]
}
```

| Field | Type | Meaning |
|---|---|---|
| `failed_items` | `array<object>` | Items that failed; absent or empty means all succeeded |
| `failed_items[].item` | `object` | The original `{feed_id, feed_type}` element |
| `failed_items[].error_code` | `integer` | Numeric error code |
| `failed_items[].error_message` | `string` | Human-readable failure reason |

## batch_remove_item

Remove feed cards from one feed group. Same request and response shape as `batch_add_item`.

```bash
lark-cli im feed.groups batch_remove_item --as user \
  --params '{"feed_group_id":"ofg_xxx"}' \
  --data '{
    "items":[
      {"feed_id":"oc_xxx","feed_type":"chat"}
    ]
  }'
```

### Request

| Source | Field | Required | Description |
|---|---|---|---|
| `--params` | `feed_group_id` | Yes | Path parameter — the target feed group |
| `--data` | `items[]` | Yes | Array of feed cards to remove |
| `--data` | `items[].feed_id` | No | The chat ID to remove |
| `--data` | `items[].feed_type` | Yes (`"chat"` only) | Wire-typed as an open string. v1 OAPI service accepts only `chat`; anything else is rejected at runtime. See the Enums section. |

> Note: same caveat as `batch_add_item` — `items[].feed_id` is `Required: No` per the meta but must be present in practice.

### Response

Identical shape to `batch_add_item` — `failed_items[]` lists rows that did not remove cleanly.

## batch_query_item

Look up feed cards inside one feed group by an explicit list of IDs.

```bash
lark-cli im feed.groups batch_query_item --as user \
  --params '{"feed_group_id":"ofg_xxx"}' \
  --data '{
    "items":[
      {"feed_id":"oc_xxx","feed_type":"chat"},
      {"feed_id":"oc_yyy","feed_type":"chat"}
    ]
  }'
```

### Request

| Source | Field | Required | Description |
|---|---|---|---|
| `--params` | `feed_group_id` | Yes | Path parameter — the feed group to look in |
| `--data` | `items[]` | Yes | Array of feed cards to query |
| `--data` | `items[].feed_id` | No | The chat ID to query |
| `--data` | `items[].feed_type` | Yes (`"chat"` only) | Wire-typed as an open string. v1 OAPI service accepts only `chat`; anything else is rejected at runtime. See the Enums section. |

> Note: same caveat as `batch_add_item` — `items[].feed_id` is `Required: No` per the meta but must be present in practice.

### Response

```json
{
  "items": [
    { "feed_id": "oc_xxx", "feed_type": "chat", "update_time": "1767196800000" }
  ],
  "deleted_items": [
    { "feed_id": "oc_yyy", "feed_type": "chat", "update_time": "1767196800000" }
  ]
}
```

| Field | Type | Meaning |
|---|---|---|
| `items` | `array<object>` | Live feed cards matching the query |
| `deleted_items` | `array<object>` | Soft-deleted matches |
| `items[].update_time` | `string` | Last update timestamp (Unix milliseconds as a decimal string) |

## list_item

List feed cards inside one feed group, with optional update-time window and pagination.

```bash
lark-cli im feed.groups list_item --as user \
  --params '{
    "feed_group_id":"ofg_xxx",
    "page_size":50,
    "page_token":"<PAGE_TOKEN>",
    "start_time":"1767196800000",
    "end_time":"1767200000000"
  }'
```

### Request (`--params`)

| Parameter | Required | Description |
|---|---|---|
| `feed_group_id` | Yes | Path parameter — the feed group to list |
| `page_token` | No | Continuation token returned by the previous page |
| `page_size` | No | Records per page |
| `start_time` | No | Update-time window start (Unix milliseconds as a decimal string) |
| `end_time` | No | Update-time window end (Unix milliseconds as a decimal string) |

### Response

```json
{
  "items": [
    { "feed_id": "oc_xxx", "feed_type": "chat", "update_time": "1767196800000" }
  ],
  "deleted_items": [
    { "feed_id": "oc_yyy", "feed_type": "chat", "update_time": "1767196800000" }
  ],
  "page_token": "YhljsPiGfUgnVAg9urvRFd-BvSqRLxxxx",
  "has_more": true
}
```

| Field | Type | Meaning |
|---|---|---|
| `items` | `array<object>` | Live feed cards in this page |
| `deleted_items` | `array<object>` | Soft-deleted feed cards in this page |
| `page_token` | `string` | Token for the next page when `has_more=true` |
| `has_more` | `boolean` | Whether more pages exist |

## Enums

The enums below are sourced from the internal datasync IDL (`lark.im.datasync.open.thrift`). All values listed here are exhaustive.

### `feed_group_type`

Used in `feed_group_creator.type` and the response `groups[].type`.

- `normal` — empty group; members managed explicitly via `batch_add_item` / `batch_remove_item`.
- `rule` — auto-populated; `feed_group_creator.rules` must be supplied.

### `feed_card_type`

Used in `items[].feed_type` everywhere a feed card appears. Wire type is the open string alias `FeedCardTypeV1`.

- `chat` — the only value the v1 OAPI service accepts. `feed_id` is therefore a chat ID such as `oc_xxx`.

The CLI does not pre-validate this field — passing anything other than `chat` reaches the server and is rejected at runtime. Treat `chat` as effectively required.

### `feed_group_rule_action`

Used inside `feed_group_rules.rules[].action`.

- `add` — when the condition matches, add the matching feed into this group.
- `remove` — when the condition matches, remove the matching feed from this group.

### `feed_group_rule_cond_match_type`

Used inside `feed_group_rules.rules[].condition.match_type`.

- `match_all` — every condition item must match.
- `match_any` — at least one condition item must match.

### `feed_group_rule_cond_item_type`

Used inside `feed_group_rules.rules[].condition.condition_items[].type`. Determines which sibling field of the item is consulted.

- `keyword` — match against a keyword; consult the `keyword` field.
- `chatter` — match against a user; consult the `user_id` field (interpreted per the request's `user_id_type`).
- `chat_type` — match against a chat type; consult the `chat_type` field.

### `feed_group_rule_cond_item_operator`

Used inside `feed_group_rules.rules[].condition.condition_items[].operator`. Typically paired with the relevant `type`:

- `contain` — substring match; typically paired with `keyword`.
- `not_contain` — substring non-match; typically paired with `keyword`.
- `is` — equality; typically paired with `chatter` or `chat_type`.
- `is_not` — non-equality; typically paired with `chatter` or `chat_type`.

### `feed_group_rule_cond_item_chat_type`

Used inside `feed_group_rules.rules[].condition.condition_items[].chat_type` when `type=chat_type`.

- `p2p`
- `group`
- `thread_group`
- `helpdesk`
- `bot`
- `mute`
- `flag`
- `cross_tenant`
- `any`

### `update_fields`

Used inside `feed_group_updater.update_fields`. Multiple values may be listed.

- `name` — update name only.
- `rules` — update rules only.

Wire form: lowercase strings (`"name"`, `"rules"`), even though the underlying IDL defines an integer enum (`FeedGroupUpdateField`). Omit the array (or pass an empty array) to make no field updates.

## feed_group_rules

The same nested object is used in `feed_group_creator.rules` (create), `feed_group_updater.rules` (update), and in read responses under `groups[].rules`. Shape:

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

- `type=keyword` → `keyword` is required; `user_id` and `chat_type` are ignored.
- `type=chatter` → `user_id` is required; the request's `user_id_type` query parameter tells the server how to interpret it.
- `type=chat_type` → `chat_type` is required.

## Permissions

| Method | Scope |
|---|---|
| `feed.groups.create` | `im:feed_group_v1:write` |
| `feed.groups.update` | `im:feed_group_v1:write` |
| `feed.groups.delete` | `im:feed_group_v1:write` |
| `feed.groups.batch_query` | `im:feed_group_v1:read` |
| `feed.groups.list` | `im:feed_group_v1:read` |
| `feed.groups.batch_add_item` | `im:feed_group_v1:write` |
| `feed.groups.batch_remove_item` | `im:feed_group_v1:write` |
| `feed.groups.batch_query_item` | `im:feed_group_v1:read` |
| `feed.groups.list_item` | `im:feed_group_v1:read` |

If a required scope is missing, the CLI surfaces a hint such as `lark-cli auth login --scope "im:feed_group_v1:write"`.

## References

- [lark-im](../SKILL.md) — all IM commands
- [lark-shared](../../lark-shared/SKILL.md) — authentication and global parameters
- Design wiki: `https://bytedance.larkoffice.com/wiki/LIdSwrCzaitg3MkH8oScLhBCnFQ`
- IDL source (internal): `lark.im.datasync.open.thrift`
