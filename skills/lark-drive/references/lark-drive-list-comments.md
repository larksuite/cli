# drive +list-comments

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和权限处理。

列出 doc/docx/sheet/file/slides/base(bitable) 的评论卡片。优先传用户给出的完整 URL，shortcut 会自动识别类型；如果传 wiki URL 或 `--token <wiki_token> --type wiki`，会先解析到真实文档。

## 重要默认口径

- 默认只查未解决评论，即不额外传 `--solved-status` 或显式传 `--solved-status false`。即使用户说“所有评论”“全部评论”“把评论都列出来”，只要没有明确提到包含已解决评论，仍然按默认口径查询未解决评论。
- 仅当用户明确要求“包含已解决评论”“已解决和未解决都要”“全部历史评论”这类语义时，才传 `--solved-status all`。
- 是否还有下一页以输出里的 `has_more` 为准；`page_token` 只作为 `has_more=true` 时续跑下一页的游标。

## 命令

```bash
# 推荐：直接传用户给出的完整 URL。默认只查未解决评论。
lark-cli drive +list-comments \
  --url "<DOCUMENT_URL>"

# 只有用户明确要求包含已解决评论时，才查询已解决和未解决的全部评论。
lark-cli drive +list-comments \
  --url "<DOCUMENT_URL>" \
  --solved-status all

# 查询已解决评论。
lark-cli drive +list-comments \
  --url "<DOCUMENT_URL>" \
  --solved-status true

# 只查全文评论或局部评论。
lark-cli drive +list-comments \
  --url "<DOCUMENT_URL>" \
  --comment-scope whole

lark-cli drive +list-comments \
  --url "<DOCUMENT_URL>" \
  --comment-scope partial

# 电子表格 URL 保留 /sheets/ 路径，直接原样传入；不要把 sheet token 拼成 /docx/<token>。
lark-cli drive +list-comments \
  --url "https://example.larksuite.com/sheets/<SHEET_TOKEN>"

# wiki URL 会自动解包。
lark-cli drive +list-comments \
  --url "https://example.larksuite.com/wiki/<WIKI_TOKEN>"

# 裸 wiki token 也支持，但必须显式声明 --type wiki。
lark-cli drive +list-comments \
  --token "<WIKI_TOKEN>" \
  --type wiki

# 裸 token 需要声明 token 对应类型；不要默认当作 docx。这里以 sheet 为例。
lark-cli drive +list-comments \
  --token "<DOCUMENT_TOKEN>" \
  --type sheet \
  --page-size 100

# docx 需要评论定位关系时再带 need-relation；非 docx 会静默忽略。
lark-cli drive +list-comments \
  --url "https://example.larksuite.com/docx/<DOCX_TOKEN>" \
  --need-relation

# 分页续跑。
# 先看上一页输出的 has_more；只有 has_more=true 时，才用返回的 page_token 继续。
lark-cli drive +list-comments \
  --url "<DOCUMENT_URL>" \
  --page-size 100 \
  --page-token "<NEXT_PAGE_TOKEN>"

# 预览请求链路，不发真实请求。
lark-cli drive +list-comments \
  --url "https://example.larksuite.com/wiki/<WIKI_TOKEN>" \
  --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--url` | 与 `--token` 二选一 | 推荐入口。支持 doc/docx/sheet/file/slides/base/bitable/wiki URL；wiki URL 会自动解析到真实文档。 |
| `--token` | 与 `--url` 二选一 | 裸 token 或 URL。裸 token 必须搭配 `--type`；wiki token 使用 `--type wiki`。 |
| `--type` | 裸 token 时必填 | 传 token 对应类型：`doc`、`docx`、`sheet`、`file`、`slides`、`bitable`、`base`、`wiki`。wiki token 使用 `wiki`；传 `base` 时，CLI 会按 `bitable` 类型处理。 |
| `--solved-status` | 否 | `false` / `true` / `all`，默认 `false`。`false` 查未解决评论；`true` 查已解决评论；`all` 查全部评论。 |
| `--comment-scope` | 否 | `all` / `whole` / `partial`，默认 `all`。`all` 查全部范围；`whole` 查全文评论；`partial` 查局部评论。 |
| `--need-reaction` | 否 | 是否返回评论卡片上的 reaction 数据；只有用户明确需要 reaction 时才带。 |
| `--need-relation` | 否 | docx 评论定位关系字段；仅 docx 生效，非 docx 静默忽略。需要定位正文时先读 [`lark-drive-comment-location.md`](lark-drive-comment-location.md)。 |
| `--page-size` | 否 | 默认 50，最大 100。 |
| `--page-token` | 否 | 分页游标；本 shortcut 不自动翻页，按返回的 `page_token` 继续请求下一页。 |

## 行为说明

- `--comment-scope all` 查全部范围；`whole` 查全文评论；`partial` 查局部/选区评论。
- 当用户已经给出完整 URL 时，原样传给 `--url`；不要先提取 token 再重组成其他类型 URL。比如 sheet 保留 `/sheets/<token>`，wiki 保留 `/wiki/<token>`。
- URL 输入时不需要传 `--type`；如果 URL 类型和显式 `--type` 冲突，shortcut 会返回 validation error，建议移除 `--type`。
- wiki 输入会自动解析到真实文档，再查询评论列表。JSON 输出不额外返回 wiki token 或 wiki node。
- 输出中的 `items` 保留评论卡片字段，外层补充 `file_token`、`file_type`、`has_more`、`page_token`、`count`；`count` 是当前页返回的评论卡片数。是否继续分页以 `has_more` 为准，而不是只看 `page_token` 是否存在。
- 如果需要批量按评论 ID 查询、获取更多回复、创建/编辑/删除回复，继续使用原生 `drive file.comments batch_query` 或 `drive file.comment.replys.*`。

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
- [lark-drive-comments-guide](lark-drive-comments-guide.md) -- 评论统计、回复限制和原生 API 说明
- [lark-drive-comment-location](lark-drive-comment-location.md) -- 使用 `need_relation` 定位 docx 正文
