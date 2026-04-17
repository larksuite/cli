
# vc +meeting-events

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

查询当前 bot 在一场正在进行的视频会议中收到的会中事件列表。该命令是**读操作**，但只有当 bot 仍在会中时才能成功返回事件。

本 skill 对应 shortcut：`lark-cli vc +meeting-events`（调用 `GET /open-apis/vc/v1/bots/events`）。

## 命令

```bash
# 查询当前会议的首批事件
lark-cli vc +meeting-events --meeting-id 69xxxxxxxxxxxxx28

# 指定时间范围
lark-cli vc +meeting-events --meeting-id 69xxxxxxxxxxxxx28 --start 2026-04-17T15:00:00+08:00 --end 2026-04-17T16:00:00+08:00

# pretty 时间线输出
lark-cli vc +meeting-events --meeting-id 69xxxxxxxxxxxxx28 --format pretty

# 自动翻页（最多 10 页）
lark-cli vc +meeting-events --meeting-id 69xxxxxxxxxxxxx28 --page-limit 10

# 自动翻到上限
lark-cli vc +meeting-events --meeting-id 69xxxxxxxxxxxxx28 --page-all

# 预览 API 调用（不实际请求）
lark-cli vc +meeting-events --meeting-id 69xxxxxxxxxxxxx28 --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--meeting-id <id>` | 是 | 会议 ID（长数字 ID，不是 9 位会议号） |
| `--start <time>` | 否 | 起始时间，支持 ISO 8601 / `YYYY-MM-DD` / Unix 秒 |
| `--end <time>` | 否 | 结束时间，支持 ISO 8601 / `YYYY-MM-DD` / Unix 秒 |
| `--page-token <token>` | 否 | 从指定分页游标继续拉取下一页 |
| `--page-size <n>` | 否 | 单页模式每页大小。CLI 会自动夹紧到 `20-100`；传 `--page-all` 时固定使用 `100` |
| `--page-limit <n>` | 否 | 自动分页最大页数。设置该参数会开启自动分页 |
| `--page-all` | 否 | 自动分页，并在未显式指定 `--page-limit` 时使用最大页上限 |
| `--format <fmt>` | 否 | 输出格式：json (默认) / pretty / table / ndjson / csv |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 核心约束

### 1. 输入必须是 meeting_id，不是 9 位会议号

`--meeting-id` 必须是会议的长数字 ID。它通常来自：
- `+meeting-join` 返回体中的 `meeting.id`
- `+search` 结果中的 `id`

**不要**把 9 位会议号（`--meeting-number`）传给这个命令。

### 2. 仅支持 user 身份

该命令仅支持 `user` 身份，使用前需完成 `lark-cli auth login`。

### 3. bot 必须仍在会中

这是查询“bot 在会中观察到的事件”的接口。若 bot 已离会、未入会、或会议已经无法再判断 bot 身份，后端通常会报：
- `bot is not in meeting, no permission`

因此，最稳妥的调用顺序通常是：

```bash
# 先入会
lark-cli vc +meeting-join --meeting-number 123456789

# 记录返回的 meeting.id

# 再查询事件
lark-cli vc +meeting-events --meeting-id <meeting.id>
```

### 4. 自动分页规则

- 不传 `--page-all` 也不传 `--page-limit`：只查 1 页
- 传 `--page-limit N`：开启自动分页，最多拉 `N` 页
- 传 `--page-all`：开启自动分页；若未显式传 `--page-limit`，使用最大页上限
- `--page-all` 时，CLI 固定使用最大 `page_size=100`

### 5. pretty / json 输出差异

- `--format pretty`：输出“会议主题 / 会议时间 / 时间线”，适合人读
- `--format json`：保留原始 `events` 结构，适合 Agent / 程序消费

> **注意**：pretty 输出中的正文文本会做单行转义，像真实换行会显示为 `\n`，避免打乱时间线布局。

## 返回结构

常见顶层字段：

| 字段 | 说明 |
|------|------|
| `events` | 事件列表 |
| `has_more` | 是否还有下一页 |
| `page_token` | 下一页游标 |

事件 `event_type` 常见类型：

| event_type | 含义 |
|-----------|------|
| `participant_joined` | 有参会人加入会议 |
| `participant_left` | 有参会人离开会议 |
| `chat_received` | 收到会中聊天消息 |
| `transcript_received` | 收到转写文本 |
| `magic_share_started` | 开始共享内容 / 文档 |
| `magic_share_ended` | 结束共享 |

## pretty 输出示例

```text
会议主题：张三的视频会议
会议时间：2026-04-17 15:28:52（进行中）

[00:00:33] 明日之虾BOE(ou_xxx) 加入了会议
[00:00:41] 张三(ou_xxx): [text] 6666
[00:00:44] 张三(ou_xxx) 开始共享《智能纪要：飞书20251022-140223 2026年3月9日》
           URL: https://...
[00:01:32] 张三(ou_xxx): [reaction] JIAYI
```

## 如何获取输入参数

| 输入参数 | 获取方式 |
|---------|---------|
| `meeting-id` | `+meeting-join` 返回的 `meeting.id`；或 `+search` 结果中的 `id` |
| `start` / `end` | 用户给出的时间范围；如未给出则默认取全量可见事件 |
| `page-token` | 上一页返回体中的 `page_token` |

## Agent 组合场景

### 场景 1：入会后查看会中发生了什么

```bash
# 第 1 步：加入会议，记录返回的 meeting.id
lark-cli vc +meeting-join --meeting-number 123456789

# 第 2 步：查询事件流
lark-cli vc +meeting-events --meeting-id <meeting.id> --format pretty
```

### 场景 2：会中事件排障 / 会话回放

```bash
# 先看结构化原始数据
lark-cli vc +meeting-events --meeting-id <meeting.id> --format json

# 需要继续翻页时
lark-cli vc +meeting-events --meeting-id <meeting.id> --page-limit 5 --format json
```

### 场景 3：过滤某段时间内的事件

```bash
lark-cli vc +meeting-events \
  --meeting-id <meeting.id> \
  --start 2026-04-17T15:00:00+08:00 \
  --end 2026-04-17T16:00:00+08:00 \
  --format pretty
```

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--meeting-id is required` | 未传入 `--meeting-id` | 传入长数字 `meeting.id` |
| `invalid --page-limit` | `page-limit` 小于 1 或超过上限 | 调整到允许范围内 |
| `bot is not in meeting` | bot 当前不在该会议中 | 先 `+meeting-join` 成功，再查事件 |
| `HTTP 404` / `HTTP 500` | 服务端当前无法找到或处理该会议实例 | 换一个正在进行且 bot 可见的 meeting_id，或排查后端问题 |
| `missing required scope(s)` | 未授权 `vc:meeting.meetingevent:read` | 按提示重新 `auth login` 补 scope |

## 提示

- 这是**会中事件流**查询，不适合拿来搜历史会议记录；搜历史会议请用 `+search`。
- 如果你只需要最终纪要、录制、逐字稿，不必查事件列表，直接用 `+notes` / `+recording`。
- 事件列表是否完整，取决于 bot 何时入会、何时离会，以及后端当前可见的会中事件范围。

## 参考

- [lark-vc-meeting-join](lark-vc-meeting-join.md) — 先真实入会
- [lark-vc-meeting-leave](lark-vc-meeting-leave.md) — 完成任务后离会
- [lark-vc-search](lark-vc-search.md) — 搜索历史会议（获取 meeting_id）
- [lark-vc-recording](lark-vc-recording.md) — 查询 minute_token
- [lark-vc-notes](lark-vc-notes.md) — 获取会议纪要
- [lark-vc](../SKILL.md) — 视频会议全部命令
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
