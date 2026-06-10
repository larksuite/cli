# AddRecordAction

```json
{
  "table_name": "订单表",
  "field_values": [
    { "field_name": "客户名称", "value": [{ "value_type": "text", "value": "张三" }] },
    { "field_name": "金额", "value": [{ "value_type": "number", "value": 100 }] },
    { "field_name": "创建人", "value": [{ "value_type": "ref", "value": "$.trigger_1.fieldIdxxx" }] }
  ]
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `table_name` | 是 | 目标数据表名 |
| `field_values` | 是 | RecordFieldValue[] |

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
