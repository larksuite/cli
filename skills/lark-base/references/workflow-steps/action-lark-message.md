# LarkMessageAction

```json
{
  "receiver": [{ "value_type": "user", "value": {"id": "ou_xxxx"} }],
  "send_to_everyone": false,
  "title": [{ "value_type": "text", "value": "新订单通知" }],
  "content": [
    { "value_type": "text", "value": "客户 " },
    { "value_type": "ref", "value": "$.trigger_1.fldCustomerName" },
    { "value_type": "text", "value": " 创建了新订单" }
  ],
  "btn_list": [
    { "text": "查看详情", "btn_action": "openLink", "link": [{ "value_type": "text", "value": "https://example.com" }] }
  ]
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `receiver` | 是 | ValueInfo[] |
| `send_to_everyone` | 是 | 是否发送给所有人 |
| `title` | 否 | TextRefItem[] 消息标题 |
| `content` | 是 | TextRefItem[] 消息内容 |
| `btn_list` | 是 | 按钮列表，不需要时为空数组 |

`ButtonConfig`：

| 字段 | 类型 | 说明 |
|------|------|------|
| `text` | string | 按钮文字 |
| `btn_action` | string | `addRecord` / `setRecord` / `openLink` |
| `link` | ValueInfo[] | 跳转链接（`openLink` 时使用） |
| `table_name` | string | 操作表名（`addRecord` 时使用） |
| `record_values` | RecordFieldValue[] | 记录赋值（`addRecord` / `setRecord` 时使用） |

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
