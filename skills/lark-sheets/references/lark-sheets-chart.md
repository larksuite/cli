# Lark Sheet Chart

## 真对象硬约束

当用户要求"画个图 / 数据可视化 / 趋势图 / 对比图 / 占比图"时，**必须**通过图表创建命令创建真实的图表对象。**禁止**用本地脚本调 matplotlib / seaborn 生成图片再插入到表格代替——静态图片无法随源数据更新，且失去交互能力。判断标准：最终对象必须能被 `+chart-list` 返回；基础单图可先用创建调用返回的完整 `snapshot` 验证，批量创建必须按受影响的 sheet 回读列表。

## 使用场景

读写图表对象。基础创建和常用更新优先用语义 shortcut，只在高级配置时使用原始 snapshot：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 查看已有图表 | `+chart-list` | 获取图表的类型、数据源和样式配置 |
| 按类型和范围创建基础图 | `+chart-create-basic` | 支持 column/bar/line/area/pie/scatter/combo/radar/bubble/waterfall/pareto、行/列方向与整图配色；无需构造 snapshot |
| 更新标题、轴、图例、标签、堆叠、平滑或整图配色 | `+chart-config-update` | CLI 读取当前快照并只回写配置 patch |
| 修正已有图表的数据范围或方向 | `+chart-data-update` | CLI 读取当前快照并只回写 data patch，保留其它配置 |
| 批量创建多个独立图表 | `+batch-chart-create` | 保留成功图表，并逐项返回失败原因；只重试失败项 |
| 批量更新多个独立图表 | `+batch-chart-update` | 逐图读取当前快照并生成 partial properties |
| 高级创建/更新、删除图表 | `+chart-{create\|update\|delete}` | 按系列/数据点精细设置等高级需求才使用原始 properties；更新只提交必要的局部 properties |

## 统一决策顺序

明确目标后，始终按以下顺序选入口，不从原始 snapshot 起步：

1. 普通单图创建 → `+chart-create-basic`；
2. 多张独立图创建 → `+batch-chart-create`；
3. 已有图的数据源 / 方向 / 系列变化 → `+chart-data-update`；
4. 已有图的标题 / 轴 / 图例 / 标签 / 堆叠 / 平滑 / 整图配色变化 → `+chart-config-update`；
5. 只有上述语义 shortcut 无法表达的单系列、单数据点或高级字段，才使用 `+chart-create` / `+chart-update` 的原始 `properties`。

进入高级入口前先写明“哪个用户要求无法由哪个语义参数表达”。答不出来就退回语义 shortcut。不要因为语义调用失败一次就改走原始 snapshot；先根据明确错误修正参数。

普通创建、数据源修正和常用配置更新不要构造原始 snapshot。

典型工作流：先确认表头和精确数据范围，用 `+chart-create-basic` 一次创建并尽量在同次调用中带上已知标题/轴/标签内容要求；标签位置只有用户明确指定时才传。创建后用返回的完整 `snapshot` 检查范围、方向与系列，再按需用 `+chart-list` 验证。已有图表的数据范围或方向错误时用 `+chart-data-update`，常用配置修正用 `+chart-config-update`。只有用户要求单个系列、数据点或高级引擎字段时，才读取现有 snapshot 并调 `+chart-update --properties`。不要为了常用配置先输出整份 schema，也不要删除重建已经创建成功的图表。

**多图表工作流**：先完成所有辅助数据和表头，列出每张目标图的类型、精确数据范围、标题和落点；确认清单后，用一次 `+batch-chart-create` 批量创建。它的每个 operation 直接填写 `+chart-create-basic` flags，CLI 内部固定按 `+chart-create-basic` 执行，不要再套 `shortcut` / `input`。图表之间独立时允许部分成功：按返回的逐项结果定位失败图表，只重试失败项。批量 create 的逐项结果不返回完整 snapshot；批次后每个受影响的 sheet 各调用一次 `+chart-list`。已经成功创建的图表有数据源或配置差异时，用 `+batch-chart-update` 批量执行对应的语义更新，不要删除重建。

**图表错误处理工作流（必须按顺序）**：
1. **基础单图走快路径**：sheet、范围、类型和落点都明确时，直接调用 `+chart-create-basic`，并检查返回的完整 `snapshot`；不要为了预览而固定多做一次 `--dry-run`。
2. **以下情况创建前必须 `--dry-run`**：批量创建、多范围或跨子表数据源、包含“每个 / 分别 / 逐一”等数量词、落点不确定，或确实需要原始高级配置。检查数量、sheet、范围、类型和落点；输出中的 `tool_name` / `operation` / `basic_chart` / `properties` 是 CLI 翻译后的内部 MCP body，**只能读，不能复制回 operations**。
3. 批量执行后同时检查 `succeeded`、`failed` 和逐项 `results[index]`；命令退出成功或顶层 `ok=true` 不代表每张图都成功。单图则检查返回的 `snapshot`。
4. 有失败时保留成功图表，按原始 `index` 重新生成只包含失败项的新 operations。禁止复用原始整批 payload，否则会重复创建已经成功的图表。
5. 批量成功后每个受影响 sheet 只调用一次 `+chart-list`，核对总数、标题、范围、方向与系列；基础单图的返回 `snapshot` 完整且符合预期时不再重复 list，只有响应不完整、后续又更新或结果存疑时再 list。
6. 快照不符合预期时原地修复：数据源、方向、维度/系列、分离表头用 `+chart-data-update`；标题、轴、图例、标签、堆叠、平滑、配色用 `+chart-config-update`；只有高级字段才用 `+chart-update --properties` 的最小局部 patch。不要删除重建。

**失败归因与恢复**：
- 参数校验失败：只根据 stderr 指出的未知 flag、缺失字段或 operations 结构修正一次；不要把 `--dry-run` 展示的内部 body 复制回命令。
- 批量部分失败：保留成功项，只重试 `failed` 对应的原始 index；重试前断言新 operations 数量等于失败数。
- 执行成功但结果不符：以返回 snapshot / `+chart-list` 为准，在原图上走语义更新；不要因标题、范围或配色不对就删除重建。
- 返回空输出或无法确认：检查退出码和 stderr，并做一次对象回读；仍无法确认时如实报告，禁止声称已完成。
- 同一种修正再次失败：停止改猜 schema、MCP body 或完整 snapshot。若语义 shortcut 能表达就回到语义入口；否则保留原对象并报告明确错误。

**图片图表 → 真图表迁移（“把截图 / 贴图换成真图表”类任务）**：
1. 先用 `+float-image-list` 读取待替换浮动图片的 ID、位置、尺寸和数量，并确认每张图片与目标真图表的对应关系；不得把 logo、说明图或无法确认对应关系的图片当成待替换图表。
2. 用户要求“配色 / 样式与原图一致”时，必须先使用可用的图像理解能力视觉检查原图，确认图表类型、标题、系列配色、图例、标签、堆叠方式、位置和尺寸；`+float-image-list` 只用于获取对象信息，不能代替视觉检查。对无法确认的样式不得凭空猜测。
3. 优先用 `+chart-create-basic` / `+batch-chart-create` 的语义参数复刻已确认的类型、标题、配色、图例、标签和堆叠方式，并尽量按原图位置与尺寸落图。普通整图配色使用 `--colors` / `--color-palette`；只有原图明确包含语义 shortcut 无法表达的单系列或单数据点样式时，才使用原始 `properties`。
4. 建好真图表后，必须先用创建返回的完整 `snapshot` 或 `+chart-list` 确认图表数量、标题、数据源、系列和位置正确，再按 [Lark Sheet Float Image](./lark-sheets-float-image.md) 的高风险删除流程用 `+float-image-delete` 删除与其一一对应的原浮动图片。
5. 删除后再调用一次 `+float-image-list`，确认被替换图片已消失，其它图片未受影响。

**数量词必须展开**：用户说“每个 / 每天 / 分别 / 逐一 / 各一张图”时，先从数据中数出实体数 `N`，把这 `N` 张图逐项写进清单，再加上其它汇总图得到目标总数 `M`；一个包含全部实体的多系列图不能替代这 `N` 张独立图。批次前断言 operations 中恰有 `M` 个图表创建，批次后断言图表总数、逐图标题与实体集合一致。

**范围与系列前置校验（创建前必做）**：清单中同时记录每张图的表头范围、纳入维度、明确排除维度、数据方向和预期系列数。当前每张图**最多 50 个数值系列**；按列组织时通常为“所选数值列数”，按行组织时通常为“所选数值行数”。创建时就用 `+chart-create-basic --dim1-index ... --dim2-indexes ...` 显式选择类别与不超过 50 个数值系列；如果业务要求展示超过 50 个系列，应先建立紧凑汇总表或 Top-N，而不是反复删除重建。创建前根据实际表头确认索引和边界，不凭字母猜范围；创建后范围、方向或系列数不符时，使用 `+chart-data-update` 修正，CLI 会读取当前快照、重建 `refs` / `dim1` / `dim2.series` 并只提交 data patch，不要删除后重建。

**坐标轴语义与范围**：所有带坐标轴的图表都要在清单中记录每条轴对应的字段语义、类别轴 / 连续轴类型、单位、边界、刻度间隔以及主副轴归属，不能只核对轴标题。多图对比时，先判断“范围 / 尺度一致”指绝对边界相同，还是跨度和刻度可比；用户未明确要求所有图共用相同最小值和最大值时，不要默认使用各数据子集的并集边界。按连续区间分图时，各图使用自己的区间边界并保持跨度和刻度可比；对比同一指标时保持值轴口径一致，不同单位或量级的指标不强行共用边界。

**横向类别行配方**：当日期/月份等类别横向排列在一行、目标数值在另一行时，把“类别行 + 数值行”一起放进 `--data-range` 并传 `--data-direction row`，例如 `--data-range "'Sheet1'!A1:M1,'Sheet1'!A3:M3" --data-direction row`。此时类别行属于数据映射，**不要**传给 `--header-range`。`--header-range` 仅表示与纯数据分离的“维度/系列名称”：column 方向必须是一行，row 方向必须是一列。row 方向却传入多列表头，通常说明把类别行误当成了分离表头。

**整图配色优先走语义参数**：只要求统一主题或一组系列颜色时，在创建时传 `--color-palette` 或 `--colors`，已有图表用 `+chart-config-update` 更新；二者互斥。`--colors` 接受逗号分隔且至少包含 2 个十六进制色值的字符串；批量 operation 的 `colors` 同时接受字符串或字符串数组，也必须至少包含 2 个颜色。`--colors` 是整图色板：引擎按颜色数组的顺序**循环**给每个系列上色（柱子、折线、扇区等各类系列元素都算一个上色单位），颜色数少于系列数时从头循环复用。若要**明确指定每个系列的颜色**，必须传入与系列数量相同的颜色（否则会因循环导致部分系列共用同一颜色）。只有指定某个系列或某个数据点的颜色时才使用原始 snapshot。

## 需求→图表类型映射（创建前必查）

| 用户说 | 图表类型 | 备注 |
|--------|---------|------|
| "占比"、"比例"、"各XX占多少" | 饼图（pie） | 单维度占比首选 |
| "对比"、"各XX的YY" | 柱形图（column，纵向） | 多类别数值对比；横向条形用 `bar` |
| "趋势"、"变化"、"走势" | 折线图（line） | 时间序列首选 |
| "趋势与量级"、"累计变化"、"区间规模" | 面积图（area） | 用面积强调趋势与数值量级 |
| "堆积"、"组成构成" | 堆积柱形图（column + stack） | 多系列累加 |
| "簇状堆积柱形图" | 堆积柱形图（column + stack） | 当前不支持原生簇状堆积；将簇状维度拆分到横轴类别，用堆积柱状图实现类似效果 |
| "分布"、"相关性" | 散点图（scatter） | 两变量关系 |
| "气泡大小"、"三变量关系"、"分组散点" | 气泡图（bubble） | x/y 决定位置，size 决定气泡大小，group 决定分组 |
| "逐项增减"、"变动贡献"、"从期初到期末" | 瀑布图（waterfall） | 展示正负变化及总计/小计；通常选一个分类列和一个增减值列 |
| "主要原因"、"累计占比"、"80/20" | 排列图（pareto） | 降序柱形 + 累计百分比曲线；只允许一个数值系列 |

**多图表需求**：当用户同时提到多种分析（如"统计占比 + 对比数量"），必须创建多个图表，每个对应一种类型，不要只做一个。

**常见配置错误（必须注意）**：
- **图表类型选择错误**：用户说"堆积柱形图 / 百分比堆积"时，用 `+chart-create-basic --stack normal|percent` 或 `+chart-config-update --stack normal|percent`；用户说"占比 / 比例"时，优先考虑饼图或百分比堆积图。注意 `column` 是纵向柱形图、`bar` 是横向条形图，"对比 / 各 XX" 类纵向柱默认用 `column`；面积图原生支持 `snapshot.plotArea.plot.type="area"`，别因速查表没列就判"不支持"。
- **数据标签开关**：创建时用 `--data-labels`，已有图用 `+chart-config-update --data-labels`；明确关闭时传 `none`，不要为常用标签配置构造原始 `labels` 对象。高级配置中 `plotArea.plot.labels` 对象的存在性即开关；关闭标签时应省略整个 `labels` 字段，不能用全部字段置为 `false` 代替。用常量或重复值系列表示基准、目标、阈值或上下限时，默认关闭该系列标签；不支持单点标签时，不得用全系列重复标签代替，改用包含名称和值的系列名、图例或标题。
- **数据标签位置**：只有用户明确要求且已有标签时才传 `--data-label-position`；它只调整已有标签的位置，不会单独开启标签。需要同时显示标签时一并传 `--data-labels`；未明确位置时省略，让图表按类型自动选择。标签位置只控制摆放方式，不能实现仅显示末点或关键点。
- **数据源范围与系列名来源要对齐**：
  - 默认让 `--data-range` 包含真正的表头行 / 列；表头上方的合并大标题必须跳过。
  - 数据和语义表头分离时，`--data-range` 只传纯数据，`--header-range` 传对应的一行（column）或一列（row）表头。范围可以是不连续多范围，也支持来自多个子表；不要因为跨子表就退回原始 snapshot。
  - 横向类别行属于 `--data-range`，不是 `--header-range`；按行组织时传 `--data-direction row`。
- **数据源必须是数值 / 日期型**：图表只渲染数值型单元格。用 `+cells-set` 构造数据源时，给数字 / 日期单元格设 `cell_styles.number_format`，不要留成纯文本，否则该系列渲染为空。
- **数值 / 日期显示异常**：坐标轴沿用源单元格格式。日期显示成序列号、大数值显示成科学计数法时，修正源数据的 `cell_styles.number_format`，不要给图表轴构造未定义的 format 字段。
- **轴口径错误**：用户要"占比 / 比例"时，用饼图或 `--stack percent`，并核对数据源与标签确实表达百分比，不要交付仍以原始计数为纵轴的图。
- **对象语义验证**：基础单图先核对返回的完整 `snapshot`；批量创建、响应不完整、后续又更新或结果存疑时，再按受影响的 sheet 调一次 `+chart-list`。这里只核对数量、数据源、方向、系列和配置，不能代替交付前的布局检查。

> **⚠️ 硬性规则：当用户通过列标题名称（而非列索引）指定横轴/纵轴系列时，必须先读取表格首行（表头）来确定列名与列索引的对应关系，再设置普通图表的 `--dim1-index` / `--dim2-indexes` 或气泡图的角色索引。**
> 例如用户说"横轴为车型系列，纵轴为 Q1-Q4 的销量"，不能猜测列索引；先用 `+cells-get` 读取数据源范围的表头，再将确认后的 1-based 索引传给 `+chart-create-basic`。

## ⚠️ chart 数据源引用 pivot 时必须排除总计行

当 chart 要基于刚创建的 pivot 产物画图时，**禁止凭猜写 `refs`**。pivot 默认启用 `show_row_grand_total` / `show_col_grand_total`，产物最后一行/一列通常是"总计"。如果 `refs` 把总计行一并框进去：
- **柱形图**末尾会多一根天文数字柱子（=所有数据求和），把其他柱子压扁到看不见
- **饼图**会多一个"总计"扇区占 33%+，真实类别的比例完全失真

**正确流程**：
1. `+pivot-create create` 返回 `sheet_id` + `pivot_table_id`
2. 调 `+csv-get(sheet_id, 'A1:E30')` 或 `+pivot-list` 读 pivot 产物的**实际数据范围**
3. 识别并排除"总计"/"小计"行（通常最后一行；嵌套 pivot 还要排除中间层小计）
4. 用 `+chart-create-basic` 创建图表，`--data-range` 精确到数据行（如 pivot 占 A1:D9、总计在 row9 → chart 用 `A1:D8`）

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

**标题与轴文案**：优先沿用用户明确指定的文案；未指定时，只根据已读取的表头生成简洁自然语言。图表标题概括对象、指标及必要的趋势/对比关系；副标题仅补充已确认的时间范围或统计口径，无必要则省略；X 轴写类别或时间维度，Y 轴写指标名，单位明确时可附单位。禁止把单元格引用、公式、内部 ID、占位符、未解析文字、乱码或空括号写入标题，也不得臆造时间、单位和业务口径。

## 交付前验收（任何图表改动后必做）

完成本次所有图表创建或更新后，再逐图核对以下项；全部通过才算完成：

1. **数量**：图表数 = 用户明确要求的数量（"每个 / 分别 / 逐一"等数量词已逐项展开为独立图，不用一张多系列图代替）。
2. **文案与展示项**：回读图表标题、副标题和坐标轴标题，确认语义准确且无乱码、占位符或空括号；图例、数据标签按用户要求展示或隐藏（未要求时不擅自增删），辅助系列不得用全点重复标签模拟单点或末点。带坐标轴的图表还要回读每条轴的字段语义、类型、单位、最小值 / 最大值、刻度以及主副轴归属；多图对比时再核对边界、跨度和口径是否符合用户的可比性要求。
3. **位置与布局**：图表创建、配置更新、数据更新或位置调整后，每个受影响子表运行一次 `python scripts/lark_chart_layout_check.py "<表格 URL 或 spreadsheet token>" --worksheet-id "<reference_id>"`，无需先用 `ls` 探测脚本。`data.passed=true` 且退出码为 `0` 才可交付；退出码 `2` 且 `data.passed=false` 表示检查成功发现问题，按返回位置用 `+chart-update --properties` 最小 patch 调整后重跑。退出码 `1`、网络超时或无有效 JSON 时只重试一次；仍失败则明确报告布局未完成验收，禁止用人工估算代替。

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+chart-list` | read | 对象 |
| `+chart-create-basic` | write | 对象 |
| `+chart-config-update` | write | 对象 |
| `+chart-data-update` | write | 对象 |
| `+chart-create` | write | 对象 |
| `+chart-update` | write | 对象 |
| `+chart-delete` | high-risk-write | 对象 |

## Flags

### `+chart-list`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--chart-id` | string | optional | 指定单个图表 reference_id 过滤 |

### `+chart-create-basic`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--chart-type` | string | required | 图表类型（可选值：`column` / `bar` / `line` / `area` / `pie` / `scatter` / `combo` / `radar` / `bubble` / `waterfall` / `pareto`） |
| `--data-range` | string | required | 数据范围；未传 --header-range 时须包含表头，传入时只传纯数据；支持逗号分隔及跨子表多范围 |
| `--header-range` | string | optional | 可选的分离表头范围；column 方向须为一行、row 方向须为一列，表头数须等于数据维度数 |
| `--data-direction` | string | optional | 数据系列方向；column 表示首列为类别，row 表示首行为类别（可选值：`column` / `row`）（默认 `column`） |
| `--x-axis-numbers-as` | string | optional | 横轴数字的解释方式；text 将数字视为等间距文本类别，values 按连续数值及真实间距绘制（可选值：`text` / `values`）（默认 `text`） |
| `--x-axis-min` | float64 | optional | 连续数值 X 轴的显示范围下界；需同时使用 --x-axis-numbers-as values |
| `--x-axis-max` | float64 | optional | 连续数值 X 轴的显示范围上界；需同时使用 --x-axis-numbers-as values |
| `--y-axis-min` | float64 | optional | 左 Y 轴的显示范围下界；必须小于 --y-axis-max |
| `--y-axis-max` | float64 | optional | 左 Y 轴的显示范围上界；必须大于 --y-axis-min |
| `--dim1-index` | int | optional | 类别/X 轴维度在数据范围中的 1-based 索引；默认 1 |
| `--dim2-indexes` | string | optional | 值/Y 轴系列的 1-based 索引列表，逗号分隔；不能包含 dim1，最多 50 个。气泡图旧调用按 `x,y[,group][,size]` 顺序传 2–4 个，新调用优先使用角色索引；饼图和排列图只传 1 个 |
| `--series-types` | string | optional | 仅组合图；按 --dim2-indexes 顺序指定系列类型，逗号分隔，可选 column、line、area，数量必须与数值系列一致 |
| `--series-y-axes` | string | optional | 仅组合图；按 --dim2-indexes 顺序指定系列使用 left 或 right Y 轴，逗号分隔，数量必须与数值系列一致 |
| `--key-index` | int | optional | 仅气泡图：标识/名称维度的 1-based 索引；与 dim1/dim2 索引互斥，默认 1 |
| `--x-index` | int | optional | 仅气泡图：X 值维度的 1-based 索引；须与 --y-index 一起提供 |
| `--y-index` | int | optional | 仅气泡图：Y 值维度的 1-based 索引；须与 --x-index 一起提供 |
| `--group-index` | int | optional | 仅气泡图：可选分组维度的 1-based 索引 |
| `--size-index` | int | optional | 仅气泡图：可选气泡大小维度的 1-based 索引 |
| `--title` | string | optional | 图表标题 |
| `--subtitle` | string | optional | 图表副标题 |
| `--legend-position` | string | optional | 图例位置；hidden 隐藏图例（可选值：`top` / `bottom` / `left` / `right` / `hidden`） |
| `--x-axis-title` | string | optional | X 轴标题 |
| `--y-axis-title` | string | optional | 左 Y 轴标题 |
| `--secondary-y-axis-title` | string | optional | 右 Y 轴标题 |
| `--x-axis-label-angle` | int | optional | X 轴标签旋转角度（可选值：`-90` / `-45` / `0` / `45` / `90`） |
| `--y-axis-label-angle` | int | optional | 左 Y 轴标签旋转角度（可选值：`-90` / `-45` / `0` / `45` / `90`） |
| `--data-labels` | string | optional | 数据标签内容；value、category、percentage 可按 value_category_percentage 顺序组成任意非空组合；series 显示系列名称，none 隐藏标签（可选值：`none` / `value` / `category` / `percentage` / `value_category` / `value_percentage` / `category_percentage` / `value_category_percentage` / `series`） |
| `--data-label-position` | string | optional | 仅当用户明确指定时传入；只调整已有数据标签的位置，不会单独开启标签；省略时按图表类型自动优化数据标签位置（可选值：`auto` / `top` / `bottom` / `left` / `right` / `center` / `inside` / `outside`） |
| `--stack` | string | optional | 堆叠模式（可选值：`none` / `normal` / `percent`） |
| `--stacked` | bool | optional | 兼容别名；等价于 --stack normal（隐藏 flag：不在 `--help` 列出，但可正常传入） |
| `--smooth` | bool | optional | 是否使用平滑曲线；支持 --smooth=false 和 --smooth false |
| `--color-palette` | string | optional | 预设整图配色主题；与 --colors 互斥（可选值：`brandColorSeries@v2` / `rainbowColorSeries@v2` / `complementaryColorSeries@v2` / `converseColorSeries@v2` / `primaryColorSeries@v2` / `singleColorSeries-B-@v2` / `singleColorSeries-W-@v2` / `singleColorSeries-G-@v2` / `singleColorSeries-Y-@v2` / `singleColorSeries-O-@v2` / `singleColorSeries-R-@v2` / `singleColorSeries-D-@v2`） |
| `--colors` | string_slice | optional | 自定义整图系列颜色，逗号分隔且至少 2 个十六进制色值；与 --color-palette 互斥 |
| `--anchor-cell` | string | optional | 可选图表锚点单元格，如 F2；省略时放到数据范围右侧 |
| `--width` | int | optional | 可选图表宽度；必须与 --height 同时传 |
| `--height` | int | optional | 可选图表高度；必须与 --width 同时传 |

### `+chart-config-update`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--chart-id` | string | required | 目标图表 reference_id |
| `--title` | string | optional | 图表标题 |
| `--subtitle` | string | optional | 图表副标题 |
| `--legend-position` | string | optional | 图例位置；hidden 隐藏图例（可选值：`top` / `bottom` / `left` / `right` / `hidden`） |
| `--x-axis-title` | string | optional | X 轴标题 |
| `--y-axis-title` | string | optional | 左 Y 轴标题 |
| `--secondary-y-axis-title` | string | optional | 右 Y 轴标题 |
| `--x-axis-label-angle` | int | optional | X 轴标签旋转角度（可选值：`-90` / `-45` / `0` / `45` / `90`） |
| `--y-axis-label-angle` | int | optional | 左 Y 轴标签旋转角度（可选值：`-90` / `-45` / `0` / `45` / `90`） |
| `--x-axis-min` | float64 | optional | 连续数值 X 轴的显示范围下界；必须小于 --x-axis-max |
| `--x-axis-max` | float64 | optional | 连续数值 X 轴的显示范围上界；必须大于 --x-axis-min |
| `--y-axis-min` | float64 | optional | 左 Y 轴的显示范围下界；必须小于 --y-axis-max |
| `--y-axis-max` | float64 | optional | 左 Y 轴的显示范围上界；必须大于 --y-axis-min |
| `--data-labels` | string | optional | 数据标签内容；value、category、percentage 可按 value_category_percentage 顺序组成任意非空组合；series 显示系列名称，none 隐藏标签（可选值：`none` / `value` / `category` / `percentage` / `value_category` / `value_percentage` / `category_percentage` / `value_category_percentage` / `series`） |
| `--data-label-position` | string | optional | 仅当用户明确指定时传入；只调整已有数据标签的位置，不会单独开启标签；省略时按图表类型自动优化数据标签位置（可选值：`auto` / `top` / `bottom` / `left` / `right` / `center` / `inside` / `outside`） |
| `--last-point-label` | bool | optional | 仅折线图、面积图、雷达图及组合图中的线性系列；true 开启每个系列最后一个数据点的数值标签，false 关闭这些单点标签 |
| `--stack` | string | optional | 堆叠模式（可选值：`none` / `normal` / `percent`） |
| `--stacked` | bool | optional | 兼容别名；等价于 --stack normal（隐藏 flag：不在 `--help` 列出，但可正常传入） |
| `--smooth` | bool | optional | 是否使用平滑曲线；支持 --smooth=false 和 --smooth false |
| `--color-palette` | string | optional | 预设整图配色主题；与 --colors 互斥（可选值：`brandColorSeries@v2` / `rainbowColorSeries@v2` / `complementaryColorSeries@v2` / `converseColorSeries@v2` / `primaryColorSeries@v2` / `singleColorSeries-B-@v2` / `singleColorSeries-W-@v2` / `singleColorSeries-G-@v2` / `singleColorSeries-Y-@v2` / `singleColorSeries-O-@v2` / `singleColorSeries-R-@v2` / `singleColorSeries-D-@v2`） |
| `--colors` | string_slice | optional | 自定义整图系列颜色，逗号分隔且至少 2 个十六进制色值；与 --color-palette 互斥 |

### `+chart-data-update`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--chart-id` | string | required | 目标图表 reference_id |
| `--data-range` | string | required | 新数据范围；未传 --header-range 时须包含表头，传入或原图已使用分离表头时只传纯数据；支持逗号分隔及跨子表多范围 |
| `--header-range` | string | optional | 可选的分离表头范围；提供后自动使用 detached 表头映射，省略时保留原图已有的 detached 映射 |
| `--data-direction` | string | optional | 数据系列方向；省略时沿用现有图表方向（可选值：`column` / `row`） |
| `--dim1-index` | int | optional | 类别/X 轴维度在数据范围中的 1-based 索引；省略时使用第 1 个维度 |
| `--dim2-indexes` | string | optional | 值/Y 轴系列在数据范围中的 1-based 索引，逗号分隔；省略时使用除 dim1 外的全部维度 |
| `--key-index` | int | optional | 仅气泡图：标识/名称维度的 1-based 索引；与 dim1/dim2 索引互斥，默认 1 |
| `--x-index` | int | optional | 仅气泡图：X 值维度的 1-based 索引；须与 --y-index 一起提供 |
| `--y-index` | int | optional | 仅气泡图：Y 值维度的 1-based 索引；须与 --x-index 一起提供 |
| `--group-index` | int | optional | 仅气泡图：可选分组维度的 1-based 索引 |
| `--size-index` | int | optional | 仅气泡图：可选气泡大小维度的 1-based 索引 |

### `+chart-create`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--properties` | string + File + Stdin（复合 JSON） | required | 图表完整配置 JSON。顶层字段为 `position` / `offset` / `size` / `snapshot`（无顶层 `data`，也无再嵌一层 `properties`）；图表数据配置在 `snapshot.data` 下（含 `refs` / `headerMode` / `dim1` / `dim2`）；必须至少含 `snapshot.data.dim1.serie.index` 或 `dim2.series[].index` 之一，否则 server 拒。结构嵌套深，完整结构跑 `--print-schema --flag-name properties` |
| `--print-example` | string | optional | 打印指定图表类型的最小可用 `--properties` 模板后直接退出（`area` / `bar` / `bubble` / `column` / `combo` / `line` / `pareto` / `pie` / `radar` / `scatter` / `waterfall`）。纯本地执行，不需要 locator flag、不发网络请求；传入未知类型时列出全部可用类型 |

### `+chart-update`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--chart-id` | string | required | 目标图表 reference_id |
| `--properties` | string + File + Stdin（复合 JSON） | required | 图表配置补丁 JSON；默认只传变化字段，未传字段保持不变；普通对象递归合并，数组整体替换 |

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
- `last_point_label` (boolean?) — update 使用
- `snapshot` (oneOf?) — 图表快照配置

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR 规则同 `+csv-get`）。

### `+chart-list`

输出契约：返回按工作表分组的图表列表，每个图表含 `chart_id` / `position` / `details.snapshot` 等。

### `+chart-create-basic`

默认使用第 1 个维度作为类别/X 轴，其余维度作为数值系列；普通图表可用 1-based 的 `--dim1-index` 和逗号分隔的 `--dim2-indexes` 精确选择。组合图默认首个数值系列为左轴柱、其余为右轴折线；需要其它组合时，用 `--series-types` 和 `--series-y-axes` 按 `--dim2-indexes` 的顺序逐项指定系列类型与左右轴，两组参数的数量都必须与最终数值系列数一致。横轴数字默认按等间距文本类别处理；只有数字之间的真实间距需要影响图形位置时，才传 `--x-axis-numbers-as values` 使用连续数轴。气泡图改用 `--key-index`、`--x-index`、`--y-index` 和可选的 `--group-index` / `--size-index`，其中 x/y 必须同时提供，key 默认 1；角色索引不能与 dim1/dim2 索引混用。旧气泡图的 dim1/dim2 位置调用仍兼容。饼图和排列图只允许一个数值系列；组合图至少需要两个数值系列；所有图表最多选择 50 个数值系列。默认让 `--data-range` 包含真实表头；只有“维度/系列名称”与纯数据分离时，才让 `--data-range` 只传纯数据，并用 `--header-range` 传对应的一行（column）或一列（row）表头。类别维度与数值维度不连续时，范围参数可传逗号分隔的多范围，也支持来自多个子表；沿数据点轴对齐的跨子表范围会保留独立引用，同一子表内错行、错列或重叠时合并为最小包围矩形，跨子表范围无法对齐时会报错。单独调用成功后返回完整 `snapshot`，可直接检查创建结果并继续修改。参数名使用 `--anchor-cell` 和 `--data-labels`。兼容调用中，`--type` / `--range` 会分别按 `--chart-type` / `--data-range` 处理，`--x-axis` / `--y-axis` 会按轴标题处理；新调用仍优先使用规范参数名。

**连续数值 X 轴的可读性**：`--x-axis-numbers-as values` 会保留数字的真实间距，但未指定范围时可能自动包含 0。如果数据集中在远离 0 的窄区间，数据点会挤在图表一侧；此时应保留 `values`，创建时用 `--x-axis-min` / `--x-axis-max` 收紧范围，已有图表用 `+chart-config-update` 修正，不要改成 `text` 掩盖问题。两个边界可单独设置；同时设置时 min 必须小于 max。

```bash
# 柱形图：默认放在数据范围右侧
lark-cli sheets +chart-create-basic --url "..." --sheet-name "Sheet1" \
  --chart-type column --data-range "'Sheet1'!A1:C10" \
  --title "销售额对比" --x-axis-title "品类" --y-axis-title "销售额" \
  --legend-position bottom --data-labels value

# 双轴组合图：月度目标、实际完成为左轴柱，完成率为右轴折线
lark-cli sheets +chart-create-basic --url "..." --sheet-name "Sheet1" \
  --chart-type combo --data-range "'Sheet1'!A1:D13" \
  --dim1-index 1 --dim2-indexes 2,3,4 \
  --series-types column,column,line --series-y-axes left,left,right \
  --title "价格与效率" --y-axis-title "价格" --secondary-y-axis-title "效率" \
  --anchor-cell F2 --width 700 --height 400

# 气泡图：x、y 必填，group、size 可选
lark-cli sheets +chart-create-basic --url "..." --sheet-name "Sheet1" \
  --chart-type bubble --data-range "'Sheet1'!A1:E20" \
  --key-index 1 --x-index 2 --y-index 3 --group-index 4 --size-index 5 \
  --title "客户分布"

# 数值散点图：保留真实 X 间距，同时收紧远离 0 的显示范围
lark-cli sheets +chart-create-basic --url "..." --sheet-name "Sheet1" \
  --chart-type scatter --data-range "'Sheet1'!A1:B20" \
  --x-axis-numbers-as values --x-axis-min 237 --x-axis-max 239

# 表头与数据分离：data-range 只传纯数据，header-range 按相同维度顺序传表头
lark-cli sheets +chart-create-basic --url "..." --sheet-name "Sheet1" \
  --chart-type line \
  --data-range "'Sheet1'!A2:A10,'Sheet1'!K2:L10" \
  --header-range "'Sheet1'!A1,'Sheet1'!K1:L1"

# 横向类别行 + 一行数值：类别行也属于 data-range，不要放进 header-range
lark-cli sheets +chart-create-basic --url "..." --sheet-name "Sheet1" \
  --chart-type line \
  --data-range "'Sheet1'!A1:M1,'Sheet1'!A3:M3" \
  --data-direction row --dim1-index 1 --dim2-indexes 2
```

多张基础图一次创建。先把所有数据准备完成，再生成 `ops.json`：

```json
[
  {
    "sheet_name": "Sheet1",
    "chart_type": "column",
    "data_range": "'Sheet1'!A1:C10",
    "title": "分类对比",
    "anchor_cell": "F2"
  },
  {
    "sheet_name": "Sheet1",
    "chart_type": "line",
    "data_range": "'Sheet1'!E1:G10",
    "title": "趋势变化",
    "anchor_cell": "F18"
  }
]
```

```bash
lark-cli sheets +batch-chart-create --url "..." --operations @ops.json
lark-cli sheets +chart-list --url "..." --sheet-name "Sheet1"
```

为了兼容旧调用，CLI 仍能读取历史 `{shortcut:"+chart-create-basic",input:{...}}` 结构，但新任务直接填写上面的扁平 `+chart-create-basic` flags。

批量修正已有图表时，operations 只放配置或数据更新；CLI 会先读取每张目标图的当前快照，再把对应 partial properties 合并进一次 `batch_update`：

```json
[
  {"shortcut":"+chart-config-update","input":{"sheet_name":"Sheet1","chart_id":"chrA","title":"新标题"}},
  {"shortcut":"+chart-data-update","input":{"sheet_name":"Sheet1","chart_id":"chrB","data_range":"'Sheet1'!A1:D10"}}
]
```

```bash
lark-cli sheets +batch-chart-update --url "..." --operations @updates.json
```

### `+chart-data-update`

当创建后发现漏列、范围过宽、辅助分类列发生变化、系列选择错误或数据方向错误时，只更新数据源，保留标题、配色、图例和落点。更新必须指定 `--chart-id`；范围、方向、普通 dim1/dim2 索引及气泡图角色索引的语义与 `+chart-create-basic` 相同。`--data-direction` 省略时沿用现有图表方向。默认让新范围包含表头；原图已经使用 detached 表头且表头不变时可省略 `--header-range`，工具会保留现有映射。工具返回更新后的 `data` 和实际采用的 `normalized_data_ranges`。

```bash
# 把遗漏的最后一列纳入原折线图，保留标题、配色、图例和落点
lark-cli sheets +chart-data-update --url "..." --sheet-id "$SID" --chart-id "chrXXX" \
  --data-range "'Sheet1'!A1:M6"
```

### `+chart-config-update`

只传需要改的字段，成功后返回更新后的 `viewModel`。`--data-labels` 支持 `value`、`category`、`percentage` 的任意非空组合，组合值按 `value_category_percentage` 顺序拼接；另可用 `series` 显示系列名称、用 `none` 删除数据标签。折线图、面积图、雷达图及组合图中的线性系列可用 `--last-point-label=true` 只开启每个系列最后一个数据点的数值标签，传 `false` 关闭这些单点标签。`--legend-position hidden` 隐藏图例；`--smooth=false` 和 `--smooth false` 都可显式关闭平滑曲线。为减少参数重试，`--stacked` 自动按 `--stack normal` 处理，`percentage,value` 或 `value,percentage` 自动按 `value_percentage` 处理，`--x-axis` / `--y-axis` 自动按 `--x-axis-title` / `--y-axis-title` 处理；新调用仍优先使用规范参数。

```bash
lark-cli sheets +chart-config-update --url "..." --sheet-id "$SID" --chart-id "chrXXX" \
  --title "新标题" --x-axis-label-angle -45 --legend-position right

lark-cli sheets +chart-config-update --url "..." --sheet-id "$SID" --chart-id "chrXXX" \
  --data-labels value_percentage --stack percent

lark-cli sheets +chart-config-update --url "..." --sheet-id "$SID" --chart-id "chrXXX" \
  --last-point-label=true
```

### `+chart-create`

基础图表优先使用 `+chart-create-basic`。仅当语义 shortcut 无法表达单系列、单数据点或高级引擎字段时，才使用 `+chart-create`。高级创建需要结构完整的 snapshot；先用 `+chart-create --print-example <type>` 取得对应图表类型的最小结构，再只修改任务需要的字段。不要先打印或阅读整份大 schema。

### `+chart-update`

标题、轴、图例、标签、堆叠、平滑、配色优先使用 `+chart-config-update`，数据范围和方向使用 `+chart-data-update`。只有高级字段才使用 `+chart-update`；不要为常见修改构造 raw properties。

`+chart-update` 支持真正的局部更新：只传实际变化的字段，未传字段保持不变，不要复制并回写完整 snapshot。

- `snapshot` 内普通对象递归合并；
- `refs` / `axes` / `series` 等数组整体替换。只改数组中的一项时，先从 `+chart-list` 读取当前完整数组，修改后只回写该数组；
- `snapshot.data.isStaticData` 不能通过 update 改变；需要切换静态 / 非静态数据时删除后重建；
- 只调整尺寸时直接传 `size`，不需要传 `snapshot`；
- 执行前用 `--dry-run` 检查目标 sheet、chart_id 和最小 patch，执行后用 `+chart-list --chart-id <id>` 核对实际 snapshot。

```bash
# 只调整尺寸；无需携带 snapshot
lark-cli sheets +chart-update --url "..." --sheet-id "$SID" --chart-id "chrXXX" \
  --properties '{"size":{"width":640,"height":360}}'
```

#### 高级 `properties` 边界

- 只查询本次要改的子树，不先打印完整大 schema：
  ```bash
  lark-cli sheets +chart-update --print-schema \
    --flag-name properties.snapshot.plotArea.axes
  ```
- `--dry-run` 输出中的 `tool_name` / `operation` / `basic_chart` / `properties` 是 CLI 翻译后的内部请求，只用于检查，不能复制回 operations 或再次当作 MCP body 提交。
- `--data-range` 本身支持逗号分隔的多个范围和跨子表范围。仅因数据不连续或跨子表，不构成手写 raw data 映射的理由。
- raw data 使用 inline 表头时，`refs` 包含真正表头且不写 `nameRef`；只有 `refs` 只覆盖纯数据、真正表头位于范围外时才用 detached：显式设置 `headerMode='detached'`，并让 `dim1.serie.nameRef` 与每个 `dim2.series[].nameRef` 指向对应表头单元格。
- raw 堆叠字段位于 `snapshot.plotArea.plot.extra.stack`；普通任务仍使用 `--stack normal|percent`。`plotArea.plot.labels` 对象的存在性就是开关，关闭标签时省略整个对象；普通任务使用 `--data-labels none`。
- `axes[].label` 不接受 `format` / `number_format`。日期、百分比和数值格式应修改源单元格的 `cell_styles.number_format`。

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

- `Validate`：XOR 公共四件套；`+chart-data-update` 要求 `--chart-id` 和 `--data-range`，并校验 `--dim1-index` / `--dim2-indexes` 是正整数索引；`+chart-create` / `+chart-update` 的 `--properties` 必须能解析为合法 JSON；`+chart-delete`（high-risk-write）校验 `--yes` 或 `--dry-run` 至少一个。
- `DryRun`：`+chart-data-update` / `+chart-create` / `+chart-update` 输出"将要 POST 的 body 模板"；`+chart-delete` 输出"将要删除的 chart_id 及隶属 sheet"，零网络副作用。
- `Execute`：`+chart-create-basic` 成功后返回完整 `snapshot`，可直接验证；批量创建、响应不完整、后续更新或结果存疑时，再按受影响 sheet 调用一次 `+chart-list` 比对结果。

> `+chart-create` / `+chart-update` 是 write 级别，按需可用 `--dry-run` 预览，不要求 `--yes`。只有 `+chart-delete`（high-risk-write）必须 `--yes`。
