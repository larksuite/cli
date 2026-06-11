# ChangeRecordTrigger

记录满足条件时触发，**新增和修改都会触发**。"修改为 X 或新增 X 时执行动作"这类需求用本触发器 + `condition_list`，一条工作流即可表达，不要拆成 AddRecordTrigger 和 SetRecordTrigger 两条。

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
