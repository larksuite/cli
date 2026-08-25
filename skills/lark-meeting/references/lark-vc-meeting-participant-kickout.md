# vc +meeting-participant-kickout

以主持人或联席主持人的用户身份，从一场正在进行中的会议移出一个或多个指定参会人。该命令不会结束整场会议。

本 skill 对应 shortcut：`lark-cli vc +meeting-participant-kickout`（调用 `POST /open-apis/vc/v1/meetings/{meeting_id}/kickout`）。

## 命令

```bash
# 先预览一个目标，不移出参会人
lark-cli vc +meeting-participant-kickout \
  --as user \
  --meeting-id <meeting_id> \
  --participant '<participant_id>=<user_type>' \
  --dry-run

# 用户明确确认后，可按输入顺序提交多个目标
lark-cli vc +meeting-participant-kickout \
  --as user \
  --meeting-id <meeting_id> \
  --participant '<participant_id_1>=<user_type_1>' \
  --participant '<participant_id_2>=<user_type_2>' \
  --yes
```

## 参数与身份

| 参数 | 必填 | 说明 |
|------|------|------|
| `--meeting-id <id>` | 是 | 正十进制且大于 0 的 int64 会议 ID；会先去除首尾空白 |
| `--participant '<id>=<user_type>'` | 是 | 可重复 1 至 10 次；每项必须恰好包含一个 `=`，ID 必须是正十进制且大于 0 的 int64 且首尾不能有空白，`user_type` 必须是 1 至 7 的整数 |
| `--dry-run` | 否 | 只预览 POST 路径和请求体，不发送 API 请求，也不移出参会人 |
| `--yes` | 真实执行必需 | 确认高风险写操作；只有用户明确确认会议和目标参会人后才能传入 |

- 仅支持 `user` 身份，必须显式使用 `--as user`；没有应用身份端点，不要改用应用身份重试。
- 需要 `vc:meeting` scope。
- 执行者必须是目标会议的主持人或具备相应权限的联席主持人；权限拒绝时不得静默更换用户身份。

## participant tuple 规则

1. 从目标会议的参会人快照读取精确的 participant ID 和 `user_type`。可使用：

   ```bash
   lark-cli vc meeting get --params '{"meeting_id":"<meeting_id>","with_participants":true}' --as user
   ```

2. 不要仅凭 open_id、设备 ID 或显示名猜测 `user_type`。
3. CLI 将 ID 当作字符串原样发送，因此前导零会保留；如果 tuple 里的 ID 含首尾空白，CLI 会直接报错，不会帮你 trim 后继续执行。
4. CLI 不去重、不排序，也不替请求做冲突消解；重复 tuple、同一 ID 的不同类型都会按输入顺序发送。先确认这正是用户意图。
5. 不接受 JSON `--kickout-users`、CSV 或分离的 ID/type 数组；只使用可重复的 `--participant '<id>=<user_type>'`。

请求体形状为：

```json
{
  "kickout_users": [
    {"id": "<participant_id>", "user_type": 1}
  ]
}
```

## 输出与 partial result

- `--format json` 和其他机器可读格式都会完整保留服务端成功响应的 `code`、`msg`、`log_id`、`data` 与附加字段；`data.kickout_results` 的字段、值与顺序不会被重新映射、排序或合并。
- 批量请求可能同时包含成功和失败的目标。逐项根据服务端返回的 `id`、`user_type` 和 `result` 判断，不要把一个目标的结果外推到其他目标，也不要由 CLI 自行解释未知结果值。
- 请求级参数错误、权限拒绝或框架错误保持为 CLI 的结构化错误；排查时保留错误码与 `log_id`。
- dry-run 或未通过高风险确认时不会调用会议 API。

## 使用边界

- 结束所有人的整场会议：使用 [vc +meeting-end](lark-vc-meeting-end.md)。
- 只让应用机器人自行离会：使用 [vc +meeting-leave](lark-vc-agent-meeting-leave.md)。
- participant tuple 不确定时先读取快照；不要尝试默认参会人或默认身份。

## 相关场景

- [会中事件、互动与主持管理](../scenes/live-meeting-interact.md)
- [结束整场会议](lark-vc-meeting-end.md)
