# Lark Sheet Sheet Structure

## 结构性操作影响面预检（插入 / 删除行列前必做）

插入 / 删除行列、隐藏 / 取消隐藏、冻结、行列分组都会让原表的引用关系发生偏移。**操作前必须**先打印以下三类信息，并评估操作是否会让它们失效；否则禁止执行：

1. **当前合并单元格范围**（来自 `+sheet-info` 的 `merged_cells`）：插入行 / 列时，跨过插入位置的合并区域可能扩张或断裂；删除行 / 列时合并区域可能直接消失。
2. **现有公式的引用范围**（用 `+cells-get` 抽样附近行 + 跨表引用 + 透视表 / 图表 / 条件格式 / 筛选器的数据源 range）：插入 / 删除会导致 `=SUM(B4:B13)` 这种相对引用偏移；如果操作发生在引用范围内部，可能产生 `#REF!`。
3. **数据验证（下拉列表）规则的应用范围**：列表来源是某个区域时，区域被部分删除会让规则失效。

不可逆的影响必须先在回复中告知用户，得到确认再执行。

## 使用场景

读写。管理子表结构与布局。本 reference 覆盖 9 个 shortcut（按用途分两类）：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 查看子表布局 | `+sheet-info` | 获取行高、列宽、隐藏行列、行列分组、合并单元格等信息 |
| 变更子表结构 | `+dim-{insert|delete|hide|unhide|freeze|group|ungroup|move}` | 插入/删除/隐藏/取消隐藏/冻结/分组/移动行列 |

注意：

- 当表格存在合并单元格时，应结合返回的 `merged_cells` 判断表头、分组标题和区域语义
- 不要把合并区域中非左上角的空白单元格理解为"无内容"；通常应将左上角单元格的内容视为整个合并区域的语义内容
- 插入用 `+dim-insert`：`--position`（插入位置；行用 1-based 行号如 `3`，列用字母如 `C`，新行/列插在此位置**之前**）+ `--count`（插入数量，>0）。新行/列样式继承用 `--inherit-style`（`before`/`after`/`none`）
- 例如"在第 20 行后新增 116 行"：`--position 21 --count 116`（"第 20 行后"即 1-based 行号 21）

**区间表达统一为 A1 风格**：所有涉及"一段连续行/列"的 shortcut 都用同一套 A1 闭区间字符串语法，**不存在 inclusive / exclusive / 0-based / 1-based 跨命令差异**：

| 命令 | 用什么 flag 表达区间 / 位置 | 例子 |
| --- | --- | --- |
| `+dim-insert` | `--position` + `--count` | `--position 3 --count 5`（在第 3 行前插 5 行）/ `--position C --count 2`（在 C 列前插 2 列） |
| `+dim-delete` / `+dim-hide` / `+dim-unhide` / `+dim-group` / `+dim-ungroup` / `+rows-resize` / `+cols-resize` | `--range` | `"3:7"`（第 3-7 行，闭区间）/ `"C:F"`（C-F 列，闭区间）/ `"5"` 或 `"C"`（单行/列） |
| `+dim-move` | `--source-range`（源区间）+ `--target`（目标位置） | `--source-range "3:7" --target 12`（把第 3-7 行移到第 12 行前）/ `--source-range "C:F" --target H` |

行用 1-based 数字、列用字母——跟 Excel / 飞书 UI 看到的行号、列字母完全一致。

**常见配置错误（必须注意）**：
- **插入列直接用字母**：`+dim-insert` 的 `--position` 在列场景直接传字母（如 `C`），不要把列字母换算成 0-based 索引
- **插入后引用偏移**：插入行/列后，原有数据的行号 / 列字母会发生偏移。如果插入后还需要对原有区域执行写入操作，必须重新计算偏移后的位置
- **删除行列前先确认范围**：删除操作不可逆，执行前应确认 `--range` 精确无误。可先用 `+csv-get` 读取目标区域验证内容（`+csv-get` / `+cells-get` 见 `lark-sheets-read-data`）
- **"在 D 列左侧新增一列"的正确写法**：`--position D --count 1`（新列插在 D 列之前）；要继承左侧列样式加 `--inherit-style before`
- **`+dim-move` 同维度约束**：`--source-range` 是行区间时 `--target` 必须是行号（数字），是列区间时 `--target` 必须是列字母——不可一行一列混用
- **插入列后必须检查多行表头合并区域**：很多表格有 2-3 行的合并表头。插入列后，原有的合并区域不会自动扩展到新列。必须先用 `+sheet-info --include merges` 读取合并区域，插入后将跨越插入位置的合并区域重新设置（用 `+cells-{merge|unmerge}`），否则新列的表头会是空的、格式不连续
- **公式写入范围跳过表头行**：写入公式时从数据行开始（不是第 1 行）。先确认表头占几行（可能 1-3 行），公式的起始行 = 表头行数 + 1

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+sheet-info` | read | 工作表 |
| `+dim-insert` | write | 工作表 |
| `+dim-delete` | high-risk-write | 工作表 |
| `+dim-hide` | write | 工作表 |
| `+dim-unhide` | write | 工作表 |
| `+dim-freeze` | write | 工作表 |
| `+dim-group` | write | 工作表 |
| `+dim-ungroup` | write | 工作表 |
| `+dim-move` | write | 工作表 |

## Flags

### `+sheet-info`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--include` | string_slice | optional | 要返回的结构信息类别，逗号分隔多个（可选值：`merges` / `row_heights` / `col_widths` / `hidden_rows` / `hidden_cols` / `groups` / `frozen`） |
| `--range` | string | optional | 限定只返回该 A1 范围的结构信息；省略时返回整表 |

### `+dim-insert`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--inherit-style` | string | optional | 新行/列样式继承策略 enum：`before`（继承前一行/列）/ `after`（继承后一行/列）/ `none`（默认）（可选值：`before` / `after` / `none`） |
| `--position` | string | required | 插入位置（在此行/列**之前**插入）：行用 1-based 行号如 `3`；列用字母如 `C` |
| `--count` | int | required | 插入数量（>0） |

### `+dim-delete`

_公共四件套 · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | required | 要删除的行/列闭区间；行用 1-based 数字如 `3:7` 或单行 `5`，列用字母如 `C:F` 或单列 `C` |

### `+dim-hide`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | required | 要隐藏的行/列闭区间；行如 `3:7`，列如 `C:F` |

### `+dim-unhide`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | required | 要取消隐藏的行/列闭区间；行如 `3:7`，列如 `C:F` |

### `+dim-freeze`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--dimension` | string | required | 维度方向（行或列）（可选值：`row` / `column`） |
| `--count` | int | required | 冻结前 N 行/列；传 0 解除冻结 |

### `+dim-group`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--depth` | int | optional | 嵌套分组的层级（创建到第几层），默认 1 |
| `--group-state` | string | optional | 分组初始展开状态（可选值：`expand` / `fold`）（默认 `expand`） |
| `--range` | string | required | 要创建分组的行/列闭区间；行如 `3:7`，列如 `C:F` |

### `+dim-ungroup`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--depth` | int | optional | 要取消的分组层级，默认 1（1=最外层，数字越大越内层） |
| `--range` | string | required | 要取消分组的行/列闭区间；行如 `3:7`，列如 `C:F` |

### `+dim-move`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--source-range` | string | required | 要移动的源行/列闭区间；行如 `3:7`，列如 `C:F` |
| `--target` | string | required | 目标位置（移到此行/列**之前**）：行用 1-based 行号如 `12`，列用字母如 `H`。必须与 `--source-range` 同维度（行/列） |

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。

### `+sheet-info`

输出契约：返回子表的行高 / 列宽 / 隐藏 / 合并 / 分组等布局元信息。

### `+dim-insert`

```bash
# 在第 10 行前插 3 行，继承上方样式
lark-cli sheets +dim-insert --url "https://example.feishu.cn/sheets/shtXXX" \
  --sheet-id "$SID" --position 10 --count 3 --inherit-style before

# 在 C 列前插 2 列
lark-cli sheets +dim-insert --url "..." --sheet-id "$SID" --position C --count 2
```

### `+dim-delete`

```bash
# 删除第 5-7 行
lark-cli sheets +dim-delete --url "..." --sheet-id "$SID" --range "5:7" --yes

# 删除 D-F 列
lark-cli sheets +dim-delete --url "..." --sheet-id "$SID" --range "D:F" --yes
```

### `+dim-hide` / `+dim-unhide`

```bash
lark-cli sheets +dim-hide   --url "..." --sheet-id "$SID" --range "5:7"
lark-cli sheets +dim-unhide --url "..." --sheet-id "$SID" --range "5:7"
lark-cli sheets +dim-hide   --url "..." --sheet-id "$SID" --range "C:F"
```

### `+dim-move`

```bash
# 把第 3-7 行移到第 12 行前
lark-cli sheets +dim-move --url "..." --sheet-id "$SID" --source-range "3:7" --target 12

# 把 C-F 列移到 H 列前
lark-cli sheets +dim-move --url "..." --sheet-id "$SID" --source-range "C:F" --target H
```

### `+rows-resize` / `+cols-resize`

> ⚠️ 这两条 shortcut 来自 `lark-sheets-range-operations` 的 `+rows-resize / +cols-resize` tool（分组在"工作表"是为了发现性）。详细参数和示例在 `lark-sheets-range-operations.md`。
>
> 常规写法：行高走 `--range` + `--height <px>`、列宽走 `--range` + `--width <px>`，无需再传 `--type`（等价于 `--type pixel`）；多行 / 多列不同尺寸用 map 形态 `--heights` / `--widths`（如 `--widths '{"A":100,"C:E":120}'`）一次原子完成，不要拆多次调用或走 `+batch-update`。`--type standard` / `--type auto` 用于非像素模式，不能与像素 flag 同给。`+cols-resize.--type` 不接受 `auto`（列宽不支持自动适应）。⚠️ 单位是像素（不是 Excel 字符单位 / 磅）。

### `+dim-freeze`

```bash
# 冻结前 1 行（--count 传 0 解除冻结）
lark-cli sheets +dim-freeze --url "..." --sheet-id "$SID" --dimension row --count 1
```

### `+dim-group` / `+dim-ungroup`（大纲）

> 仅当用户明确说"行分组 / 列分组 / 大纲 / outline"时触发；按字段做数据分组用 `+pivot-create`。

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`--range` / `--source-range` 必须是合法 A1 闭区间（行用数字、列用字母，不可混用）；`+dim-insert` 的 `--count` > 0；`+dim-move` 的 `--target` 必须与 `--source-range` 同维度（行 vs 列）；`+dim-delete` 强制 `--yes` 或 `--dry-run`；`+rows-resize` / `+cols-resize` 的统一形态（`--range` + `--height`/`--width` 或 `--type`）与 map 形态（`--heights`/`--widths`）二选一、不可混用；详见 `lark-sheets-range-operations.md`。
- `DryRun`：写操作输出"将要 PATCH 的目标范围 + 目标参数"。
- `Execute`：写后不自动回读；如需确认，自行调用 `+sheet-info --include row_heights,col_widths,hidden_rows,hidden_cols,groups,frozen` 查看受影响的范围。
