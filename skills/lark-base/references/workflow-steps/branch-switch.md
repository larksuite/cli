# SwitchBranch

`children.links` 包含多个 `case` 边（`label` 建议用 `branch_1`、`branch_2`，语义写在 `desc`）。

```json
{
  "mode": "exclusive",
  "no_match_action": "classifyToOther",
  "child_branch_list": [
    {
      "name": "高优先级",
      "condition": {
        "conjunction": "or",
        "conditions": [
          {
            "conjunction": "and",
            "conditions": [
              {
                "left_value": { "value_type": "ref", "value": "$.step_1.fieldxxx" },
                "operator": "is",
                "right_value": [{ "value_type": "text", "value": "P0" }]
              }
            ]
          }
        ]
      }
    }
  ]
}
```

| 字段 | 必填 | 说明 |
|------|------|------|
| `mode` | 否 | 分支模式。`exclusive`：排他模式，仅执行一个满足条件的子分支；`parallel`：并行模式，执行所有满足条件的子分支。默认 `exclusive` |
| `no_match_action` | 否 | `mode=exclusive` 时使用，无匹配时的处理策略。`classifyToOther`：归类到其他分支；`fail`：报错终止。默认 `classifyToOther` |
| `fail_mode` | 否 | `mode=parallel` 时使用，部分分支出错时策略。`partialSuccess`：部分成功即继续；`fail`：任一失败即终止。默认 `partialSuccess` |
| `match_mode` | 否 | `mode=parallel` 时使用，所有分支不满足时策略。`noneMatchSkip`：跳过继续；`noneMatchFail`：报错终止。默认 `noneMatchSkip` |
| `child_branch_list` | 是 | BranchItem[]，1-10 个条件分支 |

`BranchItem`：

| 字段 | 类型 | 说明 |
|------|------|------|
| `name` | string | 分支名称 |
| `condition` | OrGroup | 分支条件 |

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
