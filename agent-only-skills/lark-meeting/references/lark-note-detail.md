# note +detail

通过 `note_id` 查询会议纪要详情，获取下挂文档 Token（AI 智能纪要、逐字稿、会中共享文档）。只读，支持 `--as user`。

## 命令

```bash
lark-cli note +detail --note-id <note_id>
```

`note_id` 由其他命令取得时，必须显式沿用来源身份 `--as user`。返回 `note_display_type=unified` 时，使用相同身份执行 `note +transcript`。

## 相关场景
- [基于 note_id 查询纪要、逐字稿、共享文档等](../scenes/query-note-and-artifacts.md)
