# Lark Sheet Filter

## 真对象硬约束 + 数量校验

1. **真对象**：当用户要求"筛选 / 只看 / 仅保留 X"时，**必须**通过 `+filter-{create|update|delete}` 创建真实的筛选器对象。**禁止**用"删除不符合条件的行" / "新建子表只放符合条件的行" / 用 `+cells-set` 覆盖原表来代替——这些做法会让原数据丢失或不可恢复。
2. **筛选数量必校**：执行筛选后**必须**回读，断言 `len(visible_rows) == expected_count`。`expected_count` 来自先用 本地脚本 在源数据上独立复现该筛选条件得到的结果数。两者不一致时禁止交付，需排查筛选条件 / 数据列类型问题。
3. **混合文本列禁止字面比较**：筛选 key 是公式文本（如 `1000+200=1200`）或带单位的混合文本时，先在辅助列里抽出纯数值再筛选；不能直接用文本比较。

## 使用场景

读写筛选器对象。本 Skill 包含两个工具：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 查看已有筛选器 | `+filter-list` | 获取筛选器的范围、规则和条件配置 |
| 创建/更新/删除筛选器 | `+filter-{create|update|delete}` | 对筛选器执行写入操作 |

典型工作流：先读取现有筛选器了解配置 → 执行创建/更新/删除 → **必须再次读取验证结果**。

**只读场景例外**：用户只是想知道哪些数据满足条件、并不要求修改表格展示时，可以走 `lark-sheets-read-data` 读后文本回答，不必创建筛选器。

**常见配置错误（必须注意）**：
- **筛选范围必须覆盖表头行**：筛选器的 range 必须从表头行开始（如 `A1:F100`），不能只包含数据行。缺少表头会导致筛选条件无法正确匹配列
- **更新已有筛选器前先读取**：如果子表上已存在筛选器，直接创建会报错或覆盖原有配置。应先用 `+filter-list` 查看是否存在筛选器，存在时使用 update 而非 create
- **筛选条件的列索引要精确**：筛选条件中的列标识必须与实际数据列精确对应，不要凭猜测填写
- **”调整筛选逻辑”要先读旧配置**：用户说”调整筛选”时，先读取现有筛选器的完整配置，理解当前规则后再修改，不要从零创建
- **创建后必须验证**：调用 `+filter-list` 确认筛选器配置正确且生效
- **筛选不支持正则表达式**：飞书表格筛选器不支持正则表达式，传入正则会当成普通文本处理。

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+filter-list` | read | 对象 |
| `+filter-create` | write | 对象 |
| `+filter-update` | write | 对象 |
| `+filter-delete` | high-risk-write | 对象 |

## Flags

### `+filter-list`

_公共四件套 · 系统：`--dry-run`_

_仅含公共 / 系统 flag。_

### `+filter-create`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | 是 | 筛选范围（A1 表示法，含表头行，如 `A1:F1000`）；不要重复写入 --data 中的 range 字段 |
| `--properties` | string + File + Stdin（复合 JSON） | 否 | +filter-create / --data: 筛选规则 JSON，含 `rules`（列级筛选规则数组，必填）和 `filtered_columns?`（激活列索引提示）。`range` 是独立 flag（不要再放此 JSON 里） |

### `+filter-update`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--properties` | string + File + Stdin（复合 JSON） | 是 | +filter-update / --data: 筛选规则 JSON，含 `rules` 和 `filtered_columns?`；update 是整组覆盖式（传空 `rules: []` 清空）。`range` 已拎为独立 flag |
| `--range` | string | 是 | 筛选作用的单元格范围（A1 表示法，如 `A1:F1000`）；优先级高于 `--data` 中同名字段 |

### `+filter-delete`

_公共四件套 · 系统：`--yes`、`--dry-run`_

_仅含公共 / 系统 flag。_

## Schemas

> 复合 JSON flag（如 `--cells` / `--properties` / `--operations` / `--border-styles` / `--sort-keys`）的字段速查：只列顶层字段 + 一层嵌套结构。深层结构看 `## Examples` 段的真实示例；要拿完整 JSON Schema 跑 `lark-cli sheets <shortcut> --print-schema --flag-name <name>`。先 `--print-schema`（不带 `--flag-name`）会列出该 shortcut 所有可查询的 flag。

### `+filter-create` `--properties` / `+filter-update` `--properties`

_创建/更新的筛选器属性_

**顶层字段**：
- `filtered_columns` (array<string>?) — 可选
- `range` (string) — 筛选对象作用的单元格范围（A1 表示法）
- `rules` (array<object>) — 列级筛选规则列表，每一项对应一个具体列的筛选条件 each: { column_index: string, conditions: array<oneOf>, filtered_rows?: array<number> }

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。`filter_id` 等同于 `sheet_id`（每个工作表至多一个筛选器）。

### `+filter-list`

### `+filter-create`

`--range` 是独立 flag（含表头行）；`rules` 走 `--properties`：

```bash
lark-cli sheets +filter-create --url "..." --sheet-id "$SID" \
  --range "A1:F1000" \
  --properties '{"rules":[{"col":"B","filter_type":"multiValue","expected":["北京","上海"]}]}'
```

### `+filter-update`

> ⚠️ update 是覆盖式：`--properties` 中传新 `rules` 会替换旧组。如只想加一条，要带上已有的全部条件再追加。必填 `--range`。

### `+filter-delete`

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`+filter-create` 校验 `--range` 至少 2 行（表头 + 至少 1 行数据）；`+filter-update` 必须先 `+filter-list` 确认目标存在；`+filter-delete` 强制 `--yes` 或 `--dry-run`。
- `DryRun`：输出"将要 POST/PATCH/DELETE 的 filter 请求模板"。
- `Execute`：写后调用 `+filter-list` 回读，envelope.meta.verification 给出当前筛选条件 + 已过滤行数。
