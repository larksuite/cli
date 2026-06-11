# Workflow steps JSON SSOT

本文档是 Workflow steps 的按需读取入口。先读 [lark-base-workflow-guide.md](lark-base-workflow-guide.md) 确定任务路径；只有需要具体 step 字段时，再按 type 打开对应小文件。

## 读取顺序

1. 查询、启停 workflow：只用 `+workflow-list/get/enable/disable` 和命令返回，不读本目录，也不要默认看 help。
2. 创建或更新 workflow：先读 guide；如果 guide 的场景表不足以构造 step，再读本文件的基础结构和 step 路由表。
3. 先确定本次 workflow 会用到的完整 step type 集合，去重后一次性打开对应 step md 文件；不要每确定一个节点就读一次文件。
4. 需要 value/ref/filter 条件时，把 [common-types-and-refs.md](workflow-steps/common-types-and-refs.md) 加入同一批读取；不需要这些结构时不要读。

## WorkflowStep 基础结构

```json
{
  "id": "step_xxx",
  "type": "AddRecordTrigger",
  "title": "监控新订单",
  "next": "step_yyy",
  "data": {}
}
```

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `id` | string | 是 | 步骤唯一 ID，被 `next` 和 `children.links[].to` 引用 |
| `type` | string | 是 | 步骤类型，按下方路由打开对应文件 |
| `title` | string | 否 | 步骤标题 |
| `children` | StepChildren | 否 | 分支/循环关系边；普通 trigger/action 不设置 |
| `next` | string/null | 否 | 线性后继节点 ID；`null` 表示流程结束 |
| `data` | object | 是 | 按 `type` 区分的配置对象 |

总原则：连线写 `children`，扩展标识写 `meta`，输入参数写 `data`。

## StepChildren 与 ChildLink

```json
{
  "links": [
    { "kind": "if_true", "to": "step_4", "label": "branch_1", "desc": "金额大于1000" }
  ]
}
```

| kind | 使用节点 | 说明 |
|---|---|---|
| `if_true` | IfElseBranch | 条件为真时跳转 |
| `if_false` | IfElseBranch | 条件为假时跳转 |
| `case` | SwitchBranch | 多路分支，`label` 建议用 `branch_1` 等中性标签，`desc` 写语义 |
| `loop_start` | Loop | 循环体入口 |
| `slot` | AIAgentAction | 挂载 LLM / 工具 / 记忆子节点，`label` 为 `llm` / `tool` / `memory` |

## Step type 路由

### Trigger

| type | 说明 | 按需读取 |
|---|---|---|
| `AddRecordTrigger` | 新增记录时触发 | [trigger-add-record.md](workflow-steps/trigger-add-record.md) |
| `SetRecordTrigger` | 记录被修改时触发 | [trigger-set-record.md](workflow-steps/trigger-set-record.md) |
| `ChangeRecordTrigger` | 记录满足条件时触发；新增或修改都触发 | [trigger-change-record.md](workflow-steps/trigger-change-record.md) |
| `TimerTrigger` | 定时触发 | [trigger-timer.md](workflow-steps/trigger-timer.md) |
| `ReminderTrigger` | 日期提醒触发 | [trigger-reminder.md](workflow-steps/trigger-reminder.md) |
| `ButtonTrigger` | 按钮点击触发 | [trigger-button.md](workflow-steps/trigger-button.md) |
| `LarkMessageTrigger` | 接收飞书消息触发 | [trigger-lark-message.md](workflow-steps/trigger-lark-message.md) |

触发器选型：新增记录用 `AddRecordTrigger`；只监听修改用 `SetRecordTrigger`；新增或修改都触发、或拿不准时用 `ChangeRecordTrigger`。"新增或修改满足同一条件就触发"（如"改为 X 或新增 X 时通知"）是单个 `ChangeRecordTrigger` 的典型场景，不要拆成两条工作流。

### Action

| type | 说明 | 按需读取 |
|---|---|---|
| `AddRecordAction` | 新增记录 | [action-add-record.md](workflow-steps/action-add-record.md) |
| `SetRecordAction` | 更新记录 | [action-set-record.md](workflow-steps/action-set-record.md) |
| `FindRecordAction` | 查找记录 | [action-find-record.md](workflow-steps/action-find-record.md) |
| `HTTPClientAction` | HTTP 请求 | [action-http-client.md](workflow-steps/action-http-client.md) |
| `Delay` | 延迟 | [action-delay.md](workflow-steps/action-delay.md) |
| `LarkMessageAction` | 发送飞书消息 | [action-lark-message.md](workflow-steps/action-lark-message.md) |
| `GenerateAiTextAction` | AI 生成文本 | [action-generate-ai-text.md](workflow-steps/action-generate-ai-text.md) |

所有 Action 节点不要设置 `children`，通过 `next` 串联后继。

### Branch / System

| type | 说明 | 按需读取 |
|---|---|---|
| `IfElseBranch` | 条件分支，`children.links` 含 `if_true` 和 `if_false` | [branch-if-else.md](workflow-steps/branch-if-else.md) |
| `SwitchBranch` | 多路分支，`children.links` 含多个 `case` | [branch-switch.md](workflow-steps/branch-switch.md) |
| `Loop` | 循环，`children.links` 含 `loop_start` 指向循环体入口 | [system-loop.md](workflow-steps/system-loop.md) |

## 公共结构

只有在需要构造 `value_type`、`ref`、条件过滤、字段值、节点输出引用时，才读 [common-types-and-refs.md](workflow-steps/common-types-and-refs.md)。

## 参考

- Workflow 创建/更新入口路由：[lark-base-workflow-guide.md](lark-base-workflow-guide.md)
- 命令参数以 `lark-cli base +workflow-create --help` / `+workflow-update --help` 为准；只有参数不确定或命令报错时才读取 help。
