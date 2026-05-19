# Lark Sheet Sheet Structure

## 结构性操作影响面预检（插入 / 删除行列前必做）

插入 / 删除行列、隐藏 / 取消隐藏、冻结、行列分组都会让原表的引用关系发生偏移。**操作前必须**先打印以下三类信息，并评估操作是否会让它们失效；否则禁止执行：

1. **当前合并单元格范围**（来自 `+sheet-info` 的 `merged_cells`）：插入行 / 列时，跨过插入位置的合并区域可能扩张或断裂；删除行 / 列时合并区域可能直接消失。
2. **现有公式的引用范围**（用 `+cells-get` 抽样附近行 + 跨表引用 + 透视表 / 图表 / 条件格式 / 筛选器的数据源 range）：插入 / 删除会导致 `=SUM(B4:B13)` 这种相对引用偏移；如果操作发生在引用范围内部，可能产生 `#REF!`。
3. **数据验证（下拉列表）规则的应用范围**：列表来源是某个区域时，区域被部分删除会让规则失效。

不可逆的影响必须先在回复中告知用户，得到确认再执行。

## 使用场景

读写。管理子表结构与布局。本 Skill 包含两个工具：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 查看子表布局 | `+sheet-info` | 获取行高、列宽、隐藏行列、行列分组、合并单元格等信息 |
| 变更子表结构 | `+dim-{insert|delete|hide|unhide|freeze|group|ungroup}` | 插入/删除/隐藏/取消隐藏/冻结行列、行列分组操作 |

注意：

- 当表格存在合并单元格时，应结合返回的 `merged_cells` 判断表头、分组标题和区域语义
- 不要把合并区域中非左上角的空白单元格理解为"无内容"；通常应将左上角单元格的内容视为整个合并区域的语义内容
- 当前插入语义使用 `operation="insert"` + `position` + `count` + `side`
- 处理"在第 N 行后追加"这类请求时，要显式区分 `before` 和 `after`，避免 off-by-one
- 例如"在第 20 行后新增 116 行"，应优先理解为 `position="20"`、`side="after"`、`count=116`

**常见配置错误（必须注意）**：
- **插入列位置偏移**：插入列时 `position` 是基于 0 的列索引，不是列字母。插入前先通过 `+workbook-info` 或读取表头确认目标位置的实际列索引，不要凭猜测
- **插入后引用偏移**：插入行/列后，原有数据的行列号会发生偏移。如果插入后还需要对原有区域执行写入操作，必须重新计算偏移后的行列号
- **删除行列前先确认范围**：删除操作不可逆，执行前应确认 `position` 和 `count` 精确无误。可先用 `+csv-get` 读取目标区域验证内容
- **"在左侧新增一列"的正确写法**：用户说"在 D 列左侧新增一列"时，应使用 `position` 对应 D 列索引 + `side="before"`，而不是 C 列 + `side="after"`（两者效果一样但前者语义更清晰）
- **插入列后必须检查多行表头合并区域**：很多表格有 2-3 行的合并表头。插入列后，原有的合并区域不会自动扩展到新列。必须先用 `+sheet-info`（`info_type: merged_cells_infos`）读取合并区域，插入后将跨越插入位置的合并区域重新设置（用 `+cells-{merge|unmerge}`），否则新列的表头会是空的、格式不连续
- **公式写入范围跳过表头行**：写入公式时从数据行开始（不是第 1 行）。先确认表头占几行（可能 1-3 行），公式的起始行 = 表头行数 + 1

## Shortcuts

| MCP tool | CLI shortcut | Risk | 分组 |
| --- | --- | --- | --- |
| `get_sheet_structure` | `+sheet-info` | read | 工作表 |
| `modify_sheet_structure` | `+dim-insert` | write | 工作表 |
|  | `+dim-delete` | high-risk-write | 工作表 |
|  | `+dim-hide` | write | 工作表 |
|  | `+dim-unhide` | write | 工作表 |
|  | `+dim-freeze` | write | 工作表 |
|  | `+dim-group` | write | 工作表 |
|  | `+dim-ungroup` | write | 工作表 |
| `move_dimension` | `+dim-move` | write | 工作表 |

## Flags

### `+sheet-info`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--include` | string_slice + Enum | 否 | `merges` / `row_heights` / `col_widths` / `hidden_rows` / `hidden_cols` / `groups` / `frozen`，逗号拆分 |

### `+dim-insert`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--dimension` | string + Enum | 是 | `row` / `column` |
| `--start` | int | 是 | 插入起始位置（0-based） |
| `--end` | int | 是 | 插入结束位置（exclusive） |
| `--inherit-style` | string + Enum | 否 | `before` / `after` / `none`；默认 `none` |

### `+dim-delete`

_公共四件套 · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--dimension` | string + Enum | 是 | `row` / `column` |
| `--start` | int | 是 | 起始（0-based） |
| `--end` | int | 是 | 结束（exclusive） |

### `+dim-hide`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--dimension` | string + Enum | 是 | `row` / `column` |
| `--start` | int | 是 | 范围 |
| `--end` | int | 是 | 范围 |

### `+dim-unhide`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--dimension` | string + Enum | 是 | `row` / `column` |
| `--start` | int | 是 | 范围 |
| `--end` | int | 是 | 范围 |

### `+dim-freeze`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--dimension` | string + Enum | 是 | `row` / `column` |
| `--count` | int | 是 | 冻结前 N 行/列；传 0 解除冻结 |

### `+dim-group`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--dimension` | string + Enum | 是 | `row` / `column` |
| `--start` | int | 是 | 范围 |
| `--end` | int | 是 | 范围 |
| `--depth` | int | 否 | 嵌套层级（`+dim-group` 用），默认 1 |

### `+dim-ungroup`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--dimension` | string + Enum | 是 | `row` / `column` |
| `--start` | int | 是 | 范围 |
| `--end` | int | 是 | 范围 |
| `--depth` | int | 否 | 嵌套层级（`+dim-group` 用），默认 1 |

### `+dim-move`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--dimension` | string + Enum | 是 | `row` / `column` |
| `--start` | int | 是 | 源起始位置（0-indexed，inclusive） |
| `--end` | int | 是 | 源结束位置（0-indexed，inclusive） |
| `--target` | int | 是 | 目标位置（move 到该 index 前面，0-indexed） |

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。

### `+sheet-info`

输出契约：返回子表的行高 / 列宽 / 隐藏 / 合并 / 分组等布局元信息。

### `+dim-insert`

示例：

```bash
# 在第 10 行前插 3 行，继承上方样式
lark-cli sheets +dim-insert --url "https://example.feishu.cn/sheets/shtXXX" \
  --sheet-id "$SID" --dimension row --start 10 --end 13 --inherit-style before
```

### `+dim-delete`

### `+dim-hide` / `+dim-unhide`

### `+rows-resize` / `+cols-resize`

> ⚠️ 这两条 shortcut 来自 `lark_sheet_range_operations` 的 `resize_range` tool（分组在"工作表"是为了发现性）。详细参数和示例在 `lark_sheet_range_operations/cli-shortcuts.md`。
>
> 行 vs 列底层 schema 有差异：`+rows-resize.--type` 支持 `pixel` / `standard` / `auto`，`+cols-resize.--type` 只支持 `pixel` / `standard`（列宽不支持自动适应）。

### `+dim-freeze`

### `+dim-group` / `+dim-ungroup`（大纲）

> 仅当用户明确说"行分组 / 列分组 / 大纲 / outline"时触发；按字段做数据分组用 `+pivot-create`。

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`--start ≤ --end`；`+dim-delete` 强制 `--yes` 或 `--dry-run`；`+rows-resize` / `+cols-resize` 的 `--type` 必填，`--type pixel` 时 `--size` 必填、其它 type 时 `--size` 应省略；`+cols-resize.--type` 不接受 `auto`（详见 `lark_sheet_range_operations/cli-shortcuts.md`）。
- `DryRun`：写操作输出"将要 PATCH 的 dimension 区间 + 目标参数"。
- `Execute`：写后自动调用 `+sheet-info --include row_heights,col_widths,hidden_rows,hidden_cols,groups,frozen` 回读对比，envelope.meta.verification 给出受影响的范围。
