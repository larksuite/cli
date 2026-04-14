
# docs +table-update（飞书文档表格编辑）

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

直接操作飞书文档中的表格，支持修改单元格内容、插入/删除行列。与 `docs +update` 不同，本命令通过 docx block API 精确操作表格内部结构，不依赖文本定位。

## 重要说明

> **⚠️ 本命令直接操作文档 block 结构，修改单元格时会先清空再写入，请确保操作正确。**

## 命令

```bash
# 修改单元格内容（第1个表格的第2行第1列）
lark-cli docs +table-update --doc "<doc_id_or_url>" --table-index 0 --action update-cell --row 2 --col 1 --markdown "新内容"

# 插入一行（在第3行位置插入）
lark-cli docs +table-update --doc "<doc_id>" --table-index 0 --action insert-row --at 3

# 删除行（删除第2~3行，不含第3行）
lark-cli docs +table-update --doc "<doc_id>" --table-index 0 --action delete-rows --from 2 --to 3

# 插入一列（在第2列位置插入）
lark-cli docs +table-update --doc "<doc_id>" --table-index 0 --action insert-col --at 2

# 删除列（删除第1~2列，不含第2列）
lark-cli docs +table-update --doc "<doc_id>" --table-index 0 --action delete-cols --from 1 --to 2

# 直接指定表格 block_id（跳过查找）
lark-cli docs +table-update --doc "<doc_id>" --table-id "blk_xxxxx" --action update-cell --row 0 --col 0 --markdown "标题"
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--doc` | 是 | 文档 URL 或 token（支持 wiki/docx 链接） |
| `--action` | 否 | 操作类型，默认 `update-cell`。可选：`update-cell`、`insert-row`、`delete-rows`、`insert-col`、`delete-cols` |
| `--table-index` | 否 | 文档中第 N 个表格（0-based），默认 `0` |
| `--table-id` | 否 | 表格 block_id（覆盖 `--table-index`） |
| `--row` | 视 action | 行索引（0-based），`update-cell` 时必填 |
| `--col` | 视 action | 列索引（0-based），`update-cell` 时必填 |
| `--at` | 视 action | 插入位置索引，`insert-row`/`insert-col` 时必填 |
| `--from` | 视 action | 起始索引（inclusive），`delete-rows`/`delete-cols` 时必填 |
| `--to` | 视 action | 结束索引（exclusive），`delete-rows`/`delete-cols` 时必填 |
| `--markdown` | 视 action | 单元格新内容，`update-cell` 时必填 |

## 操作说明

### update-cell — 修改单元格

通过行列索引精确定位单元格，清空原内容后写入新内容。

```bash
lark-cli docs +table-update --doc "<doc_id>" --row 1 --col 2 --markdown "更新的数据"
```

**注意**：当前 `update-cell` 将 markdown 作为纯文本写入。

### insert-row — 插入行

在指定位置插入一个空行。`--at 0` 在最前面插入，`--at N` 在第 N 行前插入。

```bash
lark-cli docs +table-update --doc "<doc_id>" --action insert-row --at 2
```

### delete-rows — 删除行

删除 `[from, to)` 范围的行。

```bash
# 删除第2行（索引从0开始）
lark-cli docs +table-update --doc "<doc_id>" --action delete-rows --from 2 --to 3
```

### insert-col — 插入列

在指定位置插入一个空列。

```bash
lark-cli docs +table-update --doc "<doc_id>" --action insert-col --at 1
```

### delete-cols — 删除列

删除 `[from, to)` 范围的列。

```bash
lark-cli docs +table-update --doc "<doc_id>" --action delete-cols --from 1 --to 2
```

## 返回值

```json
{
  "ok": true,
  "data": {
    "success": true,
    "action": "update-cell",
    "doc_id": "文档ID",
    "table_id": "表格block_id",
    "cell_id": "单元格block_id",
    "row": 1,
    "col": 2,
    "message": "cell [1,2] updated successfully"
  }
}
```

## 与 docs +update 的区别

| 特性 | `docs +update` | `docs +table-update` |
|------|---------------|---------------------|
| 操作粒度 | 文档 block 级 | 表格单元格级 |
| 表格定位 | 文本匹配（可能不精确） | 索引或 block_id（精确） |
| 行列操作 | 不支持 | 支持 insert/delete |
| 单元格编辑 | 需整表覆盖或文本替换 | 精确定位修改 |
| 适用场景 | 文档整体内容更新 | 表格内部精细编辑 |

## 最佳实践

1. **先 fetch 查看表格结构**：用 `docs +fetch` 查看文档内容，确认表格索引和行列数
2. **优先用 table-index**：通常 `--table-index 0` 即可定位第一个表格
3. **批量修改分步执行**：每次只修改一个单元格，避免行列索引因结构变化而错位
4. **先插入后填充**：插入行/列后，使用 `update-cell` 填充内容

## 参考

- [lark-doc-update](lark-doc-update.md) — 文档级更新
- [lark-doc-fetch](lark-doc-fetch.md) — 获取文档内容
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
