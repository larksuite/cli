# Lark Sheet Chart

## 真对象硬约束

当用户要求"画个图 / 数据可视化 / 趋势图 / 对比图 / 占比图"时，**必须**通过 `+chart-{create|update|delete}` 创建真实的图表对象。**禁止**用本地脚本调 matplotlib / seaborn 生成图片再插入到表格代替——静态图片无法随源数据更新，且失去交互能力。判断标准：交付后 `+chart-list` 必须能返回该对象。

## 使用场景

读写图表对象。本 reference 覆盖 4 个 shortcut：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 查看已有图表 | `+chart-list` | 获取图表的类型、数据源和样式配置 |
| 创建/更新/删除图表 | `+chart-{create|update|delete}` | 对图表对象执行写入操作 |

典型工作流：先读取现有图表了解配置 → 执行创建/更新/删除 → 再次读取验证结果。

## 需求→图表类型映射（创建前必查）

| 用户说 | 图表类型 | 备注 |
|--------|---------|------|
| "占比"、"比例"、"各XX占多少" | 饼图（pie） | 单维度占比首选 |
| "对比"、"各XX的YY" | 柱形图（column，纵向） | 多类别数值对比；横向条形用 `bar` |
| "趋势"、"变化"、"走势" | 折线图（line） | 时间序列首选 |
| "堆积"、"组成构成" | 堆积柱形图（column + stack） | 多系列累加 |
| "分布"、"相关性" | 散点图（scatter） | 两变量关系 |

**多图表需求**：当用户同时提到多种分析（如"统计占比 + 对比数量"），必须创建多个图表，每个对应一种类型，不要只做一个。

**`--properties` 结构锚点（构造前必读）**：`--properties` 顶层只有 `position` / `offset` / `size` / `snapshot` 四个字段，**没有**顶层 `data`，也没有再嵌一层 `properties`。图表数据配置全部挂在 `snapshot.data` 下——下文及示例里出现的 `refs` / `headerMode` / `dim1` / `dim2` / `nameRef` 一律指 `snapshot.data.refs` / `snapshot.data.headerMode` / `snapshot.data.dim1` / `snapshot.data.dim2`（及其下的 `serie.nameRef` / `series[].nameRef`）；样式 / 堆叠 / 数据标签等在 `snapshot.plotArea` 下。完整结构以 `lark-cli sheets +chart-create --print-schema --flag-name properties` 为准。

**常见配置错误（必须注意）**：
- **图表类型选择错误**：用户说"堆积柱形图/百分比堆积"时，应在 `properties.snapshot.plotArea.plot.extra.stack` 中配置堆叠；百分比堆叠需在该 stack 下设置 `percentage: true`。用户说"占比/比例"时，优先考虑饼图或百分比堆积图。注意区分 `column`（柱形图，纵向）与 `bar`（条形图，横向）是两个不同的 type 取值，"对比/各 XX" 类纵向柱默认用 `column`
- **数据标签缺失**：用户需要看到具体数值时，需配置 `properties.snapshot.plotArea.plot.labels`（数据标签）相关字段
- **数据源范围与系列名来源要对齐**：
  - **默认情况（inline 模式）**：`refs` 范围**应包含表头行**（首行/首列即系列名），且范围要精确覆盖目标数据，不要多选或少选。
  - **合并标题行要跳过**：如果表格在表头上方存在合并的标题行（如"员工统计表"横跨多列的大标题），`refs` 必须跳过标题行、从真正的列标题行开始。例如表头在第 3 行、数据在第 4-20 行，则 `refs` 应为 `A3:G20` 而非 `A1:G20`。包含合并标题行会导致列名识别错误、表头被当作数据参与聚合计算。
  - **数据与表头分离时必须用 detached 模式**：当 `refs` 只覆盖完整数据的一个子集（按筛选/分组只画其中一段），而真正的语义表头在该子集之外时，**必须**设置 `snapshot.data.headerMode='detached'`：refs 仅传纯数据范围，维度名/系列名通过 `snapshot.data.dim1.serie.nameRef` / `snapshot.data.dim2.series[].nameRef` 指向真正的表头单元格。详见下文"硬性规则：数据与表头分离场景必须使用 detached 模式"。
- **axes[].label 不接受 `format` / `number_format` 字段**：想给坐标轴数值加千分位、百分号等格式化时，不要在 `axes[i].label` 里传 `format` 或 `number_format`（schema 未定义，会报 `unexpected property "format" is not defined in schema`）。数值格式化统一在源数据单元格的 `cell_styles.number_format` 里设置（写 `+cells-set` 时），图表会沿用单元格格式。**日期轴同理**：横轴显示成 `45297` 这类 Excel 序列号，是因为源日期列没设日期格式——给源列设 `number_format="yyyy-mm-dd"` 后横轴才会显示成日期（反例：折线图横轴日期显示为序列号）。大数值轴显示科学计数法同理，给源列设整数 / 千分位格式（反例：透视表数值轴显示科学计数法）。
- **轴口径要对齐用户要的指标**：用户要"占比 / 比例"时，**纵轴应是百分比**——用饼图，或柱 / 条形图设 `stack.percentage: true` 让纵轴变 %，并把数据源指向占比列 / 让数据标签显示百分比；不要交付纵轴仍是原始计数的图（反例：要求看各类占比，却用普通堆积柱、纵轴是 0–350 的人数而非百分比）。
- **创建后必须验证**：图表创建后必须调用 `+chart-list` 验证配置是否正确

> **⚠️ 硬性规则：当用户通过列标题名称（而非列索引）指定横轴/纵轴系列时，必须先读取表格首行（表头）来确定列名与列索引的对应关系，再设置 `dim1`/`dim2` 的 `index`。**
> 例如用户说"横轴为车型系列，纵轴为Q1-Q4的销量"，你不能猜测列索引，必须先通过读取表格数据源范围的首行内容（使用 `lark-sheets-read-data` 的 `+cells-get` 或其他读取单元格的工具），确认"车型系列"是第几列、"Q1"~"Q4"分别是第几列，然后再将正确的列索引填入 `dim1.serie.index` 和 `dim2.series[].index`。

> **⚠️ 硬性规则：数据与表头分离场景必须使用 detached 模式。** 当 `refs` 仅覆盖数据的一个子集，而真正的语义表头行/列位于该子集之外时，**必须** `snapshot.data.headerMode='detached'` 并配上 `nameRef`。不能用 inline 模式 + 把 refs 多带 1 行兜底表头来替代——那种写法已废弃。否则图表会把错误的首行/首列当系列名，或图例显示成"系列1/系列2"等默认名，或者 refs 里混入相邻分组的数据。
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
> 3. **`nameRef` 必填**：给 `dim1.serie.nameRef` 写真正表头中"类别名"那一格的 A1 引用（如 `'Sheet2'!A1`，sheet 名按 A1 标准单引号包裹），给每个 `dim2.series[i].nameRef` 写对应数值列的 A1 引用（如 `'Sheet2'!C1`、`'Sheet2'!D1`）。任一缺失会被校验拦下并报 `headerMode=detached requires ... nameRef`；
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

## ⚠️ chart 数据源引用 pivot 时必须排除总计行

当 chart 要基于刚创建的 pivot 产物画图时，**禁止凭猜写 `refs`**。pivot 默认启用 `show_row_grand_total` / `show_col_grand_total`，产物最后一行/一列通常是"总计"。如果 `refs` 把总计行一并框进去：
- **柱形图**末尾会多一根天文数字柱子（=所有数据求和），把其他柱子压扁到看不见
- **饼图**会多一个"总计"扇区占 33%+，真实类别的比例完全失真

**正确流程**：
1. `+pivot-create create` 返回 `sheet_id` + `pivot_table_id`
2. 调 `+csv-get(sheet_id, 'A1:E30')` 或 `+pivot-list` 读 pivot 产物的**实际数据范围**
3. 识别并排除"总计"/"小计"行（通常最后一行；嵌套 pivot 还要排除中间层小计）
4. `+chart-create create` 时 `snapshot.data.refs` 精确到数据行（如 pivot 占 A1:D9、总计在 row9 → chart 用 `A1:D8`）

## 图表位置选择（创建前必做）

凭感觉挑列号/行号会被 API 拒（`position is out of sheet range`）。按以下四步走：

1. **查尺寸**：`+workbook-info` 拿该 sheet 的 `row_count` / `column_count`（下文记为 rowCount / columnCount；`+sheet-info` 只返回布局，不含行列总数）。
2. **估跨度**：默认单元格 **105 px 宽 × 27 px 高**，`needCols = ceil(width/105)`，`needRows = ceil(height/27)`。
3. **校验**：`position.row + needRows ≤ rowCount` 且 `col_idx + needCols ≤ columnCount`（`position.row` 为 **0-based**：首行 = `row:0`，与 A1 区间 / `+dim-insert --position` 的 1-based 行号不同；col 按 A=0、B=1、…、Z=25、AA=26… 换算）。
4. **不够就先扩表**，二选一，禁止硬塞越界位置：
   - **优先**放数据下方空区：`position = {row: data_end_row + 2, col: "A"}`；
   - 否则先调 `+dim-insert`（`lark-sheets-sheet-structure`）扩行/列，再 create。

⚠️ **图表落点禁止压在已有数据矩形内**——必须落在数据区**右侧或下方的空白**，否则图表浮层会遮挡原始数据被判失败（反例：折线图落在数据区中间，遮挡了下方原始数据）。

**示例**：21 列 sheet 放 600×400 图 → `needCols=6, needRows=15`
- ❌ `{row: 0, col: "W"}` — col=22 越界
- ✅ `{row: 42, col: "A"}` — 放数据下方
- ✅ 先 `+dim-insert --position V --count 6`（在 V 列前插 6 列，即 U 列之后），再放图到 `{row: 0, col: "V"}`

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
| `--chart-id` | string | optional | 指定单个图表 reference_id 过滤 |

### `+chart-create`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--properties` | string + File + Stdin（复合 JSON） | required | 图表完整配置 JSON。顶层字段为 `position` / `offset` / `size` / `snapshot`（无顶层 `data`，也无再嵌一层 `properties`）；图表数据配置在 `snapshot.data` 下（含 `refs` / `headerMode` / `dim1` / `dim2`）。结构嵌套深，完整结构跑 `--print-schema --flag-name properties` |

### `+chart-update`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--chart-id` | string | required | 目标图表 reference_id |
| `--properties` | string + File + Stdin（复合 JSON） | required | 完整或足够完整的图表配置 JSON（先 `+chart-list` 回读再 patch） |

### `+chart-delete`

_公共四件套 · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--chart-id` | string | required | 目标图表 reference_id |

## Schemas

> 复合 JSON flag 字段速查（只列顶层 + 一层嵌套）。深层结构看下方 `## Examples`，或用 `--print-schema` 读完整 JSON Schema（用法见 SKILL.md「公共 flag 速查」与「Agent 使用提示」）。

### `+chart-create` `--properties` / `+chart-update` `--properties`

_创建/更新的图表属性_

**顶层字段**：
- `position` (object?) — 必填 { row: number, col: string }
- `offset` (object?) — 可选 { row_offset?: number, col_offset?: number }
- `size` (object?) — 必填 { width: number, height: number }
- `snapshot` (object?) — 图表快照配置 { title?: object, subTitle?: object, style?: object, legend?: oneOf, plotArea: object, …共 6 项 }

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR 规则同 `+csv-get`）。

### `+chart-list`

输出契约：返回按工作表分组的图表列表，每个图表含 `chart_id` / `position` / `details.snapshot` 等。

### `+chart-create`

> **`snapshot.data` 必填 `dim1.serie.index` 或 `dim2.series[].index` 之一**（1-based，对应 `refs.value` 范围内的列序）。schema 允许传空 `{}` 但 server 运行时强制：缺则被拒为 `snapshot.data.dim1.serie.index and dim2.series[].index are both missing; at least one must be set`，即便侥幸通过也只会渲染空图。

> ⚠️ **含 `'Sheet'!` 前缀的 `--properties` 必须走 stdin 或 `@file`，不要用 inline 单引号**。`refs` / `nameRef` 里的 sheet 前缀带单引号（`'Sheet1'!A1`），若塞进 inline 的 `--properties '{...}'`，bash 会把内层那对单引号吃掉（sheet 名带空格还会被拆成多个词），JSON 直接被破坏。下面示例统一用 `--properties - <<'JSON' … JSON`（heredoc 定界符加引号 = 不做 shell 替换），或 `--properties @file.json`（`@` 只接 cwd 下相对路径）。

最小可用列图（inline 模式：refs 含表头行）：

```bash
lark-cli sheets +chart-create --url "https://example.feishu.cn/sheets/shtXXX" \
  --sheet-name "Sheet1" --properties - <<'JSON'
{
  "position":{"row":42,"col":"A"},
  "size":{"width":600,"height":400},
  "snapshot":{
    "data":{
      "refs":[{"value":"'Sheet1'!A1:B10"}],
      "dim1":{"serie":{"index":1}},
      "dim2":{"series":[{"index":2}]}
    },
    "plotArea":{"plot":{"type":"column"}}
  }
}
JSON

# 或落到 cwd 下相对路径文件再用 @file
lark-cli sheets +chart-create --url "..." --sheet-name "Sheet1" --properties @chart-config.json
```

**饼图专属示例**（`sectors` 必须嵌在 `plotArea.plot.series[i].sectors.sector[]`，且 `sector[].index` 1-based）：

饼图比 column / bar 更复杂：`sectors` 是 object，里面再包一个**单数** `sector` 数组——CLI 不替你 normalize，写错路径会被 server schema 直接拒。

```bash
lark-cli sheets +chart-create --url "..." --sheet-name "Sheet1" --properties - <<'JSON'
{
  "position":{"row":24,"col":"F"},
  "size":{"width":600,"height":450},
  "snapshot":{
    "title":{"text":"各部门员工人数占比"},
    "plotArea":{"plot":{
      "type":"pie",
      "series":[{
        "index":1,
        "sectors":{"sector":[{"index":1,"offsetRadius":0.05}]}
      }]
    }},
    "data":{
      "refs":[{"value":"'Sheet1'!A1:B11"}],
      "dim1":{"serie":{"index":1,"aggregate":true}},
      "dim2":{"series":[{"index":2,"aggregateType":"sum"}]}
    }
  }
}
JSON
```

**数据与表头分离（必须用 `detached` + `nameRef`）**：

场景：周度销量明细表，真实表头在第 1 行（A1=周次、C1=订单量、D1=退款量），数据按 B 列"店铺"分段；用户只要"3 号店"那一段（第 11–17 行）。

```bash
lark-cli sheets +chart-create --url "..." --sheet-name "Sheet2" --properties - <<'JSON'
{
  "position":{"row":7,"col":"F"},
  "size":{"width":600,"height":360},
  "snapshot":{
    "title":{"text":"3 号店周度订单/退款"},
    "plotArea":{"plot":{"type":"column"}},
    "data":{
      "headerMode":"detached",
      "direction":"column",
      "refs":[{"value":"'Sheet2'!A11:D17"}],
      "dim1":{"serie":{"index":1,"nameRef":"'Sheet2'!A1"}},
      "dim2":{"series":[
        {"index":3,"nameRef":"'Sheet2'!C1"},
        {"index":4,"nameRef":"'Sheet2'!D1"}
      ]}
    }
  }
}
JSON
```

约束：
- `refs` 只覆盖纯数据 `A11:D17`，**不要**把表头行 A1 并进来
- `nameRef` 在 detached 模式下**必填**，缺了被校验报 `headerMode=detached requires ... nameRef`
- `index` 按 refs 内的列序算（A=1、B=2、C=3、D=4），**不是**全表列号
- `nameRef` 必须配对应的 `index`；单写 `nameRef` 不传 `index` 直接报参数错

**多张图共享同一组表头（按维度拆图，必须用 detached）**：

场景：销售明细表头在 A1:E1（月份/区域/销售额/订单数/客单价），数据按区域分 3 段（华北 A2:E9、华东 A10:E17、华南 A18:E25），要分别画 3 张图。

❌ 常见错误：

```jsonc
// 错误 1：refs 含全局表头但跨段 —— 多个区域被混进同一张图
{"data":{"refs":[{"value":"'Sheet'!A1:E17"}], ... }}   // 华东图混进华北 8 行
// 错误 2：inline + refs 只取数据段、不写 detached/nameRef —— 图例显示成具体数据值
{"data":{"refs":[{"value":"'Sheet'!A10:E17"}],"dim1":{"serie":{"index":1}}, ... }}
```

✅ 正确模式：3 张图各自 detached、refs 干净不重叠：

```jsonc
// 图 1：华北
{"data":{
  "headerMode":"detached","direction":"column",
  "refs":[{"value":"'Sheet'!A2:E9"}],
  "dim1":{"serie":{"index":1,"nameRef":"'Sheet'!A1"}},
  "dim2":{"series":[
    {"index":3,"nameRef":"'Sheet'!C1"},
    {"index":4,"nameRef":"'Sheet'!D1"}
  ]}
}}
// 图 2：华东 —— refs 改 'Sheet'!A10:E17，其余同上
// 图 3：华南 —— refs 改 'Sheet'!A18:E25，其余同上
```

> `--properties` JSON 关键字段：
> - `position.row` / `position.col` 必须留足空间，越界会被 API 拒（按本文件"图表位置选择"四步走）
> - `snapshot.data.headerMode`：默认 inline；当 refs 仅覆盖数据子集而语义表头在子集之外，必须 `detached` + `nameRef`
> - chart 引用 pivot 输出时，`snapshot.data.refs` 必须排除总计 / 小计行

### `+chart-update`

**Update 三步法**（缺一步会丢字段）：

1. `+chart-list --chart-id <id>` 拿到完整 snapshot
2. 在拿到的 snapshot 上**局部**修改要改的字段，其余保持不变
3. 把**完整 snapshot** 整个回写到 `--properties.snapshot`

```bash
lark-cli sheets +chart-update --url "..." --sheet-id "$SID" --chart-id "chrXXX" \
  --properties '{
    "position":{"row":0,"col":"A"},
    "size":{"width":480,"height":320},
    "snapshot": <完整快照（由 +chart-list 取回后局部修改）>
  }'
```

> 关键：**不能只提交局部 snapshot**，否则未传字段会被还原为默认值。`+chart-update` 的语义是 PUT（整体覆盖），不是 PATCH。

### `+chart-delete`

示例：

```bash
# dry-run 先看会删什么（sheet 定位必填）
lark-cli sheets +chart-delete --url "https://example.feishu.cn/sheets/shtXXX" --sheet-id "$SID" \
  --chart-id "chrXXX" --dry-run

# 真正执行
lark-cli sheets +chart-delete --url "https://example.feishu.cn/sheets/shtXXX" --sheet-id "$SID" \
  --chart-id "chrXXX" --yes
```

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`+chart-create` / `+chart-update` 的 `--properties` 必须能解析为合法 JSON；`+chart-delete`（high-risk-write）校验 `--yes` 或 `--dry-run` 至少一个。
- `DryRun`：`+chart-create` / `+chart-update` 输出"将要 POST 的 body 模板"；`+chart-delete` 输出"将要删除的 chart_id 及隶属 sheet"，零网络副作用。
- `Execute`：写操作执行后不自动回读；如需确认，自行调用 `+chart-list` 比对结果。

> `+chart-create` / `+chart-update` 是 write 级别，按需可用 `--dry-run` 预览，不要求 `--yes`。只有 `+chart-delete`（high-risk-write）必须 `--yes`。
