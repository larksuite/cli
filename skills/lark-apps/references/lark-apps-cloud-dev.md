# 云端会话开发 playbook（Story 3）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)（认证 / 全局参数 / 安全），并读完 [`../SKILL.md`](../SKILL.md) 的命令索引。
>
> **接口状态：** 本期新增命令（`+session-*` / `+chat`）接口仍在收敛，flag 以 `lark-cli apps +<cmd> --help` 为准；标 `（待定）` 的是设计中、可能变动的参数。

适用场景：用户想**给云端的妙搭 Agent 发消息**，让它在云端生成 / 迭代应用，而不是自己在本地写代码。发完消息后**走轮询拿结果**——云端生成 + 构建 + 部署整体约 10 分钟，不走长连接 stream。

## 核心心智

- **会话发生在云端。** 你发消息 → 云端 Agent 写代码、commit 到 develop、构建、部署。本地不产出代码。
- **异步 + 轮询。** `+session-create` / `+chat` 发完即返回（秒级返回 `session_id` / `task_id`），实际生成在后台跑；用 `+session-read` 间隔 5–10s 轮询拿 `status` / `progress` / `app_url`。
- **拿到 url 后引流到产品内深度开发。** 三方 agent 一期承担轻量入口职责；多轮深度迭代 / 协作者 / 可见范围等建议进妙搭产品内或转本地全栈（见 [`lark-apps-local-dev.md`](lark-apps-local-dev.md)）。

## 端到端流程

```
+session-create(--app-id, --message) → 拿 session_id
   → 轮询 +session-read(--session-id) 直到 status=ready 拿 app_url
   → 需要继续迭代：+chat(--session-id, --message) → 再轮询
   → 结束：+session-stop(--session-id)
```

| 步骤 | 命令 | 说明 |
|------|------|------|
| 1. 创建会话 + 发首条需求 | `+session-create` | 在应用下开会话发首条需求，秒级返回 `session_id`。**新建**应用先 `+create`；**存量**应用先解析 `app_id`（按 [`../SKILL.md`](../SKILL.md)：`.spark/meta.json` 或 `+list --filter`）。可带附件 |
| 2. 轮询进度 | `+session-read` | 间隔 5–10s 拉一次，看 `status` / `progress`，`ready` 后拿 `app_url`（见下方轮询约定） |
| 3.（可选）继续迭代 | `+chat` | 同一会话内继续发消息，之后回到 step 2 轮询 |
| 4.（可选）列 / 停会话 | `+session-list` / `+session-stop` | 列出某应用下会话 / 主动终止会话（P1） |

> 各命令的 flag（`--message` / `--attach` / `--session-id` 等）、附件类型大小见命令自带 reference（`lark-cli apps +<cmd> --help`）。

## 轮询约定（这条 playbook 的关键）

- 节奏：**5–10 秒一次**，上限约 30 分钟。
- 优先看响应里的 `_hint`（若服务端给出下次合适的拉取时机，按它来，避免无效轮询）。
- `status=ready` → 把 `app_url` 报给用户；`status=failed` → 转述 `error`。

报告话术：

> 应用正在云端生成中（约需 10 分钟），我会每隔 ~10s 查一次进度。
> ——完成后——
> 已生成完毕，访问链接：`{app_url}`

## 何时不要用这条 playbook

- 用户要**自己在本地写代码 / 连库调试** → 走 [`lark-apps-local-dev.md`](lark-apps-local-dev.md)。
- 用户只是要**发布现成的 HTML** → 走 [`lark-apps-html-publish.md`](lark-apps-html-publish.md)。

## 参考

- [lark-apps](../SKILL.md) — 路由表 + 命令索引 + 心智模型
- [lark-apps-local-dev](lark-apps-local-dev.md) — 想转本地深度开发时
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
