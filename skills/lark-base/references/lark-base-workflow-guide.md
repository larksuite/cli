# Workflow guide

本文档是 Workflow 的操作地图：先用它决定最短路径，再按需打开 schema 小文件。Guide 要一次读完后能完成大多数查询、启停和常见创建/修改；schema 才是零件手册。

## 先判断任务类型

| 目标 | 最短路径 | 是否读 schema |
|---|---|---|
| 列出 workflow | `+workflow-list --base-token <base>`；需要筛选启停状态时用 `--status` | 不读 |
| 查看一个 workflow | 先 `+workflow-list` 后按标题本地匹配 `workflow_id`，再 `+workflow-get --workflow-id <wkf>` | 不读，除非要解释完整 `steps` |
| 启用/停用 workflow | `+workflow-list --status <enabled|disabled>` 定位，再 `+workflow-enable/disable` | 不读 |
| 创建简单 workflow | 读本 guide，按下方场景表打开必要 step schema | 只读命中的 step |
| 修改 workflow | `+workflow-get` 取现状，保留无关字段，只改目标 step；复杂 step 再读 schema | 只读被改的 step |
| 解释复杂 `steps` | 先用本 guide 的结构速记理解连线，再按 step type 打开 schema | 按需读 |

不要默认看 `--help`。只有命令报错、参数名不确定、或要确认复杂写入参数时，才看当前命令的 help。

## 资源发现顺序

1. 从用户链接提取 `base_token`。
2. 需要知道文档内资源时用 `+base-block-list` 或 `+table-list`；不要两者都跑，除非一个结果不够。
3. 字段发现默认用 `+field-list --compact`；只有需要公式、lookup 或完整字段配置时再 `+field-get`。
4. 多表字段发现用 `+field-list-batch --compact --table-id <id1> --table-id <id2>`。
5. workflow 定位用 `+workflow-list` 读取列表，再按 `title` 本地匹配；当前命令没有 `--title` flag。

## Workflow 结构速记

```json
{
  "client_token": "unique-create-token",
  "title": "工作流标题",
  "steps": [
    {
      "id": "step_trigger",
      "type": "AddRecordTrigger",
      "title": "触发器",
      "next": "step_action",
      "data": {}
    },
    {
      "id": "step_action",
      "type": "LarkMessageAction",
      "title": "动作",
      "next": null,
      "data": {}
    }
  ]
}
```

- `id` 要稳定、可读，被 `next` 和 `children.links[].to` 引用。
- 普通 trigger/action 用 `next` 串联；最后一个节点 `next:null`。
- `IfElseBranch` / `SwitchBranch` / `Loop` 用 `children.links` 表达分支或循环入口。
- Action 节点不要设置 `children`。
- `ref` 引用前置 step 的输出，字段下钻通常是 `$.{stepId}.{fieldId}`；循环内当前项常用 `$.{loopStepId}.item.{fieldId}`。
- `+workflow-create` 需要唯一 `client_token`；新 workflow 创建后默认 disabled，用户需要启用时再调用 `+workflow-enable`。
- `+workflow-update` 是完整替换；从 `+workflow-get` 返回中保留不想改的 `title/status/steps`。

## Step 选型

创建/修改前先产出一个草图：列出全部节点 `id/type/next/children`，把会用到的 `type` 去重后，再一次性读取对应的 step md 文档。不要“读一个 step、想一轮、再读下一个 step”；这会增加轮次和上下文重放。

| 用户说法 | 选型 |
|---|---|
| 新增记录时 | `AddRecordTrigger` |
| 记录被修改时 | `SetRecordTrigger` |
| 新增或修改都触发、或拿不准 | `ChangeRecordTrigger` |
| 每天/每周/每月/固定时间 | `TimerTrigger` |
| 日期字段到期提醒 | `ReminderTrigger` |
| 点击按钮 | `ButtonTrigger` |
| 收到群消息/私聊消息 | `LarkMessageTrigger` |
| 新增一条记录 | `AddRecordAction` |
| 更新当前或查找到的记录 | `SetRecordAction` |
| 查找多条记录再处理 | `FindRecordAction`，多条时接 `Loop` |
| 分两路判断 | `IfElseBranch` |
| 多档位/多类别判断 | `SwitchBranch` |
| 发送飞书消息 | `LarkMessageAction` |
| 调外部接口 | `HTTPClientAction` |
| 等待一段时间 | `Delay` |
| AI 生成文本 | `GenerateAiTextAction` |

用户描述"修改为 X **或** 新增 X 时"这类同条件多来源需求，是单个 `ChangeRecordTrigger` + `condition_list` 的典型场景，一条工作流即可表达，不要拆成 `AddRecordTrigger` 和 `SetRecordTrigger` 两条工作流。

## 常见场景

| 场景 | 推荐步骤 | 需要读的 schema |
|---|---|---|
| 新增记录后发通知 | `AddRecordTrigger -> LarkMessageAction` | `trigger-add-record.md`, `action-lark-message.md` |
| 记录变化后更新同一行字段 | `ChangeRecordTrigger -> SetRecordAction` | `trigger-change-record.md`, `action-set-record.md`; 条件复杂再读 common refs |
| 金额/状态分档处理 | `AddRecordTrigger -> SwitchBranch -> SetRecordAction...` | `trigger-add-record.md`, `branch-switch.md`, `action-set-record.md`, common conditions |
| 二选一判断 | `... -> IfElseBranch -> ...` | `branch-if-else.md`, common conditions |
| 定时汇总并逐人通知 | `TimerTrigger -> FindRecordAction -> Loop -> LarkMessageAction` | `trigger-timer.md`, `action-find-record.md`, `system-loop.md`, `action-lark-message.md`, common refs |
| 群消息触发后回复 | `LarkMessageTrigger -> FindRecordAction/Loop -> LarkMessageAction` | `trigger-lark-message.md`, `action-find-record.md`, `system-loop.md`, `action-lark-message.md` |
| 按钮触发外部系统 | `ButtonTrigger -> HTTPClientAction -> AddRecordAction` | `trigger-button.md`, `action-http-client.md`, `action-add-record.md` |
| 调用 AI 生成内容并写回 | `... -> GenerateAiTextAction -> SetRecordAction` | `action-generate-ai-text.md`, `action-set-record.md`, common refs |

Schema 入口：[lark-base-workflow-schema.md](lark-base-workflow-schema.md)。不要一次性打开所有 step 文件；先确定本次 workflow 的完整 step type 集合，再一次性打开这些文件。只有确定会写 `ref`、条件、字段值或节点输出引用时，才把 `common-types-and-refs.md` 加入同一批读取。

## 最小例子：新增记录后发送消息

只读 `trigger-add-record.md` 和 `action-lark-message.md` 即可。

```json
{
  "client_token": "wf-unique-token",
  "title": "新订单通知",
  "steps": [
    {
      "id": "trig_new_order",
      "type": "AddRecordTrigger",
      "title": "新增订单时",
      "next": "act_notify",
      "data": {
        "table_name": "订单表",
        "watched_field_name": "订单号"
      }
    },
    {
      "id": "act_notify",
      "type": "LarkMessageAction",
      "title": "发送通知",
      "next": null,
      "data": {
        "receiver": [{ "value_type": "user", "value": { "id": "ou_xxx" } }],
        "send_to_everyone": false,
        "title": [{ "value_type": "text", "value": "新订单提醒" }],
        "content": [{ "value_type": "text", "value": "收到新订单" }],
        "btn_list": []
      }
    }
  ]
}
```

## 修改现有 workflow

1. `+workflow-list` 后按标题定位 `workflow_id`。
2. `+workflow-get --workflow-id <wkf>` 获取完整定义。
3. 只修改目标 step，保留其他 steps 的 `id/type/title/data/next/children`。
4. 用 `+workflow-update` 提交完整定义。
5. 若只启停，不走 update，直接 `+workflow-enable/disable`。

## 常见错误

| 错误 | 处理 |
|---|---|
| 查询/启停也读 schema | 停下，直接用 `+workflow-list/get/enable/disable` |
| 为多个可能命令批量看 help | 只看当前报错或即将执行的一个命令 |
| 把字段名当 field ID 写入 ref | 先 `+field-list --compact`，ref 下钻优先用 field ID |
| 分支/循环没有 `children.links` | 按 branch/loop schema 补 `if_true/if_false/case/loop_start` |
| SetRecordAction/FindRecordAction 缺定位条件 | 提供 `filter_info` 或 `ref_info` |
| HTTPClientAction 后续节点引用不到字段 | `response_type: "json"` 时填写 `response_value` 声明输出字段 |
| Loop 内引用错路径 | 用 `$.{loopStepId}.item.{fieldId}` 和 `$.{loopStepId}.index` |

## 参考

- [lark-base-workflow-schema.md](lark-base-workflow-schema.md)：step type 路由和基础结构。
- [workflow-steps/common-types-and-refs.md](workflow-steps/common-types-and-refs.md)：ValueInfo、ref、Condition、节点输出；只有构造这些细节时才读。
