# 云端会话开发 playbook（Story 3）

> **前置条件：** 先读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md)（认证 / 全局参数 / 安全）+ [`../SKILL.md`](../SKILL.md) 命令索引。
>
> **接口状态：** 本期新增（v7.8），flag 以 `lark-cli apps +<cmd> --help` 为准。

适用场景：用户想**给云端的妙搭 Agent 发消息**，让它在云端生成 / 迭代应用，而不是自己在本地写代码。发完消息**走轮询拿结果**——云端生成约几分钟，不走长连接 stream。

## 核心心智

- **会话在云端。** 你发消息 → 云端 Agent 写代码、构建、部署；本地不产出代码。
- **异步 + 单次读取轮询。** `+chat` 发完即返回 `next_poll_after_ms`（本期固定 30000ms），**不返 turn_id**（turn 还在排队未生成）；用 `+session-read` 间隔轮询拿状态，CLI 无内置 `--wait`，轮询循环由你控制。
- **资源层级** app > session > message：session 是 app 子资源，一轮 chat 是 session 子资源。全部 UAT 用户身份。

## 命令路由

| 命令 | 作用 | 必填参数 | scope |
|------|------|---------|-------|
| `+session-create` | 在 app 下新建 session（**不带消息**） | `--app-id` | `spark:app:write` |
| `+chat` | 向 session 发消息，发起 / 继续对话 | `--app-id` `--session-id` `--message` | `spark:app:write` |
| `+session-read` | 查 session 状态 / 队列 / 最近一轮（轮询入口） | `--app-id` `--session-id` | `spark:app:read` |
| `+session-stop` | 打断 session 正在执行的那一轮 | `--app-id` `--session-id` `--turn-id` | `spark:app:write` |
| `+session-list` | 列出 app 下 session（分页 `--page-size`/`--page-token`） | `--app-id` | `spark:app:read` |

余下 flag 见 `--help`；任意命令加 `--dry-run` 预览。**附件（`attachment_ids`）本期不支持**（独立模块）。

```bash
lark-cli apps +session-create --app-id app_xxx                                    # → session_id
lark-cli apps +chat --app-id app_xxx --session-id conv_xxx --message "做个待办清单页面"  # 异步，→ next_poll_after_ms
lark-cli apps +session-read --app-id app_xxx --session-id conv_xxx                 # 轮询
lark-cli apps +session-stop --app-id app_xxx --session-id conv_xxx --turn-id <turn_id>
lark-cli apps +session-list --app-id app_xxx --format table
```

## 选哪个对话（用户提改动时先决定落点）

| 情形 | 动作 |
|------|------|
| 要「全新应用 + 立刻改动」 | `+create --app-type fullstack`（自带首个 session，JSON 响应里有 `data.session_id`；若无则 `+session-create`）→ 直接 `+chat` |
| 已知 app_id，没指明哪个对话 | 先 `+session-list`；有 `is_active=true` 的对话就**问用户**：现有里继续，还是新开 |
| 「新开一段 / 换个话题」 | `+session-create --app-id <id>` → `+chat` |
| 「接着刚才那段」 | 复用上下文里的 session_id 直接 `+chat`；拿不到就 `+session-list` 让用户选 |
| 拿不到 app_id | **不枚举 / 搜索应用**，向用户要妙搭链接或 app_id（或按 `../SKILL.md`：`.spark/meta.json` / `+list --filter`） |

> 有活跃对话且用户没指明时，先问一句再动手，别替用户拍板。

## 端到端流程 + 轮询

`+chat` 异步：只返回 `next_poll_after_ms`，**不返 turn_id**。`turn_id` 在 turn 执行后从 `+session-read` 的 `latest_turn.turnID` 取（刚进 RUNNING 时可能仍为空），它也是 `+session-stop` 的定位句柄。

```
+session-create / 全栈 +create → session_id
  → +chat(--app-id,--session-id,--message)   # 异步，只回 next_poll_after_ms
  → loop: +session-read(--app-id,--session-id)
        is_streaming=false 且 latest_turn.status=completed → 本轮完成，可发下一条 +chat
        latest_turn.status in (failed,cancelled)           → 转述失败原因，问是否重试
        否则 sleep(next_poll_after_ms) 再读
  →（可选）要停这一轮：从 latest_turn.turnID 取 turn_id 再 +session-stop
```

- 节奏按 `next_poll_after_ms`（本期 30000ms），上限约 30 分钟；别忙等也别让用户反复问。
- 应用预览 URL 来自 `+session-read` 的 `version_anchor`，**本期可能为空**（后续补 checkpoint 集成）；为空时不要向用户编造链接。

## `+session-read` 字段判读

关键字段：`is_active`（能否再 `+chat`）、`is_streaming`（是否有轮在跑）、`latest_turn`（`turnID`，turn 执行后才有 / `status`: running/completed/failed/cancelled）、`queued_count`（排队中待处理的用户消息数）、`summary`、`version_anchor`、`next_poll_after_ms`。

- `is_streaming=false` 且 `latest_turn.status=completed` → 本轮结束，可发下一条。
- `is_active=false` → 会话已关闭，引导 `+session-create` 开新会话。

> ⚠️ **字段大小写不一致**：顶层 snake_case（`session_id` / `is_active` / `is_streaming`），嵌套 turn / 排队消息字段是 **camelCase**（`turnID` / `seqNo` / `submittedAt` / `attachmentIDs`）。按 camelCase 取嵌套键，否则取到 `nil`。

## `+session-stop` 语义

返回 `{stopped, state}`。只停**正在跑（RUNNING）**的当前轮；排队中 / 已结束的轮为 no-op（`stopped=false`）。停断**不关闭会话**，停完仍可 `+chat`。

## 错误码

失败返回 `{ok:false, error:{type,code,message,hint}}`，转述 `hint`（优先）或 `message`，别原样吐 JSON。

| 错误码 | 触发 |
|--------|------|
| `INVALID_PARAM` | 必填缺失 / 参数非法 / 归属错配（session 不属于该 app、turn 不属于该 session） |
| `APP_NOT_FOUND` / `SESSION_NOT_FOUND` | app_id / session_id 无效或无权限 |
| `SESSION_CLOSED` | 向 `is_active=false` 的会话写入 |
| `RATE_LIMITED` / `QUOTA_EXCEEDED` | 限流 / 额度耗尽 |
| `PERMISSION_DENIED` | 鉴权失败 / 无权限 |
| `INTERNAL` | 后端 5xx 兜底 |

## 何时不要用这条 playbook

- 用户要**自己在本地写代码 / 连库调试** → 走 [`lark-apps-local-dev.md`](lark-apps-local-dev.md)。
- 用户只是要**发布现成的 HTML** → 走 [`lark-apps-html-publish.md`](lark-apps-html-publish.md)。

## 参考

[lark-apps](../SKILL.md) · [lark-apps-local-dev](lark-apps-local-dev.md) · [lark-shared](../../lark-shared/SKILL.md)
