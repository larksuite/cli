# Lark Sheet Batch Update

## 写入边界 + 回读校验

`+batch-update` 把多次写入打包成单次请求，但每个子操作仍受编辑类任务硬性默认规则约束：

1. **目标 range 必须落在用户授权范围内**：除用户明示要修改的区域外，子操作禁止扩张到无关单元格 / 列 / Sheet。规划 range 时先确认每个子操作的边界。
2. **批次完成后必须回读校验**：整个 `+batch-update` 执行成功后，用 `+csv-get` 或 `+cells-get` 抽样回读受影响区域，至少校验 3-5 个代表性单元格（首 / 中 / 末），与 `doubao_code_interpreter` 预先计算的预期值对照。
3. **预期条数前置断言**：涉及"批量填充 N 行"或"对 M 个区域分别写入"时，先把 N、M 硬编码进代码，回读后断言实际等于预期；不一致就再发一轮 `+batch-update` 补齐，禁止交付半成品。

## 使用场景

写入。批量执行多个写入工具操作。将多个工具调用合并为一次请求，按顺序依次执行。适合需要连续执行多个写入操作的场景（如先修改结构再写入数据）。注意：不支持嵌套 batch_update。

**⚠️ 何时必须使用 `+batch-update`（硬性要求）**：
- 需要对**多个**不同区域执行 `+cells-{merge|unmerge}` 时（如按分组合并多列相同内容）
- 需要对**多个**不同区域执行 `+rows-resize / +cols-resize` 时（如统一调整多列列宽或多行行高）
- 需要先插入行列再写入数据时（`+dim-{insert|delete|hide|unhide|freeze|group|ungroup}` + `+cells-set`）
- 需要对多个区域执行不同写入操作时（多次 `+cells-set` + `+cells-clear` 等组合）

当同一工具需要对多个区域重复调用时，**必须**改用 `+batch-update` 合并为单次请求。逐个调用会快速耗尽工具调用轮次上限（60R），导致任务无法完成。

## Shortcuts

| MCP tool | CLI shortcut | Risk | 分组 |
| --- | --- | --- | --- |
| `batch_update` | `+batch-update` | high-risk-write | 批量 |
|  | `+cells-batch-set-style` | write | 批量 |
|  | `+dropdown-update` | write | 对象 |
|  | `+dropdown-delete` | high-risk-write | 对象 |

## Flags

### `+batch-update`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet 定位（与子操作的 sheet 定位独立） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet 定位（与子操作的 sheet 定位独立） |
| `--operations` | 专有 | string + File + Stdin（复合 JSON） | 是 | JSON：`{"operations":[{"tool":"set_cell_range","params":{...}}, ...]}`；按数组顺序串行执行 |
| `--yes` | 系统 | bool | 是 | `high-risk-write`，必须二次确认（不带时退出码 10） |
| `--dry-run` | 系统 | bool | 否 | 输出每个子操作的请求模板，零网络副作用 |

### `+cells-batch-set-style`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--ranges` | 专有 | string + File + Stdin（简单 JSON） | 是 | 目标范围 JSON 数组，每项必须带 sheet 前缀（如 `["sheet1!A1:B2","sheet1!D1:D10"]`）；所有 range 应用同一组 style |
| `--background-color` | 专有 | string | 否 | 背景颜色（十六进制，如 `#ffffff`） |
| `--font-color` | 专有 | string | 否 | 字体颜色（十六进制，如 `#000000`） |
| `--font-size` | 专有 | number | 否 | 字体大小（px，例：10、12、14） |
| `--font-style` | 专有 | string + Enum | 否 | 字体样式 enum：`normal` / `italic` |
| `--font-weight` | 专有 | string + Enum | 否 | 字重 enum：`normal` / `bold` |
| `--font-line` | 专有 | string + Enum | 否 | 字体线条样式 enum：`none` / `underline` / `line-through` |
| `--horizontal-alignment` | 专有 | string + Enum | 否 | 水平对齐 enum：`left` / `center` / `right` |
| `--vertical-alignment` | 专有 | string + Enum | 否 | 垂直对齐 enum：`top` / `middle` / `bottom` |
| `--word-wrap` | 专有 | string + Enum | 否 | 换行策略 enum：`overflow` / `auto-wrap` / `word-clip`（默认 `overflow`） |
| `--number-format` | 专有 | string | 否 | 数字格式（例：文本 `@`、数字 `0.00`、货币 `$#,##0.00`、日期 `mm/dd/yyyy`） |
| `--border-styles` | 专有 | string + File + Stdin（复合 JSON） | 否 | 边框配置 JSON（结构同 +cells-set-style） |
| `--dry-run` | 系统 | bool | 否 |  |

### `+dropdown-update`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--ranges` | 专有 | string + File + Stdin（简单 JSON） | 是 | 目标范围 JSON 数组（如 `["sheet1!A2:A100"]`），每项必须带 sheet 前缀 |
| `--options` | 专有 | string + File + Stdin（复合 JSON） | 是 | 选项 JSON 数组 |
| `--colors` | 专有 | string + File + Stdin（简单 JSON） | 否 | 颜色数组（与 `--options` 等长） |
| `--multiple` | 专有 | bool | 否 | 启用多选 |
| `--highlight` | 专有 | bool | 否 | 选项配色 |
| `--dry-run` | 系统 | bool | 否 |  |

### `+dropdown-delete`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--ranges` | 专有 | string + File + Stdin（简单 JSON） | 是 | 目标范围 JSON 数组（最多 100 个，每项带 sheet 前缀） |
| `--yes` | 系统 | bool | 是 | `high-risk-write`，必须二次确认（不带时退出码 10） |
| `--dry-run` | 系统 | bool | 否 |  |

## Schemas

> 复合 JSON flag（`--data` / `--style` / `--options` / `--sort-keys`）的字段速查：只列顶层字段 + 一层嵌套结构。深层结构看 `## Examples` 段的真实示例；要拿完整 JSON Schema 跑 `lark-cli sheets <shortcut> --print-schema --flag <name>`（runtime introspection，待落地）。

### `+batch-update` `--operations`

_要批量执行的操作列表，按顺序依次执行_

**数组项**（类型 object）：
- `input` (object) — 对应工具的入参，结构与单独调用该工具时完全一致
- `tool_name` (string) — 要执行的工具名称，如 "set_cell_range"、"clear_cell_range"、"modify_sheet_structure" 等

### `+cells-batch-set-style` `--border-styles`

_单元格边框配置，含 top/bottom/left/right 四个方向，每个方向的结构相同（见 top）_

**顶层字段**：
- `bottom` (object?) { color?: string, style?: enum, weight?: enum }
- `left` (object?) { color?: string, style?: enum, weight?: enum }
- `right` (object?) { color?: string, style?: enum, weight?: enum }
- `top` (object?) { color?: string, style?: enum, weight?: enum }

### `+dropdown-update` `--options`

_数据验证配置_

**顶层字段**：
- `help_text` (string?) — 验证失败时显示的提示文本
- `items` (array<string>?) — 列表选项（type='list' 时必填）
- `operator` (enum?) — 比较运算符（type='number'/'date'/'textLength' 时必填） [equal / notEqual / greaterThan / greaterThanOrEqual / lessThan / lessThanOrEqual / between / notBetween]
- `range` (string?) — 源数据区域（type='listFromRange' 时必填，格式：'SheetName!A1:A10'）
- `support_multiple_values` (boolean?) — 列表验证是否支持多选（type='list'/'listFromRange' 时可选，默认 false）
- `type` (enum) — 数据验证类型：list（下拉列表）、listFromRange（引用范围下拉列表）、number（数字）、date（日期）、textLength（文本长度）、… [list / listFromRange / number / date / textLength / checkbox]
- `values` (array<oneOf>?) — 比较值（operator 为 'between'/'notBetween' 时需要两个值，其它运算符需要一个值）

## Examples

公共四件套：`--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（前两者 XOR；`+batch-update` 本身不强制 sheet-id，子操作各自携带）。

### `+batch-update`

示例：

```bash
lark-cli sheets +batch-update --url "https://example.feishu.cn/sheets/shtXXX" --yes \
  --operations @ops.json

# ops.json （array<{tool_name, input}>）:
# [
#   {"tool_name": "modify_sheet_structure", "input": {"sheet_id":"...","operation":"insert","dimension":"row","start":10,"end":12}},
#   {"tool_name": "set_cell_range",         "input": {"sheet_id":"...","range":"A11:B12","cells":[[{"value":"a"},{"value":"b"}],[{"value":"c"},{"value":"d"}]]}}
# ]
```

### `+cells-batch-set-style`

多 range 应用同一组 style（服务端走 `batch_update` 原子事务）：

```bash
# 表头行 + 汇总行同时刷成蓝底白字
lark-cli sheets +cells-batch-set-style --url "..." \
  --ranges '["sheet1!A1:F1","sheet1!A30:F30"]' \
  --background-color "#1E5BC6" --font-color "#FFFFFF" --font-weight bold
```

### Validate / DryRun / Execute 约束

- `Validate`：`+batch-update` 的 `--operations` 必须合法 JSON，且为非空数组；逐个子操作 `tool_name` / `input` 字段必填校验；**禁止嵌套 batch_update**。`+cells-batch-set-style` 的 `--ranges` 必须 JSON 数组、每项带 sheet 前缀；样式 flag 至少一个非空（或带 `--border-styles`）。
- `DryRun`：按顺序输出每个子操作的目标 API + 请求 body 模板；首个失败则整批 fail-fast（不实际执行任何后续）。
- `Execute`：按声明顺序串行执行；任一子操作失败立即中断并回滚到该子操作前状态（具体回滚能力取决于子操作类型，沿用 MCP `batch_update` 的语义）。
