# 读取会中事件、会中互动与主持管理

围绕一场正在进行的会议执行只读查询或用户明确授权的会中写操作。真实入会/离会使用应用机器人入会场景；已结束会议和会后产物使用会议查询场景。

如果任务包含“应用机器人入会后继续拉取事件或互动”，只读取并执行 [应用机器人参会与会中互动](live-meeting-attend.md) 的完整流程，不要在两个场景之间来回切换。

## 发现进行中的会议

没有 `meeting_id` 时，按用户需要的视角查询：

```bash
# 当前登录用户正在参加的会议
lark-cli vc +meeting-list-active --as user --format json

# 目标用户正在参加、且应用机器人也在会中的会议
lark-cli vc +meeting-list-active --as bot --user-id <open_id> --format json
```

- `--user-id` 必须是目标用户的 `ou_` open_id。
- 应用身份返回空不代表目标用户没有在开会，只代表没有找到目标用户与应用机器人同时在会中的会议。
- 返回多个会议时，展示标题、会议号和 `meeting_id` 让用户选择，不按“最近”擅选。
- 用户只给 9 位会议号时，在活跃会议结果中按 `meeting_no` 匹配；匹配失败时不要自动入会。
- `meeting_id` 从哪种身份取得，后续读取事件、截图、发送消息和操作倒计时就沿用哪种身份。结束会议也显式沿用选定的用户身份或应用身份；移出参会人仅支持用户身份，从应用身份流程切入时必须先以用户身份重新确认目标会议。

身份可见范围和会议号匹配见 [`lark-vc-meeting-list-active`](../references/lark-vc-meeting-list-active.md)。

## 读取最新会中事件

```bash
lark-cli vc +meeting-events --as <same_identity> --meeting-id <meeting_id> --page-all --format pretty
```

- 默认使用 `--page-all` 获取当前完整事件流，并保留返回的 `page_token` 供下次增量查询。
- 回答“现在、刚刚、最新”或当前会议总结前，重新查询事件；只有用户明确要求基于历史快照时才复用旧结果。
- 默认用 pretty 理解时间线；需要精确结构化字段、文档上下文或转发到 IM 时使用 JSON。
- 不要用会中事件代替已结束会议的参会人快照或会后复盘。

事件类型、分页、五分钟窗口和错误码见 [`lark-vc-meeting-events`](../references/lark-vc-meeting-events.md)。

## 读取共享内容和文档上下文

按事件中的 `share_id`、`share_doc`、`comment_id`、`element_token` 和 `block_id` 精确关联：

- 读取评论时只查询当前 `comment_id`，不要扫描整篇文档评论。
- 多个共享文档按用户问题选择相关文档；不要用“最近一次共享”替代当前 item 的 `share_id`。
- 只有用户明确要求预览且事件提供受支持的 `element_type` 与 token 时才下载，并显式选择输出路径。
- 关联或读取失败时标记 partial，保留原始标识和 raw payload；不要自动下载或猜测文档类型兜底。

精确事件 schema 和后续命令见 [`lark-vc-meeting-events`](../references/lark-vc-meeting-events.md) 的文档上下文部分。

## 读取当前会议画面

仅当用户的问题必须读取当前会议合成画面中的视觉信息，且结构化内容不足以回答时读取画面。适用任务包括识别投屏中实际显示的网页地址、界面状态或报错，理解图表、幻灯片等依赖版式或图像的信息，以及查看摄像头画面。

事件、字幕、聊天或可直接读取的共享文档已经足够回答时，不要截图；会议内容查询、总结或共享文档定位也不以截图兜底，不要仅因为会议正在进行就读取画面。

需要读取时执行：

```bash
lark-cli vc +meeting-screenshot --as <same_identity> --meeting-id <meeting_id>
```

身份、会议 ID、输出文件和失败处理见 [`lark-vc-meeting-screenshot`](../references/lark-vc-meeting-screenshot.md)。

## 发送会中文本或表情

只有用户明确要求发送并确认目标会议与内容时执行：

```bash
lark-cli vc +meeting-message-send --as <same_identity> --meeting-id <meeting_id> --msg-type text --text <message>
```

- 发送沿用 `meeting_id` 的来源身份；不要为了发送自动入会或先查会议详情。
- reaction 使用 Reference 中大小写敏感的完整 emoji key；不要编造 key。
- 发送失败时停止并报告，不自动换身份或重复发送，避免重复可见副作用。
- 用户要发送绑定群或 IM 消息时改用 `lark-im`，不要把会中消息命令当作群消息能力。

文本、reaction 和权限规则见 [`lark-vc-meeting-message-send`](../references/lark-vc-meeting-message-send.md)。

## 操作会中倒计时

只有用户明确要求设置、延长、提前结束或关闭倒计时时执行：

```bash
lark-cli vc +meeting-countdown --as <same_identity> --meeting-id <meeting_id> --action set --duration <minutes>
```

- 这是会中可见的写操作；执行前确认目标会议和动作。
- 操作沿用 `meeting_id` 的来源身份；不要为了倒计时自动入会或切换身份。
- 用户只给 9 位会议号时，先用当前身份执行 `+meeting-list-active` 并按 `meeting_no` 匹配。
- `set` 和 `prolong` 需要 `--duration`；提前结束或关闭时不要携带时长、提醒点或结束音频参数。

动作、提醒点和权限规则见 [`lark-vc-meeting-countdown`](../references/lark-vc-meeting-countdown.md)。

## 结束整场会议或移出参会人

只有用户明确要求主持管理动作，且目标会议与目标参会人已经确认时执行：

```bash
# 以用户身份结束整场会议：先 dry-run，确认后再补 --yes
lark-cli vc +meeting-end --as user --meeting-id <meeting_id> --dry-run

# 以当前 Host 应用机器人结束整场会议：先 dry-run，确认后再补 --yes
lark-cli vc +meeting-end --as bot --meeting-id <meeting_id> --dry-run

# 移出参会人：participant tuple 必须来自 meeting get 快照
lark-cli vc +meeting-participant-kickout --as user --meeting-id <meeting_id> \
  --participant '<participant_id>=<user_type>' --dry-run
```

- `vc +meeting-end` 是一个命令：`--as user` 调用用户端 PATCH 接口并要求 `vc:meeting`，`--as bot` 调用应用机器人端 POST 接口并要求 `vc:meeting.bot.manage:write`；应用机器人必须是当前 Host。
- `vc +meeting-participant-kickout` 仅支持 `--as user`，不提供应用身份端点。不要为了执行成功静默切换身份，也不要在未确认前直接补 `--yes`。
- 以用户身份结束会议或移出参会人前，先用 `vc meeting get --params '{"meeting_id":"<meeting_id>","with_participants":true}' --as user` 核对会议与参会人快照；以应用身份结束会议时，沿用应用身份发现或入会得到的 `meeting_id` 并确认机器人是当前 Host。
- 所有 dry-run 都必须显式传入 `--as user` 或 `--as bot`。`vc +meeting-end` 的 dry-run 只预览所选身份的请求路径；`vc +meeting-participant-kickout` 的 dry-run 会回显按输入顺序提交的 `kickout_users`。如果回显与用户意图不完全一致，停止并修正参数，不要继续执行。
- `--participant '<id>=<user_type>'` 每次调用接受 1 到 10 个重复 flag；ID 必须来自快照且首尾不能有空白。不要根据 open_id、昵称或设备信息猜 `user_type`，也不要把多个目标塞进 CSV/JSON。
- 结束会议的用户身份与应用身份参考分别见 [`lark-vc-meeting-end`](../references/lark-vc-meeting-end.md) 和 [`lark-vc-agent-meeting-end`](../references/lark-vc-agent-meeting-end.md)；移出参会人见 [`lark-vc-meeting-participant-kickout`](../references/lark-vc-meeting-participant-kickout.md)。

## 处理未发现会议或权限错误

- 用户身份未发现活跃会议时，可以查询当天最近结束的会议；仍无结果再询问时间、主题或会议号，不自行扩大时间范围。
- 应用身份未发现活跃会议时，只解释当前身份的空结果，不自动查询历史会议或真实入会。
- 用户身份调用活跃会议或事件查询时，普通 scope 缺失按 CLI hint 申请 `vc:meeting.meetingevent:read`；普通 scope 缺失不表示接口不支持用户身份，只有 CLI 明确说明不支持时才切到应用身份流程。
- 应用身份缺少权限时不要执行 `auth login`。优先按 CLI 返回的 `missing_scopes`、`hint` 和 `console_url` 处理；手工判断时按能力配置 scope：应用身份活跃会议查询需要 `vc:meeting.bot.join:write`，会中发消息需要 `vc:meeting.message:write`，会中倒计时需要 `vc:meeting.interaction:write`，结束会议需要 `vc:meeting.bot.manage:write`。随后依次检查应用发布、租户安装和“权限可访问的数据范围”；数据范围应为“按条件筛选”，条件为“会议的归属者 包含 与应用的可用范围一致”。
- scope、安装和数据范围都正确后仍失败时，保留 CLI 返回的错误码和 `log_id`，按服务端权限异常排查；不要反复登录或改用其他身份重试。
