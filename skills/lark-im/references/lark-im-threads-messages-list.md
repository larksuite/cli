# im +threads-messages-list

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +threads-messages-list`. **Run `lark-cli im +threads-messages-list --help` for the authoritative flags (`--thread` / `--order` / `--page-size` / `--page-token` / `--no-reactions` / `--download-resources` / `--as` / `--format`), the accepted `om_xxx`/`omt_xxx` input, and page-size range (1-500).** This file covers only what `--help` cannot.

Supports both `--as user` (default) and `--as bot`. Enriches replies with reactions / `update_time` per [message enrichment](lark-im-message-enrichment.md).

## Gotchas

- **Don't guess a `thread_id`.** It comes from the `thread_id` field of [`im +chat-messages-list`](lark-im-chat-messages-list.md) or [`im +messages-search`](lark-im-messages-search.md) output. Passing a plain `om_` message ID also works — the CLI resolves it to that message's thread — but an invented ID returns empty, not an error, so an empty result usually means a wrong/stale ID rather than "no replies".
- **No time filtering** — unlike `+chat-messages-list`, threads accept no `--start`/`--end` (Feishu API limitation). Scope the read with `--order` + pagination only: `--order desc --page-size 1` to just confirm replies exist, `--order desc --page-size 10` for recent, `--order asc --page-size 50` (then paginate) for the full thread in order.
- **`--order` defaults to `asc`** here (opposite of `+chat-messages-list`'s `desc`). (Note: the flag is `--order`, not `--sort`.)
- **Image content is a placeholder, not bytes**: replies render images as `[Image: img_xxx]`; files/audio/video stay as resource keys. Nothing downloads unless you pass `--download-resources` (writes to `./lark-im-resources/`) or use [`im +messages-resources-download`](lark-im-messages-resources-download.md).
- Permission denied here usually means the calling identity is **not a member of the parent chat** — thread access is gated by chat membership, not just OAuth scope.
