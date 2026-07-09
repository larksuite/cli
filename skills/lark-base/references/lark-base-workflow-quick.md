# Workflow quick

本文档是 Base Workflow 的高频路径速查。先用它判断是否能走短路径；完整 guide 和 schema 仍是新 step 字段的事实来源，只有 quick 不够时再读相关小节。

## 路由

| 任务 | 优先命令 | 下一步/阅读判断 |
|---|---|---|
| 按名称查找 workflow | `+workflow-list --base-token <base> --as user --format json --jq '<filter>'` | 只有唯一匹配且需要查看或更新时，才对该 workflow 用 `+workflow-get`；查 ID 不读 schema。 |
| 启用或停用 | `+workflow-list` -> `+workflow-disable` / `+workflow-enable` | 不读 steps schema；启停不修改 steps。 |
| 查看 title/status/steps | `+workflow-get --workflow-id <wkf>` | 只报告 title、status、ID 或 steps 为空时，不读 schema。 |
| 更新 workflow steps | `+workflow-get` -> 编辑返回的 `title/status/steps` -> `+workflow-update` | 更新是全量替换语义；不想改的字段必须保留。修改 step 结构或字段含义时再读 schema/guide。 |

## 保持短路径

以下任务不要打开完整 guide/schema：

- 列出 workflow、查找唯一 workflow ID、启用、停用、报告当前状态；
- 只改 `title`，并原样保留返回的 `status` 和 `steps`；状态变更使用 `+workflow-enable` 或 `+workflow-disable`；
- 基于 `+workflow-get` 返回结果小幅修改现有 workflow，并保持已有 step 的 `type`、`data`、`next`、`children` 结构不变；

创建 workflow、从零构造 steps、新增 step 类型、修改 branch/loop 链接、编写 ref 路径，或恢复平台 schema 错误时，不走 quick 的短路径，直接读完整 guide/schema 的相关小节。本地阅读时先搜索标题，只读必要的小节。

## 不支持的触发边界

- 当前 workflow step schema 没有原生“记录被物理删除”触发器。不要搜索或编造 `DeleteRecordTrigger`、`DeletedRecordTrigger`、`RecordDeletedTrigger`。
- 如果用户要求“record deleted” / “删除记录”触发 workflow，说明 Base workflow steps 不支持精确的物理删除事件。可建议可建模替代方案：删除前先把状态改为“离职/已删除”、清空标记字段，或更新一个布尔字段，再用 `SetRecordTrigger` / `ChangeRecordTrigger` 监听该字段变化。
- 如果任务必须在物理行已经删除之后触发，停止并说明能力边界，不要创建误导性的 workflow。

## 常见 Step Data Key

下面这些 key 足够搭出常见 workflow 骨架。只有下表不够、平台错误要求特定字段，或需要 branch/loop/ref 细节时，才读完整 schema。

| Step type | 最小 `data` key |
|---|---|
| `AddRecordTrigger` | `table_name`、`watched_field_name`，可选 `trigger_control_list` 和 `condition_list` |
| `TimerTrigger` | `rule`（常见 `DAILY` / `WORKDAY`）、`start_time`（`YYYY-MM-DD HH:mm`），可选 `is_never_end` |
| `SetRecordTrigger` | `table_name`、`field_watch_info: [{ "field_name": "<field>" }]`，可选 `trigger_control_list` 和 `condition_list` |
| `ChangeRecordTrigger` | `table_name`，可选 `trigger_control_list` 和 `condition_list`；需要新增或修改都触发时使用 |
| `FindRecordAction` | `table_name`、`field_names`、`should_proceed_when_no_results`，并且 `filter_info` / `ref_info` 二选一；构造定位条件时读 schema/guide |
| `GenerateAiTextAction` | `prompt` 使用 text/ref items；后续 step 引用生成文本时用 `$.<stepId>`，不是嵌套路径 |
| `LarkMessageAction` | `receiver`、`send_to_everyone`、`content`，可选 `title`，不需要按钮时 `btn_list` 为 `[]` |
| `Delay` | `duration` 单位为分钟，范围 1-120 |

## 必要检查

- 从 `/base/<token>` URL 中提取 `base_token` 后再运行 Base 命令。
- 使用命令返回的真实名称和 ID。不要猜 table name、field name、workflow ID、group ID、user ID 或 receiver。
- 构造 `steps` 前，读取会引用的真实 Base 结构：表用 `+table-list`，字段用 `+field-list`，现有 workflow 用 `+workflow-list/get`。
- `+workflow-update` 会替换完整 workflow 定义。更新现有 workflow 时先从 `+workflow-get` 开始，并保留不变的 `title`、`status` 和 `steps`。
- 写操作默认继续使用 `--as user`，除非用户明确要求 bot 身份，或权限恢复需要走 shared auth 流程。

## Step Schema 指针

只有 quick 不包含所需 step 细节时，才读完整 schema 或 guide。

| 需求 | 阅读位置 |
|---|---|
| 记录删除触发器 | 不要为了 `DeleteRecordTrigger` 继续读；当前不支持。改用删除前状态/字段变化建模，或在必须精确监听物理删除时说明不支持。 |
| 常见简单流程：record/timer trigger -> message、add/update/find record、delay、AI text action | 先搜索标题；只读缺失的 guide 示例或 schema step data 小节 |
| 基础 step 结构、`next`、`children.links` | `lark-base-workflow-schema.md` -> WorkflowStep / StepChildren |
| 触发器类型选择 | `lark-base-workflow-schema.md` -> Trigger types |
| 消息接收人与内容 | `lark-base-workflow-schema.md` -> LarkMessageAction |
| AI 文本生成 | `lark-base-workflow-schema.md` -> GenerateAiTextAction |
| 查找记录、新增记录、更新记录 | `lark-base-workflow-schema.md` -> Action data |
| 定时、工作日、提醒时间 | `lark-base-workflow-schema.md` -> TimerTrigger / ReminderTrigger |
| if/else、switch、loop | `lark-base-workflow-guide.md` 示例，以及 schema 的 branch/system 小节 |

## 最小 Body 形状

```json
{
  "title": "workflow title",
  "status": "disabled",
  "steps": [
    {
      "id": "step_trigger",
      "type": "AddRecordTrigger",
      "title": "trigger title",
      "next": "step_action",
      "data": {}
    },
    {
      "id": "step_action",
      "type": "LarkMessageAction",
      "title": "action title",
      "next": null,
      "data": {}
    }
  ]
}
```

这只是外层结构。每个 `data` 对象必须来自相关 schema 小节或已有 `+workflow-get` 返回；不要根据自然语言编造不支持的字段。
