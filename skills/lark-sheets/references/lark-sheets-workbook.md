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
| 获取当前 revision | `+revision-get` | 获取当前文档 revision（版本号），可作为 recover / undo / changeset 复核的版本锚点 |
| 新建工作簿（可预填数据） | `+workbook-create` | 从内存数据建一张新表（`--values` / `--sheets` typed） |
| 导入本地文件为新表 | `+workbook-import` | 把本地 `.xlsx` / `.xls` / `.csv` 导入为新的飞书电子表格 |
| 导出工作簿到本地 | `+workbook-export` | 导出为本地 `.xlsx`（整簿）或单子表 `.csv` |
| 变更工作簿结构 | `+sheet-{create|delete|rename|move|copy|hide|unhide|set-tab-color}` | 新建/删除/移动/重命名/复制/隐藏子表、修改标签颜色 |
| 切换子表网格线显隐 | `+sheet-show-gridline` / `+sheet-hide-gridline` | 显示 / 隐藏单个子表的网格线 |

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
| `+revision-get` | read | 工作簿 |
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

### `+revision-get`

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
| `--type` | string | optional | 新子表类型：sheet（电子表格）；默认 sheet（可选值：`sheet`） |

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
| `--source-index` | int | optional | 源位置（0-based）；standalone 调用时可选，未传时由 CLI runtime 根据 `--sheet-id` / `--sheet-name` 当前在工作簿中的 index 自动派生。但在 `+batch-update` 内不可省（须显式传）——batch 中途无法发起结构查询自动派生 |

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
| `--values` | string + File + Stdin（简单 JSON） | optional | untyped 初始数据，一个 JSON 二维数组（表头并入第一行）：`[["列A","列B"],["alice",95]]`；值原样写入、类型由飞书自动识别（日期 / 数字会落成文本，需类型保真改用 --sheets），走与 --sheets 相同的分批 `+cells-set`；配 --styles 控制格式/颜色/合并/行列尺寸 |
| `--sheets` | string + File + Stdin（复合 JSON） | optional | 建表后写入的 typed 表格协议 JSON（同 +table-put）：顶层 `{"sheets":[...]}`，每个数组项是一张子表 `{name, start_cell?, mode?, header?, allow_overwrite?, columns:["colA","colB",...], data:[[...]], dtypes?:{colA:pandasDtype, ...}, formats?:{colA:numberFormat, ...}}` —— `name` 与外层 `sheets` 数组都不可省。Agents 用 `scripts/sheets_df.py` 的 `df_to_sheet(df, name)` 把 DataFrame 转成一项再包 `{"sheets":[...]}`。与 --values 互斥；新表默认子表复用为第一个子表，日期/数字类型保真。 |
| `--styles` | string + File + Stdin（复合 JSON） | optional | 建表时同时写入的视觉处理操作 JSON：顶层 `{styles:[...]}`，每项对应一个目标子表、含 `name`，并至少给 `cell_styles` / `row_sizes` / `col_sizes` / `cell_merges` 之一。`cell_styles` 用 A1 单元格 range + 扁平样式字段（字段同 +cells-set-style，含 number_format / 颜色 / 对齐 / border_styles）；row/col sizes 用行/列范围 + type/size；merges 用单元格 range + 可选 merge_type。与 --sheets 搭配时 styles 数组长度/顺序/name 必须与 --sheets.sheets 对应；与 --values 搭配时只给一个 styles 项（其 name 忽略）。完整 cell_styles 字段结构跑 `+workbook-create --print-schema --flag-name styles`。 |

### `+workbook-export`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--file-extension` | string | optional | 导出文件格式；`csv` 模式必须配 `--sheet-id`（可选值：`xlsx` / `csv`）（默认 `xlsx`） |
| `--sheet-id` | string | optional | 仅 csv 模式必填：指定要导出哪张 sheet 为 CSV。这是 `+workbook-export` 专有 flag，与公共四件套的 sheet 定位无关（本 shortcut 不接受公共 sheet 定位） |
| `--output-path` | string | optional | 本地保存路径；省略时**只触发并轮询导出任务、不下载文件**（返回 file_token / status，便于稍后续传）。要落盘传具体路径（如 `./out.xlsx`）或目录（如 `.`，服务端给的文件名落在该目录下）。注意：对应的 `lark-cli drive +export --doc-type sheet` 走 `--output-dir` / `--file-name` / `--overwrite` 三 flag 且默认下载到当前目录——本 wrapper 把它们合成单一 `--output-path` 简化常见用例，但默认不下载，需要的话也可改用 `drive +export`。 |

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
- `columns` (array<string>) — 列名字符串数组，顺序与 `data` 中每行取值一一对应
- `data` (array<array<string|number|boolean|null>>) — 数据行；每行是一个数组，长度必须等于 `columns` 数
- `dtypes` (object?) — 可选
- `formats` (object?) — 可选

### `+workbook-create` `--styles`


**数组项**（类型 object）：
- `cell_merges` (array<object>?) — 单元格合并操作数组；range 使用 A1 单元格范围，merge_type 默认 all each: { merge_type?: enum, range: string }
- `cell_styles` (array<object>?) — 单元格样式操作数组；每项用 A1 单元格 range 指定范围，字段名与 +cells-set-style 对齐 each: { background_color?: string, border_styles?: object, font_color?: string, font_family?: string, font_line?: enum, …共 13 项 }
- `col_sizes` (array<object>?) — 列宽操作数组；range 使用列范围如 A:C，type 为 pixel/standard，pixel 需要 size each: { range: string, size?: number, type: enum }
- `name` (string) — 子表名
- `row_sizes` (array<object>?) — 行高操作数组；range 使用行范围如 1:3，type 为 pixel/standard/auto，pixel 需要 size each: { range: string, size?: number, type: enum }

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。`+workbook-info` 只用前两者；`+sheet-*` 系列对单个工作表操作，需 `--sheet-id` 或 `--sheet-name`。

### `+workbook-info`

输出契约：返回 `sheets[]`，每个含 `sheet_id` / `title`（工作表显示名；旧 payload 用 `sheet_name`，读取时优先取 `title`、缺失再回退 `sheet_name`）/ `index` / `resource_type` / `row_count` / `column_count` / `is_hidden`，以及计数字段 `merged_cells_count` / `chart_count` / `pivot_table_count` / `float_image_count`（无 `frozen_*` 字段，冻结信息请用 `+sheet-info` 读取）。是操作飞书表格的第一步——任何后续 sheet 级动作都需要先拿这里的 sheet_id。

> **子表类型 `resource_type`**：`sheet`（普通网格子表）/ `bitable`（内嵌的多维表格子表）/ `#UNSUPPORTED_TYPE`（其它暂不支持的嵌入子表）。
> - 网格类操作（读写单元格 / 区域 / 样式 / CSV / 筛选 / 透视 / 图表等）**仅适用于 `sheet`**。对 `bitable` / `#UNSUPPORTED_TYPE` 子表执行网格操作会被直接拒绝并返回明确报错，不再静默出错。
> - 要操作 `bitable` 子表里的数据：该子表条目会附带 `bitable_app_token` + `bitable_table_id` 两个字段，直接用多维表格命令操作，例如 `lark-cli base +record-list --base-token <bitable_app_token> --table-id <bitable_table_id>`（记录增删改查、字段、视图等整套 `lark-cli base` 命令均可用）。不要走 sheets 网格命令。
> - `bitable` / `#UNSUPPORTED_TYPE` 子表条目**只含** `sheet_id` / `sheet_name` / `index` / `resource_type`（bitable 另加上述两个 token）以及 `is_hidden` / `tab_color`；**不输出** `row_count` / `column_count` / `merged_cells_count` / `chart_count` / `pivot_table_count` / `float_image_count` / `frozen_*` 等网格指标（对非网格子表无意义）。
> - tab 管理类操作（`+sheet-rename` / `+sheet-move` / `+sheet-delete` / `+sheet-hide` 等）对任意 `resource_type` 的子表都合法，不受此限制。

### `+revision-get`

输出契约：返回单个 `revision` 字段，即当前文档版本号。它是 recover / undo / `+changeset-get` 的版本锚点：如果刚执行过一次读写操作，也可以直接复用那次响应里的 `revision`；当只想单独取当前版本号、且不需要其它结构信息时，用 `+revision-get` 最直接。

### `+workbook-create`

新建电子表格，可选预填数据。两种数据入口（untyped `--values` / typed `--sheets` JSON）**互斥**，按需选一——两者都走同一条分批写入：

```bash
# 1) untyped：--values（一个二维数组，表头并入第一行；值原样写、类型由飞书自动识别，
#    日期会落成文本，配 --styles 控制格式）
lark-cli sheets +workbook-create --title "销售" \
  --values '[["门店","销售额"],["北京",259874]]'

# 2) typed JSON：--sheets（一步建表 + 类型保真）。date 列落成真日期（可排序/透视）、
#    number 不丢精度、string 列保前导零（如订单号 00123）；多子表一次建。
lark-cli sheets +workbook-create --title "交易" --sheets '{
  "sheets":[
    {"name":"明细",
     "columns":["日期","金额","单号"],
     "dtypes":{"日期":"datetime64[ns]","金额":"float64","单号":"object"},
     "formats":{"金额":"#,##0.00"},
     "data":[["2024-01-15",1234.5,"00123"]]}
  ]}'
```

`--sheets` 协议与 `+table-put` 完全同构（字段含义见 lark-sheets-write-cells 的 `+table-put`，大 payload 走 stdin / `@file`）。关键差异：**新建工作簿的默认子表会被复用为第一个子表**（重命名后承载数据），不会残留空 `Sheet1`；其余子表按需新建。它把 `+table-put` 单独做不到的"建表 + typed 写入"合到一条命令，是「pandas 算完直接落地一张带真日期的新表」的首选。回读校验用 `+table-get`（与 `--sheets` 同构、可 round-trip）。

> 💡 pandas DataFrame 走 `--sheets` 时直接 `from sheets_df import df_to_sheet`（[`scripts/sheets_df.py`](../scripts/sheets_df.py)，与 `+table-put` 共用同一份 helper），多子表场景 helper 优势更明显：
> ```python
> payload = {"sheets": [df_to_sheet(income, "Income Statement"),
>                       df_to_sheet(balance, "Balance Sheet"),
>                       df_to_sheet(cashflow, "Cash Flow")]}
> ```

`--styles` 可在建表写入时同时写视觉处理。它和 `--sheets` 一样只有一种外层写法：顶层对象里放 `styles` 数组；数组每项对应一个子表，含 `name`，并按能力拆成四类可选数组：

- `cell_styles`：像 `+cells-set-style`，用 A1 单元格 `range` 加扁平样式字段（`font_weight` / `background_color` / `horizontal_alignment` / `vertical_alignment` / `number_format` 等）和可选 `border_styles`；这些样式会随内容在同一次写入里一并应用。完整字段跑 `+workbook-create --print-schema --flag-name styles`。
- `cell_merges`：用 A1 单元格 `range` 设置合并，`merge_type` 默认为 `all`，可选 `rows` / `columns`。
- `row_sizes`：用行范围（如 `1:3`）设置行高，`type` 为 `pixel` / `standard` / `auto`；`pixel` 需要 `size`。
- `col_sizes`：用列范围（如 `A:C`）设置列宽，`type` 为 `pixel` / `standard`；`pixel` 需要 `size`。

同一单元格命中多个 `cell_styles` 项时，后面的操作继续合并覆盖已传字段。`cell_merges` / `row_sizes` / `col_sizes` 在内容写入后顺序执行。

```bash
# 3) untyped：仍用 {"styles":[...]}，只有一个子表样式项（name 忽略）；range 覆盖 --values 初始区域
lark-cli sheets +workbook-create --title "销售" \
  --values '[["门店","销售额"],["北京",259874],["上海",198320]]' \
  --styles '{
    "styles":[
      {"name":"Sheet1","cell_styles":[
        {"range":"A1:B1","font_weight":"bold","background_color":"#f5f5f5","horizontal_alignment":"center","vertical_alignment":"middle"},
        {"range":"B2:B3","number_format":"#,##0"}
      ]}
    ]
  }'

# 4) typed 单子表：--styles.styles[0].name 必须对应 --sheets.sheets[0].name
lark-cli sheets +workbook-create --title "交易" --sheets '{
  "sheets":[
    {"name":"明细",
     "columns":["日期","金额"],
     "dtypes":{"日期":"datetime64[ns]","金额":"float64"},
     "formats":{"金额":"#,##0.00"},
     "data":[["2024-01-15",1234.5]]}
  ]}' --styles '{
    "styles":[
      {"name":"明细",
       "cell_styles":[
        {"range":"A1:B1","font_weight":"bold","background_color":"#f5f5f5",
          "border_styles":{"bottom":{"style":"solid","weight":"thin","color":"#000000"}}},
        {"range":"A2:A2","number_format":"yyyy-mm-dd"},
        {"range":"B2:B2","number_format":"#,##0.00","font_color":"#0f7b0f"}
       ],
       "cell_merges":[{"range":"A1:B1"}],
       "col_sizes":[{"range":"A:B","type":"pixel","size":120}],
       "row_sizes":[{"range":"1:1","type":"pixel","size":28}]}
    ]
  }'

# 5) typed 多子表：styles 数组和 sheets 数组长度、顺序、name 都必须一致
lark-cli sheets +workbook-create --title "经营看板" --sheets '{
  "sheets":[
    {"name":"收入","columns":["月份","收入"],"dtypes":{"收入":"int64"},"formats":{"收入":"#,##0"},"data":[["2026-05",1200000]]},
    {"name":"成本","columns":["月份","成本"],"dtypes":{"成本":"int64"},"formats":{"成本":"#,##0"},"data":[["2026-05",730000]]}
  ]}' --styles '{
    "styles":[
      {"name":"收入","cell_styles":[
        {"range":"A1:B1","font_weight":"bold","background_color":"#f0f7ff"},
        {"range":"B2:B2","font_color":"#0f7b0f"}
      ]},
      {"name":"成本","cell_styles":[
        {"range":"A1:B1","font_weight":"bold","background_color":"#fff7ed"},
        {"range":"B2:B2","font_color":"#b42318"}
      ]}
    ]
  }'
```

> ⚠️ **`+workbook-create` 是把内存里的数据写成新表；要把已有的本地 Excel/CSV 文件原样导入成新表，用 `+workbook-import`**（见下），不要先在本地读出文件再 `+workbook-create` 重灌。

### `+workbook-import`

把已有的本地 `.xlsx` / `.xls` / `.csv` 文件导入为一个**新的**飞书电子表格（异步任务 + 内置轮询），与 `+workbook-export`（导出）对称，固定导入为电子表格类型。

```bash
# 导入到云空间根目录；表格名默认取本地文件名（去掉扩展名）
lark-cli sheets +workbook-import --file ./data.xlsx

# 指定目标文件夹与导入后表格名
lark-cli sheets +workbook-import --file ./report.csv --folder-token <FOLDER_TOKEN> --name "月度报表"
```

- **不接受任何 spreadsheet / sheet 定位 flag**（它是新建，不操作已有表）：只有 `--file`（必填）/ `--folder-token` / `--name`。
- 本地表格文件 → 飞书电子表格一律用本命令，**不要**用 `drive +import` 导电子表格——它是 sheets 之外的通用导入、还需额外指定 `--type`，绕路且更易错。只有要把本地表格导入成**多维表格**（bitable）时，才改用 `lark-cli drive +import --type bitable`。
- 返回 `token` / `url`（导入完成的新表格）/ `ticket` / `ready` / `job_status`；未在内置轮询窗口内完成时返回 `timed_out=true` 与续查命令 `next_command`。

### `+workbook-export`

把飞书电子表格导出为本地 `.xlsx`（整工作簿）或单子表 `.csv`（异步任务 + 内置轮询 + 可选下载）。

```bash
# 1) 只创建并轮询导出任务，不下载（默认）：返回 file_token / status 便于稍后续传
lark-cli sheets +workbook-export --url "https://example.feishu.cn/sheets/shtXXX"

# 2) 下载到具体文件名
lark-cli sheets +workbook-export --url "..." --output-path ./report.xlsx

# 3) 下载到目录（保留服务端给的文件名）
lark-cli sheets +workbook-export --url "..." --output-path ./downloads/

# 4) csv 模式必须传 --sheet-id（API 一次只导一张子表）
lark-cli sheets +workbook-export --url "..." --file-extension csv --sheet-id "$SID" --output-path ./sheet.csv
```

> ⚠️ **默认不下载**：省略 `--output-path` 时只触发并轮询导出任务，不写本地文件——给「先排队再续传」用例留出口。要落盘必须显式给 `--output-path`。
>
> **与 `drive +export --doc-type sheet` 的关系**：本 wrapper 是它的特化封装，固定 `--doc-type sheet`，并把 drive 的 `--output-dir` / `--file-name` / `--overwrite` 三 flag 折叠成单一 `--output-path` 简化常见用例。代价是默认值不同：`drive +export` 默认下载到当前目录、本 wrapper 默认不下载。需要细控目录/文件名/是否覆盖的，回退到 `drive +export --doc-type sheet`。

### `+sheet-create`

示例：

```bash
lark-cli sheets +sheet-create --url "https://example.feishu.cn/sheets/shtXXX" \
  --title "汇总" --index 0
```

> 💡 `+sheet-create` 只建一张**空子表**。要在已有工作簿里建子表并一步写入 typed 数据和/或样式，用 `+table-put`（payload 里命名的子表缺则自动新建）配合它的 `--sheets` / `--styles`，省掉先建表再 `+cells-set` / `+cells-set-style` 的二次往返。

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

> 💡 `+sheet-copy` 连**公式 / 合并 / 分组底色 / 列宽 / 条件格式**一起整表复制。"照一张现成子表批量造结构相同的新子表"（如参考模板给每份数据各建一张同构子表）时，先 `+sheet-copy` 复制模板再用 `+cells-*` 只改数据，比从零 `+sheet-create` + 重建公式 / 样式省一大截，也天然满足"公式 / 分组 / 颜色照搬"。要把本地文件 / 数据并入**已有工作簿**当子表时走它（或 `+sheet-create`），别用 `+workbook-import` / `+workbook-create`——那两条只会新建独立表。

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

- `Validate`：XOR 公共四件套；`+sheet-create` 校验 `--title` 非空、`--row-count` ≤ 50000、`--col-count` ≤ 200；`+sheet-delete` 必须 `--yes` 或 `--dry-run`；`+workbook-create` 的 `--sheets` 与 `--values` **互斥**，给了 `--sheets` 则按 typed 协议校验 payload（其余约束同 `+table-put`）。
- `DryRun`：`+sheet-*` 写操作输出"将要 PATCH 的 sheet metadata"；`--sheet-name` 在 dry-run 输出里生成为 `<resolve:Sheet1>` 占位符，不实际解析为 sheet-id。
- `Execute`：写操作不自动回读；如需确认目标 sheet 的新状态，自行调用 `+workbook-info`。
