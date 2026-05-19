# Lark Sheet Filter View

## 概念回顾

筛选视图是 sheet 内的多份独立筛选配置，每个视图持有自己的 `range` 和 `rules`，由独立 `view_id`（10 位随机字符串）标识。一个 sheet 可有多个视图，视图的隐藏行仅在用户进入该视图时本地生效，不影响其他协作者，也不与该 sheet 上可能并存的筛选器（filter）互相影响。

`+filter-{view-create|view-update|view-delete}` 负责视图本身的 CRUD（create / update / delete）；视图的"进入 / 退出"（激活态）是本地状态，不在工具语义内。

## 使用场景

读写筛选视图对象。本 Skill 包含两个工具：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 查看已有筛选视图 | `+filter-view-list` | 获取 sheet 上所有视图（视图名、范围、规则） |
| 创建 / 更新 / 删除筛选视图 | `+filter-{view-create|view-update|view-delete}` | 3 种 operation：create / update / delete |

典型工作流：先读取现有视图了解配置 → 执行创建 / 更新 / 删除 → **必须再次读取验证结果**。

**常见配置错误（必须注意）**：
- **视图范围必须覆盖表头行**：视图的 range 必须从表头行开始（如 `A1:F100`），不能只包含数据行
- **更新前先读取**：用户说"调整这个视图"时，先用 `+filter-view-list` 拉到目标视图当前 rules，**只改差异列**再回写
- **多次 create 不能复用 view_id**：复用应走 `update`，重复 `create` 会产生新视图
- **筛选不支持正则表达式**：飞书表格筛选器不支持正则表达式，传入正则会当成普通文本处理

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+filter-view-list` | read | 对象 |
| `+filter-view-create` | write | 对象 |
| `+filter-view-update` | write | 对象 |
| `+filter-view-delete` | high-risk-write | 对象 |

## Flags

### `+filter-view-list`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--view-id` | string | 否 | 按筛选视图 reference_id 过滤（命中即只返回单个视图） |

### `+filter-view-create`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--properties` | string + File + Stdin（复合 JSON） | 是 | +filter-view-create / --data: 视图规则 JSON，含 `rules?`（列级筛选规则数组）和 `filtered_columns?`。`range` 和 `view_name` 已拎为独立 flag |
| `--range` | string | 是 | 筛选作用的单元格范围（A1 表示法，如 `A1:F1000`）；优先级高于 `--data` 中同名字段（create 必填，必须覆盖表头行） |
| `--view-name` | string | 否 | 视图名称；create 不传时系统自动分配，update 不传时保留原名。优先级高于 `--data` 中同名字段 |

### `+filter-view-update`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--view-id` | string | 是 | 目标视图 reference_id |
| `--properties` | string + File + Stdin（复合 JSON） | 是 | +filter-view-update / --data: 视图规则 JSON，含 `rules?` 和 `filtered_columns?`。`range` 和 `view_name` 已拎为独立 flag；至少传 `--data.rules` / `--range` / `--view-name` 之一 |
| `--range` | string | 否 | 筛选作用的单元格范围（A1 表示法，如 `A1:F1000`）；优先级高于 `--data` 中同名字段（update 时省略表示保留当前 range） |
| `--view-name` | string | 否 | 视图名称；create 不传时系统自动分配，update 不传时保留原名。优先级高于 `--data` 中同名字段 |

### `+filter-view-delete`

_公共四件套 · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--view-id` | string | 是 | 目标视图 reference_id |

## Schemas

> 复合 JSON flag（如 `--cells` / `--properties` / `--operations` / `--border-styles` / `--sort-keys`）的字段速查：只列顶层字段 + 一层嵌套结构。深层结构看 `## Examples` 段的真实示例；要拿完整 JSON Schema 跑 `lark-cli sheets <shortcut> --print-schema --flag-name <name>`。先 `--print-schema`（不带 `--flag-name`）会列出该 shortcut 所有可查询的 flag。

### `+filter-view-create` `--properties` / `+filter-view-update` `--properties`

_create / update 的视图属性_

**顶层字段**：
- `filtered_columns` (array<string>?) — 可选
- `range` (string?) — 视图作用的单元格范围（A1 表示法）
- `rules` (array<object>?) — 列级筛选规则列表，每一项对应一个具体列的筛选条件 each: { column_index: string, conditions: array<oneOf>, filtered_rows?: array<number> }
- `view_name` (string?) — 可选

## Examples

> ⚠️ 本 skill 是 **CLI 独有**（meta `surface: cli-only`）；`generate_mcp` 跳过，不会进 sheet-ai-skills SKILL 集。AI/MCP 侧暂不暴露筛选视图能力。

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。`view_id` 是 10 位随机字符串，每个 sheet 可有多个视图。

### `+filter-view-list`

```bash
# 列出某个 sheet 的全部筛选视图
lark-cli sheets +filter-view-list --url "..." --sheet-id "$SID"

# 按 view_id 精确定位
lark-cli sheets +filter-view-list --url "..." --sheet-id "$SID" --view-id vAbcde1234
```

### `+filter-view-create`

`--range`（必填）/ `--view-name`（可选）是独立 flag；`rules` 走 `--properties`：

```bash
lark-cli sheets +filter-view-create --url "..." --sheet-id "$SID" \
  --view-name "活跃用户" --range "A1:F1000" \
  --properties '{"rules":[{"col":"C","filter_type":"number","compare":"greater","expected":[100]}]}'
```

> `--range` **必须覆盖表头行**（如 `A1:F1000`），不能只包含数据行；`--view-name` 重名时服务端自动改名。

### `+filter-view-update`

> ⚠️ update 是 patch：`--view-name` / `--range` / `--properties.rules` 任传一个或多个；至少传一个。先 `+filter-view-list` 读取当前 rules 再回写差异。重复 `+filter-view-create` 不会复用 view_id，会产生新视图。

### `+filter-view-delete`

> ⚠️ 视图删除不可逆；视图不存在按幂等成功处理。先 `--dry-run` 看 view_id 确认。

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`+filter-view-create` 校验 `--range` 起始行为表头（第一行）；`+filter-view-update` 必须先 `+filter-view-list` 确认 view 存在，且 `--view-name` / `--range` / `--properties` 至少传一个；`+filter-view-delete` 强制 `--yes` 或 `--dry-run`。
- `DryRun`：输出"将要 POST/PATCH/DELETE 的 view 请求模板"，零网络副作用；`--sheet-name` 在 dry-run 输出里生成为 `<resolve:Sheet1>` 占位符。
- `Execute`：写后调用 `+filter-view-list --view-id <new>` 回读，envelope.meta.verification 给出当前 range + rules 与请求体的对比。
