# im +feed-shortcut-list

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) for authentication, global parameters, and security rules.

This skill maps to shortcut: `lark-cli im +feed-shortcut-list`. Underlying API: `GET /open-apis/im/v2/feed_shortcuts`.

## What it does

Lists **one page** of the **current user's** feed shortcuts.

- Only **CHAT-type** shortcuts are exposed via OpenAPI today (others in the IDL are not yet whitelisted).
- The shortcut is a **thin one-page wrapper** — there is no built-in auto-pagination. Callers drive their own loop when they actually need to paginate.
- Server-side page size is controlled by the service; in normal use one page usually covers the list.
- Pagination tokens are opaque. If a token is rejected because the shortcut list changed, restart by omitting `--page-token`.

## Commands

```bash
# First page (the only call most users ever need — --page-token omitted)
lark-cli im +feed-shortcut-list --as user

# Continue from the previous response's page_token
lark-cli im +feed-shortcut-list --as user --page-token <token-from-previous-response>

# Skip detail enrichment when only IDs are needed; avoids the extra im:chat:read lookup
lark-cli im +feed-shortcut-list --as user --no-detail -q '.data.shortcuts[].feed_card_id'
```

> If you need to walk every page, write the loop yourself: read `data.page_token` from each response and pass it back in until `has_more=false`. The shortcut intentionally does not auto-walk because page-token errors require the caller to decide whether to restart from the first page.

## Parameters

| Parameter | Required | Description |
|------|------|------|
| `--page-token <token>` | no | Opaque pagination token from the previous response. **Omit it for the first page.** |
| `--no-detail` | no (default `false`) | Skip fetching each entry's full info object. By default enrichment is enabled: CHAT-type entries call `im.chats.batch_query`, need `im:chat:read`, and attach the object under the `detail` field. Pass `--no-detail` to skip the extra call and scope. |
| `--as user` | yes | Server only accepts user_access_token for this API |

## Response Structure

| Field | Type | Description |
|------|------|------|
| `shortcuts` | array | Feed shortcut entries; each has `feed_card_id` (oc_xxx) and `type` (1=CHAT). By default (without `--no-detail`), each entry also has a `detail` field with the full per-type info object. |
| `has_more` | boolean | Whether more pages exist |
| `page_token` | string | Opaque token to pass to the next call when continuing pagination |

Example (with detail enrichment, CHAT type):

```json
{
  "data": {
    "shortcuts": [
      {
        "feed_card_id": "oc_092f0100fe59c35995727db1039777a8",
        "type": 1,
        "detail": {
          "chat_id": "oc_092f0100fe59c35995727db1039777a8",
          "chat_mode": "group",
          "name": "Engineering",
          "avatar": "https://...",
          "description": "",
          "external": false,
          "owner_id": "ou_xxx",
          "owner_id_type": "open_id",
          "tenant_key": "..."
        }
      },
      {
        "feed_card_id": "oc_c82061d126a06635aa3569587b134bb1",
        "type": 1,
        "detail": {
          "chat_id": "oc_c82061d126a06635aa3569587b134bb1",
          "chat_mode": "p2p",
          "name": "",
          "p2p_target_id": "ou_xxx",
          "p2p_target_type": "user",
          "avatar": "",
          "description": "",
          "external": false,
          "tenant_key": "..."
        }
      }
    ],
    "has_more": false,
    "page_token": "v1.example-opaque-token"
  }
}
```

## Detail Enrichment

The `detail` payload is dispatched **per `type`**. Today only CHAT is wired in; future shortcut types can attach different object shapes. Callers should `switch` on `type` before parsing `detail`. For CHAT (`type=1`):

- **Source**: `POST /open-apis/im/v1/chats/batch_query` (50 ids per call, server limit).
- **Payload**: the **full chat object** is passed through verbatim — `chat_id`, `chat_mode` (`group` / `p2p` / `topic`), `name`, `avatar`, `description`, `external`, `tenant_key`, plus type-specific fields (`owner_id*` for groups, `p2p_target_*` for p2p).
- **P2P chats** return an empty `name` because the Feishu client renders the partner's display name there. The rest of the object (especially `p2p_target_id`) still flows through, so callers can resolve the partner via `+contact-search` if a display title is needed.
- **Lookup failure** (missing scope, network error) → the list still returns successfully; a warning is printed to stderr, the data payload carries a `_notice` field (`"detail enrichment skipped: ..."`), and affected entries simply lack the `detail` field. Check `_notice` to tell "enrichment skipped" from "nothing to enrich".

## Permissions

- Required scope: `im:feed.shortcut:read`
- Conditional scope (default detail path only): `im:chat:read`; pass `--no-detail` to avoid this extra scope and lookup.
- Only available with user identity (`--as user`).
