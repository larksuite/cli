# mail +decline-receipt

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

关闭收到邮件的已读回执请求 banner，**但不向发件人发送回执**。**本命令仅在对方邮件请求了已读回执（`READ_RECEIPT_REQUEST` 标签，系统 ID `-607`）时使用**。对齐飞书客户端上已读回执 banner 右侧的"不发送"按钮。

本 skill 对应 shortcut：`lark-cli mail +decline-receipt`。

## 使用时机

决策分支：拉信看到 `READ_RECEIPT_REQUEST` 标签 → **必须先问用户**：

- 用户愿意告知对方"已读" → `+send-receipt`
- 用户不愿意告知但想消掉提示 → `+decline-receipt`（本命令）
- 用户既不想回执也不关心 banner → 什么都不做

## 命令

```bash
# 标准用法
lark-cli mail +decline-receipt --message-id <message-id>

# 指定邮箱（公共邮箱场景）
lark-cli mail +decline-receipt --mailbox shared@example.com --message-id <message-id>

# Dry Run（不真改）
lark-cli mail +decline-receipt --message-id <message-id> --dry-run
```

## 参数

| 参数 | 必填 | 默认 | 说明 |
|------|------|------|------|
| `--message-id <id>` | 是 | — | 请求了已读回执的原邮件 message ID |
| `--mailbox <email>` | 否 | `me` | 邮件归属的邮箱 |
| `--dry-run` | 否 | — | 仅打印请求，不执行 |

> 注意本命令没有 `--yes` —— 它只是移除一个本地 label，不对外发信，Risk 级别是 `write` 而非 `high-risk-write`。

## 行为细节

- 先 `fetchFullMessage` 拉一遍原邮件校验：若 `label_ids` 中不含 `READ_RECEIPT_REQUEST`（也不含数字 `-607`），直接返回 `already_cleared: true`，**不发请求**，幂等。
- 标签存在时调 `PUT /user_mailboxes/<mailbox>/messages/<id>/modify`（`user_mailbox.message.modify`），body `{"remove_label_ids":["READ_RECEIPT_REQUEST"]}`。
- **不发任何外发邮件**：等价于飞书客户端"不发送"按钮——只清除本地标签，发件人不会收到任何通知。

## 返回值

标签已清除（无副作用）：

```json
{
  "ok": true,
  "data": {
    "message_id":             "原邮件 message ID",
    "decline_receipt_for_id": "原邮件 message ID",
    "declined":               false,
    "already_cleared":        true
  }
}
```

本次真正移除了标签：

```json
{
  "ok": true,
  "data": {
    "message_id":             "原邮件 message ID",
    "decline_receipt_for_id": "原邮件 message ID",
    "declined":               true
  }
}
```

## 典型场景

### 场景 1：用户选择不发回执

```bash
# 1. 拉信
lark-cli mail +message --message-id msg-1 --format json | jq '.data.label_ids'
# → ["UNREAD", "READ_RECEIPT_REQUEST"]

# 2. 向用户提示：
#    "这封来自 alice@example.com 的邮件请求已读回执。主题：《周报》。
#     要不要回一封告诉对方你已阅读？
#     也可以选择：不发送回执，但关闭这条提示。"

# 3. 用户选了"不发送" → 
lark-cli mail +decline-receipt --message-id msg-1
```

### 场景 2：幂等重跑

```bash
# 第一次移除标签
lark-cli mail +decline-receipt --message-id msg-1
# → {"declined": true}

# 再跑一次 —— 不会报错，也不会再发 modify 请求
lark-cli mail +decline-receipt --message-id msg-1
# → {"declined": false, "already_cleared": true}
```

## 不要这样做

- ❌ 替用户自动 decline —— 违反隐私规则的对称面：不回执的"沉默"也属于用户选择
- ❌ 拿 `+decline-receipt` 当"标记已读"——它只移 `READ_RECEIPT_REQUEST` 一个标签，不改 `UNREAD`
- ❌ 在没有 `READ_RECEIPT_REQUEST` 标签的邮件上调用 —— 虽然幂等返回 `already_cleared`，但多发一次 GET 无意义

## 相关命令

- `lark-cli mail +send-receipt` — 同意回执（发一封系统样式的已读回执邮件）
- `lark-cli mail +message` — 拉单封邮件（在 `label_ids` 里检查 `READ_RECEIPT_REQUEST`）
- `lark-cli mail +send --request-receipt` — 反向：**请求**别人回执
