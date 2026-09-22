# vc +meeting-chat

用户明确要求为正在进行的视频会议创建或复用聊天时使用，返回 `chat_id`。服务端决定新建或复用，不接受目标群或成员列表。

本 reference 对应 shortcut：`lark-cli vc +meeting-chat`（调用 `POST /open-apis/vc/v1/bots/chat`）。这是写操作，服务端决定复用已有聊天还是创建聊天，调用者不需要预先判断。

只查询绑定使用 `vc meeting get`，会后也可按原权限查询；命令及结果解析见 [获取会议群聊](../scenes/query-meeting-and-artifacts.md#获取会议群聊)。未返回 `chat_id` 不等于授权创建。已有 `chat_id` 且只需消息操作时直接使用下方 IM 参考。用户明确要日历日程绑定群时使用 [lark-calendar](../../lark-calendar/SKILL.md)；即时会议没有日程，不能走日程群路径。两种绑定按各自结果读取，不推断相同或在失败后自动改走另一条创建路径。

## 权限和前置条件

- 会议必须正在进行，调用主体当前在会。查询到会议详情并不代表满足在会条件。
- `--as user`：以当前用户身份操作；UAT / AAT 沿用用户身份路径。
- `--as bot`：以当前应用机器人身份操作（TAT），该机器人必须已在这场会议中。
- 创建操作需要 `vc:meeting.interaction:write`；消息读取、发送还需满足 IM 域的独立权限要求。
- 服务端沿用会中点击聊天的规则；命令不负责自动入会。

## 如何获取会议 ID 与选择身份

`--meeting-id` 使用长数字字符串 `meeting_id`，不是 9 位会议号；按原字符串传递，避免数值精度丢失。

已有 `meeting_id` 时直接复用，并沿用取得它时的身份。没有来源身份时使用用户指定的身份；未指定时，明确要求应用机器人操作才用 `--as bot`，否则用 `--as user`。不因失败自动切换身份。

如果用户只提供 9 位会议号：

1. 按当前身份使用 [`vc +meeting-list-active`](lark-vc-meeting-list-active.md) 查询进行中的会议。
2. 用 `meeting_no` 匹配，取得对应的长数字 `meeting_id`。
3. 有多个候选时请用户选定会议；没有匹配时说明未找到，不自动入会或创建另一场会议。

## 参数与命令

业务参数只有 `--meeting-id`；身份通过 `--as user` 或 `--as bot` 指定。`--dry-run` 预览请求，`--format json` 用于结构化消费。完整参数以 `lark-cli vc +meeting-chat --help` 为准。

### 预览请求

```bash
lark-cli vc +meeting-chat --as user --meeting-id "<meeting_id>" --dry-run
```

预览中的 API 请求如下，不会调用创建接口；预览成功也不代表服务端权限或在会状态已经校验通过。

```json
{
  "method": "POST",
  "url": "/open-apis/vc/v1/bots/chat",
  "body": {
    "meeting_id": "<meeting_id>"
  }
}
```

### 创建或复用聊天

```bash
# 用户身份
lark-cli vc +meeting-chat --as user --meeting-id "<meeting_id>" --format json

# 已在会中的应用机器人身份
lark-cli vc +meeting-chat --as bot --meeting-id "<meeting_id>" --format json
```

两个示例是身份选择的不同分支，不是需要依次执行的步骤。

## 输出与结果解释

成功 JSON 示例（ID 为占位示意）：

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "chat_id": "oc_example"
  }
}
```

- `identity` 是本次调用身份；`data.chat_id` 是后续 IM 操作使用的聊天 ID，原样传递。
- 响应不区分新建和复用，也不返回聊天类型；复用 P2P 聊天同样是成功，不必再创建一个群。
- 返回 ID 不证明正式入群或消息可访问。命令没有返回有效 `chat_id` 时，CLI 会报响应异常，不应拼造 ID 或报告创建成功。
- 失败沿用 CLI 的标准结构化错误，不能从失败响应推断“聊天一定没有创建”。

## 常见错误与处理

以下按错误含义处理，保留实际响应中的错误码、提示和 `log_id`（若有）。不要将服务内部错误码当作稳定的 CLI 错误分类。

| 错误现象 | 处理方式 |
| --- | --- |
| 缺少或无效的会议 ID | 核对长数字 `meeting_id`，不要传 9 位会议号；身份相关参数错误则按实际提示检查调用配置。 |
| 会议不存在 | 核对目标会议和 ID 来源，必要时重新查询当前会议，不自动创建会议。 |
| 会议不是进行中 | 停止创建操作；若仅需已有绑定，改用只读详情查询，不自动重新发起会议。 |
| 调用者不在会中 | 核对当前身份是否仍在该会议中。说明需要该身份先入会；只有用户明确要求入会时才执行入会操作。 |
| 无权限 | 检查当前身份、`vc:meeting.interaction:write` 授权及应用安装配置，结合实际提示排查；不要靠换身份或反复登录绕过。 |
| 会议聊天不支持 | 告知当前会议场景不支持此操作；响应未说明具体原因时不猜测，不改用 IM 创建替代群。 |
| 服务异常、超时或响应缺少 chat_id | 先说明未确认成功，可用同一身份查询详情核对是否已有绑定。需要重试时保持相同会议和身份、限制次数；持续失败时提供错误信息供排查。 |

## 后续读取或发送消息

只有用户要求读写消息时，才沿用同一身份和返回的 `chat_id`，读取相应 IM reference 后继续操作：

- [im +chat-messages-list](../../lark-im/references/lark-im-chat-messages-list.md)：读取聊天消息。
- [im +messages-send](../../lark-im/references/lark-im-messages-send.md)：发送用户授权的消息。

IM 拒绝访问时，按 IM 返回的权限或成员条件处理，不重新创建会议聊天，也不自动换身份。临时参与不等于正式成员资格，不承诺离会后仍能访问消息。

## 相关

- [获取会议群聊](../scenes/query-meeting-and-artifacts.md#获取会议群聊)：只读查询当前绑定，或复用已有详情结果。
- [vc +meeting-list-active](lark-vc-meeting-list-active.md)：发现进行中的会议。
- [vc +meeting-join](lark-vc-agent-meeting-join.md)：用户明确要求应用机器人入会时使用。
