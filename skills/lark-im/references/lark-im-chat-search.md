# im +chat-search

> **Prerequisite:** Before executing this command, ensure [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) has been read once in the current task for authentication, global parameters, and safety rules. Do not reread it if already loaded.

**Run `lark-cli im +chat-search --help` for the authoritative flags, defaults, filter enums, and completeness rule.** This file covers only what `--help` cannot.

Keyword matching covers **both chat names and member names**, including pinyin and prefix fuzzy search — so a person's name can surface the group they are in, not just groups named after them.

> **CAUTION:** `--sort` is **always descending** — the search API only ranks the chosen field high-to-low (e.g. `member_count` = most members first). There is no ascending option. If the user asks for "fewest first / ascending / 从少到多", tell them the search API does not support ascending order; any low-to-high view requires re-sorting the fetched page client-side and is not an upstream sort. Do **not** invent values like `member_count_asc` or pass `asc` (they are rejected).

## Search scope is not global

Only chats **visible to the current identity** can be found — joined chats plus public chats visible to it. This is not a search over all chats in the tenant, so an empty result does not mean the chat does not exist; it may simply be invisible to this identity.

**NEVER fall back to `+chat-list`.** When `+chat-search` returns nothing, do not try `+chat-list` or the raw chats list API — the list API has no keyword filter and will not locate the target. Instead ask the user to refine the keyword, or check whether the chat is visible to the identity in use.

## Filtering muted chats

`--exclude-muted` is applied **client-side after** the search call — the CLI batches the page's chat_ids through a mute-status lookup and drops the muted ones. Three consequences `--help` does not state:

- **Under `--as bot` the filter is silently skipped** (the mute API accepts user tokens only), so bot output comes back unfiltered even with the flag set.
- **Only confirmed-muted chats count toward `filtered_count`.** Non-member public groups are **retained** and merely mentioned in `hint`. For strict member-only results, combine with `--search-types "private,public_joined,external"`.
- **Filtering happens per page, so a page can come back short.** The JSON envelope gains a `filter` sub-object (absent when the flag is off), where `fetched_count == returned_count + filtered_count` always holds:

```json
{
  "chats": [...],
  "filter": {
    "applied": "exclude_muted",
    "fetched_count": 20,
    "returned_count": 19,
    "filtered_count": 1,
    "hint": "Filtered out 1 muted chat(s) on this page (19 remaining, including 2 non-member public group(s)); use --page-token to fetch more."
  }
}
```

Do not read a short page as "no more results" — follow the `hint` and keep paginating.

## Follow-up actions

After locating a chat, the usual next steps are listing its messages ([`+chat-messages-list`](lark-im-chat-messages-list.md)) or sending to it ([`+messages-send`](lark-im-messages-send.md)). Keep the same identity across the steps — a chat visible to the user is not necessarily visible to the bot.

## Common Errors and Troubleshooting

Only errors whose cause or fix is not evident from `--help`:

| Symptom | Root cause | Solution |
|---------|---------|---------|
| Permission denied (99991672) | Bot app lacks the `im:chat:read` tenant-token permission | Enable it for the app in the Open Platform console |
| Permission denied (99991679) with `--as user` | User token not authorized for `im:chat:read` | `lark-cli auth login --scope "im:chat:read"` |
| `Bot ability is not activated` (232025) | App has no bot capability | Enable bot capability in the Open Platform console |
| Empty results | Chat not visible to this identity, or keyword too narrow | Refine the keyword or switch identity — **do not fall back to `+chat-list`** |

## References

- [lark-im](../SKILL.md) — all IM commands
