# 编辑已有 PPT：读-改-写闭环

编辑走 **shortcut [`+replace-slide`](lark-slides-replace-slide.md)**（块级替换 / 插入），配合 `xml_presentation.slide.get` 读原页拿 `block_id`。

> 生成 XML 前**必读** [xml-schema-quick-ref.md](xml-schema-quick-ref.md)。

## 决策树：block_replace vs block_insert

| 需求 | 推荐 action | 理由 |
|------|------------|------|
| 已知某块的 `block_id`，要换这块内容（改标题、换图、挪坐标） | `block_replace` | 精准替换，原子性好；`replacement` 根 `id` 由 CLI 自动注入为 `block_id` |
| 只加 1~N 个元素、不动现有布局 | `block_insert` | 新增不覆盖，可选 `insert_before_block_id` 指定位置 |
| 一次动多个元素（如：换标题 + 加图） | 单次 `--parts` 里拼多条 | 整批作为原子事务，任一失败整批不生效；`block_replace` 和 `block_insert` 可混用 |

> **没有字段级 patch**：即便只想改一个 `shape` 的 `topLeftX`，也得把整个块的新 XML 写出来用 `block_replace`。这不是"微调"，是块级重写。

## 最小读-改-写闭环

```bash
PID="xml_presentation_id_here"
SID="slide_id_here"

# 1. 读原页，从 XML 里挑出要改的块的 3 位 short id（如 bUn / bab）
lark-cli slides xml_presentation.slide get --as user \
  --params "{\"xml_presentation_id\":\"$PID\",\"slide_id\":\"$SID\"}"

# 2. 用 +replace-slide 直接改那个块（不需要搬原 XML）
lark-cli slides +replace-slide --as user \
  --presentation "$PID" --slide-id "$SID" \
  --parts '[{"action":"block_replace","block_id":"bUn","replacement":"<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>新标题</p></content></shape>"}]'
```

`slide_id` / 页序不会变。`block_replace` 的 `replacement` 根元素 `id` 会自动注入为 `block_id`，用户手写 XML 时不需要自己加。

## `revision_id` 参数

`--revision-id` 默认 `-1`，表示基于当前最新版执行。传具体版本号时，服务端以该版本为 base 应用变更：

```bash
# 读时拿当前 revision_id
REV=$(lark-cli slides xml_presentation.slide get --as user \
  --params "{\"xml_presentation_id\":\"$PID\",\"slide_id\":\"$SID\"}" \
  | jq '.data.revision_id')

# 写时传该版本号，服务端以此为 base
lark-cli slides +replace-slide --as user \
  --presentation "$PID" --slide-id "$SID" --revision-id "$REV" \
  --parts '[{"action":"block_replace","block_id":"bUn","replacement":"<shape type=\"rect\" topLeftX=\"100\" topLeftY=\"100\" width=\"200\" height=\"100\"/>"}]'
```

注意：传不存在的版本号（超过当前 revision）会返回 3350002 not found；不确定时用 `-1` 即可。

## `--tid` 事务锁

跨请求的并发事务 ID，多人协作长事务才用得上。**单人单次调用留空**即可。

## 两种 action 详解

### block_replace — 整块替换

适合"已知块 ID，要换这块整体内容"的场景。`replacement` 根元素的 `id="<block_id>"` 由 CLI 自动注入（用户手写的 XML 如果没带 `id` 直接省略即可；如果带了错的会被覆盖为正确值）。

```bash
lark-cli slides +replace-slide --as user \
  --presentation "$PID" --slide-id "$SID" \
  --parts '[{"action":"block_replace","block_id":"bab","replacement":"<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>新标题</p></content></shape>"}]'
```

字段说明：

| 字段 | 必填 | 说明 |
|------|------|------|
| `action` | 是 | 固定为 `block_replace` |
| `block_id` | 是 | 目标块的 3 位 short element ID（从 `slide.get` 返回的 XML 里读）|
| `replacement` | 是 | 新 XML 片段；根元素 `id` 会被 CLI 自动注入为 `block_id` |

### block_insert — 整块插入

适合"只想加一个元素，不动现有元素"的场景（典型：给已有页加图）。

```bash
lark-cli slides +replace-slide --as user \
  --presentation "$PID" --slide-id "$SID" \
  --parts "$(jq -n --arg token "$FILE_TOKEN" \
    '[{action:"block_insert",insertion:("<img src=\""+$token+"\" topLeftX=\"500\" topLeftY=\"100\" width=\"200\" height=\"150\"/>"),insert_before_block_id:"baa"}]')"
```

字段说明：

| 字段 | 必填 | 说明 |
|------|------|------|
| `action` | 是 | 固定为 `block_insert` |
| `insertion` | 是 | 要插入的完整 XML 片段 |
| `insert_before_block_id` | 否 | 插到这个块之前；省略（不提供此字段）则追加到页面末尾 |

> **`<img>` 必须用 `file_token`**，不能用外链 URL——先 `slides +media-upload --file ./pic.png --presentation $PID` 拿 token。

### 批量 parts

一次 `--parts` 最多 200 条，按数组顺序串行执行。`block_replace` 和 `block_insert` 可以在同一批次混用。举例：一次性把标题块替换、然后在末尾追加一个装饰图。

```bash
lark-cli slides +replace-slide --as user \
  --presentation "$PID" --slide-id "$SID" \
  --parts '[
    {"action":"block_replace","block_id":"bab","replacement":"<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>新标题</p></content></shape>"},
    {"action":"block_insert","insertion":"<img src=\"<file_token>\" topLeftX=\"700\" topLeftY=\"400\" width=\"180\" height=\"100\"/>"}
  ]'
```

整批作为原子事务：任一条失败整批不生效。失败时后端通常返回 3350001；若响应中带 `failed_part_index` / `failed_reason` 字段，shortcut 会原样透传。

## 大 --parts 用 jq 或 stdin 组装

`--parts` 支持 `@file`（读文件）和 `-`（stdin）作为值来源，适合批量 XML 场景：

```bash
# 从文件读
lark-cli slides +replace-slide --as user --presentation "$PID" --slide-id "$SID" \
  --parts @parts.json

# 从 stdin 读
cat parts.json | lark-cli slides +replace-slide --as user --presentation "$PID" --slide-id "$SID" \
  --parts -
```

## 错误排查

| 现象 | 原因 | 对策 |
|------|------|------|
| 3350001，hint 含 "block_id not found" | `parts[i].block_id` 在当前页不存在 | 重新 `slide.get` 拿最新 XML，按里面的 short ID 再填 |
| 3350002 not found | `--revision-id` 传了不存在的版本号 | 用 `-1` 或实际存在的 `revision_id` |
| `<img>` 不显示 / 显示破图 | `src` 写了外链 URL | 换成通过 `+media-upload` 拿到的 `file_token` |
| 3350001（block_replace 返回） | 正常情况下 CLI 已自动注入 `id` 和 `<content/>`；如果仍报错，确认 `block_id` 在当前页存在（重新 `slide.get`），检查 XML 结构是否合法；坐标是否超出 960×540 范围 | — |

## 相关文档

- [lark-slides-replace-slide.md](lark-slides-replace-slide.md) — +replace-slide shortcut 参数详情
- [lark-slides-xml-presentation-slide-get.md](lark-slides-xml-presentation-slide-get.md) — slide.get 参考（拿 `block_id` / `revision_id`）
- [lark-slides-xml-presentation-slide-replace.md](lark-slides-xml-presentation-slide-replace.md) — 底层 replace API 参考（一般直接用 shortcut 即可）
- [lark-slides-media-upload.md](lark-slides-media-upload.md) — 上传图片拿 file_token
- [xml-schema-quick-ref.md](xml-schema-quick-ref.md) — XML 元素和属性速查
