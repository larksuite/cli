
# docs +media-preview（预览文档素材）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

优先用于查看、预览文档中的图片或文件素材（`file_token`），也支持评论 sidecar 中 `<img src="..."/>` 暴露的图片 token。命令先请求素材 source preview；若该接口不接受评论图片 token，会自动回退到素材下载接口，并把结果保存到本地路径。

## 选择规则

- 用户说“看一下素材 / 图片 / 附件”“预览一下”时，优先使用 `docs +media-preview`
- token 来自 `reference_map.comments` 的 `<img src="TOKEN"/>` 时，直接使用 `docs +media-preview`；无需先做文件导出权限检查
- 用户明确说“下载”时，使用 [`docs +media-download`](lark-doc-media-download.md)
- 如果目标明确是画板 / whiteboard / 画板缩略图，不要使用 `+media-preview`，改用 `docs +media-download --type whiteboard`

## 命令

```bash
# 预览图片/文件素材
lark-cli docs +media-preview --token "Z1Fjxxxxxxxx" --output ./asset

# 指定输出文件名（带扩展名则不会自动补全）
lark-cli docs +media-preview --token "Z1Fjxxxxxxxx" --output ./asset.png
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--token <token>` | 是 | 文档素材 `file_token`，或评论 sidecar 的图片 `src` token |
| `--output <path>` | 是 | 本地保存路径；不带扩展名会自动补全 |

## token 从哪里来

- 若你是从文档内容里提取：`lark-doc-fetch` 返回的内容里可能包含：
  - 图片：`<img token="..." .../>`
  - 文件：`<source token="..." name="..."/>`
- 若你是从评论 sidecar 提取：读取 `data.document.reference_map.comments.*.data` 中的 `<img src="..."/>`

## 参考

- [lark-doc-fetch](lark-doc-fetch.md) — 获取文档内容（用于提取 token）
- [lark-doc-media-download](lark-doc-media-download.md) — 明确下载素材，或下载画板缩略图
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
