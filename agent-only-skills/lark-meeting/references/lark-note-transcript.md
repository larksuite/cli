# note +transcript

只在 `note +detail` 已确认 `note_display_type=unified` 时使用。普通纪要逐字稿是独立 Docx 文档，应回到 [lark-doc](../../lark-doc/SKILL.md) 读取 `verbatim_doc_token`。

`note +transcript` 使用 AAT 的 `--as user` 通道，沿用 `note +detail` 的来源身份读取 unified 逐字稿。

```bash
lark-cli note +transcript --note-id <note_id>
```

## 行为契约

- CLI 会先校验该 Note 是否为 `unified`；不是 unified 时不拉取 transcript。
- CLI 内部自动翻页并拼接完整内容；任一页失败时整体报错，不保存半截 transcript。
- 默认保存到 `./notes/{note_id}/unified_transcript.md`；`--transcript-format plain_text` 时保存为 `.txt`。
- 目标文件已存在时会失败；用户明确要覆盖时才加 `--overwrite`。

## 相关场景
- [基于 note_id 查询纪要、逐字稿、共享文档等](../scenes/query-note-and-artifacts.md)
