# ReminderTrigger

```json
{
  "table_name": "项目表",
  "field_name": "截止日期",
  "offset": 1,
  "unit": "DAY",
  "hour": 9,
  "minute": 0,
  "condition_list": null
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `table_name` | 是 | 数据表名 |
| `field_name` | 是 | 日期字段名（必须为 `datetime` / `created_at` / `formula` / `lookup` 类型） |
| `unit` | 是 | 偏移单位：`MINUTE` / `HOUR` / `DAY` / `WEEK` / `MONTH` |
| `offset` | 是 | 提前/延后的偏移量（正数=提前，负数=延后；范围由 `unit` 决定）：`MINUTE` ∈ {0, 5, 15, 30, -5, -15, -30}；`HOUR` ∈ [-6, -1] ∪ [1, 6]；`DAY` ∈ [-7, 7]；`WEEK` ∈ [-7, -1] ∪ [1, 7]；`MONTH` ∈ [-7, -1] ∪ [1, 7] |
| `hour` | 是 | 触发小时 (0-23)，默认 9 |
| `minute` | 是 | 触发分钟 (0-59)，默认 0 |
| `condition_list` | 否 | 过滤条件数组，数组中每个元素为 AndCondition 结构，多个 AndCondition 之间为 OR 关系  |

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
