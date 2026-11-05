# vc
> skill: lark-meeting

## +search

### Skills
- lark-meeting/references/lark-vc-search.md

## +detail

查询会议当前聊天绑定时使用；会后也可按既有详情权限查询。

### Tips
- 从对应会议结果读取可选 chat_id；未返回绑定不自动创建聊天。

### Skills
- lark-meeting/references/lark-vc-detail.md

## meeting get

## +recording

### Skills
- lark-meeting/references/lark-vc-recording.md

## +meeting-recording-start

### Skills
- lark-meeting/references/lark-vc-recording-control.md

## +meeting-recording-stop

### Skills
- lark-meeting/references/lark-vc-recording-control.md

## +meeting-list-active

### Skills
- lark-meeting/references/lark-vc-meeting-list-active.md

## +meeting-events

### Skills
- lark-meeting/references/lark-vc-meeting-events.md

## +meeting-screenshot

### Skills
- lark-meeting/references/lark-vc-meeting-screenshot.md

## +meeting-message-send

### Skills
- lark-meeting/references/lark-vc-meeting-message-send.md

## +meeting-chat

需要显式创建或复用当前会议的 Chat 时使用。

### Avoid when
- 只查询现有绑定 → [[+detail]]。

### Prerequisites
- 会议进行中，调用主体已经在会；沿用取得 meeting_id 时的身份。

### Tips
- 返回 Chat ID 不保证正式入群或 IM 消息权限；不自动入会或切换身份重试。

### Skills
- lark-meeting/references/lark-vc-meeting-chat.md

## +meeting-countdown

### Skills
- lark-meeting/references/lark-vc-meeting-countdown.md

## +meeting-end
Use this only when the user explicitly asks to end the entire ongoing meeting. This is a high-risk write for either supported identity: preview the target with `--dry-run` and an explicit `--as user` or `--as bot`, and pass `--yes` only after explicit confirmation.

### Avoid when
- Only the application bot should leave the meeting → use [[+meeting-leave]].
- Only specific participants should be removed → use [[+meeting-participant-kickout]].

### Prerequisites
- Obtain the exact `meeting_id` for the target ongoing meeting from [[+meeting-list-active]] or [[meeting get]].
- Choose the endpoint deliberately: `--as user` uses `PATCH /open-apis/vc/v1/meetings/{meeting_id}/end` with `vc:meeting`; `--as bot` uses `POST /open-apis/vc/v1/bots/end` with `vc:meeting.bot.manage:write` and requires the application bot to be the current Host.

### Examples

**Preview the user endpoint without making the API call**
```bash
lark-cli vc +meeting-end --as user --meeting-id <meeting_id> --dry-run
```

**Preview the application bot endpoint without making the API call**
```bash
lark-cli vc +meeting-end --as bot --meeting-id <meeting_id> --dry-run
```

### Skills
- lark-meeting/references/lark-vc-meeting-end.md
- lark-meeting/references/lark-vc-agent-meeting-end.md

## +meeting-participant-kickout
Use this only when the user explicitly asks a host or cohost to remove specific participants. This is a user-only, high-risk write: preview uncertain targets with `--dry-run`, and pass `--yes` only after explicit confirmation.

### Avoid when
- The entire meeting should end → use [[+meeting-end]].
- Only the application bot should leave by itself → use [[+meeting-leave]].

### Prerequisites
- Obtain the exact `meeting_id` plus each participant tuple from the target meeting's participant snapshot via [[meeting get]]; do not infer `user_type` from an ID.
- Use the dedicated reference for tuple syntax, dry-run shape, and result interpretation instead of re-encoding that schema here.

### Examples

**Preview removing one participant without making the API call**
```bash
lark-cli vc +meeting-participant-kickout --as user --meeting-id <meeting_id> --participant '<open_id>=<user_type>' --dry-run
```

### Skills
- lark-meeting/references/lark-vc-meeting-participant-kickout.md

## +meeting-transfer-host
用户明确要求把当前会议的主持人交给某个已确认的参会人时使用。

### Avoid when
- 结束整场会议 → [[+meeting-end]]。
- 移出指定参会人 → [[+meeting-participant-kickout]]。

### Prerequisites
- 使用当前主持会议的用户身份（`--as user`）；遇到权限错误不要切换身份重试。
- 从目标会议的参会人快照（[[meeting get]]）取得 `meeting_id` 和接任者的 ID、`user_type`；不要根据 ID 或昵称推断 `user_type`。

### Tips
- 输入不确定时先用 `--dry-run` 预览；用户确认会议和接任者后再加 `--yes`。
- 只有成功响应中的新主持人就是目标参会人，才算转移成功。

### Examples

**预览转移主持人请求**
```bash
lark-cli vc +meeting-transfer-host --as user --meeting-id <meeting_id> --participant '<open_id>=<user_type>' --dry-run
```

### Skills
- lark-meeting/references/lark-vc-meeting-transfer-host.md

## +meeting-participant-mute
Use this only when the user explicitly asks to mute one identified participant in an ongoing meeting. This is a write operation; preview an uncertain meeting, target, or identity with `--dry-run` before executing.

### Avoid when
- The participant should be asked to turn their microphone on → use [[+meeting-participant-unmute]].
- The participant should be removed from the meeting → use [[+meeting-participant-kickout]].

### Prerequisites
- Obtain the exact long `meeting_id` and target user ID from the intended ongoing meeting; do not substitute a 9-digit meeting number or a device ID.
- Preserve the selected `--as user` or `--as bot` identity instead of switching identities to make the request succeed.

### Skills
- lark-meeting/references/lark-vc-meeting-participant-audio.md

## +meeting-participant-unmute
Use this only when the user explicitly asks to request that one identified participant turn their microphone on. A successful API response means the request was sent, not that the participant is already unmuted.

### Avoid when
- The participant should be muted → use [[+meeting-participant-mute]].
- The participant should be removed from the meeting → use [[+meeting-participant-kickout]].

### Prerequisites
- Obtain the exact long `meeting_id` and target user ID from the intended ongoing meeting; do not substitute a 9-digit meeting number or a device ID.
- Preserve the selected `--as user` or `--as bot` identity instead of switching identities to make the request succeed.

### Skills
- lark-meeting/references/lark-vc-meeting-participant-audio.md

## +meeting-join

### Skills
- lark-meeting/references/lark-vc-agent-meeting-join.md

## +meeting-invite

### Skills
- lark-meeting/references/lark-vc-agent-meeting-invite.md

## +meeting-leave

### Skills
- lark-meeting/references/lark-vc-agent-meeting-leave.md

## +meeting-share

### Prerequisites
- 执行 `start` 时，当前所有在会成员（包括应用机器人）必须同租户。

### Tips
- 跨租户成员导致的 `start` 拒绝当前返回 `12005`（`no permission`）；该权限错误也可能来自其他权限校验。应以实际接口响应为准，不能仅凭错误码判断外部会议；核对成员与资源权限，保留 `log_id`，不要直接重试 `start`。
- 评论面板按用户要求开关；未指定时结合任务和文档评论判断是否展开，无评论时不主动打开，具体用法见文档共享参考。

### Skills
- lark-meeting/references/lark-vc-meeting-share.md

## +meeting-share-status

### Tips
- 用于核对当前共享；最近持久化的位置不能证明客户端已应用跳转。

### Skills
- lark-meeting/references/lark-vc-meeting-share.md
