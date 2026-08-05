# drive +list-comments

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和权限处理。

列出 doc/docx/sheet/file/slides/base(bitable)/apps 的评论卡片。优先传用户给出的完整 URL，shortcut 会自动识别类型；apps 为妙搭类型，支持 `/page/<token>` URL；如果传 wiki URL 或 `--token <wiki_token> --type wiki`，会先解析到真实文档。

## 重要默认口径

- 默认只查未解决评论，即不额外传 `--solved-status` 或显式传 `--solved-status false`。即使用户说“所有评论”“全部评论”“把评论都列出来”，只要没有明确提到包含已解决评论，仍然按默认口径查询未解决评论。
- 仅当用户明确要求“包含已解决评论”“已解决和未解决都要”“全部历史评论”这类语义时，才传 `--solved-status all`。
- 是否还有下一页以输出里的 `has_more` 为准；`page_token` 只作为 `has_more=true` 时续跑下一页的游标。

## 命令

```bash
# 推荐：直接传用户给出的完整 URL。默认只查未解决评论。
lark-cli drive +list-comments --url "<DOCUMENT_URL>"

# 只有用户明确要求包含已解决评论时，才传 --solved-status all。
lark-cli drive +list-comments --url "<DOCUMENT_URL>" --solved-status all
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 与 `--token` 二选一 | 推荐入口。支持 doc/docx/sheet/file/slides/base/bitable/apps/wiki URL；apps 妙搭 URL 使用 `/page/<token>`；wiki URL 会自动解析到真实文档。 |
| `--token` | 与 `--url` 二选一 | 裸 token 或 URL。裸 token 必须搭配 `--type`；wiki token 使用 `--type wiki`。 |
| `--type` | 裸 token 时必填 | 传 token 对应类型：`doc`、`docx`、`sheet`、`file`、`slides`、`bitable`、`base`、`apps`、`wiki`。wiki token 使用 `wiki`；传 `base` 时，CLI 会按 `bitable` 类型处理。 |
| `--solved-status` | 否 | `false` / `true` / `all`，默认 `false`。`false` 查未解决评论；`true` 查已解决评论；`all` 查全部评论。 |
| `--comment-scope` | 否 | `all` / `whole` / `partial`，默认 `all`。`all` 查全部范围；`whole` 查全文评论；`partial` 查局部评论。 |
| `--need-reaction` | 否 | 是否返回评论卡片上的 reaction 数据；只有用户明确需要 reaction 时才带。 |
| `--need-relation` | 否 | docx 评论定位关系字段；仅 docx 生效，非 docx 静默忽略。需要定位正文时先读 [`lark-drive-comment-location.md`](lark-drive-comment-location.md)。 |
| `--page-size` | 否 | 默认 50，最大 100。 |
| `--page-token` | 否 | 分页游标；本 shortcut 不自动翻页，按返回的 `page_token` 继续请求下一页。 |

## 行为说明

- `--comment-scope all` 查全部范围；`whole` 查全文评论；`partial` 查局部/选区评论。
- 当用户已经给出完整 URL 时，原样传给 `--url`；不要先提取 token 再重组成其他类型 URL。比如 sheet 保留 `/sheets/<token>`，wiki 保留 `/wiki/<token>`，妙搭 apps 保留 `/page/<token>`。
- URL 输入时不需要传 `--type`；如果 URL 类型和显式 `--type` 冲突，shortcut 会返回 validation error，建议移除 `--type`。
- wiki 输入会自动解析到真实文档，再查询评论列表。JSON 输出不额外返回 wiki token 或 wiki node。
- 输出中的 `items` 保留评论卡片字段，外层补充 `file_token`、`file_type`、`has_more`、`page_token`、`count`；`count` 是当前页返回的评论卡片数。是否继续分页以 `has_more` 为准，而不是只看 `page_token` 是否存在。

## 评论卡片模型

- 返回的 `items` 是评论卡片列表，每个 `item` 对应用户界面中的一张评论卡片，不是平铺的互动消息列表。
- 创建评论时会同时创建该卡片里的第一条 reply；真正承载正文的是 `item.reply_list.replies`，其中第一条 reply（根回复）在用户视角下就是这张卡片里的“评论本身”。更新根回复即改写评论正文（见 [`lark-drive-update-reply.md`](lark-drive-update-reply.md)）；删除按 reply 逐条生效，卡片在最后一条回复被删时才消失（见 [`lark-drive-delete-reply.md`](lark-drive-delete-reply.md)）。
- `item.has_more=true` 表示该评论卡片下还有回复未包含在本次返回中；这与外层 `has_more`（是否还有下一页评论卡片）是两个不同字段。需要完整回复时继续用 `drive +list-replies --comment-id <id>` 分页拉全。

## 统计口径

- 统计“评论数”或“评论卡片数”：统计 `items` 长度；全量统计时对所有分页返回的 `items` 长度累加。
- 统计“回复数”：统计所有 `item.reply_list.replies` 长度之和，再减去 `items` 长度。
- 统计“总互动数”：统计所有 `item.reply_list.replies` 长度之和，包含每张评论卡片里的首条评论。
- 任一 `item.has_more=true` 时，先用 `drive +list-replies --comment-id <id>` 把该卡片的回复拉全，再做回复数或总互动数统计，否则会少算。

## 排序

- 只有当用户明确提到“最新评论”“最后评论”“最早评论”时，才需要按 `create_time` 排序。
- 排序前必须拉完所有评论分页，不能只取第一页。
- “最新评论”/“最后评论”：按 `create_time` 降序取第一条。“最早评论”：按 `create_time` 升序取第一条。
- 用户只说“第一条评论”时，直接使用返回的第一条，不需要额外排序。

## 输出

```json
{
  "file_token": "docx_token",
  "file_type": "docx",
  "items": [],
  "has_more": false,
  "page_token": "",
  "count": 0
}
```

## 参考

- [lark-drive](../SKILL.md) -- 云空间（云盘/云存储）全部命令
- [lark-drive-list-replies](lark-drive-list-replies.md) -- 拉全某张卡片下的回复（统计与 `item.has_more` 补全）
- [lark-drive-comment-location](lark-drive-comment-location.md) -- 使用 `need_relation` 定位 docx 正文
