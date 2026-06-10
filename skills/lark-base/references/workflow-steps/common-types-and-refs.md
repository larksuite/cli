# Workflow common types and refs

### ValueInfo

所有值的基础类型，通过 `value_type` 区分：

| value_type | value 类型 | 说明 | 示例 |
|------------|-----------|------|------|
| `text` | string | 文本 | `"张三"` |
| `number` | number | 数字 | `100` |
| `boolean` | boolean | 布尔值 | `true` |
| `date` | string | 日期，可以是具体时间字符串，或者相对时间值 | `"2025/01/01"`、`"2025/01/01 11:00"`、`"now"`、`"now 11:00"`、`"today"`、`"today 11:00"`、`"yesterday"`、`"yesterday 11:00"`、`"lastWeek"`、`"currentMonth"`、`"lastMonth"`、`"theLastWeek"`、`"theNextWeek"`、`"theLastMonth"`、`"theNextMonth"` |
| `option` | `{ id, name }` | 选项 | `{ "id": "opt1", "name": "已完成" }` |
| `link` | `{ text, link }` | 链接（含文字和 URL）， 文字和 URL 的格式可以是 ValueInfo 中的 text/ref 类型 | `{ "text": [{ "value_type": "text", "value": "查看详情" }], "link": [{ "value_type": "text", "value": "https://example.com" }] }`、`{ "text": [{ "value_type": "text", "value": "查看详情" }], "link": [{ "value_type": "ref", "value": "$.step_1.fldXXX" }] }` |
| `user` | `{ id, name }` | 用户 OpenID、名字 | `{ "id": "ou_xxxx", "name": "张三" }` |
| `group` | `{ id, name }` | 群 Chat ID、名字 | `{ "id": "oc_xxx", "name": "测试群" }` |
| `ref` | `string` | 引用前置节点输出的路径 | 参考 ref 引用变量详解 章节 |

> ⚠️ **所有涉及用户的 value 中的 id 统一使用 OpenID（`ou_xxxx` 格式）**，由 CLI 层来完成转换
> ⚠️ **所有涉及群的 value 中的 id 统一使用 ChatID（`oc_xxxx` 格式）**，由 CLI 层来完成转换

### ref 引用变量详解

`ref` 类型是工作流中节点间数据传递的核心机制。当 `value_type` 为 `ref` 时，`value` 指向前置节点的某个输出变量。本节详细描述每个节点可供引用的输出变量定义。

#### 引用路径格式

```
$.{stepId}
$.{stepId}.{pathId}
$.{stepId}.{pathId}.{childPathId}
$.{stepId}.{pathId}.{childPathId}.{grandChildPathId}
```

- `{stepId}`：前置节点的 `id`（即 WorkflowStep 中的 `id` 字段）
- `{pathId}`：节点输出的路径标识符
- 支持多层下钻，如引用字段的属性：`$.step_1.fldXXX.name`

---

#### 触发器节点输出

##### 记录触发器（AddRecordTrigger / ChangeRecordTrigger / SetRecordTrigger / ReminderTrigger）

这 4 个触发器的输出结构完全一致：

| pathId | 说明 | 引用示例 |
|--------|------|----------|
| `{fieldId}` | 字段id，从配置表的所有字段或者指定字段id生成，可下钻字段属性 | `$.{stepId}.{fieldId}` |
| `{fieldId}.fieldId` | 字段id属性 | `$.{stepId}.{fieldId}.fieldId` |
| `{fieldId}.fieldName` | 字段名属性 | `$.{stepId}.{fieldId}.fieldName` |
| `startTime` | 触发时间戳 | `$.{stepId}.startTime` |
| `recordId` | 记录 ID | `$.{stepId}.recordId` |
| `recordLink` | 记录链接 | `$.{stepId}.recordLink` |
| `recordCreatedUser` | 记录创建者 | `$.{stepId}.recordCreatedUser` |
| `recordCreatedTime` | 记录创建时间 | `$.{stepId}.recordCreatedTime` |
| `recordModifiedUser` | 最后修改者 | `$.{stepId}.recordModifiedUser` |
| `recordModifiedTime` | 最后修改时间 | `$.{stepId}.recordModifiedTime` |

**动态字段输出规则**：

- 读取触发器所配置的数据表的所有字段
- 每个字段生成一条输出：`pathId` = fieldId
- 若字段为关联字段，children 为关联表所有字段（单层下钻，不再递归）
- 每个字段可下钻特定的字段属性（见「字段属性下钻」）

**recordLink 的 children**：如果配置了数据表，则为该表所有视图的列表，每个视图 `{ pathId: viewId, pathName: viewName, pathType: 'string' }`。引用示例：`$.{stepId}.recordLink.{viewId}`。

##### ButtonTrigger（按钮触发器）

`ButtonTrigger` 的输出取决于 `button_type`：

#### `button_type = buttonField`

| pathId | 说明 | 引用示例 |
|--------|------|----------|
| `{fieldId}` | 字段id，从配置表的所有字段或者指定字段id生成，可下钻字段属性 | `$.{stepId}.{fieldId}` |
| `{fieldId}.fieldId` | 字段id属性 | `$.{stepId}.{fieldId}.fieldId` |
| `{fieldId}.fieldName` | 字段名属性 | `$.{stepId}.{fieldId}.fieldName` |
| `recordId` | 记录 ID | `$.{stepId}.recordId` |
| `recordLink` | 记录链接 | `$.{stepId}.recordLink` |
| `recordCreatedUser` | 记录创建者 | `$.{stepId}.recordCreatedUser` |
| `recordModifiedUser` | 最后修改者 | `$.{stepId}.recordModifiedUser` |
| `recordModifiedTime` | 最后修改时间 | `$.{stepId}.recordModifiedTime` |
| `time` | 触发时间 | `$.{stepId}.time` |
| `user` | 触发人 | `$.{stepId}.user` |
| `buttonName` | 触发的按钮名称 | `$.{stepId}.buttonName` |

#### `button_type = buttonElement`

| pathId | 说明 | 引用示例 |
|--------|------|----------|
| `time` | 触发时间 | `$.{stepId}.time` |
| `user` | 触发人 | `$.{stepId}.user` |
| `buttonName` | 触发的按钮名称 | `$.{stepId}.buttonName` |

##### TimerTrigger（定时触发器）

| pathId | 说明 | 引用示例 |
|--------|------|----------|
| `scheduleTime` | 定时触发时间 | `$.{stepId}.scheduleTime` |

##### LarkMessageTrigger（飞书消息触发器）

| pathId | 说明 | 引用示例 |
|--------|------|----------|
| `Sender` | 消息发送者 | `$.{stepId}.Sender` |
| `AtUser` | 消息中被@的用户 | `$.{stepId}.AtUser` |
| `SenderGroup` | 消息所在群（仅群聊场景） | `$.{stepId}.SenderGroup` |
| `MessageSendTime` | 消息发送时间 | `$.{stepId}.MessageSendTime` |
| `MessageContent` | 消息正文 | `$.{stepId}.MessageContent` |
| `MessageType` | 消息类型标识 | `$.{stepId}.MessageType` |
| `MessageID` | 消息唯一标识 | `$.{stepId}.MessageID` |
| `MessageLink` | 消息链接（仅群聊场景） | `$.{stepId}.MessageLink` |
| `ParentID` | 回复的消息 ID | `$.{stepId}.ParentID` |
| `ThreadID` | 所在话题消息 ID | `$.{stepId}.ThreadID` |
| `Attachments` | 消息中的附件 | `$.{stepId}.Attachments` |

条件限制：

- 若场景为单聊（`receive_scene = "Chat"`），则 `SenderGroup` 和 `MessageLink` 不可用

---

#### 操作节点输出

##### FindRecordAction（查找记录）

| pathId | 说明 | 引用示例|
|--------|------|-------|
| `fieldRecords` | 所有找到的记录的引用（可用于 Loop 遍历） | `$.{stepId}.fieldRecords`|
| `firstfieldsRecord` | 第一条匹配记录 | `$.{stepId}.firstfieldsRecord`|
| `firstfieldsRecord.{fieldId}` | 首条记录的字段值，可下钻字段属性 | `$.{stepId}.firstfieldsRecord.{fieldId}`|
| `firstfieldsRecord.recordId` | 记录 ID 数组 | `$.{stepId}.firstfieldsRecord.recordId`|
| `fields` | 查找到的所有记录某列值 | 不支持引用|
| `fields.{fieldId}` | 用户选择的字段 | `$.{stepId}.fields.{fieldId}`|
| `fields.{fieldId}.fieldId` | 用户选择的字段id数组 | `$.{stepId}.fields.{fieldId}.fieldId`|
| `fields.{fieldId}.fieldName` | 用户选择的字段名数组 | `$.{stepId}.fields.{fieldId}.fieldName`|
| `fields.recordId` | 记录 ID 数组 | `$.{stepId}.fields.recordId`|
| `recordNum` | 找到记录总数 | `$.{stepId}.recordNum`|

##### AddRecordAction（新增记录）

| pathId | 说明 | 引用示例 |
|--------|------|----------|
| `{fieldId}` | 用户配置的字段值，可下钻字段属性 | `$.{stepId}.{fieldId}` |
| `{fieldId}.fieldId` | 用户配置的字段id | `$.{stepId}.{fieldId}.fieldId` |
| `{fieldId}.fieldName` | 用户配置的字段名 | `$.{stepId}.{fieldId}.fieldName` |
| `recordId` | 新增的记录 ID | `$.{stepId}.recordId` |
| `recordLink` | 新增的记录 URL | `$.{stepId}.recordLink` |

##### SetRecordAction（更新记录）

| pathId | 说明 | 引用示例 |
|--------|------|----------|
| `{fieldId}` | 用户配置的字段值，可下钻字段属性 | `$.{stepId}.{fieldId}` |
| `{fieldId}.fieldId` | 用户配置的字段id | `$.{stepId}.{fieldId}.fieldId` |
| `{fieldId}.fieldName` | 用户配置的字段名 | `$.{stepId}.{fieldId}.fieldName` |
| `recordId` | 记录 ID 数组（因可能更新多条记录） | `$.{stepId}.recordId` |

##### HTTPClientAction（HTTP 请求）

HTTPClientAction 的输出取决于 `response_type`：

| response_type | 是否可引用 | 输出说明 | 引用示例 |
|--------------|-----------|----------|----------|
| `none` | 否 | 无任何可引用输出 | 不支持引用 |
| `text` | 是 | 整个响应文本作为节点整体输出 | `$.{stepId}` |
| `json` | 是 | 响应体整体挂在 `body` 下，同时返回 `status_code`；仅可引用 `response_value` 中声明的字段 | `$.{stepId}.body`、`$.{stepId}.body.success`、`$.{stepId}.body.message`、`$.{stepId}.status_code` |

**补充说明**：

- 当 `response_type = none` 时，后续节点无法引用 HTTPClientAction 的任何输出
- 当 `response_type = text` 时，`$.{stepId}` 表示整个响应文本
- 当 `response_type = json` 时，`$.{stepId}.body` 表示整个 JSON body，`$.{stepId}.body.字段名` 表示 body 中某个字段
- 仅当 `response_type = json` 时，`$.{stepId}.status_code` 表示请求该 HTTP URL 后返回的 HTTP 状态码
- 仅当 `response_type = json` 时，`response_value` 必填
- 当 `response_type = json` 时，后续节点只能引用 `response_value` 中声明过的字段

**案例**：

假设某个 `HTTPClientAction` 的配置如下：

```json
{
  "id": "step_http_1",
  "type": "HTTPClientAction",
  "data": {
    "response_type": "json",
    "response_value": "{\"success\":true,\"message\":\"ok\"}"
  }
}
```

则后续节点仅可以引用：

- `$.step_http_1.body`
- `$.step_http_1.body.success`
- `$.step_http_1.body.message`
- `$.step_http_1.status_code`

但**不能**引用未在 `response_value` 中声明的字段，例如：

- `$.step_http_1.body.data`
- `$.step_http_1.body.request_id`

##### GenerateAiTextAction（AI 生成文本）

| pathId | 说明 | 引用示例 |
|--------|------|----------|
| （整体出参） | AI 生成的文本内容（不支持下钻，只能引用 `$.{stepId}`） | `$.{stepId}` |

##### 无输出的操作节点

以下节点不产生任何可引用的输出数据：

- **Delay**（延时等待）
- **LarkMessageAction**（发送飞书消息）

---

#### 分支节点输出

以下分支节点均不产生任何可引用的输出数据：

- **IfElseBranch**（条件分支）
- **SwitchBranch**（多条件分支）

---

#### 系统节点输出

##### Loop（循环）

| pathId | 说明 | 引用示例 |
|--------|------|----------|
| `item` | 当前循环元素 | `$.{stepId}.item` |
| `index` | 从 0 开始的循环索引 | `$.{stepId}.index` |

**`item` 的类型推断规则**（由循环数据源决定）：

**场景一：遍历组合记录** — 数据源为 `record` 类型时（如 FindRecordAction 的 `fieldRecords`），`item` 类型为 `record`，可向下选择具体字段：

| 说明 | 引用示例 |
|------|----------|
| 当前遍历的记录（record） | `$.{loopStepId}.item` |
| 记录的具体字段 | `$.{loopStepId}.item.{fieldId}` |
| 从 0 开始的索引（number） | `$.{loopStepId}.index` |

**场景二：遍历字段** — 数据源为某个多值类型字段时，比如附件字段、人员字段，`item` 继承该字段的类型并可继续下钻字段属性：

| 说明 | 引用示例 |
|------|----------|
| 当前遍历的元素（类型继承数据源字段类型，例如人员字段） | `$.{loopStepId}.item` |
| 用户姓名 | `$.{loopStepId}.item.name` |
| 从 0 开始的索引（number） | `$.{loopStepId}.index` |

---

#### 字段属性下钻

每个字段变量都可以进一步下钻选择字段的属性。所有字段至少支持 `fieldId` 和 `fieldName` 两个基础属性，部分字段还支持额外属性：

| 字段类型 | 属性名称 | 属性 pathId | 属性 pathType | 说明 |
|----------|---------|-------------|--------------|------|
| **所有字段（基础）** | 字段 ID | `fieldId` | `string` | 字段的唯一标识 |
| | 字段名称 | `fieldName` | `string` | 字段的显示名称 |
| **人员字段**（`user` / `created_by` / `updated_by`） | 姓名 | `name` | `string` | 用户姓名 |
| **日期字段**（`datetime` / `created_at` / `updated_at`） | 时间戳 | `timestamp` | `number` | 时间戳数值 |
| **附件字段**（`attachment`） | 文件名 | `fileName` | `string` | 附件文件名 |
| | 文件类型 | `fileType` | `string` | MIME 类型 |
| | 文件大小 | `size` | `number` | 文件字节数 |
| | 文件 Token | `fileToken` | `string` | 附件 token |
| **超链接文本字段**（`text` 且 `style.type=url`） | 文本 | `text` | `string` | 链接文本部分 |
| | 链接 | `link` | `string` | 链接 URL 部分 |
| **自动编号字段**（`auto_number`） | 序号 | `sequence` | `number` | 编号的纯数字序号 |
| **关联字段**（`link`） | 字段下钻 | `{fieldId}` | - | 可下钻到关联表的字段 |

> 其他字段类型（如 `text`、`number`、`checkbox`、`select`、`location`、`formula`、`lookup` 等）仅支持 `fieldId` 和 `fieldName` 两个基础属性。

下钻引用示例：

```
$.{stepId}.{fieldId} → 字段值本身
$.{stepId}.{fieldId}.fieldId → 字段 ID（string）
$.{stepId}.{fieldId}.fieldName    → 字段名称（string）
$.{stepId}.{fieldId}.name → 人员姓名列表（array<string>，仅人员字段）
$.{stepId}.{fieldId}.unionId → 人员 unionId 列表（array<string>，仅人员字段）
$.{stepId}.{fieldId}.timestamp    → 时间戳（array<number>，仅日期字段）
$.{stepId}.{fieldId}.fileName     → 文件名列表（array<string>，仅附件字段）
$.{stepId}.{fieldId}.fileToken    → 文件 Token 列表（array<string>，仅附件字段）
```

---

#### 节点输出能力总览

| 节点 | 类型 | 有输出 | 输出特性 |
|------|------|--------|---------|
| AddRecordTrigger | 触发器 | ✅ | 动态（表字段 + 记录属性） |
| ChangeRecordTrigger | 触发器 | ✅ | 动态（表字段 + 记录属性） |
| SetRecordTrigger | 触发器 | ✅ | 动态（表字段 + 记录属性） |
| ReminderTrigger | 触发器 | ✅ | 动态（表字段 + 记录属性） |
| ButtonTrigger | 触发器 | ✅ | 动态（表字段 + 记录属性；buttonElement 仅基础触发属性） |
| TimerTrigger | 触发器 | ✅ | 静态（仅 scheduleTime） |
| LarkMessageTrigger | 触发器 | ✅ | 静态（消息属性列表） |
| FindRecordAction | 动作 | ✅ | 动态（用户选择的字段） |
| AddRecordAction | 动作 | ✅ | 动态（用户配置的字段） |
| SetRecordAction | 动作 | ✅ | 动态（用户配置的字段） |
| HTTPClientAction | 动作 | ✅ | 动态（取决于用户配置的 HTTP 响应输出） |
| GenerateAiTextAction | 动作 | ✅ | 静态（单 string） |
| Delay | 动作 | ❌ | 无输出 |
| LarkMessageAction | 动作 | ❌ | 无输出 |
| IfElseBranch | 分支 | ❌ | 无输出 |
| SwitchBranch | 分支 | ❌ | 无输出 |
| Loop | 系统 | ✅ | 动态（取决于数据源） |

---

### TextRefItem

文本与引用混排，用于消息内容等动态拼接场景：

```json
[
  { "value_type": "text", "value": "客户 " },
  { "value_type": "ref", "value": "$.step_1.fieldxxx" },
  { "value_type": "text", "value": " 创建了新订单" }
]
```

### RecordFieldValue

```json
{ "field_name": "客户名称", "value": [{ "value_type": "text", "value": "张三" }] }
```

### AndCondition（Trigger 过滤条件）

```json
{
  "conjunction": "and",
  "conditions": [
    { "field_name": "状态", "operator": "is", "value": [{ "value_type": "text", "value": "进行中" }] }
  ]
}
```

### OrGroup（Branch 分支条件）

```json
{
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
```

**operator 可选值：** `is` / `isNot` / `containsAny` / `doesNotContainAny` / /`containsAll`/ `isEmpty` / `isNotEmpty` / `isGreater` / `isGreaterEqual` / `isLess` / `isLessEqual`

### RecordFilterInfo
** 由于 conjunction 只支持 and，若需要实现 字段X 等于 A 或 B，你可以使用 containsAny
```json
{
  "conjunction": "and",
  "conditions": [
    { "field_name": "状态", "operator": "is", "value": [{ "value_type": "text", "value": "进行中" }] }
  ]
}
```

### `select` 字段多值匹配

| 操作 | operator | 正确写法 |
|------|---------|---------|
| 等于单个值 | `is` | `[{"value_type": "option", "value": {"name": "L2"}}]` |
| 匹配多个值（L2 或 L3） | `containsAny` | `[{"value_type": "option", "value": {"name": "L2"}}, {"value_type": "option", "value": {"name": "L3"}}]` |

> ⚠️ 不要用多个 `is` 条件（会被当作 OR，无法实现 AND）。推荐使用 `containsAny` 操作符匹配多个值。

> ⚠️ **Select 字段条件**：`value_type` 必须为 `option`，`value` 对象可只传 `name`（如 `{"name": "L2"}`），无需提供选项 ID。

### RefInfo

```json
{ "step_id": "step_trigger" }
```

---

返回 [Workflow schema index](../lark-base-workflow-schema.md)。
