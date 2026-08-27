# docs
> skill: lark-doc

## +create
Create a new Lark document from DocxXML or Markdown, optionally in a folder or Wiki node.

### Tips
- Match `--doc-format` to `--content`: XML is the default for rich DocxXML; use `--doc-format markdown` for Markdown input.
- Before authoring `--content`, read the matching XML or Markdown guide under Related skills when available, unless already read. For XML, use only documented DocxXML tags.
- For multiline `--content`, prefer `@file` or `-` (stdin) to avoid shell-escaping damage.

### Skills
- lark-doc/references/lark-doc-create-workflow.md
- lark-doc/references/lark-doc-create.md
- lark-doc/references/lark-doc-xml.md
- lark-doc/references/lark-doc-md.md

## +fetch
Read an entire Lark document, or limit the result to an outline, section, block range, or keyword match.

### Skills
- lark-doc/references/lark-doc-fetch.md

## +update
Apply targeted text or block edits, append content, or deliberately replace an entire Lark document.

### Tips
- Prefer `str_replace` or `block_*` commands for targeted edits. Use `overwrite` only when replacing the entire document is intended; it can discard unrelated rich content.
- Before a `block_*` edit, fetch the target with `lark-cli docs +fetch --detail with-ids` and a narrow `--scope`; refetch after structural changes before reusing block IDs.
- Before authoring `--content`, read the matching XML or Markdown guide under Related skills when available, unless already read. For XML, use only documented DocxXML tags.
- Match `--doc-format` to `--content`; for multiline content, prefer `@file` or `-` (stdin).

### Skills
- lark-doc/references/lark-doc-update.md
- lark-doc/references/lark-doc-xml.md
- lark-doc/references/lark-doc-md.md

## +history-list

### Skills
- lark-doc/references/lark-doc-history.md

## +history-revert

### Prerequisites
- `history_version_id` from [[+history-list]]

### Skills
- lark-doc/references/lark-doc-history.md

## +history-revert-status

### Prerequisites
- `task_id` from [[+history-revert]]

### Skills
- lark-doc/references/lark-doc-history.md
