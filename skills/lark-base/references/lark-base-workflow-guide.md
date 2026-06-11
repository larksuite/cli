# Workflow guide

本文档只做 Workflow 创建/更新入口路由，避免默认读入完整 steps schema。查询、启停 workflow 不需要读本文档。

## 什么时候读

| 目标 | 处理方式 |
|---|---|
| 列出或查看 workflow | 先看 `lark-cli base +workflow-list --help` / `+workflow-get --help`，按返回摘要回答 |
| 启用或停用 workflow | 先确认 workflow ID 和当前状态，再用 `+workflow-enable` / `+workflow-disable` |
| 创建或更新简单 workflow | 读本文件，再按 step type 打开 schema 小文件 |
| 复用或解释复杂 `steps` | 读 [lark-base-workflow-schema.md](lark-base-workflow-schema.md) 的路由表，只打开涉及的 step 文件 |

## 创建/更新最小流程

1. 调用命令前先看 `--help`，不要猜参数名或 JSON 结构。
2. 先确认真实 Base、表、字段、视图和用户/群 ID；不要凭口述猜字段名或 field ID。
3. 选择一个 trigger；新增记录用 `AddRecordTrigger`，只监听修改用 `SetRecordTrigger`，新增或修改都触发/拿不准用 `ChangeRecordTrigger`。用户描述"修改为 X **或** 新增 X 时"这类同条件多来源需求，是 `ChangeRecordTrigger` 的典型场景：**一条工作流 + condition_list 即可，不要拆成 AddRecordTrigger 和 SetRecordTrigger 两条工作流**。
4. 选择 action/branch/system step，打开对应 schema 文件。
5. 需要 `value_type`、`ref`、条件、字段值或输出引用时，再读 [common-types-and-refs.md](workflow-steps/common-types-and-refs.md)。
6. 组装 `title/status/steps` 后用 `+workflow-create` 或 `+workflow-update`。

## Step 文件路由

入口：[lark-base-workflow-schema.md](lark-base-workflow-schema.md)

| 场景 | 常见步骤 | 只读这些文件 |
|---|---|---|
| 新增记录后通知 | `AddRecordTrigger -> LarkMessageAction` | [trigger-add-record.md](workflow-steps/trigger-add-record.md), [action-lark-message.md](workflow-steps/action-lark-message.md), common refs |
| 定时查找并循环处理 | `TimerTrigger -> FindRecordAction -> Loop -> ...` | [trigger-timer.md](workflow-steps/trigger-timer.md), [action-find-record.md](workflow-steps/action-find-record.md), [system-loop.md](workflow-steps/system-loop.md), common refs |
| 条件分支 | `... -> IfElseBranch -> ...` | [branch-if-else.md](workflow-steps/branch-if-else.md), common conditions |
| 多路分类 | `... -> SwitchBranch -> ...` | [branch-switch.md](workflow-steps/branch-switch.md), common conditions |
| 按钮触发外部系统 | `ButtonTrigger -> HTTPClientAction -> AddRecordAction` | [trigger-button.md](workflow-steps/trigger-button.md), [action-http-client.md](workflow-steps/action-http-client.md), [action-add-record.md](workflow-steps/action-add-record.md), common refs |
| AI 生成内容 | `... -> GenerateAiTextAction` | [action-generate-ai-text.md](workflow-steps/action-generate-ai-text.md), common refs |

## 结构速记

```json
{
  "title": "工作流标题",
  "status": "enable",
  "steps": [
    {"id":"step_trigger","type":"AddRecordTrigger","title":"触发器","next":"step_action","data":{}},
    {"id":"step_action","type":"LarkMessageAction","title":"动作","next":null,"data":{}}
  ]
}
```

- 普通 trigger/action 用 `next` 串联。
- `IfElseBranch` / `SwitchBranch` / `Loop` 用 `children.links` 表达分支或循环入口。
- `ref` 路径用前置 step 的 `id`，字段下钻通常是 `$.{stepId}.{fieldId}` 或 `$.{loopStepId}.item.{fieldId}`。
- Action 节点不要设置 `children`。

## 常见错误

| 错误 | 处理 |
|---|---|
| 把字段名当 field ID 写入 ref | 先读真实字段结构；ref 下钻通常使用 field ID |
| 分支/循环没有 `children.links` | 按 branch/loop schema 补 `if_true/if_false/case/loop_start` |
| SetRecordAction/FindRecordAction 缺定位条件 | 提供 `filter_info` 或 `ref_info` |
| HTTPClientAction 后续节点引用不到字段 | `response_type=json` 时填写 `response_value` 声明输出字段 |
| Loop 内引用错路径 | 用 `$.{loopStepId}.item.{fieldId}` 和 `$.{loopStepId}.index` |

## 参考

- [lark-base-workflow-schema.md](lark-base-workflow-schema.md)：steps 基础结构和按需路由
- [workflow-steps/common-types-and-refs.md](workflow-steps/common-types-and-refs.md)：ValueInfo、ref、Condition、节点输出
