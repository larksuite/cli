# note +detail

通过 `note_id` 查询会议纪要详情，获取下挂文档 Token（AI 智能纪要、逐字稿、会中共享文档）。只读，支持 `--as user` 或 `--as bot`。

## 命令

```bash
lark-cli note +detail --note-id <note_id>
lark-cli note +detail --note-id <note_id> --as bot
```

`note_id` 由其他命令取得时，必须显式沿用来源身份。应用身份能否读到数据取决于应用对纪要主文档的查看权限。若 `--as bot` 返回 `note_display_type=unified`，不要静默切换到用户身份执行 `note +transcript`；先向用户说明该命令仅支持用户身份。

## 相关场景
- [基于 note_id 查询纪要、逐字稿、共享文档等](../scenes/query-note-and-artifacts.md)
