# vc +meeting-transfer-host

以当前主持人的用户身份，将一场正在进行中的会议的主持人权限交给指定参会人。

本 skill 对应 shortcut：`lark-cli vc +meeting-transfer-host`（调用 `PATCH /open-apis/vc/v1/meetings/{meeting_id}/set_host`）。

## 命令

```bash
# 预览转移给一个参会人的请求，不转移主持人
lark-cli vc +meeting-transfer-host \
  --as user \
  --meeting-id <meeting_id> \
  --participant '<open_id>=<user_type>' \
  --dry-run

# 用户已明确要求转移，且会议与接任者已确定时执行
lark-cli vc +meeting-transfer-host \
  --as user \
  --meeting-id <meeting_id> \
  --participant '<open_id>=<user_type>' \
  --yes
```

真实执行需要 `--yes`；dry-run 只预览请求构造，不能证明调用权限、接任资格或交接成功。

## 参数与身份

| 参数 | 必填 | 说明 |
|------|------|------|
| `--meeting-id <id>` | 是 | 正十进制且大于 0 的 int64 会议 ID；会先去除首尾空白，不能用 9 位会议号替代 |
| `--user-id-type <type>` | 否 | `host_user.id` 的用户 ID 类型；可选 `open_id`、`union_id`、`user_id`，默认 `open_id` |
| `--participant '<id>=<user_type>'` | 是 | 接任者 tuple，只能传一个；必须恰好包含一个 `=`，ID 首尾不能有空白，`user_type` 必须是 1 至 7 的整数 |
| `--dry-run` | 否 | 只预览 PATCH 路径、query 和请求体，不发送 API 请求 |
| `--yes` | 真实执行必需 | 确认转移主持人；只有用户明确确认会议和接任者后才能传入 |

- 仅支持 `user` 身份，必须显式使用 `--as user`；普通用户和智能体用户均走 `--as user`。不要因权限错误自动切换到其他用户或应用身份。
- 需要 `vc:meeting` scope。
- 当前用户必须是目标会议的主持人；接任者仍在同一会议中，并满足服务端的接任条件。
- 本命令按用户身份调用，请求体不包含 `old_host_user`。

## 目标参会人

接任者的 ID 和 `user_type` 从会议参会人快照取得，不根据昵称猜测。需要查询时，沿用同一 profile，明确请求参会人：

```bash
lark-cli vc meeting get \
  --as user \
  --meeting-id "<meeting_id>" \
  --with-participants
```

从成功结果的 `data.meeting` 核对目标会议、当前主持人和接任者的 ID、类型及当前参会状态。出现在参会人快照中不等于当前仍在会中；已有可靠的当前信息时无需重复查询。

请求 query 默认为：

```json
{"user_id_type": "open_id"}
```

请求体形状为：

```json
{
  "host_user": {"id": "<open_id>", "user_type": 1}
}
```

## 使用边界

- 只转移主持人：使用本命令；结束整场会议使用 [vc +meeting-end](lark-vc-meeting-end.md)；移出参会人使用 [vc +meeting-participant-kickout](lark-vc-meeting-participant-kickout.md)。
- `meeting_id` 或接任者不确定时，先用 `vc meeting get` 确定唯一目标，不猜测默认会议或参会人。

## 输出与错误

- `--format json` 和其他机器可读格式都会完整保留服务端成功响应的 `code`、`msg`、`log_id` 与 `data`。
- 默认 JSON 输出中，只有真实请求成功、`ok == true`，且 `data.data.host_user` 与接任者一致，才能报告转移成功；失败响应可能包含当前 `host_user`，不代表转移成功。
- 参数错误、权限拒绝和服务端错误保持为 CLI 的结构化错误；排查时保留错误码与 `log_id`，不要切换身份重试。
- 结果不确定时，先用同一身份查询当前主持人，不直接重复转移。
- dry-run 或未通过高风险确认时不会调用会议 API。

## 相关场景

- [会中事件、互动与主持管理](../scenes/live-meeting-interact.md)
