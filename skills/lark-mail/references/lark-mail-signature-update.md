# mail +signature-update

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

编辑个人 USER 邮件签名。命令先读取当前签名列表，定位 `--signature-id`，合并 `--patch-file` 和 flat flags 后 PUT 全量签名对象。该接口没有乐观锁，语义是 last-write-wins。

## 命令

```bash
# 查看当前签名，不写入
lark-cli mail +signature-update --as user --signature-id 123 --inspect

# 直接替换名称和内容
lark-cli mail +signature-update --as user \
  --signature-id 123 \
  --set-name '新的签名' \
  --set-content '<p>Regards,<br>Alice</p>'

# 打印 patch 模板
lark-cli mail +signature-update --print-patch-template

# 使用 patch 文件
lark-cli mail +signature-update --as user --signature-id 123 --patch-file ./signature-patch.json
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--signature-id <id>` | 条件必填 | 十进制签名 ID；除 `--print-patch-template` 外必填，`--inspect` 也必须传 |
| `--inspect` | 否 | 只读取当前签名投影，不发 PUT |
| `--print-patch-template` | 否 | 输出 patch JSON 骨架，不访问网络 |
| `--patch-file <path>` | 否 | 相对路径 JSON patch 文件 |
| `--set-name <text>` | 否 | 替换名称，trim 后非空且 ≤100 字符 |
| `--set-content <html-or-text>` | 否 | 替换内容；与 `--set-content-file` 互斥 |
| `--set-content-file <path>` | 否 | 从相对路径读取替换内容 |
| `--set-device pc|mobile` | 否 | 替换设备 |
| `--set-images-json <json>` | 否 | 替换图片元数据；传 `[]` 可清空 |
| `--mailbox <email>` | 否 | 所属邮箱，默认 `me` |
| `--dry-run` | 否 | 展示 GET + PUT 计划 |

## Patch 文件

```json
{
  "id": "123",
  "name": "新的签名",
  "content": "<p>Regards</p>",
  "signature_device": "PC",
  "images": [{"image_name": "logo.png", "file_key": "file_xxx", "cid": "logo1"}]
}
```

`id` 省略时由 CLI 用 path `--signature-id` 补齐；若与 path 不一致会报错。

## 限制

- 只允许更新 USER 签名；TENANT 企业签名会被拒绝。
- 新内容中的 `cid:` 引用必须与最终 images 数组完全一致。
