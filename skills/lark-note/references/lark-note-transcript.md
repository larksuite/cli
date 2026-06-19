# note +transcript

只在 `note +detail` 或 `vc +notes` 已确认 `note_display_type=unified` 时使用。普通纪要逐字稿是独立 Docx 文档，应回到 [lark-doc](../../lark-doc/SKILL.md) 读取 `verbatim_doc_token`。

```bash
lark-cli note +transcript --note-id NOTE_ID
```

## 行为契约

- CLI 会先校验该 Note 是否为 `unified`；不是 unified 时不拉取 transcript。
- CLI 内部自动翻页并拼接完整内容；任一页失败时整体报错，不保存半截 transcript。
- 默认保存到 `./notes/{note_id}/unified_transcript.md`；`--transcript-format plain_text` 时保存为 `.txt`。
- 目标文件已存在时会失败；用户明确要覆盖时才加 `--overwrite`。

## 何时不要用

| 场景 | 正确路由 |
|------|---------|
| 只有纪要文档标题 | 先文档搜索，再 `docs +fetch --api-version v2`；有 `vc-node-id` 才回 Note 域 |
| 只有 Docx URL / `doc_token` | 先 `docs +fetch --api-version v2`；不要从 `doc_token` 反推 `note_id` |
| `note_display_type=normal` | `docs +fetch --api-version v2 --doc <verbatim_doc_token>` |
| `note_display_type=unknown` 且 `verbatim_doc_token` 非空 | 先按独立逐字稿文档读取 |
