# 应用机器人参会与会中互动

编排应用机器人的完整会中流程：发现已在参加的会议，或在用户明确授权后发起或加入会议；随后拉取会中事件、发送文本或会中表情、操作倒计时，并仅在用户明确要求时结束会议或离会。

## 选择入口

| 当前条件 | 起点 |
|---|---|
| 已有应用身份取得的 `meeting_id` | 直接拉取事件，不重复查询或入会 |
| 应用机器人可能已在会中 | 已知目标用户 `user_open_id` 时，先用 `+meeting-list-active --as bot --user-id <user_open_id>` 发现会议 |
| 用户明确要求机器人入会、旁听或代参会 | 使用 `+meeting-join --as bot` |
| 用户明确要求机器人发起日程会议 | 使用 `+meeting-join --as bot --action start` |
| 只想查当前用户所在会议 | 使用 [会中事件与会中互动](live-meeting-interact.md) 的用户身份路径，不让应用机器人入会 |

用户只提供 9 位会议号或询问会议内容，不等于授权机器人入会。

## 发现应用机器人已在参加的会议

已知目标用户 `ou_` open_id 时，先查询“目标用户正在参会且应用机器人也在同一会议”的活跃会议：

```bash
lark-cli vc +meeting-list-active --as bot --user-id <user_open_id> --format json
```

- 返回多个会议时，展示主题、会议号和 `meeting_id` 让用户选择；不擅自取第一个。
- 返回空不代表目标用户没有在开会，只表示没有找到应用机器人也在会中的可见会议。
- 用户提供 9 位会议号时，在结果中按 `meeting_no` 匹配；匹配失败时不自动入会。
- 保存选定的长整数 `meeting_id`，后续事件、消息、倒计时和离会命令都沿用 `--as bot`。

身份可见范围、多会议选择和会议号匹配见 [`lark-vc-meeting-list-active`](../references/lark-vc-meeting-list-active.md)。

## 发起或加入会议

只有用户明确要求应用机器人发起、加入、旁听或代参会时才执行。输入是 9 位会议号，不是长整数 `meeting_id`。

```bash
# 发起日程会议并加入
lark-cli vc +meeting-join --as bot --meeting-number <9_digit_meeting_number> --action start

# 加入正在进行的会议
lark-cli vc +meeting-join --as bot --meeting-number <9_digit_meeting_number>
```

- 入会前确认目标会议号和用户意图；这是对其他参会人可见的写操作。
- `--action start` 仅用于发起符合条件的日程会议；未传时保持加入正在进行的会议。
- 保存返回的 `meeting.id`；后续邀请、拉取事件、发送会中消息、操作倒计时、结束或离会都使用该 ID 与 `--as bot`。
- 应用机器人可以同时加入多场会议；加入新会议前不需要退出其他会议。
- 根据返回状态确认入会成功，不要把“请求已发起”当作已入会。

会议密码、等候室、写操作风险和异常恢复见 [`lark-vc-agent-meeting-join`](../references/lark-vc-agent-meeting-join.md)。

## 邀请参会人

只有用户明确要求邀请时才执行。输入是长数字 `meeting_id`，不是 9 位会议号。

```bash
# 邀请指定用户
lark-cli vc +meeting-invite --as bot --meeting-id <meeting_id> --type SELECTED --open-ids <open_id>

# 邀请全部合格日程参会人
lark-cli vc +meeting-invite --as bot --meeting-id <meeting_id> --type ALL_SUGGESTED
```

- 应用机器人必须已在目标 Calendar VC 中。
- `SELECTED` 接收用户 `open_id`；`ALL_SUGGESTED` 由服务端筛选合格日程参会人。
- 以返回结果确认邀请状态，不把请求提交当作参会人已入会。

邀请类型、人数上限和结果语义见 [`lark-vc-agent-meeting-invite`](../references/lark-vc-agent-meeting-invite.md)。

## 拉取会中事件

使用应用身份发现或入会得到的 `meeting_id`：

```bash
lark-cli vc +meeting-events --as bot --meeting-id <meeting_id> --page-all --format pretty
```

- 默认使用 `--page-all` 拉取当前完整事件流，并保留返回的 `page_token` 供后续增量查询。
- 回答“现在、刚刚、最新”或总结当前会议前，重新拉取最新事件；不直接复用旧快照。
- 应用机器人必须在会中，或在会议结束后的可见宽限窗口内曾经参会；不要用任意 `meeting_id` 尝试读取。
- 会中事件不能替代已结束会议的参会人快照、纪要、逐字稿或录制。

事件类型、分页、结束后五分钟窗口和文档上下文处理见 [`lark-vc-meeting-events`](../references/lark-vc-meeting-events.md)。

## 发送会中文本或表情

每次发送都是对会中参会人可见的写操作。只有用户明确要求发送，并已确认目标会议和内容时才执行。

```bash
# 文本消息
lark-cli vc +meeting-message-send --as bot --meeting-id <meeting_id> --msg-type text --text "<message>"

# 普通会中表情
lark-cli vc +meeting-message-send --as bot --meeting-id <meeting_id> --msg-type reaction --emoji-type THUMBSUP
```

- 始终沿用产生 `meeting_id` 的应用身份；不要切换成用户身份。
- reaction 必须使用 Reference 中大小写敏感的完整 `emoji_type` 列表；不编造 key。
- 发送失败时停止并报告；不自动重试或换身份，避免产生重复可见消息。
- 用户要发绑定群或 IM 消息时改用 `lark-im`，不使用会中消息命令。

文本、reaction 语义、完整 emoji key 和幂等参数见 [`lark-vc-meeting-message-send`](../references/lark-vc-meeting-message-send.md)。

## 操作会中倒计时

每次倒计时操作都是对会中参会人可见的写操作。只有用户明确要求设置、延长、提前结束或关闭倒计时时才执行。

```bash
# 设置倒计时
lark-cli vc +meeting-countdown --as bot --meeting-id <meeting_id> --action set --duration <minutes>

# 延长倒计时
lark-cli vc +meeting-countdown --as bot --meeting-id <meeting_id> --action prolong --duration <minutes>
```

- 始终沿用产生 `meeting_id` 的应用身份；不要切换成用户身份。
- 用户只给 9 位会议号时，先按应用身份活跃会议列表匹配；匹配失败时不要为了倒计时自动入会，除非用户明确要求机器人入会。
- `end_in_advance` 和 `close_window` 不携带 `--duration`、提醒点或结束音频参数。
- 操作失败时停止并报告；不自动重试或换身份，避免重复可见副作用。

动作、提醒点和权限规则见 [`lark-vc-meeting-countdown`](../references/lark-vc-meeting-countdown.md)。

## 结束会议

只有用户明确要求结束整场会议时才执行；不要把结束会议和机器人离会混用。

```bash
lark-cli vc +meeting-end --as bot --meeting-id <meeting_id> --yes
```

- 输入是长数字 `meeting_id`。
- 当前应用机器人必须是 Host；结束成功会结束整场会议。
- 根据返回状态确认会议已结束。

身份、权限和失败原因见 [`lark-vc-agent-meeting-end`](../references/lark-vc-agent-meeting-end.md)。

## 离开会议

只有用户明确要求机器人退出、离开或结束参会时才执行：

```bash
lark-cli vc +meeting-leave --as bot --meeting-id <meeting_id>
```

- 使用入会返回或应用身份活跃会议查询得到的 `meeting_id`，并确认机器人当前在该会议中。
- 不要因为任务完成而自动离会。
- 用户只要会后产物时，转入会议产物场景，不为此先执行离会。
- 根据返回状态确认离会完成。

离会参数、可见副作用和完成判定见 [`lark-vc-agent-meeting-leave`](../references/lark-vc-agent-meeting-leave.md)。

## 应用身份权限配置检查

应用身份返回 `no permission`、`missing required scope(s)` 或 `missing_scopes` 时，不要执行 `auth login`。按顺序检查：

1. 按 CLI 错误中的 `hint` 处理；返回 `console_url` 时将其原样提供给用户。
2. 确认应用已开通对应权限，已发布并安装到当前租户。入会和应用身份会议查询需要 `vc:meeting.bot.join:write`；会中发消息需要 `vc:meeting.message:write`；会中倒计时需要 `vc:meeting.interaction:write`。
3. 在开放平台确认“权限可访问的数据范围”已保存为“按条件筛选”，条件为“会议的归属者 包含 与应用的可用范围一致”。
4. 上述配置均正确仍失败时，保留 CLI 返回的错误码和 `log_id`，按服务端权限异常排查；不要反复登录或改用其他身份重试。

## 会后边界

- 已结束会议的搜索、参会人快照、智能纪要、逐字稿、妙记或录制，转入 [查询会议及其产物](query-meeting-and-artifacts.md)。
- 会后要把产物发到群或私聊，先使用会议产物场景获取结果，再转 `lark-im`。
