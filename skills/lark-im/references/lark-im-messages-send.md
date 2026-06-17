# im +messages-send

> **Prerequisite:** Read [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) first for authentication, global parameters, and safety rules.

Maps to `lark-cli im +messages-send`. **Run `lark-cli im +messages-send --help` for the authoritative flag list, content/media flags, mutual-exclusion rules, msg-type inference, `--video`/`--video-cover` pairing, and the cwd-relative media path rules.** This file only covers what `--help` cannot.

Safety: sent messages are visible to others — confirm recipient, content, and identity before sending (see lark-shared risk policy). `--as bot` requires the app to already be in the target chat.

## Picking the content flag (when to use which)

- `--markdown` — headings / lists / links / summaries / reports. **Converted to a Feishu `post` payload (forces `msg_type=post`), single `zh_cn` locale.**
- `--text` — exact plain text: logs, code, indentation, or literal Markdown that must **not** render.
- `--content` — exact JSON when you need a post title, multiple locales, cards, or unsupported structures.

## `--markdown` boundaries (non-obvious)

- Not full CommonMark/GFM. Always a single-`zh_cn` `post`; **cannot set a post title** — use `--content` for a title.
- Headings are rewritten: `# Title` → `#### Title`; `##`–`######` → `#####` when the content has H1–H3.
- **Local image paths in `![x](./a.png)` are NOT auto-uploaded** and will not render. Pre-upload first to get an `img_xxx` key:
  ```bash
  lark-cli im images create --data '{"image_type":"message"}' --file ./diagram.png   # → {"image_key":"img_v3_xxxx"}
  lark-cli im +messages-send --chat-id oc_xxx --markdown $'## Report\n\n![d](img_v3_xxxx)'
  ```
- Remote `https://` images are auto-downloaded + uploaded at runtime; on download/upload failure the image is silently dropped with a warning.

## Preserve exact formatting → `--text` with `$'...'`

For multi-line text, indentation, code blocks, or literal backslashes, use shell ANSI-C quoting so `\n` is honored literally instead of relying on the shell:

```bash
lark-cli im +messages-send --chat-id oc_xxx --text $'Build failed\nBranch: feature/x\nAction: check logs'
```

## @Mention format (differs by message type)

The shortcut only normalizes mentions for `text` and `post`; **`interactive` card content is passed through verbatim**, so cards must use the card-native syntax — this asymmetry is the trap.

- **`text`**: `<at user_id="ou_xxx">name</at>` (inner name optional); @all: `<at user_id="all"></at>`. The shortcut also normalizes `<at id=...>` / `<at open_id=...>` into `user_id`, but author examples with `user_id`.
- **`post`**: same inline form inside a `text`/`md` element, or a dedicated node `{"tag":"at","user_id":"ou_xxx"}` (`"all"` for everyone).
- **`interactive` (card)**: NOT normalized — use card-native `<at>` inside a `lark_md`/`markdown` element: single `<at id=ou_xxx></at>`, multiple `<at ids=ou_xxx1,ou_xxx2></at>`, by email `<at email=user@example.com></at>`.

## Common mistakes (the non-obvious ones)

- `--text` for headings/lists/reports → use `--markdown`. `--markdown` when exact line breaks / logs / literal Markdown matter → use `--text` with `$'...'`.
- Local image paths inside `--markdown` (`![x](./a.png)`) — pre-upload via `images.create` instead.

## HELP-GAP — not yet in `--help`/schema; keep here until CLI adds it

> These are USAGE that belongs in schema but isn't there yet. Once CLI adds them, delete this section.

- **`--content` JSON shape per `msg_type`**: `text` `{"text":"..."}` · `post` `{"zh_cn":{"title":"...","content":[[{"tag":"text","text":"..."}]]}}` · `image` `{"image_key":"img_xxx"}` · `file`/`audio` `{"file_key":"file_xxx"}` · `media` `{"file_key":"...","image_key":"<cover, required>"}` · `share_chat` `{"chat_id":"oc_xxx"}` · `share_user` `{"user_id":"ou_xxx"}` · `interactive` = card JSON.
- **Return value**: `{"message_id":"om_xxx","chat_id":"oc_xxx","create_time":"..."}`.
- **`--idempotency-key`**: the same key sends only one message within 1 hour.
