# vc +meeting-share / +meeting-share-status

控制会中文档共享并查询当前共享快照。

- `lark-cli vc +meeting-share` 调用 `POST /open-apis/vc/v1/bots/meeting_share`
- `lark-cli vc +meeting-share-status` 调用 `GET /open-apis/vc/v1/bots/meeting_share_status`

两个命令支持 `--as user`（用户身份）和 `--as bot`（应用身份）。必须沿用获得 `meeting_id` 时的身份；具体操作权限由服务端校验。

## 文档地址

`--doc-url` 必须传完整 HTTPS 文档地址。

- `start` 支持 Doc、Docx 和 Sheet，也支持最终解析为这三类资源的 Wiki URL。
- Base / Bitable、Mindnote、Slides、普通文件和外链暂不支持，服务端会拒绝。
- `update_context`、`comment_panel` 和 `start` 的 `--comment-panel-open` 只支持 Docx 和最终解析为 Docx 的 Wiki URL。Doc 和 Sheet 可以开始、查询、停止共享，但更新正文位置、评论上下文、元素预览或评论面板会被服务端拒绝。

Docx URL 示例：

```bash
lark-cli vc +meeting-share --as bot \
  --meeting-id <meeting_id> --action start \
  --doc-url 'https://example.feishu.cn/docx/<doc_token>'
```

## 开始与停止

```bash
lark-cli vc +meeting-share --as bot \
  --meeting-id <meeting_id> --action start \
  --doc-url 'https://example.feishu.cn/docx/<doc_token>'

lark-cli vc +meeting-share --as bot \
  --meeting-id <meeting_id> --action stop --share-id <share_id>
```

`meeting_id` 必须是长数字会议 ID，不是 9 位会议号。`start` 成功返回 `share_id`；后续停止、更新上下文或切换评论面板必须使用该 ID。当前操作身份必须已在会中，且不能抢占其他参会人的共享。

发起 `start` 时，当前所有在会成员（包括应用机器人）必须属于同一租户。存在跨租户成员时，当前返回 `12005`（`no permission`）。

这项发起限制不会因共享开始后加入跨租户成员而自动停止已有共享，已有上下文更新仍按原有权限规则处理。

若 `start` 返回共享冲突错误或请求结果不确定，不要直接重试。先沿用同一身份调用 `+meeting-share-status`，但该结果只是快照：必须确认 `presenter` 是当前操作人且文档 URL 符合预期，才能把返回的 `share_id` 用于后续 stop、update_context 或 comment_panel；否则保留 `log_id` 并报告失败，不能操作其他参会人的共享。错误码以实际接口响应为准。

## 切换评论面板

仅适用于 Docx / Wiki-Docx。**用户明确要求开关时优先遵从；未指定时，由 Agent 结合任务和文档评论自行判断。有评论且展开有助于当前任务时主动打开，无评论时不主动打开。不要固定传 `true` 或 `false`，也不要只因用户未明确要求就一律不打开。** Doc 和 Sheet 不传该参数。

```bash
# 开始共享并讨论文档中的评论，显式传 true
lark-cli vc +meeting-share --as bot \
  --meeting-id <meeting_id> --action start \
  --doc-url 'https://example.feishu.cn/docx/<doc_token>' \
  --comment-panel-open=true

# 已在共享时显式关闭面板；改为 true 则打开
lark-cli vc +meeting-share --as bot \
  --meeting-id <meeting_id> --action comment_panel --share-id <share_id> \
  --comment-panel-open=false
```

决定打开时传 `--comment-panel-open=true`，决定关闭时传 `false`；无需操作面板时可省略。使用原始 API 时，对应字段是 `document.comment_panel_open`。需要了解文档评论时，可用 `docs +fetch` 随正文读取可见评论。

已在共享时，沿用同一身份查询 `+meeting-share-status`，确认 `presenter` 是当前操作人且文档 URL 符合预期后，使用该 `share_id` 执行 `comment_panel`，不要重新 `start`。

`comment_panel` 必须提供正整数 `share_id` 和显式布尔值，不接受 `--doc-url` 或任何上下文参数。`stop` 和 `update_context` 不接受 `--comment-panel-open`，即使值为 `false`。

评论面板开关不改变正文位置、当前评论或评论聚焦，不能与正文位置、评论上下文或元素预览同时传入。

## 更新正文位置

仅适用于 Docx / Wiki-Docx。

```bash
lark-cli vc +meeting-share --as bot \
  --meeting-id <meeting_id> --action update_context --share-id <share_id> \
  --block-id <block_id> --block-y-ratio 0.35
```

## 更新评论上下文

```bash
lark-cli vc +meeting-share --as bot \
  --meeting-id <meeting_id> --action update_context --share-id <share_id> \
  --comment-focus-shown=true --comment-id <comment_id> \
  --comment-anchor-id <block_id>

# 退出聚焦时保留当前评论
lark-cli vc +meeting-share --as bot \
  --meeting-id <meeting_id> --action update_context --share-id <share_id> \
  --comment-focus-shown=false --comment-id <current_comment_id>
```

开启聚焦或切换到下一条目标评论时，必须同时传 `--comment-focus-shown=true`、`--comment-id` 和该评论对应的 `--comment-anchor-id`。`comment-anchor-id` 是评论挂靠的正文锚点/record ID，不是 `comment_id`，携带它才会同步跳转正文；拿不到该字段时，不要执行下一条评论聚焦。显式关闭聚焦且已知当前评论时，传 `--comment-focus-shown=false --comment-id <current_comment_id>`，以保留该评论；仅在当前评论未知时才使用 bare `--comment-focus-shown=false`。`comment-anchor-id` 只能与 `comment_id` 一起使用。

## 更新元素预览

```bash
lark-cli vc +meeting-share --as bot \
  --meeting-id <meeting_id> --action update_context --share-id <share_id> \
  --element-action open --element-type whiteboard \
  --element-block-id <block_id>
```

`--element-action` 为 `open|close`，`--element-type` 为 `image|whiteboard`。正文位置、评论上下文和元素预览三类参数互斥。

## 查询共享快照

```bash
lark-cli vc +meeting-share-status --as bot --meeting-id <meeting_id>
```

无共享时返回空状态；有共享时返回 `share_id`、文档信息、主讲人以及最近持久化的正文位置。该位置是服务端快照，不代表参会人自由浏览时的实时视口。

## 权限异常

缺少权限时，按 CLI 返回的 `missing_scopes` 和 `hint` 处理。

`12005`（`no permission`）是通用权限错误，也可能来自其他权限校验，应以实际接口响应为准，不能仅凭错误码判断会议是否跨租户。核对当前在会成员的租户和资源权限，保留 `log_id`；不要自动切换身份或重复执行写操作。

## 相关

- [lark-vc-meeting-list-active](lark-vc-meeting-list-active.md)
- [lark-vc-meeting-events](lark-vc-meeting-events.md)
- [lark-vc-agent-meeting-join](lark-vc-agent-meeting-join.md)
