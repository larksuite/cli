# Loop

`children.links` 包含 `loop_start` 边指向循环体入口，`next` 指向循环结束后的后继节点。

```json
{
  "loop_mode": "continue",
  "max_loop_times": 100,
  "data": [{ "value_type": "ref", "value": "$.find_record_stepIdxxx.fieldRecords" }]
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `data` | 是 | ValueInfo[]（仅支持 `ref` 类型），循环数据源，只能填一个 |
| `loop_mode` | 否 | 单次错误时是否继续：`end`（终止）/ `continue`（继续） |
| `max_loop_times` | 否 | 最大循环次数 |

---

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
