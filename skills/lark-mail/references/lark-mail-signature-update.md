# mail +signature-update

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

全量替换个人（USER）邮箱签名。此命令不是 patch：未提供的字段会按空值写入并清空原值，执行时会在 stderr 输出 warning。

## 命令

```bash
lark-cli mail +signature-update --as user \
  --signature-id 712345 \
  --name '新版工作签名' \
  --content '<p>Best regards</p><img src="./logo.png">'

lark-cli mail +signature-update --as user \
  --signature-id 712345 \
  --name '移动端签名' \
  --content-file './signature.html' \
  --device MOBILE
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--signature-id <id>` | 是 | 要更新的签名 ID。先运行 `+signature` 查看 |
| `--name <text>` | 是 | 替换后的签名名称 |
| `--content <html>` | 否* | 替换后的签名正文；支持本地 `<img src>` 自动上传并改写 |
| `--content-file <path>` | 否* | 从文件加载正文；与 `--content` 互斥 |
| `--device <PC\|MOBILE>` | 否 | 签名设备类型，默认 `PC` |
| `--mailbox <email>` | 否 | 所属邮箱，默认 `me`。`--as bot` 时必须传显式邮箱 |
| `--dry-run` | 否 | 仅打印计划中的 Drive 上传和 PUT 请求 |

\* 两者都留空会把签名正文清空；两者不能同时提供。

## 返回值

成功返回：

```json
{
  "signature": {
    "id": "712345",
    "name": "新版工作签名",
    "signature_type": "USER",
    "signature_device": "PC",
    "content": "<p>Best regards</p>",
    "images": []
  }
}
```

## 注意

- 这是全量替换接口。更新前如果不确定当前内容，先运行 `+signature --detail <id>` 查看。
- 本地图片处理与 `+signature-create` 相同：上传 Drive、生成 CID、改写正文、填充 `images[]`。

## 相关

- 查看签名：[`+signature`](./lark-mail-signature.md)
- 创建签名：[`+signature-create`](./lark-mail-signature-create.md)
- 删除签名：[`+signature-delete`](./lark-mail-signature-delete.md)
