# vc +recording-start / +recording-stop

在进行中的会议里开始或停止录制。这两个命令是会中写操作；查询已经生成的录制和 `minute_token` 仍使用只读命令 `vc +recording`。

## 命令

```bash
# 先预览请求
lark-cli vc +recording-start --as user --meeting-id 6911188411932033028 --dry-run
lark-cli vc +recording-stop --as user --meeting-id 6911188411932033028 --dry-run

# 开始或停止录制
lark-cli vc +recording-start --as user --meeting-id 6911188411932033028
lark-cli vc +recording-stop --as user --meeting-id 6911188411932033028
```

## 核心约束

- 仅支持 `--as user`，所需权限为 `vc:record`（更新会议录制信息）。应用后台和用户授权都必须包含该 scope。
- 操作者必须在会议中且是当前主持人。开始录制要求会议正在进行；停止录制要求会议正在录制。
- `--meeting-id` 必须是会议开始后产生的长会议 ID，不是 9 位会议号。可先运行 `lark-cli vc +meeting-list-active --as user`，从唯一目标会议中取得 `meeting_id`。
- `+recording-start` 不发送可选的 `timezone` 请求字段，使用 OpenAPI 的默认行为。

## OpenAPI 契约

| 命令 | 方法与路径 | 请求体 |
|---|---|---|
| `vc +recording-start` | `PATCH /open-apis/vc/v1/meetings/:meeting_id/recording/start` | 无 |
| `vc +recording-stop` | `PATCH /open-apis/vc/v1/meetings/:meeting_id/recording/stop` | 无 |

两个接口都只接受 `user_access_token`。成功 JSON 的 `data` 包含 `meeting_id` 和 `action`；OpenAPI 错误按 CLI 的结构化错误契约原样分类并透传错误码。

## 常见错误

| 错误码 | 含义 |
|---|---|
| `121005` | token、操作者身份或资源归属不满足权限要求 |
| `122001` | 会议或录制状态不符合当前动作 |
| `122002` | 会议不存在 |
| `122003` | 操作者不在会议中 |
