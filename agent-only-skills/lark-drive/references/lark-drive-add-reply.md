# drive +add-reply

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和权限处理；`--content` 完整格式见 [`lark-drive-comment-content.md`](lark-drive-comment-content.md)。

给已有评论添加一条回复。

## 命令

```bash
# 推荐：完整 URL + 目标评论 ID + 回复内容
lark-cli drive +add-reply --url "https://example.larksuite.com/docx/<DOCX_TOKEN>" --comment-id '<id>' --content '[{"type":"text","text":"回复内容"}]'
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--url` | 与 `--token` 二选一 | 推荐入口。支持 doc/docx/sheet/file/slides/base/bitable/apps/wiki URL；apps 妙搭 URL 使用 `/page/<token>`；wiki URL 会自动解析到真实文档。 |
| `--token` | 与 `--url` 二选一 | 裸 token 或 URL。裸 token 必须搭配 `--type`；wiki token 使用 `--type wiki`。 |
| `--type` | 裸 token 时必填 | 传 token 对应类型：`doc`、`docx`、`sheet`、`file`、`slides`、`bitable`、`base`、`apps`、`wiki`。wiki token 使用 `wiki`；传 `base` 时，CLI 会按 `bitable` 类型处理。 |
| `--comment-id` | 是 | 要回复的评论 ID；来自 `drive +list-comments` 的 `items[].comment_id` |
| `--content` | 是 | `reply_elements` JSON，`type=text` 文本自动转义；完整 schema、mention_user/link、10000 字符限制见 [`lark-drive-comment-content.md`](lark-drive-comment-content.md) |

## 回复限制

- `is_whole=true` 的全文评论、`is_solved=true` 的已解决评论都不能回复。
- 目标的 `is_whole` / `is_solved` 通常在上一步 `+list-comments` / `+batch-query-comments` 的结果里已有，据此判断即可；信息不足时再补查一次。
- 补查时注意 `+list-comments` 默认只返回未解决评论：要核对某条评论是否已被解决，需要带 `--solved-status all`，否则已解决评论根本不出现在结果里，看起来像评论不存在。
- 命中限制时如实提示（“全文评论不支持回复” / “该评论已被解决，无法回复”），不要自动替用户改回复到别的评论。

## 输出

```json
{
  "file_token": "docx_token",
  "file_type": "docx",
  "comment_id": "<comment_id>",
  "created": true,
  "reply_id": "<reply_id>"
}
```

## 参考

- [lark-drive-comment-content](lark-drive-comment-content.md) -- `--content` 格式
- [lark-drive-batch-query-comments](lark-drive-batch-query-comments.md) -- 按 ID 查 is_whole/is_solved
- [lark-drive-list-replies](lark-drive-list-replies.md) -- 获取回复
