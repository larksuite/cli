
# vc +meeting-join

通过 9 位会议号让应用机器人或用户身份加入一场正在进行的视频会议，也可以发起符合条件的 Calendar 会议。这是一次**写操作**，会实际让当前身份加入会议。

本 skill 对应 shortcut：`lark-cli vc +meeting-join`（调用 `POST /open-apis/vc/v1/bots/join`）。

应用机器人使用 `--as bot`；用户身份使用 `--as user`。两种身份在加入或通过 `--action start` 发起日程会议时，AI 总结未开启和“允许智能体加入会议”未开启都返回 `121016` 同一长引导；AI 总结配额不足返回 `121020`；当前身份不允许发起日程会议返回 `121021`。

> **不要把 9 位会议号等同于入会意图。** 用户给出 9 位会议号并询问“会议讲了什么 / 查会中事件”时，先用 `vc +meeting-list-active` 查当前 active meetings 并按 `meeting_no` 匹配；只有用户明确要求“入会 / 让应用机器人旁听 / 代我参会”时才调用本命令。

## 命令

```bash
# 应用机器人加入正在进行的会议
lark-cli vc +meeting-join --as bot --meeting-number 123456789

# 应用机器人发起日程会议
lark-cli vc +meeting-join --as bot --meeting-number 123456789 --action start

# 用户身份加入正在进行的会议
lark-cli vc +meeting-join --as user --meeting-number 123456789

# 用户身份发起日程会议
lark-cli vc +meeting-join --as user --meeting-number 123456789 --action start

# 可选：用户明确要求智能体用户在会中等候室等待主持人准入时
lark-cli vc +meeting-join --as user --meeting-number 123456789 --enable-lobby
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--meeting-number <no>` | 是 | 会议号，必须为 **9 位纯数字** |
| `--password <pw>` | 否 | 会议密码，仅在该会议设置了入会密码时传入 |
| `--call-id <id>` | 否 | 从 `vc.bot.meeting_invited_v1` 邀请事件透传的 `call_id`，原样回传即可。当前身份主动入会或无邀请事件来源时不传 |
| `--action join\|start` | 否 | 默认 `join`，保持普通入会链路；`start` 是日程发起专属参数，应用机器人和用户身份均可使用 |
| `--enable-lobby` | 否 | 可选项，默认不传。仅智能体用户（`lark-cli whoami` 显示 `principal` 为 `agent_employee`）以 `--as user` 加入进行中的会议时生效：需要审批的会议会进入会中等候室，而不是直接失败。只在用户或调用方明确要求时传入；不能与 `--action start` 或 `--as bot` 同时使用。见[会中等候室](#5-会中等候室--enable-lobby可选) |
| `--dry-run` | 否 | 预览 API 调用，不实际加入会议；会议号或身份不确定时先用它确认请求 |

## 核心约束

### 1. 沿用来源身份

- 应用机器人使用 `--as bot`。
- 用户身份使用 `--as user`，不要为了执行成功切换为应用身份。
- CLI 只负责沿用所选身份，具体会议权限由服务端校验。

默认 `join` 不会向请求体写入 `action` 字段，也不进入日程发起筛选。`--action start` 才写入 `action: 2`，单独进入日程发起的筛选与校验。

### 2. 会议号格式严格校验

`--meeting-number` 必须是 9 位纯数字，否则本地校验直接报错：
`--meeting-number must be exactly 9 digits`。

常见错误来源：
- 把会议链接整条粘进来（应仅取尾部的 9 位数字）
- 把 `meeting_id`（长数字 ID）当成会议号传入（两者不是同一个东西）

### 3. 会议必须已开始且允许入会

- `--action join` 要求会议处于**进行中**状态；`--action start` 用于启动符合条件的日程会议。
- 默认入会请求不进入等候室；若会议要求审批，或当前身份不具备直接入会权限，会直接失败，需要主持人调整会控或入会权限后重试。不要因为这类失败自动加上 `--enable-lobby` 重试（见[会中等候室](#5-会中等候室--enable-lobby可选)）。
- `--action start` 返回 `HTTP 403: no permission`（错误码 `121003`）表示当前身份未通过日程发起的通用权限或灰度校验；不要将此错误当作“允许智能体加入会议”开关关闭，也不要通过新增 scope 解决。
- 默认 `--action join` 返回 `121003` 时，再优先检查所选身份：`--as bot` 需确认应用为企业自建应用、已获授“机器人入离会”权限，且应用与租户一致；`--as user` 需确认当前用户身份具备基础入会能力。会议号、密码、会议状态和会议准入拒绝分别有对应的专属错误码。

### 4. 机器人入会后对其他参会人可见

这是一次真实入会操作，机器人会立即出现在参会人列表中，其他参会人可见，并产生会议日志。误入错会的社交成本高于技术成本——执行前优先确认 9 位会议号的来源（用户输入 / 会议链接末尾），不要臆造。参数格式有疑问时可用 `--dry-run` 预览请求体。

### 5. 会中等候室（`--enable-lobby`，可选）

`--enable-lobby` 是按需开启的选项，不是智能体用户入会的默认行为。不传时入会结果与原来一致：正式入会，或直接返回失败。

- **何时使用**：只有用户或调用方明确要求“进入等候室等待主持人准入”时才传入。不要为了提高入会成功率主动添加，也不要在入会失败后自动带上它重试——传入后返回结果可能变为“等待中”，会改变调用方对入会结果的预期。
- **适用范围**：仅智能体用户以 `--as user` 加入进行中的会议时生效；与 `--as bot` 或 `--action start` 同用会在本地直接报错。是否具备候会资格由服务端判定。
- **两种结果**：返回 `join_status=1` 表示已正式入会；`join_status=2` 表示已进入等候室、**尚未入会**，此时只返回 `lobby_meeting.meeting_id`，不要调用 `+meeting-events`、`+meeting-message-send` 等会中命令。
- **等待期间**：每轮等待默认 10 分钟，重复执行 `+meeting-join` 不会延长等待时间，也不要循环调用。主持人准入后服务端自动完成入会，无需再次执行本命令。
- **状态通知**：等候室状态变化通过 `vc.meeting.lobby_changed_v1` 事件只投递给当前身份，`status` 取值为 `waiting`（`reason` 为 `joined_lobby` 或 `host_moved`）、`rejected`、`expired`、`left`（`participant_left` 或 `capability_disabled`）、`meeting_ended`；准入并正式入会后沿用普通入会事件。收到 `rejected` / `expired` / `left` / `meeting_ended` 后停止等待并告知用户，重新进入等候室需要用户再次确认。
- **会中被移入等候室**：以 `--enable-lobby` 入会后，主持人可以把当前身份移回等候室（`reason=host_moved`），此时会中命令会失败，等待再次准入即可。
- **退出等候室**：用户要求放弃等待时，使用 `+meeting-leave --as user --meeting-id <lobby_meeting.meeting_id>`。

## 输出结果

接口返回会议基本信息，字段视具体响应而定，常见字段：

| 字段 | 说明 |
|------|------|
| `join_status` | 仅在传入 `--enable-lobby` 时返回：`1` 已正式入会，`2` 在会中等候室等待准入 |
| `lobby_meeting.meeting_id` | 仅在 `join_status=2` 时返回，等候中的会议 ID；此时没有 `meeting` 对象 |
| `join_user` | 本次入会使用的身份 |
| `meeting.id` | 会议 ID（可后续传给 `+meeting-leave`，并沿用本次入会的 `--as` 身份） |
| `meeting.meeting_no` | 会议号（与入参一致） |
| `meeting.topic` | 会议主题 |
| `meeting.start_time` | 会议开始时间 |

> **重要**：拿到 `meeting.id`（或等候时的 `lobby_meeting.meeting_id`）后务必保留，退出会议或等候室（`+meeting-leave`）需要使用它，而不是会议号。`join_status=2` 时 pretty 输出为 `Waiting in the meeting lobby for host approval.`，不代表已入会。

## 如何获取输入参数

| 输入参数 | 获取方式 |
|---------|---------|
| `meeting-number` | 会议号由主持人分享；也可从会议链接尾部解析 9 位数字 |
| `password` | 若会议设置了入会密码，由主持人提供 |
| `call-id` | 由 `vc.bot.meeting_invited_v1` 邀请事件的 `call_id` 字段携带，Agent 收到事件时透传过来；无邀请事件场景（如 Agent 主动入会）不传 |

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--meeting-number must be exactly 9 digits` | 会议号不是 9 位纯数字 | 检查是否误传了会议链接或 meeting_id |
| `--enable-lobby cannot be used with --action start` | 发起日程会议不支持会中等候室 | 发起会议时去掉 `--enable-lobby`；只在加入进行中的会议时使用 |
| `--enable-lobby is not supported for bot identity` | 应用机器人身份不支持会中等候室 | 去掉 `--enable-lobby`；不要为了进入等候室切换 `--as user` / `--as bot` 身份 |
| `121016`: `This agent is currently unable to join the meeting. Ask the meeting host to enable "AI Summary" and try again. If it is already enabled, verify that "Allow agents to join meetings" is enabled in the current meeting's "Security" settings.` | AI 总结未开启，或当前会议“允许智能体加入会议”开关未开启 | 按 message 顺序处理：先请会议主持人开启 AI 总结；如果已开启，再到当前会议“安全”中确认“允许智能体加入会议”已开启。不要切换 `--as user` / `--as bot` 身份绕过该限制 |
| `121020`: `AI summary quota is insufficient` | 组织的 AI 总结配额不足 | 保留错误码和 `log_id`，联系组织管理员确认 AI 总结配额；配额恢复后再执行 `--action start`，不要自动重试 |
| `121009 need password` / `121010 wrong password` | 未提供会议密码或密码错误 | 向主持人确认会议密码后传入 `--password`；不要高频重试 |
| `121004 meeting status unexpected` / `121005 meeting not exist` / `121014 meeting not started` | 会议状态不支持入会、会议号无效，或会议尚未开始 | 核对会议号和当前飞书环境；等待会议开始后重试。日程会议发起使用 `--action start` |
| `121021`: `You are not permitted to start this scheduled meeting.` | 当前身份不允许发起该日程会议 | 核对调用身份是否具备日程发起资格；保留 `log_id` 排查具体业务权限或灰度条件 |
| `HTTP 403: no permission` / `121003` | `--action start` 时，当前身份未通过日程发起的通用权限或灰度校验；默认 `join` 时，应用身份、权限或租户一致性不满足，或当前用户身份未通过基础入会能力校验 | `start`：确认调用身份具备日程发起能力，并保留 `log_id` 排查灰度或业务权限；不要新增 OAuth scope，也不要将此错误当作智能体开关关闭。`join`：`--as bot` 确认应用为企业自建应用、已获授“机器人入离会”权限且应用与租户一致；`--as user` 确认当前用户身份具备基础入会能力 |
| `121013 no permission to join` | 当前身份未被允许直接加入该会议 | 确认当前身份已受邀，且会议安全策略允许其加入；必要时请主持人调整入会权限。不要通过切换 `--as user` / `--as bot` 身份绕过限制 |
| 应用身份权限不足 | 应用权限、租户安装或权限可访问的数据范围未配置完整 | 不要执行 `auth login`。以 CLI 返回的 metadata / error envelope 为准确认缺失权限；检查应用发布/安装，以及开放平台“权限可访问的数据范围”：选择“按条件筛选”，条件为“会议的归属者 包含 与应用的可用范围一致”；配置正确仍失败时，保留错误码和 `log_id`，按服务端权限异常排查 |
| 用户身份操作失败 | 当前 `--as user` 身份不具备目标会议的操作权限，或目标会议不支持该操作 | 保留错误码和 `log_id`，检查当前用户身份，不要改用 `--as bot` 重试 |

## 提示

- 仅在 Agent 需要**真实加入**会议（例如参会机器人、会中助手）时使用；只拉取会议数据不需要入会。
- 入会会让当前身份立即出现在参会列表；若用户要求退出 / 离开 / 结束参会，使用 `+meeting-leave --as <source_identity> --meeting-id <meeting.id>`。参数格式不确定时可选 `--dry-run` 预览，但不是必经步骤。
- 执行成功后，立即记录返回的 `meeting.id`，用于后续 `+meeting-leave` / `+meeting-events`；返回 `join_status=2` 时记录 `lobby_meeting.meeting_id`，只能用于退出等候室，入会前不要用它调用会中命令。

## 相关场景
- [应用机器人参会与会中互动](../scenes/live-meeting-attend.md)
