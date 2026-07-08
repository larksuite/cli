
# mail +watch

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

实时监听新邮件事件（`mail.user_mailbox.event.message_received_v1`）。新代码优先直接使用统一事件入口 `event consume mail.message_received_v1`；`mail +watch` 保留为兼容封装。

**权限要求：** 应用需要 `mail:event`、`mail:user_mailbox.message:readonly` 权限，以及字段权限 `mail:user_mailbox.message.address:read`、`mail:user_mailbox.message.subject:read`、`mail:user_mailbox.message.body:read`，且机器人需订阅事件 `mail.user_mailbox.event.message_received_v1`。按需权限（缺失时会提示申请）：使用 `--folders` / `--folder-ids` 筛选自定义文件夹时需要 `mail:user_mailbox.folder:read`；使用 `--labels` / `--label-ids` 筛选自定义标签时需要 `mail:user_mailbox.message:modify`。

## 命令

```bash
# 推荐：统一事件消费，输出 message 元数据 NDJSON
lark-cli event consume mail.message_received_v1 --as user -p mailbox=me

# AI 子进程：等待 stderr ready marker 后读取 stdout；收到 1 条或 30 秒后退出
lark-cli event consume mail.message_received_v1 --as user -p mailbox=me --max-events 1 --timeout 30s

# 直接用 jq 过滤输出
lark-cli event consume mail.message_received_v1 --as user -p mailbox=me --jq '.message.message_id'

# 按文件夹/标签过滤（名称和 ID 均支持）
lark-cli event consume mail.message_received_v1 --as user \
  -p mailbox=me \
  -p folders='["收件箱项目"]' \
  -p label_ids='["FLAGGED"]'

# 写入文件
lark-cli event consume mail.message_received_v1 --as user -p mailbox=me --output-dir ./mail-events

# 兼容旧入口：内部转为 event consume，不再自建 WebSocket
lark-cli mail +watch --msg-format metadata --format data

# 兼容旧入口：监听指定邮箱
lark-cli mail +watch --mailbox alice@company.com

# 查看各 --msg-format 的输出字段说明（解析前先运行）
lark-cli mail +watch --print-output-schema
```

## EventKey 参数

| 参数 | 默认 | 说明 |
|------|------|------|
| `-p mailbox=<id>` | `me` | 订阅目标邮箱；`me` 会在启动时解析为真实邮箱地址用于本地过滤 |
| `-p msg_format=<mode>` | `metadata` | 输出模式：`metadata` / `minimal` / `plain_text_full` / `full` / `event` |
| `-p folder_ids=<json-array>` | — | 文件夹 ID 过滤，如 `["INBOX","SENT"]` |
| `-p folders=<json-array>` | — | 文件夹名称过滤（与 `folder_ids` 取并集） |
| `-p label_ids=<json-array>` | — | 标签 ID 过滤，如 `["FLAGGED","IMPORTANT"]` |
| `-p labels=<json-array>` | — | 标签名称过滤（与 `label_ids` 取并集） |

> **过滤逻辑：** `--folder-ids`/`--folders` 与 `--label-ids`/`--labels` 之间是 **AND** 关系，即邮件必须**同时**匹配指定的文件夹和标签才会输出。同类参数内部是 **OR** 关系（匹配其中任一即可）。新收到的邮件通常只有系统标签（如 `UNREAD`、`IMPORTANT`），不会自动带有自定义标签。

通用消费选项由 `event consume` 提供：`--jq`、`--max-events`、`--timeout`、`--output-dir`、ready marker、控制台事件预检和订阅清理。

## 兼容入口参数

`mail +watch` 仍支持旧参数名：`--mailbox`、`--msg-format`、`--format json|data`、`--folders`、`--folder-ids`、`--labels`、`--label-ids`、`--output-dir`、`--max-events`、`--timeout`、`--jq`。

## --msg-format 输出结构

每条事件输出为一行 NDJSON。

**`metadata`**（默认，适合分拣/通知）
```json
{"message":{"message_id":"...","thread_id":"...","subject":"...","head_from":{"name":"Alice","mail_address":"alice@example.com"},"to":[{"name":"Bob","mail_address":"bob@example.com"}],"folder_id":"INBOX","label_ids":["IMPORTANT"],"internal_date":"1742800000000","message_state":1,"body_preview":"Please find attached..."}}
```

**`minimal`**（仅 ID 和状态，适合追踪已读/文件夹变更）
```json
{"message":{"message_id":"...","thread_id":"...","folder_id":"INBOX","label_ids":["IMPORTANT"],"internal_date":"1742800000000","message_state":1}}
```

**`plain_text_full`**（metadata 全部字段 + 完整纯文本正文）
```json
{"message":{"message_id":"...","subject":"...","head_from":{},"folder_id":"INBOX","label_ids":[],"body_preview":"...","body_plain_text":"<plain text>"}}
```

**`event`**（原始事件，不发起 message API 请求，适合调试）
```json
{"header":{"event_id":"abc123","event_type":"mail.user_mailbox.event.message_received_v1","create_time":"1742800000000"},"event":{"message_id":"...","mail_address":"user@example.com"}}
```

**`full`**（全部字段，含 HTML 正文和附件）
```json
{"message":{"message_id":"...","subject":"...","head_from":{},"body_preview":"...","body_plain_text":"<plain text>","body_html":"<base64url>","attachments":[{"name":"report.pdf","size":102400}]}}
```

`mail +watch --format json` 会为兼容旧脚本把上述 payload 包进 `{"ok":true,"data":...}` envelope；直接 `event consume` 不包 envelope。

## 参考

- [lark-mail](../SKILL.md) — 邮箱域总览
- [lark-mail-triage](lark-mail-triage.md) — 邮件摘要列表
- [lark-event-subscribe](../../lark-event/references/lark-event-subscribe.md) — 通用事件订阅
