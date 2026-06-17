# im +messages-resources-download

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +messages-resources-download`. **Run `lark-cli im +messages-resources-download --help` for the authoritative flags (`--message-id` / `--file-key` / `--type` / `--output` / `--as`), the `image`/`file` type enum, and the `--output` behavior (relative-only / no `..`, Content-Disposition filename fallback, extension inference).** This file covers only what `--help` cannot.

## Gotchas

- **A resource is identified by `message_id` + `file_key` together** — the `file_key` must come from *that* message's own content (returned by `im +chat-messages-list`). A `file_key` from a different message fails to download even if both look valid. Read-only message commands render the key in content but never fetch the bytes; this command is the only way to get them.
- **`--type` must match the marker, not the original media kind**: `img_xxx` → `--type image`; `file_xxx` → `--type file`. Audio and video both surface as `file_xxx`, so they download with `--type file` (not `audio`/`video`).
- **`234002` / `14005` are permission/state errors, NOT missing scope** — no access to the chat, or the file was deleted. Do not retry and do not re-run `auth login`; return the error to the user. (`im:message:readonly` not authorized is a *different* failure → `auth login --scope "im:message:readonly"`.)
- **Large files auto-chunk via HTTP Range** (128 KB size-probe, then sequential 8 MB chunks, up to 2 retries w/ backoff, post-download size-integrity check). A "file size mismatch" or "Content-Range" error is transient network instability — just retry the command.

## HELP-GAP — not yet in `--help`/schema; keep until CLI adds it

- The chunked-download internals above (probe size, 8 MB chunk, sequential single-worker, 2 retries, integrity check) are runtime behavior not surfaced by `--help`.
