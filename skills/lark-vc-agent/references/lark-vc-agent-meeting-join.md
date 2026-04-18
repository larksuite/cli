
# vc +meeting-join

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

通过 9 位会议号加入一场正在进行的视频会议（bot join）。这是一次**写操作**，会实际让当前身份加入会议。

本 skill 对应 shortcut：`lark-cli vc +meeting-join`（调用 `POST /open-apis/vc/v1/bots/join`）。

## 命令

```bash
# 仅指定会议号（无密码）
lark-cli vc +meeting-join --meeting-number 123456789

# 指定会议号 + 密码
lark-cli vc +meeting-join --meeting-number 123456789 --password 8888

# 输出格式
lark-cli vc +meeting-join --meeting-number 123456789 --format json

# 预览 API 调用（不实际加入会议）
lark-cli vc +meeting-join --meeting-number 123456789 --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--meeting-number <no>` | 是 | 会议号，必须为 **9 位纯数字** |
| `--password <pw>` | 否 | 会议密码，仅在该会议设置了入会密码时传入 |
| `--format <fmt>` | 否 | 输出格式：json (默认) / pretty / table / ndjson / csv |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 核心约束

### 1. 仅支持 user 身份

该命令仅支持 `user` 身份，使用前需完成 `lark-cli auth login`。

### 2. 会议号格式严格校验

`--meeting-number` 必须是 9 位纯数字，否则本地校验直接报错：
`--meeting-number must be exactly 9 digits`。

常见错误来源：
- 把会议链接整条粘进来（应仅取尾部的 9 位数字）
- 把 `meeting_id`（长数字 ID）当成会议号传入（两者不是同一个东西）

### 3. 会议必须已开始且允许入会

- 会议必须处于**进行中**状态，bot 无法加入尚未开始或已结束的会议。
- 若会议设置了**等候室 / 入会审批**，bot 可能需要主持人放行后才真正入会。

### 4. 写操作不可回放

这是一次真实入会操作，执行即产生会议内事件（参会人变化、会议日志等）。需要确认会议号再执行，或先 `--dry-run` 核对请求体。

## 输出结果

接口返回会议基本信息，字段视具体响应而定，常见字段：

| 字段 | 说明 |
|------|------|
| `meeting.id` | 会议 ID（可后续传给 `+meeting-leave --meeting-id`） |
| `meeting.meeting_no` | 会议号（与入参一致） |
| `meeting.topic` | 会议主题 |
| `meeting.start_time` | 会议开始时间 |

> **重要**：拿到 `meeting.id` 后务必保留，退出会议（`+meeting-leave`）需要使用它，而不是会议号。

## 如何获取输入参数

| 输入参数 | 获取方式 |
|---------|---------|
| `meeting-number` | 会议号由主持人分享；也可从会议链接尾部解析 9 位数字 |
| `password` | 若会议设置了入会密码，由主持人提供 |

## Agent 组合场景

### 场景 1：加入会议 → 离开会议（最小闭环）

```bash
# 第 1 步：加入会议，记录返回的 meeting.id
lark-cli vc +meeting-join --meeting-number 123456789

# 第 2 步：完成任务后，使用上一步返回的 meeting.id 离开会议
lark-cli vc +meeting-leave --meeting-id <meeting.id>
```

### 场景 2：加入会议 → 会后拉取纪要 / 录制

```bash
# 第 1 步：加入并参会
lark-cli vc +meeting-join --meeting-number 123456789

# 第 2 步：离会
lark-cli vc +meeting-leave --meeting-id <meeting.id>

# 第 3 步：会议结束后，查询录制（拿到 minute_token）
lark-cli vc +recording --meeting-ids <meeting.id>

# 第 4 步：查询会议纪要（总结 / 待办 / 章节 / 逐字稿）
lark-cli vc +notes --meeting-ids <meeting.id>
```

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--meeting-number must be exactly 9 digits` | 会议号不是 9 位纯数字 | 检查是否误传了会议链接或 meeting_id |
| `missing required scope(s)` | 未授权 `vc:meeting.bot.join:write` | 按提示运行 `auth login --scope vc:meeting.bot.join:write` |
| 会议密码错误 | `--password` 错误或未提供 | 向主持人确认会议密码 |
| 会议不存在 / 已结束 | 会议号错误或会议未进行中 | 确认会议正在进行中 |
| 入会被拒绝 | 等候室 / 入会审批 / 限制外部入会 | 联系主持人放行或调整会议设置 |

## 提示

- 仅在 Agent 需要**真实加入**会议（例如参会机器人、会中助手）时使用；只拉取会议数据不需要入会。
- 写操作**不可撤销**，先 `--dry-run` 确认请求体再执行。
- 执行成功后，立即记录返回的 `meeting.id`，用于后续 `+meeting-leave`。

## 参考

- [lark-vc-agent-meeting-leave](lark-vc-agent-meeting-leave.md) — 对应的离会命令
- [lark-vc-agent-meeting-events](lark-vc-agent-meeting-events.md) — 会中事件流
- [lark-vc-search](../../lark-vc/references/lark-vc-search.md) — 搜索历史会议记录
- [lark-vc-recording](../../lark-vc/references/lark-vc-recording.md) — 查询 minute_token
- [lark-vc-notes](../../lark-vc/references/lark-vc-notes.md) — 获取会议纪要
- [lark-vc-agent](../SKILL.md) — Agent 参会能力（本 skill）
- [lark-vc](../../lark-vc/SKILL.md) — 视频会议原子域（Meeting / Note 等核心概念）
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
