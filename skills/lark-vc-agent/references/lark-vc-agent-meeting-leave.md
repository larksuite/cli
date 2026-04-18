
# vc +meeting-leave

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

通过 `meeting_id` 离开当前身份所在的视频会议（bot leave）。这是一次**写操作**，会实际把当前身份从会议中移出。

本 skill 对应 shortcut：`lark-cli vc +meeting-leave`（调用 `POST /open-apis/vc/v1/bots/leave`）。

## 命令

```bash
# 通过 meeting_id 离会
lark-cli vc +meeting-leave --meeting-id 69xxxxxxxxxxxxx28

# 输出格式
lark-cli vc +meeting-leave --meeting-id 69xxxxxxxxxxxxx28 --format json

# 预览 API 调用（不实际离会）
lark-cli vc +meeting-leave --meeting-id 69xxxxxxxxxxxxx28 --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--meeting-id <id>` | 是 | 会议 ID（**不是 9 位会议号**） |
| `--format <fmt>` | 否 | 输出格式：json (默认) / pretty / table / ndjson / csv |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 核心约束

### 1. 入参是 meeting_id，不是会议号

`--meeting-id` 必须是会议的长数字 ID，通常由 `+meeting-join` 返回体中的 `meeting.id` 提供，也可从 `+search` 结果中的 `id` 字段获取。**传 9 位会议号会失败**。

### 2. 仅支持 user 身份

该命令仅支持 `user` 身份，使用前需完成 `lark-cli auth login`。只能让当前身份自己离会，无法强制移出其他参会人。

### 3. 当前身份必须在会议中

必须先通过 `+meeting-join` 或其他方式在该会议中，否则接口会报错。

### 4. 写操作不可回放

离会会立刻生效，会议录制、纪要的完整性可能受影响。建议会议任务完成后再调用。

## 输出结果

接口成功返回时，默认输出：`Left meeting <meeting-id> successfully.`。
`--format json` 返回 API 原始响应体。

## 如何获取输入参数

| 输入参数 | 获取方式 |
|---------|---------|
| `meeting-id` | `+meeting-join` 返回的 `meeting.id`；或 `+search` 结果中的 `id` 字段 |

## Agent 组合场景

### 场景 1：加入 → 完成任务 → 离开（最小闭环）

```bash
# 第 1 步：加入会议，记录 meeting.id
lark-cli vc +meeting-join --meeting-number 123456789

# 第 2 步：在会中完成任务（如监听发言、记录信息等）
# ...

# 第 3 步：使用上一步记录的 meeting.id 离会
lark-cli vc +meeting-leave --meeting-id <meeting.id>
```

### 场景 2：会后补拉产物

```bash
# 第 1 步：离会后会议仍在进行或已结束
lark-cli vc +meeting-leave --meeting-id <meeting.id>

# 第 2 步：会议结束后查询录制
lark-cli vc +recording --meeting-ids <meeting.id>

# 第 3 步：查询会议纪要
lark-cli vc +notes --meeting-ids <meeting.id>
```

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--meeting-id is required` | 未传入 `--meeting-id` | 传入从 `+meeting-join` 得到的 `meeting.id` |
| `meeting not found` / `invalid meeting_id` | 误传了 9 位会议号 | 必须使用 `meeting.id`，不是会议号 |
| `not in meeting` | 当前身份并不在该会议中 | 确认先 `+meeting-join` 成功 |
| `missing required scope(s)` | 未授权 `vc:meeting.bot.join:write` | 按提示运行 `auth login --scope vc:meeting.bot.join:write` |

## 提示

- 写操作**不可撤销**，执行前先 `--dry-run` 核对请求体。
- 与 `+meeting-join` 成对使用：能 join 的身份才能 leave。
- 若需要重新入会，再次调用 `+meeting-join`。

## 参考

- [lark-vc-agent-meeting-join](lark-vc-agent-meeting-join.md) — 对应的入会命令
- [lark-vc-agent-meeting-events](lark-vc-agent-meeting-events.md) — 会中事件流
- [lark-vc-search](../../lark-vc/references/lark-vc-search.md) — 搜索历史会议（获取 meeting_id）
- [lark-vc-recording](../../lark-vc/references/lark-vc-recording.md) — 查询 minute_token
- [lark-vc-notes](../../lark-vc/references/lark-vc-notes.md) — 获取会议纪要
- [lark-vc-agent](../SKILL.md) — Agent 参会能力（本 skill）
- [lark-vc](../../lark-vc/SKILL.md) — 视频会议原子域（Meeting / Note 等核心概念）
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
