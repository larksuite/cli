# im +feed-shortcut-remove

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +feed-shortcut-remove`. **Run `lark-cli im +feed-shortcut-remove --help` for the authoritative flags (`--chat-id` / `--as` / `--dry-run` / `--format` / `-q`) and the 10-ID batch limit.** This file covers only what `--help` cannot.

## Gotchas

- **Removing a chat that is not in the shortcut list is idempotent** — the server returns `ok:true`, `failure_count=0`, no `failed_shortcuts` entry. This means you cannot distinguish "removed" from "was not present."
- **Partial failure exits non-zero**: any non-empty `failed_shortcuts` sets `ok:false` on stdout and exits `1`. The full batch ledger remains on stdout — check exit code AND `ok` in scripts. See [`+feed-shortcut-create`](lark-im-feed-shortcut-create.md) for the shared ledger field definitions and `reason_label` codes.
- **User identity only** (`--as user`). The server does not accept bot identity.
- **To inspect the list before removing**, run `+feed-shortcut-list --no-detail` (avoids the extra `im:chat:read` call when only `feed_card_id` values are needed).

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- **Required scope**: `im:feed.shortcut:write`.
