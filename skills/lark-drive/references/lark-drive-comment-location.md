# 文档评论定位字段

当用户需要根据评论定位文档正文位置、对文档做 review、区分多处相同引用文本，或把评论落点映射到 `docs +fetch --detail with-ids` 的内容时，优先使用 `drive +list-comments --need-relation` 查询 docx 评论位置。

## 适用范围

- 当前只有 `file_type=docx` 支持通过 `need_relation=true` 查询评论的位置，并返回可用于定位正文 block 的 `relation`、`parent_type`、`parent_token` 等字段。
- `drive +list-comments` 会在目标不是 docx 时静默忽略 `--need-relation`，避免把无效参数传给 OpenAPI。遇到 sheet、bitable、slides、普通文件等类型的评论时，不要承诺可以用 `need_relation` 精确定位正文位置，应退回普通评论字段、对应资源能力下钻或人工确认。

## 调用方式

分页列出评论时，优先传 URL；Wiki URL / Wiki token 会自动解析到底层真实 token/type：

```bash
lark-cli drive +list-comments --url '<docx_or_wiki_url>' --need-relation
```

如果只有 Wiki token，显式传 `--type wiki`：

```bash
lark-cli drive +list-comments --token '<wiki_token>' --type wiki --need-relation
```

只有在需要未被 shortcut 暴露的底层参数时，才直接调用 raw OpenAPI。此时把 `need_relation` 放在 query params：

```bash
lark-cli drive file.comments list \
  --params '{"file_token":"<doc_token>","file_type":"docx","is_solved":false,"need_relation":true}'
```

已知评论 ID 批量查询时，把 `need_relation` 放在请求体里：

```bash
lark-cli drive file.comments batch_query \
  --params '{"file_token":"<doc_token>","file_type":"docx"}' \
  --data '{"comment_ids":["<comment_id>"],"need_relation":true}'
```

同时获取文档内容，并要求返回 block id：

```bash
lark-cli docs +fetch --doc '<doc_token_or_url>' --detail with-ids
```

## 字段含义

- `relation`：评论在文档内容中的结构化位置。`relation.relation` 是一个 JSON 字符串，需要再解析一次；其中 `positionInfo.blockID` 是最关键字段，用于匹配 `docs +fetch --detail with-ids` 返回的文档 block。
- `relation.content_deleted`：评论引用的内容是否已被删除。为 `true` 时，不要假设还能在当前正文中找到原位置。
- `parent_type`：评论所在的父级嵌入资源类型。常见值包括 `SHEET_BLOCK`、`BITABLE_BLOCK`、`WHITEBOARD_BLOCK`，表示评论落在文档内嵌电子表格、多维表格或画板内部。
- `parent_token`：父级嵌入资源 token。对 sheet / bitable / whiteboard 内部评论，服务端可能无法给出内部单元格、记录或画板节点的文档 block 级 `relation`，但可以通过 `parent_type` + `parent_token` 定位到文档里的父级嵌入 block。

## 准确度分级

输出定位结论时，必须区分以下三类，不要把弱推断说成精确定位：

| 等级 | 判定条件 | 输出口径 |
|---|---|---|
| `relation 精确` | `relation.relation` 中有 `positionInfo.blockID`，且能在 `docs +fetch --detail with-ids` 中匹配到同一 block | 可说“准确定位到 block” |
| `父级资源精确，内部需下钻` | 只有父级嵌入资源的 `blockID` / `parent_type` / `parent_token`，或内部资源的 `positionInfo` 为空 | 可说“准确定位到嵌入资源；内部单元格/记录/节点需用对应 skill 下钻确认” |
| `弱匹配/推断` | 只能依赖 `quote`、序号、当前展示顺序或文本搜索 | 必须标明“推断”，说明歧义来源和需要的补充信息 |

## 返回示例

普通 docx block 上的评论会返回 `relation`。注意 `relation.relation` 本身是字符串，需要再 JSON parse 一次：

```json
{
  "comment_id": "7646774324967295982",
  "quote": "code2",
  "relation": {
    "content_deleted": false,
    "relation": "{\"22-doc_token_xxx\":{\"objType\":22,\"index\":2,\"objVersion\":10,\"positionInfo\":{\"blockID\":\"block_id_xxx\"}}}"
  },
  "parent_type": null,
  "parent_token": null
}
```

把 `relation.relation` 再解析后，取 `positionInfo.blockID`：

```json
{
  "22-doc_token_xxx": {
    "objType": 22,
    "index": 2,
    "objVersion": 10,
    "positionInfo": {
      "blockID": "block_id_xxx"
    }
  }
}
```

然后在 `docs +fetch --detail with-ids` 的结果里查找同一个 block id，例如：

```json
{
  "block_id": "block_id_xxx",
  "block_type": "code",
  "text": "code1\ncode2"
}
```

嵌入 sheet / bitable / whiteboard 内部评论可能没有可用 `relation`，但会返回父级标记：

```json
{
  "comment_id": "7646775036988148672",
  "quote": "记录 2",
  "relation": null,
  "parent_type": "BITABLE_BLOCK",
  "parent_token": "bitable_app_token_xxx_table_id_xxx"
}
```

这种情况下，用 `parent_type` 判断目标是嵌入资源，再用 `parent_token` 匹配 `docs +fetch --detail with-ids` 中的 bitable / sheet block。定位粒度是文档里的父级嵌入 block，不是内部记录、字段或单元格。

画板内部评论的返回形态类似：

```json
{
  "comment_id": "7646775036988148673",
  "quote": "画板节点文本",
  "relation": null,
  "parent_type": "WHITEBOARD_BLOCK",
  "parent_token": "whiteboard_token_xxx"
}
```

此时 `parent_token` 对应 `docs +fetch --detail with-ids` 结果中 `<whiteboard>` 的 `token` 属性，例如：

```xml
<whiteboard id="whiteboard_block_id_xxx" token="whiteboard_token_xxx"></whiteboard>
```

匹配到这个 `<whiteboard>` 后，`id` 就是文档正文里的父级画板 block id。定位粒度是文档里的画板 block；如果需要继续定位到画板内部具体节点，需要再用画板能力读取画板内部结构。

## 定位流程

1. 确认目标是 `file_type=docx`；只有 docx 文档支持通过 `need_relation` 查询评论位置。
2. 用 `drive +list-comments --need-relation` 获取评论；已知评论 ID 且需要批量查询时，可用 `drive file.comments batch_query` 并带 `need_relation=true`。raw `drive file.comments list` 仅作为低层参数兜底。
3. 用 `docs +fetch --detail with-ids` 获取文档内容。
4. 对每条评论先看 `relation`：
   - 如果存在 `relation.relation`，解析这个 JSON 字符串。
   - 从解析结果里取 `positionInfo.blockID`。
   - 在 `docs +fetch` 结果中查找相同 block id，这就是评论对应的文档 block。
5. 如果没有可用 `relation`，但有 `parent_type` 和 `parent_token`：
   - `SHEET_BLOCK`：定位到文档中的 sheet 嵌入 block；`parent_token` 通常包含 sheet token 和 sheet id，必要时取 `_` 前的 token 与文档 block 的嵌入资源 token 对比。
   - `BITABLE_BLOCK`：定位到文档中的 bitable 嵌入 block；`parent_token` 通常包含 bitable app token 和 table id，必要时取 `_` 前的 token 与文档 block 的嵌入资源 token 对比。
   - `WHITEBOARD_BLOCK`：定位到文档中的 whiteboard 嵌入 block；`parent_token` 对应 `docs +fetch --detail with-ids` 中 `<whiteboard>` 的 `token` 属性。
   - 这种场景能定位到父级嵌入 block，但通常不能仅凭评论接口定位到嵌入资源内部的具体单元格、字段、记录或画板节点。
6. 只有在 `relation`、`parent_type`、`parent_token` 都缺失时，才退回使用 `quote` 文本做弱匹配；`quote` 是评论接口返回的引用文本字段。弱匹配不能区分多处相同文本。

## 嵌入资源内部定位

### Sheet 内部评论

- `parent_token` 常见格式是 `<spreadsheet_token>_<sheet_id>`；也可能在 `relation.relation` 中看到 `subToken` 为 `3-<spreadsheet_token>`。
- 评论接口通常只把 `positionInfo.blockID` 指到文档里的 `<sheet>` block，内部 sheet 的 `positionInfo` 可能为空。
- 如果 `quote` 是 `C3`、`A1` 这类单元格坐标，可拆出 `spreadsheet_token` / `sheet_id` 后用 `lark-sheets` 读取该单元格确认：

```bash
lark-cli sheets +read \
  --spreadsheet-token '<spreadsheet_token>' \
  --sheet-id '<sheet_id>' \
  --range '<cell>'
```

- 准确度口径：父级 sheet block 可由 relation/parent token 精确定位；单元格坐标若只来自 `quote`，应说明“单元格来自 quote，已通过 sheets 读取验证”，不要说它来自 `positionInfo`。

### Bitable / Base 内部评论

- `parent_token` 常见格式是 `<base_token>_<table_id>`，其中 `table_id` 通常以 `tbl` 开头。解析时优先按最后一个 `_tbl` 边界拆分，避免 base token 内出现 `_` 时误拆。
- 评论接口可能只返回 `parent_type=BITABLE_BLOCK` 和 `parent_token`，没有 `relation`；即使有 relation，也通常只足够定位到文档里的 `<bitable>` block。
- 下钻读取时切到 `lark-base`，最少确认表、字段、记录：

```bash
lark-cli base +table-list --base-token '<base_token>'
lark-cli base +field-list --base-token '<base_token>' --table-id '<table_id>'
lark-cli base +record-list --base-token '<base_token>' --table-id '<table_id>' --limit 200 --format json
```

- 如果 `quote` 是某个稳定业务值，优先用字段/记录数据做精确匹配；如果 `quote` 只是“第 N 条”“第 N 行”这类 UI 序号，只能基于当前记录顺序推断对应记录，必须输出为“推断”，并说明评论接口没有返回 `record_id` / `field_id`。
- 如果 `record-list` 返回 `has_more=true`，不要基于第一页下全局结论；继续分页或说明只能覆盖已读取范围。
- 需要写入时，如果评论没有字段信息，不要自行猜字段；除非用户给出默认规则，否则请求用户确认字段，或明确说明将使用哪个字段作为默认。

### Whiteboard 内部评论

- `parent_token` 对应文档 XML 中 `<whiteboard token="...">`；先用它匹配文档里的 whiteboard block。
- 若要定位画板内部节点，切到 `lark-whiteboard` 读取 raw 节点结构：

```bash
lark-cli whiteboard +export \
  --whiteboard-token '<whiteboard_token>' \
  --output-type raw
```

- 如果 raw 节点中存在唯一匹配 `quote` 的文本节点，可定位到该节点；如果有多个相同文本节点，仍然是弱匹配，需要结合位置、样式、用户描述或人工确认。
- 修改画板节点前，先说明匹配到的节点 id 和文本；复杂画板不要只凭 `quote` 批量替换全部同名节点。

## 使用原则

- Review 文档时，不要只依赖 `quote` 文本定位评论；多处相同文本会产生歧义。
- 能拿到 `relation.positionInfo.blockID` 时，以 block id 为准，再用 block 内容理解上下文。
- 对嵌入 sheet / bitable / whiteboard 内的评论，以父级嵌入 block 作为文档正文定位点；如需继续定位到表格单元格、多维表格记录或画板内部节点，需要再调用对应 sheet / bitable / whiteboard 能力读取内部数据。
