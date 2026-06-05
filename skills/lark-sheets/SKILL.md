---
name: lark-sheets
version: 2.0.0
description: "飞书电子表格：创建和操作电子表格。支持创建表格、管理工作表与行列结构（增删/合并/调整尺寸/隐藏/冻结）、读写单元格（值/公式/样式/批注/单元格图片）、查找替换、多操作原子批量更新，以及图表、透视表、条件格式、筛选器、迷你图、浮动图片等对象的创建与维护。当用户需要创建电子表格、管理工作表、批量读写或编辑数据、统计汇总与可视化、表格美化、公式计算（含 Excel 公式迁移）等任务时使用。若用户是想按名称或关键词搜索云空间（云盘/云存储）里的表格文件，请改用 lark-drive 的 drive +search 先定位资源。当用户给出 doubao.com 的 /sheets/ URL/token 时，也应直接使用本 skill，不要因为域名不是飞书而回退到 WebFetch；路由依据是 URL 路径模式和 token，而不是域名。仅针对飞书在线电子表格，不适用于本地 Excel 文件。"
metadata:
  requires:
    bins: ["lark-cli"]
    siblings: ["lark-shared"]
  cliHelp: "lark-cli sheets --help"
---

# sheets

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理。**

## 术语约定

下列词在本 skill 各文档中可能交替出现，但**指同一对象**；解析用户口语时按此映射，不要当成不同概念：

| 标准用语 | 同义 / 口语（均指同一对象） | 说明 |
| --- | --- | --- |
| 工作表（sheet） | 子表、tab、标签页 | spreadsheet 内的单张表；`sheet_id` 是其稳定标识 |
| 电子表格（spreadsheet） | 工作簿、表格 | 顶层容器；由 `--url` 或 `--spreadsheet-token` 定位 |
| reference_id | id | **表内对象**的稳定标识，即各对象主键 flag 接受的值（见下表）。⚠️ 与 `lark-sheets-float-image` 的 `--image-uri`（图片上传句柄）不是一回事，后者不属于 reference_id |

每类对象用各自的主键 flag 定位（命名不统一，按此表对照，不要凭直觉拼）：

| 对象 | 主键 flag | 对象 | 主键 flag |
| --- | --- | --- | --- |
| 工作表 sheet | `--sheet-id` | 条件格式规则 | `--rule-id` |
| 图表 chart | `--chart-id` | 筛选视图 | `--view-id` |
| 透视表 pivot | `--pivot-table-id` | 迷你图（按组） | `--group-id` |
| 浮动图片 | `--float-image-id` | | |

## 场景 → 命令速查（拿不准命令名先查这里，别按直觉拼）

把高频意图映射到**真实存在**的 shortcut / flag。agent 常从 Excel / Google Sheets / 飞书 OpenAPI 误迁移命令名或 flag，先对照本表，避免一次必然失败的试错。完整 shortcut 见各工具参考。

| 你要做的事 | ✅ 正确写法 | ❌ 不存在（会被 cobra 拒） |
| --- | --- | --- |
| 读数据（纯值 / CSV） | `+csv-get`（范围用 `--range`） | — |
| 读值 + 公式 / 样式 / 批注 | `+cells-get --include value,formula,style,comment,data_validation` | `--with-styles`、`--with-merges`、`--include-merged-cells` |
| 写纯值（整块 CSV 平铺） | `+csv-put`（定位用 `--start-cell`，单个左上角锚点格；也接受 `--range` 别名，区间自动取左上角） | — |
| 写值 / 公式 / 样式 | `+cells-set`（定位用 `--range`） | — |
| 查找单元格 | `+cells-search`（关键字用 `--find`） | `+cells-find`、`+find`、`--query` |
| 查找并替换 | `+cells-replace` | — |
| 看子表结构（合并 / 行高列宽 / 冻结 / 隐藏） | `+sheet-info` | `+sheet-get`、`+structure-get`、`+sheet-structure-get` |
| 看工作簿 / 子表清单 | `+workbook-info` | — |
| 导出 xlsx / 单表 csv | `+workbook-export` | — |
| 清除内容 / 格式 | `+cells-clear`（范围维度用 `--scope`，取值 content / formats / all） | `--type` |
| 批量清除多区域 | `+cells-batch-clear`（`--scope`） | `--target` |
| 调整列宽 / 行高 | `+cols-resize` / `+rows-resize`（行、列是两个独立命令） | `--dimension`（无此 flag） |
| 分组汇总 / 透视 | `+pivot-create`（默认不传落点 flag → 自动新建子表，零覆盖） | 用 SUMIF / 本地脚本拼一张假透视表 |

> ⚠️ **定位 flag**：`+cells-get` / `+cells-set` / `+csv-get` 用 `--range`；`+csv-put` 规范用 `--start-cell`（单个左上角锚点格），也接受 `--range` 别名（区间自动取左上角），二者择一即可。
> ⚠️ **读取附加信息**一律走 `+cells-get --include …`，**没有** `--with-styles` 这类 flag；**看合并单元格**用 `+sheet-info` 的 `merged_cells`，不要在 `+cells-get` 里找 merge flag。

## References

本 skill 的 reference 分两组：先读**通用方法与规范**（横切所有任务的工作流、铁律、样式、公式规则，不含具体 shortcut），它们规定了"怎么做对"；再按操作对象进入**工具参考**查具体 shortcut 与调用细节。编辑类任务务必先过一遍通用方法与规范，其中的铁律对所有工具参考一律生效。

### 通用方法与规范（先读，横切所有任务，不含具体 shortcut）

| Reference | 描述 |
| --- | --- |
| [飞书表格核心操作：分析、编辑与可视化](references/lark-sheets-core-operations.md) | 飞书表格核心操作工作流。当用户需要对已有的飞书表格进行查看、分析、编辑或可视化时使用。适用场景：数据查询与统计、公式计算、表格美化、创建图表/透视表、筛选排序、批量修改数据、调整表格结构等。即使用户没有明确说"飞书表格"，只要操作对象是已有的在线表格，都应触发此工作流。不适用于本地 Excel 文件操作。 |
| [飞书表格样式与配色规范](references/lark-sheets-visual-standards.md) | 飞书表格样式与配色规范：表头/数据区/汇总行的颜色、字号、对齐、边框等取值标准，以及新增汇总行、追加行列继承原表风格、已有区域美化等典型场景的决策流程与样式要点。工具调用参数细节请参考对应的 lark-sheets-write-cells / lark-sheets-range-operations / lark-sheets-batch-update。条件格式（高亮、标红、数据条、色阶）请使用 lark-sheets-conditional-format。仅针对飞书表格，不适用于本地 Excel 文件。 |
| [飞书表格公式生成规则](references/lark-sheets-formula-translation.md) | Excel 公式到飞书表格公式的迁移与生成规则。核心目标不是保留 Excel 原语法，而是按飞书表格可执行规则重写公式，并在结果上尽量对齐 Excel。当用户要求把 Excel 公式改写成飞书表格公式，或需要生成飞书公式（尤其涉及 ARRAYFORMULA、原生数组函数、INDEX/OFFSET、MAP/LAMBDA、日期差、多层范围结果与二次展开）时使用。仅针对飞书在线表格，不适用于本地 Excel 文件执行。 |

### 按对象的工具参考（含 shortcut）

| Reference | 描述 |
| --- | --- |
| [Lark Sheet Workbook](references/lark-sheets-workbook.md) | 管理飞书表格的工作簿结构（子表列表及元数据）。当用户提到"看看这个表格有什么"、"表格结构"、"有哪些 sheet"、"新建一个 sheet"、"删除这个工作表"、"重命名"、"复制一份"、"移动到前面"时使用。仅针对飞书表格。 |
| [Lark Sheet Sheet Structure](references/lark-sheets-sheet-structure.md) | 管理飞书表格的子表结构与布局。适用场景：查看行高、列宽、隐藏行列、合并单元格等布局信息，以及"插入一行"、"删除这列"、"隐藏行"、"冻结表头"、行列分组（大纲折叠/展开）等操作。行列大纲仅在用户明确提到"行分组"、"列分组"、"大纲"、"outline"时才触发，"按XXX分组"等数据分组场景请使用 lark-sheets-pivot-table。如需在表尾追加数据，应先通过此 skill 插入行，再通过 lark-sheets-write-cells 写入。仅针对飞书表格。 |
| [Lark Sheet Read Data](references/lark-sheets-read-data.md) | 读取飞书表格中的单元格数据。当用户需要"看看数据"、"分析数据"、"统计/汇总"时使用；也适用于需要查看公式、样式、批注等详细信息的场景。仅针对飞书表格。 |
| [Lark Sheet Search & Replace](references/lark-sheets-search-replace.md) | 在飞书表格中搜索和替换文本，支持限定范围、大小写匹配、精确匹配、正则表达式。当用户需要"查找"、"搜索"、"定位"某个值，或"替换"、"批量修改文本"、"把 A 改成 B"时使用。不要用于理解表格结构（应读取数据）、不要用于数据分析（应读取数据后计算）、不要把用户操作动作中的关键词（如"汇总金额""统计数量"）当作搜索词。仅针对飞书表格。 |
| [Lark Sheet Write Cells](references/lark-sheets-write-cells.md) | 向飞书表格的指定区域批量写入值、公式、样式、批注或单元格图片。适用场景：填写数据、设置公式、修改格式、添加批注、嵌入单元格图片（如需操作浮动图片，请使用 lark-sheets-float-image）；若只需把一块 CSV 纯值批量铺到表格上（不带公式/样式），直接使用 `+csv-put` 更短更快。追加数据需先通过 lark-sheets-sheet-structure 插入行列。仅针对飞书表格。 |
| [Lark Sheet Range Operations](references/lark-sheets-range-operations.md) | 对飞书表格中指定区域执行结构性操作（不涉及写入单元格数据值）。适用场景：清除内容或格式（"清空"、"删除内容"、"去掉格式"）、合并/取消合并单元格、调整行高列宽（"加宽列"、"自适应列宽"）、移动/复制/填充/排序数据（"移动数据"、"复制到"、"自动填充"、"按某列排序"）。写入单元格数据请使用 lark-sheets-write-cells。仅针对飞书表格。 |
| [Lark Sheet Batch Update](references/lark-sheets-batch-update.md) | 将多个飞书表格写入操作合并为一次批量执行，按顺序依次完成。适合需要连续执行多个写入操作的场景（如先修改结构再写入数据）。仅针对飞书表格。 |
| [Lark Sheet Chart](references/lark-sheets-chart.md) | 管理飞书表格中的图表（柱形图、折线图、饼图、条形图、面积图、散点图、组合图、雷达图等）。当用户需要创建图表、修改图表样式或数据源、查看已有图表配置、删除图表时使用。也适用于用户提到"数据可视化"、"画个图"、"趋势分析"、"对比图"、"占比分析"、"做个图表"等数据可视化相关场景。仅针对飞书表格。 |
| [Lark Sheet Pivot Table](references/lark-sheets-pivot-table.md) | 管理飞书表格中的数据透视表。当用户需要创建透视表、修改透视表的行列字段/聚合方式/筛选条件、查看已有透视表配置、删除透视表时使用。也适用于用户提到"分组汇总"、"交叉分析"、"按XXX统计"、"按字段分组"、"再分下组"、"多维分析"、"数据透视"等场景。仅针对飞书表格。 |
| [Lark Sheet Conditional Format](references/lark-sheets-conditional-format.md) | 管理飞书表格中的条件格式规则（重复值高亮、单元格值比较、数据条、色阶、排名、自定义公式等）。当用户需要创建条件格式、修改已有规则的范围或样式、查看当前条件格式配置、删除规则时使用。也适用于用户提到"高亮"、"标红"、"颜色标记"、"数据条"、"色阶"、"条件样式"等场景。仅针对飞书表格。 |
| [Lark Sheet Filter](references/lark-sheets-filter.md) | 管理飞书表格中的筛选器（filter）。当用户需要筛选数据（按文本/数值/颜色/日期条件过滤行）、查看已有筛选配置、修改或删除筛选器时使用。也适用于"只看"、"筛选出"、"仅保留符合条件的"等场景。仅针对飞书表格。 |
| [Lark Sheet Filter View](references/lark-sheets-filter-view.md) | 管理飞书表格中的筛选视图（filter view）。当用户需要"建一个 XX 视图"、"保存这个筛选状态"、"切换不同筛选"、维护一个 sheet 上多份独立筛选配置时使用。视图与筛选器（filter）相互独立，可在同一 sheet 共存；视图的隐藏行仅在用户进入该视图时本地生效，不影响其他协作者。仅针对飞书表格。 |
| [Lark Sheet Sparkline](references/lark-sheets-sparkline.md) | 管理飞书表格中的迷你图（折线迷你图、柱形迷你图、胜负迷你图）。当用户需要在单元格内嵌入小型图表来展示数据趋势时使用。也适用于"趋势线"、"单元格内图表"、"迷你图"等场景。注意：不等同于被禁用的 SPARKLINE() 公式函数。仅针对飞书表格。 |
| [Lark Sheet Float Image](references/lark-sheets-float-image.md) | 管理飞书表格中的浮动图片。当用户需要在表格中插入浮动图片、调整图片位置和大小、查看已有浮动图片、删除图片时使用。也适用于"插入图片"、"添加 logo"、"放一张图"等场景。注意：如果用户需要将图片嵌入到某个单元格内部（单元格图片），请阅读 lark-sheets-write-cells。仅针对飞书表格。 |

## 公共 flag 速查

各 reference 的每个 shortcut 标题下用一行徽章标注该 shortcut 支持的公共 / 系统 flag，例如：

- `_公共四件套 · 系统：--dry-run_` — URL/token + sheet 定位（两组各**必给一个**，详见下方「公共 flag」），加 `--dry-run`
- `_公共：URL/token（无 sheet 定位） · 系统：--yes、--dry-run_` — 只接 URL/token，常见于 `+batch-update` 等不强制 sheet 定位的 shortcut

徽章里只列名字。type / 必填 / 描述都在本段统一声明：

### 公共 flag（定位资源）

**公共四件套** = `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`，分成两组 XOR，**每组都必须给且只能给一个**（XOR = 二选一必填，不是"可选"）：

1. **spreadsheet 定位（必填）**：`--url` 与 `--spreadsheet-token` 二选一，**必须给其中之一**。两个都不给 → 校验报错 `specify at least one of --url or --spreadsheet-token`；两个都给 → 互斥冲突。
   - **`--url` 只解析 `/sheets/` 与 `/spreadsheets/` 两种链接**（从路径里抽出 token；也可以直接把裸 token 传给 `--spreadsheet-token`）。其它形态的链接不会被解析成表格 token。
   - ⚠️ **`/wiki/` 知识库链接不能直接当表格定位用**：wiki 链接背后可能是电子表格，也可能是文档 / 多维表格等其它类型，`--url` **不会**自动把 wiki token 解析成 spreadsheet token，直接传会失败。必须先把它解析成真实文档 token —— `lark-cli wiki +node-get --node-token "<wiki 链接或 token>"`，确认返回的 `obj_type` 为 `sheet` 后，取其 `obj_token` 作为 `--spreadsheet-token` 传入（解析细节见 [`../lark-wiki/SKILL.md`](../lark-wiki/SKILL.md)）。
   - **例外**：`+workbook-create` 是新建一个还不存在的表格，**不接受任何 spreadsheet / sheet 定位 flag**（只有 `--title` / `--folder-token` / `--headers` / `--values`）。
2. **sheet 定位（公共四件套 shortcut 必填）**：`--sheet-id` 与 `--sheet-name` 二选一，**必须给其中之一**。两个都不给 → 校验报错 `specify at least one of --sheet-id or --sheet-name`。
   - ⚠️ **不确定 sheet 名时禁止直接猜 `Sheet1`**：除非用户对话明确说出 sheet 名 / id，或上下文（之前的工具调用 / URL 锚点 `?sheet=xxx`）已经出现过具体值，否则**第一步先调 `+workbook-info --url "..."`**（或 `--spreadsheet-token`）拿 `sheets[].sheet_id` / `sheets[].title` 列表再选。中文环境下子表常叫"数据" / "Sheet"（无数字）/ "工作表 1" / 业务名，猜 `Sheet1` 大概率撞 `sheet not found`，比先查多耗一次失败调用 + 重试。
   - ⚠️ **`--range` 里的 `Sheet1!` 前缀不能替代 sheet 定位**：即使写了 `--range 'Sheet1!A1:B2'`，仍**必须**额外传 `--sheet-id` 或 `--sheet-name`，否则照样报上面的错。
   - ⚠️ **A1 reference 含 `!`**（`--source` / `--range` / `--ranges`）**：shell session 起手先 `set +H`** 关 bash history expansion，否则 `"Sheet1!A1"` 会被拦成 `event not found`；含特殊字符（`-` / 空格 / 非 ASCII）的 sheet 名还要内部 single-quote 包，如 `--source "'Sales-2025'!A1:D100"`。
   - **例外**：徽章标为 `_公共：URL/token（无 sheet 定位）…_` 的 shortcut（如 `+workbook-info` / `+workbook-export` / `+batch-update` / `+dropdown-update|delete` / `+cells-batch-set-style` / `+cells-batch-clear` / `+sheet-create`）**不接受也不需要** sheet 定位，只给一组 spreadsheet 定位即可。`+pivot-create` 用 `--target-sheet-id` / `--target-sheet-name`（XOR，可都不传，落点细节见 `lark-sheets-pivot-table`）。

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--url` | string | 二选一必填（与 `--spreadsheet-token`） | spreadsheet URL |
| `--spreadsheet-token` | string | 二选一必填（与 `--url`） | spreadsheet token |
| `--sheet-id` | string | 二选一必填（与 `--sheet-name`；仅公共四件套 shortcut） | 工作表 reference_id |
| `--sheet-name` | string | 二选一必填（与 `--sheet-id`；仅公共四件套 shortcut） | 工作表名称 |

**统一调用范式**（公共四件套 shortcut 的所有示例都遵循此形状，两组定位缺一不可）：

```bash
lark-cli sheets <shortcut> <workbook 定位> <sheet 定位> <其它 flag>
#   workbook 定位：--url "..."        或 --spreadsheet-token "..."           （二选一，必给）
#   sheet 定位：    --sheet-id "$SID"  或 --sheet-name "<真实表名>"            （二选一，必给；占位符不要原样填）
# 例：lark-cli sheets +csv-get --url "https://.../sheets/shtXXX" --sheet-name "<真实表名>" --range "A1:F30"
# 注意：真实表名不要直接填 "Sheet1"——大多数表的子表不叫这个；先 +workbook-info 拿 sheets[].title 再代入。
```

### 系统 flag

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--dry-run` | bool | 否 | 零副作用：仅打印请求路径与参数模板，不发起调用；多步操作会输出每个子操作的请求模板 |
| `--yes` | bool | 是（仅 `high-risk-write`） | 二次确认；不带时退出码 10。详见 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) 高风险审批协议 |
| `--print-schema` | bool | 否 | 本地打印复合 JSON flag 的 JSON Schema 并退出，不发起任何调用、不需要其它 required flag。与 `--flag-name <name>` 搭配指定要查哪个 flag；省略 `--flag-name` 时列出该 shortcut 所有可查询的 flag。**仅在 shortcut 含复合 JSON flag 时有效**——判断方法：该 shortcut 的 Flags 表里出现类型标注为「复合 JSON」的 flag（如 `--cells` / `--properties` / `--operations` / `--border-styles` / `--sort-keys` / `--options`）即支持；纯标量 flag 的 shortcut 不支持。 |
| `--flag-name` | string | 否 | 配合 `--print-schema` 使用，指定要打印 JSON Schema 的 flag 名（不带 `--` 前缀，如 `cells` / `properties` / `operations`）。 |

**Agent 使用提示**：写复合 JSON flag（`--cells` / `--properties` / `--operations` / `--border-styles` / `--sort-keys` / `--options` 等）时，如果对结构不确定，先跑 `lark-cli sheets <shortcut> --print-schema --flag-name <name>` 把完整 JSON Schema 读出来再构造 payload，比靠 reference 的速查表更精确，也避免因为字段拼写或缺失被服务端拒绝。reference 的 `## Schemas` 段只给一层结构，深层只能靠 `--print-schema` 或 `## Examples` 的真实示例。

### flag 内容类型与输出约定（术语速记）

- flag 表里 JSON 类入参标三类：**复合 JSON** = 深层嵌套对象（用 `--print-schema` 取完整结构）；**简单 JSON** = 一维 / 二维标量数组（如 `["sheet1!A1:B2",...]` / `[["alice",95]]`，结构简单无需 print-schema）；**非 JSON 文本** = 原样文本（如 CSV）。`--print-schema` 只对**复合 JSON** flag 有效（同一 shortcut 的简单 JSON flag 如 `--colors` 不在此列）。
- **envelope**：所有 shortcut 返回统一外层结构 `{ok, identity, data, ...}`。正文里 `envelope.data` 指业务数据层（如 `+csv-get` 的 `annotated_csv`）；写操作不会自动回读，如需校验请自行调用对应的 `+*-list` / `+*-get` / `+cells-get`。

## 复合 JSON / 大入参：优先 stdin

flag 帮助里标注支持 **Stdin** 的入参，当 payload 较大、含换行 / 引号等特殊字符，或已经落在某个文件里时，优先用 stdin（`-`）传入，避免命令行超长与 shell 转义问题。

推荐写法：payload 写到用户项目目录之外的临时文件（放系统临时目录，避免污染项目），再用 stdin 喂进去：

```bash
# TMPFILE 指向系统临时目录下的 payload 文件（脚本里用 tempfile.gettempdir() / os.tmpdir() 等取临时目录）
lark-cli sheets +cells-set --url "..." --sheet-name "Sheet1" --range "A1:B2" --cells - < "$TMPFILE"
```

**`@file` 接绝对路径会被拒，且被拒后不要照报错提示做。** `@file` 出于安全只接受 cwd 下的相对路径，传 cwd 之外的绝对路径会被拒。此时报错会建议"先 cd 到目标目录，或改用相对路径"——**两条都不要照做**：cd 过去、或把临时文件写进用户项目目录，都会污染工作目录。正解是改用 stdin（`--<flag> - < 文件`）。
