# 发送投递状态

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

发送后确认投递状态，处理发送拦截。命令选择见 [`../SKILL.md`](../SKILL.md) 的“命令选择”章节。

## 查询时机

- 立即发送：发送成功并返回非空 `message_id` 后立即查询。
- 定时发送：不要立即查询；等预定发送时间后，再使用发送产生的 `message_id` 查询投递状态。

## 立即发送

邮件发送成功后，若响应中包含非空 `message_id`，必须调用 `send_status` 查询投递状态并向用户报告。

```bash
lark-cli mail user_mailbox.messages send_status \
  --params '{"user_mailbox_id":"me","message_id":"<发送返回的 message_id>"}'
```

返回每个收件人的投递状态（`status`）：

| status | 含义 |
|--------|------|
| 1 | 正在投递 |
| 2 | 投递失败重试 |
| 3 | 退信 |
| 4 | 投递成功 |
| 5 | 待审批 |
| 6 | 审批拒绝 |

向用户简要报告结果；如有退信、审批拒绝等异常状态，需要重点提示。

## 发送被拦截

若发送响应中包含 `automation_send_disable_reason` / `automation_send_disable_reference`，说明邮件未真正发出，而是被邮箱设置拦截。

- 直接向用户展示拦截原因和草稿打开链接。
- 不要继续假设已经发送成功。
- 不要调用 `send_status`。

## 相关命令

- `lark-cli mail +send --confirm-send` — 发送新邮件。
- `lark-cli mail +reply --confirm-send` / `+reply-all --confirm-send` — 发送回复。
- `lark-cli mail +forward --confirm-send` — 发送转发。
