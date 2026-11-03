# AI Agent 工作流节点

用于在工作流运行时理解输入、生成文本或结构化结果，并按需调用工具。创建或更新仍使用 `base +workflow-create/+workflow-update`；节点选择见 [场景速查表](lark-base-workflow.md#场景速查表)。

## 节点关系

- 根节点 `AIAgentAction` 的 `data.model` 管模型，`data.memory` 管记忆。不要创建 `AIAgentLLMAction` / `AIAgentMemoryAction` 独立步骤。
- 工具是 `steps` 中的独立步骤，通过根节点 `children.links` 的 `{"kind":"slot","label":"tool","to":"工具步骤ID"}` 挂载。
- 当前转换器要求主链末节点位于 `steps` 数组末尾；工具步骤放在其前面，不能直接追加到末节点之后。
- 工具步骤没有 `next`；Agent 完成后的动作挂在根节点 `next`。工具由 Agent 按需调用，不保证按 links 顺序执行，也不保证每个工具都会被调用。
- 当前对话模型直接搜索或写记录，不能代替工作流里的 Agent 调用工具。
- 通过上游引用或已挂载工具接入任务所需的原始内容；对象名称、标识不能代替待分析内容。数据范围、字段和引用须匹配真实表结构。
- `get` 返回的 `llm/memory` 空链接（`to:""`）可以保留；不要填入不存在的内部节点 ID。同一工具只挂一个 Agent。

## 根节点 data

公开 CLI 字段使用 snake_case；引用字符串中的 ID 和输出路径保持原样。

| 字段 | 说明 |
|---|---|
| `prompt` | 输入指令，text/ref 片段数组；执行前应有明确内容 |
| `system_message` | 系统设定，同样为 text/ref 数组 |
| `model.name` | 模型 pack ID，例如 `doubao_chat_model`；可用模型从现有工作流配置或服务端可用配置确认，不能凭显示名编造 |
| `model.temperature` | 数字；允许范围由服务端当前模型配置决定 |
| `model.max_token` | 正整数；上限随模型变化 |
| `model.timeout` | 正整数，单位毫秒；不要把秒直接填入 |
| `model.thinking_mode` | 可选：本地只接受 `auto`（自动／智能匹配）、`quick`（高效执行／即刻执行）、`deep`（探索分析／深入思考）；各模型支持范围由服务端校验 |
| `max_iterations` | 最大迭代次数，正整数；上限由服务端决定 |
| `enable_streaming_reply` | 布尔值，是否流式回复；不能替代下游记录或消息动作 |
| `output_format_type` | `text` / `json` / `form` |
| `json_output` | JSON 输出示例的字符串；例如 `"{\"摘要\":\"文本\",\"数量\":0}"` |
| `form_output` | 结构化字段数组，每项 `{name,type,description?}`；type 为 `string/number/boolean/timestamp/fileInfo` |
| `memory.history_rounds` | 记忆轮数，正整数。仅 `LarkMessageTrigger` 生效；其他触发器不要填写 |

注意：

- 省略 `thinking_mode` 时使用模型默认模式；不要传空字符串或 `null`。不支持所选模式时按错误提示修正或报告限制，不自动替换。
- `json_output` 填预期结果形状的 JSON 示例字符串，不接受 JSON Schema。字段名、数组结构和标量类型决定下游可引用路径。
- 指定 JSON 模式时必须同时填写 `json_output`；指定 form 模式时填写非空 `form_output`。不要同时填写多种输出配置。
- 修改已有流程先完整 get，保留模型、输出配置和工具选择。不要因只改 prompt 而丢失其他参数；启停仍用专用命令并回读。
- 模型未指定时不要假定服务端自动选中可运行模型；创建可运行的新 Agent 时显式填写已确认的 `model.name`。

## MCP 工具包

当前仅支持下表 5 个 MCP 包。工具步骤类型统一为 `AIAgentMCPAction`，用 `meta.component_id` 选包。

| 能力 | component_id | 已知常用 tool_list 选择 |
|---|---|---|
| 联网搜索 | `search_mcp` | `["web_search"]` |
| 读取 URL | `link_reader_mcp` | `["link_reader"]` |
| 多维表格 | `feishu_base_mcp` | `["add_base_record","update_base_record"]` |
| 飞书用户和云文档 | `openplatform_mcp` | 按需选择 `search-user/get-user/search-doc/fetch-doc/create-doc/update-doc/list-docs/fetch-file/get-comments/add-comments` |
| 豆包联网搜索 | `doubao_general_search_mcp` | `["GeneralSearch"]`；可能消耗相应额度 |

这 5 类预置 MCP 使用服务提供的工具描述，`data.tool_info.description` 不参与编译；调用时机和业务约束写入 Agent 的 `prompt`。

这是包边界和常用选择，不是包内工具的永久全集。服务端通过 `GetMCPInfo` 获取真实可用工具并校验；新增工具只能使用已确认的真实 key。当前 CLI 没有专用工具目录查询命令，不要虚构 `+workflow-meta` 等接口。

| MCP data 字段 | 说明 |
|---|---|
| `tool_list` | 工具 key 数组，大小写敏感。省略采用服务端默认选择；`[]` 表示显式空选择，两者不同。优先显式列出任务需要的工具 |
| `identity_type` | `maker` / `triggerPersonal`。按钮、飞书消息触发才支持触发人身份；其他触发器使用 `maker` |
| `env` | 字符串键值配置。只设置工具确实需要的配置，避免把凭证写入工作流、prompt 或报告 |

配置读写指定数据表的 MCP 工具时，先查询并确认 Base URL、`table_id` 和字段信息，再写入 Agent 的 prompt 或上游输入。只写表名不能代替 `table_id`；不要让 Agent 猜 ID。

`openplatform_mcp` 需要所选运行身份对当前 Base 完成飞书 MCP 授权。遇到 `UAT is empty` 时提示完成授权后再运行；CLI 已登录不代表该授权已完成，不要把登录凭证填进 `env` 或 prompt。

不支持：自定义 MCP、`feishu_doc_mcp`、`doubao_fetch_mcp` 等未列出的包。不要自动替换成相近工具；先说明差异。

## 普通 Action 工具

复用对应 Action 的 `data` 协议，并通过 slot/tool 挂载；需要延迟时，将 `Delay` 放在工作流主链中：

| 类型 | 用途 |
|---|---|
| `FindRecordAction` | 查找记录 |
| `AddRecordAction` / `SetRecordAction` | 新增 / 修改记录 |
| `LarkMessageAction` | 发送飞书消息 |
| `LarkEmailAction` | 发送飞书邮件；见 [节点参数](lark-base-workflow-schema.md#larkemailaction) |
| `HTTPClientAction` | HTTP 请求 |
| `GenerateAiTextAction` | AI 生成文本；注意 `Ai` 大小写 |
| `AIAnalysisAction` | AI 分析 |
| `APIHubAction` | 仅 [已支持的 9 个二方动作](lark-base-workflow-apihub.md) |

当前边界：

- 普通 Action 的参数仍须符合该节点协议；当前不能通过留空字段或伪造“AI 自动填写”标记，假定 Agent 会补齐。
- 邮件作为工具时可配置 `data.tool_info.description`。不要把 Agent 最终输出引用回它自己的工具；需要发送最终结果时，将邮件作为 Agent 后续普通节点。

## 工具调用说明

普通 Action 和已支持的 APIHub 工具可在 `data.tool_info.description` 中说明调用时机，与该工具的业务参数并列：

```json
{"tool_info":{"description":"需要查询当前库存时调用。"}}
```

- 描述用于帮助 Agent 选工具，不会替代业务参数、授权或强制执行工具。
- `+workflow-update` 提交完整 steps：先 get，保留各工具原有 `tool_info`；传 `{"description":"新说明"}` 替换，传 `null` 清除。不要通过省略字段来保留原描述。空字符串会原样保存，是否允许启用由服务端校验。
- 只配置 `description`；不要传内部 `AIData/ai_data`、`description_is_user_change` 或 `AIInferList`。`title`、链接 `desc` 和 `meta.op_desc` 不能代替工具描述。

## 输出与引用

| 输出配置 | 下游引用 |
|---|---|
| text | 整体字符串：`$.agent_id` |
| json object | 按示例字段：`$.agent_id.摘要` |
| json 对象内的数组字段 | 例如输出 `{"items":[{"名称":"文本"}]}`，Loop 引用 `$.agent_id.items`，循环内用 `$.loop_id.item.名称` |
| form | 按配置字段名引用：`$.agent_id.字段名` |

当前顶层 JSON 数组的根引用被识别为 object，不能直接把 `$.agent_id` 作为 Loop 数据源；使用上面的对象包装写法。

不要引用内部 LLM/记忆节点。下游业务动作应优先消费 Agent 的最终输出，而不是假定某个工具一定调用过并引用它的输出。

## 示例：联网整理输入，写入结果表

先查询真实表名和字段 ID，将示例中的表名、字段 ID 替换后提交。`client_token` 每次新建使用新值。

```json
{
  "client_token": "new-unique-request-id",
  "title": "联网整理后写入结果",
  "steps": [
    {
      "id": "input", "type": "AddRecordTrigger", "title": "新增待研究主题",
      "next": "agent",
      "data": {"table_name": "输入表", "watched_field_name": "主题"}
    },
    {
      "id": "agent", "type": "AIAgentAction", "title": "联网整理",
      "next": "save",
      "children": {"links": [{"kind": "slot", "label": "tool", "to": "search"}]},
      "data": {
        "prompt": [
          {"value_type": "text", "value": "调用联网搜索核实以下主题，输出摘要和来源链接："},
          {"value_type": "ref", "value": "$.input.fld真实主题字段ID"}
        ],
        "system_message": [{"value_type": "text", "value": "只基于查到的结果回答，查询失败明确说明。"}],
        "model": {"name": "doubao_chat_model"},
        "max_iterations": 5,
        "enable_streaming_reply": false,
        "output_format_type": "json",
        "json_output": "{\"摘要\":\"文本\",\"来源\":\"URL\"}"
      }
    },
    {
      "id": "search", "type": "AIAgentMCPAction", "title": "联网搜索",
      "meta": {"component_id": "search_mcp"},
      "data": {"tool_list": ["web_search"], "identity_type": "maker"}
    },
    {
      "id": "save", "type": "AddRecordAction", "title": "保存结果",
      "data": {
        "table_name": "结果表",
        "field_values": [
          {"field_name": "摘要", "value": [{"value_type": "ref", "value": "$.agent.摘要"}]},
          {"field_name": "来源", "value": [{"value_type": "ref", "value": "$.agent.来源"}]}
        ]
      }
    }
  ]
}
```

## 保存和检查

1. 保存前可用 `--dry-run` 检查请求；它只验证 CLI 配置，不代表服务端接受或实际执行。
2. 按用户要求 create/update 后，用 `+workflow-get` 核对模型、prompt 引用及所需数据来源、输出格式、工具组件和选择；启停按 [Workflow 入口](lark-base-workflow.md) 确定的目标运行态处理并回读。
3. 用户要求验证实际执行时，按 [检查与实际运行](lark-base-workflow.md#检查与实际运行) 的授权范围触发，检查下游输出；验证记忆时使用同一会话的多轮输入。
4. 核验工具调用优先查看已有调用记录及对应资源变化；仅有最终文本或模型声称“已搜索”不能证明工具已调用。说明已确认的结果及未验证部分，不为补齐证据自行新增业务操作。
