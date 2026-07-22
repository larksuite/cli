# lark-apps 云端会话开发

适用：用户希望让云端妙搭 Agent 生成或迭代应用，而不是把代码拉到本地开发。

## 核心流程

整个开发在云端进行：本地只负责「发消息 + 轮询状态」，不拉源码、不产出代码、不启动本地 dev server。所有 session/chat 命令都以用户身份执行（`--as user`）。

### 资源模型：app → session → turn

三层父子关系，下层都挂在上层之下：

- **app（应用资产）**：一个妙搭应用，由 `+create` 创建并拿到 `app_id`。云端生成应用类型用 `full_stack`。
- **session（会话）**：一个 app 下的一段独立对话上下文，由 `+session-create` 创建并拿到 `session_id`。一个 app 可有多个 session；`is_active` 表示该 session 当前是否可写（可发起对话）。
- **turn（轮）**：一个 session 里的一轮交互 = 一条用户消息 + 妙搭 Agent 针对它的生成/迭代。`+chat` 发一条消息就发起一轮；轮的句柄是 `turn_id`，状态看 `latest_turn.status`。

### 执行模型：异步 + 轮询

`+chat` 把消息入队后**立即返回、不等生成完成，响应不带 `turn_id`**；本轮状态与轮询节奏全靠 `+session-get` 读 `latest_turn.status` / `is_streaming` / `next_poll_after_ms`。

`+session-get` 关键字段：

- `is_streaming`：当前是否有一轮正在跑（`true`=还在生成）。
- `latest_turn.status`：最近一轮的状态，只有 `running` / `completed` / `failed` / `cancelled`。
- `latest_turn.turn_id`：最近一轮的句柄（`+session-stop --turn-id` 用它）。
- `latest_turn.user_message`：本轮用户发的消息。
- `latest_turn.messages`：本轮完成后回看全貌的消息列表，按时序排列、每条带 `role`（用户消息、模型回复、工具调用等都在内，role 取值如 `user` / `assistant` / `tool`）。注意它在 `latest_turn` 仍 running/初始化期可能为空——该轮**进行中**的实时进展改用 `+session-messages-list --turn-id <latest_turn.turn_id>` 读（见下方轮询规则）。
- `queued_messages` / `queued_count`：还没开始跑、排在后面的消息。
- `next_poll_after_ms`：建议的下次轮询间隔（毫秒，固定值）；非空时优先用它。

轮询规则：

- 节奏按 [初始化 vs 增量修改](#初始化-vs-增量修改) 判定：增量 5-10 秒一次；初始化 60-120 秒一次；`next_poll_after_ms` 非空时用它。
- `is_streaming=true`、`building` / `running` / `streaming` 表示仍在生成，继续轮询，不傻等也不提前放弃；初始化阶段单次 sleep 拉到 60-120 秒，进入 `streaming` 或属增量修改时切回 5-10 秒。
- `is_streaming=false` 且 `latest_turn.status=completed` 表示本轮完成，可发下一条。
- `failed` / `cancelled` 时转述错误字段或 hint，由用户决定是否重试，不要静默重发。
- 不知道某 app 有哪些 session 时，先 `+session-list --app-id <id>`，再选最近活跃的或让用户确认，别直接猜 `session_id`。
- 要中止正在运行的一轮，从 `+session-get` 的 `latest_turn.turn_id` 取值，再调用 `+session-stop --turn-id <turn_id>`。
- 状态与节奏看 `+session-get`，本轮实时内容看 `+session-messages-list`：想在 running 期间向用户播报"云端 Agent 此刻在做什么"，用 `+session-messages-list --turn-id <latest_turn.turn_id>` 读已产出的增量消息（running 期间即可读，不必等本轮结束）。复用上面的轮询节奏、不另起更密的轮询；续拉时把上次响应的 `next_page_token` 作 `--page-token` 只取新消息，转述时简述进展、不原样打印整段消息或工具输出。

### 典型链路

```bash
# 1) 建 app，拿 app_id（云端生成走 full_stack）
lark-cli apps +create --name "待办应用" --app-type full_stack \
  --description "支持新增、完成、筛选待办"

# 2) 在该 app 下建 session，拿 session_id
lark-cli apps +session-create --app-id app_xxx

# 3) 发消息发起一轮（异步入队，立即返回，无 turn_id）
lark-cli apps +chat --app-id app_xxx --session-id sess_xxx --message "做一个待办清单页面"

# 4) 轮询本轮状态；完成后从 latest_turn.messages 读取结果
lark-cli apps +session-get --app-id app_xxx --session-id sess_xxx

# 找该 app 已有的会话（续聊/不确定 session 时用）
lark-cli apps +session-list --app-id app_xxx
```

## 完成态不等于发布态

通用发布态判定（is_published 语义、开发态链接拼接、发布态链接来源）见 SKILL.md「发布态护栏」。本 reference 只补云端会话特有的措辞：

- `+session-get` 返回 `is_streaming=false` 且 `latest_turn.status=completed`，只说明本轮云端生成/迭代结束，不等于已发布部署。
- 如果只完成了云端会话、没有确认发布完成，就明确告诉用户“开发态链接可进入继续编辑，发布态是否为最新版本尚未确认”。

## 需求发送

- 只有用户明确选择云端路径，或明确说“让妙搭 Agent / 云端 AI 生成/迭代”时，才进入本 reference；不要因为用户只说“做个 X”或“给我链接”就默认云端。
- 进入云端路径后，极简需求也可直接发起生成，例如“做个投票工具”“做个站会小应用”。先建 `full_stack` app，再用 `+chat --message "<用户原话>"` 透传需求，不编造实体、字段或业务细节。
- 如果需求过泛，可在 `+chat --message` 中保留原话，并只补一句“请先生成通用版本，后续可继续迭代”，不要用多轮追问阻塞生成。

## 会话落点

| 情形 | 动作 |
|---|---|
| 全新应用 + 云端生成 | 先 `+create --app-type full_stack` 拿 `app_id`，再 `+session-create` -> `+chat` |
| 已知 app_id，用户没指定会话 | 先 `+session-list`；有活跃会话时问用户继续现有还是新开 |
| 用户说“新开一段/换个话题” | `+session-create` 后再 `+chat` |
| 用户说“接着刚才” | 复用上下文 session_id；拿不到就 `+session-list` 让用户选 |
| 用户问会话“进行到哪一步/当前状态/最新进展” | 用 `+session-get --session-id <sid>` 读状态。`+session-list` 只负责发现/选择会话，不含执行状态；它返回空不等于无状态可查（session_id 也可能来自上下文），别用 `+session-list`/`+release-list` 代替 `+session-get` 回答进度 |

## 初始化 vs 增量修改

`+chat` 单轮的耗时差距很大，取决于目标 app 是否**已初始化**。两者的轮询节奏不同，**`+chat` 前先把状态判定清楚**，不要拿"是不是第一次发消息"当代理判断——session 是新建的不代表 app 没初始化过。

### 判定规则

**已初始化**（满足任一即认为已初始化）：

1. 本地存在该 app 的项目目录（已 `+init` 或 clone 过），**且** git commit 数 > 2；
2. 应用维度（云端）至少有一个已提交的版本，按以下任一信号判断：
   - `lark-cli apps +session-get --app-id <app_id> --session-id <session_id>` 的返回里出现已提交版本信息；
   - 在 `lark-cli apps +list`（必要时配 `--keyword <name>` 定位）的目标 app 条目里 `is_published: true`。

**未初始化**（两个条件同时成立）：

1. 本地不存在该 app 的项目目录；
2. 应用维度没有任何已提交版本（即上面两路云端信号都判 false）。

### 两种 `+chat` 的行为

| 状态 | 服务端动作 | 单轮耗时 | 轮询建议 |
|---|---|---|---|
| 已初始化 → **增量修改** | 云端 Agent 在已有云端工作区上对**已提交代码**做局部修改，跳过方案设计与首次生成 | 通常分钟级 | `next_poll_after_ms` 为空时 5-10 秒一次 |
| 未初始化 → **首次初始化 + 生成** | 服务端跑完整的应用初始化流程：需求分析、技术方案、数据模型、UI 与后端代码生成、首版代码提交到云端工作区 | 视需求复杂度，**通常 20~50 分钟** | `next_poll_after_ms` 为空时 60-120 秒一次 |

初始化阶段 `+session-get` 可能长时间持续返回 `building` / `running`，是正常状态，**不要按失败处理，也不要催用户**。

## 字段注意

所有字段统一 snake_case，顶层和嵌套 turn 字段都一样：`session_id`、`is_active`、`is_streaming`、`next_poll_after_ms`、`latest_turn.turn_id`、`latest_turn.status`、`latest_turn.user_message`、`latest_turn.messages`。

`+session-stop` 只停止正在运行的当前轮，不关闭会话；停完仍可继续 `+chat`。

## 不适用

- 用户要本地写代码、改仓库、跑 dev server：读 [`lark-apps-local-dev.md`](lark-apps-local-dev.md)。
