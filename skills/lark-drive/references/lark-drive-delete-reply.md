# drive +delete-reply

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和权限处理。

删除某条回复。**高风险写操作**：真实执行需要按 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 的高风险审批协议向用户确认后追加 `--yes`；删除不可恢复。

## 命令

```bash
# 先预览（--dry-run 不需要 --yes）
lark-cli drive +delete-reply --url "https://example.larksuite.com/docx/<DOCX_TOKEN>" --comment-id '<id>' --reply-id '<id>' --dry-run

# 确认后真实删除（把 --dry-run 换成 --yes）
lark-cli drive +delete-reply --url "https://example.larksuite.com/docx/<DOCX_TOKEN>" --comment-id '<id>' --reply-id '<id>' --yes
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--url` | 与 `--token` 二选一 | 推荐入口。支持 doc/docx/sheet/file/slides/base/bitable/apps/wiki URL；apps 妙搭 URL 使用 `/page/<token>`；wiki URL 会自动解析到真实文档。 |
| `--token` | 与 `--url` 二选一 | 裸 token 或 URL。裸 token 必须搭配 `--type`；wiki token 使用 `--type wiki`。 |
| `--type` | 裸 token 时必填 | 传 token 对应类型：`doc`、`docx`、`sheet`、`file`、`slides`、`bitable`、`base`、`apps`、`wiki`。wiki token 使用 `wiki`；传 `base` 时，CLI 会按 `bitable` 类型处理。 |
| `--comment-id` | 是 | 回复所属的评论 ID；来自 `drive +list-comments` |
| `--reply-id` | 是 | 要删除的回复 ID；来自 `drive +list-replies` 的 `items[].reply_id`，或 `drive +list-comments` 的 `items[].reply_list.replies[].reply_id` |
| `--yes` | 真实执行时是 | 高风险确认；`--dry-run` 预览不需要 |

## 行为说明

- 删除永久生效，回复没有回收站或撤销。
- 删除按 reply 逐条生效：删除某条回复（包括第一条/根回复）不影响其它回复；把该评论卡片下的所有回复都删完后，评论卡片在前端页面才不再显示。
- **删除整条评论没有专门的命令，需要用本命令删光该卡片下的所有回复**（先用 `drive +list-replies` 拉全回复 id）。删除前先和用户确认删的是某条回复还是整条评论。

## 输出

```json
{
  "file_token": "docx_token",
  "file_type": "docx",
  "comment_id": "<comment_id>",
  "reply_id": "<reply_id>",
  "deleted": true
}
```

## 参考

- [lark-drive-list-replies](lark-drive-list-replies.md) -- 获取回复与 reply_id
