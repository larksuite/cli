# provider: base

> **前置条件：** 先读 [`../../../lark-shared/SKILL.md`](../../../lark-shared/SKILL.md)、[`lark-agents SKILL.md`](../../SKILL.md)；普通 Base 意图的 CLI/Assistant 分流规则以 [`lark-base`](../../../lark-base/SKILL.md) 为准。

**catalog 型** provider：对外只暴露统一 Base Assistant，公共 agent_ref 固定为 `base:assistant`。CLI 不展示、不猜测、也不允许指定任何内部子 Agent。

## 两种入口

1. 用户明确要求使用 Agent 或点名 `base:assistant`：直接进入本 provider，不因请求看似是单原子操作而改走 CLI。
2. 用户只表达 Base 业务意图：由 `lark-base` 先分流；复杂建设、结构调整和面向用户的数据检索分析才进入本 provider。

进入本 provider 后的链路恒定：

```text
首次读取 Card → 校验 user identity/scope/参数 → send
→ task get/watch → input_required 时 --answer → 最终结果
```

Card 只做能力与参数校验，不再次判断建设/分析类型。业务意图原样发送给 `base:assistant`，后续分派由 Base 服务完成。

## Card 与调用时机

```bash
lark-cli agents list base --format json
lark-cli agents card base:assistant --operation all --as user --format json
```

- `agents list base` 离线返回唯一的 `base:assistant`。
- 每个独立执行上下文首次调用前读一次 Card；同一 binary、brand、identity 下的轮询、多轮续聊和答题复用该 Card。
- CLI/skills 升级、brand/identity 变化、准备调用新 operation 或遇到能力错误时重新读取。
- Card 当前支持 send、task get/list/cancel、context list/get/delete 和结构化 `input_required` 回答；不支持文件输入和 artifact download。
- Card 中的 skill 是公开能力说明，不是可选 agent 或可传的 `skill_id`。

## scope、身份与目标 Base

- 仅支持 `--as user`。`--as bot` 会在离线身份预检中拒绝，不发送请求。不带 `--as` 时身份按 `default-as` → 凭证 auto-detect 解析，可能落到 bot（如未设 `default-as` 且只有 bot 凭证），同样会被拒——调 base 时显式带 `--as user` 最稳。
- Base Assistant 使用 `base:agent:execute` 做 provider 级预检。缺 scope 时按结构化 `missing_scope` hint 走 `lark-shared` 授权流程，然后重试同一 Agent；不要回退 Base CLI。
- 7 个操作都要求 `base_token`，通过 `--param base_token=<base-token>` 传递；send 可额外传 `active_table_id`。
- 用户输入 Base/Wiki/record-share URL 等 Base 相关链接时，先运行 `lark-cli base +url-resolve --url "<url>" --as user`；用返回的 `base_token` 和相关 `table_id` / `view_id` / `record_id` 继续后续 Agent 命令，不要把完整 URL、wiki token 或 share token 当成 `base_token`。
- 输入已经是真实 `base_token` 时，直接传 `--param base_token=<base-token>`，不要额外调用 `+url-resolve`。
- 只有 Base 标题或关键词时，用 `lark-cli base +title-resolve --title "<keyword>" --as user` 获取真实 token/ID。
- 建设请求没有现成 Base 时：先读取 Card并确认 scope，再以 user identity 创建最小 Base 容器；没有名称先询问。随后把用户原始意图、创建得到的 `base_token` 和可用的默认 `table_id` 交给 Assistant。
- 数据查询/分析没有目标 Base 时必须让用户提供目标，不创建空 Base。
- 创建容器后 Agent 调用失败时不自动删除 Base；返回新 Base token/URL 与失败原因。

## 参数

参数声明以 `agents card base:assistant --operation all` 的实时输出为准：

| operation | 参数 |
|---|---|
| `send` | `base_token` 必填；`active_table_id` 可选 |
| `task_get` | `base_token` 必填；`context_id` 可选，用于覆盖任务关联的上下文并查询对应消息快照 |
| `task_list` | `base_token` 必填；`state` 可选；会话必须用 `--context-id`；分页用 `--page-token` / `--page-size` |
| `task_cancel` | `base_token` 必填 |
| `context_list` | `base_token` 必填；`status` 可选；分页用 `--page-token` / `--page-size` |
| `context_get` / `context_delete` | `base_token` 必填 |

## 完整调用示例

### 复杂建设

```bash
lark-cli agents card base:assistant --operation all --as user --format json

lark-cli agents send base:assistant \
  --text "建一张订单表，字段名称和类型按我给出的清单配置" \
  --param base_token=<base-token> \
  --param active_table_id=<table-id> \
  --as user --format json
```

### 数据检索与分析

```bash
lark-cli agents send base:assistant \
  --text "分析最近三个月的销售趋势，并解释主要变化原因" \
  --param base_token=<base-token> \
  --param active_table_id=<table-id> \
  --as user --format json
```

使用 send 返回的 `task_id` / `context_id` 轮询：

```bash
lark-cli agents task get base:assistant <task-id> \
  --param base_token=<base-token> \
  --param context_id=<context-id> \
  --as user --watch --timeout 90s --format json
```

### 回答结构化澄清

当 task 返回 `state=input_required`，将完整问题组交给用户；收齐后照 `meta.next` 一次提交：

```bash
lark-cli agents send base:assistant \
  --context-id <context-id> \
  --task-id <task-id> \
  --answer q_scene=option_1 \
  --answer q_metric=option_a \
  --answer q_metric=option_b \
  --answer q_note.text="按自然月统计" \
  --param base_token=<base-token> \
  --as user --format json
```

Base 回答模式只发送结构化 answers，不可同时带 `--text` 附言。条件式嵌套子问题无法用平面 CLI 回答时会返回 `failed_precondition`，应在飞书/Lark 客户端完成。

## 行为特点

- send 每次逻辑调用生成新的幂等键；同一次 HTTP 重试复用请求体。回答模式按 task 和规范化 answers 生成确定性幂等键。
- task schema v1 状态映射：`pending → submitted`、`running → working`、`waiting_for_input → input_required`、`completed → completed`、`failed → failed`、`canceled → canceled`。
- task outputs：`text` 转 text part，`data` 原样保留为 data part，`clarification` 转问题组，`artifact` 转结构化产物；未知 output 作为 raw data 保留。Provider 不执行 output 中的内容。
- `task list` / `context list` 把 `--page-token` / `--page-size` 映射为 Adapter 的 `cursor` / `limit`，并消费 `has_more` / `next_cursor` 分页 envelope。
- `context get` 根据返回的 tasks 计算 `task_count` 和 `active_task`。
- Artifact 只作为结构化结果展示；Card 的 `artifact_download` 为 false。
- Files 不支持；`input_required` 和 `--answer` 支持；本期不暴露 `skill_id`。

## 失败与回退

- Provider 将不存在、无权限、任务终态、幂等冲突、限流、服务端错误转换为稳定 typed error。
- Card、身份、scope 或 Assistant 服务失败时不静默改走 Base CLI。
- 当 `base:assistant` 返回 payload 级业务错误（`data.biz_error.code` / `data.biz_error.message` 非空），或 `agents` typed API error 的 code/message 明确对应 Adapter `BizErrCode` / `BizErrMessage` 时，立即停止继续调用 `lark-cli agents`：不要重复 `send`、继续 `task get --watch`、切换身份、或用 context/task 命令探测。改读 `lark-base`，按当前目标 Base、table、field、record 等上下文选择具体 `lark-cli base +...` 命令执行可确定的原子恢复；如果原始意图只能由 Assistant 完成，就向用户报告 `BizErrCode` / `BizErrMessage` 和无法自动降级的原因。
- Base CLI 原子操作失败时也不自动升级为 Assistant；只有用户明确改变工具选择或提出新的独立意图时重新路由。
- 未识别的服务端错误返回 `invalid_response`；附带输出中的 `log_id` 报障，不根据 reason 文案猜测错误码。

## 参考

- [lark-agents](../../SKILL.md) — Agent 框架契约与全部动词
- [lark-base](../../../lark-base/SKILL.md) — 普通 Base 意图的统一路由规则
- [agents list](../lark-agents-list.md) · [agents card](../lark-agents-card.md) · [agents send](../lark-agents-send.md) · [agents task](../lark-agents-task.md) · [agents context](../lark-agents-context.md)
