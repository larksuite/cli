# mail +signature-delete

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

删除个人（USER）邮箱签名。

## 命令

```bash
lark-cli mail +signature-delete --as user --signature-id 712345

lark-cli mail +signature-delete --as bot \
  --mailbox alice@example.com \
  --signature-id 712345

lark-cli mail +signature-delete --as user --signature-id 712345 --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--signature-id <id>` | 是 | 要删除的签名 ID。先运行 `+signature` 查看 |
| `--mailbox <email>` | 否 | 所属邮箱，默认 `me`。`--as bot` 时必须传显式邮箱 |
| `--dry-run` | 否 | 仅打印计划中的 DELETE 请求 |

## 返回值

成功返回：

```json
{
  "deleted": true,
  "mailbox_id": "me",
  "signature_id": "712345"
}
```

## 相关

- 查看签名：[`+signature`](./lark-mail-signature.md)
- 创建签名：[`+signature-create`](./lark-mail-signature-create.md)
- 更新签名：[`+signature-update`](./lark-mail-signature-update.md)
