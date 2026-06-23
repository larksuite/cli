# ButtonTrigger

```json
{
  "button_type": "buttonField",
  "table_name": "审批表"
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `button_type` | 是 | 按钮类型：`buttonField`（表格里的按钮，可操作当前记录数据）/ `buttonElement`（仪表盘、应用页面上的按钮，可执行整体操作） |
| `table_name` | 否 | 绑定的数据表名，仅 `button_type=buttonField` 时填写 |

> `buttonField` 和 `buttonElement` 的输出能力不同，详见下方「ButtonTrigger（按钮触发器）」输出说明。

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
