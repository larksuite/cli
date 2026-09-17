# docs +update（更新飞书云文档）

使用文本或 block 指令精确更新飞书云文档。默认使用 XML；仅在用户明确要求或必须保真 Markdown 时使用 Markdown。

写入前必须按 `--doc-format` 读取对应格式参考：`xml` 读取 [`lark-doc-xml.md`](lark-doc-xml.md)，`markdown` 读取 [`lark-doc-md.md`](lark-doc-md.md)；

## 常用示例

```bash
# 先定位内容并获取最新 block ID
lark-cli docs +fetch --doc "文档URL或token" --scope keyword --keyword "key1|key2" --detail with-ids

# 替换文本；--content "" 可删除文本
lark-cli docs +update --doc "xx" --command str_replace --pattern "旧内容" --content "新内容"

# 替换单个 block，或同父连续范围内的 block
lark-cli docs +update --doc "xx" --command block_replace --block-id blkTarget --content '<p>新段落</p>'
lark-cli docs +update --doc "xx" --command block_replace --start-block-id blkFirst --end-block-id blkLast --content '<p></p>'

lark-cli docs +update --doc "xx" --command block_insert_after --block-id blkAnchor --content '<h2>新章节</h2><p>章节内容</p>'

# 删除单个 block 或范围内的 block
lark-cli docs +update --doc "xx" --command block_delete --block-id blkA
lark-cli docs +update --doc "xx" --command block_delete --start-block-id blkFirst --end-block-id blkLast
```

## 推荐流程

1. **Observe（读取现状）**：先 `docs +fetch` 读取当前文档状态，并按意图选择最小范围。
   - 改某一节或大文档：先 `--scope outline --max-depth 2` 找章节，再 `--scope section --start-block-id <标题id> --detail with-ids`
   - 精确跨节区间：用 `--scope range --start-block-id xxx --end-block-id yyy`
   - 只有模糊关键词：用 `--scope keyword --keyword "key1|key2" --context-before 1 --context-after 1 --detail with-ids`
   - 明确整篇重构才读 `--detail with-ids` 全文；只读摘要或确认事实时用更轻的 fetch
2. **Diagnose（诊断问题）**：判断用户目标、当前结构、语气、重复、断流、事实口径和需要保留的资源；识别哪些 block 必须原样保留。
3. **Patch Plan（制定局部计划）**：把修改拆成最小安全操作：简单行内文本替换用 `str_replace`，但它不支持资源替换；单个 block 用一个 `--block-id`，同一直接父节点下的连续 block 用 `--start-block-id`/`--end-block-id`。连续范围适用于 `block_replace` 和 `block_delete`。整段/整块重写用 `block_replace`；增补章节用 `block_insert_after`；删冗余用 `block_delete`；调整顺序用 `block_move_after`。
4. **Patch（精确修改）**：按 block / section 执行局部命令。替换内容必须符合目标父容器的结构；例如替换列表项范围时使用 `<li>...</li>`。保护 `<cite>`、`<img>`、`<source>`、`<whiteboard>`、`<sheet>`、`<bitable>`、`<synced_reference>` 等 token 化内容，不要改成纯文本或占位符。同一 block 的多处修改合并成一次 `block_replace`。
5. **Verify（fetch 验证）**：每轮写操作后按影响范围重新 fetch，检查用户要求、结构、语气、事实、资源块和 block ID 是否符合预期；不满足就基于最新 fetch 结果继续 Diagnose / Patch，不要沿用上一轮 block ID。

除非用户明确要求完全重建，或原文已无保留价值，否则不要使用 `overwrite`；它可能丢失评论和暂不支持的资源。

## 生成 block 直达链接

用户需要某个 block 的直达链接时，只定位 block，不执行文档写操作：

1. 使用局部 `docs +fetch --detail with-ids` 获取目标 `block_id`。
2. 返回 `文档基础 URL#block_id`；没有 `block_id` 时不得猜测。

## 参数

|参数|必填|说明|
|-|-|-|
|`--doc`|是|文档 URL 或 token|
|`--command`|是|更新指令，见下表|
|`--doc-format`|否|`xml`（默认）或 `markdown`|
|`--content`|视指令|写入内容；`str_replace` 传空字符串可删除文本|
|`--pattern`|视指令|`str_replace` 的简单行内匹配文本；不要用于多行、整段或多个 block|
|`--block-id`|视指令|目标 block ID；`-1` 表示文档末尾，`0` 表示文档开头（仅适用于支持这些锚点的指令）|
|`--start-block-id` / `--end-block-id`|视指令|`block_replace` / `block_delete` 的同父连续闭区间，必须成对使用，且不能与 `--block-id` 混用；`--start-block-id` 用 `0` 表示从文档开头开始，`--end-block-id` 用 `-1` 表示到文档末尾结束|
|`--src-block-ids`|视指令|要复制或移动的源 block ID，多个 ID 用逗号分隔|
|`--reference-map`|否|保留或回放既有 `reference_map`，需与 `--content` 配合；支持 JSON、任务目录内的相对 `@file` 或 stdin `-`|
|`--revision-id`|否|基准版本号，默认 `-1`（最新版本）|

## 指令速查

|指令|用途与限制|必需参数|
|-|-|-|
|`str_replace`|全文查找替换；支持富文本内的文本替换，但不支持资源替换；涉及多个 block 时建议用 `block_replace`；空 `--content` 表示删除|`--pattern`、`--content`|
|`block_insert_after`|在指定 block 后插入内容；逐章填充时指定对应标题的 block ID|`--block-id`、`--content`|
|`block_copy_insert_after`|按 ID 顺序复制源 block，源 block 不变；基础标签均支持，资源块仅支持 `img`、`source`、`whiteboard`、`sheet`、`chat_card`、`sub-page-list`，不支持 `task`、`bitable`、`base_ref`、`synced_reference`、`synced_source`、`okr`|`--block-id`、`--src-block-ids`|
|`block_replace`|替换单个 block（`--block-id`）或同父连续闭区间（`--start-block-id`/`--end-block-id`）；不支持跨容器或反向区间|`--content`，以及 `--block-id` 或 `--start-block-id`+`--end-block-id`|
|`block_delete`|删除单个 block（`--block-id`）或同父连续闭区间（`--start-block-id`/`--end-block-id`）；不支持跨容器或反向区间|`--block-id` 或 `--start-block-id`+`--end-block-id`|
|`block_move_after`|移动已有 block，支持所有块类型；|`--block-id`、`--src-block-ids`|
|`append`|仅在文末追加，等价于 `block_insert_after --block-id -1`|`--content`|
|`overwrite`|清空后重写全文，丢失图片、评论等内容，非必要不使用|`--content`|

## 通用安全规则

- 每次写操作后都按 block ID 已变化处理。新插入或复制的内容一定使用新 ID；替换、删除和覆盖会使旧 ID 失效；移动会改变章节与 range 语义。
- 同一 block 有多处修改时，应合并为一次 `block_replace`，避免连续使用旧 ID。

## 返回值

```json
{
  "ok": true,
  "identity": "user",
  "data": {
    "document": {
      "revision_id": 2,
      "new_blocks": [
        { "block_id": "blkcnXXXX", "block_type": "whiteboard", "block_token": "boardXXXX" }
      ]
    },
    "result": "success",
    "updated_blocks_count": 1,
    "warnings": [],
    "tips": ""
  }
}
```

|字段|说明|
|-|-|
|`result`|`success` \| `partial_success` \| `failed`|
|`updated_blocks_count`|实际更新的 block 数量|
|`warnings`|服务端返回的警告列表；即使 `result=success` 也要检查是否存在降级或未完全处理的内容|
|`tips`|服务端返回的后续处理建议；为空表示没有额外建议，非空本身不表示更新失败|
|`document.new_blocks`|新增 block；`block_id` 用于后续编辑，资源块的 `block_token` 可交给对应 skill 继续处理|

## 需要查文档

可查看 [`+fetch`](lark-doc-fetch.md)。
