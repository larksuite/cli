# Lark Sheet Batch Update

## 写入边界 + 回读校验

`+batch-update` 把多次写入打包成单次请求，但每个子操作仍受编辑类任务硬性默认规则约束：

1. **目标 range 必须落在用户授权范围内**：除用户明示要修改的区域外，子操作禁止扩张到无关单元格 / 列 / Sheet。规划 range 时先确认每个子操作的边界。
2. **批次完成后必须回读校验**：整个 `+batch-update` 执行成功后，用 `+csv-get` 或 `+cells-get` 抽样回读受影响区域，至少校验 3-5 个代表性单元格（首 / 中 / 末），与 本地脚本 预先计算的预期值对照。
3. **预期条数前置断言**：涉及"批量填充 N 行"或"对 M 个区域分别写入"时，先把 N、M 硬编码进代码，回读后断言实际等于预期；不一致就再发一轮 `+batch-update` 补齐，禁止交付半成品。

## 使用场景

写入。批量执行多个写入工具操作。将多个工具调用合并为一次请求，按顺序依次执行。适合需要连续执行多个写入操作的场景（如先修改结构再写入数据）。注意：不支持嵌套 `+batch-update`。

**⚠️ 何时必须使用 `+batch-update`（硬性要求）**：
- 需要对**多个**不同区域执行 `+cells-{merge|unmerge}` 时（如按分组合并多列相同内容）
- 需要对**多个**不同区域执行 `+rows-resize / +cols-resize` 时（如统一调整多列列宽或多行行高）
- 需要先插入行列再写入数据时（`+dim-{insert|delete|hide|unhide|freeze|group|ungroup}` + `+cells-set`）
- 需要对多个区域执行不同写入操作时（多次 `+cells-set` + `+cells-clear` 等组合）

当同一工具需要对多个区域重复调用时，**必须**改用 `+batch-update` 合并为单次请求。逐个调用会快速耗尽工具调用轮次上限（60R），导致任务无法完成。

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+batch-update` | high-risk-write | 批量 |
| `+cells-batch-set-style` | write | 批量 |
| `+dropdown-update` | write | 对象 |
| `+dropdown-delete` | high-risk-write | 对象 |

## Flags

### `+batch-update`

_公共：URL/token（无 sheet 定位） · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--operations` | string + File + Stdin（复合 JSON） | required | JSON 数组：[{"shortcut":"+xxx-yyy","input":{...}}, ...]。shortcut 用 CLI 名；input 是该 shortcut 的入参集（不含 spreadsheet 定位），基础 flag 查 --help，复合 JSON flag 查 --print-schema --flag-name <flag>；禁手填 operation。默认严格事务，传 --continue-on-error 翻软批；不支持嵌套；按数组顺序串行执行 |
| `--continue-on-error` | bool | optional | 遇子操作失败时继续执行剩余操作；默认 false（首个失败即整批中断） |

### `+cells-batch-set-style`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--ranges` | string + File + Stdin（简单 JSON） | required | 目标范围 JSON 数组，每项必须带 sheet 前缀（如 `["sheet1!A1:B2","sheet1!D1:D10"]`）；所有 range 应用同一组 style |
| `--background-color` | string | optional | 背景颜色（十六进制，如 `#ffffff`） |
| `--font-color` | string | optional | 字体颜色（十六进制，如 `#000000`） |
| `--font-size` | float64 | optional | 字体大小（px，例：10、12、14） |
| `--font-style` | string | optional | 字体样式（可选值：`normal` / `italic`） |
| `--font-weight` | string | optional | 字重（可选值：`normal` / `bold`） |
| `--font-line` | string | optional | 字体线条样式（可选值：`none` / `underline` / `line-through`） |
| `--horizontal-alignment` | string | optional | 水平对齐（可选值：`left` / `center` / `right`） |
| `--vertical-alignment` | string | optional | 垂直对齐（可选值：`top` / `middle` / `bottom`） |
| `--word-wrap` | string | optional | 换行策略（可选值：`overflow` / `auto-wrap` / `word-clip`） |
| `--number-format` | string | optional | 数字格式（例：文本 `@`、数字 `0.00`、货币 `$#,##0.00`、日期 `mm/dd/yyyy`） |
| `--border-styles` | string + File + Stdin（复合 JSON） | optional | 边框配置 JSON（结构同 +cells-set-style） |

### `+dropdown-update`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--ranges` | string + File + Stdin（简单 JSON） | required | 目标范围 JSON 数组（如 `["sheet1!A2:A100"]`），每项必须带 sheet 前缀 |
| `--options` | string + File + Stdin（复合 JSON） | required | 选项 JSON 数组（如 `["opt1","opt2"]`） |
| `--colors` | string + File + Stdin（简单 JSON） | optional | 颜色数组（与 `--options` 等长） |
| `--multiple` | bool | optional | 启用多选 |
| `--highlight` | bool | optional | 选项配色 |

### `+dropdown-delete`

_公共：URL/token（无 sheet 定位） · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--ranges` | string + File + Stdin（简单 JSON） | required | 目标范围 JSON 数组（最多 100 个，每项必须带 sheet 前缀） |

## Schemas

> 复合 JSON flag（如 `--cells` / `--properties` / `--operations` / `--border-styles` / `--sort-keys`）的字段速查：只列顶层字段 + 一层嵌套结构。深层结构看 `## Examples` 段的真实示例；要拿完整 JSON Schema 跑 `lark-cli sheets <shortcut> --print-schema --flag-name <name>`。先 `--print-schema`（不带 `--flag-name`）会列出该 shortcut 所有可查询的 flag。

### `+batch-update` `--operations`

_要批量执行的 CLI shortcut 操作列表，按声明顺序串行执行；任一失败立即中断_

**数组项**（类型 object）：
- `shortcut` (enum) — CLI shortcut 名（不是底层 MCP tool 名） [+cells-set / +cells-set-style / +cells-clear / +cells-merge / +cells-unmerge / +cells-replace / +csv-put / +dropdown-set / …共 50 项]
- `input` (object) — 该 shortcut 的入参集（不含 spreadsheet 定位）；基础 flag 跑 `lark-cli sheets <shortcut> --help…

### `+cells-batch-set-style` `--border-styles`

_单元格边框配置，含 top/bottom/left/right 四个方向，每个方向的结构相同（见 top）_

**顶层字段**：
- `bottom` (object?) { color?: string, style?: enum, weight?: enum }
- `left` (object?) { color?: string, style?: enum, weight?: enum }
- `right` (object?) { color?: string, style?: enum, weight?: enum }
- `top` (object?) { color?: string, style?: enum, weight?: enum }

### `+dropdown-update` `--options`

_列表选项（type='list' 时必填）_

**数组项**（类型 string）：
- 标量：string

## Examples

公共四件套：`--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（前两者 XOR；`+batch-update` 本身不强制 sheet-id，子操作各自携带）。

### `+batch-update`

示例：

```bash
lark-cli sheets +batch-update --url "https://example.feishu.cn/sheets/shtXXX" --yes \
  --operations @ops.json

# ops.json （array<{shortcut, input}>，shortcut 用 CLI 名）:
# [
#   {"shortcut": "+dim-insert", "input": {"sheet_id":"...","dimension":"row","start":10,"end":12}},
#   {"shortcut": "+cells-set",  "input": {"sheet_id":"...","range":"A11:B12","cells":[[{"value":"a"},{"value":"b"}],[{"value":"c"},{"value":"d"}]]}}
# ]
```

> **常见组合：插列 + 写表头 + 整列回填**——一次原子提交，不要拆成 N 次独立调用。批量回填同一列 **只需一次** `+cells-set`（range 写整列范围、cells 写 N×1 矩阵），不需要逐行循环。
>
> ```jsonc
> // 在 C 列前插入新列 → 写表头 C1 → 回填 C2:C100 共 99 行
> [
>   {"shortcut": "+dim-insert",
>    "input": {"sheet_id": "...", "dimension": "column", "start": 3, "end": 4}},
>   {"shortcut": "+cells-set",
>    "input": {"sheet_id": "...", "range": "C1:C100",
>              "cells": [[{"value":"score"}], [{"value":95}], [{"value":87}], /* ... 97 more rows ... */ ]}}
> ]
> ```

### `+cells-batch-set-style`

多 range 应用同一组 style（服务端走 `+batch-update` 原子事务）：

```bash
# 表头行 + 汇总行同时刷成蓝底白字
lark-cli sheets +cells-batch-set-style --url "..." \
  --ranges '["sheet1!A1:F1","sheet1!A30:F30"]' \
  --background-color "#1E5BC6" --font-color "#FFFFFF" --font-weight bold
```

### Validate / DryRun / Execute 约束

- `Validate`：`+batch-update` 的 `--operations` 必须合法 JSON，且为非空数组；逐个子操作 `shortcut` / `input` 字段必填校验；**禁止嵌套 `+batch-update`**。`+cells-batch-set-style` 的 `--ranges` 必须 JSON 数组、每项带 sheet 前缀；样式 flag 至少一个非空（或带 `--border-styles`）。
- `DryRun`：按顺序输出每个子操作的目标 API + 请求 body 模板；首个失败则整批 fail-fast（不实际执行任何后续）。
- `Execute`：按声明顺序串行执行；任一子操作失败立即中断并回滚到该子操作前状态（具体回滚能力取决于子操作类型，沿用 MCP `+batch-update` 的语义）。
