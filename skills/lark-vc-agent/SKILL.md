---
name: lark-vc-agent
version: 1.0.0
description: "飞书视频会议：让机器人代当前用户加入/离开正在进行的会议，并读取会议期间的实时事件（参会人加入离开、发言、聊天、屏幕共享等）。1. 用户提供 9 位会议号、要求代为入会或离会时使用 +meeting-join / +meeting-leave——会真实产生入会/离会记录。2. 会议进行中用户想知道"谁加入了"、"谁离开了"、"谁在发言"、"有人共享屏幕吗"等会中动态时，机器人入会后用 +meeting-events 读取事件时间线。3. 典型场景：参会机器人、会中助手、代为旁听、代为参会。前提：机器人只能读到它自己参加过的会议的事件，会议必须仍在进行中；查询已结束会议的参会名单、纪要或逐字稿请使用 lark-vc 技能。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli vc --help"
---

# vc-agent (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取以下两份文档：**
- [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) — 认证、身份切换、权限处理
- [`../lark-vc/SKILL.md`](../lark-vc/SKILL.md) — 视频会议原子域的**核心概念**（Meeting / meeting_id / Note 等，本 skill 直接复用，不再重复定义）

## 定位

本 skill 是 `lark-vc` 的**姊妹 skill**，专注 **Agent 参会闭环**（join → events → leave）。把写操作 + 实时事件这类"会中"语义从 `lark-vc` 的"会后查询"语义里切出来，避免混淆。

| 用户意图 | 应路由到 |
|---------|---------|
| "帮我入会 123456789"、"作为 agent 参会"、"会中事件"、"谁加入了"（**进行中会议**）| **本 skill** |
| "搜昨天的会"、"查纪要/逐字稿"、"录制"、"参会人快照"（任意时点）| [`lark-vc`](../lark-vc/SKILL.md) |
| "帮我参会并会后发纪要到群里" 这类跨 skill 场景 | 本 skill + [`lark-vc`](../lark-vc/SKILL.md) + [`lark-minutes`](../lark-minutes/SKILL.md) + [`lark-im`](../lark-im/SKILL.md) 按序编排 |

## 核心场景

### 1. 加入正在进行的会议（写操作）

1. 只有用户明确表达"让 Agent **真实入会**"（参会机器人、会中助手、代为旁听、代参会）时才用 `+meeting-join`。只是查数据不要入会。
2. `+meeting-join --meeting-number` 只接受 **9 位纯数字**会议号，不是会议链接整串、也不是 `meeting_id`。
3. 返回体中的 `meeting.id` **必须立刻记录**——后续 `+meeting-events` / `+meeting-leave` 都靠它，**不能用 9 位会议号替代**。
4. 写操作**不支持回放**，执行前优先 `--dry-run` 核对请求体。
5. 仅支持 `user` 身份，需提前 `lark-cli auth login` 并拥有 `vc:meeting.bot.join:write` scope。

### 2. 感知会中事件（读操作）

1. 用户要看"会议里正在发生什么"（参会人加入/离开、聊天、转写、屏幕共享）时，用 `+meeting-events`。
2. 输入是 **`meeting_id`**（长数字 ID），不是 9 位会议号。
3. **硬约束**——选型前必读：
   - Bot 必须**真实参会过**（先 `+meeting-join`），否则返回 `10005 bot is not in meeting`。
   - 仅对**进行中**会议有效，已结束会议返回 `20001 meeting_status_MEETING_END`。
   - **不能做会后复盘**，**不能替代参会人快照查询**。已结束会议的发言请用 `vc +notes` 取 `verbatim_doc_token`；参会人快照请用 `vc meeting get --with-participants`（见 [`lark-vc`](../lark-vc/SKILL.md)）。
4. 默认单页；需要完整事件流用 `--page-all` 或 `--page-limit`。
5. 输出格式优先 `--format pretty`（时间线更易读）；Agent 程序消费场景才用 `--format json`。
6. 保留响应里的 `page_token`，下次增量拉取直接续，不要从头再拉。

### 3. 离开会议（写操作）

1. 完成任务或用户喊停后用 `+meeting-leave --meeting-id <从 +meeting-join 拿到的 meeting.id>`。
2. **不接受 9 位会议号**，只接受 `meeting_id`。
3. 写操作**不可撤销**：离会后录制 / 纪要完整性可能受影响，执行前 `--dry-run` 核对。
4. 仅 `user` 身份，同样需要 `vc:meeting.bot.join:write` scope。

### 4. Agent 参会最小闭环示范

```bash
# 1. 入会，捕获 meeting.id
JOIN=$(lark-cli vc +meeting-join --meeting-number 123456789 --format json)
MID=$(echo "$JOIN" | jq -r '.meeting.id // .data.meeting.id')

# 2. 会中轮询事件（每隔 N 秒，带递进 --start 避免重复）
lark-cli vc +meeting-events --meeting-id "$MID" --format pretty

# 3. 任务完成或用户喊停后离会
lark-cli vc +meeting-leave --meeting-id "$MID"

# 4. 会后可选：取纪要 / 逐字稿（跨到 lark-vc）
lark-cli vc +notes --meeting-ids "$MID"
```

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli vc +<verb> [flags]`）。

| Shortcut | 类型 | 说明 |
|----------|------|------|
| [`+meeting-join`](references/lark-vc-agent-meeting-join.md)   | 写 | Join an in-progress meeting by 9-digit meeting number |
| [`+meeting-events`](references/lark-vc-agent-meeting-events.md) | 读 | List bot meeting events (participant joined/left, transcripts, chat, share) |
| [`+meeting-leave`](references/lark-vc-agent-meeting-leave.md) | 写 | Leave a meeting by meeting_id |

- 使用 `+meeting-join` 前**必须**阅读 [references/lark-vc-agent-meeting-join.md](references/lark-vc-agent-meeting-join.md)，了解入参格式与写操作风险。
- 使用 `+meeting-events` 前**必须**阅读 [references/lark-vc-agent-meeting-events.md](references/lark-vc-agent-meeting-events.md)，了解 `meeting_id` 来源、分页、错误码（10005 / 20001 / 20002）与"bot 仍在会中"限制。
- 使用 `+meeting-leave` 前**必须**阅读 [references/lark-vc-agent-meeting-leave.md](references/lark-vc-agent-meeting-leave.md)，了解 `meeting_id` 的来源与写操作风险。

> **写操作铁律**：`+meeting-join` / `+meeting-leave` 会真实入会 / 离会、产生会议日志和参会记录。执行前优先 `--dry-run`，`meeting.id` 必须保留。

## 权限表

| Shortcut | 所需 scope |
|----------|-----------|
| `+meeting-join`  | `vc:meeting.bot.join:write` |
| `+meeting-leave` | `vc:meeting.bot.join:write` |
| `+meeting-events`| `vc:meeting.meetingevent:read` |

## 延伸

- 查已结束会议 / 会议纪要 / 参会人快照 / 搜索历史会议 → [`lark-vc`](../lark-vc/SKILL.md)
- 妙记产物（AI 总结 / 转写 / 章节）→ [`lark-minutes`](../lark-minutes/SKILL.md)
- 认证、身份切换、scope 管理 → [`lark-shared`](../lark-shared/SKILL.md)
