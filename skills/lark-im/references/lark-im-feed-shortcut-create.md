# im +feed-shortcut-create

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +feed-shortcut-create`. **Run `lark-cli im +feed-shortcut-create --help` for the authoritative flags (`--chat-id` / `--head` / `--tail` / `--as` / `--dry-run` / `--format` / `-q`), the 10-ID batch limit, and `--head`/`--tail` mutual exclusion.** This file covers only what `--help` cannot.

## Gotchas

- **Only CHAT-type shortcuts are supported** (`--chat-id` must be an `oc_xxx` open_chat_id). If you only know a group name, resolve its `oc_xxx` first with `im +chat-search`.
- **Re-adding an existing shortcut is idempotent** — the server treats it as a no-op rather than an error; `ok:true`, `failure_count=0`.
- **Partial failure exits non-zero**: any non-empty `failed_shortcuts` sets `ok:false` on stdout and exits `1`. The full batch ledger (`total`, `success_count`, `failure_count`, `succeeded_shortcuts`, `failed_shortcuts`) remains machine-readable on stdout even on partial failure — check exit code AND `ok` AND `failure_count` in scripts.
- **`failed_shortcuts[].reason_label`** is a CLI-added human-readable label alongside the numeric `reason`. The server only returns the number; the CLI enriches it. Reason codes: `1=no_permission`, `2=invalid_item`, `3=has_pending_delete`, `4=type_not_support`, `5=internal_error`.
- **User identity only** (`--as user`). The CLI rejects `--as bot` locally; the server also rejects it.

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- **Required scope**: `im:feed.shortcut:write`.
- **Response ledger fields**: `total` · `success_count` · `failure_count` · `succeeded_shortcuts[]{feed_card_id, type}` · `failed_shortcuts[]{shortcut{feed_card_id, type}, reason, reason_label}`.
