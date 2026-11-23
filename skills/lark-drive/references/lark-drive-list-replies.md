# drive +list-replies

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和权限处理。

分页获取某条评论下的回复。

## 命令

```bash
# 推荐：完整 URL + 评论 ID
lark-cli drive +list-replies --url "https://example.larksuite.com/docx/<DOCX_TOKEN>" --comment-id '<id>'
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--url` | 与 `--token` 二选一 | 推荐入口。支持 doc/docx/sheet/file/slides/base/bitable/apps/wiki URL；apps 妙搭 URL 使用 `/page/<token>`；wiki URL 会自动解析到真实文档。 |
| `--token` | 与 `--url` 二选一 | 裸 token 或 URL。裸 token 必须搭配 `--type`；wiki token 使用 `--type wiki`。 |
| `--type` | 裸 token 时必填 | 传 token 对应类型：`doc`、`docx`、`sheet`、`file`、`slides`、`bitable`、`base`、`apps`、`wiki`。wiki token 使用 `wiki`；传 `base` 时，CLI 会按 `bitable` 类型处理。 |
| `--comment-id` | 是 | 评论 ID；来自 `drive +list-comments` 的 `items[].comment_id` |
| `--page-size` | 否 | 1-100，默认 50 |
| `--page-token` | 否 | 上次输出的 `page_token`；`has_more=true` 时用它续拉 |
| `--need-reaction` | 否 | 在回复上返回 reaction 数据，见 [`lark-drive-reactions.md`](lark-drive-reactions.md) |

## 行为说明

- 根回复承载评论正文本身，是回复列表中创建最早的一条：**仅第一页（未传 `--page-token`）的 `items[0]` 是根回复**；翻页后（传了 `--page-token`）返回的 `items[0]` 只是普通回复，不要按位置当作根回复去更新或删除。
- 输出字段：`items[].reply_id` / `user_id` / `create_time` / `update_time` / `content.elements`，供 `+update-reply`、`+delete-reply` 使用。
- 检查回复归属（更新/删除前）：比对 `items[].user_id`（open_id）与当前身份，判断是不是自己创建的回复。
- 输出的 `items` 始终是 JSON 数组（服务端省略时归一化为 `[]`）。

## 输出

```json
{
  "file_token": "docx_token",
  "file_type": "docx",
  "comment_id": "<comment_id>",
  "items": [],
  "has_more": false,
  "page_token": "",
  "count": 0
}
```

`items` 是回复数组；是否继续翻页以 `has_more` 为准，`has_more=true` 时用返回的 `page_token` 续拉。

## 参考

- [lark-drive-list-comments](lark-drive-list-comments.md) -- 评论卡片模型与统计口径
- [lark-drive-update-reply](lark-drive-update-reply.md) -- 更新回复
- [lark-drive-delete-reply](lark-drive-delete-reply.md) -- 删除回复
- [lark-drive-reactions](lark-drive-reactions.md) -- reaction 查询与写入
