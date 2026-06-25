# card.action.trigger

> **Prerequisite:** Read [`../../lark-event/SKILL.md`](../../lark-event/SKILL.md) first for `event consume` essentials.

Fires when a user interacts with an interactive card — button click, form submit, dropdown select,
checkbox toggle, date/time pick, etc.

## Setup (required)

> **Console configuration required**: In the Feishu Developer Console, go to
> **App → Events & Callbacks → Callback Configuration** (应用--事件与回调--回调配置) and enable it.
> The consumer starts without errors even when not configured, but **no events will be received**.
> There is no preflight check for this setting.

After enabling, events are delivered over the existing WebSocket long connection — no additional
URL configuration needed.

## Scopes & auth

| Scope | Required for |
|---|---|
| `im:message:readonly` | Auto-fetch `card_content` via message get API (covers both p2p and group messages) |

Auth: `bot` only.

## Output fields

| Field | Type | Description |
|---|---|---|
| `type` | string | Always `card.action.trigger` |
| `event_id` | string | Unique event ID; safe for deduplication |
| `timestamp` | string (timestamp_ms) | Event delivery time (ms since epoch) |
| `operator_id` | string (open_id) | Open ID of the user who interacted |
| `message_id` | string (message_id) | Message ID of the card (`om_xxx`) |
| `chat_id` | string (chat_id) | Chat ID (`oc_xxx`) |
| `host` | string | `im_message` (chat card) or `im_top_notice` (top banner) |
| `token` | string | Delayed-update token; valid 30 min, max 2 uses |
| `action_tag` | string | Component type that was triggered (see decision table) |
| `action_value` | string | Developer-defined value on the component; serialized to JSON string |
| `action_name` | string | `name` attribute of the component |
| `timezone` | string | User timezone, e.g. `Asia/Shanghai`; only populated for date/time picker interactions |
| `form_value` | string (JSON) | All form field values as JSON string, keyed by component `name`; only present when a button inside a form container is clicked |
| `input_value` | string | Input text; only for standalone `input` components (not inside a form) |
| `option` | string | Selected value for standalone single-select: `select_static`, `select_person`, `overflow`, `date_picker`, `picker_time`, `picker_datetime` |
| `options` | string | Comma-separated selected values for standalone multi-select: `multi_select_static`, `multi_select_person` |
| `checked` | bool | Checkbox state for standalone `checker` elements |
| `card_content` | string | Original card content (userDSL text format) from when the card was sent; auto-fetched via message get API at consume time; empty if `message_id` absent or fetch fails — skip if empty |

## `card_content` — what it is and how to use it

`card_content` is the `user_dsl` field extracted from the card message content, auto-fetched
at event consume time. It represents the card's original definition — use it as the starting
point to understand the current card structure and construct the updated card JSON.

No extra API call is needed — the consumer fetches it automatically. If empty, skip — no fallback required.

## action_tag decision table

> **Form container rule**: when a component is inside a `form` container, its value appears in
> `form_value[name]` instead of the standalone fields (`option`, `options`, `input_value`,
> `checked`). There is no `form_submit` tag — form submission comes through as `button` with
> `form_value` populated.

| `action_tag` | Read field(s) | Notes |
|---|---|---|
| `button` | `action_value` (fromjson if object); `form_value` if inside a form | Most common; `form_value` non-empty = form submit |
| `overflow` | `option` | Collapsible button group selection |
| `select_static` | `option` (standalone) or `form_value[name]` (in form) | Single-select dropdown |
| `multi_select_static` | `options` (standalone) or `form_value[name]` (in form) | Multi-select dropdown |
| `select_person` | `option` — open_id of selected user | Single-select person |
| `multi_select_person` | `options` — comma-separated open_ids | Multi-select person |
| `input` | `input_value` (standalone) or `form_value[name]` (in form) | Text input |
| `checker` | `checked` (standalone) or `form_value[name]` (in form) | Checkbox |
| `date_picker` | `option` (date string) + `timezone` | e.g. `"2024-04-01 +0800"` |
| `picker_time` | `option` (time string) + `timezone` | e.g. `"08:30 +0800"` |
| `picker_datetime` | `option` (datetime string) + `timezone` | e.g. `"2024-04-29 07:07 +0800"` |
| `select_img` | `option` (single) or `options` (multi) | Image picker |

## Key constraints

1. Token **valid 30 minutes**, **max 2 uses** — if update fails after exhaustion, inform the user
2. Delayed-update API requires **complete new card JSON** — partial updates are not supported
3. SDK auto-responds `{"code":200}` within 3 s — your update call can be sent any time within 30 min
4. `card_content` is auto-populated — no extra API call needed; if empty, skip it

## After starting the listener

Once the listener is running, check whether your agent runtime supports background event
monitoring (i.e. can receive and process stdout lines from a running subprocess while
continuing to respond to the user). If it does, prompt the user:

> "Card callback listener is now active. Do you want me to automatically handle card
> interactions and update the card based on user actions?"

Only enter the auto-update workflow below if the user confirms. If your runtime does not
support background monitoring, inform the user that automatic card updates are not available
and they will need to handle interactions manually.

## Agent workflow

When a `card.action.trigger` event arrives (**each stdout JSON line is one event — process it immediately**):

```
1. Read action fields to understand what the user did:
   - action_tag: which component was triggered
   - action_value / option / options / checked / input_value / form_value: what value was set

2. Decide: does this interaction require a card update?
   - e.g. button click with a business action → yes
   - e.g. navigation / pagination → no (just record, no update needed)
   - Not every callback requires a card update — decide based on business semantics
   - Before updating, explicitly state what visual change the action requires. If you cannot articulate one, skip the update.

3. If update is needed:
   a. If card_content is empty: inform the user that the original card could not be fetched,
      so it is not possible to determine whether an update is needed — do not guess
   b. Determine the new card state based on the action
   c. Use card_content as the structural basis to construct the updated card JSON
   d. Detect card version: if card_content contains `"schema":"2.0"` or `"schema": "2.0"` it is Card 2.0; otherwise assume Card 1.0
   e. For Card 1.0: include `"open_ids": ["<operator_id>"]` inside the `card` object, or the API returns code 300090
   f. Call the delayed update API with the token and new card JSON

4. If no update: end (the SDK has already acknowledged the callback)
```

## Updating the card

```bash
lark-cli api POST /open-apis/interactive/v1/card/update --as bot \
  --data '{"token":"<token>","card":<new_card_json>}'
```

`--data` parameters:

| Field | Required | Description |
|---|---|---|
| `token` | Yes | Delayed-update token from the event |
| `card` | Yes | Complete new card JSON — construct based on `card_content` from the event, modified to reflect the new state |
| `card.open_ids` | No | **Card 1.0 only.** Array of `open_id`s defining which users see the updated card. Must contain at least one open_id (e.g. the operator's); passing `[]` or omitting the key both cause "openid empty" (code 300090). |

## Examples

```bash
# Stream all card interactions
lark-cli event consume card.action.trigger --as bot

# Grab one callback to inspect shape (debugging only — do not use in production workflows)
lark-cli event consume card.action.trigger --as bot --max-events 1 --timeout 60s

# Button clicks only (not form submit), with action value
lark-cli event consume card.action.trigger --as bot \
  --jq 'select(.action_tag == "button" and .form_value == "") | {op: .operator_id, val: (.action_value | fromjson?), token: .token}'

# Form submits (button with form_value present)
lark-cli event consume card.action.trigger --as bot \
  --jq 'select(.action_tag == "button" and .form_value != "") | {op: .operator_id, form: (.form_value | fromjson), token: .token}'

# Date picker interactions
lark-cli event consume card.action.trigger --as bot \
  --jq 'select(.action_tag == "date_picker") | {op: .operator_id, date: .option, tz: .timezone}'

# Filter to one chat
lark-cli event consume card.action.trigger --as bot \
  --jq 'select(.chat_id == "oc_xxx")'
```

## Gotchas

- **No `form_submit` tag**: form submission comes as `action_tag = "button"` with `form_value`
  populated. Check `form_value != ""` to distinguish from a standalone button click.
- **`action_value` type is developer-defined**: the original may be an object or a plain string.
  Use `fromjson?` (with `?` to swallow errors) or check before parsing.
- **Standalone vs form fields**: `input_value`, `option`, `options`, `checked` are only populated
  for components **not** inside a form container. Inside a form, all values appear in `form_value`.
- **WebSocket delivery**: no separate callback URL needed; uses the existing WS connection.
