# Lark Sheet Range Operations

## 结构性操作影响面预检（清除 / 合并 / 排序 / 移动前必做）

`+cells-clear`、`+cells-{merge|unmerge}`、`+range-{move|copy|fill|sort}`（移动 / 复制 / 排序 / 自动填充）都会让既有引用关系发生偏移或失效。**操作前必须**先确认以下两点；否则禁止执行：

1. **打印当前合并单元格 + 公式引用 + 数据验证范围**：用 `+sheet-info` + `+cells-get` 抽样目标区域和它周边的公式 / 透视表 / 图表 / 条件格式 / 筛选器的数据源；评估操作后这些引用是否仍指向正确数据。
2. **`+cells-clear` 不得侵入用户授权范围之外**：清除范围只能是用户明示要清的区域；不要顺手清除"看起来没用"的相邻单元格。

排序场景的存储类型识别 + 辅助列抽数值的细则见下方「sort 操作前必读」章节。

## 使用场景

写入。对指定区域执行结构性操作。本 Skill 包含四个工具：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 清除内容/格式 | `+cells-clear` | "清空"、"删除内容"、"去掉格式" |
| 合并/取消合并单元格 | `+cells-{merge|unmerge}` | "合并单元格"、"取消合并" |
| 调整行高/列宽 | `+rows-resize / +cols-resize` | "加宽列"、"调整行高"、"自适应列宽" |
| 移动/复制/填充/排序 | `+range-{move|copy|fill|sort}` | "移动数据"、"复制到"、"自动填充"、"按某列排序" |

注意：

- 用户说"这行 / 整行 / 首行"时，优先使用整行范围如 `1:1`；"这列 / 整列"时使用 `J:J`。不要截断为局部矩形
- 合并后只保留左上角单元格的内容，其余清除。写入合并区域用 `+cells-set` 对左上角单元格操作
- 调整行高列宽时，先读取相邻行列尺寸再决定像素值，不要随意猜测
- `copy_to_range`（`+cells-set` 的参数）复制的是值/公式/样式，不含行高列宽。需要统一尺寸时另行调用 `+rows-resize / +cols-resize`

## 写入后列宽自适应（防内容遮挡）

写入文本 / 数值后**必须**主动检查列宽是否适配，否则会出现"内容被截断 / 长数字显示为科学计数法 / 文本溢出被相邻列遮挡"等用户感知问题：

1. **写入后回读最长内容字符数**：用 `+csv-get` 读目标列的实际写入内容，统计最长单元格的字符数（`max(len(cell) for cell in col)`）。汉字按 2 字符宽度估算，半角字母数字按 1 字符。
2. **判定阈值**：当前列宽（用 `get_sheet_structure --info_type=row_heights_column_widths` 拿）≥ 最长字符数 × 字体宽度系数 + buffer 才算适配。默认列宽 11 通常只够 11 个半角字符或 5-6 个汉字，写长文本前必扩宽。
3. **修复二选一**：
   - **扩列宽**：用 `+rows-resize / +cols-resize` 把目标列宽设为 `max(表头字符数, 内容采样最长字符数) × 8 + 16` 像素（经验值）
   - **自动换行**：在 `+cells-set` 时给单元格设置 `cell_styles.word_wrap="auto-wrap"`（可选值：`overflow` / `auto-wrap` / `word-clip`），并用 `+rows-resize / +cols-resize` 调高对应行的行高
4. **新增列默认列宽规则**：新增列宽度 ≥ `max(表头字符数, 内容采样最长字符数) × 8 + 16` 像素，**禁止**用默认 11 直接交付。

**典型反例**：默认列宽 11 但内容含 12+ 字符的中文 / 含单位的数值（如 `109.10μmol/L`）/ 长数字未设 `number_format` 显示为科学计数法 —— 用户在结果表里看不到完整原值。

**⚠️ 合并单元格安全操作规则**（`+cells-{merge|unmerge}` 必读）：

1. **先读后写**：操作前必须用 `+sheet-info`（`info_type: merged_cells_infos`）或 `+cells-get` 识别已有合并区域（特征：多个连续单元格中只有左上角有值，其余为空）。
2. **不要对已合并区域重复 merge**：对已合并的区域再次调用 merge 会报错或产生不可预期结果。
3. **修改合并区域的正确顺序**：先 `unmerge` → 修改内容/样式 → 再 `merge`。
4. **对合并区域设置样式**：只对完整 range 设置一次 `cell_styles`（写在左上角单元格），其余位置用 `{}` 占位。
5. **新增合并时数据保护**：合并前确认目标区域只有左上角有数据，其余单元格为空，否则合并会导致非左上角的数据丢失。
6. **批量取消合并一次调用即可**：当一个范围（整列 `A:A`、整行 `3:3`、矩形 `A1:D100`）内存在多个合并区域，直接调一次 `+cells-{merge|unmerge}(operation: unmerge)` 传入这个大范围，会一次性取消该范围内所有合并区域；**不要**为每个合并区域单独调用 unmerge，也不要用 `+batch-update` 拆成多次 unmerge。

**⚠️ 批量操作必须用 `+batch-update`**：

当需要对**多个**不同区域执行 `+cells-{merge|unmerge}`（merge）或 `+rows-resize / +cols-resize` 时，**禁止逐个调用**，必须使用 `+batch-update`（参见 `lark-sheets-batch-update` skill）将所有操作合并为一次请求。逐个调用会快速耗尽工具调用轮次上限。

**例外**：`+cells-{merge|unmerge}(operation: unmerge)` 原生支持对覆盖多个合并区域的大 range 一次性取消，应直接单次调用，**不要**拆进 `+batch-update`。

示例：需要合并 A1:A3、B1:B3、C1:C3 三个区域时，应使用：
```json
{
  "excel_id": "${excel_id}",
  "operations": [
    {"tool_name": "merge_cells", "input": {"sheet_id": "${sheet_id}", "range": "A1:A3", "operation": "merge"}},
    {"tool_name": "merge_cells", "input": {"sheet_id": "${sheet_id}", "range": "B1:B3", "operation": "merge"}},
    {"tool_name": "merge_cells", "input": {"sheet_id": "${sheet_id}", "range": "C1:C3", "operation": "merge"}}
  ]
}
```
而不是分三次单独调用 `+cells-{merge|unmerge}`。

示例：需要将 A、B、C 三列列宽设为 120px，同时将第 1-3 行行高设为 40px 时，应使用：
```json
{
  "excel_id": "${excel_id}",
  "operations": [
    {"tool_name": "resize_range", "input": {"sheet_id": "${sheet_id}", "range": "A:A", "resize_width": {"type": "pixel", "value": 120}}},
    {"tool_name": "resize_range", "input": {"sheet_id": "${sheet_id}", "range": "B:B", "resize_width": {"type": "pixel", "value": 120}}},
    {"tool_name": "resize_range", "input": {"sheet_id": "${sheet_id}", "range": "C:C", "resize_width": {"type": "pixel", "value": 120}}},
    {"tool_name": "resize_range", "input": {"sheet_id": "${sheet_id}", "range": "1:3", "resize_height": {"type": "pixel", "value": 40}}}
  ]
}
```
而不是分四次单独调用 `+rows-resize / +cols-resize`。

**⚠️ sort 操作前必读：确认目标列的数据类型**

排序按单元格的**存储类型**比较：纯数字按数值排序；文本字符串按**字典序**（`"1000"` 排在 `"999"` 之前，与数值相反）；日期按时间戳排序。

以下形态**看起来像数字但实际是字符串**，直接 sort 会得到错误结果：

| 示例 | 说明 |
|------|------|
| `843688.69+20042.35=863731.04` | 表达式文本（无前导 `=` 不是公式，整串按字典序比较） |
| `¥1,234.56` / `$1,234` | 带货币符号 |
| `1.2万` / `3.5亿` / `100kg` | 带中文 / 英文单位 |
| 前后含空格或不可见字符的数字串 | 被当文本 |
| 同列混文本和数字 | 排序后分块 |

**硬性流程**：

1. sort 前先用 `+csv-get` 抽样目标列的前 3–5 行，或用 `+cells-get`（`value_render_option: "raw_value"` 看原始值；默认 `formatted_value` 返回显示值）确认原始值形态，不要只看列名和用户问题就直接排。
2. 若是纯数字或日期 → 直接 sort。
3. 若是带符号 / 表达式 / 单位的文本 → **不要直接排**：
   - 简单场景（货币、千分位、单位前缀）：新增辅助列，用公式提取数值（如 `=VALUE(SUBSTITUTE(SUBSTITUTE(A2,"¥",""),",",""))`），按辅助列排序，排完可按需清除辅助列。
   - 复杂场景（多段表达式、中文单位、混合格式）：`export_sheet_to_sandbox` + `doubao_code_interpreter` 在沙箱里按数值排序后 `+cells-set` 回写。

## Shortcuts

| MCP tool | CLI shortcut | Risk | 分组 |
| --- | --- | --- | --- |
| `clear_cell_range` | `+cells-clear` | high-risk-write | 单元格 |
| `merge_cells` | `+cells-merge` | write | 单元格 |
|  | `+cells-unmerge` | write | 单元格 |
| `resize_range` | `+rows-resize` | write | 工作表 |
|  | `+cols-resize` | write | 工作表 |
| `transform_range` | `+range-move` | write | 区域 |
|  | `+range-copy` | write | 区域 |
|  | `+range-fill` | write | 区域 |
|  | `+range-sort` | write | 区域 |

## Flags

### `+cells-clear`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--sheet-id` | 公共 | string | XOR | 工作表 reference_id（与 `--sheet-name` 二选一） |
| `--sheet-name` | 公共 | string | XOR | 工作表名称（与 `--sheet-id` 二选一） |
| `--range` | 专有 | string | 是 | 清除范围 A1 格式 |
| `--scope` | 专有 | string + Enum | 否 | `content` / `formats` / `all`，默认 `content`（仅清内容） |
| `--yes` | 系统 | bool | 是 | `high-risk-write`，清除不可逆 |
| `--dry-run` | 系统 | bool | 否 |  |

### `+cells-merge`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--sheet-id` | 公共 | string | XOR | 工作表 reference_id（与 `--sheet-name` 二选一） |
| `--sheet-name` | 公共 | string | XOR | 工作表名称（与 `--sheet-id` 二选一） |
| `--range` | 专有 | string | 是 | 待合并 / 取消合并的范围 |
| `--merge-type` | 专有 | string + Enum | 否 | （仅 `+cells-merge`）`all` / `rows` / `columns`，默认 `all` |
| `--dry-run` | 系统 | bool | 否 |  |

### `+cells-unmerge`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--sheet-id` | 公共 | string | XOR | 工作表 reference_id（与 `--sheet-name` 二选一） |
| `--sheet-name` | 公共 | string | XOR | 工作表名称（与 `--sheet-id` 二选一） |
| `--range` | 专有 | string | 是 | 待合并 / 取消合并的范围 |
| `--merge-type` | 专有 | string + Enum | 否 | （仅 `+cells-merge`）`all` / `rows` / `columns`，默认 `all` |
| `--dry-run` | 系统 | bool | 否 |  |

### `+rows-resize`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--sheet-id` | 公共 | string | XOR | 工作表 reference_id（与 `--sheet-name` 二选一） |
| `--sheet-name` | 公共 | string | XOR | 工作表名称（与 `--sheet-id` 二选一） |
| `--start` | 专有 | int | 是 | 起始行（0-based，inclusive） |
| `--end` | 专有 | int | 是 | 结束行（0-based，inclusive） |
| `--type` | 专有 | string + Enum | 是 | 尺寸方式 enum：`pixel`（指定 px 像素值，需配 `--size`）/ `standard`（重置为默认标准行高）/ `auto`（自动适应内容） |
| `--size` | 专有 | int | 否 | 行高（像素，例：30 / 40 / 60）；`--type pixel` 时必填，其它 type 忽略 |
| `--dry-run` | 系统 | bool | 否 |  |

### `+cols-resize`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--sheet-id` | 公共 | string | XOR | 工作表 reference_id（与 `--sheet-name` 二选一） |
| `--sheet-name` | 公共 | string | XOR | 工作表名称（与 `--sheet-id` 二选一） |
| `--start` | 专有 | int | 是 | 起始列（0-based，inclusive） |
| `--end` | 专有 | int | 是 | 结束列（0-based，inclusive） |
| `--type` | 专有 | string + Enum | 是 | 尺寸方式 enum：`pixel`（指定 px 像素值，需配 `--size`）/ `standard`（重置为默认标准列宽） |
| `--size` | 专有 | int | 否 | 列宽（像素，例：80 / 120 / 200）；`--type pixel` 时必填，其它 type 忽略 |
| `--dry-run` | 系统 | bool | 否 |  |

### `+range-move`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--sheet-id` | 公共 | string | XOR | 工作表 reference_id（与 `--sheet-name` 二选一） |
| `--sheet-name` | 公共 | string | XOR | 工作表名称（与 `--sheet-id` 二选一） |
| `--source-range` | 专有 | string | 是 | 源 A1 范围 |
| `--target-sheet-id` | 专有 | string | 否 | 目标子表；省略时同 sheet |
| `--target-range` | 专有 | string | 是 | 目标 A1 范围（起点 cell 即可，按源尺寸自动推断） |
| `--paste-type` | 专有 | string + Enum | 否 | （仅 `+range-copy`）`values` / `formulas` / `formats` / `all`，默认 `all` |
| `--dry-run` | 系统 | bool | 否 |  |

### `+range-copy`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--sheet-id` | 公共 | string | XOR | 工作表 reference_id（与 `--sheet-name` 二选一） |
| `--sheet-name` | 公共 | string | XOR | 工作表名称（与 `--sheet-id` 二选一） |
| `--source-range` | 专有 | string | 是 | 源 A1 范围 |
| `--target-sheet-id` | 专有 | string | 否 | 目标子表；省略时同 sheet |
| `--target-range` | 专有 | string | 是 | 目标 A1 范围（起点 cell 即可，按源尺寸自动推断） |
| `--paste-type` | 专有 | string + Enum | 否 | （仅 `+range-copy`）`values` / `formulas` / `formats` / `all`，默认 `all` |
| `--dry-run` | 系统 | bool | 否 |  |

### `+range-fill`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--sheet-id` | 公共 | string | XOR | 工作表 reference_id（与 `--sheet-name` 二选一） |
| `--sheet-name` | 公共 | string | XOR | 工作表名称（与 `--sheet-id` 二选一） |
| `--source-range` | 专有 | string | 是 | 填充模板范围（系列起始 cells） |
| `--target-range` | 专有 | string | 是 | 目标填充范围 |
| `--series-type` | 专有 | string + Enum | 否 | `auto` / `linear` / `growth` / `date` / `copy`，默认 `auto` |
| `--dry-run` | 系统 | bool | 否 |  |

### `+range-sort`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--sheet-id` | 公共 | string | XOR | 工作表 reference_id（与 `--sheet-name` 二选一） |
| `--sheet-name` | 公共 | string | XOR | 工作表名称（与 `--sheet-id` 二选一） |
| `--range` | 专有 | string | 是 | 排序范围（含或不含表头由 `--has-header` 决定） |
| `--sort-keys` | 专有 | string + File + Stdin（复合 JSON） | 是 | JSON：`[{"col":"B","order":"asc"},{"col":"D","order":"desc"}]` |
| `--has-header` | 专有 | bool | 否 | 第一行是表头不参与排序，默认 false |
| `--dry-run` | 系统 | bool | 否 |  |

## Schemas

> 复合 JSON flag（`--data` / `--style` / `--options` / `--sort-keys`）的字段速查：只列顶层字段 + 一层嵌套结构。深层结构看 `## Examples` 段的真实示例；要拿完整 JSON Schema 跑 `lark-cli sheets <shortcut> --print-schema --flag <name>`（runtime introspection，待落地）。

### `+range-sort` `--sort-keys`

_排序条件列表（仅 sort 操作）_

**数组项**（类型 object）：
- `ascending` (boolean) — 是否升序排序
- `column` (string) — 排序依据的列字母（如 "C"、"D"），必须在 range 范围内

## Examples

> ⚠️ 本 skill 派生的 shortcut 跨 3 个分组：`+rows-resize` / `+cols-resize` → 工作表，`+cells-*` → 单元格，`+range-*` → 区域。skill 视角统一在这里讲解。

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。

### `+cells-clear`

```bash
# dry-run 先看
lark-cli sheets +cells-clear --url "..." --sheet-id "$SID" --range "A2:Z1000" --scope all --dry-run
# 执行
lark-cli sheets +cells-clear --url "..." --sheet-id "$SID" --range "A2:Z1000" --scope all --yes
```

### `+cells-merge` / `+cells-unmerge`

### `+rows-resize` / `+cols-resize`

行高列宽分两条 shortcut，避免行 / 列在底层 schema 的差异（行支持 `auto`，列不支持）混在一起。每条 `--type` 必填：

```bash
# 把第 2-10 行设为固定 30 px
lark-cli sheets +rows-resize --url "..." --sheet-id "$SID" --start 2 --end 10 --type pixel --size 30

# 把 A-C 列设为固定 120 px
lark-cli sheets +cols-resize --url "..." --sheet-id "$SID" --start 0 --end 2 --type pixel --size 120

# 行高自动适应内容（列宽不支持 auto）
lark-cli sheets +rows-resize --url "..." --sheet-id "$SID" --start 0 --end 0 --type auto

# 重置为默认
lark-cli sheets +cols-resize --url "..." --sheet-id "$SID" --start 0 --end 5 --type standard
```

> 同时出现在 `lark_sheet_sheet_structure/cli-shortcuts.md` —— 行高 / 列宽调整也算行列结构层动作。

### `+range-move` / `+range-copy`

> `+range-move` 会**清空源区域**（move = copy + clear_source）；`+range-copy` 不动源。

### `+range-fill`

### `+range-sort`

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`+cells-clear` 强制 `--yes` 或 `--dry-run`；`+range-*` 校验源 / 目标 range 在同一 spreadsheet；`+range-sort` 的 `--sort-keys` 必须合法 JSON 数组且 col 都在 `--range` 内；`+rows-resize` / `+cols-resize` 的 `--type` 必填，`--type pixel` 时 `--size` 必填、其它 type 时 `--size` 应省略；`+cols-resize.--type` 不接受 `auto`（只行高支持自适应）。
- `DryRun`：所有写操作输出"将要 PATCH 的 range + 受影响 cell 数估算"。
- `Execute`：写后调用 `+cells-get --ranges <影响范围>` 抽样回读对比，envelope.meta.verification 沉淀对比结果。
