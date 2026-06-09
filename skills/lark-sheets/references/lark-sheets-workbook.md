# Lark Sheet Workbook

## Sheet 结构变更保守化（编辑类任务必做）

`+sheet-{create|delete|rename|move|copy|hide|unhide|set-tab-color}` 会改变原表的物理结构，是高副作用动作。执行前必须遵守：

1. **删除 / 重命名 / 隐藏 / 移动原 Sheet 需用户明示**：除非用户明示要这些操作，**禁止**擅自对**已存在**的 Sheet 执行 delete / rename / hide / move。新建 Sheet 是允许的（用于承载中间结果或透视表 / 图表对象），但应优先在原表右侧加列；只有当中间结果数量较大或会与原数据混淆时，才新建空白 Sheet（同 R1）。
2. **Sheet 级操作前先列清单**：调用 `+sheet-{create|delete|rename|move|copy|hide|unhide|set-tab-color}` 之前，必须先调用 `+workbook-info`，把"当前所有 Sheet 名 + 可见性 + 行列数"列出来，再决定是否操作。禁止跳过列清单直接 create / delete / rename。
3. **删除 / 重命名前向用户确认**：删除是不可逆的，重命名会让其他公式 / 透视表 / 图表的数据源失效——执行前必须在回复里确认"将删除 / 改名 X，影响 Y 个引用"。

## 使用场景

读写。管理工作簿结构。本 reference 覆盖 14 个 shortcut：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 查看工作簿结构 | `+workbook-info` | 获取子表列表、名称、行列数、冻结位置等元数据 |
| 变更工作簿结构 | `+sheet-{create|delete|rename|move|copy|hide|unhide|set-tab-color}` | 新建/删除/移动/重命名/复制/隐藏子表、修改标签颜色 |

注意：

- 如果用户请求包含多个动作，例如"先重命名，再新建工作表"，请按顺序发起多次调用，覆盖全部动作
- `create` 时若用户指定了工作表名称，应显式传入 `sheet_name`；不要省略后依赖默认命名
- 若 `+workbook-info` 返回包含 `warning_message`，说明部分 `sheet_id` 已失效（被删除/改名或输入错误），应停止复用这些 id，重新不带 `sheet_ids` 全量获取结构后再继续操作

**常见配置错误（必须注意）**：
- **获取结构是第一步**：任何表格操作前必须先调用 `+workbook-info`，不要跳过直接操作。返回的行列数、子表列表是后续所有操作的基础
- **sheet_id 不要写错**：从 `+workbook-info` 返回值中精确获取 `sheet_id`，不要手动拼写或从 URL 中猜测
- **优先使用 `sheet_id`**：虽然飞书表格不允许子表重名，但 `sheet_id` 是稳定标识符，跨多轮操作时不会因用户中途重命名而失效

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+workbook-info` | read | 工作簿 |
| `+sheet-create` | write | 工作簿 |
| `+sheet-delete` | high-risk-write | 工作簿 |
| `+sheet-rename` | write | 工作簿 |
| `+sheet-move` | write | 工作簿 |
| `+sheet-copy` | write | 工作簿 |
| `+sheet-hide` | write | 工作簿 |
| `+sheet-unhide` | write | 工作簿 |
| `+sheet-set-tab-color` | write | 工作簿 |
| `+sheet-hide-gridline` | write | 工作簿 |
| `+sheet-show-gridline` | write | 工作簿 |
| `+workbook-create` | write | 工作簿 |
| `+workbook-export` | read | 工作簿 |
| `+workbook-import` | write | 工作簿 |

## Flags

### `+workbook-info`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

_仅含公共 / 系统 flag。_

### `+sheet-create`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--title` | string | required | 新工作表名称 |
| `--index` | int | optional | 插入位置（0-based）；省略时附加到末尾 |
| `--row-count` | int | optional | 初始行数（默认 200，上限 50000） |
| `--col-count` | int | optional | 初始列数（默认 20，上限 200） |

### `+sheet-delete`

_公共四件套 · 系统：`--yes`、`--dry-run`_

_仅含公共 / 系统 flag。_

### `+sheet-rename`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--title` | string | required | 新名称 |

### `+sheet-move`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--index` | int | required | 目标位置（0-based） |
| `--source-index` | int | optional | 源位置（0-based）；可选，未传时由 CLI runtime 根据 `--sheet-id` / `--sheet-name` 当前在工作簿中的 index 自动派生 |

### `+sheet-copy`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--title` | string | optional | 副本名称；省略时由服务端生成 |
| `--index` | int | optional | 副本插入位置（0-based）；省略时附加到末尾 |

### `+sheet-hide`

_公共四件套 · 系统：`--dry-run`_

_仅含公共 / 系统 flag。_

### `+sheet-unhide`

_公共四件套 · 系统：`--dry-run`_

_仅含公共 / 系统 flag。_

### `+sheet-set-tab-color`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--color` | string | required | Hex 色值如 `#FF0000`，传空 `""` 清除 |

### `+sheet-hide-gridline`

_公共四件套 · 系统：`--dry-run`_

_仅含公共 / 系统 flag。_

### `+sheet-show-gridline`

_公共四件套 · 系统：`--dry-run`_

_仅含公共 / 系统 flag。_

### `+workbook-create`

_系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--title` | string | required | 新 spreadsheet 标题 |
| `--folder-token` | string | optional | 目标文件夹 token；省略时放在云空间根目录 |
| `--headers` | string + File + Stdin（简单 JSON） | optional | 表头行 JSON 数组：`["列A","列B"]` |
| `--values` | string + File + Stdin（简单 JSON） | optional | 初始数据 JSON 二维数组：`[["alice",95]]` |
| `--sheets` | string + File + Stdin（复合 JSON） | optional | 建表后写入的 typed 表格协议 JSON（同 +table-put）：顶层 sheets 数组，每项 {name, start_cell?, mode?, header?, allow_overwrite?, columns:[{name,type,format?}], rows:[[...]]}；type 为 string/number/date/bool。与 --headers/--values 互斥；新表默认子表复用为第一个子表，日期/数字类型保真。 |
| `--header-style` | bool | optional | 把 typed 表头行加粗（仅 --sheets 时生效，默认 true） |

### `+workbook-export`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--file-extension` | string | optional | 导出文件格式；`csv` 模式必须配 `--sheet-id`（可选值：`xlsx` / `csv`）（默认 `xlsx`） |
| `--sheet-id` | string | optional | 仅 csv 模式必填：指定要导出哪张 sheet 为 CSV。这是 `+workbook-export` 专有 flag，与公共四件套的 sheet 定位无关（本 shortcut 不接受公共 sheet 定位） |
| `--output-path` | string | optional | 本地保存路径；省略时只触发导出不下载 |

### `+workbook-import`

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--file` | string | required | 本地文件路径（.xlsx / .xls / .csv） |
| `--folder-token` | string | optional | 目标文件夹 token；省略则导入到云空间根目录 |
| `--name` | string | optional | 导入后表格名称；省略则用本地文件名（去掉扩展名） |

## Schemas

> 复合 JSON flag 字段速查（只列顶层 + 一层嵌套）。深层结构看下方 `## Examples`，或用 `--print-schema` 读完整 JSON Schema（用法见 SKILL.md「公共 flag 速查」与「Agent 使用提示」）。

### `+workbook-create` `--sheets`

_一个或多个子表的 typed 数据，每个数组元素写入一张子表；支持多 DataFrame → 多子表一次写入_

**数组项**（类型 object）：
- `name` (string) — 目标子表名
- `start_cell` (string?) — 写入起点单元格（A1 记法，如 "B2"），默认 "A1"
- `mode` (enum?) — overwrite（默认）：从 start_cell 起写「表头 + 数据」块；append：把数据追加到子表已有数据下方（默认不重复表头） [overwrite / append]
- `header` (boolean?) — 是否写一行列名表头
- `allow_overwrite` (boolean?) — 为 false 时，若写入会落在非空单元格则拒写以保护原数据（返回 partial_success）
- `columns` (array<object>) — 列定义，顺序与 rows 中每行的取值一一对应 each: { name: string, type: enum, format?: string }
- `rows` (array<array<string|number|boolean|null>>) — 数据行；每行是一个数组，长度必须等于 columns 数

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。`+workbook-info` 只用前两者；`+sheet-*` 系列对单个工作表操作，需 `--sheet-id` 或 `--sheet-name`。

### `+workbook-info`

输出契约：返回 `sheets[]`，每个含 `sheet_id` / `title`（工作表显示名；旧 payload 用 `sheet_name`，读取时优先取 `title`、缺失再回退 `sheet_name`）/ `row_count` / `column_count` / `index` / `is_hidden`，以及计数字段 `merged_cells_count` / `chart_count` / `pivot_table_count` / `float_image_count`（无 `frozen_*` 字段，冻结信息请用 `+sheet-info` 读取）。是操作飞书表格的第一步——任何后续 sheet 级动作都需要先拿这里的 sheet_id。

### `+workbook-create`

新建电子表格，可选预填数据。两种数据入口**互斥**，按需二选一：

```bash
# 1) untyped：--headers + --values（纯值；类型由飞书自动识别，日期会落成文本）
lark-cli sheets +workbook-create --title "销售" \
  --headers '["门店","销售额"]' --values '[["北京",259874]]'

# 2) typed：--sheets（一步建表 + 类型保真）。date 列落成真日期（可排序/透视）、
#    number 不丢精度、string 列保前导零（如订单号 00123）；多子表一次建。
lark-cli sheets +workbook-create --title "交易" --sheets '{
  "sheets":[
    {"name":"明细","columns":[
      {"name":"日期","type":"date"},
      {"name":"金额","type":"number","format":"#,##0.00"},
      {"name":"单号","type":"string"}
    ],"rows":[["2024-01-15",1234.5,"00123"]]}
  ]}'
```

`--sheets` 协议与 `+table-put` 完全同构（字段含义见 lark-sheets-write-cells 的 `+table-put`，大 payload 走 stdin / `@file`）。关键差异：**新建工作簿的默认子表会被复用为第一个子表**（重命名后承载数据），不会残留空 `Sheet1`；其余子表按需新建。它把 `+table-put` 单独做不到的"建表 + typed 写入"合到一条命令，是「pandas 算完直接落地一张带真日期的新表」的首选。回读校验用 `+table-get`（与 `--sheets` 同构、可 round-trip）。

> ⚠️ **`+workbook-create` 是把内存里的数据写成新表；要把已有的本地 Excel/CSV 文件原样导入成新表，用 `+workbook-import`**（见下），不要先在本地读出文件再 `+workbook-create` 重灌。

### `+workbook-import`

把已有的本地 `.xlsx` / `.xls` / `.csv` 文件导入为一个**新的**飞书电子表格（异步任务 + 内置轮询），与 `+workbook-export`（导出）对称。底层复用 drive 的导入实现，固定导入为电子表格类型。

```bash
# 导入到云空间根目录；表格名默认取本地文件名（去掉扩展名）
lark-cli sheets +workbook-import --file ./data.xlsx

# 指定目标文件夹与导入后表格名
lark-cli sheets +workbook-import --file ./report.csv --folder-token <FOLDER_TOKEN> --name "月度报表"
```

- **不接受任何 spreadsheet / sheet 定位 flag**（它是新建，不操作已有表）：只有 `--file`（必填）/ `--folder-token` / `--name`。
- 仅导入为电子表格（sheet）。若要把本地表格导入成多维表格（bitable），改用 `lark-cli drive +import --type bitable`。
- 返回 `token` / `url`（导入完成的新表格）/ `ticket` / `ready` / `job_status`；未在内置轮询窗口内完成时返回 `timed_out=true` 与续查命令 `next_command`。

### `+sheet-create`

示例：

```bash
lark-cli sheets +sheet-create --url "https://example.feishu.cn/sheets/shtXXX" \
  --title "汇总" --index 0
```

### `+sheet-delete`

> ⚠️ 工作表删除不可逆；先 `--dry-run` 看输出 sheet_id + title 确认是要删的那张。

### `+sheet-rename`

```bash
lark-cli sheets +sheet-rename --url "..." --sheet-id "$SID" --title "汇总"
```

### `+sheet-move`

standalone 路径在缺 `--source-index` / 只给 `--sheet-name` 时会自动发起一次 `+workbook-info` 读把它们解出来。

> ⚠️ **在 `+batch-update` 内调用 `+sheet-move`**：必须同时显式传 `--sheet-id`、`--source-index` 和 `--index`（目标位置）。batch 中途无法发起结构查询，且 `--index` 不显式给会静默落到默认位置 0，所以 batch translator 强制要求三者都显式。

### `+sheet-copy`

```bash
# --title 省略时由服务端生成副本名
lark-cli sheets +sheet-copy --url "..." --sheet-id "$SID" --title "副本"
```

### `+sheet-hide` / `+sheet-unhide`

```bash
lark-cli sheets +sheet-hide   --url "..." --sheet-id "$SID"
lark-cli sheets +sheet-unhide --url "..." --sheet-id "$SID"
```

### `+sheet-set-tab-color`

```bash
# Hex 色值；传空字符串 "" 清除标签色
lark-cli sheets +sheet-set-tab-color --url "..." --sheet-id "$SID" --color "#FF0000"
```

### `+sheet-show-gridline` / `+sheet-hide-gridline`

```bash
# 切换子表网格线显隐；二态语义在命令名里，无需额外参数（同 +sheet-hide/+sheet-unhide）
lark-cli sheets +sheet-show-gridline --url "..." --sheet-id "$SID"
lark-cli sheets +sheet-hide-gridline --url "..." --sheet-id "$SID"
```

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`+sheet-create` 校验 `--title` 非空、`--row-count` ≤ 50000、`--col-count` ≤ 200；`+sheet-delete` 必须 `--yes` 或 `--dry-run`；`+workbook-create` 的 `--sheets` 与 `--headers`/`--values` **互斥**，给了 `--sheets` 则按 typed 协议校验 payload（其余约束同 `+table-put`）。
- `DryRun`：`+sheet-*` 写操作输出"将要 PATCH 的 sheet metadata"；`--sheet-name` 在 dry-run 输出里生成为 `<resolve:Sheet1>` 占位符，不实际解析为 sheet-id。
- `Execute`：写操作不自动回读；如需确认目标 sheet 的新状态，自行调用 `+workbook-info`。
