# vc +meeting-list-active

列出当前进行中的会议，用来发现 `+meeting-events` 需要的长数字 `meeting_id`。

本 skill 对应 shortcut：`lark-cli vc +meeting-list-active`（调用 `GET /open-apis/vc/v1/bots/user_active_meeting`）。

## 命令

```bash
# 查询当前登录用户正在参加的会议
lark-cli vc +meeting-list-active --as user --format json

```

## 身份

使用 AAT 的 `--as user` 通道查询当前用户会议，不传 `--user-id`。后续事件查询延续同一身份。

## 多会议选择

- 如果返回多个会议，不要自动挑第一个。
- 向用户展示每个候选的 `meeting_title` / `meeting_no` / `meeting_id`，等待用户选择。
- 选择后用同一身份执行 `+meeting-events` 读取事件。

## 9 位会议号匹配

用户提供 9 位会议号但没有明确要求当前身份入会时，把会议号当作 active meeting 的筛选条件，而不是写操作指令。

匹配规则：

- 在返回会议中匹配 `meeting_no == <9位会议号>`。
- 匹配到唯一会议：取该项的长数字 `meeting_id`，后续用同一身份调用 `+meeting-events`。
- 匹配到多个会议：展示候选，让用户选择。
- 没有匹配：说明当前身份没有发现该会议号对应的 active meeting；不要自动调用 `+meeting-join`，除非用户明确要求当前身份入会。

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| 用户身份返回空列表 | 当前登录用户没有可见的进行中会议 | 确认用户是否在会中，或是否切错身份 |
| 用户身份无权限 / 不可见 | 当前身份无法读取该会议 | 确认用户是否在会中；认证与权限问题按 lark-shared 处理，不切换身份重试 |

## 相关场景
- [会中事件与会中互动](../scenes/live-meeting-interact.md)
- [用户身份参会与会中互动](../scenes/live-meeting-attend.md)
