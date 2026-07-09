# Drive 评论查询、统计与回复指南

> 前置条件：先阅读 [`../SKILL.md`](../SKILL.md) 的“评论能力入口”，添加评论参数细节见 [`lark-drive-add-comment.md`](lark-drive-add-comment.md)，reaction 见 [`lark-drive-reactions.md`](lark-drive-reactions.md)。

## 评论模式

- `drive +add-comment` 支持全文评论和局部评论。
- 全文评论：未传 `--block-id` 时默认启用，也可显式传 `--full-comment`；支持 `docx`、旧版 `doc` URL、白名单扩展名的 Drive file，以及最终解析为 `doc` / `docx` / `file` 的 wiki URL。
- 局部评论：传 `--block-id` 时启用；`docx` 支持文本定位或 block id，`sheet` 支持 `<sheetId>!<cell>`，`slides` 支持 `<slide-block-type>!<xml-id>`，wiki URL 解析到这些类型时也支持对应局部评论。
- Drive file 只支持全文评论，不支持局部评论。支持扩展名：`.md`、`.txt`、`.json`、`.csv`、`.go`、`.js`、`.py`、`.pptx`、`.png`、`.jpg`、`.jpeg`、`.zip`、`.mp3`、`.mp4`。`.pdf`、`.docx`、`.xlsx` 等未在白名单内的普通文件暂不支持。
- Review / 审阅 / 校对 / 逐条指出问题场景优先使用局部评论，不要把多个可定位问题汇总成一条全文评论。
- `drive +add-comment` 的 `--content` 需要传 `reply_elements` JSON 数组字符串，例如 `--content '[{"type":"text","text":"正文"}]'`。
- `slides` 评论要求显式传 `--block-id <slide-block-type>!<xml-id>`；CLI 会将其拆分后写入 `anchor.block_id` 和 `anchor.slide_block_type`。其中 `<xml-id>` 是 PPT XML 协议中的元素 `id`；不支持 `--selection-with-ellipsis` 和 `--full-comment`。
- 评论写入内容里的文本不能直接出现 `<`、`>`；提交前应转义为 `&lt;`、`&gt;`。`drive +add-comment` 会对 `type=text` 文本元素自动兜底转义；直接调用原生评论 API 时需要自行转义。
- 如果 wiki 解析后不是 `doc` / `docx` / `file` / `sheet` / `slides`，不要用 `+add-comment`。

## 查询默认口径

`drive file.comments list` 默认必须传 `is_solved:false`，即仅查询未解决评论。即使用户说“所有评论”“全部评论”“把评论都列出来”，只要没有明确提到要包含已解决评论，仍然按默认口径查询未解决评论。仅当用户明确要求包含已解决评论时，才可省略 `is_solved` 参数。

```bash
# 默认查询：仅未解决评论
lark-cli drive file.comments list --params '{"file_token":"xxx","file_type":"docx","is_solved":false}'

# 包含已解决评论：仅当用户明确要求时使用
lark-cli drive file.comments list --params '{"file_token":"xxx","file_type":"docx"}'
```

## 评论卡片与统计

- `drive file.comments list` 返回的 `items` 是评论卡片列表，每个 `item` 对应用户界面中的一张评论卡片，不是平铺的互动消息列表。
- 创建第一条评论时会同时创建该卡片里的第一条 reply；真正承载正文的是 `item.reply_list.replies`，其中第一条 reply 在用户视角下就是这张卡片里的“评论本身”。
- 统计“评论数”或“评论卡片数”：统计 `items` 长度；全量统计时对所有分页返回的 `items` 长度累加。
- 统计“回复数”：统计所有 `item.reply_list.replies` 长度之和，再减去 `items` 长度。
- 统计“总互动数”：统计所有 `item.reply_list.replies` 长度之和，包含每张评论卡片里的首条评论。
- 如果 `item.has_more=true`，说明该评论卡片下还有更多回复未包含在当前返回中；需要继续调用 `drive file.comment.replys list` 拉全后，再做全量回复数或总互动数统计。

## 排序

- 只有当用户明确提到“最新评论”“最后评论”“最早评论”时，才需要按 `create_time` 排序。
- 排序前必须拉完所有评论分页，不能只取第一页。
- “最新评论”/“最后评论”：按 `create_time` 降序取第一条。
- “最早评论”：按 `create_time` 升序取第一条。
- 用户只说“第一条评论”时，直接使用 `drive file.comments list` 返回的第一条，不需要额外排序。

## 回复限制

- 回复前先检查目标评论状态。
- `is_whole=true` 的全文评论不支持回复；遇到时提示“全文评论不支持回复”。
- `is_solved=true` 的已解决评论不支持回复；遇到时提示“该评论已被解决，无法回复”。
- 当目标评论不能回复时，只提示限制，不要自动替用户寻找其他可回复评论。

## batch_query 与 list

- `drive file.comments batch_query` 用于已知评论 ID 后的批量查询，需要传入具体评论 ID 列表。
- `drive file.comments list` 用于分页获取评论列表，适合统计评论总数、遍历所有评论、获取最新或最后 N 条评论等场景。

## 评论定位字段

- 需要根据评论定位到文档正文位置时（例如根据评论 review 文档、区分多处相同引用文本、把评论落点映射到 `docs +fetch` 的 block），先确认目标是 `file_type=docx`，再阅读 [`lark-drive-comment-location.md`](lark-drive-comment-location.md)。
- 其他文档类型暂不支持返回定位字段。

## 原生 API

需要更底层地直接调用评论 V2 协议时，先查看 schema，再调用原生命令。全文评论省略 `anchor`，局部评论传 `anchor.block_id`。

```bash
lark-cli schema drive.file.comments.create_v2
lark-cli drive file.comments create_v2 \
  --params '{"file_token":"<DOC_TOKEN>"}' \
  --data '{"file_type":"docx","reply_elements":[{"type":"text","text":"全文评论内容"}]}'
```
