# mail +send-receipt

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

响应收到的已读回执请求。**本命令仅在对方邮件请求了已读回执（`READ_RECEIPT_REQUEST` 标签，系统 ID `-607`）时使用**，用于向原发件人发送一封短回复以告知"已阅读"。

本 skill 对应 shortcut：`lark-cli mail +send-receipt`。

## CRITICAL — 工作流与安全规则

1. **触发条件严格**：仅当拉信（`+message` / `+messages` / `+thread`）看到 `label_ids` 里有 `READ_RECEIPT_REQUEST` 时，才应该问用户是否发回执。对普通邮件**绝不**调用此命令。
2. **必须先问用户**：发回执之前**必须**向用户展示原邮件摘要（发件人、主题）并请求确认；用户明确同意后才执行。**不要替用户自动回执**——这会造成隐私泄露（告诉对方"我读了"）。
3. **`--yes` 不省略**：本命令被标记为 `high-risk-write`，框架要求 `--yes` 才执行（无 `--confirm-send` flag）。仅在用户确认后附上。
4. **失败安全**：若原邮件没有 `READ_RECEIPT_REQUEST` 标签，命令会拒绝执行并报错——这是防御，不要通过其他方式绕过。

## 命令

```bash
# 标准用法：对指定 message-id 发回执
lark-cli mail +send-receipt --message-id <message-id> --yes

# 指定邮箱（公共邮箱场景）
lark-cli mail +send-receipt --mailbox shared@example.com --message-id <message-id> --yes

# Dry Run（不真发）
lark-cli mail +send-receipt --message-id <message-id> --dry-run
```

## 参数

| 参数 | 必填 | 默认 | 说明 |
|------|------|------|------|
| `--message-id <id>` | 是 | — | 请求了已读回执的原邮件 message ID |
| `--mailbox <email>` | 否 | `me` | 回执邮件归属的邮箱 |
| `--from <email>` | 否 | 邮箱主地址 | 回执 From 头 |
| `--yes` | 是 | — | 确认高危写操作。仅在用户明确同意发回执后附上 |
| `--dry-run` | 否 | — | 仅打印请求，不执行 |

> **没有 `--body` 参数**：回执正文**由命令自动生成**（见下方"行为细节"），对齐业界惯例（Outlook / Thunderbird / Lark 客户端等均不支持逐封自定义回执正文）。若真需要自由回复，请改用 `mail +reply`——那本来就是"自由回复"的命令，不该与"已读回执"混用。

## 行为细节

- **Subject**：按原邮件主题语言（`detectSubjectLang`）自动选前缀 —— <code>已读回执：&lt;原邮件主题&gt;</code>（zh）或 <code>Read receipt:&nbsp;&lt;原邮件主题&gt;</code>（en）。后端 `GetRealSubject` 正则剥除这两类前缀用于会话聚合，zh 已内置；en 需在 TCC `MailPrefixConfig.SubjectPrefixListForAdvancedSearch` 加入 `Read receipt:`。
- **正文**（自动生成，纯文本 + HTML 双版本走 `multipart/alternative`）：
    - 按原邮件主题语言（`detectSubjectLang`）在 `zh` 与 `en` 之间切换，label 套通过 `receiptMetaLabels` 集中维护
    - 结构化 4 行（纯文本版，zh）：
      ```text
      您发送的邮件已被阅读，详情如下：
      > 主题：<原邮件主题>
      > 收件人：<回执发件人地址>
      > 发送时间：<原邮件发送时间>
      > 阅读时间：<当前时间>
      ```
    - en 版：`Your message has been read. Details:` + <code>Subject:&nbsp;</code> / <code>To:&nbsp;</code> / <code>Sent:&nbsp;</code> / <code>Read:&nbsp;</code>
    - HTML 版同信息量，包在一个浅灰 quote-block
- **会话挂接**：自动设置 `In-Reply-To`（原信的 SMTP Message-ID）和 `References`（原信 references + 原信 SMTP Message-ID），保证在发件人邮箱里聚合到原邮件回复链。
- **发送路径**：走现有 drafts raw 路径（`drafts.create` + `drafts.send`），与 `+send` / `+reply` 共用基础设施。后端会自动标记这是一封回执邮件并在原邮件会话里清除"请求回执"状态。
- **即时发送**：本命令不支持保存草稿——回执邮件按语义是"立即告知对方已读"，保存草稿无意义。

## 返回值

```json
{
  "ok": true,
  "data": {
    "message_id":             "回执邮件的 message ID",
    "thread_id":              "挂到原会话的 thread ID",
    "receipt_for_message_id": "原邮件的 message ID"
  }
}
```

`message_id` 可用于后续 `send_status` 查询投递状态。

## 典型场景

### 场景 1：用户在拉信时看到 `-607` 标签

```bash
# 1. 拉信
lark-cli mail +message --message-id msg-1 --format json | jq '.data.label_ids'
# 输出 ["UNREAD", "READ_RECEIPT_REQUEST"] → 原邮件请求了已读回执

# 2. 向用户提示：
#    "这封来自 alice@example.com 的邮件请求已读回执。主题：《周报》。
#     要不要回一封告诉对方你已阅读？"

# 3. 用户确认后发回执
lark-cli mail +send-receipt --message-id msg-1 --yes
```

### 场景 2：批量拉信中发现多封请求回执

```bash
# 1. 筛出带 -607 标签的邮件
lark-cli mail +triage --folder INBOX --format json \
  | jq '.data.messages[] | select(.label_ids | index("READ_RECEIPT_REQUEST")) | {message_id, subject, from}'

# 2. 对每封分别问用户 → 用户确认后再发
```

### 场景 3：公共邮箱的回执

```bash
# 公共邮箱收到的回执请求，用 --mailbox 指定
lark-cli mail +send-receipt --mailbox support@example.com --message-id <id> --yes
```

## 不要这样做

- ❌ **自动回执**（不经用户确认就发）——违反隐私规则
- ❌ 对普通邮件调用 `+send-receipt`（命令会拒绝，但 agent 也不应尝试）
- ❌ 用 `+send` / `+reply` 手工拼 "已读回执" 回复——会缺少 `X-Lark-Read-Receipt-Mail` 头，后端不会打 `-608` 标签，收信人看不到系统样式的回执
- ❌ 一次调用发多条（本命令设计为单次响应）

## 相关命令

- `lark-cli mail +message` — 拉单封邮件（在 `label_ids` 里检查 `READ_RECEIPT_REQUEST`）
- `lark-cli mail +send --request-receipt` — 反向：**请求**别人回执
- `lark-cli mail user_mailbox.messages send_status` — 查询回执邮件的投递状态
