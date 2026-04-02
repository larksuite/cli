# base +workflow-create

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

在 Base 中创建一个新的自动化工作流。新建后状态为 `disabled`，需调用 `+workflow-enable` 才能启用。

## 推荐命令

```bash
# 内联 JSON（简单工作流）
lark-cli base +workflow-create \
  --base-token BascXxxxxx \
  --json '{"client_token":"1700000000","title":"新订单自动通知","steps":[{"id":"trigger_1","type":"AddRecordTrigger","title":"监控新订单","next":"action_1","data":{"table_name":"订单表","watched_field_name":"订单号"}},{"id":"action_1","type":"LarkMessageAction","title":"发送通知","next":null,"data":{"receiver":[{"value_type":"user","value":"ou_xxxx"}],"send_to_everyone":false,"title":[{"value_type":"text","value":"新订单提醒"}],"content":[{"value_type":"text","value":"收到新订单"}],"btn_list":[]}}]}'

# 从文件读取（推荐用于复杂工作流）
lark-cli base +workflow-create \
  --base-token BascXxxxxx \
  --json @workflow.json
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--base-token <token>` | 是 | 多维表格 Base Token（`Basc` 开头） |
| `--json <body>` | 是 | 工作流 body JSON，包含 `title` 和/或 `steps`；支持 `@path/to/file.json` 从文件读取 |

## 如何从链接中提取参数

用户通常会提供如下 URL：

```
https://example.feishu.cn/base/<base_token>
```

- `--base-token`：取 `/base/` 后面的字符串（`Basc` 开头）

## API 入参详情

**HTTP 方法和路径：**

```
POST /open-apis/base/v3/bases/:base_token/workflows
```

**Path 参数：**

| 参数 | 必填 | 说明 |
|------|------|------|
| `base_token` | 是 | 多维表格 Base Token |

**Request Body（JSON）：**

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `client_token` | string | **是** | 幂等键，每次创建必须传一个唯一随机值（如时间戳 `"1704067200"`），防重复提交，缺失会报错 |
| `title` | string | 否 | 工作流标题 |
| `steps` | WorkflowStep[] | 否 | 步骤列表 |
| `user_id_type` | string | 否 | 用户 ID 类型：`open_id` / `union_id` / `user_id`，默认 `open_id` |

> **步骤数据结构非常复杂，详见 [lark-base-workflow-schema.md](lark-base-workflow-schema.md)**

**Request Body 示例：**
```json
{
  "client_token": "124131231421312312",
  "title": "新订单自动通知",
  "steps": [
    {
      "id": "trigger_1",
      "type": "AddRecordTrigger",
      "title": "监控新订单",
      "next": "action_1",
      "data": {
        "table_name": "订单表",
        "watched_field_name": "订单号"
      }
    },
    {
      "id": "action_1",
      "type": "LarkMessageAction",
      "title": "发送通知",
      "next": null,
      "data": {
        "receiver": [{ "value_type": "user", "value": "ou_xxxx" }],
        "send_to_everyone": false,
        "title": [{ "value_type": "text", "value": "新订单提醒" }],
        "content": [
          { "value_type": "text", "value": "收到新订单，客户：" },
          { "value_type": "ref", "value": { "path": "$.trigger_1.客户名称" } }
        ],
        "btn_list": []
      }
    }
  ]
}
```

## API 出参详情

**Response `data` 字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| `workflow_id` | string | 新建工作流的唯一 ID（`wkf` 开头） |
| `title` | string | 工作流标题 |
| `status` | string | 工作流状态，新建固定为 `disabled` |
| `steps` | WorkflowStep[] | 完整步骤列表 |
| `creator_id` | string | 创建人 OpenID |
| `updater_id` | string | 最后修改人 OpenID |
| `create_time` | number | 创建时间（Unix 秒级时间戳） |
| `update_time` | number | 更新时间（Unix 秒级时间戳） |

## 返回值

```json
{
  "ok": true,
  "data": {
    "workflow_id": "wkfxxxxxx",
    "title": "新订单自动通知",
    "status": "disabled",
    "steps": [...],
    "creator_id": "ou_xxxx",
    "updater_id": "ou_xxxx",
    "create_time": 1704067200,
    "update_time": 1704067200
  }
}
```

## 工作流

> [!CAUTION]
> 这是**写入操作** — 执行前必须向用户确认。

1. 与用户确认 `--base-token` 和工作流定义（`--json` 内容）
2. 对于复杂工作流，建议先将 JSON 写入文件，再用 `@file.json` 传入
3. 执行命令，报告返回的 `workflow_id`（`wkf` 开头）
4. 提示用户：新建工作流初始状态为 `disabled`，需调用 `+workflow-enable --workflow-id <返回的 workflow_id>` 才会生效

## 坑点

- ⚠️ **client_token 必传**：缺失会返回 `[code=800004006] client token is empty`，这不是权限问题，是 JSON body 缺字段。每次请求传唯一值即可（如 `"$(date +%s)"`）
- ⚠️ **新建后默认禁用**：`status` 固定返回 `disabled`，需要额外调用 `+workflow-enable` 才能让工作流生效；不要误报"创建成功即启用"
- ⚠️ **steps 中 id 字段必须唯一**：每个步骤的 `id` 由调用方指定，且在工作流内必须唯一；`next` 和 `children.links[].to` 引用的 ID 必须在同一 steps 数组中存在，否则服务端返回 `[2200] Internal Error`
- ⚠️ **@file 路径限制**：`--json @workflow.json` 会读取文件内容，复杂 workflow 强烈建议用文件而不是命令行内联。CLI 强制要求相对路径（如 `@./workflow.json`），绝对路径（包括 `/tmp/xxx` 和 `/Users/.../xxx`）会被拒绝
- ⚠️ **权限不足**：如遇 `permission denied`，先确认当前身份（bot 或 user）是否对该 Base 有编辑权限，再检查 scope 是否已开通。参考 [lark-shared](../../lark-shared/SKILL.md) 中的权限不足处理流程
- ⚠️ **user_id_type**：涉及用户的 `value_type: "user"` 的 value 字段传 OpenID，服务端会根据 `user_id_type`（默认 `open_id`）解析；如需传 `user_id` 格式需在 body 里显式声明 `"user_id_type": "user_id"`

## 参考

- [lark-base-workflow-schema.md](lark-base-workflow-schema.md) — 完整 Workflow 数据结构
- [lark-base-workflow-update](lark-base-workflow-update.md) — 全量更新工作流
- [lark-base-workflow-enable](lark-base-workflow-enable.md) — 启用工作流
- [lark-base-workflow-list](lark-base-workflow-list.md) — 列出全部工作流
- [lark-base](../SKILL.md) — 多维表格全部命令
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
