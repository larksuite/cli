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
| `--group-id` | string | 否 | 按 group_id 过滤 |

### `+sparkline-create`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--properties` | string + File + Stdin（复合 JSON） | 是 | JSON：`{"type":"line\|column\|winLoss","data_range":"A2:F10","target_range":"G2:G10","style":{...},"special_points":{...}}`；type 三种 enum；data_range 与 target_range 行/列数需对齐 |

### `+sparkline-update`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--group-id` | string | 是 | 目标组 id |
| `--properties` | string + File + Stdin（复合 JSON） | 是 | 完整或足够完整的配置（先 `+sparkline-list --group-id <id>` 回读再 patch）；可改 `type` / `data_range` / `target_range` / `style` / `special_points` 等字段 |

### `+sparkline-delete`

_公共四件套 · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--group-id` | string | 是 | 目标组 id |

## Schemas

> 复合 JSON flag（如 `--cells` / `--properties` / `--operations` / `--border-styles` / `--sort-keys`）的字段速查：只列顶层字段 + 一层嵌套结构。深层结构看 `## Examples` 段的真实示例；要拿完整 JSON Schema 跑 `lark-cli sheets <shortcut> --print-schema --flag-name <name>`。先 `--print-schema`（不带 `--flag-name`）会列出该 shortcut 所有可查询的 flag。

### `+sparkline-create` `--properties` / `+sparkline-update` `--properties`

_创建/更新/部分删除的迷你图属性_

**顶层字段**：
- `config` (object?) — 迷你图样式配置, 相同 groupId 的迷你图共享相同的样式 { axis?: object, contain_hidden_cells?: boolean, empty_show_as?: enum, extremum_max?: object, extremum_min?: object, …共 13 项 }
- `sparklines` (array<object>?) — 迷你图项列表 each: { position?: object, source?: string, source_range?: object, sparkline_id?: string }

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。迷你图按 `group_id` 管理——一组同形态的迷你图共享类型 / 样式 / 数据源映射。注意：不等同于已禁用的 `SPARKLINE()` 公式函数。

### `+sparkline-list`

### `+sparkline-create`

> `data_range` 是每个迷你图的数据序列；`target_range` 是迷你图生成的目标 cells（通常每个 cell 一个迷你图）。

```bash
lark-cli sheets +sparkline-create --url "..." --sheet-id "$SID" --properties @sparkline.json
```

### `+sparkline-update`

### `+sparkline-delete`

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`--properties.type` 必须命中 enum（`line` / `column` / `winLoss`）；`--properties.data_range` 与 `--properties.target_range` 行/列数需对齐；`+sparkline-delete` 强制 `--yes` 或 `--dry-run`。
- `DryRun`：写操作输出"将要 POST/PATCH/DELETE 的 sparkline group 请求模板"。
- `Execute`：写后调用 `+sparkline-list --group-id <id>` 回读，envelope.meta.verification 给出 type / style / 生成范围对比。
