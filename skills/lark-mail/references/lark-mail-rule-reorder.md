# Mail Rule Reorder

Use `lark-cli mail +rule-reorder` when the user wants to move one or more inbox rules to the front but only provides a partial rule order.

The raw `mail user_mailbox.rules reorder` API requires a complete `rule_ids` array. This shortcut first lists the current rule order, places the requested IDs first, appends all remaining rule IDs in the current list order, then submits the complete reorder request.

## Command

```bash
lark-cli mail +rule-reorder \
  --as user \
  --mailbox me \
  --rule-id rule_3 \
  --rule-id rule_1
```

Equivalent single-flag forms:

```bash
lark-cli mail +rule-reorder --as user --rule-ids '["rule_3","rule_1"]'
lark-cli mail +rule-reorder --as user --rule-ids 'rule_3,rule_1'
```

## Behavior

- `--mailbox` defaults to `me`.
- `--rule-id` can be repeated and preserves the exact order provided.
- `--rule-ids` accepts either a JSON string array or a comma-separated list.
- Duplicate requested IDs are rejected before any API call.
- Requested IDs that are not present in the current rule list are rejected after the list call.
- Empty current rule lists are rejected.
- The shortcut requires user identity and both rule read/write scopes.

## Dry Run

```bash
lark-cli mail +rule-reorder --as user --rule-ids 'rule_3,rule_1' --dry-run
```

Dry-run output shows both calls:

1. `GET /open-apis/mail/v1/user_mailboxes/:user_mailbox_id/rules`
2. `POST /open-apis/mail/v1/user_mailboxes/:user_mailbox_id/rules/reorder`

The POST body uses `<auto-completed-after-list>` because the final order is only known after the list response.

## Output

Run this before parsing output fields:

```bash
lark-cli mail +rule-reorder --print-output-schema
```

Successful output includes:

| Field | Meaning |
|---|---|
| `mailbox` | Mailbox ID used in the operation |
| `requested_rule_ids` | IDs explicitly supplied by the user |
| `appended_rule_ids` | Remaining rule IDs appended by the CLI |
| `final_rule_ids` | Complete order submitted to reorder |
| `total` | Number of IDs in `final_rule_ids` |

## When To Use Raw API Instead

Use `lark-cli mail user_mailbox.rules reorder` only when the caller already has the full rule ID order and wants exact raw API passthrough.
