# GenerateAiTextAction

```json
{
  "prompt": [
    { "value_type": "text", "value": "请总结以下内容：" },
    { "value_type": "ref", "value": "$.step_1.fieldxxx" }
  ]
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `prompt` | 是 | TextRefItem[] 提示词，支持 `text` / `ref` |

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
