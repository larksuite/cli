# mail +signature-create

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

创建个人 USER 邮件签名。HTML 内容原样保存；纯文本会转换成可在邮件客户端渲染换行的 HTML fragment。签名图片只接受已有 `file_key/cid` 元数据，不会上传本地图片。

## 命令

```bash
lark-cli mail +signature-create --as user \
  --name '工作签名' \
  --content '<p>Regards,<br>Alice</p>'

lark-cli mail +signature-create --as user \
  --name '带 Logo 签名' \
  --content '<p>Alice</p><img src="cid:logo1">' \
  --images-json '[{"image_name":"logo.png","file_key":"file_xxx","cid":"logo1"}]'

lark-cli mail +signature-create --as user \
  --name '移动端签名' \
  --content-file './signature.html' \
  --device mobile
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--name <text>` | 是 | 签名名称，trim 后非空且 ≤100 字符 |
| `--content <html-or-text>` | 否 | 签名内容；与 `--content-file` 互斥 |
| `--content-file <path>` | 否 | 从相对路径读取签名内容 |
| `--device pc|mobile` | 否 | 签名设备，默认 `pc` |
| `--images-json <json>` | 否 | 图片元数据数组，`cid` 必须匹配内容里的 `cid:` 引用 |
| `--mailbox <email>` | 否 | 所属邮箱，默认 `me` |
| `--dry-run` | 否 | 只展示将调用的 POST 请求 |

## 返回值

成功返回 `signature` 对象，并额外给出 `id`、`name`、`signature_device`、`content_preview`，后续可把 `id` 传给 `+send / +reply / +forward --signature-id`。

## 限制

- 只创建个人 USER 签名，不支持企业 TENANT 签名。
- `<img src="./logo.png">` 这类本地路径会报错；先准备后端可校验的 `file_key/cid` 再传 `--images-json`。
