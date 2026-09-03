# note +detail

通过 `note_id` 查询会议纪要详情，获取下挂文档 Token（AI 智能纪要、逐字稿、会中共享文档）。只读，支持 `--as user` 或 `--as bot`。

## 命令

```bash
lark-cli note +detail --note-id <note_id>
lark-cli note +detail --note-id <note_id> --as bot
```

`note_id` 由其他命令取得时，必须显式沿用来源身份。应用身份能否读到数据取决于应用对纪要主文档的查看权限。若 `--as bot` 返回 `note_display_type=unified`，不要静默切换到用户身份执行 `note +transcript`；先向用户说明该命令仅支持用户身份。

## 输出

返回下挂文档 token（`note_doc_token`、`verbatim_doc_token`、`shared_doc_tokens`）、`note_display_type` 和 `meeting_id`（仅在该纪要由会议生成时返回）。

## 反查关联会议

`meeting_id` 提供从纪要反向定位会议的入口，传给 `vc +detail --meeting-ids` 可继续查会议详情、参会人或反查日程（会议详情可含 `calendar_event_id`，用于再反查日程）。

字段为空说明该纪要不是由会议生成（如手动创建的纪要），不要据此反查，也不视为错误。

## 相关场景
- [基于 note_id 查询纪要、逐字稿、共享文档等](../scenes/query-note-and-artifacts.md)
