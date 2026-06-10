# IfElseBranch

`children.links` 包含 `if_true` 和 `if_false` 两条边，`next` 指向两个分支汇合后的后继节点。

**如果涉及到复杂的多分支场景(分支数目 >= 3时)，你应该采用 SwitchBranch，而不是嵌套的 IfElseBranch**

```json
{
  "condition": {
    "conjunction": "or",
    "conditions": [
      {
        "conjunction": "and",
        "conditions": [
          {
            "left_value": { "value_type": "ref", "value": "$.step_1.fieldxxx" },
            "operator": "isGreater",
            "right_value": [{ "value_type": "number", "value": 1000 }]
          }
        ]
      }
    ]
  }
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `condition` | 是 | OrGroup 判断条件，结构为 `(A and B) or (C and D)` |

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
