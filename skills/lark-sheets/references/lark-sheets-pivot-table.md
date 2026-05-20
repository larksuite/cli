# Lark Sheet Pivot Table

## 真对象硬约束

当用户要求"透视表 / 分组汇总 / 交叉分析 / 按 X 统计 Y"时，**必须**通过 `+pivot-{create|update|delete}` 创建真实的透视表对象。**禁止**用 `SUMIFS` / `COUNTIFS` 等普通公式 + `+cells-set` 在原表中拼一张"看起来像透视表的汇总表"来代替。判断标准：交付后 `+pivot-list` 必须能返回该对象。

## 使用场景

读写透视表对象。本 Skill 包含两个工具：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 查看已有透视表 | `+pivot-list` | 获取透视表的结构、数据源和配置 |
| 创建/更新/删除透视表 | `+pivot-{create|update|delete}` | 对透视表执行写入操作 |

典型工作流：先读取现有透视表了解配置 → 执行创建/更新/删除 → **必须再次读取验证结果**。

## 行/值字段映射（创建前必做）

创建透视表前先识别用户需求中的分组维度和聚合指标，**不要搞反**：

- **rows（行字段）** = 分组维度，即"按什么分组"。例：部门、地区、医生、产品类别
- **values（值字段）** = 聚合指标，即"统计什么数值"。例：SUM(销售额)、COUNT(订单数)
- **columns（列字段）** = 交叉维度（可选），即"再按什么横向展开"。例：月份、性别

| 用户说 | rows | values | columns |
|--------|------|--------|---------|
| "按部门统计人数" | 部门 | COUNT(姓名) | — |
| "按医生统计费用和结余" | 主管医生 | SUM(费用), SUM(结余) | — |
| "各部门男女人数" | 部门 | COUNT(姓名) | 性别 |

**常见配置错误（必须注意）**：
- **数据源范围必须精确**：透视表的数据源范围必须包含表头行，且精确覆盖全部数据行列。范围过大（包含空行/空列）或过小（遗漏数据列）都会导致透视表结果错误
- **行列字段选择要匹配用户意图**：用户说"按商品统计金额"→ 行字段=商品，值字段=金额（SUM）。不要把行列字段搞反
- **聚合类型要匹配**：用户说"统计数量"→ COUNT；"统计总额"→ SUM；"统计平均"→ AVERAGE。默认不要用 COUNT 替代 SUM
- **参数长度限制**：如果透视表配置 JSON 过长（数据源范围跨越大量行列），可能导致工具调用失败。此时应先确认数据范围的精确边界，避免传入过大的 range
- **创建后必须验证**：调用 `+pivot-list` 确认透视表结构正确

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

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--properties` | string + File + Stdin（复合 JSON） | required | JSON：`{"data_range":"Sheet1!A1:F1000","rows":[...],"columns":[...],"values":[...],"filters":[...],"show_row_grand_total":true,"show_col_grand_total":true}` |
| `--target-sheet-id` | string | optional | 透视表落点子表 id；省略时自动新建子表（推荐） |
| `--target-position` | string | optional | 落点起始 cell（A1 格式，如 `A1`），默认 `A1` |
| `--source` | string | required | 透视表源数据区域（A1 表示法，格式 `SheetName!StartCell:EndCell`，如 `Sheet1!A1:D100`） |
| `--range` | string | optional | 透视表放置位置（左上角 A1 单值，如 `F1`）；省略时放在新建子表的左上角 |

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

> 复合 JSON flag（如 `--cells` / `--properties` / `--operations` / `--border-styles` / `--sort-keys`）的字段速查：只列顶层字段 + 一层嵌套结构。深层结构看 `## Examples` 段的真实示例；要拿完整 JSON Schema 跑 `lark-cli sheets <shortcut> --print-schema --flag-name <name>`。先 `--print-schema`（不带 `--flag-name`）会列出该 shortcut 所有可查询的 flag。

### `+pivot-create` `--properties` / `+pivot-update` `--properties`

_创建/更新的透视表属性_

**顶层字段**：
- `auto_fit_col` (boolean?) — 是否自动调整列宽以适应内容
- `calculated_fields` (array<object>?) — 计算字段列表 each: { formula: string, name: string, summarize_by?: enum }
- `collapse` (object?) — 行字段展开/折叠状态：字段名 -> 要折叠的项目列表
- `columns` (array<object>?) — 横向分组字段（列字段） each: { condition_filter?: object, display_name?: string, field: string, filter?: object, group?: object, …共 6 项 }
- `filters` (array<object>?) — 筛选区域字段（页字段） each: { condition_filter?: object, display_name?: string, field: string, filter?: object, group?: object }
- `range` (string?) — 放置透视表的左上角单元格 A1 地址（例如：'F1'）（仅 create 时有效）
- `repeat_row_labels` (boolean?) — 是否显示重复项标签
- `rows` (array<object>?) — 纵向分组字段（行字段） each: { condition_filter?: object, display_name?: string, field: string, filter?: object, group?: object, …共 6 项 }
- `show_col_grand_total` (boolean?) — 是否显示列总计（默认 true）
- `show_row_grand_total` (boolean?) — 是否显示行总计（默认 true）
- `show_subtotals` (boolean?) — 是否显示分类小计（默认 true，应用于所有字段）
- `source` (string?) — 源数据区域地址，格式为 'SheetName!StartCell:EndCell'（例如：'Sheet1!A1:D100'）
- `values` (array<object>?) — 要汇总的字段（至少需要 1 个） each: { base_field?: string, display_name?: string, field: string, show_data_as?: enum, summarize_by?: enum }

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。`+pivot-create` 默认自动新建子表存放透视表产物（推荐）。

### `+pivot-list`

### `+pivot-create`

> 数据源 `--source` 必须从表头行开始；空行 / 汇总行会被当作数据参与聚合，需提前用 `+csv-get` 确认起止边界。`--source` 和 `--range` 是独立 flag（不要再放 `--properties`）；`rows` / `columns` / `values` 等数组字段走 `--properties`。

```bash
lark-cli sheets +pivot-create --url "..." --sheet-id "$SRC_SID" \
  --source "Sheet1!A1:D100" --range "F1" --properties @pivot.json
```

### `+pivot-update`

> 不允许改 `--source` / `--range`（透视表创建后位置/数据源固定）；只能用 `--properties` 改 rows / columns / values / filters 等。先 `+pivot-list --pivot-table-id <id>` 回读再 patch，避免漏字段。

### `+pivot-delete`

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`+pivot-create` 的 `--source` 必填且必须含表头行；`--properties` 中 `rows` / `columns` / `values` 至少非空之一；`+pivot-delete` 强制 `--yes` 或 `--dry-run`。
- `DryRun`：写操作输出"将要 POST/PATCH/DELETE 的 pivot 请求模板"+ 预估输出尺寸（行数 × 列数）。
- `Execute`：写后调用 `+pivot-list --pivot-table-id <id>` 回读 + `+csv-get` 抽样读透视产物，envelope.meta.verification 给出实际输出尺寸 + 总计行位置。

> ⚠️ pivot 输出包含总计 / 小计行；后续 chart 引用 pivot 时，`data_range` 必须排除这些行（见 `lark-sheets-chart` 决策段）。
