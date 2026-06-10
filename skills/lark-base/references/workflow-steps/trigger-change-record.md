# ChangeRecordTrigger

```json
{
  "table_name": "任务表",
  "trigger_control_list": [],
  "condition": null
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `table_name` | 是 | 监控的数据表名 |
| `trigger_control_list` | 否 | 触发控制，可选值：`pasteUpdate` / `automationBatchUpdate` / `syncUpdate` / `appendImport` |
| `condition_list` | 否 | 过滤条件数组，数组中每个元素为 AndCondition 结构，多个 AndCondition 之间为 OR 关系 |

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
