# slides +replace-slide（块级替换 / 插入）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

对指定 slide 做块级替换或插入。编辑已有 PPT 的主路径——`slide_id` 不变、页序不动、只影响被指定的块。

相比直接调 `xml_presentation.slide.replace`，这个 shortcut 的四个额外价值：

1. `--presentation` 接受 `xml_presentation_id` / `/slides/` URL / `/wiki/` URL（wiki 自动解析）；
2. `block_replace` 的 `replacement` 根元素 `id="<block_id>"` 由 CLI 自动注入——底层 API 的硬约束（不注入返回 3350001）；直接调原生 API 需自己加，用 Shortcut 则自动注入；
3. `<shape>` 元素缺少 `<content/>` 子元素时由 CLI 自动注入——SML 2.0 schema 要求每个 `<shape>` 必须有 `<content/>` 子元素，缺失同样触发 3350001；自闭合的 `<shape .../>` 也会被自动展开为 `<shape ...><content/></shape>`；
4. 3350001 错误时提供上下文感知的 hint，帮助 AI agent 和用户快速定位原因。

## 命令

```bash
# block_insert：在页末追加一个新元素
lark-cli slides +replace-slide --as user \
  --presentation slidesXXXXXXXXXXXXXXXXXXXXXX \
  --slide-id pfG \
  --parts '[{"action":"block_insert","insertion":"<shape type=\"rect\" topLeftX=\"500\" topLeftY=\"100\" width=\"200\" height=\"100\"/>"}]'

# block_replace：已知某块 id，整块替换（replacement 根 id 自动注入为 bUn）
lark-cli slides +replace-slide --as user \
  --presentation slidesXXXXXXXXXXXXXXXXXXXXXX \
  --slide-id pfG \
  --parts '[{"action":"block_replace","block_id":"bUn","replacement":"<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>新标题</p></content></shape>"}]'

# 大 --parts 走文件或 stdin（auto-gen 命令不支持 @file，但 shortcut 支持）
lark-cli slides +replace-slide --as user \
  --presentation $PID --slide-id $SID --parts @parts.json
cat parts.json | lark-cli slides +replace-slide --as user \
  --presentation $PID --slide-id $SID --parts -

# wiki URL 直接传（CLI 自动 get_node → 拿真实 xml_presentation_id）
lark-cli slides +replace-slide --as user \
  --presentation "https://xxx.feishu.cn/wiki/wikcnXXXXXX" --slide-id pfG \
  --parts '[{"action":"block_insert","insertion":"<shape type=\"rect\" width=\"100\" height=\"100\"/>"}]'

# 预览（不实际调用）
lark-cli slides +replace-slide --as user \
  --presentation $PID --slide-id $SID --parts "$PARTS" --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--presentation` | 是 | `xml_presentation_id`、`/slides/<token>` URL，或 `/wiki/<token>` URL |
| `--slide-id` | 是 | 页面 ID（`xml_presentation.slide.get` / `xml_presentations.get` 都能拿到） |
| `--parts` | 是 | JSON 数组（`[{...}, ...]`），单次最多 200 条。支持 `@<file>` 和 `-`（stdin）读取 |
| `--revision-id` | 否 | 基础版本号；默认 `-1` 表示基于最新版执行；传具体版本号时，服务端以该版本为 base 执行；**传不存在的版本号（超过当前 revision）返回 3350002** |
| `--tid` | 否 | 并发事务 ID；多人协作长事务才用，单次单人调用留空 |

## parts 元素结构

> **限制**：最多 200 条；`block_replace` 和 `block_insert` 可以在同一批次混用。**其他 action（含 `str_replace`）CLI 会直接报错拒绝**。

每条 part 按 `action` 取不同字段：

### action = `block_replace`

| 字段 | 必填 | 说明 |
|------|------|------|
| `action` | 是 | `"block_replace"` |
| `block_id` | 是 | 目标块的 3 位 short element ID（从 `slide.get` 返回 XML 里读） |
| `replacement` | 是 | 新 XML 片段；**根元素 `id` 会被 CLI 自动注入为 `block_id`**，用户不用自己加（如果已经加了且不一致会被覆盖为正确值） |

### action = `block_insert`

| 字段 | 必填 | 说明 |
|------|------|------|
| `action` | 是 | `"block_insert"` |
| `insertion` | 是 | 要插入的 XML 片段 |
| `insert_before_block_id` | 否 | 插到这个块之前；省略（不提供此字段）则追加到页末 |

## 合法根元素速查

`block_replace.replacement` 和 `block_insert.insertion` 必须以 SML 2.0 定义的合法元素为根。完整权威定义看 [`slides_xml_schema_definition.xml`](slides_xml_schema_definition.xml)；这里只列能作为**根**的类型 + 每种类型的最小可工作片段。

| 元素 | 用途 | 关键点 |
|---|---|---|
| `<shape>` | 矩形/椭圆/三角/文本框等所有形状 | `type` 必填；`<content/>` 缺失时 CLI 会自动注入 |
| `<line>` | 直线 | 需 `startX/startY/endX/endY` |
| `<polyline>` | 折线 | `points` 读回时被服务端规整丢弃（几何已入库） |
| `<img>` | 图片 | `src` 必须是 [`+media-upload`](lark-slides-media-upload.md) 返回的 `file_token`，不能是 URL |
| `<icon>` | 图标 | `iconType` 取自 iconpark 资源 |
| `<table>` | 表格 | 整表替换会**重建内部 td id**，旧 td block_id 立即失效 |
| `<td>` | 单元格局部替换 | 只能 `block_replace`，不能 `block_insert`；`block_id` 必须是最新 `slide.get` 拿到的 td id |
| `<chart>` | 图表（line/bar/column/pie/area/radar/combo） | 必须嵌 `<chartPlotArea>` + `<chartData>` + `<dim1>/<dim2>/<chartField>` |

**不可作为根元素**：

- `<video>` / `<audio>` / `<whiteboard>` —— SML 2.0 没有这三个原生元素；`<undefined type="video|audio|whiteboard">` 是**导出时**的占位符（服务端遇到不支持的类型时用它代替），**不能写入**。尝试 insert/replace 都会返回 3350001。

### 最小 XML 片段（JSON 嵌入时记得把 `"` 转义成 `\"`）

`<shape>`（文本框；`type` 还可选 `rect`/`ellipse`/`triangle`/`custom` 等）：
```xml
<shape type="text" topLeftX="80" topLeftY="80" width="800" height="120">
  <content textType="title"><p>标题</p></content>
</shape>
```

`<img>`：
```xml
<img src="{file_token}" topLeftX="600" topLeftY="20" width="80" height="80"/>
```

`<polyline>`：
```xml
<polyline topLeftX="10" topLeftY="10" width="100" height="50" points="0,0 50,50 100,0"/>
```

`<table>`（2×2）：
```xml
<table topLeftX="30" topLeftY="80">
  <colgroup><col span="2" width="110"/></colgroup>
  <tr><td><content><p>A</p></content></td><td><content><p>B</p></content></td></tr>
  <tr><td><content><p>C</p></content></td><td><content><p>D</p></content></td></tr>
</table>
```

`<td>`（`block_replace` 单元格；`block_id` 必须是最新 `slide.get` 拿到的 td id）：
```xml
<td><content><p>新内容</p></content></td>
```

`<chart>`（`type` 改成 `bar`/`column`/`pie`/`area`/`radar`/`combo` 切换图型）：
```xml
<chart topLeftX="30" topLeftY="300" width="300" height="200">
  <chartPlotArea><chartPlot type="line"/></chartPlotArea>
  <chartData>
    <dim1><chartField name="x" valueType="string">Q1,Q2,Q3,Q4</chartField></dim1>
    <dim2><chartField name="Sales" valueType="number">10,20,15,30</chartField></dim2>
  </chartData>
</chart>
```

## 返回值

```json
{
  "xml_presentation_id": "slidesXXXXXXXXXXXXXXXXXXXXXX",
  "slide_id": "pfG",
  "parts_count": 1,
  "revision_id": 102
}
```

| 字段 | 说明 |
|------|------|
| `xml_presentation_id` | 解析后的真实 token（wiki URL 解析后会变化） |
| `slide_id` | 与入参一致 |
| `parts_count` | 本次提交的 parts 条数 |
| `revision_id` | 成功后的新版本号，下次做乐观锁时用 |
| `failed_part_index` | 有部分失败时存在，指向第几条 part 失败 |
| `failed_reason` | 失败原因文字描述 |

整批作为原子事务：任一 part 失败则整批不生效，服务端通过 `failed_part_index` / `failed_reason` 告诉你是哪条；按此定位修正后重发。

## 使用流程

### 给已有页加图（典型场景）

```bash
PID=xxx
SID=yyy

# 1) 上传图片
TOKEN=$(lark-cli slides +media-upload --as user \
  --file ./pic.png --presentation "$PID" | jq -r '.data.file_token')

# 2) block_insert 到页末
lark-cli slides +replace-slide --as user \
  --presentation "$PID" --slide-id "$SID" \
  --parts "$(jq -n --arg token "$TOKEN" \
    '[{action:"block_insert",insertion:("<img src=\""+$token+"\" topLeftX=\"500\" topLeftY=\"100\" width=\"200\" height=\"150\"/>")}]')"
```

### 改标题（block_replace）

```bash
# 先拿原页 XML，从里面找到标题块的 3 位 short id（如 bUn）
lark-cli slides xml_presentation.slide get --as user \
  --params "{\"xml_presentation_id\":\"$PID\",\"slide_id\":\"$SID\"}"

# block_replace 换掉整个标题块（id 自动注入）
lark-cli slides +replace-slide --as user \
  --presentation "$PID" --slide-id "$SID" \
  --parts '[{"action":"block_replace","block_id":"bUn","replacement":"<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>新标题</p></content></shape>"}]'
```

### 批量：一次换标题 + 追加装饰图

`block_replace` 和 `block_insert` 可以在同一个 `--parts` 里混用，整批原子执行。

```bash
lark-cli slides +replace-slide --as user \
  --presentation "$PID" --slide-id "$SID" \
  --parts '[
    {"action":"block_replace","block_id":"bab","replacement":"<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>新标题</p></content></shape>"},
    {"action":"block_insert","insertion":"<img src=\"<file_token>\" topLeftX=\"700\" topLeftY=\"400\" width=\"180\" height=\"100\"/>"}
  ]'
```

### 乐观锁

```bash
# 读时记录 revision_id
REV=$(lark-cli slides xml_presentation.slide get --as user \
  --params "{\"xml_presentation_id\":\"$PID\",\"slide_id\":\"$SID\"}" \
  | jq '.data.revision_id')

# 写时传 --revision-id；传不存在的版本号（超过当前 revision）返回 3350002
lark-cli slides +replace-slide --as user \
  --presentation "$PID" --slide-id "$SID" --revision-id "$REV" \
  --parts "$PARTS"
```

## 常见错误

| 现象 | 原因 | 对策 |
|------|------|------|
| 3350001 + hint "block_id not found" | `parts[i].block_id` 在当前页不存在 | 重新 `slide.get` 拿最新 XML，按里面的 short ID 再填 |
| 3350002 not found | `--revision-id` 传了不存在的版本号（超过当前 revision） | 用 `-1` 或用 `slide.get` 拿到的有效 `revision_id` |
| `--parts[i] action "str_replace" is not supported` | CLI 不暴露 `str_replace` | 把替换需求改写成 `block_replace` / `block_insert` |
| `--parts contains N items, exceeds maximum of 200` | 一次提交 parts 太多 | 拆多次调用 |
| `--parts[i] (block_replace) requires non-empty block_id` / `replacement` | 字段缺失 | 按 parts 元素结构补齐 |
| `<img>` 不显示 / 显示破图 | `src` 写了外链 URL | 换成通过 [`+media-upload`](lark-slides-media-upload.md) 拿到的 `file_token` |
| 3350001 | `replacement` 不是合法单根 XML 片段，或 `block_id` 不存在 | CLI 已自动注入 `id` 和 `<content/>`；如果仍报错，重新 `slide.get` 拿最新 XML 确认 `block_id` 存在；检查 XML 结构是否合法；坐标是否超出 960×540 |
| 403 | 权限不足 | 需要 `slides:presentation:update` 或 `slides:presentation:write_only`；wiki URL 还需要 `wiki:node:read` |

## 相关命令

- [xml_presentation.slide get](lark-slides-xml-presentation-slide-get.md) — 读原页拿 `block_id` / `revision_id`
- [xml_presentation.slide replace](lark-slides-xml-presentation-slide-replace.md) — 底层 replace API 参考
- [+media-upload](lark-slides-media-upload.md) — 上传图片拿 `file_token`
- [lark-slides-edit-workflows.md](lark-slides-edit-workflows.md) — 读-改-写闭环 + 决策树
