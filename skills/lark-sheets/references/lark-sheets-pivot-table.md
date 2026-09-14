# Lark Sheet Pivot Table

## 真对象硬约束

当用户要求"透视表 / 分组汇总 / 交叉分析 / 按 X 统计 Y"时，**必须**通过 `+pivot-{create|update|delete}` 创建真实的透视表对象。**禁止**用 `SUMIFS` / `COUNTIFS` 等普通公式 + `+cells-set` 在原表中拼一张"看起来像透视表的汇总表"来代替。判断标准：交付后 `+pivot-list` 必须能返回该对象。

## 使用场景

读写透视表对象。本 reference 覆盖 4 个 shortcut：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 查看已有透视表 | `+pivot-list` | 获取透视表的结构、数据源和配置 |
| 创建/更新/删除透视表 | `+pivot-{create|update|delete}` | 对透视表执行写入操作 |

典型工作流：先读取现有透视表了解配置 → 执行创建/更新/删除 → **必须再次读取验证结果**。

## 行/值字段映射（创建前必做）

创建透视表前先识别用户需求中的分组维度和聚合指标，**不要搞反**：

- **rows（行字段）** = 分组维度，即"按什么分组"。例：部门、地区、医生、产品类别
- **values（值字段）** = 聚合指标，即"统计什么数值"。例：销售额（聚合方式 `sum`）、订单数（聚合方式 `count`）
- **columns（列字段）** = 交叉维度（可选），即"再按什么横向展开"。例：月份、性别

| 用户说 | rows | values | columns |
|--------|------|--------|---------|
| "按部门统计人数" | 部门 | 姓名（`summarize_by: "count"`） | — |
| "按医生统计费用和结余" | 主管医生 | 费用（`"sum"`）、结余（`"sum"`） | — |
| "各部门男女人数" | 部门 | 姓名（`"count"`） | 性别 |

**常见配置错误（必须注意）**：
- **值字段类型与聚合器匹配**：`sum/average/median/product/stdDev/stdDevp/var/varp` 只用于数值列；数字个数用 `countNums`，非空记录数用 `count`。mixed 列先保留原值并新增清洗结果/失败标记，记录总数、成功、失败、空值和统计分母，再对清洗后的数值列聚合。
- **数据源范围必须精确**：透视表的数据源范围必须包含表头行，且精确覆盖全部数据行列。范围过大（包含空行/空列）或过小（遗漏数据列）都会导致透视表结果错误
- **行列字段选择要匹配用户意图**：用户说"按商品统计金额"→ 行字段=商品，值字段=金额（`summarize_by: "sum"`）。不要把行列字段搞反
- **聚合类型要匹配**：用户说"统计数量"→ `summarize_by: "count"`；"统计总额"→ `"sum"`；"统计平均"→ `"average"`。完整合法值：`sum` / `count` / `average` / `max` / `min` / `product` / `countNums` / `stdDev` / `stdDevp` / `var` / `varp` / `distinct` / `median`。按用户意图选聚合方式，不要拿 `count` 顶替 `sum`
- **`--properties` 还原生支持**：计算字段 `calculated_fields[].summarize_by ∈ {sum, custom}`、重复行标签 `repeat_row_labels: true`——别因速查表没列就判"不支持"绕路
- **参数长度限制**：如果透视表配置 JSON 过长（数据源范围跨越大量行列），可能导致工具调用失败。此时应先确认数据范围的精确边界，避免传入过大的 range
- **落点不能覆盖任何已有数据（不只是 `--source` 范围）**：透视表创建后会向右下**展开**，展开区域哪怕只盖到一个已有单元格（即便已避开源数据），也会报「目标位置不能与数据源重叠」并产生 `#REF!`。创建前无法精确预知展开尺寸，故**强烈优先默认策略**（不传 `--target-sheet-id/-name` 与 `--target-position`/`--range`，后端自动新建空白子表），零覆盖风险；非要落到已有子表，必须挑一片足够大的纯空白区
- **创建后轮询并校验**：调用 `+pivot-list --sheet-id/--sheet-name <落点表> --pivot-table-id <id>`。`Loading` / `ServiceCalcLoading` 是瞬态，继续轮询到 `info.loaded=true` 且 `error_state=None`；`Cover` / `Shrink` 等终态错误再删除重建。随后用 `info.content_range/page_range` 回读展开区，确认非空、尺寸、总计位置和用户点名的指标。

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+pivot-list` | read | 对象 |
| `+pivot-create` | write | 对象 |
| `+pivot-update` | write | 对象 |
| `+pivot-delete` | high-risk-write | 对象 |

## Flags

### `+pivot-list`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--pivot-table-id` | string | optional | 按 id 过滤 |

### `+pivot-create`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--properties` | string + File + Stdin（复合 JSON） | required | JSON：{"rows":[...],"columns":[...],"values":[...],"filters":[...],"show_row_grand_total":true,"show_col_grand_total":true}（数据源走 --source，不要再放进 properties.source） |
| `--target-position` | string | optional | 透视表落点子表内的起始 cell（A1 格式，如 `A1`），默认 `A1`（值为 A1 时不下发）。它与 `--range` 落在同一 wire 字段 `properties.range`，给非默认值时优先于 `--range`；两者同时给非默认值会被拒绝，只传其一 |
| `--target-sheet-id` | string | xor | 透视表落点目标子表的 reference_id（与 `--target-sheet-name` 互斥，优先于 --target-sheet-name；都不传时自动新建一张子表放置透视表——推荐）。与数据源 sheet 区分：数据源 sheet 写在 --source 的 A1 引用里（带 sheet 前缀，形如 `'Sheet1'!A1:D100`）。 |
| `--target-sheet-name` | string | xor | 透视表落点目标子表的名称（与 `--target-sheet-id` 互斥；都不传时自动新建一张子表放置透视表——推荐）。与数据源 sheet 区分：数据源 sheet 写在 --source 的 A1 引用里（带 sheet 前缀，形如 `'Sheet1'!A1:D100`）。 |
| `--source` | string | required | 透视表源数据区域（A1 表示法，格式 `'SheetName'!StartCell:EndCell`，如 `'Sheet1'!A1:D100`） |
| `--range` | string | optional | 透视表左上角放置位置（A1 单值，如 `F1`，仅 create 生效），映射到 `properties.range`；省略时放在落点子表（默认新建子表）的左上角。它与 `--target-position` 落在同一 wire 字段，两者同时给非默认值会被拒绝，只传其一 |

### `+pivot-update`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--pivot-table-id` | string | required | 目标透视表 id |
| `--properties` | string + File + Stdin（复合 JSON） | required | 完整或足够完整的配置（先 `+pivot-list --pivot-table-id <id>` 回读再 patch） |

### `+pivot-delete`

_公共四件套 · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--pivot-table-id` | string | required | 目标透视表 id |

## Schemas

> 复合 JSON flag 字段速查（只列顶层 + 一层嵌套）。深层结构看下方 `## Examples`，或用 `--print-schema` 读完整 JSON Schema（用法见 SKILL.md「公共 flag 速查」与「Agent 使用提示」）。

### `+pivot-create` `--properties` / `+pivot-update` `--properties`

_创建/更新的透视表属性_

**顶层字段**：
- `range` (string?) — 放置透视表的左上角单元格 A1 地址（例如：'F1'）（仅 create 时有效） — ⚠️ 已拎为独立 flag `--range`，请勿在此 JSON 内重复填写（同名以独立 flag 为准）
- `source` (string?) — 源数据区域地址，格式为 'SheetName!StartCell:EndCell'（例如：'Sheet1!A1:D100'） — ⚠️ 已拎为独立 flag `--source`，请勿在此 JSON 内重复填写（同名以独立 flag 为准）
- `rows` (array<object>?) — 纵向分组字段（行字段） each: { field: string, display_name?: string, sort?: object, filter?: object, condition_filter?: object, …共 6 项 }
- `columns` (array<object>?) — 横向分组字段（列字段） each: { field: string, display_name?: string, sort?: object, filter?: object, condition_filter?: object, …共 6 项 }
- `filters` (array<object>?) — 筛选区域字段（页字段） each: { field: string, display_name?: string, filter?: object, condition_filter?: object, group?: object }
- `values` (array<object>?) — 要汇总的字段（至少需要 1 个） each: { field: string, display_name?: string, summarize_by?: enum, show_data_as?: enum, base_field?: string }
- `auto_fit_col` (boolean?) — 是否自动调整列宽以适应内容
- `show_row_grand_total` (boolean?) — 是否显示行总计（默认 true）
- `show_col_grand_total` (boolean?) — 是否显示列总计（默认 true）
- `show_subtotals` (boolean?) — 是否显示分类小计（默认 true，应用于所有字段）
- `repeat_row_labels` (boolean?) — 是否显示重复项标签
- `calculated_fields` (array<object>?) — 计算字段列表 each: { name: string, formula: string, summarize_by?: enum }
- `collapse` (object?) — 行字段展开/折叠状态：字段名 -> 要折叠的项目列表

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`，其中 `--sheet-id` / `--sheet-name` 在 `+pivot-update` / `+pivot-delete` / `+pivot-list` 上是公共四件套语义（定位透视表所在 sheet，XOR 必传一个）。

**`+pivot-create` 例外**：placement 选择器用 `--target-sheet-id` / `--target-sheet-name`（至多一个、都可省略；省略时后端自动新建子表，推荐）。数据源 sheet 写在 `--source` 的 `'SheetName'!Range` 里。

### `+pivot-list`

```bash
lark-cli sheets +pivot-list --url "..." --sheet-id "$SID"
```

> **返回值含 `info`（展开后的占用区域与状态）**：每个透视表对象除 `position` / `snapshot` 外，还返回 `info`，标明它在 sheet 上的平铺区域与状态——`info.page_range`（筛选/分页区 A1）、`info.content_range`（主体数据区 A1）、`info.span_range`（空表合并区 A1）、`info.error_state`（错误状态，如 `None`/`Cover`/`Shrink`/`Loading`）、`info.is_empty` / `info.is_hidden`、`info.row`/`info.col`（锚点）等。
> **用途 1（判断改值还是改配置）**：当用户描述某个单元格要改动时，先 `+pivot-list` 拿到 `info`，判断该单元格是否落在 `page_range` / `content_range` 内——**落在区域内 = 属于透视表，应走 `+pivot-update` 改配置**（透视表单元格不能直接 `+cells-set` 改值）；**落在区域外 = 普通单元格，正常 `+cells-set` 改值**。
> **用途 2（创建后校验覆盖）**：建完后轮询 `info.loaded/error_state`；`Loading` / `ServiceCalcLoading` 继续等待，`Cover` / `Shrink` 等终态错误才表示冲突。成功后用 `content_range/page_range` 核对真实占用区域与原数据边界。

### `+pivot-create`

> 数据源 `--source` 必须从表头行开始；空行 / 汇总行会被当作数据参与聚合，需提前用 `+csv-get` 确认起止边界。`--source` 和 `--range` 是独立 flag（不要再放 `--properties`）；`rows` / `columns` / `values` 等数组字段走 `--properties`。
>
> **先理清 `+pivot-create` 上 4 个位置类入参（语义不同，别混）**：
> - `--source`（**必填**）：**源数据**区域，须自带 `Sheet!` 前缀（如 `'Sheet1'!A1:D100`，sheet 名按 A1 标准单引号包裹）。源 sheet 的名字在 `--source` 字符串里，**不**通过单独 flag 传。
> - `--target-sheet-id` / `--target-sheet-name`：**透视表的落点 sheet**（即产物放哪张子表）。两个互斥（最多传一个），都不传时后端自动新建子表存放产物（强烈推荐）。
> - `--target-position`（可选，默认 `A1`）与 `--range`（可选）都映射到 `properties.range`，表达同一落点；不要同时给两个非默认值。
>
> **落点 3 种策略（互斥，选其一）**：
> 1. **默认（强烈推荐）**：`--target-sheet-id` / `--target-sheet-name` / `--target-position` / `--range` **全都不传** → 服务端**自动新建子表**存放产物，绝不碰任何已有数据。
> 2. **放进指定的已有子表**：传 `--target-sheet-id <落点子表 id>`（或 `--target-sheet-name`），可选 `--target-position <子表内起点 cell>`。⚠️ **若落点子表就是源数据所在的 sheet**，必须配 `--target-position` 或 `--range` 指向源数据范围**之外**的位置，否则产物默认从 A1 起会盖在源数据上。
> 3. **`--range`**：跟策略 2 等价（同样需要 `--target-sheet-id` / `--target-sheet-name` 指定落点子表，不然落到自动新建子表），只是改用 `--range` 表达同一落点（与 `--target-position` 同一 wire 字段）。同样的覆盖风险，同样需要避开源数据范围。
>
> 一般用策略 1（默认新建子表）即可，零覆盖风险，无需任何 `--target-*` / `--range` flag。

```bash
# 策略 1（强烈推荐）：不传任何落点 flag → 后端自动新建子表，零覆盖风险
lark-cli sheets +pivot-create --url "..." \
  --source "'Sheet1'!A1:D100" --properties @pivot.json

# 策略 2：落进指定的已有目标子表（注意目标 sheet ≠ 源 sheet，否则要配 --target-position 避开源数据）
lark-cli sheets +pivot-create --url "..." \
  --source "'Sheet1'!A1:D100" --target-sheet-id "$DEST_SID" --target-position "A1" --properties @pivot.json
```

### `+pivot-update`

> 不允许改落点 range；更新配置前先 `+pivot-list --sheet-id/--sheet-name <落点表> --pivot-table-id <id>` 回读完整 snapshot，再 patch rows / columns / values / filters。需要切换数据源时，可在 `--properties` 中提供新的 `source`。

### `+pivot-delete`

```bash
lark-cli sheets +pivot-delete --url "..." --sheet-id "$SHEET_ID" --pivot-table-id "$PIVOT_TABLE_ID" --yes
```

### Validate / DryRun / Execute 约束

- `Validate`：`--url` / `--spreadsheet-token` XOR 必填；update/delete/list 的 `--sheet-id` / `--sheet-name` XOR 必填；create 的 target selector 至多一个、可都省略；`--source` 与合法 `--properties` 必填；delete 强制 `--yes` 或 `--dry-run`。schema 校验类型与枚举，但允许创建空壳配置，业务完整性须靠创建后 list/data 验证。
- `DryRun`：输出将发送的 pivot 请求模板和本地 placement_warning；不联网、不预估实际展开尺寸。
- `Execute`：写后不自动回读；create/update 后必须按落点 sheet + pivot id 轮询 `+pivot-list` 到 loaded，核 error_state/content_range 与数据；delete 后 list 确认目标不存在。

> ⚠️ pivot 输出包含总计 / 小计行；后续 chart 引用 pivot 时，`snapshot.data.refs` 必须排除这些行（见 `references/lark-sheets-chart.md` 的「⚠️ chart 数据源引用 pivot 时必须排除总计行」段）。
