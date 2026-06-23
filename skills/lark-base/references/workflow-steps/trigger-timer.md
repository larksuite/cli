# TimerTrigger

```json
{
  "rule": "WEEKLY",
  "start_time": "2025-01-01 09:00",
  "sub_unit": [1, 3, 5],
  "is_never_end": true
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `rule` | 是 | `NO_REPEAT` / `DAILY` / `WEEKLY` / `MONTHLY` / `YEARLY` / `WORKDAY` / `CUSTOM` |
| `start_time` | 否 | 开始时间，格式 `yyyy-MM-dd HH:mm` |
| `interval` | 否 | 自定义间隔 [1,30]（仅 CUSTOM） |
| `unit` | 否 | 自定义单位：`SECOND` / `MINUTE` / `HOUR` / `DAY` / `WEEK` / `MONTH` / `YEAR` |
| `sub_unit` | 否 | 子单位（`WEEKLY` 时为星期几数组 0-6，`MONTHLY` 时为几号数组 1-31） |
| `end_time` | 否 | 结束时间 |
| `is_never_end` | 否 | 是否永不结束 |

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
