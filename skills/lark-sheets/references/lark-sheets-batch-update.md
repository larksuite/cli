# Lark Sheet Batch Update

## 写入边界 + 回读校验

`+batch-update` 把多次写入打包成单次请求，但每个子操作仍受编辑类任务硬性默认规则约束：

1. **目标 range 必须落在用户授权范围内**：除用户明示要修改的区域外，子操作禁止扩张到无关单元格 / 列 / Sheet。规划 range 时先确认每个子操作的边界。
2. **批次完成后必须回读校验**：整个 `+batch-update` 执行成功后，用 `+csv-get` 或 `+cells-get` 抽样回读受影响区域，至少校验 3-5 个代表性单元格（首 / 中 / 末），与本地脚本预先计算的预期值对照。
3. **预期条数前置断言**：涉及"批量填充 N 行"或"对 M 个区域分别写入"时，先把 N、M 硬编码进代码，回读后断言实际等于预期；不一致就再发一轮 `+batch-update` 补齐，禁止交付半成品。

若本次 `+batch-update` 的任一子操作写入了公式、复制了公式模板、或导入了含公式的数据块，**回读校验之后还必须继续执行 `+formula-verify`**。`+batch-update` 的原子提交只保证“写入动作都执行了”，不保证整批公式运行结果 zero-error。

## 使用场景

写入。批量执行多个写入工具操作。将多个工具调用合并为一次请求，按顺序依次执行。适合需要连续执行多个写入操作的场景（如先修改结构再写入数据）。注意：不支持嵌套 `+batch-update`。

**不可放进 `--operations` 的写 shortcut**（`shortcut` 枚举不含它们，强行写入会被校验拒）：`+cells-set-image`（需本地上传图片）、`+dropdown-update` / `+dropdown-delete` / `+cells-batch-set-style` / `+cells-batch-clear`（自身已是批量入口，不可再嵌套）、`+dim-move`。这些操作需在 `+batch-update` 之外单独调用。

**⚠️ 何时必须使用 `+batch-update`（硬性要求）**：
- 需要对**多个**不同区域执行 `+cells-{merge|unmerge}` 时（如按分组合并多列相同内容）
- 需要先插入行列再写入数据时（`+dim-{insert|delete|hide|unhide|freeze|group|ungroup}` + `+cells-set`）
- 需要对多个区域执行不同写入操作时（多次 `+cells-set` + `+cells-clear` 等组合）

**行高列宽批量不走这里**：多行 / 多列不同尺寸直接用 `+rows-resize --heights` / `+cols-resize --widths` 的 map 形态（如 `--widths '{"A":100,"C:E":120}'`，见 `lark-sheets-range-operations`），一次调用原子完成；map 形态不可作为 `--operations` 子操作嵌入（子操作里仍可用单区间形态 `range` + `height`/`width`）。

当同一工具需要对多个区域重复调用时，**必须**改用 `+batch-update` 合并为单次请求——`+batch-update` 是原子提交（要么全成功要么整批回滚）；逐个调用非原子，中途失败会留下半成品。

**公式相关批处理的默认闭环**：
- 写前：先读 `lark-sheets-formula-translation`，把公式改写成飞书可执行语义。
- 写时：用 `+batch-update` 一次性完成插行/写公式/复制模板等原子动作。
- 写后：抽样回读之外，继续跑 `lark-sheets-formula-verify`，直到 `+formula-verify` 返回 `status='success'`。

**`+dropdown-update` 的选项模式（`--options` / `--source-range` 二选一）+ 配色规则**（`--colors` 长度可短不能长、必须配 `--highlight=true` 才生效、不传按内置 10 色色板循环补色）见 [`lark-sheets-write-cells`](./lark-sheets-write-cells.md) 的「Dropdown 选项 + 配色」节，本文不重复。`+dropdown-delete` 不涉及这些 flag。

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+batch-update` | high-risk-write | 批量 |
| `+cells-batch-set-style` | write | 批量 |
| `+dropdown-update` | write | 对象 |
| `+dropdown-delete` | high-risk-write | 对象 |
| `+cells-batch-clear` | high-risk-write | 批量 |

## Flags

### `+batch-update`

_公共：URL/token（无 sheet 定位） · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--operations` | string + File + Stdin（复合 JSON） | required | JSON 数组：[{"shortcut":"+xxx-yyy","input":{...}}, ...]。shortcut 用 CLI 名；input 是该 shortcut 的入参集——含子表定位 sheet_id（或 sheet_name），但不含 spreadsheet token/url（后者只在顶层 --url/--spreadsheet-token 给一次；+batch-update 顶层没有 --sheet-id）；input 的键是该 shortcut 的 flag 展平成 JSON（如 "range":"A11:B12"），不是再套一层嵌套。基础 flag 查 --help，复合 JSON flag 查 --print-schema --flag-name <flag>；不要手填 operation 字段（由 CLI 按 shortcut 自动注入）。默认严格事务（首个失败即整批中断），传 --continue-on-error 切换为软批量（遇失败仍继续）；不支持嵌套；按数组顺序串行执行 |
| `--continue-on-error` | bool | optional | 遇子操作失败时继续执行剩余操作；默认 false（首个失败即整批中断） |

### `+cells-batch-set-style`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--ranges` | string + File + Stdin（简单 JSON） | required | 目标范围 JSON 数组（最多 100 个），每项必须带 sheet 前缀（如 `["Sheet1!A1:B2","Sheet2!D1:D10"]`，前缀裸写不加引号）；前缀必须与 sheet 真实显示名完全一致（含大小写），不接受 sheet reference_id；支持跨 sheet；所有 range 应用同一组 style |
| `--background-color` | string | optional | 背景颜色（十六进制，如 `#ffffff`） |
| `--font-color` | string | optional | 字体颜色（十六进制，如 `#000000`） |
| `--font-family` | string | optional | 字体名称（如 `Arial`、`微软雅黑`） |
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
| `--ranges` | string + File + Stdin（简单 JSON） | required | 目标范围 JSON 数组（最多 100 个，如 `["Sheet1!A2:A100","Sheet1!C2:C100"]`，前缀裸写不加引号），每项必须带 sheet 前缀；前缀必须与 sheet 真实显示名完全一致（含大小写），不接受 sheet reference_id |
| `--options` | string + File + Stdin（复合 JSON） | xor | 下拉选项 JSON 数组，例如 `["opt1","opt2"]`。服务端不限制选项数量，也不限制单个选项长度；含逗号的选项可以接受（写入时会自动转义）。大量选项建议改用 `--source-range`。 |
| `--colors` | string + File + Stdin（简单 JSON） | optional | 下拉胶囊背景色，RGB hex 数组（如 `["#1FB6C1","#F006C2"]`）。长度可短不可长——超长 Validate 拦截（`--colors length (N) must not exceed dropdown source size (M)`），未指定项按内置 10 色色板循环补色。**单独传即生效**；`--highlight=false` 时被忽略。 |
| `--multiple` | bool | optional | 启用多选 |
| `--highlight` | bool | optional | 下拉胶囊背景色高亮开关。**不传 = 开**（按内置 10 色色板循环上色）；`--highlight=false` 关闭得到纯白下拉。配色用 `--colors` 覆盖。 |
| `--source-range` | string | xor | listFromRange 模式的下拉源 range，A1 表示法 + sheet 前缀（如 `'Sheet1'!T1:T3`）。映射到 server `data_validation.range`，搭配 server `data_validation.type='listFromRange'` 自动生效。跟 `--options` 二选一：传 `--options` 走 inline 列表（type=list），传本 flag 走 range 引用（type=listFromRange）。`--colors` 长度规则不变（≤ 源 range 单元格数），`--highlight` / `--multiple` 行为相同。当 `--highlight` 开启且 source 覆盖单元格数超过 2000 时，服务端会将该下拉判为 option-error（这是不支持的组合）；CLI 会向 stderr 输出 warning。如需取消，传 `--highlight=false`。 |

### `+dropdown-delete`

_公共：URL/token（无 sheet 定位） · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--ranges` | string + File + Stdin（简单 JSON） | required | 目标范围 JSON 数组（最多 100 个，如 `["Sheet1!E2:E6"]`，前缀裸写不加引号），每项必须带 sheet 前缀；前缀必须与 sheet 真实显示名完全一致（含大小写），不接受 sheet reference_id |

### `+cells-batch-clear`

_公共：URL/token（无 sheet 定位） · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--ranges` | string + File + Stdin（简单 JSON） | required | 目标范围 JSON 数组（最多 100 个），每项必须带 sheet 前缀（如 `["Sheet1!A2:Z1000","Sheet2!A2:Z1000"]`，前缀裸写不加引号）；前缀必须与 sheet 真实显示名完全一致（含大小写），不接受 sheet reference_id；支持跨 sheet；对所有 range 执行同一 scope 的清除 |
| `--scope` | string | optional | 清除范围 enum：`content`（默认，仅清内容）/ `formats`（仅清格式）/ `all`（清内容 + 格式）（可选值：`content` / `formats` / `all`） |

## Schemas

> 复合 JSON flag 字段速查（只列顶层 + 一层嵌套）。深层结构看下方 `## Examples`，或用 `--print-schema` 读完整 JSON Schema（用法见 SKILL.md「公共 flag 速查」与「Agent 使用提示」）。

### `+batch-update` `--operations`

_要批量执行的 CLI shortcut 操作列表，按声明顺序串行执行；任一失败立即中断_

**数组项**（类型 object）：
- `shortcut` (enum) — CLI shortcut 名（不是底层 MCP tool 名） [+cells-set / +cells-set-style / +cells-clear / +cells-merge / +cells-unmerge / +cells-replace / +csv-put / +dropdown-set / +dim-insert / +dim-delete / +dim-hide / +dim-unhide / +dim-freeze / +dim-group / +dim-ungroup / +rows-resize / +cols-resize / +range-move / +range-copy / +range-fill / +range-sort / +sheet-create / +sheet-delete / +sheet-rename / +sheet-move / +sheet-copy / +sheet-hide / +sheet-unhide / +sheet-set-tab-color / +sheet-show-gridline / +sheet-hide-gridline / +chart-create / +chart-update / +chart-delete / +pivot-create / +pivot-update / +pivot-delete / +cond-format-create / +cond-format-update / +cond-format-delete / +filter-create / +filter-update / +filter-delete / +filter-view-create / +filter-view-update / +filter-view-delete / +sparkline-create / +sparkline-update / +sparkline-delete / +float-image-create / +float-image-update / +float-image-delete]
- `input` (object) — 该 shortcut 的入参集——含子表定位 sheet_id（或 sheet_name），但不含 spreadsheet token/url（后者只在顶层 …

### `+cells-batch-set-style` `--border-styles`

_单元格边框配置，含 top/bottom/left/right 四个方向，每个方向的结构相同（见 top）_

**顶层字段**：
- `top` (object?) { style?: enum, weight?: enum, color?: string }
- `bottom` (object?) { style?: enum, weight?: enum, color?: string }
- `left` (object?) { style?: enum, weight?: enum, color?: string }
- `right` (object?) { style?: enum, weight?: enum, color?: string }

### `+dropdown-update` `--options`

_列表选项_

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
#   {"shortcut": "+dim-insert", "input": {"sheet_id":"...","position":10,"count":3}},
#   {"shortcut": "+cells-set",  "input": {"sheet_id":"...","range":"A11:B12","cells":[[{"value":"a"},{"value":"b"}],[{"value":"c"},{"value":"d"}]]}}
# ]
```

> ⚠️ **子操作定位规则**：
> - spreadsheet 定位（`--url` / `--spreadsheet-token`）**只在顶层给一次**；`+batch-update` 顶层**没有** `--sheet-id` / `--sheet-name`，在顶层传不生效。
> - **每个子操作的子表定位 `sheet_id`（或 `sheet_name`）写进它自己的 `input`**（见上方 ops.json 每个 item）。
> - `input` 的键是该 shortcut 的 flag **展平**成 JSON（`"range":"A11:B12"`、`"position":11`），不要把整组 `--operations` 再套一层嵌套 JSON。

> **常见组合：插列 + 写表头 + 整列回填**——一次原子提交，不要拆成 N 次独立调用。批量回填同一列 **只需一次** `+cells-set`（range 写整列范围、cells 写 N×1 矩阵），不需要逐行循环。
>
> ```jsonc
> // 在 C 列前插入新列 → 写表头 C1 → 回填 C2:C100 共 99 行
> [
>   {"shortcut": "+dim-insert",
>    "input": {"sheet_name": "Sheet1", "position": "C", "count": 1}},
>   {"shortcut": "+cells-set",
>    "input": {"sheet_name": "Sheet1", "range": "C1:C100",
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

### `+cells-batch-clear`

多 range 一次性清除（服务端走 `+batch-update` 原子事务）；`--scope` 同 `+cells-clear`（`content` / `formats` / `all`，默认 `content`），`high-risk-write` 强制 `--yes`：

```bash
# dry-run 先看清除范围
lark-cli sheets +cells-batch-clear --url "..." \
  --ranges '["sheet1!A2:Z1000","sheet2!A2:Z1000"]' --scope all --dry-run
# 执行
lark-cli sheets +cells-batch-clear --url "..." \
  --ranges '["sheet1!A2:Z1000","sheet2!A2:Z1000"]' --scope all --yes
```

### Validate / DryRun / Execute 约束

- `Validate`：`+batch-update` 的 `--operations` 必须合法 JSON，且为非空数组；逐个子操作 `shortcut` / `input` 字段必填校验；**禁止嵌套 `+batch-update`**。`+cells-batch-set-style` 的 `--ranges` 必须 JSON 数组、每项带 sheet 前缀；样式 flag 至少一个非空（或带 `--border-styles`）。`+cells-batch-clear` 的 `--ranges` 同样必须 JSON 数组、每项带 sheet 前缀，`high-risk-write` 强制 `--yes` 或 `--dry-run`（`--scope` 默认 `content`）。
- `DryRun`：按顺序输出每个子操作的目标 API + 请求 body 模板；首个失败则整批 fail-fast（不实际执行任何后续）。
- `Execute`：按声明顺序串行执行；任一子操作失败立即中断并回滚到该子操作前状态（具体回滚能力取决于子操作类型，沿用 `+batch-update` 的语义）。
