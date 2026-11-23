# vc +meeting-list-active

列出当前进行中的会议，用来发现 `+meeting-events` 需要的长数字 `meeting_id`。

本 skill 对应 shortcut：`lark-cli vc +meeting-list-active`（调用 `GET /open-apis/vc/v1/bots/user_active_meeting`）。

## 命令

```bash
# 查询当前登录用户正在参加的会议
lark-cli vc +meeting-list-active --as user --format json

# 查询指定用户当前参加、且应用机器人也在会中的会议
lark-cli vc +meeting-list-active --as bot --user-id ou_xxx --format json
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--user-id <id>` | 应用身份必填 | 目标用户 open_id，格式为 `ou_...`。用户身份不传；应用身份直接透传给接口，不接受 internal user_id 或数字 ID |

## 身份语义

不要向用户暴露内部身份缩写；对用户只说“用户身份”或“应用身份”。

| 身份 | 命令 | 返回范围 | 后续事件读取 |
| ---- | ---- | -------- | ------------ |
| 用户身份 | `--as user` | 当前登录用户正在参加的会议 | 继续 `+meeting-events --as user` |
| 应用身份 | `--as bot --user-id <user_open_id>` | 目标用户正在参加、且应用机器人也在会中的会议 | 继续 `+meeting-events --as bot` |

硬规则：`meeting_id` 从哪种身份路径拿到，后续 `+meeting-events` 就沿用哪种身份。不要把应用身份拿到的 `meeting_id` 改用用户身份读事件，也不要把用户身份拿到的 `meeting_id` 强制切到应用身份。

应用身份返回空，不代表目标用户不在任何会议中，只能说明没有找到“目标用户在会中且应用机器人也在会中”的当前会。

## 多会议选择

- 如果返回多个会议，不要自动挑第一个。
- 向用户展示每个候选的 `meeting_title` / `meeting_no` / `meeting_id`，等待用户选择。
- 选择后用同一身份执行 `+meeting-events` 读取事件。

## 9 位会议号匹配

用户提供 9 位会议号但没有明确要求应用机器人入会时，把会议号当作 active meeting 的筛选条件，而不是写操作指令。

匹配规则：

- 在返回会议中匹配 `meeting_no == <9位会议号>`。
- 匹配到唯一会议：取该项的长数字 `meeting_id`，后续用同一身份调用 `+meeting-events`。
- 匹配到多个会议：展示候选，让用户选择。
- 没有匹配：说明当前身份没有发现该会议号对应的 active meeting；不要自动调用 `+meeting-join`，除非用户明确要求应用机器人入会。

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--user-id is required when --as bot` | 应用身份未传目标用户 | 传入目标用户 open_id |
| 用户身份返回空列表 | 当前登录用户没有可见的进行中会议 | 确认用户是否在会中，或是否切错身份 |
| 用户身份无权限 / 不可见 | 当前登录用户没有可见的进行中会议，或当前身份无法读取该会议 | 不要反复执行 `auth login`。确认用户是否在会中、是否切错 profile；用户明确要查询应用机器人可见的会议时，再拿目标用户 open_id 执行 `+meeting-list-active --as bot --user-id <user_open_id>` |
| 应用身份返回空列表 | 没有满足“目标用户在会中且应用机器人也在会中”的当前会 | 先让应用机器人入会，或确认 `user_id` 和会议状态 |
| `--user-id` 格式错误 | 传入了 internal user_id 或其他非 `ou_...` 值 | 改传目标用户 open_id |
| 应用身份权限不足 | 应用权限、租户安装或权限可访问的数据范围未配置完整 | 不要执行 `auth login`。请应用开发者开通 `vc:meeting.bot.join:write`；再检查应用发布/安装和权限可访问的数据范围；配置正确仍失败时，保留错误码和 `log_id`，按服务端权限异常排查 |

## 相关场景
- [会中事件与会中互动](../scenes/live-meeting-interact.md)
- [应用机器人参会与会中互动](../scenes/live-meeting-attend.md)
