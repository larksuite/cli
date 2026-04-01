# mail +forward

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

转发指定邮件，自动处理：
- 主题前缀 `Fwd: `（已含前缀时不重复）
- 自动拼接标准 “Forwarded message” 区块（From/Date/Subject/To + 原文）
- 支持纯文本和 HTML 转发

> **默认草稿**：`+forward` 默认保存为草稿，不会立即发送。如需立即发送，添加 `--confirm-send` 参数（仅在用户明确确认后使用）。

本 skill 对应 shortcut：`lark-cli mail +forward`。

## CRITICAL — 发送工作流（必须遵循）

此命令默认**只保存草稿**，不会发送邮件。转发会将原邮件内容发送给新收件人，需要发送时**必须**按以下步骤操作：

**Step 1** — 创建转发草稿（不带 `--confirm-send`）：
```bash
lark-cli mail +forward --message-id <邮件ID> --to <收件人>
```
→ 返回 `draft_id`

**Step 2** — 向用户展示转发摘要（被转发邮件、收件人、附加说明），请求确认发送

**Step 3** — 用户明确同意后，发送该草稿：
```bash
lark-cli mail user_mailbox.drafts send --params '{"user_mailbox_id":"me","draft_id":"<Step 1 返回的 draft_id>"}'
```

**禁止跳过 Step 1 直接使用 `--confirm-send`。禁止在用户未明确同意的情况下执行 Step 3。**

## 命令

```bash
# 转发邮件（默认保存为草稿）— HTML 推荐
lark-cli mail +forward --message-id <邮件ID> --to alice@example.com --body '<p>FYI，请看下面原邮件。</p>'

# 转发并附加说明 + 抄送（草稿）
lark-cli mail +forward --message-id <邮件ID> --to alice@example.com --cc bob@example.com --body '<b>请参考</b>'

# 转发时插入内嵌图片（CID 为唯一标识符，可用随机字符串）
lark-cli mail +forward --message-id <邮件ID> --to alice@example.com --body '<img src="cid:a1b2c3d4e5f6a7b8c9d0"> 详见图示。' --inline '[{"cid":"a1b2c3d4e5f6a7b8c9d0","file_path":"./logo.png"}]'

# 纯文本转发（仅在内容极简时使用）
lark-cli mail +forward --message-id <邮件ID> --to alice@example.com

# 确认发送（用户明确确认后才可使用）
lark-cli mail +forward --message-id <邮件ID> --to alice@example.com --confirm-send

# Dry Run（仅打印请求，不发送）
lark-cli mail +forward --message-id <邮件ID> --to alice@example.com --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--message-id <id>` | 是 | 被转发的邮件 ID |
| `--to <emails>` | 是 | 收件人邮箱，多个用逗号分隔 |
| `--body <text>` | 否 | 转发时附加的说明文字。推荐使用 HTML 获得富文本排版；也支持纯文本。根据转发正文和原邮件正文自动检测 HTML。使用 `--plain-text` 可强制纯文本模式 |
| `--from <email>` | 否 | 发件人邮箱地址（默认读取 user_mailboxes.profile.primary_email_address） |
| `--cc <emails>` | 否 | 抄送邮箱，多个用逗号分隔 |
| `--bcc <emails>` | 否 | 密送邮箱，多个用逗号分隔 |
| `--plain-text` | 否 | 强制纯文本模式，忽略所有 HTML 自动检测。不可与 `--inline` 同时使用 |
| `--attach <paths>` | 否 | 附件文件路径，多个用逗号分隔，追加在原邮件附件之后。相对路径 |
| `--inline <json>` | 否 | 内嵌图片 JSON 数组，每项包含 `cid`（唯一标识符，可用随机十六进制字符串，如 `a1b2c3d4e5f6a7b8c9d0`）和 `file_path`（相对路径）。格式：`'[{"cid":"a1b2c3d4e5f6a7b8c9d0","file_path":"./logo.png"}]'`。不可与 `--plain-text` 同时使用，在 body 中用 `<img src="cid:...">` 引用 |
| `--confirm-send` | 否 | 确认发送转发（默认只保存草稿）。仅在用户明确确认后使用 |
| `--dry-run` | 否 | 仅打印请求，不执行 |

## 返回值

默认（草稿模式）：
```json
{
  "ok": true,
  "data": {
    "draft_id": "草稿ID",
    "tip": "draft saved. To send: lark-cli mail user_mailbox.drafts send --params '{...}'"
  }
}
```

`--confirm-send` 模式：
```json
{
  "ok": true,
  "data": {
    "message_id": "邮件ID",
    "thread_id":  "会话ID"
  }
}
```

## 典型场景

### 场景 1：用户说"把这封邮件转发给 Bob"（只创建草稿）
```bash
lark-cli mail +forward --message-id <邮件ID> --to bob@example.com --body '<p>FYI</p>'
```
→ 返回 `draft_id`，告诉用户转发草稿已创建。

### 场景 2：用户说"转发给 Bob 并发送"（需要发送）
```bash
# Step 1: 创建转发草稿
lark-cli mail +forward --message-id <邮件ID> --to bob@example.com --body '<p>FYI，请查收。</p>'
# → 返回 draft_id

# Step 2: 向用户确认 "转发草稿已创建：收件人 bob@example.com。确认发送吗？"

# Step 3: 用户确认后发送
lark-cli mail user_mailbox.drafts send --params '{"user_mailbox_id":"me","draft_id":"<draft_id>"}'
```

## 转发整个会话

`+forward` 操作的是单封邮件（`--message-id`），但转发整个会话时应 forward **会话中最后一条消息**，因为邮件客户端会将完整的回复链嵌套在最新一条中。典型流程：

```bash
# 1. 用 +triage 或 +thread 找到会话
lark-cli mail +thread --thread-id <THREAD_ID> --html=false --format json

# 2. 取最后一条消息的 message_id
#    messages 按时间升序排列，最后一条 = messages[-1].message_id

# 3. 转发该消息
lark-cli mail +forward --message-id <最后一条的message_id> --to recipient@example.com --body '请过目'
```

## 实现说明

- 自动拉取原邮件后构建转发内容。
- 纯文本模式下会生成标准转发头块并附上原文文本。
- HTML 模式下会生成结构化转发块并尽量保留原 HTML 正文。

## 发送后跟进

转发发送成功后：

**1. 确认投递状态**（必须）— 用返回的 `message_id` 查询投递状态：

```bash
lark-cli mail user_mailbox.messages send_status --params '{"user_mailbox_id":"me","message_id":"<发送返回的 message_id>"}'
```

状态码：1=正在投递, 2=投递失败重试, 3=退信, 4=投递成功, 5=待审批, 6=审批拒绝。向用户简要报告投递结果，异常状态需重点提示。

**2. 标记已读**（可选）— 询问用户是否需要将原邮件标记为已读。如果用户同意：

```bash
lark-cli mail user_mailbox.messages batch_modify_message --params '{"user_mailbox_id":"me"}' --data '{"message_ids":["<原邮件ID>"],"remove_label_ids":["UNREAD"]}'
```

## 编辑转发草稿

`+forward` 创建的草稿正文包含引用区（原邮件的引用块）。如果需要编辑转发草稿的正文，**必须通过 `--patch-file` 使用 `set_reply_body` op**，它仅替换用户撰写部分，自动保留引用区。value 只传新的用户撰写内容，不要包含引用区。

```bash
# 编辑转发草稿正文（自动保留引用区）
cat > ./patch.json << 'EOF'
{ "ops": [{ "op": "set_reply_body", "value": "<p>修改后的转发附言</p>" }] }
EOF
lark-cli mail +draft-edit --draft-id <draft_id> --patch-file ./patch.json
```

如果用户要修改引用区内容或去掉引用区，则使用 `set_body` 全量替换。

## 相关命令

- `lark-cli mail +send` — 发送新邮件
- `lark-cli mail +reply` — 回复邮件
- `lark-cli mail user_mailbox.messages get` — 查看邮件详情
