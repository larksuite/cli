# mail +reply

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

回复指定邮件，自动处理：
- 主题前缀 `Re: `（已含常见回复前缀时不重复叠加）
- 默认收件人为原邮件发件人
- RFC 2822 会话头（`In-Reply-To` / `References`）维护邮件会话

> **默认草稿模式**：`+reply` 默认保存为草稿，不会立即发送。如需立即发送，使用 `--confirm-send` 参数（须经用户明确确认）。**优先使用 `+reply` 而不是 `+draft-create` 来创建回复草稿**，因为 `+reply` 会自动处理主题、收件人和会话头。

本 skill 对应 shortcut：`lark-cli mail +reply`，内部步骤：
1. `GET /open-apis/mail/v1/user_mailboxes/me/messages/{message_id}` — 获取原邮件元数据
2. `GET /open-apis/mail/v1/user_mailboxes/me/profile` — 获取邮箱主地址（`primary_email_address`，填入默认 From 头）
3. `POST /open-apis/mail/v1/user_mailboxes/me/drafts` — 创建草稿
4. `POST /open-apis/mail/v1/user_mailboxes/me/drafts/{draft_id}/send` — 发送草稿（仅在指定 `--confirm-send` 时执行）

## CRITICAL — 发送工作流（必须遵循）

此命令默认**只保存草稿**，不会发送邮件。需要发送时，**必须**按以下步骤操作：

**Step 1** — 创建回复草稿（不带 `--confirm-send`）：
```bash
lark-cli mail +reply --message-id <邮件ID> --body '<回复正文>'
```
→ 返回 `draft_id`

**Step 2** — 向用户展示回复摘要（目标邮件、回复内容、收件人），请求确认发送

**Step 3** — 用户明确同意后，发送该草稿：
```bash
lark-cli mail user_mailbox.drafts send --params '{"user_mailbox_id":"me","draft_id":"<Step 1 返回的 draft_id>"}'
```

**禁止跳过 Step 1 直接使用 `--confirm-send`。禁止在用户未明确同意的情况下执行 Step 3。**

## 命令

```bash
# 回复一封邮件（默认保存为草稿，返回 draft_id）— HTML 推荐
lark-cli mail +reply --message-id <邮件ID> --body '<p><b>已收到</b>，稍后跟进。</p>'

# 回复并追加收件人/抄送（保存为草稿）
lark-cli mail +reply --message-id <邮件ID> --body '<p>已处理</p>' --to lead@example.com --cc colleague@example.com

# 回复时插入内嵌图片（CID 为唯一标识符，可用随机字符串）
lark-cli mail +reply --message-id <邮件ID> --body '<img src="cid:a1b2c3d4e5f6a7b8c9d0"> 详见图示。' --inline '[{"cid":"a1b2c3d4e5f6a7b8c9d0","file_path":"./logo.png"}]'

# 纯文本回复（仅在内容极简时使用）
lark-cli mail +reply --message-id <邮件ID> --body '收到，谢谢！'

# 指定发件人地址
lark-cli mail +reply --message-id <邮件ID> --body '收到' --from me@example.com

# 确认发送回复（用户明确确认后使用）
lark-cli mail +reply --message-id <邮件ID> --body '<p>收到，谢谢！</p>' --confirm-send

# Dry Run（仅打印请求，不执行）
lark-cli mail +reply --message-id <邮件ID> --body '<p>测试</p>' --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--message-id <id>` | 是 | 被回复的邮件 ID |
| `--body <text>` | 是 | 回复正文。推荐使用 HTML 获得富文本排版；也支持纯文本。根据回复正文和原邮件正文自动检测 HTML。使用 `--plain-text` 可强制纯文本模式 |
| `--from <email>` | 否 | 发件人邮箱地址（默认读取 user_mailboxes.profile.primary_email_address） |
| `--to <emails>` | 否 | 额外收件人，多个用逗号分隔（追加到原发件人） |
| `--cc <emails>` | 否 | 抄送邮箱，多个用逗号分隔 |
| `--bcc <emails>` | 否 | 密送邮箱，多个用逗号分隔 |
| `--plain-text` | 否 | 强制纯文本模式，忽略所有 HTML 自动检测。不可与 `--inline` 同时使用 |
| `--attach <paths>` | 否 | 附件文件路径，多个用逗号分隔。相对路径 |
| `--inline <json>` | 否 | 内嵌图片 JSON 数组，每项包含 `cid`（唯一标识符，可用随机十六进制字符串，如 `a1b2c3d4e5f6a7b8c9d0`）和 `file_path`（相对路径）。格式：`'[{"cid":"a1b2c3d4e5f6a7b8c9d0","file_path":"./logo.png"}]'`。不可与 `--plain-text` 同时使用，在 body 中用 `<img src="cid:...">` 引用 |
| `--confirm-send` | 否 | 确认发送回复（默认只保存草稿）。仅在用户明确确认后使用 |
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

`--confirm-send` 模式（发送成功）：
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

### 场景 1：用户说"帮我写个回复草稿"（只创建草稿）
```bash
lark-cli mail +reply --message-id <邮件ID> --body '<p>收到，谢谢！</p>'
```
→ 返回 `draft_id`，告诉用户回复草稿已创建。**注意：用 `+reply` 而不是 `+draft-create`**，这样草稿会自动关联原邮件的主题、收件人和会话头。

### 场景 2：用户说"回复这封邮件说已处理"（需要发送）
```bash
# Step 1: 创建回复草稿
lark-cli mail +reply --message-id <邮件ID> --body '<p>已处理，谢谢。</p>'
# → 返回 draft_id

# Step 2: 向用户确认 "回复草稿已创建：回复给 alice@example.com，内容「已处理，谢谢。」确认发送吗？"

# Step 3: 用户确认后发送
lark-cli mail user_mailbox.drafts send --params '{"user_mailbox_id":"me","draft_id":"<draft_id>"}'
```

## 实现说明

### 会话维护

本 shortcut 通过 raw EML 方式发送，包含标准 RFC 2822 会话头：

```
In-Reply-To: <原邮件smtp_message_id>
References:  <原邮件references + smtp_message_id>
```

若原邮件有 `thread_id`，发送时会一并传入，确保回复归入同一会话。

### 收件人与引用

- 默认回复给原邮件发件人（`head_from`）
- `--to` 会在默认收件人基础上追加
- 自动拼接引用块（纯文本或 HTML）

## 发送后跟进

回复发送成功后：

**1. 确认投递状态**（必须）— 用返回的 `message_id` 查询投递状态：

```bash
lark-cli mail user_mailbox.messages send_status --params '{"user_mailbox_id":"me","message_id":"<发送返回的 message_id>"}'
```

状态码：1=正在投递, 2=投递失败重试, 3=退信, 4=投递成功, 5=待审批, 6=审批拒绝。向用户简要报告投递结果，异常状态需重点提示。

**2. 标记已读**（可选）— 询问用户是否需要将原邮件标记为已读。如果用户同意：

```bash
lark-cli mail user_mailbox.messages batch_modify_message --params '{"user_mailbox_id":"me"}' --data '{"message_ids":["<原邮件ID>"],"remove_label_ids":["UNREAD"]}'
```

## 编辑回复草稿

`+reply` 创建的草稿正文包含引用区（原邮件的引用块）。如果需要编辑回复草稿的正文，**必须通过 `--patch-file` 使用 `set_reply_body` op**，它仅替换用户撰写部分，自动保留引用区。value 只传新的用户撰写内容，不要包含引用区。

```bash
# 编辑回复草稿正文（自动保留引用区）
cat > ./patch.json << 'EOF'
{ "ops": [{ "op": "set_reply_body", "value": "<p>修改后的回复内容</p>" }] }
EOF
lark-cli mail +draft-edit --draft-id <draft_id> --patch-file ./patch.json
```

如果用户要修改引用区内容或去掉引用区，则使用 `set_body` 全量替换。

## 注意事项

- 需要已登录（`lark-cli auth login --scope "mail:user_mailbox.message:modify mail:user_mailbox.message:readonly mail:user_mailbox:readonly"`）且具备写/读邮件权限
- 邮件 ID 可从 `lark-cli mail user_mailbox.messages list` 获取
- `--bcc` 仅在发送链路中生效，通常不会在收件方看到

## 相关命令

- `lark-cli mail user_mailbox.messages list` — 列出邮件
- `lark-cli mail user_mailbox.messages get` — 读取邮件详情
- `lark-cli mail +reply-all` — 回复全部
- `lark-cli mail +forward` — 转发邮件
