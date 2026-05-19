# Lark Sheet Chart

## 真对象硬约束

当用户要求"画个图 / 数据可视化 / 趋势图 / 对比图 / 占比图"时，**必须**通过 `+chart-{create|update|delete}` 创建真实的图表对象。**禁止**用 本地脚本 调 matplotlib / seaborn 生成图片再插入到表格代替——静态图片无法随源数据更新，且失去交互能力。判断标准：交付后 `+chart-list` 必须能返回该对象。

## 使用场景

读写图表对象。本 Skill 包含两个工具：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 查看已有图表 | `+chart-list` | 获取图表的类型、数据源和样式配置 |
| 创建/更新/删除图表 | `+chart-{create|update|delete}` | 对图表对象执行写入操作 |

典型工作流：先读取现有图表了解配置 → 执行创建/更新/删除 → 再次读取验证结果。

## 需求→图表类型映射（创建前必查）

| 用户说 | 图表类型 | 备注 |
|--------|---------|------|
| "占比"、"比例"、"各XX占多少" | 饼图（pie） | 单维度占比首选 |
| "对比"、"各XX的YY" | 柱状图（bar） | 多类别数值对比 |
| "趋势"、"变化"、"走势" | 折线图（line） | 时间序列首选 |
| "堆积"、"组成构成" | 堆积柱状图（bar + stack） | 多系列累加 |
| "分布"、"相关性" | 散点图（scatter） | 两变量关系 |

**多图表需求**：当用户同时提到多种分析（如"统计占比 + 对比数量"），必须创建多个图表，每个对应一种类型，不要只做一个。

**常见配置错误（必须注意）**：
- **图表类型选择错误**：用户说"堆积柱状图/百分比堆积"时，应在 `properties.snapshot.plotArea.plot.extra.stack` 中配置堆叠；百分比堆叠需在该 stack 下设置 `percentage: true`。用户说"占比/比例"时，优先考虑饼图或百分比堆积图
- **数据标签缺失**：用户需要看到具体数值时，需配置 `properties.snapshot.plotArea.plot.labels`（数据标签）相关字段
- **数据源范围与系列名来源要对齐**：
  - **默认情况（inline 模式）**：`refs` 范围**应包含表头行**（首行/首列即系列名），且范围要精确覆盖目标数据，不要多选或少选。
  - **合并标题行要跳过**：如果表格在表头上方存在合并的标题行（如"员工统计表"横跨多列的大标题），`refs` 必须跳过标题行、从真正的列标题行开始。例如表头在第 3 行、数据在第 4-20 行，则 `refs` 应为 `A3:G20` 而非 `A1:G20`。包含合并标题行会导致列名识别错误、表头被当作数据参与聚合计算。
  - **数据与表头分离时必须用 detached 模式**：当 `refs` 只覆盖完整数据的一个子集（按筛选/分组只画其中一段），而真正的语义表头在该子集之外时，**必须**设置 `data.headerMode='detached'`：refs 仅传纯数据范围，维度名/系列名通过 `dim1.serie.nameRef` / `dim2.series[].nameRef` 指向真正的表头单元格。详见下文"硬性规则：数据与表头分离场景必须使用 detached 模式"。
- **axes[].label 不接受 `format` / `number_format` 字段**：想给坐标轴数值加千分位、百分号等格式化时，不要在 `axes[i].label` 里传 `format` 或 `number_format`（schema 未定义，会报 `unexpected property "format" is not defined in schema`）。数值格式化统一在源数据单元格的 `cell_styles.number_format` 里设置（写 `+cells-set` 时），图表会沿用单元格格式。
- **创建后必须验证**：图表创建后必须调用 `+chart-list` 验证配置是否正确

> **⚠️ 硬性规则：当用户通过列标题名称（而非列索引）指定横轴/纵轴系列时，必须先读取表格首行（表头）来确定列名与列索引的对应关系，再设置 `dim1`/`dim2` 的 `index`。**
> 例如用户说"横轴为车型系列，纵轴为Q1-Q4的销量"，你不能猜测列索引，必须先通过读取表格数据源范围的首行内容（使用 `lark-sheets-read-data` 的 `+cells-get` 或其他读取单元格的工具），确认"车型系列"是第几列、"Q1"~"Q4"分别是第几列，然后再将正确的列索引填入 `dim1.serie.index` 和 `dim2.series[].index`。

> **⚠️ 硬性规则：数据与表头分离场景必须使用 detached 模式。** 当 `refs` 仅覆盖数据的一个子集，而真正的语义表头行/列位于该子集之外时，**必须** `data.headerMode='detached'` 并配上 `nameRef`。不能用 inline 模式 + 把 refs 多带 1 行兜底表头来替代——那种写法已废弃。否则图表会把错误的首行/首列当系列名，或图例显示成"系列1/系列2"等默认名，或者 refs 里混入相邻分组的数据。
>
> **触发该规则的典型信号**（满足任意一条都必须走 detached）：
> - 用户要求"针对 X 类的数据画图"、"只看某个分组"、"只画筛选后的部分"，而 X 类对应的行段在数据中间或末尾，与表头不连续；
> - 用户要求"按 X 分别画图"、"按某个维度（部门/品类/地区/时间段等）拆图"——**多张图共享同一组表头**；
> - `refs` 起始行 > 表头行（如表头在第 1 行，但 `refs` 从第 11 行开始）；
> - `refs` 起始列 > 表头列（如表头在 A 列，但 `refs` 从 C 列开始）。
>
> **正确做法**：
> 1. 在 `data` 下显式设置 `"headerMode": "detached"`；
> 2. `refs` **只覆盖该子集的纯数据**，不要向上/向左多带 1 行/列，也不要把全局表头整段并进来（否则会把其它分组的数据混进图）；
> 3. **`nameRef` 必填**：给 `dim1.serie.nameRef` 写真正表头中"类别名"那一格的 A1 引用（如 `Sheet2!A1`），给每个 `dim2.series[i].nameRef` 写对应数值列的 A1 引用（如 `Sheet2!C1`、`Sheet2!D1`）。任一缺失会被校验拦下并报 `headerMode=detached requires ... nameRef`；
> 4. `refs[i].value` 必须是单元格或普通矩形范围（CELL / NORMAL），不接受整行/整列/开区间；`direction='column'` 时起始行必须 > 0，`direction='row'` 时起始列必须 > 0；
> 5. `index` 仍按 `refs` 内的列/行号填，从 1 开始。
>
> **两种场景对照（互斥，二选一）**：
>
> | 场景 | 何时命中 | 写法 |
> |---|---|---|
> | A. 表头与数据连在一起 | 单张图、refs 首行/首列就是表头（典型整段画图） | **省略 headerMode**（默认 inline），refs 含表头，**不写 nameRef** |
> | B. 表头与数据分离 | 上面 4 条信号任一命中（数据子集、按维度拆图等） | **`headerMode='detached'`**，refs 仅纯数据，**`nameRef` 必填** |
>
> **反向约束**：场景 A 下不要写 `nameRef`——首行命名已经生效，多写反而冗余。`nameRef` 仅在场景 B 下使用（且必填）。

## ⚠️ chart 数据源引用 pivot 时必须排除总计行（高频致命错误）

当 chart 要基于刚创建的 pivot 产物画图时，**禁止凭猜写 `refs`**。pivot 默认启用 `show_row_grand_total` / `show_col_grand_total`，产物最后一行/一列通常是"总计"。如果 `refs` 把总计行一并框进去：
- **柱状图**末尾会多一根天文数字柱子（=所有数据求和），把其他柱子压扁到看不见
- **饼图**会多一个"总计"扇区占 33%+，真实类别的比例完全失真

**正确流程**：
1. `+pivot-create create` 返回 `sheet_id` + `pivot_table_id`
2. 调 `+csv-get(sheet_id, 'A1:E30')` 或 `+pivot-list` 读 pivot 产物的**实际数据范围**
3. 识别并排除"总计"/"小计"行（通常最后一行；嵌套 pivot 还要排除中间层小计）
4. `+chart-create create` 时 `data.refs` 精确到数据行（如 pivot 占 A1:D9、总计在 row9 → chart 用 `A1:D8`）

详细规则见 `lark-sheets-pivot-table` skill 第 5 节"pivot → chart 组合场景"。

## 图表位置选择（创建前必做）

凭感觉挑列号/行号会被 API 拒（`position is out of sheet range`），浪费一轮调用。按以下四步走：

1. **查尺寸**：`+sheet-info` 拿 `rowCount` / `columnCount`。
2. **估跨度**：默认单元格 **105 px 宽 × 27 px 高**，`needCols = ceil(width/105)`，`needRows = ceil(height/27)`。
3. **校验**：`position.row + needRows ≤ rowCount` 且 `col_idx + needCols ≤ columnCount`（col 按 A=0、B=1、…、Z=25、AA=26… 换算）。
4. **不够就先扩表**，二选一，禁止硬塞越界位置：
   - **优先**放数据下方空区：`position = {row: data_end_row + 2, col: "A"}`；
   - 否则先调 `+dim-insert(operation="insert")`（`lark-sheets-sheet-structure` skill）扩行/列，再 create。

**示例**：21 列 sheet 放 600×400 图 → `needCols=6, needRows=15`
- ❌ `{row: 0, col: "W"}` — col=22 越界
- ✅ `{row: 42, col: "A"}` — 放数据下方
- ✅ 先 `insert position="U" count=6 side="after"`，再 `{row: 0, col: "V"}`

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+chart-list` | read | 对象 |
| `+chart-create` | write | 对象 |
| `+chart-update` | write | 对象 |
| `+chart-delete` | high-risk-write | 对象 |

## Flags

### `+chart-list`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--chart-id` | string | 否 | 指定单个图表 reference_id 过滤 |

### `+chart-create`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--properties` | string + File + Stdin（复合 JSON） | 是 | 图表完整配置 JSON（`position` / `data` / `properties` 等），结构嵌套深，统一走 JSON 注入 |

### `+chart-update`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--chart-id` | string | 是 | 目标图表 reference_id |
| `--properties` | string + File + Stdin（复合 JSON） | 是 | 完整或足够完整的图表配置 JSON（先 `+chart-list` 回读再 patch） |

### `+chart-delete`

_公共四件套 · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--chart-id` | string | 是 | 目标图表 reference_id |

## Schemas

> 复合 JSON flag（如 `--cells` / `--properties` / `--operations` / `--border-styles` / `--sort-keys`）的字段速查：只列顶层字段 + 一层嵌套结构。深层结构看 `## Examples` 段的真实示例；要拿完整 JSON Schema 跑 `lark-cli sheets <shortcut> --print-schema --flag-name <name>`。先 `--print-schema`（不带 `--flag-name`）会列出该 shortcut 所有可查询的 flag。

### `+chart-create` `--properties` / `+chart-update` `--properties`

_创建/更新的图表属性_

**顶层字段**：
- `offset` (object?) — 可选 { col_offset?: number, row_offset?: number }
- `position` (object?) — 必填 { col: string, row: number }
- `size` (object?) — 必填 { height: number, width: number }
- `snapshot` (object?) — 图表快照配置 { data?: object, legend?: oneOf, plotArea?: object, style?: object, subTitle?: object, …共 6 项 }

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR 规则同 `+csv-get`）。

### `+chart-list`

输出契约：返回按工作表分组的图表列表，每个图表含 `chart_id` / `position` / `details.snapshot` 等。

### `+chart-create`

示例：

```bash
# 内联 JSON
lark-cli sheets +chart-create --url "https://example.feishu.cn/sheets/shtXXX" \
  --sheet-name "Sheet1" --properties '{"position":{"row":42,"col":"A"},"size":{"width":600,"height":400},"snapshot":{...}}'

# 走文件（推荐配置较多时）
lark-cli sheets +chart-create --url "https://example.feishu.cn/sheets/shtXXX" \
  --sheet-name "Sheet1" --properties @chart-config.json
```

> **`--properties` JSON 关键字段**（结构见上方 `## Schemas` 段；详见语义内容章节）：
> - `position.row` / `position.col` 必须留足空间，越界会被 API 拒
> - `snapshot.data.headerMode`：默认 inline；当 refs 仅覆盖数据子集且语义表头在子集之外，必须 `detached` + `nameRef`
> - chart 引用 pivot 输出时，`snapshot.data.data_range` 必须排除总计 / 小计行

### `+chart-update`

> 更新前必须先 `+chart-list --chart-id <id>` 回读完整配置，再在其基础上修改，避免漏字段把图表回退到默认状态。

### `+chart-delete`

示例：

```bash
# dry-run 先看会删什么
lark-cli sheets +chart-delete --url "https://example.feishu.cn/sheets/shtXXX" \
  --chart-id "chrXXX" --dry-run

# 真正执行
lark-cli sheets +chart-delete --url "https://example.feishu.cn/sheets/shtXXX" \
  --chart-id "chrXXX" --yes
```

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`+chart-create` / `+chart-update` 的 `--properties` 必须能解析为合法 JSON；`+chart-delete`（high-risk-write）校验 `--yes` 或 `--dry-run` 至少一个。
- `DryRun`：`+chart-create` / `+chart-update` 输出"将要 POST 的 body 模板"；`+chart-delete` 输出"将要删除的 chart_id 及隶属 sheet"，零网络副作用。
- `Execute`：写操作执行后自动调用 `+chart-list` 回读对比，记录到 `envelope.meta.verification`，便于上层根据回读结果判定是否符合预期。

> `+chart-create` / `+chart-update` 是 write 级别，按需可用 `--dry-run` 预览，不要求 `--yes`。只有 `+chart-delete`（high-risk-write）必须 `--yes`。
