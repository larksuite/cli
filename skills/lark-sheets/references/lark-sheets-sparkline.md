# Lark Sheet Sparkline

## 真对象硬约束

当用户要求"迷你图 / 趋势线 / 单元格内图表"时，**必须**通过 `+sparkline-{create|update|delete}` 创建真实的迷你图对象。**禁止**用文本字符（如 `▁▂▃▅▇`）拼接在单元格里、或用 `SPARKLINE()` 公式函数（已禁用）代替。判断标准：交付后 `+sparkline-list` 必须能返回该对象。

## 使用场景

读写迷你图对象。本 Skill 包含两个工具：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 查看已有迷你图 | `+sparkline-list` | 获取迷你图的类型、数据源和样式配置 |
| 创建/更新/删除迷你图 | `+sparkline-{create|update|delete}` | 对迷你图执行写入操作 |

典型工作流：先读取现有迷你图了解配置 → 执行创建/更新/删除 → **必须再次读取验证结果**。

**常见配置错误（必须注意）**：
- **数据源范围要精确**：迷你图的数据源范围必须与实际数据行列精确对应，范围偏移会导致图形展示错误
- **不要与 SPARKLINE() 公式混淆**：飞书表格的 `SPARKLINE()` 公式函数已被禁用，迷你图只能通过本 Skill 的对象方式创建
- **创建后必须验证**：调用 `+sparkline-list` 确认迷你图配置正确

## Shortcuts

| MCP tool | CLI shortcut | Risk | 分组 |
| --- | --- | --- | --- |
| `get_sparkline_objects` | `+sparkline-list` | read | 对象 |
| `manage_sparkline_object` | `+sparkline-create` | write | 对象 |
|  | `+sparkline-update` | write | 对象 |
|  | `+sparkline-delete` | high-risk-write | 对象 |

## Flags

### `+sparkline-list`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--sheet-id` | 公共 | string | XOR | 工作表 reference_id（与 `--sheet-name` 二选一） |
| `--sheet-name` | 公共 | string | XOR | 工作表名称（与 `--sheet-id` 二选一） |
| `--group-id` | 专有 | string | 否 | 按 group_id 过滤 |
| `--dry-run` | 系统 | bool | 否 |  |

### `+sparkline-create`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--sheet-id` | 公共 | string | XOR | 工作表 reference_id（与 `--sheet-name` 二选一） |
| `--sheet-name` | 公共 | string | XOR | 工作表名称（与 `--sheet-id` 二选一） |
| `--config` | 专有 | string + File + Stdin（复合 JSON） | 否 | 迷你图共享样式配置 JSON：`{"type":"line\|column\|winLoss","series_color":"#4472C4","line_width":2,"axis":{...},"extremum_max":{...},...}`。同组 sparkline 共享一份 config；省略时取默认样式（type 默认 line） |
| `--sparklines` | 专有 | string + File + Stdin（复合 JSON） | 是 | 迷你图列表 JSON：`[{"position":"G2","source":"A2:F2"}, {"position":"G3","source":"A3:F3"}]`。每项必填 `position`（目标单元格）+ `source`（数据序列范围）；create 时至少 1 条 |
| `--dry-run` | 系统 | bool | 否 |  |

### `+sparkline-update`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--sheet-id` | 公共 | string | XOR | 工作表 reference_id（与 `--sheet-name` 二选一） |
| `--sheet-name` | 公共 | string | XOR | 工作表名称（与 `--sheet-id` 二选一） |
| `--group-id` | 专有 | string | 是 | 目标组 id |
| `--config` | 专有 | string + File + Stdin（复合 JSON） | 否 | 更新整组共享样式（patch 模式）。结构同 `+sparkline-create --config`；先 `+sparkline-list --group-id <id>` 回读再 patch。与 `--sparklines` 至少传一个 |
| `--sparklines` | 专有 | string + File + Stdin（复合 JSON） | 否 | 更新 / 新增 / 删除迷你图项 JSON 数组。每项需带 `sparkline_id`；`upsert=true` 时无 id 项按新增处理（必填 position + source）。与 `--config` 至少传一个 |
| `--dry-run` | 系统 | bool | 否 |  |

### `+sparkline-delete`

| Flag | 分类 | Type | 必填 | 说明 |
| --- | --- | --- | --- | --- |
| `--url` | 公共 | string | XOR | spreadsheet URL（与 `--spreadsheet-token` 二选一） |
| `--spreadsheet-token` | 公共 | string | XOR | spreadsheet token（与 `--url` 二选一） |
| `--sheet-id` | 公共 | string | XOR | 工作表 reference_id（与 `--sheet-name` 二选一） |
| `--sheet-name` | 公共 | string | XOR | 工作表名称（与 `--sheet-id` 二选一） |
| `--group-id` | 专有 | string | 是 | 目标组 id |
| `--yes` | 系统 | bool | 是 | `high-risk-write`，删除不可逆 |
| `--dry-run` | 系统 | bool | 否 |  |

## Schemas

> 复合 JSON flag（`--data` / `--style` / `--options` / `--sort-keys`）的字段速查：只列顶层字段 + 一层嵌套结构。深层结构看 `## Examples` 段的真实示例；要拿完整 JSON Schema 跑 `lark-cli sheets <shortcut> --print-schema --flag <name>`（runtime introspection，待落地）。

### `+sparkline-create` `--config` / `+sparkline-update` `--config`

_迷你图样式配置, 相同 groupId 的迷你图共享相同的样式_

**顶层字段**：
- `axis` (object?) — 坐标轴配置，包含坐标轴颜色、是否翻转、是否显示坐标轴 { color?: string, reverse?: boolean, visible?: boolean }
- `contain_hidden_cells` (boolean?) — 隐藏的单元格数据是否参与绘制
- `empty_show_as` (enum?) — 空单元格显示方式：zero=显示为0，gap=显示为间距，average=取前后均值 [zero / gap / average]
- `extremum_max` (object?) — 最大极值配置，包含极值类型、极值 { type: enum, value?: number }
- `extremum_min` (object?) — 最小极值配置，包含极值类型、极值 { type: enum, value?: number }
- `line_width` (enum?) — 折线图线宽，可选值：1=1px，2=2px，3=3px，4=4px [1 / 2 / 3 / 4]
- `non_num_show_as` (enum?) — 非数字单元格显示方式：zero=显示为0，gap=显示为间距，average=取前后均值 [zero / gap / average]
- `points` (object?) — 特殊点样式配置，包含高点、低点、标记点、首点、尾点、负点 { first_point?: object, high_point?: object, last_point?: object, low_point?: object, markers_point?: object, …共 6 项 }
- `series_color` (string?) — 主系列颜色，例如 "#4472C4"
- `show_gradient` (boolean?) — 是否显示渐变效果
- `show_radius` (boolean?) — 是否显示圆角，仅对柱形图和盈亏图生效
- `theme_type` (enum?) — 主题类型：pro、light、soft、brand、fresh [pro / light / soft / brand / fresh]
- `type` (enum?) — 迷你图类型，可选值：line=折线图，column=柱形图，win_loss=盈亏图 [line / column / win_loss]

### `+sparkline-create` `--sparklines` / `+sparkline-update` `--sparklines`

_迷你图项列表_

**数组项**（类型 object）：
- `position` (object?) — 迷你图位置 { col: string, row: number }
- `source` (string?) — A1 范围字符串，表示数据来源，例如 "Sheet1!A2:A10"
- `source_range` (object?) — 结构化数据源范围（与 source 等价） { range: string }
- `sparkline_id` (string?) — 迷你图 reference_id

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。迷你图按 `group_id` 管理——一组同形态的迷你图共享类型 / 样式 / 数据源映射。注意：不等同于已禁用的 `SPARKLINE()` 公式函数。

### `+sparkline-list`

### `+sparkline-create`

> `--config` 与 `--sparklines` 拆为独立 flag（同 chart 拎 position/offset/size 的拆法）：
> - `--config`（可选）整组共享样式：`type` enum `line` / `column` / `winLoss`，可选 `series_color` / `line_width` / `theme_type` / `axis` / `extremum_max` / `extremum_min` / `points` 等；省略时取默认（type 默认 line）
> - `--sparklines`（必填）每项一条迷你图，必填 `position`（目标 cell，如 `G2`）+ `source`（数据序列范围，如 `A2:F2`）；至少 1 条

```bash
# 折线迷你图组，G2 / G3 / G4 分别绘制 A2:F2 / A3:F3 / A4:F4
lark-cli sheets +sparkline-create --url "..." --sheet-id "$SID" \
  --config @config.json --sparklines @items.json

# config.json:
# { "type": "line", "series_color": "#4472C4", "line_width": 2 }
#
# items.json:
# [
#   { "position": "G2", "source": "A2:F2" },
#   { "position": "G3", "source": "A3:F3" },
#   { "position": "G4", "source": "A4:F4" }
# ]

# 取默认样式（省略 --config），inline 列表
lark-cli sheets +sparkline-create --url "..." --sheet-id "$SID" \
  --sparklines '[{"position":"G2","source":"A2:F2"}]'
```

### `+sparkline-update`

> update 是 patch：先 `+sparkline-list --group-id <id>` 拿到当前 `config` + `sparklines`，再分别按需 patch。`--config` 改整组样式；`--sparklines` 增删 / 修改迷你图项（每项需带 `sparkline_id`）。至少传一个。

```bash
# 只改整组颜色，不动单条
lark-cli sheets +sparkline-update --url "..." --sheet-id "$SID" --group-id grpXXX \
  --config '{"series_color":"#E64545"}'

# 只增删单条迷你图
lark-cli sheets +sparkline-update --url "..." --sheet-id "$SID" --group-id grpXXX \
  --sparklines @patched-items.json
```

### `+sparkline-delete`

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`--config.type` 若提供必须命中 enum（`line` / `column` / `winLoss`）；`--sparklines` 必须非空数组（create 时必填，update 时与 `--config` 至少传一个），每项 `position` + `source` 必填；同组迷你图 `position` 不得重复；`+sparkline-delete` 强制 `--yes` 或 `--dry-run`。
- `DryRun`：写操作输出"将要 POST/PATCH/DELETE 的 sparkline group 请求模板"。
- `Execute`：写后调用 `+sparkline-list --group-id <id>` 回读，envelope.meta.verification 给出 type / style / 生成范围对比。
