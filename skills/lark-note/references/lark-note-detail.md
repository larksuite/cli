# note +detail

通过 `note_id` 查询会议纪要详情，获取下挂文档 Token（AI 智能纪要、逐字稿、会中共享文档）。只读，仅支持 `--as user`。

## 命令

```bash
lark-cli note +detail --note-id <note_id>
```

## `note_id` 来源

- 可以来自用户直接给出的 `note_id`。
- 如果入口是文档，先由 [lark-doc](../../lark-doc/SKILL.md) 读取 Docx；只有 `<vc-transcribe-tab vc-node-id="...">` 的 `vc-node-id` 可以作为 `note_id`。
- 没有 `vc-node-id` 时，不要从 `doc_token`、标题、正文或 backlink 反推 `note_id`。

## 输出后的路由

| detail 字段 | 后续动作 |
|---------|---------|
| `note_doc_token` | 读纪要正文 / 总结 / 待办 / 章节：`docs +fetch --doc <note_doc_token>` |
| `note_display_type=normal` + `verbatim_doc_token` | 读逐字稿：`docs +fetch --doc <verbatim_doc_token>` |
| `note_display_type=unknown` + `verbatim_doc_token` | 先按普通独立逐字稿文档读取；不要猜成 unified |
| `note_display_type=unified` | 读逐字稿 / 原始记录：转 [`note +transcript`](lark-note-transcript.md) |

判别键是 `note_display_type`。即使 unified 纪要返回了非空 `verbatim_doc_token`，逐字稿仍按 unified 路由。
