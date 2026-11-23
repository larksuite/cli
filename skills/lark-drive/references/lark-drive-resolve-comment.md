# drive +resolve-comment

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和权限处理。

把一条评论标记为已解决。反向操作——重新打开已解决评论——是独立命令 [`lark-drive-restore-comment.md`](lark-drive-restore-comment.md)。

用户说“把这条评论标记为已处理 / 已完成 / 关闭”对应本命令。

## 命令

```bash
# 推荐：完整 URL + 评论 ID
lark-cli drive +resolve-comment --url "https://example.larksuite.com/docx/<DOCX_TOKEN>" --comment-id '<id>'
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--url` | 与 `--token` 二选一 | 推荐入口。支持 doc/docx/sheet/file/slides/base/bitable/apps/wiki URL；apps 妙搭 URL 使用 `/page/<token>`；wiki URL 会自动解析到真实文档。 |
| `--token` | 与 `--url` 二选一 | 裸 token 或 URL。裸 token 必须搭配 `--type`；wiki token 使用 `--type wiki`。 |
| `--type` | 裸 token 时必填 | 传 token 对应类型：`doc`、`docx`、`sheet`、`file`、`slides`、`bitable`、`base`、`apps`、`wiki`。wiki token 使用 `wiki`；传 `base` 时，CLI 会按 `bitable` 类型处理。 |
| `--comment-id` | 是 | 要解决的评论 ID；来自 `drive +list-comments` 的 `items[].comment_id` |

## 行为说明

- 这是写操作。
- 对同一条评论连续翻转解决状态可能触发服务端限流（HTTP 429）；连续调用之间留间隔或短暂延迟后重试。

## 输出

```json
{
  "file_token": "docx_token",
  "file_type": "docx",
  "comment_id": "<comment_id>",
  "action": "resolve",
  "is_solved": true,
  "updated": true
}
```

## 参考

- [lark-drive-restore-comment](lark-drive-restore-comment.md) -- 恢复（重新打开）评论
