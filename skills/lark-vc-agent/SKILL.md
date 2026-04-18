---
name: lark-vc-agent
version: 1.0.0
description: "飞书视频会议：让机器人代当前用户加入/离开正在进行的会议，并读取会议期间的实时事件（参会人加入与离开、发言、聊天、屏幕共享等）。1. 用户提供 9 位会议号、要求代为入会或离会时使用 +meeting-join / +meeting-leave——会真实产生入会/离会记录。2. 会议进行中用户想知道“谁加入了”“谁离开了”“谁在发言”“有人共享屏幕吗”等会中动态时，机器人入会后用 +meeting-events 读取事件时间线。3. 典型场景：参会机器人、会中助手、代为旁听、代为参会。前提：机器人只能读到它自己参会过且仍在进行中的会议的事件；查询已结束会议的参会名单、纪要或逐字稿请使用 lark-vc 技能。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli vc --help"
---

# vc-agent (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取以下两份 skill 文档：**
- [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) — 认证、身份切换、权限处理
- [`../lark-vc/SKILL.md`](../lark-vc/SKILL.md) — 视频会议的核心概念（Meeting / Note / Minutes 等），本 skill 直接复用，不再重复定义

## 定位

本 skill 与 [`lark-vc`](../lark-vc/SKILL.md) 并列：
- **`lark-vc` 负责"会后查询"**：搜索历史会议、参会人快照、纪要/逐字稿/录制
- **`lark-vc-agent` 负责"会中动作"**：机器人入会 / 读取进行中会议的实时事件 / 机器人离会

按此分工路由，避免两个 skill 语义混淆。

| 用户意图示例 | 应路由到 |
|---|---|
| "帮我入会 123456789"、"代我参会"、"让机器人进会旁听" | **本 skill** `+meeting-join` |
| "会议现在还开着，谁刚加入了"、"会议里谁在发言"、"有人共享屏幕吗"（**进行中会议**，且**机器人已入会**）| **本 skill** `+meeting-events` |
| "退出会议"、"让机器人离开" | **本 skill** `+meeting-leave` |
| "昨天那场会有谁参加过"、"搜昨天的会"、"查纪要/逐字稿/录制" | [`lark-vc`](../lark-vc/SKILL.md) |
| "帮我参会，结束后把纪要发到群" 等跨阶段场景 | 按序编排：本 skill（入会 → 读事件 → 离会）→ [`lark-vc`](../lark-vc/SKILL.md) / [`lark-minutes`](../lark-minutes/SKILL.md)（拉纪要）→ [`lark-im`](../lark-im/SKILL.md)（发群） |

## 核心场景

### 1. 加入正在进行的会议（写操作）

1. 只有用户明确表达"让 Agent **真实入会**"（参会机器人、会中助手、代为旁听、代参会）时才用 `+meeting-join`。只是查数据不要入会。
2. `+meeting-join --meeting-number` 只接受 **9 位纯数字**会议号，不是会议链接整串、也不是 `meeting_id`。
3. 返回体中的 `meeting.id` **必须立刻记录**——后续 `+meeting-events` / `+meeting-leave` 都靠它，**不能用 9 位会议号替代**。
4. 入会对所有参会人可见，执行前核实 9 位会议号来源，避免误入错会。
5. 仅支持 `user` 身份，需提前 `lark-cli auth login` 并拥有 `vc:meeting.bot.join:write` scope。
6. 若入会失败，优先查看 `+meeting-join` reference 的错误排查段落，重点确认会议号、密码、会议状态、等候室 / 审批以及会议是否禁止当前身份加入。

### 2. 感知会中事件（读操作）

1. 用户要看"会议里正在发生什么"（参会人加入/离开、聊天、转写、屏幕共享）时，用 `+meeting-events`。
2. 输入是 **`meeting_id`**（长数字 ID），不是 9 位会议号。
3. Bot 必须**真实参会过**（先 `+meeting-join`），否则事件流通常不可见。具体的状态边界、结束后宽限窗口与错误码（如 `10005 / 20001 / 20002`）请查看 `+meeting-events` reference。
4. **不能做会后复盘**，**不能替代参会人快照查询**。已结束会议的发言请用 `vc +notes` 取 `verbatim_doc_token`；参会人快照请用 `vc meeting get --with-participants`（见 [`lark-vc`](../lark-vc/SKILL.md)）。
5. 默认单页；需要完整事件流用 `--page-all` 或 `--page-limit`。
6. 输出格式优先 `--format pretty`（时间线更易读）；需要保证完整消息流才用 `--format json`。
7. 保留响应里的 `page_token`，下次增量拉取直接续，不要从头再拉。

### 3. 离开会议（写操作）

1. 任务完成、或用户要求结束时，用 `+meeting-leave --meeting-id <从 +meeting-join 拿到的 meeting.id>`。
2. `--meeting-id` **必须**是 `+meeting-join` 返回的长数字 `meeting.id`，**不接受 9 位会议号**。
3. 离会**立即生效**，机器人从会议的参会人列表中消失，对其他参会人可见；若需要重新入会，再跑一次 `+meeting-join` 即可（非真正"不可逆"）。
4. 仅支持 `user` 身份，scope 同 `+meeting-join`（`vc:meeting.bot.join:write`）。

### 4. Agent 参会最小闭环示范

```bash
# 1. 入会，捕获 meeting.id
JOIN=$(lark-cli vc +meeting-join --meeting-number 123456789 --format json)
MID=$(echo "$JOIN" | jq -r '.data.meeting.id')

# 2. 会中轮询事件
#    --start 每轮用上一次响应里最新事件的 event_time，避免拉重复事件
#    典型间隔 10-30 秒
lark-cli vc +meeting-events --meeting-id "$MID" --format pretty

# 3. 任务完成或用户要求结束时离会
lark-cli vc +meeting-leave --meeting-id "$MID"

# 4. 会后可选：取纪要 / 逐字稿（跨到 lark-vc）
lark-cli vc +notes --meeting-ids "$MID"
```

## Shortcuts

Shortcut 是对常用操作的高级封装（`lark-cli vc +<verb> [flags]`）。

| Shortcut | 类型 | 说明 |
|----------|------|------|
| [`+meeting-join`](references/lark-vc-agent-meeting-join.md)   | 写 | Join an in-progress meeting by 9-digit meeting number |
| [`+meeting-events`](references/lark-vc-agent-meeting-events.md) | 读 | List bot meeting events (participant joined/left, transcript, chat, share) |
| [`+meeting-leave`](references/lark-vc-agent-meeting-leave.md) | 写 | Leave a meeting by meeting_id |

- 使用 `+meeting-join` 前**必须**阅读 [references/lark-vc-agent-meeting-join.md](references/lark-vc-agent-meeting-join.md)，了解入参格式与写操作可见性风险。
- 使用 `+meeting-events` 前**必须**阅读 [references/lark-vc-agent-meeting-events.md](references/lark-vc-agent-meeting-events.md)，了解 `meeting_id` 来源、分页、错误码（10005 / 20001 / 20002）与 "bot 仍在会中" 硬约束。
- 使用 `+meeting-leave` 前**必须**阅读 [references/lark-vc-agent-meeting-leave.md](references/lark-vc-agent-meeting-leave.md)，了解 `meeting_id` 的来源与写操作可见性。

## 权限表

| Shortcut | 所需 scope |
|----------|-----------|
| `+meeting-join`  | `vc:meeting.bot.join:write` |
| `+meeting-events`| `vc:meeting.meetingevent:read` |
| `+meeting-leave` | `vc:meeting.bot.join:write` |

## 延伸

- 查已结束会议、参会人快照、搜索历史会议 → [`lark-vc`](../lark-vc/SKILL.md)
- 会议纪要、逐字稿 → [`lark-vc`](../lark-vc/SKILL.md) 的 `+notes`
- 妙记产物（AI 总结 / 转写 / 章节）→ [`lark-minutes`](../lark-minutes/SKILL.md)
- 会后把产物发到群 / 私聊 → [`lark-im`](../lark-im/SKILL.md)
- 认证、身份切换、scope 管理 → [`lark-shared`](../lark-shared/SKILL.md)
