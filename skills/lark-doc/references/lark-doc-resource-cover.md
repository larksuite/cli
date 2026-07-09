# docs +resource-*（Docx 封面图资源）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

Docx 封面图不是正文里的 `<img token="...">` 素材块。读取、更新、删除文档封面图时，使用 `docs +resource-download/+resource-update/+resource-delete --type cover`，不要使用 `+media-insert` 或 `+media-download --token <cover.token>` 让用户手动拼步骤。

## 选择规则

- 用户要下载文档封面图：`docs +resource-download --type cover`
- 用户要设置/替换文档封面图：`docs +resource-update --type cover`
- 用户要删除文档封面图：`docs +resource-delete --type cover`
- 用户要下载正文图片、附件、画板缩略图：继续使用 [`docs +media-download`](lark-doc-media-download.md)

## 命令

```bash
# 下载封面图。CLI 会先读取 document.cover.token，再下载图片内容并保存到本地。
lark-cli docs +resource-download --doc doxcnXXX --type cover --output ./cover

# 使用本地文件更新封面图。
lark-cli docs +resource-update --doc doxcnXXX --type cover --file ./cover.png

# 使用剪切板图片更新封面图。
lark-cli docs +resource-update --doc doxcnXXX --type cover --from-clipboard

# 使用 HTTPS URL 更新封面图。CLI 会先下载 URL 内容，再上传并写入 cover.token。
lark-cli docs +resource-update --doc doxcnXXX --type cover --url "https://example.com/cover.png"

# 可选：设置封面图裁切偏移。
lark-cli docs +resource-update --doc doxcnXXX --type cover --file ./cover.png --offset-ratio-x 0.2 --offset-ratio-y 0.8

# 删除封面图；当文档本来没有封面图时也成功返回。
lark-cli docs +resource-delete --doc doxcnXXX --type cover
```

## 参数

| 命令 | 参数 | 必填 | 说明 |
|------|------|------|------|
| all | `--doc <id>` | 是 | 文档 ID、docx URL，或可解析为 docx 的 wiki URL |
| all | `--type cover` | 否 | 当前只支持 `cover`；默认值也是 `cover` |
| download | `--output <path>` | 是 | 本地保存路径；不带扩展名会根据响应类型自动补全 |
| download | `--overwrite` | 否 | 覆盖已存在的输出文件 |
| update | `--file <path>` | 三选一 | 磁盘上的真实图片文件；大于 20MiB 自动使用分片上传 |
| update | `--from-clipboard` | 三选一 | 从系统剪切板读取图片 |
| update | `--url <https-url>` | 三选一 | 从 HTTPS URL 下载图片后上传 |
| update | `--offset-ratio-x <number>` | 否 | 视图相对原图中心的横向偏移比例：水平偏移 px / 原图宽度 px；0 为居中，正数向右，负数向左 |
| update | `--offset-ratio-y <number>` | 否 | 视图相对原图中心的纵向偏移比例：垂直偏移 px / 原图高度 px；0 为居中，正数向上，负数向下 |

## 输出契约

- `+resource-download` 成功时 stdout JSON 的 `data` 包含 `document_id`、`type`、`saved_path`、`size_bytes`、`content_type`、`cover.token`。如果文档没有封面图，命令失败退出，错误包含 `document has no cover` 和脱敏 `document_id`，不会创建输出文件。
- `+resource-update` 成功时 stdout JSON 的 `data` 包含完整 `file_token` 和 `cover.token`；stderr 只打印脱敏 token。
- `+resource-delete` 成功时 stdout JSON 的 `data.deleted` 表示本次是否真的发起删除，`data.already_empty` 表示删除前是否没有封面图。空封面图是幂等成功，不报错。

## URL 来源安全边界

`+resource-update --url` 只用于下载公开 HTTPS 图片：

- 只允许 `https://`，拒绝 HTTP、空 host 和 URL userinfo。
- 拒绝解析到 private、loopback、link-local、multicast、unspecified 地址的 host。
- 最多跟随 3 次跳转，每次跳转都重新校验 URL。
- 响应 `Content-Type` 只允许 `image/png`、`image/jpeg`、`image/gif`、`image/webp`。
- 响应体最大 20MiB。

## 参考

- [lark-doc-media-download](lark-doc-media-download.md) — 下载正文素材或画板缩略图
- [lark-doc-media-insert](lark-doc-media-insert.md) — 在正文插入图片/文件
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
