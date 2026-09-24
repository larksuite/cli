# base +form-share-qrcode-get / +form-share-qrcode-download

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

表单分享二维码是分享链接的图片形态，扫码即打开填写页。两条命令**发的是同一个 GET 请求**（无请求体、无 query 参数），区别只在输出：`+form-share-qrcode-get` 把结果原样返回，`+form-share-qrcode-download` 校验后把 PNG 落到本地文件。

## 何时使用

- 用户要"表单二维码""扫码填表""把二维码发出去/贴到文档里"时。
- 只要链接不要图片时用 `+form-share-get`，它返回分享 URL；不要为了拿链接去下载二维码。
- 开关分享、改访问范围走 `+form-share-update`；本组命令只读二维码，**不会**顺手打开分享。

## 前置：分享必须已开启

分享未开启时接口返回 `invalid_request`。此时正确做法是提示用户先开分享，而不是自动去开：

```bash
# 先确认分享状态
lark-cli base +form-share-get \
  --base-token <base_token> --table-id <table_id> --form-id <form_id>

# 确认后由用户决定是否开启（开启是独立的写操作）
lark-cli base +form-share-update \
  --base-token <base_token> --table-id <table_id> --form-id <form_id> --enabled=true
```

## 命令

```bash
# 取二维码结构（含 base64 图片内容）
lark-cli base +form-share-qrcode-get \
  --base-token <base_token> --table-id <table_id> --form-id <form_id>

# 落盘：用接口返回的 file_name 写到当前目录
lark-cli base +form-share-qrcode-download \
  --base-token <base_token> --table-id <table_id> --form-id <form_id>

# 落盘到指定路径
lark-cli base +form-share-qrcode-download \
  --base-token <base_token> --table-id <table_id> --form-id <form_id> \
  --output /tmp/form-qrcode.png

# 用户明确要求刷新同一个文件时，显式允许覆盖
lark-cli base +form-share-qrcode-download \
  --base-token <base_token> --table-id <table_id> --form-id <form_id> \
  --output /tmp/form-qrcode.png --overwrite
```

## 参数

| 参数 | 命令 | 必填 | 说明 |
|---|---|---|---|
| `--base-token` | 两条 | 是 | 表单所属 Base |
| `--table-id` | 两条 | 是 | 表单所属 Table |
| `--form-id` | 两条 | 是 | 表单 ID |
| `--output` | download | 否 | 输出文件路径或目录。不传时用返回的 `file_name` 写到当前目录；给目录时把 `file_name` 拼进该目录 |
| `--overwrite` | download | 否 | 允许覆盖落点上的已有文件，默认 `false`（已有同名文件直接报错、不写入） |

`--output` 受 CLI 内置的落盘白名单约束：只能写在当前工作目录、`/tmp` 或 `~/files` 之内。越界在本地校验阶段就被拒绝，`--dry-run` 同样会报错。

## 返回结构

`+form-share-qrcode-get` 的 `data`：

```json
{
  "share_url": "<validated-url>",
  "file_name": "form-vewForm-qrcode.png",
  "mime_type": "image/png",
  "content_base64": "iVBORw0KGgo..."
}
```

`+form-share-qrcode-download` 不回传 `content_base64`，只报最终落点：

```json
{
  "saved_path": "/tmp/form-qrcode.png",
  "file_name": "form-qrcode.png",
  "mime_type": "image/png",
  "size_bytes": 4213,
  "share_url": "<validated-url>"
}
```

拿到 `saved_path` 后要把图片实际展示或附加给用户，只说"已保存"不算交付完成。

## 落盘前的三道校验

`+form-share-qrcode-download` 在写任何字节之前会依次校验，任何一道不过都直接报错、不产生文件：

1. `mime_type` 必须是 `image/png`。
2. `content_base64` 必须能 Base64 解码。
3. 解码结果必须以 PNG 文件头 `\x89PNG\r\n\x1a\n` 开头。

这三道校验是为了避免把一个错误响应体写成后缀为 `.png` 的垃圾文件。

## 错误与重试

| 情况 | 表现 | 处理 |
|---|---|---|
| 分享未开启 | `invalid_request` | 提示用户先 `+form-share-update --enabled=true`，命令不会自动开启 |
| 响应不是合法 PNG | `invalid_response`，未写文件 | 属于服务端响应问题，重试同一请求；连续失败要上报，不要手工拼 base64 落盘 |
| `--output` 越出白名单 | 校验失败，未发请求、未写文件 | 改到当前目录、`/tmp` 或 `~/files` 之内 |
| `--output` 写成尚不存在的目录（如 `/tmp/qr`） | 被当成**文件路径**，落出一个没有 `.png` 后缀的文件 | 目录要么先建好，要么在结尾带上分隔符（`/tmp/qr/`）；否则只认已存在的目录 |
| 落点已有同名文件 | `output file <path> already exists`，未写文件 | 默认不覆盖：换一个 `--output` 重跑；用户明确要替换旧文件时才加 `--overwrite`。不带 `--output` 时文件名来自服务端返回，重复落盘必然撞上 |
| 运行环境的 file provider 不支持独占提交 | `cannot commit files exclusively`，未发请求 | 这是宿主能力问题，不是路径冲突——换 `--output` 同样失败；用户同意覆盖时可加 `--overwrite`，否则需要换运行环境 |
| 本地写文件失败（权限、磁盘满） | 报错，提示换个可写位置 | 换一个可写的 `--output` 重跑即可；请求是只读 GET，重发没有副作用 |

## 参考

- [lark-base](../SKILL.md) — 多维表格全部命令
- [lark-base-form-share.md](lark-base-form-share.md) — 二维码的前置：分享开关与访问范围
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
