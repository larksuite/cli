# SetRecordAction

```json
{
  "table_name": "订单表",
  "max_set_record_num": 10,
  "field_values": [
    { "field_name": "状态", "value": [{ "value_type": "option", "value": { "id": "opt1", "name": "已完成" } }] }
  ],
  "filter_info": { /* RecordFilterInfo */ },
  "ref_info": { "step_id": "step_trigger" }
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `table_name` | 是 | 目标数据表名 |
| `max_set_record_num` | 否 | 最大更新记录数，默认 100，范围 1-15000 |
| `field_values` | 是 | RecordFieldValue[] |
| `filter_info` | 否* | RecordFilterInfo 过滤条件（与 `ref_info` 互斥） |
| `ref_info` | 否* | RefInfo 引用前置步骤的记录（与 `filter_info` 互斥） |

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
