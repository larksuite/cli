# lark-apps 云端会话开发

适用：用户希望让云端妙搭 Agent 生成或迭代应用，而不是把代码拉到本地开发。

## 核心流程

会话在云端：本地只发消息和轮询状态，不产出代码、不启动本地 dev server。`+chat` 异步，返回建议轮询间隔；CLI 不内置长连接等待。

`app -> session -> chat turn`。所有 session/chat shortcut 都用用户身份。

```bash
# 可先创建 app；云端需求通过 session/chat 提交
lark-cli apps +create --name "待办应用" --app-type full_stack \
  --description "支持新增、完成、筛选待办"

lark-cli apps +session-create --app-id app_xxx
lark-cli apps +chat --app-id app_xxx --session-id sess_xxx --message "做一个待办清单页面"
lark-cli apps +session-read --app-id app_xxx --session-id sess_xxx
lark-cli apps +session-list --app-id app_xxx
```

## 需求发送

- 只有用户明确选择云端路径，或明确说“让妙搭 Agent/云端 AI 生成/迭代”时，才进入本 reference；不要因为用户只说“做个 X”或“给我链接”就默认云端。
- 进入云端路径后，极简需求也可直接发起生成，例如“做个投票工具”“做个站会小应用”。先建 `full_stack` app，再用 `+chat --message "<用户原话>"` 透传需求，不编造实体、字段或业务细节。
- 如果需求过泛，可在 `+chat --message` 中保留原话，并只补一句“请先生成通用版本，后续可继续迭代”，不要用多轮追问阻塞生成。

## 会话落点

| 情形 | 动作 |
|---|---|
| 全新应用 + 云端生成 | 先 `+create --app-type full_stack` 拿 `app_id`，再 `+session-create` -> `+chat` |
| 已知 app_id，用户没指定会话 | 先 `+session-list`；有活跃会话时问用户继续现有还是新开 |
| 用户说“新开一段/换个话题” | `+session-create` 后再 `+chat` |
| 用户说“接着刚才” | 复用上下文 session_id；拿不到就 `+session-list` 让用户选 |

## 轮询规则

- `+chat` 异步，只返回 `next_poll_after_ms`，不返回 `turn_id`。
- 等待 `next_poll_after_ms` 后调用 `+session-read`；由 agent 驱动轮询。若没有返回建议间隔，用 5-10 秒节奏轮询。
- 不知道已有 session 时先 `+session-list --app-id <id>`，再选最近活跃或让用户确认。
- `is_streaming=true`、`building` / `running` / `streaming` 表示仍在生成，继续轮询，不傻等也不提前放弃。
- `is_streaming=false` 且 `latest_turn.status=completed` 表示本轮完成，可发下一条。
- `failed` / `cancelled` 时转述错误字段或 hint，询问是否重试。
- 预览 URL 只来自 `+session-read` 返回的明确字段；为空时不要编造链接。
- 要中止正在运行的一轮，从 `+session-read` 的 `latest_turn.turnID` 取值，再调用：

```bash
lark-cli apps +session-stop --app-id app_xxx --session-id sess_xxx --turn-id turn_xxx
```

## 字段注意

顶层字段多为 snake_case，如 `session_id`、`is_active`、`is_streaming`、`next_poll_after_ms`。嵌套 turn 字段使用 camelCase，如 `turnID`。不要把 `turnID` 写成 `turn_id`。

`+session-stop` 只停止正在运行的当前轮，不关闭会话；停完仍可继续 `+chat`。

## 不适用

- 用户已有本地 HTML/dist，要马上发布 URL：读 [`lark-apps-html-publish.md`](lark-apps-html-publish.md)。
- 用户要本地写代码、改仓库、跑 dev server：读 [`lark-apps-local-dev.md`](lark-apps-local-dev.md)。
