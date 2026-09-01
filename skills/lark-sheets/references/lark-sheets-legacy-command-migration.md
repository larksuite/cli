# Lark Sheet 旧命令迁移指南

## 适用场景

调用 `sheets` 旧命令（`+create`、`+read`、`+write`、`+create-sheet`、`+media-upload` 等 42 个）时收到
`unknown subcommand`，或手上的脚本 / skill 早于表格命令重构。本文给出全部旧命令的替代命令，以及
**不只是改名**的那些差异（flag 名、单元格 payload 写法、响应字段路径）。

这些旧命令在重构后曾以别名形式保留了一段时间，线上使用量降到 5% 以下后整体删除。它们不再存在，
也没有 deprecation 提示——直接报 `unknown subcommand`。

## 一、命令名对照表

### 工作簿

| 旧命令 | 替代 |
| --- | --- |
| `+create` | `+workbook-create` |
| `+info` | `+workbook-info` |
| `+export` | `+workbook-export` |

### 子表

| 旧命令 | 替代 |
| --- | --- |
| `+create-sheet` | `+sheet-create` |
| `+copy-sheet` | `+sheet-copy` |
| `+delete-sheet` | `+sheet-delete` |
| `+update-sheet` | 按意图拆开：`+sheet-rename` / `+sheet-move` / `+sheet-hide` / `+sheet-unhide` / `+dim-freeze` |

### 单元格数据

| 旧命令 | 替代 |
| --- | --- |
| `+read` | `+cells-get`（只要纯值 / CSV 用 `+csv-get`，整表用 `+table-get`） |
| `+write` | `+cells-set` |
| `+append` | `+table-put --sheets '{"sheets":[{…,"mode":"append"}]}'`（追加到已有数据下方；顶层必须是 `{"sheets":[…]}` 信封，裸数组会被拒绝）；若已知目标行号，`+cells-set` 写该区域即可 |
| `+find` | `+cells-search` |
| `+replace` | `+cells-replace` |

### 样式 / 合并 / 单元格图片

| 旧命令 | 替代 |
| --- | --- |
| `+set-style` | `+cells-set-style` |
| `+batch-set-style` | `+cells-batch-set-style` |
| `+merge-cells` | `+cells-merge` |
| `+unmerge-cells` | `+cells-unmerge` |
| `+write-image` | `+cells-set-image` |

### 行列

| 旧命令 | 替代 |
| --- | --- |
| `+add-dimension` | `+dim-insert` |
| `+insert-dimension` | `+dim-insert` |
| `+move-dimension` | `+dim-move` |
| `+delete-dimension` | `+dim-delete` |
| `+update-dimension` | 按意图拆开：`+rows-resize` / `+cols-resize` / `+dim-hide` / `+dim-unhide` / `+dim-group` / `+dim-ungroup` / `+dim-freeze` |

### 筛选视图

条件（condition）不再是独立对象，已折叠进视图自身的 flag。

| 旧命令 | 替代 |
| --- | --- |
| `+create-filter-view` | `+filter-view-create` |
| `+update-filter-view` | `+filter-view-update` |
| `+list-filter-views` | `+filter-view-list` |
| `+get-filter-view` | `+filter-view-list` |
| `+delete-filter-view` | `+filter-view-delete` |
| `+create-filter-view-condition` | `+filter-view-update` |
| `+update-filter-view-condition` | `+filter-view-update` |
| `+delete-filter-view-condition` | `+filter-view-update` |
| `+list-filter-view-conditions` | `+filter-view-list` |
| `+get-filter-view-condition` | `+filter-view-list` |

### 下拉列表

| 旧命令 | 替代 |
| --- | --- |
| `+set-dropdown` | `+dropdown-set` |
| `+update-dropdown` | `+dropdown-update` |
| `+get-dropdown` | `+dropdown-get` |
| `+delete-dropdown` | `+dropdown-delete` |

### 浮动图片

单独的上传步骤已折叠进创建命令：`+float-image-create` 直接收本地 `--image` 路径。

| 旧命令 | 替代 |
| --- | --- |
| `+media-upload` | `+float-image-create`（嵌入单元格内的图片用 `+cells-set-image`） |
| `+create-float-image` | `+float-image-create` |
| `+update-float-image` | `+float-image-update` |
| `+delete-float-image` | `+float-image-delete` |
| `+get-float-image` | `+float-image-list` |
| `+list-float-images` | `+float-image-list` |

## 二、只改命令名会踩的坑

### 1. 单元格 payload 词汇变了

旧命令的 `--values` 里，公式写成 `{"type":"formula","text":"=SUM(C2:C5)"}`。这类带 `type` / `text`
的写法会被 `+cells-set` **直接拒绝**：

```text
--cells[0][0].type is not a cell field — the value type is inferred from the JSON value;
control display format via cell_styles.number_format
```

新写法把字段直接放在 cell 对象上（`{"formula":"=SUM(C2:C5)"}`）：

```bash
lark-cli sheets +cells-set --range C6 --cells '[[{"formula":"=SUM(C2:C5)"}]]'
```

内容字段只能选一个：`value` / `formula` / `rich_text` / `multiple_values`；`cell_styles`、`border_styles`、
`note`、`data_validation` 可与内容字段自由叠加。纯标量（`"文本"`、`123`）也可直接放在格位上。
完整字段用 `+cells-set --print-schema --flag-name cells` 查看。

`--values` 在 `+cells-set` 上仍作为 `--cells` 的别名被接受，所以**只有携带旧对象写法的调用才会失败**。

### 2. 响应字段路径变了

| 命令 | 取值路径 |
| --- | --- |
| `+workbook-create` | spreadsheet token 在 `data.spreadsheet.spreadsheet_token` |
| `+workbook-info` | 子表列表在 `data.sheets[]`（旧 `+info` 是 `data.sheets.sheets[]`）；**不再回显** spreadsheet token |
| `+cells-get` | 值在 `data.ranges[].cells[][].value` |
| `+cells-search` | `data.total_matches`、`data.matches[].address` |
| `+sheet-create` / `+sheet-copy` | 新子表 id 在 `data.sheet_id` |
| `+sheet-rename` / `+sheet-hide` | **只返回 `data.revision`**，要确认结果需回读 `+workbook-info`（`sheet_name`、`is_hidden`） |
| `+dim-freeze` | `data.frozen_rows`、`data.frozen_columns` |

### 3. `+update-sheet` 的一次调用要拆成多次

旧命令在一次调用里同时设置标题、隐藏态和冻结行列；新命令按意图拆开，需要分别调用
`+sheet-rename`、`+sheet-hide` / `+sheet-unhide`、`+dim-freeze`。

注意 `+dim-freeze` 是**整份冻结状态覆盖**：`--rows` 与 `--cols` 一起表达完整状态，没写的那个轴会变成未冻结。

### 4. 底层接口换了

子表增删改查从 `sheets/v2/spreadsheets/{token}/sheets_batch_update` 换成了
`sheet_ai/v2/spreadsheets/{token}/tools/invoke_write`（`modify_workbook_structure`）。
只有直接断言过 HTTP 请求体的调用方需要关心这条。

## 三、找不到对应命令时

`lark-cli sheets --help` 列出全部当前命令；单个命令的 flag 与示例用 `lark-cli sheets <命令> --help`。
按任务选命令的决策表见 [SKILL.md](../SKILL.md)。
