# vc +meeting-invite

通过 Agent Bot API 邀请指定用户，或一键邀请符合条件的 Calendar 参会人。

```bash
lark-cli vc +meeting-invite --as bot --meeting-id 7628568141510692381 --type SELECTED --open-ids ou_xxx,ou_yyy
lark-cli vc +meeting-invite --as bot --meeting-id 7628568141510692381 --type ALL_SUGGESTED
lark-cli vc +meeting-invite --as bot --meeting-id 7628568141510692381 --type ALL_SUGGESTED --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
| --- | --- | --- |
| `--meeting-id` | 是 | 长数字 Meeting ID，不是 9 位会议号。 |
| `--type` | 是 | `SELECTED` 或 `ALL_SUGGESTED`，大小写不敏感。 |
| `--open-ids` | `SELECTED` 时必填 | 用户 `open_id`（`ou_xxx`），支持逗号分隔或重复传入，最多 200 个；`ALL_SUGGESTED` 时不得传入。 |

该 shortcut 仅支持 bot 身份，调用 `POST /open-apis/vc/v1/bots/invite`。

- `SELECTED` 显式发送用户 `open_id`；本地会在请求前拒绝超过 200 个 ID 的输入。
- `ALL_SUGGESTED` 只发送邀请类型。服务端根据 Calendar 状态解析一键邀请候选集，并应用 200 人上限。
- 请求契约：`SELECTED` 发送 `invite_type=2`、`invitees=[{"id":"ou_xxx","user_type":1}]` 和查询参数 `user_id_type=open_id`；`ALL_SUGGESTED` 发送 `invite_type=1` 且省略 `invitees`。
- 返回契约：`SELECTED` 可返回显式受邀人的 `invite_results`；CLI 会按响应 `id` 展示每项 `invited` 或 `failed` 状态。`ALL_SUGGESTED` 仅返回聚合字段，不返回逐用户 `invite_results`。
- `ALL_SUGGESTED` 的 `has_more=true` 表示候选人超过服务端单次 200 人上限，不是可翻页信号。该接口没有 continuation 或 `page_token`；CLI 会显示截断提示而不输出 `has_more`。

## 权限与前置条件

- 目标必须是 Calendar VC 会议，且应用 Bot 已在会中。
- Agent Invite 依赖会议的 Agent 加入能力。日程未开启 AI/Agent 会议设置时，邀请请求会失败。
- 仅包含一名受邀人的 `SELECTED` 复用普通单点邀请策略，普通会中参会人也可能有权邀请该用户。
- `ALL_SUGGESTED` 和多用户 `SELECTED` 使用批量/建议列表邀请策略。实际调用时 Bot 应为当前 host 或 co-host；普通参会 Bot 可能没有批量邀请权限。
