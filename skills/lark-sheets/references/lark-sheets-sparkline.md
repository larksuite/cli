# Lark Sheet Sparkline

## 真对象硬约束

当用户要求"迷你图 / 趋势线 / 单元格内图表"时，**必须**通过 `+sparkline-{create|update|delete}` 创建真实的迷你图对象。**禁止**用文本字符（如 `▁▂▃▅▇`）拼接在单元格里、或用 `SPARKLINE()` 公式函数（已禁用）代替。判断标准：交付后 `+sparkline-list` 必须能返回该对象。

## 使用场景

读写迷你图对象。本 reference 覆盖 4 个 shortcut：

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

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+sparkline-list` | read | 对象 |
| `+sparkline-create` | write | 对象 |
| `+sparkline-update` | write | 对象 |
| `+sparkline-delete` | high-risk-write | 对象 |

## Flags

### `+sparkline-list`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--group-id` | string | optional | 按 group_id 过滤 |

### `+sparkline-create`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--properties` | string + File + Stdin（复合 JSON） | required | JSON：`{config（共享样式配置）, sparklines（迷你图数组）}`；完整字段结构跑 `--print-schema` |

### `+sparkline-update`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--group-id` | string | required | 目标组 id |
| `--properties` | string + File + Stdin（复合 JSON） | required | JSON：`{config, sparklines}`；先 `+sparkline-list --group-id <id>` 回读再 patch；完整字段结构跑 `--print-schema` |

### `+sparkline-delete`

_公共四件套 · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--group-id` | string | required | 目标组 id |

## Schemas

> 复合 JSON flag 字段速查（只列顶层 + 一层嵌套）。深层结构看下方 `## Examples`，或用 `--print-schema` 读完整 JSON Schema（用法见 SKILL.md「公共 flag 速查」与「Agent 使用提示」）。

### `+sparkline-create` `--properties` / `+sparkline-update` `--properties`

_创建/更新/部分删除的迷你图属性_

**顶层字段**：
- `config` (object?) — 迷你图样式配置, 相同 groupId 的迷你图共享相同的样式 { theme_type?: enum, non_num_show_as?: enum, empty_show_as?: enum, contain_hidden_cells?: boolean, series_color?: string, …共 13 项 }
- `sparklines` (array<object>?) — 迷你图项列表 each: { sparkline_id?: string, position?: object, source?: string, source_range?: object }

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。迷你图用 **两层 id** 管理——`group_id` 选组（一组同形态的迷你图共享类型 / 样式 / 数据源映射），`sparkline_id` 在组内选具体某一项。注意：不等同于已禁用的 `SPARKLINE()` 公式函数。

> **何时需要先 `+sparkline-list`：**
> - `+sparkline-update`：**总是**需要——拿到组内每一项的 `sparkline_id`，回填到 `properties.sparklines[i]`，server 用它做映射。
> - `+sparkline-delete`：**不需要** `sparkline_id`——CLI 仅支持按 `--group-id` 整组删除（该 shortcut 没有 `--properties`）。

### `+sparkline-list`

```bash
# 列出整张子表的所有迷你图组
lark-cli sheets +sparkline-list --url "..." --sheet-id "$SID"

# 钉到单组：返回该组每一项的 sparkline_id（update / partial-delete 必需）
lark-cli sheets +sparkline-list --url "..." --sheet-id "$SID" --group-id "grpA"
```

### `+sparkline-create`

> `--properties` 顶层只有 `config`（同组共享样式，如 `line_width` / `points` / `extremum_max` / `extremum_min`）和 `sparklines`（迷你图项数组）两个字段。`sparklines[i]` 每项必须含 `position`（落点 cell，`row` + `col`）+ `source`（数据 A1 范围，与 `source_range` 二选一）；create 时 `sparkline_id` 可省略，由系统生成。

```bash
lark-cli sheets +sparkline-create --url "..." --sheet-id "$SID" --properties @sparkline.json
```

`sparkline.json` 示例（在 F 列嵌入两行折线迷你图，数据分别来自 A2:E2 和 A3:E3）：

```jsonc
{
  "config": { "line_width": 2 },
  "sparklines": [
    {"position": {"row": 1, "col": "F"}, "source": "'Sheet1'!A2:E2"},
    {"position": {"row": 2, "col": "F"}, "source": "'Sheet1'!A3:E3"}
  ]
}
```

### `+sparkline-update`

> 两步式：先 `+sparkline-list --group-id <id>` 拿当前组的 `sparkline_id` 列表，再构造 `properties.sparklines[]`——**每项必须带 `sparkline_id`**。只改样式可只传 `properties.config`（不带 `sparklines`，整组样式覆盖式更新）。

```bash
# 假设 +sparkline-list 已返回 group_id=grpA，组内 sparkline_id=sl_1 / sl_2
lark-cli sheets +sparkline-update --url "..." --sheet-id "$SID" --group-id "grpA" --properties '{
  "sparklines": [
    {"sparkline_id":"sl_1","source":"'Sheet1'!A2:A20"},
    {"sparkline_id":"sl_2","source":"'Sheet1'!B2:B20"}
  ]
}'
```

### `+sparkline-delete`

> CLI 仅支持**整组删除**：传 `--group-id` 删掉该组全部迷你图。该 shortcut **没有** `--properties`，无法只删组内单项（需求上要"留一部分"时，改用 `+sparkline-update` 重写该组的 `sparklines` 列表，而不是 delete）。强制 `--yes` 或 `--dry-run`；先 `--dry-run` 确认要删的目标组。

```bash
# 删整组
lark-cli sheets +sparkline-delete --url "..." --sheet-id "$SID" --group-id "grpA" --yes
```

### Validate / DryRun / Execute 约束

- `Validate`：
  - XOR 公共四件套；`+sparkline-{update,delete}` 必须 `--group-id`。
  - **`+sparkline-update`**：当 `properties.sparklines` 非空时，每一项必须含 `sparkline_id`（CLI 预检，错误信息会指回 `+sparkline-list`，避免命中服务端的不可读拒绝）；只传 `properties.config`（config-only update）合法、不触发 sparkline_id 检查。
  - **`+sparkline-delete`**：只接 `--group-id`（整组删除），**没有** `--properties`，无法删组内单项。
  - `--properties`（仅 `+sparkline-create` / `+sparkline-update`）顶层只接 `config`（同组共享样式）和 `sparklines`（迷你图项数组）；`+sparkline-create` 要求每个 `sparklines[i]` 含 `position` 与 `source`（或 `source_range`，二选一）。
  - `+sparkline-delete` 强制 `--yes` 或 `--dry-run`。
- `DryRun`：写操作输出"将要 POST/PATCH/DELETE 的 sparkline group 请求模板"。
- `Execute`：写后不自动回读；如需确认，自行调用 `+sparkline-list --group-id <id>` 查看 `config` / `sparklines`。
