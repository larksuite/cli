# mail recipient search

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

查找收件人邮箱地址，可搜索个人、企业邮件组、群邮件地址和外部联系人。

## 何时使用

- 用户只给了人名：如"给张三发邮件" -> query `"张三"`。
- 用户只给了邮箱关键词：如"发到 larkmail 的邮箱" -> query `"@larkmail"`。
- 用户只给了群名：如"发给项目群" -> query `"项目群"`。
- 用户直接提供完整邮箱地址时不需要搜索，直接使用即可。

## 命令

```bash
lark-cli mail multi_entity search --as user --data '{"query":"<关键词>"}'
```

## 结果类型

| `type` 值 | `tag` 示例 | 说明 |
|-----------|-----------|------|
| `user` / `chatter` | `chatter` | 个人用户 |
| `enterprise_mail_group` | `mail_group` | 企业邮件组 |
| `chat` / `group` | `chat_group_tenant` / `chat_group_normal` | 群聊（有群邮件地址） |
| `external_contact` | `external_contact` | 外部联系人 |

## 处理规则

1. 从结果中筛选有 `email` 字段的条目。
2. 无论匹配数量多少，都必须列出候选项供用户确认后再使用；搜索是模糊匹配，单条结果不代表精确命中。
3. 展示尽可能多的字段帮助用户区分：`name`、`email`、`department`、`tag`、`display_name`、`type`、`member_count`。字段为空时省略。
4. 若无匹配，告知用户未找到，建议换关键词或直接提供邮箱地址。
5. 用户确认后，将 `email` 传入发信 shortcut 的 `--to` / `--cc` / `--bcc` 参数。

## 展示示例

```text
找到以下匹配"张三"的结果：
1. 张三 <zhangsan@example.com>
   类型：user | 部门：研发团队
```

```text
找到多个匹配"组"的结果，请选择：
1. 团队邮件组 <team@example.com>
   类型：enterprise_mail_group | 标签：mail_group
2. 项目群 <project@example.com>
   类型：chat | 成员数：50 | 标签：chat_group_normal
3. 张群 <zhangqun@example.com>
   类型：user | 部门：研发团队 | 备注名：张群同学
```

## 相关命令

- `lark-cli mail +send` — 新邮件收件人。
- `lark-cli mail +draft-create` — 新建草稿收件人。
- `lark-cli mail +draft-edit` — 编辑草稿收件人。
