# SetRecordTrigger

```json
{
  "table_name": "订单表",
  "record_watch_conjunction": "and",
  "record_watch_info": [ /* FieldCondition[] */ ],
  "field_watch_info": [
    { "field_name": "状态", "operator": "is", "value": [{ "value_type": "text", "value": "已发货" }] }
  ],
  "trigger_control_list": [],
  "condition_list": null
}
```

| 字段 | 必填 | 说明 |
|------|----|------|
| `table_name` | 是  | 监控的数据表名 |
| `record_watch_conjunction` | 否  | 记录筛选组合方式：`and` / `or`，默认 `and` |
| `record_watch_info` | 否  | 记录级过滤条件（修改前值匹配），为空则监听全部 |
| `field_watch_info` | 是  | 字段级监控条件列表，至少一个 |
| `trigger_control_list` | 否  | 触发控制，可选值：`pasteUpdate` / `automationBatchUpdate` / `syncUpdate` / `appendImport` |
| `condition_list` | 否  | 过滤条件数组，数组中每个元素为 AndCondition 结构，多个 AndCondition 之间为 OR 关系 |

`FieldWatchItem`：

| 字段 | 类型 | 说明 |
|------|------|------|
| `field_name` | string | 监听字段名称 |
| `operator` | string | 操作符（仅明确要求字段满足条件时填） |
| `value` | ValueInfo[] | 触发值 |

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
