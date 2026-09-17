# mail send_as

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

使用公共邮箱或别名发信。适用于 `+send` / `+draft-create` / `+reply` / `+reply-all` / `+forward` 等发信类 shortcut。

## 参数含义

- `--mailbox` 指定邮件归属邮箱（如 `shared@example.com` 或 `me`），可通过 `accessible_mailboxes` 查询可用值。
- `--from` 指定 EML From 头里的发件人地址（别名、邮件组等），可通过 `send_as` 查询可用值。
- 不使用公共邮箱或别名时无需指定 `--mailbox`，行为与默认发信一致。

## 查询可用邮箱和发信地址

```bash
# 查询可访问的邮箱（主邮箱 + 公共邮箱）
lark-cli mail user_mailboxes accessible_mailboxes --params '{"user_mailbox_id":"me"}'

# 查询某个邮箱的可用发信地址（主地址、别名、邮件组）
lark-cli mail user_mailbox.settings send_as --params '{"user_mailbox_id":"me"}'
```

## 公共邮箱发信

```bash
# --mailbox 指定公共邮箱，From 头自动使用该邮箱地址
lark-cli mail +send --mailbox shared@example.com \
  --to bob@example.com --subject '通知' --body '<p>你好</p>'
```

## 别名发信

```bash
# --mailbox 指定所属邮箱，--from 指定别名地址
lark-cli mail +send --mailbox me --from alias@example.com \
  --to bob@example.com --subject '测试' --body '<p>你好</p>'
```

## 相关命令

- `lark-cli mail +send` — 新邮件发信。
- `lark-cli mail +draft-create` — 新建草稿。
- `lark-cli mail +reply` / `+reply-all` — 回复邮件。
- `lark-cli mail +forward` — 转发邮件。
