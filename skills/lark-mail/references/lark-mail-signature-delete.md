# mail +signature-delete

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

删除个人 USER 邮件签名。命令先 GET 当前签名确认目标存在且不是 TENANT，再 DELETE，并尽量回读确认该 ID 已消失。删除会由服务端清理该签名的默认应用关系。

## 命令

```bash
lark-cli mail +signature-delete --as user --signature-id 123 --yes

lark-cli mail +signature-delete --as user --signature-id 123 --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--signature-id <id>` | 是 | 十进制 USER 签名 ID |
| `--yes` | 是 | 确认删除；无人值守场景必须显式传入 |
| `--mailbox <email>` | 否 | 所属邮箱，默认 `me` |
| `--dry-run` | 否 | 展示 GET + DELETE + 回读计划 |

## 返回值

```json
{
  "deleted": true,
  "deleted_signature_id": "123",
  "verify_status": "absent"
}
```

`verify_status=unknown` 表示 DELETE 已成功，但回读确认失败或无法确认该 ID 已消失。

## 限制

- 只允许删除 USER 签名；TENANT 企业签名会被拒绝。
- 未传 `--yes` 时不会执行删除。
