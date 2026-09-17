# 用户身份参会与会中互动

通过 AAT 的 `--as user` 通道编排会议生命周期操作。

## 选择入口

| 当前条件 | 起点 |
|---|---|
| 用户明确要求以用户身份入会 | 使用 `+meeting-join --as user` |
| 用户明确要求以用户身份发起日程会议 | 使用 `+meeting-join --as user --action start` |
| 只想查当前用户所在会议 | 使用 [会中事件与会中互动](live-meeting-interact.md) 的用户身份路径，不入会 |

用户只提供 9 位会议号或询问会议内容，不等于授权机器人入会。

## 发起或加入会议

只有用户明确要求以用户身份发起、加入、旁听或代参会时才执行。输入是 9 位会议号，不是长整数 `meeting_id`。

```bash
# 用户身份发起日程会议并加入
lark-cli vc +meeting-join --as user --meeting-number <9_digit_meeting_number> --action start

# 用户身份加入正在进行的会议
lark-cli vc +meeting-join --as user --meeting-number <9_digit_meeting_number>
```

- 入会前确认目标会议号和用户意图；这是对其他参会人可见的写操作。
- `--action start` 仅用于发起符合条件的日程会议；未传时保持加入正在进行的会议。
- 保存返回的 `meeting.id` 和来源身份；后续操作必须沿用该身份，不要切换身份。
- 根据返回状态确认入会成功，不要把“请求已发起”当作已入会。

会议密码、等候室、写操作风险和异常恢复见 [`lark-vc-agent-meeting-join`](../references/lark-vc-agent-meeting-join.md)。

## 邀请参会人

只有用户明确要求邀请时才执行。输入是长数字 `meeting_id`，不是 9 位会议号。

```bash
# 用户身份邀请指定用户
lark-cli vc +meeting-invite --as user --meeting-id <meeting_id> --type SELECTED --open-ids <open_id>
```

- 当前身份必须已在目标会议中。
- `SELECTED` 接收用户 `open_id`；`--as user` 路径只支持 `SELECTED`。
- 以返回结果确认邀请状态，不把请求提交当作参会人已入会。

邀请类型、人数上限和结果语义见 [`lark-vc-agent-meeting-invite`](../references/lark-vc-agent-meeting-invite.md)。

## 拉取会中事件

使用当前用户身份发现或入会得到的 `meeting_id`：

```bash
lark-cli vc +meeting-events --as user --meeting-id <meeting_id> --page-all --format pretty
```

- 默认使用 `--page-all` 拉取当前完整事件流，并保留返回的 `page_token` 供后续增量查询。
- 回答“现在、刚刚、最新”或总结当前会议前，重新拉取最新事件；不直接复用旧快照。
- 当前身份必须具备会议可见权限；不要用任意 `meeting_id` 尝试读取。
- 会中事件不能替代已结束会议的参会人快照、纪要、逐字稿或录制。

事件类型、分页、结束后五分钟窗口和文档上下文处理见 [`lark-vc-meeting-events`](../references/lark-vc-meeting-events.md)。

## 发送会中文本或表情

每次发送都是对会中参会人可见的写操作。只有用户明确要求发送，并已确认目标会议和内容时才执行。

```bash
# 文本消息
lark-cli vc +meeting-message-send --as user --meeting-id <meeting_id> --msg-type text --text "<message>"

# 普通会中表情
lark-cli vc +meeting-message-send --as user --meeting-id <meeting_id> --msg-type reaction --emoji-type THUMBSUP
```

- 始终沿用产生 `meeting_id` 的用户身份。
- reaction 必须使用 Reference 中大小写敏感的完整 `emoji_type` 列表；不编造 key。
- 发送失败时停止并报告；不自动重试或换身份，避免产生重复可见消息。
- 用户要发绑定群或 IM 消息时改用 `lark-im`，不使用会中消息命令。

文本、reaction 语义、完整 emoji key 和幂等参数见 [`lark-vc-meeting-message-send`](../references/lark-vc-meeting-message-send.md)。

## 操作会中倒计时

每次倒计时操作都是对会中参会人可见的写操作。只有用户明确要求设置、延长、提前结束或关闭倒计时时才执行。

```bash
# 设置倒计时
lark-cli vc +meeting-countdown --as user --meeting-id <meeting_id> --action set --duration <minutes>

# 延长倒计时
lark-cli vc +meeting-countdown --as user --meeting-id <meeting_id> --action prolong --duration <minutes>
```

- 始终沿用产生 `meeting_id` 的用户身份。
- 用户只给 9 位会议号时，先按当前用户身份活跃会议列表匹配；匹配失败时不要为了倒计时自动入会，除非用户明确要求机器人入会。
- `end_in_advance` 和 `close_window` 不携带 `--duration`、提醒点或结束音频参数。
- 操作失败时停止并报告；不自动重试或换身份，避免重复可见副作用。

动作、提醒点和权限规则见 [`lark-vc-meeting-countdown`](../references/lark-vc-meeting-countdown.md)。

## 结束会议

只有用户明确要求结束整场会议时才执行；不要把结束会议和机器人离会混用。

```bash
# 用户身份结束整场会议
lark-cli vc +meeting-end --as user --meeting-id <meeting_id> --yes

```

- 输入是长数字 `meeting_id`。
- 常规进行中会议中，当前用户必须是当前 Host 并具备结束目标会议的权限；服务端按会议状态和权限作最终判定。
- 结束成功会结束整场会议。
- 根据返回状态确认会议已结束。
- 沿用当前身份，不得为了执行成功切换身份。

身份、权限和失败原因见 [用户身份](../references/lark-vc-meeting-end.md)。

## 离开会议

只有用户明确要求当前身份退出、离开或结束参会时才执行：

```bash
# 用户身份离会
lark-cli vc +meeting-leave --as user --meeting-id <meeting_id>
```

- 使用入会返回或活跃会议查询得到的 `meeting_id`，确认当前身份在该会议中，并沿用入会身份。
- 不要因为任务完成而自动离会。
- 用户只要会后产物时，转入会议产物场景，不为此先执行离会。
- 根据返回状态确认离会完成。

离会参数、可见副作用和完成判定见 [`lark-vc-agent-meeting-leave`](../references/lark-vc-agent-meeting-leave.md)。

## 会后边界

- 已结束会议的搜索、参会人快照、智能纪要、逐字稿、妙记或录制，转入 [查询会议及其产物](query-meeting-and-artifacts.md)。
- 会后要把产物发到群或私聊，先使用会议产物场景获取结果，再转 `lark-im`。
