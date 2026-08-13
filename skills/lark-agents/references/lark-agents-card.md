# agents card

> **前置条件：** 先读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)（认证、身份、安全规则）。

取并展示一个 agent 的能力卡片：`capabilities`（能调哪些动词）、`has_parameters`（哪些动词要先查参数）、`identity`（支持的 `--as` 及前置条件）；配合 `--operation <动词|all>` 查某动词的完整参数契约。**调任何动词前先读 card**——这是决定"能调什么、要传什么"的唯一依据（`capabilities` 为准，别按文档示例假设某能力一定存在）。card 是否本地合成（离线可用）是 provider 事实，见对应 provider 文件。只读。

> card **不含 scope 声明**——scope 是内部注册项，只喂给 preflight。user 身份缺 scope 时命令会本地报 `missing_scope`（照抄 hint 一次配齐）；scope 全集见对应 provider 文件，通用流程见 [lark-agents 前置准备](../SKILL.md)。

## 命令

```bash
# 默认 JSON 信封（程序化解析用这个）
lark-cli agents card <provider>:<agent_id> --format json

# 人类可读
lark-cli agents card <provider>:<agent_id> --format pretty

# 只取 capabilities
lark-cli agents card <provider>:<agent_id> --jq '.data.capabilities'

# 查某动词的参数契约（name/type/required/enum/default + 命令形态）
lark-cli agents card <provider>:<agent_id> --operation send

# 一次拿全所有动词的参数契约（要调 2+ 个动词时省往返）
lark-cli agents card <provider>:<agent_id> --operation all
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `<agent_ref>` | 是 | `<provider>:<agent_id>` |
| `--operation <动词\|all>` | 否 | 参数契约子查询；合法动词 8 个 = 7 个操作型 capability 键 + `send`（`file_input`/`input_required` 是行为位、不是动词），拼错报 `invalid_argument` 并列出全集 |
| `--format json\|pretty` | 否 | 默认 `json`；`--jq` 会强制 JSON；其余值报 `invalid_argument` |
| `--as user\|bot` | 否 | 身份 |

## 输出

示例（base，真实输出，`agents card base:assistant --as user`）：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "provider": "base",
    "provider_label": "Base Assistant",
    "agent_id": "assistant",
    "brand": "feishu",
    "name": "Base Assistant",
    "description": "Handles multi-component Base construction and restructuring, plus user-facing data retrieval and analysis. Use Base CLI shortcuts for a single atomic edit or record create, update, or delete.",
    "capabilities": {
      "artifact_download": false,
      "context_delete": true,
      "context_get": true,
      "context_list": true,
      "file_input": false,
      "input_required": true,
      "task_cancel": true,
      "task_get": true,
      "task_list": true
    },
    "identity": [
      { "type": "user" }
    ],
    "has_parameters": [
      "send", "task_get", "task_list", "task_cancel",
      "context_list", "context_get", "context_delete"
    ],
    "agent_id_source": "Use the fixed agent reference base:assistant",
    "skills": [
      {
        "id": "base_assistant",
        "name": "Build and analyze a Base",
        "examples": [
          "Create an order table from the provided field list",
          "Build a sales management workflow and dashboard",
          "Analyze recent sales trends and explain the main changes"
        ]
      }
    ]
  }
}
```

## 字段语义与消费方式

- **`capabilities`**：9 键能力矩阵 = 7 个操作位（对应可调动词）+ 2 个行为位（`file_input`/`input_required`，不是动词）。为 `false` 的动词不要调——如 `task_cancel=false` 时 `agents task cancel` 直接报 `unsupported_capability`（exit 2），不发请求。`input_required=false` = 该 agent 不会进 `input_required` 态（追问的实际行为见 provider 文件）。`--dry-run` 是客户端行为，不在 capabilities 里，永远可用。
- **`identity`**：支持的 `--as` 身份；带 `precondition` 的身份要先满足前置条件（典型是渠道白名单，见 provider 文件）。
- **`has_parameters`**：需要带 `--param` 的动词列表（如 `["send","task_list"]`）。不在列表里的动词零参数、直接调；在列表里的先用 `--operation <动词>` 查明细。空数组 = 全部动词都不需要参数。
- **object 参数（`type:"object"`）**：带 `fields` 数组（每个字段又是一份完整声明：type/required/enum/default/min/max）。传参按点路径逐字段：`--param filter.region=east`；或 JSON 整值兜底：`--param filter='{"region":"east"}'`——两通道等价（同一对象不可混用），必填/默认值都声明在字段上。
- **`no_carry: true`**：该参数不入 meta.next 链传（每次调用应给新值，如调用链标记）；必填的 no_carry 参数在 next 命令里以 `<占位符>` 提醒填新值。
- **`--operation <动词|all>`**：参数契约子查询。`agents card <ref> --operation send` 返回 `{operation, supported, command, parameters:[{name,type,required,desc,enum?,default?,min?,max?}]}`——`command` 是该动词的命令形态（含 `<...>` 占位，照着替换）；`parameters:[]` = 该动词无参数；`supported:false` = 该 agent 未实现此动词。`--operation all` 返回 `operations` 全映射（要调多个动词时用它省往返）。动词拼错会报 `invalid_argument` 并列出合法动词全集。instance 型 provider 的输出带 `parameters_source:"template"`（模板级声明，具体 agent 以平台为准）。
- **`name` / `description`**：部分 provider（典型是 catalog 型）的 card 带每 agent 的名称与描述；没有则据 `provider_label` + `agent_id` 向用户描述。
- **`agent_id_source`**：拿 agent_id 的路径文案，用户没有 agent_id 时照这个引导。
- 未知 agent_ref：catalog 型 provider 对不在目录里的 id 本地报 `invalid_argument`（exit 2），message 形如 `unknown base agent 'nope'`，hint 指回 `agents list <scheme>`。

## 错误目录

本地校验（不发请求）：

| 触发 | subtype | exit | message / hint（真实输出） |
|---|---|---|---|
| 畸形 agent_ref（如 `agents card no-colon`） | invalid_argument | 2 | `agent_ref must look like <provider>:<agent_id>`；hint `agent_ref looks like <scheme>:<agent_id>, e.g. base:assistant` |
| 非法 `--format`（如 `--format xml`） | invalid_argument | 2 | `unsupported --format value "xml"`；hint `valid values: json \| pretty`；`param` 字段为 `--format` |
| catalog 型未知 agent_id | invalid_argument | 2 | message 形如 `unknown base agent 'nope'`；hint `run lark-cli agents list base to see the available agents` |

## 参考

- [lark-agents](../SKILL.md) — agent 全部动词
