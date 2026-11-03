# 工作流消息与邮件

基础字段、接收人约束和最小示例见 [消息](lark-base-workflow-schema.md#larkmessageaction)／[邮件](lark-base-workflow-schema.md#larkemailaction)。按需求或 `+workflow-get` 已有配置读取对应章节；完整更新须保留已有配置，即使本轮只改普通文字。

| 需求或已有配置 | 读取 |
|---|---|
| 标题、列表、样式、整体链接、分隔线；`content_type:rich_text` / `rich_text_content` | [富文本正文](#富文本正文) |
| 仪表盘截图、应用截图、仪表盘智能总结 | [图片与智能总结引用](#图片与智能总结引用) |
| 图片或其他附件 | [附件](#附件) |
| 多记录汇总、表格或列表展示 | [一条消息展示多条记录](#一条消息展示多条记录) |
| 标题背景色、按钮样式、触发 Workflow、修改前序记录、防重复点击或互斥 | [标题背景色与按钮扩展](#标题背景色与按钮扩展) |

## 富文本正文

`LarkMessageAction` 与 `LarkEmailAction` 的 `content_type` 省略或为 `plain_text` 时使用普通 `content`；设为 `rich_text` 时，以 `rich_text_content` 替代 `content`：

```json
{"content_type":"rich_text","rich_text_content":[{"type":"paragraph","line_marker_list":[{"type":"heading","level":1}],"value":[{"value_type":"text","value":"状态更新通知","style":{"bold":true,"color":"blue"}}]},{"type":"paragraph","value":[{"value_type":"ref","value":"$.find_1.recordNum"},{"value_type":"link","value":{"text":[{"value_type":"text","value":"查看详情"}],"link":[{"value_type":"ref","value":"$.find_1.firstfieldsRecord.detail_link"}]}}]},{"type":"horizontalLine"},{"type":"paragraph","value":[]}]}
```

- block 为 `paragraph` 或 `horizontalLine`；paragraph 的 value 可以是空数组，保留空段；段内 `\n` 保持用户换行。line_marker_list 可省略。普通/富文本模式切换时仅提交对应的 content 或 rich_text_content，不能同时出现；富文本必须显式 content_type rich_text。
- marker 支持 `heading`（level 1/2/3）、`unorderedList`、`orderedList`；列表 `level` 使用正整数。`orderedList` 只必填 type/level，`order_id` 和 `order` 都可省略。
- `order_id` 区分独立列表；连续列表可省略，同组跨普通段落或其他块继续编号。另起列表使用新的 `order_id`。
- `order` 仅用于序列首项起点（默认 1）；后续项省略则递增，显式值须匹配计算结果，不在中途重启。父序列跨子层继续，重新进入子层时另起子序列。
- 完整更新保留 GET 中的 `order_id` / `order`，只改文本时不重排编号。
- 片段为 text/ref/link；style 仅作用于当前片段，不向相邻片段继承。整行同样式须覆盖行内所有片段，纯文本可合并。支持 bold/italic/strikethrough 布尔及七色 `gray/red/orange/yellow/green/blue/purple`；link 整体只设外层 style，text/link 为非空 text/ref 数组，子项无 style、不嵌套 link、不插入媒体。
- 读取无法表达的存量片段时沿用文字占位或过滤，不能视为保留原始片段。
- 本地与服务端校验、`--no-validate` 和完整更新规则见 [命令契约](lark-base-workflow.md#检查与实际运行)。

正文含图片或智能总结时另读 [图片与智能总结引用](#图片与智能总结引用)；引用记录作表格或列表展示时另读 [记录展示规则](#一条消息展示多条记录)。

## 图片与智能总结引用

引用路径分别见 [仪表盘引用](lark-base-workflow-schema.md#仪表盘引用)与 [应用截图输出](lark-base-workflow-schema.md#appscreenshotaction-输出)，引用片段使用 `value_type:ref`。

| 引用 | 消息普通／富文本正文 | 邮件普通／富文本正文 |
|---|---|---|
| 仪表盘截图 | 支持 | 支持 |
| 仪表盘智能总结 | 支持 | 不支持 |
| 应用截图 | 支持 | 不支持；使用 [附件](#附件) |

仪表盘引用须在 `extra_info.dashboard_name` 填准确仪表盘名称，通过 `+dashboard-list` 确认；目标不明确时先澄清。应用截图须引用已有 `AppScreenshotAction`，输入参数见 [应用截图](lark-base-workflow-schema.md#appscreenshotaction)。这些图片与智能总结引用仅适用于 `ReminderTrigger`、`TimerTrigger`、`ButtonTrigger`。

邮件普通或富文本正文不支持应用截图，提交时明确报错并提示使用 `attachments`。保持正文片段顺序。

## 附件

消息与邮件的 `attachments` 为引用数组，每项均为 `ref`。应用截图可作为消息或邮件附件，路径见 [应用截图输出](lark-base-workflow-schema.md#appscreenshotaction-输出)；其他附件引用前序步骤中的附件字段。

## 一条消息展示多条记录

消息普通正文 `content` 可引用 `$.<find_step_id>.fieldRecords`，前置节点必须为同一流程中当前消息之前的 `FindRecordAction`。`extra_info.field_name_list` 可省略；提供时须为非空、不重复的字段名数组，按数组顺序展示，省略则使用查找节点已选择的字段。先通过 `+field-list` 核对真实字段名；消息展示可能受条数上限限制，不能承诺任意数量均能完整展示。普通正文按列表展示；富文本记录展示另见 [富文本正文](#富文本正文)。

汇总成一条消息时不加 Loop；逐条通知时才使用 Loop。作为 Loop 数据源的同一 `fieldRecords` 引用不要求展示字段列表。完整更新按 [命令契约](lark-base-workflow.md#检查与实际运行)处理。

已配置前序 `FindRecordAction`（id 为 `step_find_orders`）后，将下列数组填入消息节点的 `data.content`；接收人等其他字段按消息协议配置：

```json
[
  {"value_type":"text","value":"以下订单需要处理：\n"},
  {"value_type":"ref","value":"$.step_find_orders.fieldRecords","extra_info":{"field_name_list":["客户名称","订单金额","状态"]}}
]
```

- 富文本只有全部记录引用 `$.<find_step_id>.fieldRecords` 可按表格或列表展示：使用 `extra_info.display_type=table/list` 与有序、非空、不重复的 `field_name_list`；省略字段列表时使用查找节点的字段选择。首条记录必须选择具体字段 `$.<find_step_id>.firstfieldsRecord.<field_id>`，字段路径从该节点输出中取得；不能直接插入 `firstfieldsRecord` 整行，即使提供 `field_name_list` 也不支持。用户要求表格时保留 `display_type:"table"`。
- GET 可能为标量字段引用、视图链接和动态链接中的引用附带 `extra_info.table_name` 表名注释和 `extra_info.view_name` 视图名注释，完整更新时原样保留这些字符串注释；`display_type` 和 `field_name_list` 仍仅适用于记录引用。

完整 Workflow 请求结构见 [基础消息示例](lark-base-workflow.md#示例-1-新增记录触发--发送消息)。

## 标题背景色与按钮扩展

以下参数仅用于 `LarkMessageAction`。

| data 字段 | 必填 | 说明 |
|---|---|---|
| `title_template_color` | 否 | 标题背景色：`none` / `blue` / `wathet` / `turquoise` / `green` / `yellow` / `orange` / `red` / `carmine` / `violet` / `purple` / `indigo` / `grey`；新建省略时默认 `purple`，`none` 清除背景。普通/富文本正文均支持；完整更新先 GET，再保留回读字段提交完整配置 |
| `button_config` | 否 | 整组按钮的防重复点击模式与互斥配置，字段见下文 |

基础按钮字段见 [消息参数](lark-base-workflow-schema.md#larkmessageaction)。`btn_list` 中的扩展参数：

| 字段 | 类型 | 说明 |
|---|---|---|
| `btn_action` | string | `addRecord` / `setRecord` / `openLink` / `triggerWorkflow` |
| `style` | string | `default` / `primary` / `danger` |
| `workflow_id` | string | `triggerWorkflow` 的目标 Workflow 稳定 ID；须为同一 Base 内兼容消息按钮来源的 `ButtonTrigger`，不支持 `TimerTrigger` 等其他触发器；先 GET 确认，配置时可保持 disabled |
| `record_type` | string | `setRecord` 引用前序记录时必须为 `Ref`；修改触发记录时省略 |
| `record_info` | object | `setRecord` 的前序记录来源：`{"step_id":"<step_id>"}`；来源为 `FindRecordAction` 时还须传 `"type":"firstRecord"`，只修改首条结果 |
| `not_repeat_click` | boolean | 是否启用防重复点击；仅适用于 `addRecord` / `setRecord` / `triggerWorkflow`，`openLink` 不支持启用防重复点击：`true` 被拒绝，`false` 作为关闭状态被忽略 |

`setRecord` 的目标表由记录来源决定。修改触发记录时省略 `record_type` 和 `record_info`；引用前序记录时传 `record_type:"Ref"` 与 `record_info.step_id`，来源可以是可用的记录触发器、新增/修改记录动作或记录循环的当前项。引用查找记录时须额外传 `record_info.type:"firstRecord"`，不能批量修改查找结果全集。来源必须位于当前消息之前，字段赋值使用该来源表的真实字段名。

`button_config` 的字段：

| 字段 | 类型 | 说明 |
|---|---|---|
| `not_repeat_click_mode` | string | `AllUser`（默认）：点击后对所有人禁用该按钮；`OnlySelf`：仅对点击者禁用。只影响启用了 `not_repeat_click` 的操作按钮 |
| `mutual_exclusion` | boolean | 默认 `false`；`true` 表示整组按钮只允许点击其中一个，须配合 `AllUser` |

仅开启互斥时，才要求至少有一个 `addRecord` / `setRecord` / `triggerWorkflow` 按钮显式设置 `not_repeat_click:true`；单独设置 `OnlySelf` 没有此要求。互斥时，操作按钮的 `not_repeat_click` 不能显式为 `false`，但 `openLink` 的 `false` 会被忽略。更新时保留 GET 中原有的按钮及整组配置；省略 `button_config` 会恢复默认值。
