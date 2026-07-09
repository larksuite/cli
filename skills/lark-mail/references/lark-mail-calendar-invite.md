# 发送日程邀请邮件

在邮件中嵌入日程邀请（`text/calendar`），收件人收信后可直接接受或拒绝日程。`To` / `Cc` 收件人自动成为参会人（ATTENDEE），发件人自动成为组织者（ORGANIZER）。

适用于发信类 shortcut：`+send` / `+draft-create` / `+reply` / `+reply-all` / `+forward`。

## 命令示例

```bash
# 发送带日程邀请的新邮件
lark-cli mail +send --as user \
  --to alice@example.com --cc bob@example.com \
  --subject '产品评审' \
  --body '<p>请参加本次产品评审会议。</p>' \
  --event-summary '产品评审' \
  --event-start '2026-05-10T14:00+08:00' \
  --event-end '2026-05-10T15:00+08:00' \
  --event-location '5F 大会议室' \
  --confirm-send
```

## 参数

- `--event-summary`：日程标题。设置此参数即开启日程邀请模式，需同时设置 `--event-start` 和 `--event-end`。
- `--event-start` / `--event-end`：ISO 8601 格式时间，如 `2026-05-10T14:00+08:00`。
- `--event-location`：可选，日程地点。

## 约束

- `--event-summary`、`--event-start`、`--event-end` 必须同时出现或同时不出现。
- `--event-*` 与 `--send-time`（定时发送）互斥，不可同时使用；日程邀请必须立即发送，否则收件人可能在日程开始后才收到。
- 不可与 `--bcc` 同时使用：Bcc 收件人不会成为日程参会人，且该组合会导致发送失败。需要邀请某人参加日程请用 `--to` 或 `--cc`；如只想告知而不邀请，请单独发一封无日程的邮件。

## 读取日程邀请

读取含日程邀请的邮件时，`calendar_event` 字段包含日程详情（`method`、`summary`、`start`、`end`、`organizer`、`attendees` 等）。详见 [lark-mail-message](lark-mail-message.md)。
