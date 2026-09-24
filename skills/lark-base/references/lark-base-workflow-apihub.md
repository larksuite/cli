# APIHubAction 集成节点

`APIHubAction` 当前仅支持下表列出的 9 个飞书集成动作。只能使用表中的 `component_id`、参数和输出，不得推测其他组件标识，或将任意 OpenAPI、MCP 工具、节点捷径套入此协议。用户需要清单外能力时，明确说明当前未支持；只有其他已文档化的节点确实能满足需求时，才使用其对应协议。

用户要求自动创建任务／日程／群组时，在工作流中配置对应组件。直接调用 task/calendar/im 命令会立即操作资源，不能代替工作流节点。

## 当前支持的组件

所有节点共用 `type: "APIHubAction"`，在 `meta.component_id` 写入下表标识，参数放在 `data.params`。

| 能力 | component_id | 必填参数 | 可选参数 |
|---|---|---|---|
| 创建任务 | `Automation_Create_Task` | `rich_summary` | `collaborator_ids`、`endTime`、`rich_description`、`follower_ids` |
| 完成任务 | `Automation_Complete_Task` | `task_id` | — |
| 取消完成任务（重启） | `Automation_Uncomplete_Task` | `task_id` | — |
| 删除任务 | `Automation_Delete_Task` | `task_id` | — |
| 创建日程 | `Automation_Create_Schedule` | `startTime`、`endTime`、`calendarId` | `summary`、`attendees`、`description`、`location`、`needNotification` |
| 删除日程 | `Automation_Delete_Schedule` | `calendarId`、`eventId` | `needNotification` |
| 创建群组 | `Automation_Create_Group` | `name`、`userList`、`owner` | `description`、`topBase` |
| 添加群成员 | `Automation_Add_Group_Member` | `group`、`userList` | — |
| 移除群成员 | `Automation_Delete_Group_Member` | `group`、`userList` | — |

- `params[].key` 大小写敏感，保留上表拼写；例如 `endTime`、`userList`，不能改成 `end_time`、`user_list`。
- `params[].value` 始终是数组，数组项使用 `{ "value_type": "...", "value": ... }`。同一参数只出现一次。
- `meta` 属于 step，与 `data` 同级。更新前 get 完整工作流，保留每个节点的 `meta` 和原有连线。
- APIHub 作为主链动作时用 `next` 串联；作为 Agent 工具时，由 Agent 通过 `slot/tool` 挂载，见 [AI Agent](lark-base-workflow-ai-agent.md)。
- 可选参数按用户需求填写：未要求时可省略整个参数项；明确要求负责人、截止时间、描述、关注人等时必须配置，不能因为最小示例未填写就忽略。回读补出的空数组或默认值不表示字段不受支持。
- 节点捷径的 `CustomAction` 使用另一套协议；不要将其组件标识和表单参数直接当作 APIHubAction 参数。

## 参数值类型

下表中的 `ref` 是前置步骤输出引用，例如 `{ "value_type": "ref", "value": "$.create_task.taskId" }`。

| 参数 | 允许的 value_type | 要点 |
|---|---|---|
| `rich_summary`、`rich_description`、`summary`、`name`、`description`、`location` | `text` / `ref` | text 的 value 为字符串；任务标题和群名称必填 |
| `startTime`、`endTime` | `date` / `ref` | date 的 value 为日期时间字符串，例如 `2026-09-30 18:00`；先确认 Base 时区，日程结束时间晚于开始时间 |
| `collaborator_ids`、`follower_ids`、`userList`、`owner` | `user` / `ref` | user 的 value 为 `{ "id": "ou_...", "name": "..." }`；owner 单选 |
| `attendees` | `user` / `group` / `ref` | 可包含人员或群组；只添加用户指定的参会对象 |
| `group` | `group` / `ref` | group 的 value 为 `{ "id": "oc_...", "name": "..." }`，单选 |
| `needNotification`、`topBase` | `boolean` / `ref` | value 为 JSON `true` / `false`，不是字符串 |
| `task_id`、删除日程的 `calendarId` 和 `eventId` | `text` / `ref` | 使用真实 ID 或前置输出，不填名称、URL 或臆造 ID |
| 创建日程的 `calendarId` | **仅 `option`** | value 为 `{ "id": "真实日历ID", "name": "日历名称" }`；不接受 text 或纯 ref |

“取消完成”恢复原任务为未完成，不创建新任务。删除日程、删除任务、移除群成员均作用于真实对象，须明确目标；移除成员不等于解散群。`topBase` 表示将当前多维表格添加到群顶部标签页。

任务状态字段不能代替实际任务动作，不以改状态或隐藏记录代替删除；跨次触发时保存创建动作返回的资源标识，后续操作引用同一对象。不同触发时机分别配置流程，缺失资源只阻塞依赖它的部分。

## 创建任务的可选配置

- 负责人：`collaborator_ids`；关注人：`follower_ids`。两者是不同角色，可同时填写。
- 截止时间：`endTime`；任务描述：`rich_description`。用户未要求时可省略，填写时按上表类型提交。
- 标题、描述可混排常量与前置字段引用；不能把用户要求的动态内容替换成固定示例文案。

以下片段在创建任务的 `data.params` 中与 `rich_summary` 一起使用；人员必须替换为真实查询结果：

```json
[
  { "key": "collaborator_ids", "value": [{ "value_type": "user", "value": { "id": "<负责人open_id>", "name": "<负责人姓名>" } }] },
  { "key": "endTime", "value": [{ "value_type": "date", "value": "2026-09-30 18:30" }] },
  { "key": "rich_description", "value": [{ "value_type": "text", "value": "检查并跟进此记录" }] },
  { "key": "follower_ids", "value": [{ "value_type": "user", "value": { "id": "<关注人open_id>", "name": "<关注人姓名>" } }] }
]
```

## 先获取资源

只查询本次用到的资源，优先使用前置节点输出，避免重复发现。

| 需要的资源 | 查询入口 | 保存到节点中的内容 |
|---|---|---|
| 人员 | `lark-cli contact +search-user`；当前用户用 `contact +get-user` | 人员 open_id 和名称；不要填写内部数字 ID |
| 已有群组／成员 | `lark-cli im +chat-search`、`im +chat-members-list` | 群 chat_id 和名称；先核对移除对象确实在群内 |
| 日历 | `lark-cli calendar calendars primary` / `calendar calendars list` / `calendar calendars search` | 日历 ID 和名称；用户未指定日历时先查当前用户主日历 |
| 已有日程 | `lark-cli calendar +search-event` / `calendar +agenda` / `calendar +get` | 日历 ID 和日程 ID，不能只凭同名日程选择删除目标 |
| 已有任务 | `lark-cli task +search` / `task +get-my-tasks` | 任务 ID；先核对标题、负责人及状态 |

执行前用对应命令 `--help` 或 `schema` 确认参数。ID 以实际查询结果或工作流输出为准；若日历查询结果被 Workflow 校验拒绝，报告日历上下文或 ID 不匹配，不尝试改写 ID。资源不可访问时按原身份修复授权；不替换为任意可访问对象。

人员参数优先引用承载目标角色的人员字段。只有文本姓名时，先查询解析身份；可用新增人员字段保存映射，不改写原姓名字段。查无结果或重名时报告待确认项，不能用当前用户、创建人或占位 ID 替代。指定多人时逐人核对，不把部分添加说成全部完成；缺失身份不妨碍先完成其他独立配置。

“明天下午”等配置时确定的时间，结合当前日期和 Base 时区转为明确日期时间字符串；不要写入未定义的相对时间对象。需要随每次触发变化的时间时，引用前置日期字段或日期输出，不把配置时算出的固定时间当动态时间。

## 完整示例：新增记录时创建任务

先查询真实表名和监控字段名。以下表名、字段名需替换；`client_token` 每次创建生成新的值。示例不设置负责人，避免把未明确指定的人加到任务中。

```json
{
  "client_token": "<本次唯一请求标识>",
  "title": "新增记录时创建跟进任务",
  "steps": [
    {
      "id": "new_record",
      "type": "AddRecordTrigger",
      "data": { "table_name": "待办来源", "watched_field_name": "标题" },
      "next": "create_task"
    },
    {
      "id": "create_task",
      "type": "APIHubAction",
      "title": "创建飞书任务",
      "meta": { "component_id": "Automation_Create_Task" },
      "data": {
        "params": [
          { "key": "rich_summary", "value": [{ "value_type": "text", "value": "跟进新增记录" }] }
        ]
      }
    }
  ]
}
```

创建日程的日历参数示例（注意 option 与删除节点的 text 不同）：

```json
{
  "key": "calendarId",
  "value": [{ "value_type": "option", "value": { "id": "<查询到的日历ID>", "name": "<日历名称>" } }]
}
```

## 输出引用

只引用当前节点之前且在执行路径上可用的输出。路径中的大小写原样保留：

| 创建节点 | 可引用输出 | 后续用途 |
|---|---|---|
| 创建任务 | `$.<stepId>.taskId`、`$.<stepId>.applink`、`creator`、`createTime` | 后续任务操作的 `task_id` 引用 `taskId`；发送消息或写记录时引用 `applink` 作为任务详情链接 |
| 创建日程 | `$.<stepId>.calendarId`、`scheduleId`、`meetingUrl`、`creator` | 删除日程 `eventId` 引用 `scheduleId`，`calendarId` 引用同名输出 |
| 创建群组 | `$.<stepId>.group`、`groupShareLink` | 添加／移除群成员的 `group` 引用群对象 |

`applink` 是创建任务节点返回的任务 AppLink，不要用 `taskId` 拼接链接。

其余 6 个组件没有可引用业务输出。不要自行拼 `.data`、`.output`、`.result` 等中间层，也不推测 creator/group 的下钻字段；以服务端返回和校验为准。

## 保存和检查

1. 保存前可用 `--dry-run` 检查请求；按用户要求用 `+workflow-create` / `+workflow-update` 提交完整定义。
2. 用 `+workflow-get` 回读同一 ID，核对组件、参数、引用和连线，尤其是 `meta.component_id`、日历、人员和群。
3. 按 [Workflow 入口](lark-base-workflow.md) 确定用户需要的运行态；需要切换时使用 enable/disable 专用命令，并回读确认。
4. 创建或启用成功只说明定义被接受。用户要求验证实际执行时，按 [检查与实际运行](lark-base-workflow.md#检查与实际运行) 的授权范围触发，核对任务、日程或群的业务结果。

查看工作流写入的记录时，`+record-list` 使用 `--format json`；按 `data.fields` 的列顺序读取 `data.data` 行数组，不按 `row.fields` 解析。分页使用 `offset`；需要核对全部结果时，读到 `has_more=false`。
