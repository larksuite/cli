# vc +meeting-participant-mute / +meeting-participant-unmute

在进行中的会议里将一个指定参会人闭麦，或向其发送开麦请求。闭麦是完成型操作；请求开麦是邀请目标参会人自行开麦，不能据此断言麦克风已经打开。

## 命令

```bash
# 先预览，不调用会议 API
lark-cli vc +meeting-participant-mute --as <source_identity> \
  --meeting-id <meeting_id> --target-user-id <user_id> --dry-run
lark-cli vc +meeting-participant-unmute --as <source_identity> \
  --meeting-id <meeting_id> --target-user-id <user_id> --dry-run

# 用户已经明确会议、目标和动作后执行
lark-cli vc +meeting-participant-mute --as <source_identity> \
  --meeting-id <meeting_id> --target-user-id <user_id>
lark-cli vc +meeting-participant-unmute --as <source_identity> \
  --meeting-id <meeting_id> --target-user-id <user_id>
```

## 参数与身份

| 参数 | 必填 | 说明 |
|---|---|---|
| `--meeting-id <id>` | 是 | 会议开始后产生的长正整数会议 ID，不是 9 位会议号 |
| `--target-user-id <id>` | 是 | 目标参会人的用户 ID，不能为空 |
| `--user-id-type <type>` | 否 | 目标用户 ID 类型，可选 `open_id`、`union_id`、`user_id`，默认 `open_id` |

- 支持 `--as user` 或 `--as bot`，必须显式沿用目标会议的来源身份，不要为了执行成功静默切换。
- 两种身份都要求 `vc:meeting.bot.manage:write` scope。命令风险级别为 `write`，不使用 `--yes`；真实执行前仍需用户明确会议、目标和动作。
- OpenAPI 不接收 `device_id`；CLI 只发送 `meeting_id`、`target_user_id` 和 `user_id_type`，用户到设备的处理由服务端完成。

## OpenAPI 契约

| 命令 | 方法与路径 | 成功语义 |
|---|---|---|
| `vc +meeting-participant-mute` | `POST /open-apis/vc/v1/bots/mute` | 指定参会人闭麦完成 |
| `vc +meeting-participant-unmute` | `POST /open-apis/vc/v1/bots/unmute` | 开麦请求已发送 |

两个请求的 query 都只包含 `user_id_type`，body 只包含 `meeting_id` 与 `target_user_id`。先用 `--dry-run` 核对身份、路径、query 和 body；预览结果与用户意图不一致时停止，不执行真实请求。

## 输出与状态边界

- 闭麦成功时，pretty 输出为 `Participant muted.`，结构化结果的 `action` 为 `mute`、`status` 为 `completed`。
- 请求开麦成功时，pretty 输出为 `Unmute request sent.`，结构化结果的 `action` 为 `request_unmute`、`status` 为 `request_sent`。
- “请求已发送”不表示目标参会人已经开麦。需要确认最终状态时，必须取得参会人的后续麦克风状态证据，不能从本命令的成功响应推断。
- API 失败保持为 CLI 结构化错误；保留错误码和 `log_id` 排查，不自动切换身份或重复写请求。

## 错误码

闭麦和请求开麦使用相同的对外错误码。CLI 参数校验在请求发出前失败时属于本地结构化错误，不会产生下表中的服务端错误码。

| 错误码与 message | 含义 | 处理建议 |
|---|---|---|
| `121101`: `internal service error` | 服务端内部错误 | 保留 `log_id` 排查。确认请求确实失败后再由用户决定是否重试，不要自动循环发送写请求 |
| `121102`: `param is invalid` | `meeting_id`、`target_user_id` 或 `user_id_type` 无效 | 核对会议开始后产生的长 meeting ID、目标用户 ID 与 ID 类型；不要传 9 位会议号或 `device_id` |
| `121103`: `no permission` | 当前身份没有执行该会控操作的权限 | 核对所选 `--as` 身份、`vc:meeting.bot.manage:write` scope 和会中主持权限；不要静默切换身份绕过 |
| `121104`: `meeting status unexpected` | 会议状态不支持会控操作 | 确认会议仍在进行中；不要对未开始或已结束的会议重试 |
| `121105`: `meeting not exist` | 找不到指定会议 | 核对 meeting ID、飞书/Lark 环境和当前身份可见的会议，不要把会议号当 meeting ID |
| `121107`: `target participant is not in the meeting or is no longer active` | 目标参会人不在会中，或已经不再活跃 | 重新确认目标参会人仍在会中；不要自行补传或猜测 `device_id` |
| `122003`: `operator is not in the meeting` | 当前 user 或应用机器人身份不在该会议中 | 先让同一调用身份进入目标会议，再沿用该身份执行；不要自动改用另一种身份 |
| `122005`: `cannot operate self` | 操作人和目标参会人是同一用户 | 重新确认 `target_user_id`，不要对当前操作人执行闭麦或请求开麦 |

## 使用边界

- 移出参会人使用 [vc +meeting-participant-kickout](lark-vc-meeting-participant-kickout.md)。
- 结束整场会议使用 [vc +meeting-end](lark-vc-meeting-end.md) 或应用身份的 [vc +meeting-end](lark-vc-agent-meeting-end.md)。
- 会中主持管理流程见 [会中事件、互动与主持管理](../scenes/live-meeting-interact.md)。
