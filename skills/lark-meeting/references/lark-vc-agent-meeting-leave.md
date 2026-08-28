
# vc +meeting-leave

通过 `meeting_id` 让应用机器人或 Agent Employee 离开当前身份所在的视频会议。这是一次**写操作**，会实际把当前身份从会议中移出。

本 skill 对应 shortcut：`lark-cli vc +meeting-leave`（调用 `POST /open-apis/vc/v1/bots/leave`）。

## 命令

```bash
# 应用机器人通过 meeting_id 离会
lark-cli vc +meeting-leave --as bot --meeting-id 69xxxxxxxxxxxxx28

# Agent Employee 使用 AAT 通过 meeting_id 离会
lark-cli vc +meeting-leave --as user --meeting-id 69xxxxxxxxxxxxx28
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--meeting-id <id>` | 是 | 会议 ID（**不是 9 位会议号**） |
| `--dry-run` | 否 | 预览 API 调用，不实际离会；meeting_id 或身份不确定时先用它确认请求 |

## 核心约束

### 1. 入参是 meeting_id，不是会议号

`--meeting-id` 必须是会议的长数字 ID，通常由 `+meeting-join` 返回体中的 `meeting.id` 提供，也可从同一身份可见的 active meeting 中获取。**传 9 位会议号会失败**。

### 2. 沿用入会身份

- 应用机器人使用 `--as bot`。
- Agent Employee 使用 AAT 时必须使用 `--as user`。
- 离会必须沿用发起或入会时的身份，不要在 AAT 与应用 Bot 之间切换。该命令只能让当前身份自己离会，无法强制移出其他参会人。

### 3. 当前身份必须在会议中

当前身份必须已经在该会议中，否则接口会报错。如果 `meeting_id` 来自 active meeting 查询，必须确认它属于当前来源身份。

### 4. 离会立即生效，对其他参会人可见

当前应用机器人或 Agent Employee 会立刻从参会列表消失；若会议启用了录制/纪要，该身份的参会时段到此截止。只有在用户明确要求退出 / 离开 / 结束参会时才调用；如需要重新入会，再跑 `+meeting-join` 即可（非真正"不可逆"）。

## 输出结果

接口成功返回时，默认输出：`Left meeting <meeting-id> successfully.`。
`--format json` 返回标准 `{ok, identity, data}` 信封，例如 `{"ok":true,"identity":"user","data":{}}`，不是带 `code` / `msg` 的 API 原始响应体。

## 如何获取输入参数

| 输入参数 | 获取方式 |
|---------|---------|
| `meeting-id` | 同一来源身份执行 `+meeting-join` 返回的 `meeting.id`，或该身份可见的 active meeting 所返回的 `meeting_id` |

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--meeting-id is required` | 未传入 `--meeting-id` | 传入从同一身份执行 `+meeting-join` 得到的 `meeting.id`，或该身份可见的 active meeting 所返回的 `meeting_id` |
| `meeting not found` / `invalid meeting_id` | 误传了 9 位会议号 | 必须使用 `meeting.id`，不是会议号 |
| `not in meeting` | 当前身份并不在该会议中 | 确认先 `+meeting-join` 成功 |
| AAT 身份不匹配 | `--as user` 当前 token 不是可用的 Agent Employee AAT | 检查当前 user credential，保留错误码和 `log_id`，不要改用 `--as bot` 重试 |

## 提示

- 只有用户明确要求退出 / 离开 / 结束参会时才调用；离会会让机器人从参会列表消失，对其他参会人可见。若需要重新入会直接再 `+meeting-join`，不是真正的"不可逆"。参数格式不确定时可选 `--dry-run` 预览。
- `+meeting-leave` 优先使用同一身份执行 `+meeting-join` 返回的 `meeting.id`，但不是每次 join 后都必须调用 leave。
- `meeting_id` 如果来自 active meeting 查询，必须来自当前来源身份，并确认该身份就在会议中。不要用 9 位会议号。

## 相关场景
- [应用机器人参会与会中互动](../scenes/live-meeting-attend.md)
