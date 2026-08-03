# docs
> skill: lark-doc

## +create
Create a document from XML or Markdown content.

### Tips
- Match `--doc-format` to the payload: XML is the default for rich DocxXML; set `--doc-format markdown` for Markdown.
- Use only documented DocxXML tags; do not invent XML elements. The related XML and Markdown guides provide full syntax when available.
- For multiline content, prefer `--content @file` or `--content -` (stdin) to avoid shell-escaping damage.

### Skills
- lark-doc/references/lark-doc-create.md
- lark-doc/references/lark-doc-xml.md
- lark-doc/references/lark-doc-md.md

## +fetch
Fetch a document or a focused portion of its content.

### Skills
- lark-doc/references/lark-doc-fetch.md

## +update
Update document content with a supported document command.

### Tips
- Prefer `str_replace` or `block_*` commands for targeted edits; `overwrite` rebuilds the whole document and can discard unrelated rich content.
- Before a `block_*` edit, fetch the target with `lark-cli docs +fetch --detail with-ids` and a narrow `--scope`; refetch after structural changes before reusing block IDs.
- Match `--doc-format` to `--content`, and prefer `@file` or stdin for multiline payloads.

### Skills
- lark-doc/references/lark-doc-update.md
- lark-doc/references/lark-doc-xml.md
- lark-doc/references/lark-doc-md.md

## +history-list
List document history versions.

### Skills
- lark-doc/references/lark-doc-history.md

## +history-revert
Revert a document to a history version.

### Skills
- lark-doc/references/lark-doc-history.md

## +history-revert-status
Check the status of a document history revert.

### Skills
- lark-doc/references/lark-doc-history.md
