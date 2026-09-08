# vc +meeting-chat

显式创建或复用会议 Chat。只查询当前绑定时使用 [`vc +detail`](lark-vc-detail.md)，不要因查询未返回 `chat_id` 自动调用本命令。

## 前置条件与身份

- 会议进行中，调用主体当前在会；服务端沿用真人点击聊天的业务规则。
- 支持 `--as user`（UAT / AAT）和 `--as bot`（TAT），创建权限为 `vc:meeting.interaction:write`。沿用取得 `meeting_id` 时的身份，不自动入会、创建替代群或跨身份重试。
- `meeting_id` 是长数字字符串，不是 9 位会议号。请求仅包含 `meeting_id`，没有 `--create` 或指定目标群、成员的参数。

## 命令

```bash
# 预览请求，无副作用
lark-cli vc +meeting-chat --as user --meeting-id <meeting_id> --dry-run

# 来源为用户身份
lark-cli vc +meeting-chat --as user --meeting-id <meeting_id> --format json

# 来源为已在会中的应用身份
lark-cli vc +meeting-chat --as bot --meeting-id <meeting_id> --format json
```

## 输出与后续操作

成功信封沿用 `ok`、`identity`、`data`，`data.chat_id` 为服务端返回的 Chat ID。失败沿用标准结构化错误，不输出成功的 Chat ID。

返回 ID 不保证正式入群、群的持续有效性或 IM 消息权限。正式成员离会不自动退群，临时权限按既有规则清理，不能承诺会后继续访问。应用机器人临时参与及对应 TAT 的 IM 消息读写按现有场景规则支持，仍受 IM 的独立鉴权约束。

用户明确要求读写消息时，沿用同一身份转到 `im +chat-messages-list` 或 `im +messages-send`，读取对应 IM 命令参考；不要把获取 ID 当作消息访问成功。遇到不在会、权限或业务错误时原样报告，不以入会、建群或更换身份兜底。
