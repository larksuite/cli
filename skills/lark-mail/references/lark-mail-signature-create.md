# mail +signature-create

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

创建个人（USER）邮箱签名。正文支持 HTML；本地 `<img src="./local.png">` 会自动上传到 Drive，并改写为 `cid:` 引用后写入签名 `images[]`。

## 命令

```bash
lark-cli mail +signature-create --as user \
  --name '工作签名' \
  --content '<p>Regards</p><img src="./logo.png">'

lark-cli mail +signature-create --as user \
  --name '移动端签名' \
  --content-file './signature.html' \
  --device MOBILE

lark-cli mail +signature-create --as bot \
  --mailbox alice@example.com \
  --name '团队签名' \
  --content '<p>Team</p>'
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--name <text>` | 是 | 签名名称 |
| `--content <html>` | 否* | 签名正文；支持本地 `<img src>` 自动上传并改写 |
| `--content-file <path>` | 否* | 从文件加载正文；与 `--content` 互斥 |
| `--device <PC\|MOBILE>` | 否 | 签名设备类型，默认 `PC` |
| `--mailbox <email>` | 否 | 所属邮箱，默认 `me`。`--as bot` 时必须传显式邮箱 |
| `--dry-run` | 否 | 仅打印计划中的 Drive 上传和 POST 请求 |

\* 两者都留空会创建空正文签名；两者不能同时提供。

## 返回值

成功返回：

```json
{
  "signature": {
    "id": "712345",
    "name": "工作签名",
    "signature_type": "USER",
    "signature_device": "PC",
    "content": "<p>Regards</p>",
    "images": []
  }
}
```

## 错误提示

- 重名（`15180303`）：换一个 `--name`，或改用 `+signature-update` 更新已有签名。
- 找不到签名 ID（`15180302`，通常出现在更新/删除）：先运行 `lark-cli mail +signature` 获取真实 ID。
- 权限不足（`15180305`）：检查 `mail:user_mailbox.message:modify` scope、邮箱权限，以及 bot 身份是否传了显式 `--mailbox`。

## 相关

- 查看签名：[`+signature`](./lark-mail-signature.md)
- 更新签名：[`+signature-update`](./lark-mail-signature-update.md)
- 删除签名：[`+signature-delete`](./lark-mail-signature-delete.md)
