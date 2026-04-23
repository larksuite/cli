# lark-slides xml_presentation.slide replace

## 用途

对单页做**块级局部替换**：不覆盖整页，按 patch 列表做 `block_replace`（整块替换）或 `block_insert`（整块插入）。适合"只想加 / 换一个元素、不动其他元素"的场景。

> **推荐**：优先使用 [`+replace-slide`](lark-slides-replace-slide.md) Shortcut——它会自动注入 `id` 和 `<content/>`，直接调本 API 需自己处理这两个约束（见注意事项 5、6）。

## 命令

```bash
lark-cli slides xml_presentation.slide replace --as user --params '<json_params>' --data '<json_data>'
```

## 参数说明

| 参数 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `--params` | JSON string | 是 | 路径参数与查询参数 |
| `--data` | JSON string | 是 | patch 列表 |

### params JSON 结构

```json
{
  "xml_presentation_id": "slides_example_presentation_id",
  "slide_id": "slide_example_id",
  "revision_id": -1,
  "tid": "idMock"
}
```

| 字段 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `xml_presentation_id` | string | 是 | 演示文稿唯一标识 |
| `slide_id` | string | 是 | 页面唯一标识 |
| `revision_id` | integer | 否 | 默认 `-1`（以最新版为基准）；传具体版本号做乐观锁 |
| `tid` | string | 否 | 事务 ID，一般留空 |

### data JSON 结构

```json
{
  "parts": [
    { "action": "block_replace", "block_id": "bab", "replacement": "<shape .../>" },
    { "action": "block_insert", "insertion": "<img .../>", "insert_before_block_id": "baa" }
  ]
}
```

| 字段 | 类型 | 必需 | 说明 |
|------|------|------|------|
| `parts` | array | 是 | patch 列表，长度 1~200，顺序执行 |

### parts[] 字段（按 action 不同）

本期 CLI 文档化两种 action：

#### action = "block_replace" — 整块替换

| 字段 | 必填 | 说明 |
|------|------|------|
| `action` | 是 | 固定 `block_replace` |
| `block_id` | 是 | 目标块的 3 位 short element ID（从 `slide.get` 返回的 XML 里读到） |
| `replacement` | 是 | 新 XML 片段，替换整个目标块 |

#### action = "block_insert" — 整块插入

| 字段 | 必填 | 说明 |
|------|------|------|
| `action` | 是 | 固定 `block_insert` |
| `insertion` | 是 | 要插入的完整 XML 片段 |
| `insert_before_block_id` | 否 | 插到这个块之前；省略则追加到页面末尾 |

## 使用示例

### block_replace：换一个 shape 的整体内容

```bash
lark-cli slides xml_presentation.slide replace --as user --params '{
  "xml_presentation_id": "slides_example_presentation_id",
  "slide_id": "slide_example_id"
}' --data '{
  "parts": [
    {
      "action": "block_replace",
      "block_id": "bab",
      "replacement": "<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>新标题</p></content></shape>"
    }
  ]
}'
```

### block_insert：在已有页上加一张图

```bash
# 先拿 file_token
TOKEN=$(lark-cli slides +media-upload --file ./pic.png --presentation "$PID" --as user | jq -r '.data.file_token')

lark-cli slides xml_presentation.slide replace --as user --params "{
  \"xml_presentation_id\": \"$PID\",
  \"slide_id\": \"$SID\"
}" --data "$(jq -n --arg token "$TOKEN" '{
  parts: [
    {
      action: "block_insert",
      insertion: ("<img src=\"" + $token + "\" topLeftX=\"500\" topLeftY=\"100\" width=\"200\" height=\"150\"/>")
    }
  ]
}')"
```

### 多条 parts 原子执行

```bash
lark-cli slides xml_presentation.slide replace --as user --params '{
  "xml_presentation_id": "slides_example_presentation_id",
  "slide_id": "slide_example_id"
}' --data '{
  "parts": [
    {"action":"block_replace","block_id":"bab","replacement":"<shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"120\"><content textType=\"title\"><p>新标题</p></content></shape>"},
    {"action":"block_insert","insertion":"<img src=\"<file_token>\" topLeftX=\"700\" topLeftY=\"400\" width=\"180\" height=\"100\"/>"}
  ]
}'
```

## 返回值

### 成功

```json
{
  "code": 0,
  "data": {
    "revision_id": 105
  },
  "msg": "success"
}
```

### 失败（任一 part 失败，整批不生效）

失败时返回非零错误码（如 3350001）。若后端能定位失败的 part，`data` 中可能附带：

```json
{
  "code": 3350001,
  "data": {
    "failed_part_index": 0,
    "failed_reason": "block not found"
  }
}
```

| 字段 | 类型 | 说明 |
|------|------|------|
| `data.revision_id` | integer | 成功时返回更新后最新版本号 |
| `data.failed_part_index` | integer | 失败的 part 在 `parts` 数组中的索引（从 0 起） |
| `data.failed_reason` | string | 失败原因 |

## 常见错误

| 错误码 | 含义 | 解决方案 |
|--------|------|----------|
| 3350001 | `block_id` 在当前页不存在，或 XML 格式 / 结构错误 | 重新 `slide.get` 拿最新 XML，确认 `block_id` 存在；检查 `replacement` / `insertion` 是否合法 XML |
| 400 | `parts` 长度超过 200 | 拆多次调用 |
| 3350002 | `revision_id` 不存在（超过当前版本号） | 用 `-1` 或实际存在的 `revision_id` |
| 400 | XML 格式错误 | `replacement` / `insertion` 必须为合法的 XML 片段，标签闭合 + 属性引号 |
| 403 | 权限不足 | 需要 `slides:presentation:update` 或 `slides:presentation:write_only` |

## 注意事项

1. **parts 原子事务**：任一条失败整批回滚，不会出现"前几条成功、后几条失败"的中间态。
2. **block_id 的获取**：`slide.get` 返回的 XML 里每个块（shape、img、table、chart 等）会带 3 位 short element ID，用这个值填 `block_id` / `insert_before_block_id`。
3. **`<img>` 必须用 file_token**：不能用外链 URL——先 [`slides +media-upload`](lark-slides-media-upload.md) 拿 token。
4. **不能字段级 patch**：要改一个块的某个属性（比如只改 `topLeftX`），得写整块新 XML 走 `block_replace`；API 不支持"只改一个字段"。
5. **`block_replace` 要求 `replacement` 根元素带 `id="<block_id>"`**：底层 API 的硬约束，缺失会返回 3350001。推荐走 shortcut [`+replace-slide`](lark-slides-replace-slide.md)——它会自动把 `id` 注入到 `replacement` 根元素上，用户写 XML 时不用自己加。
6. **`<shape>` 必须有 `<content/>` 子元素**：SML 2.0 schema 要求，缺失同样触发 3350001。shortcut [`+replace-slide`](lark-slides-replace-slide.md) 会自动注入 `<content/>`，直接调底层 API 需要自己加。
7. **执行前必做**：`lark-cli schema slides.xml_presentation.slide.replace` 查看最新参数结构。

## 相关命令

- [slides +replace-slide](lark-slides-replace-slide.md) — 块级替换 shortcut（推荐，自动注入 id）
- [xml_presentation.slide get](lark-slides-xml-presentation-slide-get.md) — 读原页拿 block short ID
- [slides +media-upload](lark-slides-media-upload.md) — 上传图片拿 file_token
- [lark-slides-edit-workflows.md](lark-slides-edit-workflows.md) — 读-改-写闭环 + 决策树
