# vc +meeting-end（用户身份）

结束一场正在进行中的视频会议。该命令会结束所有人的整场会议，不是让应用机器人自行离会。

本 reference 描述 `lark-cli vc +meeting-end` 的用户身份路径：显式传入 `--as user` 后调用 `PATCH /open-apis/vc/v1/meetings/{meeting_id}/end`，请求体为空。同一个 shortcut 的应用身份路径见 [vc +meeting-end（应用身份）](lark-vc-agent-meeting-end.md)。

## 命令

```bash
# 先预览目标请求，不结束会议
lark-cli vc +meeting-end --as user --meeting-id <meeting_id> --dry-run

# 用户明确确认后结束整场会议
lark-cli vc +meeting-end --as user --meeting-id <meeting_id> --yes
```

## 参数与身份

| 参数 | 必填 | 说明 |
|------|------|------|
| `--meeting-id <id>` | 是 | 正十进制且大于 0 的 int64 会议 ID；会先去除首尾空白。9 位正整数也满足 CLI 格式校验，不要仅凭位数判断是否有效 |
| `--dry-run` | 否 | 只预览 PATCH 路径，不发送 API 请求，也不结束会议 |
| `--yes` | 真实执行必需 | 确认高风险写操作；只有用户明确确认目标会议后才能传入 |

- 本路径仅使用用户身份；dry-run 必须显式传入 `--as user`。真实执行建议显式指定身份；省略 `--as` 时由 CLI 的完整配置与凭据解析决定身份，不会在离线预检中擅自改成用户身份。
- 需要 `vc:meeting` scope。
- 当前用户必须是目标会议有权限结束会议的主持人；普通参会人或联席主持人被拒绝时，不得静默更换用户身份。

## 使用边界

- 结束整场会议：使用本命令。
- 只让应用机器人离开：使用 [vc +meeting-leave](lark-vc-agent-meeting-leave.md)。
- 只移出一个或多个参会人：使用 [vc +meeting-participant-kickout](lark-vc-meeting-participant-kickout.md)。
- `meeting_id` 不确定时，先从当前用户可见的 active meeting 或会议详情中确定唯一目标，再执行 dry-run；不要猜测默认会议。

## 输出与错误

- 默认输出格式仍是 JSON；`--format pretty` 也会保留相同的服务端成功信封，不额外生成本地结束提示。
- `--format json` 返回标准 `{ok, identity, data}` 信封，其中外层 `data` 原样保留服务端成功响应的 `code`、`msg`、`log_id`、服务端 `data` 和其他附加字段。
- 参数错误、权限拒绝和服务端错误保持为 CLI 的结构化错误；排查时保留错误码与 `log_id`。
- dry-run 或未通过高风险确认时不会调用会议 API。

## 相关场景

- [会中事件、互动与主持管理](../scenes/live-meeting-interact.md)
- [应用身份结束会议](lark-vc-agent-meeting-end.md)
- [移出指定参会人](lark-vc-meeting-participant-kickout.md)
