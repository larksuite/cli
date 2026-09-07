
# vc +meeting-leave

通过 `meeting_id` 离开当前身份所在的视频会议（bot leave）。这是一次**写操作**，会实际把当前身份从会议中移出。

本 skill 对应 shortcut：`lark-cli vc +meeting-leave`（调用 `POST /open-apis/vc/v1/bots/leave`）。

## 命令

```bash
# 通过 meeting_id 离会
lark-cli vc +meeting-leave --as bot --meeting-id 69xxxxxxxxxxxxx28
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--meeting-id <id>` | 是 | 会议 ID（**不是 9 位会议号**） |
| `--dry-run` | 否 | 预览 API 调用，不实际离会；meeting_id 或身份不确定时先用它确认请求 |

## 核心约束

### 1. 入参是 meeting_id，不是会议号

`--meeting-id` 必须是会议的长数字 ID，通常由 `+meeting-join --as bot` 返回体中的 `meeting.id` 提供，也可从应用身份 `+meeting-list-active --as bot --user-id <user_open_id>` 返回体中的 `meeting_id` 获取。**传 9 位会议号会失败**。

### 2. 优先使用 bot 身份

这是应用机器人离会能力，使用与入会或 active meeting 发现相同的 `--as bot`。只能让当前身份自己离会，无法强制移出其他参会人。

### 3. 当前身份必须在会议中

应用机器人必须已经在该会议中，否则接口会报错。如果 `meeting_id` 来自 `+meeting-list-active`，必须确认这是应用身份发现到的会议。

### 4. 离会立即生效，对其他参会人可见

机器人会立刻从参会列表消失；若会议启用了录制/纪要，bot 的参会时段到此截止。只有在用户明确要求退出 / 离开 / 结束参会时才调用；如需要重新入会，再跑 `+meeting-join` 即可（非真正"不可逆"）。

## 输出结果

接口成功返回时，默认输出：`Left meeting <meeting-id> successfully.`。
`--format json` 返回标准 `{ok, identity, data}` 信封，例如 `{"ok":true,"identity":"bot","data":{}}`，不是带 `code` / `msg` 的 API 原始响应体。

## 如何获取输入参数

| 输入参数 | 获取方式 |
|---------|---------|
| `meeting-id` | `+meeting-join --as bot` 返回的 `meeting.id`；或应用身份 `+meeting-list-active --as bot --user-id <user_open_id>` 返回的 `meeting_id` |

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--meeting-id is required` | 未传入 `--meeting-id` | 传入从 `+meeting-join --as bot` 得到的 `meeting.id`，或应用身份 `+meeting-list-active` 返回的 `meeting_id` |
| `meeting not found` / `invalid meeting_id` | 误传了 9 位会议号 | 必须使用 `meeting.id`，不是会议号 |
| `not in meeting` | 当前身份并不在该会议中 | 确认先 `+meeting-join` 成功 |

## 提示

- 只有用户明确要求退出 / 离开 / 结束参会时才调用；离会会让机器人从参会列表消失，对其他参会人可见。若需要重新入会直接再 `+meeting-join`，不是真正的"不可逆"。参数格式不确定时可选 `--dry-run` 预览。
- `+meeting-leave` 优先使用 `+meeting-join --as bot` 返回的 `meeting.id`，但不是每次 join 后都必须调用 leave。
- `meeting_id` 如果来自 `+meeting-list-active`，必须来自应用身份，并确认应用机器人就在该会议中。不要用 9 位会议号。

## 相关场景
- [应用机器人参会与会中互动](../scenes/live-meeting-attend.md)
