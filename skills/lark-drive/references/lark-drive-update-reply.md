# drive +update-reply

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和权限处理；`--content` 完整格式见 [`lark-drive-comment-content.md`](lark-drive-comment-content.md)。

整体替换某条回复的内容。

## 命令

```bash
# 推荐：完整 URL + 评论 ID + 回复 ID + 新内容（整体替换，无局部编辑）
lark-cli drive +update-reply --url "https://example.larksuite.com/docx/<DOCX_TOKEN>" --comment-id '<id>' --reply-id '<id>' --content '[{"type":"text","text":"新内容"}]'
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--url` | 与 `--token` 二选一 | 推荐入口。支持 doc/docx/sheet/file/slides/base/bitable/apps/wiki URL；apps 妙搭 URL 使用 `/page/<token>`；wiki URL 会自动解析到真实文档。 |
| `--token` | 与 `--url` 二选一 | 裸 token 或 URL。裸 token 必须搭配 `--type`；wiki token 使用 `--type wiki`。 |
| `--type` | 裸 token 时必填 | 传 token 对应类型：`doc`、`docx`、`sheet`、`file`、`slides`、`bitable`、`base`、`apps`、`wiki`。wiki token 使用 `wiki`；传 `base` 时，CLI 会按 `bitable` 类型处理。 |
| `--comment-id` | 是 | 回复所属的评论 ID；来自 `drive +list-comments` |
| `--reply-id` | 是 | 要更新的回复 ID；来自 `drive +list-replies` 的 `items[].reply_id` |
| `--content` | 是 | 新的 `reply_elements` JSON，`type=text` 文本自动转义；完整 schema 见 [`lark-drive-comment-content.md`](lark-drive-comment-content.md) |

## 行为说明

- 更新是整体替换：新 `content` 完全覆盖旧内容，没有局部修改语义。
- **只能更新当前身份自己创建的回复**；更新他人回复返回 API 错误 `1069303 forbidden`。执行前先用 `+list-replies` 核对 `items[].user_id`（open_id），并用创建该回复的同一个 `--as` 身份执行。
- 更新评论卡片的根回复（第一页 `items[0]`，即创建最早的一条 reply）等价于改写这条评论的正文本身；改写前先和用户确认改的是回复还是评论正文。

## 输出

```json
{
  "file_token": "docx_token",
  "file_type": "docx",
  "comment_id": "<comment_id>",
  "reply_id": "<reply_id>",
  "updated": true
}
```

## 参考

- [lark-drive-comment-content](lark-drive-comment-content.md) -- `--content` 格式
- [lark-drive-list-replies](lark-drive-list-replies.md) -- 获取回复与 reply_id
