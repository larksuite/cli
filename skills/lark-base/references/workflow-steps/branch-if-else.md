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

## 端到端示例

完整 workflow 示例：触发器 → IfElseBranch（金额大于 1000）→ 两个分支 action → 汇合后 AI 生成日报。重点看 `children.links` 与 `next` 怎么配合。

```json
{
  "title": "新订单自动通知",
  "steps": [
    {
      "id": "step_1",
      "type": "AddRecordTrigger",
      "title": "当「订单表」新增记录时触发",
      "next": "step_2",
      "data": { "table_name": "订单表", "watched_field_name": "订单编号" }
    },
    {
      "id": "step_2",
      "type": "IfElseBranch",
      "title": "判断订单金额是否大于 1000",
      "children": {
        "links": [
          { "kind": "if_true", "to": "step_3" },
          { "kind": "if_false", "to": "step_4" }
        ]
      },
      "next": "step_5",
      "data": {
        "condition": {
          "conjunction": "or",
          "conditions": [{
            "conjunction": "and",
            "conditions": [{
              "left_value": { "value_type": "ref", "value": "$.step_1.fldXXX" },
              "operator": "isGreater",
              "right_value": [{ "value_type": "number", "value": 1000 }]
            }]
          }]
        }
      }
    },
    {
      "id": "step_3",
      "type": "LarkMessageAction",
      "title": "通知主管审批大额订单",
      "next": null,
      "data": {
        "receiver": [{ "value_type": "ref", "value": "$.step_1.fldOwner" }],
        "send_to_everyone": false,
        "title": [{ "value_type": "text", "value": "大额订单提醒" }],
        "content": [
          { "value_type": "text", "value": "新订单金额为：" },
          { "value_type": "ref", "value": "$.step_1.fldAmount" },
          { "value_type": "text", "value": "元，请及时审批。" }
        ],
        "btn_list": []
      }
    },
    {
      "id": "step_4",
      "type": "SetRecordAction",
      "title": "自动标记小额订单为已通过",
      "next": null,
      "data": {
        "table_name": "订单表",
        "ref_info": { "step_id": "step_1" },
        "field_values": [
          { "field_name": "审批状态", "value": [{ "value_type": "text", "value": "已通过" }] }
        ]
      }
    },
    {
      "id": "step_5",
      "type": "GenerateAiTextAction",
      "title": "AI 生成订单处理日报",
      "next": null,
      "data": {
        "prompt": [
          { "value_type": "text", "value": "请根据以下订单信息生成一份简要的处理日报：" },
          { "value_type": "ref", "value": "$.step_1.fldXXX" }
        ]
      }
    }
  ]
}
```

接线要点：

- 分支节点用 `children.links` 写跳转关系（`if_true` / `if_false`），用 `next` 写"两个分支汇合后"的后继节点；分支内的 action 节点用 `next: null` 表明该分支到此结束、回到 `step_2.next` 汇合点。
- Action 节点（step_3 / step_4 / step_5）只写 `next`，不写 `children`。

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
