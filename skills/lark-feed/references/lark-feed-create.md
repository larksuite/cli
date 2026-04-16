# feed +create

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first to understand authentication, global parameters, and safety rules.

Send an app feed card to one or more Lark users. The card appears in each recipient's message feed (消息流) with a title and a clickthrough link.

This skill maps to the shortcut: `lark-cli feed +create` (internally calls `POST /open-apis/im/v2/app_feed_card`).

**Requires:** Lark client v7.6 or later on the recipient's device.

## Safety Constraints

App feed cards are delivered directly to users' message feeds. Before calling this command, confirm with the user:

1. Who should receive the card (user open_ids)
2. The title and link content
3. Whether to enable time-sensitive (temporary top pin)

**Do not** send cards without explicit user approval.

## Usage

```bash
lark-cli feed +create \
  --user-ids ou_<open_id> \
  --title "<card title>" \
  --link "https://..." \
  [--preview "<preview text>"] \
  [--time-sensitive] \
  --as bot
```

## Flags

| Flag | Required | Description |
|------|----------|-------------|
| `--user-ids` | Yes | User open_id(s) to receive the card (`ou_xxx`). Repeatable: `--user-ids ou_aaa --user-ids ou_bbb`. Max 20 users. |
| `--link` | Yes | Card clickthrough URL (HTTPS only, max 700 chars). |
| `--title` | Yes | Card title shown in the message feed (max 60 chars). |
| `--preview` | No | Preview text shown under the title (max 120 chars). |
| `--time-sensitive` | No | Temporarily pin the card at top of recipients' message feed. |

## Examples

Send a basic card to one user:
```bash
lark-cli feed +create \
  --user-ids ou_abc123 \
  --title "Weekly Report Ready" \
  --link "https://example.com/report" \
  --as bot
```

Send to multiple users with preview and top-pin:
```bash
lark-cli feed +create \
  --user-ids ou_aaa \
  --user-ids ou_bbb \
  --title "Urgent Notice" \
  --preview "Please check immediately" \
  --link "https://example.com/notice" \
  --time-sensitive \
  --as bot
```

## Output

```json
{
  "biz_id": "b90ce43a-fca8-4f42-...",
  "failed_cards": []
}
```

- `biz_id`: system-assigned business ID for this card
- `failed_cards`: list of users for whom delivery failed (each item has `user_id` and `reason`)

## DryRun

Use `--dry-run` to preview the API call without sending:
```bash
lark-cli feed +create \
  --user-ids ou_abc123 \
  --title "Test" \
  --link "https://www.feishu.cn/" \
  --dry-run \
  --as bot
```
