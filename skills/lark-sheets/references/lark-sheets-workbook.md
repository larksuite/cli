# Lark Sheet Workbook

## Sheet 结构变更保守化（编辑类任务必做）

`+sheet-{create|delete|rename|move|copy|hide|unhide|set-tab-color}` 会改变原表的物理结构，是高副作用动作。执行前必须遵守：

1. **删除 / 重命名 / 隐藏 / 移动原 Sheet 需用户明示**：除非用户明示要这些操作，**禁止**擅自对**已存在**的 Sheet 执行 delete / rename / hide / move。新建 Sheet 是允许的（用于承载中间结果或透视表 / 图表对象），但应优先在原表右侧加列；只有当中间结果数量较大或会与原数据混淆时，才新建空白 Sheet（同 R1）。
2. **Sheet 级操作前先列清单**：调用 `+sheet-{create|delete|rename|move|copy|hide|unhide|set-tab-color}` 之前，必须先调用 `+workbook-info`，把"当前所有 Sheet 名 + 可见性 + 行列数"列出来，再决定是否操作。禁止跳过列清单直接 create / delete / rename。
3. **删除 / 重命名前向用户确认**：删除是不可逆的，重命名会让其他公式 / 透视表 / 图表的数据源失效——执行前必须在回复里确认"将删除 / 改名 X，影响 Y 个引用"。

## 使用场景

读写。管理工作簿结构。本 Skill 包含两个工具：

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

| MCP tool | CLI shortcut | Risk | 分组 |
| --- | --- | --- | --- |
| `get_workbook_structure` | `+workbook-info` | read | 工作簿 |
| `modify_workbook_structure` | `+sheet-create` | write | 工作簿 |
|  | `+sheet-delete` | high-risk-write | 工作簿 |
|  | `+sheet-rename` | write | 工作簿 |
|  | `+sheet-move` | write | 工作簿 |
|  | `+sheet-copy` | write | 工作簿 |
|  | `+sheet-hide` | write | 工作簿 |
|  | `+sheet-unhide` | write | 工作簿 |
|  | `+sheet-set-tab-color` | write | 工作簿 |
| `create_workbook` | `+workbook-create` | write | 工作簿 |
| `export_workbook` | `+workbook-export` | read | 工作簿 |

## Flags

### `+workbook-info`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--include-properties` | bool | 否 | 是否返回每个 sheet 的扩展属性（默认 true） |

### `+sheet-create`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--title` | string | 是 | 新工作表名称 |
| `--index` | int | 否 | 插入位置；省略时附加到末尾 |
| `--row-count` | int | 否 | 初始行数，默认 100 |
| `--col-count` | int | 否 | 初始列数，默认 26 |

### `+sheet-delete`

_公共四件套 · 系统：`--yes`、`--dry-run`_

_仅含公共 / 系统 flag。_

### `+sheet-rename`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--title` | string | 是 | 新名称 |

### `+sheet-move`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--index` | int | 是 | 目标位置（0-based） |
| `--source-index` | int | 否 | 源位置（0-based）；可选，未传时由 CLI runtime 根据 --sheet-id / --sheet-name 当前在工作簿中的 index 自动派生 |

### `+sheet-copy`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--title` | string | 否 | 副本名称；省略时由服务端生成 |
| `--index` | int | 否 | 副本插入位置 |

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
| `--color` | string | 是 | Hex 色值如 `#FF0000`，传空 `""` 清除 |

### `+workbook-create`

_系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--title` | string | 是 | 新 spreadsheet 标题 |
| `--folder-token` | string | 否 | 目标文件夹 token；省略放根目录 |
| `--headers` | string + File + Stdin（简单 JSON） | 否 | 表头行 JSON 数组：`["列A","列B"]` |
| `--values` | string + File + Stdin（简单 JSON） | 否 | 初始数据 JSON 二维数组：`[["alice",95]]` |

### `+workbook-export`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--file-extension` | string + Enum | 否 | `xlsx` / `csv`，默认 `xlsx`；csv 模式必须配 `--sheet-id` |
| `--sheet-id` | string | 否 | 仅 csv 模式必填：指定要导出的 sheet reference_id |
| `--output-path` | string | 否 | 本地保存路径；省略只触发导出不下载 |

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。`+workbook-info` 只用前两者；`+sheet-*` 系列对单个工作表操作，需 `--sheet-id` 或 `--sheet-name`。

### `+workbook-info`

输出契约：返回 `sheets[]`，每个含 `sheet_id` / `title` / `row_count` / `column_count` / `frozen_row_count` / `frozen_col_count` / `index` / `hidden`。是操作飞书表格的第一步——任何后续 sheet 级动作都需要先拿这里的 sheet_id。

### `+sheet-create`

示例：

```bash
lark-cli sheets +sheet-create --url "https://example.feishu.cn/sheets/shtXXX" \
  --title "汇总" --index 0
```

### `+sheet-delete`

> ⚠️ 工作表删除不可逆；先 `--dry-run` 看输出 sheet_id + title 确认是要删的那张。

### `+sheet-rename`

### `+sheet-move`

### `+sheet-copy`

### `+sheet-hide` / `+sheet-unhide`

### `+sheet-set-tab-color`

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`+sheet-create` 校验 `--title` 非空、`--row-count` ≤ 50000、`--col-count` ≤ 200；`+sheet-delete` 必须 `--yes` 或 `--dry-run`。
- `DryRun`：`+sheet-*` 写操作输出"将要 PATCH 的 sheet metadata"；`--sheet-name` 在 dry-run 输出里生成为 `<resolve:Sheet1>` 占位符，不实际解析为 sheet-id。
- `Execute`：所有写操作执行后自动调用 `+workbook-info` 回读，envelope.meta.verification 包含目标 sheet 的新状态。
