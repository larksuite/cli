# Lark Sheet Batch Update

## 写入边界 + 回读校验

`+batch-update` 把多次写入打包成单次请求，但每个子操作仍应按编辑类任务的范围和回读建议处理：

1. **目标 range 应落在用户授权范围内**：除用户明示要修改的区域外，子操作避免扩张到无关单元格 / 列 / Sheet。规划 range 时先确认每个子操作的边界。
2. **批次完成后建议回读校验**：整个 `+batch-update` 执行成功后，用 `+csv-get` 或 `+cells-get` 抽样回读受影响区域，至少校验 3-5 个代表性单元格（首 / 中 / 末），与本地脚本预先计算的预期值对照。
3. **预期条数前置断言**：涉及"批量填充 N 行"或"对 M 个区域分别写入"时，建议先把 N、M 硬编码进代码，回读后比较实际与预期；不一致就优先再发一轮 `+batch-update` 补齐，补不齐则在交付说明里列出缺口。
4. **三条工具硬约束**：`--yes` 必带（high-risk-write，不带退出码 10）；单次 ≤100 条 operations，超出按批拆分；`+cells-batch-set-style` / `+cells-batch-clear` 等批量类 shortcut 不可嵌入 operations（它们本身就是批量原子操作，直接顶层调用）。

若本次 `+batch-update` 的任一子操作写入了公式、复制了公式模板、或导入了含公式的数据块，回读校验之外可继续执行 `+formula-verify` 做诊断。`+batch-update` 只保证"写入动作按序执行了"，不保证整批公式运行结果 zero-error。

## 使用场景

写入。把**跨类型、有顺序依赖**的多个写入操作合并为一次请求按序执行（如插列 → 写表头 → 回填数据）。注意：不支持嵌套 `+batch-update`。

**先分流再动手（按操作组合选入口）**：美化收尾（样式 / 合并 / 行高列宽 / 冻结的任意组合）→ 一次 `+styles-put`（声明式规格，见 `lark-sheets-styles-put`），不要拼 `--operations` 子操作数组；**同一个写操作**打多个区域 → 用该命令自身的复数形态（`+cells-set --writes` / `+cells-batch-clear` / `+dim-delete --ranges` / resize 的 map 形态等）；只有跨类型、有顺序依赖的操作链才用本命令。

**⚠️ 何时优先使用 `+batch-update`**：
- 需要先插入行列再写入数据时（`+dim-{insert|delete|hide|unhide|freeze|group|ungroup}` + `+cells-set`）
- 需要对多个区域执行**不同类型**的写入操作时（如 `+cells-set` + `+cells-clear` 组合）。同一个写操作打多区域用该命令自身的复数形态、多区域 merge 用 `+styles-put` 的 `cell_merges`、大范围 unmerge 直接单次调用——均见上方分流，不进本命令

**多个互不依赖的图表任务优先使用 `+batch-chart-create` / `+batch-chart-update`**；只有图表与其它写入存在同一批次顺序依赖时，才把图表 shortcut 放进 `+batch-update`。

**不可放进 `--operations` 的写 shortcut**（`shortcut` 枚举不含它们，强行写入会被校验拒）：`+cells-set-image`（需本地上传图片）、`+styles-put` / `+dropdown-update` / `+dropdown-delete` / `+cells-batch-clear`（自身已是批量入口，不可再嵌套）、`+dim-move`。这些操作需在 `+batch-update` 之外单独调用。

**行高列宽批量不走这里**：多行 / 多列不同尺寸用 `+styles-put` 的 `row_sizes` / `col_sizes`（可与样式同批），或 `+rows-resize --heights` / `+cols-resize --widths` 的 map 形态（见 `lark-sheets-range-operations`）；map 形态不可作为 `--operations` 子操作嵌入（子操作里仍可用单区间形态 `range` + `height`/`width`）。

**执行语义（fail-fast；失败后哪些已生效取决于批次构成）**：默认首个失败的子操作即中断剩余操作。此前的子操作**是否已落盘不统一**：纯单元格 / 行列结构类写入在提交前只累计在内存，失败时整体不落盘（等效回滚）；而图表 / 透视表等对象类子操作执行时会**先把此前累计的写入提交落盘再创建对象**——批次含这类子操作时，失败前完成的部分（含其之前的普通写入）已实际生效、无法回滚。因此失败后**不要假设"全部回滚"或"全部保留"**：先看返回 `results` 里各子操作的状态，再回读现状（行列数 / 目标格 / `+chart-list` 等对象清单）确认已生效集合，只补发未生效部分——盲目整批重发会重复应用已生效操作（如插行 / 建图），盲目只发失败尾可能写到未生效的旧结构上。传 `--continue-on-error` 则遇失败仍继续执行剩余操作，已成功部分保留（返回 "N succeeded, M failed"）。

**公式相关批处理的建议诊断**：
- 写前：先读 `lark-sheets-formula-translation`，把公式改写成飞书可执行语义。
- 写时：用 `+batch-update` 一次性完成插行/写公式/复制模板等成套动作。
- 写后：抽样回读之外，可继续跑 `lark-sheets-formula-verify` 做一次诊断。

**`+dropdown-update` 的选项模式（`--options` / `--source-range` 二选一）+ 配色规则**（`--colors` 长度可短不能长、必须配 `--highlight=true` 才生效、不传按内置 10 色色板循环补色）见 [`lark-sheets-write-cells`](./lark-sheets-write-cells.md) 的「Dropdown 选项 + 配色」节，本文不重复。`+dropdown-delete` 不涉及这些 flag。

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+batch-update` | high-risk-write | 批量 |
| `+batch-chart-create` | write | 批量 |
| `+batch-chart-update` | write | 批量 |
| `+dropdown-update` | write | 对象 |
| `+dropdown-delete` | high-risk-write | 对象 |
| `+cells-batch-clear` | high-risk-write | 批量 |

## Flags

### `+batch-update`

_公共：URL/token（无 sheet 定位） · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--operations` | string + File + Stdin（复合 JSON） | required | JSON 数组：[{"shortcut":"+xxx-yyy","input":{...}}, ...]。shortcut 用 CLI 名；input 是该 shortcut 的入参集——含子表定位 sheet_id（或 sheet_name），但不含 spreadsheet token/url（后者只在顶层 --url/--spreadsheet-token 给一次；+batch-update 顶层没有 --sheet-id）；input 的键是该 shortcut 的 flag 展平成 JSON（如 "range":"A11:B12"），不是再套一层嵌套。基础 flag 查 --help，复合 JSON flag 查 --print-schema --flag-name <flag>；不要手填 operation 字段（由 CLI 按 shortcut 自动注入）。默认 fail-fast：首个失败即中断剩余操作；此前子操作是否已落盘**不统一**（纯单元格/结构写入失败时整体不落盘，图表/透视表等对象子操作会提前把累计写入落盘且自身无法回滚），失败后不要假设全回滚或全保留——先看 results 再回读现状确认已生效集合，只补发未生效部分；传 --continue-on-error 遇失败仍继续、已成功部分保留；不支持嵌套；按数组顺序串行执行 |
| `--continue-on-error` | bool | optional | 遇子操作失败时继续执行剩余操作；默认 false（首个失败即整批中断） |

### `+batch-chart-create`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--operations` | string + File + Stdin（复合 JSON） | required | 图表创建操作 JSON 数组；每项直接填写 `+chart-create-basic` 的 flag 和目标 sheet 定位，不要再套 `shortcut` / `input`。CLI 内部固定使用 `+chart-create-basic`。默认允许部分失败，成功图表保留，只重试失败项 |
| `--continue-on-error` | bool | optional | 单个图表失败后是否继续；默认 true |

### `+batch-chart-update`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--operations` | string + File + Stdin（复合 JSON） | required | 图表更新操作 JSON 数组；每项使用 `+chart-config-update` 或 `+chart-data-update`，input 传对应命令的 flag 集合和目标 sheet 定位。CLI 会先读取各图表当前快照，再生成 partial properties；默认允许部分失败 |
| `--continue-on-error` | bool | optional | 单个图表失败后是否继续；默认 true |

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
- `shortcut` (enum) — CLI shortcut 名（不是底层 MCP tool 名） [+cells-set / +cells-set-style / +cells-clear / +cells-merge / +cells-unmerge / +cells-replace / +csv-put / +dropdown-set / +dim-insert / +dim-delete / +dim-hide / +dim-unhide / +dim-freeze / +dim-group / +dim-ungroup / +rows-resize / +cols-resize / +range-move / +range-copy / +range-fill / +range-sort / +sheet-create / +sheet-delete / +sheet-rename / +sheet-move / +sheet-copy / +sheet-hide / +sheet-unhide / +sheet-set-tab-color / +sheet-show-gridline / +sheet-hide-gridline / +chart-create / +chart-update / +chart-delete / +chart-create-basic / +chart-config-update / +chart-data-update / +pivot-create / +pivot-update / +pivot-delete / +cond-format-create / +cond-format-update / +cond-format-delete / +filter-create / +filter-update / +filter-delete / +filter-view-create / +filter-view-update / +filter-view-delete / +sparkline-create / +sparkline-update / +sparkline-delete / +float-image-create / +float-image-update / +float-image-delete]
- `input` (object) — 该 shortcut 的入参集——含子表定位 sheet_id（或 sheet_name）

### `+batch-chart-create` `--operations`


**数组项**（类型 object）：
- `sheet_id` (string?) — 目标子表 ID；与 sheet_name 二选一
- `sheet_name` (string?) — 目标子表名；与 sheet_id 二选一
- `chart_type` (enum) [column / bar / line / area / pie / scatter / combo / radar / bubble / waterfall / pareto]
- `data_range` (string)
- `header_range` (string?)
- `data_direction` (enum?) [row / column]
- `dim1_index` (integer?)
- `dim2_indexes` (oneOf?)
- `series_types` (oneOf?)
- `series_y_axes` (oneOf?)
- `key_index` (integer?) — 气泡图标识/名称维度的 1-based 索引；默认 1
- `x_index` (integer?) — 气泡图 X 值维度的 1-based 索引；与 y_index 同时提供
- `y_index` (integer?) — 气泡图 Y 值维度的 1-based 索引；与 x_index 同时提供
- `group_index` (integer?) — 气泡图可选分组维度的 1-based 索引
- `size_index` (integer?) — 气泡图可选气泡大小维度的 1-based 索引
- `title` (string?)
- `anchor_cell` (string?)

### `+batch-chart-update` `--operations`


**数组项**（类型 object）：
- `shortcut` (enum) [+chart-config-update / +chart-data-update]
- `input` (object) — 对应图表更新 shortcut 的 flag 集合；包含 sheet_id 或 sheet_name，不包含 spreadsheet token/url

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

> **常见组合：插列 + 写表头 + 整列回填**——一次批量提交，不要拆成 N 次独立调用。批量回填同一列 **只需一次** `+cells-set`（range 写整列范围、cells 写 N×1 矩阵），不需要逐行循环。
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

> **多图表组合**：先完成全部辅助数据，再把每张图的输入放进 `+batch-chart-create`；每项同时记录精确表头范围、数据方向和预期系列数。批次完成后，每个受影响的 sheet 各调用一次 `+chart-list`。已有图表的批量修正改用 `+batch-chart-update`。
>
> ```json
> [
>   {"sheet_name":"Sheet1","chart_type":"column","data_range":"'Sheet1'!A1:C10","title":"分类对比","anchor_cell":"F2"},
>   {"sheet_name":"Sheet1","chart_type":"line","data_range":"'Sheet1'!E1:G10","title":"趋势变化","anchor_cell":"F18"}
> ]
> ```
>
> ```bash
> lark-cli sheets +batch-chart-create --url "..." --operations @ops.json
> ```

### `+cells-batch-clear`

多 range 一次性清除（服务端走 `+batch-update` 批量提交，fail-fast，失败处置见下方「执行语义」）；`--scope` 同 `+cells-clear`（`content` / `formats` / `all`，默认 `content`），`high-risk-write` 强制 `--yes`：

```bash
# dry-run 先看清除范围
lark-cli sheets +cells-batch-clear --url "..." \
  --ranges '["sheet1!A2:Z1000","sheet2!A2:Z1000"]' --scope all --dry-run
# 执行
lark-cli sheets +cells-batch-clear --url "..." \
  --ranges '["sheet1!A2:Z1000","sheet2!A2:Z1000"]' --scope all --yes
```

### Validate / DryRun / Execute 约束

- `Validate`：`+batch-update` 的 `--operations` 必须合法 JSON，且为非空数组；逐个子操作 `shortcut` / `input` 字段必填校验，input 键必须在该 shortcut 的 flag 词汇表内（未知键报错并提示最近似键与完整键契约）；**校验错误聚合上报**——所有子操作的首错一次性返回，全部修完再重发一次即可；**禁止嵌套 `+batch-update`**。`+cells-batch-clear` 的 `--ranges` 必须 JSON 数组、每项带 sheet 前缀，`high-risk-write` 强制 `--yes` 或 `--dry-run`（`--scope` 默认 `content`）。
- `DryRun`：按顺序输出每个子操作的目标 API + 请求 body 模板，不发起调用。
- `Execute`：按声明顺序串行执行；默认 fail-fast——任一子操作失败即中断剩余操作。失败后哪些子操作已生效**见上方「执行语义」**（取决于批次构成，不做统一假设），按报错中的子操作状态回读确认后再补发。
