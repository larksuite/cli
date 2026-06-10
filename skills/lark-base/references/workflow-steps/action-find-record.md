# FindRecordAction

```json
{
  "table_name": "客户表",
  "field_names": ["客户名称", "联系方式", "等级"],
  "should_proceed_when_no_results": true,
  "filter_info": { /* RecordFilterInfo */ }
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `table_name` | 是 | 目标数据表名 |
| `field_names` | 是 | 要检索的字段名列表，至少一个 |
| `should_proceed_when_no_results` | 否 | 无结果时是否继续后续步骤，默认 `true` |
| `filter_info` | 否* | RecordFilterInfo（与 `ref_info` 互斥） |
| `ref_info` | 否* | RefInfo（与 `filter_info` 互斥） |

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
