# mail +send

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

发送新邮件，支持：
- 纯文本或 HTML 正文
- 抄送/密送
- 本地文件附件（`--attach`）
- 内嵌图片（`--inline`，CID 可用随机字符串）

本 skill 对应 shortcut：`lark-cli mail +send`。

## CRITICAL — 发送工作流（必须遵循）

此命令默认**只保存草稿**，不会发送邮件。需要发送时，有两种合规方式：

**方式 A（推荐）** — 先创建草稿，再确认发送：
```bash
lark-cli mail +send --to <收件人> --subject '<主题>' --body '<正文>'
```
→ 返回 `draft_id`

向用户展示邮件摘要（收件人、主题、正文预览）；如果用户想先看效果，可引导其去飞书邮件里打开该草稿查看详情。

用户明确同意后，发送该草稿：
```bash
lark-cli mail user_mailbox.drafts send --params '{"user_mailbox_id":"me","draft_id":"<Step 1 返回的 draft_id>"}'
```

**方式 B（允许）** — 用户已经明确确认收件人和内容时，可直接使用 `--confirm-send` 立即发送：
```bash
lark-cli mail +send --to <收件人> --subject '<主题>' --body '<正文>' --confirm-send
```

**禁止在用户未明确同意的情况下执行发送，无论是发送草稿还是直接使用 `--confirm-send`。**

## 命令

```bash
# 保存为草稿（默认行为，不发送）— HTML 格式推荐
lark-cli mail +send --to alice@example.com --subject '周报' \
  --body '<p>本周进展：</p><ul><li>完成 A 模块</li><li>修复 3 个 bug</li></ul>'

# 保存为草稿并抄送
lark-cli mail +send --to alice@example.com --cc bob@example.com --subject '状态更新' --body '<b>已完成</b>'

# 确认发送（仅在用户明确确认后使用）
lark-cli mail +send --to alice@example.com --subject '周报' \
  --body '<p>本周进展如下...</p>' --confirm-send

# 保存带附件的草稿
lark-cli mail +send --to alice@example.com --subject '请查收' --body '<p>见附件</p>' --attach ./report.pdf,./logs.zip

# 保存带内嵌图片的草稿（推荐：直接用相对路径，自动解析）
lark-cli mail +send --to alice@example.com --subject '预览图' --body '<img src="./logo.png" />'

# 纯文本邮件（仅在内容极简时使用）
lark-cli mail +send --to alice@example.com --subject '确认' --body '收到，谢谢'

# Dry Run（仅打印请求，不执行）
lark-cli mail +send --to alice@example.com --subject '测试' --body '<p>test</p>' --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--to <emails>` | 是 | 收件人邮箱，多个用逗号分隔 |
| `--subject <text>` | 是 | 邮件主题 |
| `--body <text>` | 是 | 邮件正文。推荐使用 HTML 获得富文本排版；也支持纯文本（自动检测）。使用 `--plain-text` 可强制纯文本模式。支持 `<img src="./local.png" />` 相对路径自动解析为内嵌图片（仅支持相对路径，不支持绝对路径） |
| `--from <email>` | 否 | 发件人邮箱地址（EML From 头）。使用别名（send_as）发信时，设为别名地址并配合 `--mailbox` 指定所属邮箱。默认读取邮箱主地址 |
| `--mailbox <email>` | 否 | 邮箱地址，指定草稿所属的邮箱（默认回退到 `--from`，再回退到 `me`）。当发件人（`--from`）与邮箱不同时使用。可通过 `accessible_mailboxes` 查询可用邮箱 |
| `--cc <emails>` | 否 | 抄送邮箱，多个用逗号分隔 |
| `--bcc <emails>` | 否 | 密送邮箱，多个用逗号分隔 |
| `--plain-text` | 否 | 强制纯文本模式，忽略 HTML 自动检测。不可与 `--inline` 同时使用 |
| `--attach <paths>` | 否 | 附件文件路径，多个用逗号分隔。相对路径。当附件导致 EML 总大小超过 25 MB 时，超出部分自动上传为超大附件（HTML 邮件插入下载卡片，纯文本邮件追加下载链接），单个文件上限 3 GB |
| `--inline <json>` | 否 | 高级用法：手动指定内嵌图片 CID 映射。推荐直接在 `--body` 中使用 `<img src="./path" />`（自动解析）。仅在需要精确控制 CID 命名时使用此参数。格式：`'[{"cid":"mycid","file_path":"./logo.png"}]'`，在 body 中用 `<img src="cid:mycid">` 引用。不可与 `--plain-text` 同时使用 |
| `--signature-id <id>` | 否 | 签名 ID。附加邮箱签名到正文末尾。运行 `mail +signature` 查看可用签名。不可与 `--plain-text` 同时使用 |
| `--priority <level>` | 否 | 邮件优先级：`high`、`normal`、`low`。省略或 `normal` 时不设置优先级 |
| `--confirm-send` | 否 | 确认发送邮件（默认只保存草稿）。仅在用户明确确认收件人和内容后使用 |
| `--send-time <timestamp>` | 否 | 定时发送时间，Unix 时间戳（秒）。需至少为当前时间 + 5 分钟。配合 `--confirm-send` 使用可定时发送邮件 |
| `--dry-run` | 否 | 仅打印请求，不执行 |

## 返回值

**草稿模式（默认）：**

```json
{
  "ok": true,
  "data": {
    "draft_id": "草稿ID",
    "tip": "draft saved. To send: lark-cli mail user_mailbox.drafts send --params '{...}'"
  }
}
```

草稿模式下，只要结果不是直接发信而是产出了草稿，就应给用户展示草稿打开链接。当前应以 `create` / `edit` / `send` 链路返回的链接信息为准，不要把 `user_mailbox.drafts get` 当作拿草稿打开链接的来源。如果返回中带有 `reference`，应把链接与 `draft_id` 一并返回；当前没有链接时，静默处理，不要伪造链接。

**发送模式（`--confirm-send`）：**

```json
{
  "ok": true,
  "data": {
    "message_id": "邮件ID",
    "thread_id": "会话ID"
  }
}
```

可选字段：

- `automation_send_disable_reason`：发送被邮箱自动化设置拦截时返回的原因
- `automation_send_disable_reference`：发送被拦截时的草稿打开链接

字段语义：

- 若返回中包含 `automation_send_disable_reason` / `automation_send_disable_reference`，说明邮件未真正发出，而是被邮箱设置拦截。此时应直接向用户展示原因和草稿打开链接，不要继续假设已经发送成功

## 典型场景

### 场景 1：用户说"帮我写一封邮件给 Alice"（只创建草稿）
```bash
lark-cli mail +send --to alice@example.com --subject '周报' --body '<p>本周进展如下...</p>'
```
→ 返回草稿结果时，如输出中带有草稿打开链接，则一起展示给用户；如果当前输出没有链接，则静默处理。如果用户想先看效果，可去飞书邮件 UI 中打开草稿查看详情。

### 场景 2：用户说"发邮件给 Alice 说收到了"（需要发送）
```bash
# 方式 A: 创建草稿
lark-cli mail +send --to alice@example.com --subject '收到' --body '<p>已收到，谢谢！</p>'
# → 返回 draft_id

# 向用户确认 "当前收件人 alice@example.com，主题「收到」。如果你想先看效果，也可以先去飞书邮件里打开草稿查看详情。确认发送吗？"

# 用户确认后发送
lark-cli mail user_mailbox.drafts send --params '{"user_mailbox_id":"me","draft_id":"<draft_id>"}'

# 方式 B: 用户已明确确认时，直接发送
lark-cli mail +send --to alice@example.com --subject '收到' --body '<p>已收到，谢谢！</p>' --confirm-send
```

### 场景 3：用户说"下午 3 点给 Alice 发一封周报"（定时发送）
```bash
# Step 1: 创建草稿（定时发送也走草稿流程）
lark-cli mail +send --to alice@example.com --subject '周报' --body '<p>本周进展如下...</p>'
# → 返回 draft_id

# Step 2: 向用户确认 "邮件草稿已创建：收件人 alice@example.com，主题「周报」，定时 <目标时间> 发送。确认吗？"

# Step 3: 用户确认后定时发送（send_time 为 Unix 时间戳，需至少当前时间 + 5 分钟）
lark-cli mail user_mailbox.drafts send --params '{"user_mailbox_id":"me","draft_id":"<draft_id>"}' --data '{"send_time":"<unix_timestamp>"}'
```

### 场景 4：用户说"等等，先不发那封邮件了"（取消定时发送）
```bash
# 取消定时发送（取消后邮件变回草稿）
lark-cli mail user_mailbox.drafts cancel_scheduled_send --params '{"user_mailbox_id":"me","draft_id":"<draft_id>"}'
```
→ 取消成功后邮件恢复为草稿状态，用户可重新编辑或在之后重新发送。

## 发送后跟进

邮件发送后，分两种情况处理：

- 若返回中有 `automation_send_disable_reason` / `automation_send_disable_reference`：说明发送被邮箱设置拦截，应直接告诉用户原因并提供草稿打开链接，**不要**调用 `send_status`

### 立即发送（无 `--send-time`）

若返回非空 `message_id`，调用：

```bash
lark-cli mail user_mailbox.messages send_status --params '{"user_mailbox_id":"me","message_id":"<发送返回的 message_id>"}'
```

状态码：1=正在投递, 2=投递失败重试, 3=退信, 4=投递成功, 5=待审批, 6=审批拒绝。向用户简要报告各收件人投递结果，异常状态需重点提示。

### 定时发送（指定了 `--send-time`）

定时发送不会立即产生 `message_id`，因此 `send_status` 在定时发送成功后会返回"待发送"状态，**不建议在定时发送后立即查询**。可在预定发送时间后再查询投递状态。

如需取消定时发送，可在预定时间前调用取消接口：

```bash
lark-cli mail user_mailbox.drafts cancel_scheduled_send --params '{"user_mailbox_id":"me","draft_id":"<draft_id>"}'
```

**取消后邮件会变回草稿**，可继续编辑或在之后重新发送。

## 实现说明

- 使用 EML 构建器生成完整 MIME 邮件并 base64url 编码后发送。
- `--attach` 作为普通附件添加。相对路径。
- `--inline` 接受 JSON 数组，每项需提供 `cid`（唯一标识符，可用随机十六进制字符串）和 `file_path`（相对路径），作为 inline part 嵌入邮件。
- **超大附件**：当附件导致 EML 总大小（headers + body + inline images + attachments，base64 编码后）超过 25 MB 时，超出的文件自动通过 `medias/upload_*` API 上传到云端。HTML 邮件插入与飞书客户端一致的下载卡片；纯文本邮件追加包含文件名、大小和下载链接的文本块。单个文件上限 3 GB，总附件数量上限 250 个。

## 相关命令

- `lark-cli mail +reply` — 回复邮件
- `lark-cli mail +reply-all` — 回复全部
- `lark-cli mail +forward` — 转发邮件
- `lark-cli mail user_mailbox.messages list` — 列出邮件
