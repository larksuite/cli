# im +chat-list

> **Prerequisite:** Before executing this command, ensure [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) has been read once in the current task for authentication, global parameters, and safety rules. Do not reread it if already loaded.

**Run `lark-cli im +chat-list --help` for the authoritative flags, defaults, sort fields, and completeness rule.** This file covers only what `--help` cannot.

**Not a search API** — there is no `--query`; the call always returns the full membership list, paginated. For keyword lookup (find a group by name or by member) use [`+chat-search`](lark-im-chat-search.md) instead.

## Including p2p single chats

`--help` says p2p requires `--types` and user identity. What it cannot tell you is which phrasing maps to which call:

| User intent | Call | Identity |
|---|---|---|
| "list my groups" / 我的群 / 我加入了哪些群 | default, omit `--types` | user or bot |
| "list my p2p chats" / 我的单聊 / 我跟谁有 1v1 | `--types p2p` | **user only** |
| "all my chats" / 全部聊天 / 所有会话 (ambiguous) | `--types p2p,group` | **user only** |

For p2p rows: `name` is the peer's display name, `chat_mode = "p2p"`, `owner_id` follows group semantics, and `p2p_target_type` / `p2p_target_id` identify the peer.

## Bot identity and p2p

`tenant_access_token` cannot enumerate p2p chats — a privacy restriction, not a scope gap, so no amount of authorization changes it. The two failure shapes differ, and the second one is the dangerous one:

- **`--as bot --types=p2p`** → rejected at validation time; **no request is sent**.
- **`--as bot --types=p2p,group`** → the CLI **silently strips** `p2p` and sends `types=group`. The request succeeds and returns only groups, so a caller who does not inspect the warning will believe p2p was covered. It is surfaced two ways:
  - stderr: `warning: bot_strip_p2p: To protect user privacy, bot identity cannot list p2p chats; --types=p2p,group was sent as types=group. Use --as user to include p2p.`
  - stdout JSON: a top-level `notices` array gains `{ "code": "bot_strip_p2p", "message": "…" }`
- `--dry-run` emits the same warning, so a preview truthfully reflects what execution will send.

To actually include p2p, switch identity: `--as user --types=p2p,group`.

## Filtering muted chats

`--exclude-muted` is applied **client-side after** the list call — the CLI batches the page's chat_ids through a mute-status lookup and drops the muted ones. Two consequences `--help` does not state:

- **Under `--as bot` the filter is silently skipped** (the mute API accepts user tokens only), so bot output comes back unfiltered even with the flag set.
- **Filtering happens per page, so a page can come back short.** The JSON envelope gains a `filter` sub-object (absent when the flag is off), where `fetched_count == returned_count + filtered_count` always holds:

```json
{
  "chats": [...],
  "filter": {
    "applied": "exclude_muted",
    "fetched_count": 20,
    "returned_count": 17,
    "filtered_count": 3,
    "hint": "Filtered out 3 muted chat(s) on this page (17 remaining); use --page-token to fetch more."
  }
}
```

Do not read a short page as "no more results" — follow the `hint` and keep paginating.

## Gotchas

- **`--sort` has no direction flag.** `create_time` is always ascending and `active_time` always descending; the command has no `--order`. "Newest-created first" is therefore **not expressible** — use `active_time` (most recently active first), or tell the user the constraint. Fabricating `--order desc` fails with unknown flag.
- **`notices` and `filter` are separate top-level keys.** Both can appear in one response; neither overrides the other, so check for both.

## Common Errors and Troubleshooting

Only errors whose cause or fix is not evident from `--help`:

| Symptom | Root cause | Solution |
|---------|---------|---------|
| p2p chats missing under bot identity | Privacy restriction stripped `p2p` from the request | Re-run with `--as user`; check `notices` for `bot_strip_p2p` |
| Fewer rows than `--page-size` | `--exclude-muted` filtered the page after fetching | Read `filter.filtered_count`, follow the `hint`, keep paginating |
| Permission denied | Chat read scope missing | `auth login --scope "im:chat:read"` |

## References

- [lark-im](../SKILL.md) — all IM commands
