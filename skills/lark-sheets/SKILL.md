---
name: lark-sheets
version: 2.0.0-draft
description: "飞书电子表格：分析、编辑与可视化飞书在线表格。每个能力子域（read / write / chart / pivot / filter ...）有独立 reference 文档，内容与 sheet-ai-skills 对应 skill 完全一致；CLI 实现按子域提供对应 shortcut，详见各 reference。"
metadata:
  requires:
    bins: ["lark-cli"]
    siblings: ["lark-shared"]
  cliHelp: "lark-cli sheets --help"
---

# sheets

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理。**

飞书电子表格：分析、编辑与可视化飞书在线表格。每个能力子域（read / write / chart / pivot / filter ...）有独立 reference 文档，内容与 sheet-ai-skills 对应 skill 完全一致；CLI 实现按子域提供对应 shortcut，详见各 reference。

## References

每个 reference 内容与 `sheet-ai-skills` 中对应 skill 完全一致，按能力子域组织。CLI shortcut / API 路由的实现按这些子域提供，并在对应 reference 中描述。

| Reference | 描述 |
| --- | --- |
| [飞书表格核心操作：分析、编辑与可视化](references/lark-sheets-core-operations.md) | 飞书表格核心操作工作流。当用户需要对已有的飞书表格进行查看、分析、编辑或可视化时使用。适用场景：数据查询与统计、公式计算、表格美化、创建图表/透视表、筛选排序、批量修改数据、调整表格结构等。即使用户没有明确说"飞书表格"，只要操作对象是已有的在线表格，都应触发此工作流。不适用于本地 Excel 文件操作。 |
| [飞书表格样式与配色规范](references/lark-sheets-visual-standards.md) | 飞书表格样式与配色规范：表头/数据区/汇总行的颜色、字号、对齐、边框等取值标准，以及新增汇总行、追加行列继承原表风格、已有区域美化等典型场景的决策流程与样式要点。工具调用参数细节请参考对应的 lark_sheet_write_cells / lark_sheet_range_operations / lark_sheet_batch_update。条件格式（高亮、标红、数据条、色阶）请使用 lark_sheet_conditional_format。仅针对飞书表格；Excel 请参考 excel_general_visual_standards。 |
| [飞书表格公式生成规则](references/lark-sheets-formula-translation.md) | Excel 公式到飞书表格公式的迁移与生成规则。核心目标不是保留 Excel 原语法，而是按飞书表格可执行规则重写公式，并在结果上尽量对齐 Excel。当用户要求把 Excel 公式改写成飞书表格公式，或需要生成飞书公式（尤其涉及 ARRAYFORMULA、原生数组函数、INDEX/OFFSET、MAP/LAMBDA、日期差、多层范围结果与二次展开）时使用。仅针对飞书在线表格，不适用于本地 Excel 文件执行。 |
| [Lark Sheet Workbook](references/lark-sheets-workbook.md) | 管理飞书表格的工作簿结构（子表列表及元数据）。当用户提到"看看这个表格有什么"、"表格结构"、"有哪些 sheet"、"新建一个 sheet"、"删除这个工作表"、"重命名"、"复制一份"、"移动到前面"时使用。仅针对飞书表格。 |
| [Lark Sheet Sheet Structure](references/lark-sheets-sheet-structure.md) | 管理飞书表格的子表结构与布局。适用场景：查看行高、列宽、隐藏行列、合并单元格等布局信息，以及"插入一行"、"删除这列"、"隐藏行"、"冻结表头"、行列分组（大纲折叠/展开）等操作。行列大纲仅在用户明确提到"行分组"、"列分组"、"大纲"、"outline"时才触发，"按XXX分组"等数据分组场景请使用 lark_sheet_pivot_table。如需在表尾追加数据，应先通过此 skill 插入行，再通过 lark_sheet_write_cells 写入。仅针对飞书表格。 |
| [Lark Sheet Read Data](references/lark-sheets-read-data.md) | 读取飞书表格中的单元格数据。当用户需要"看看数据"、"分析数据"、"统计/汇总"时使用；也适用于需要查看公式、样式、批注等详细信息的场景。仅针对飞书表格。 |
| [Lark Sheet Search & Replace](references/lark-sheets-search-replace.md) | 在飞书表格中搜索和替换文本，支持限定范围、大小写匹配、精确匹配、正则表达式。当用户需要"查找"、"搜索"、"定位"某个值，或"替换"、"批量修改文本"、"把 A 改成 B"时使用。不要用于理解表格结构（应读取数据）、不要用于数据分析（应读取数据后计算）、不要把用户操作动作中的关键词（如"汇总金额""统计数量"）当作搜索词。仅针对飞书表格。 |
| [Lark Sheet Write Cells](references/lark-sheets-write-cells.md) | 向飞书表格的指定区域批量写入值、公式、样式、批注或单元格图片。适用场景：填写数据、设置公式、修改格式、添加批注、嵌入单元格图片（如需操作浮动图片，请使用 lark_sheet_float_image）；若只需把一块 CSV 纯值批量铺到表格上（不带公式/样式），直接使用 set_range_from_csv 更短更快。追加数据需先通过 lark_sheet_sheet_structure 插入行列。仅针对飞书表格。 |
| [Lark Sheet Range Operations](references/lark-sheets-range-operations.md) | 对飞书表格中指定区域执行结构性操作（不涉及写入单元格数据值）。适用场景：清除内容或格式（"清空"、"删除内容"、"去掉格式"）、合并/取消合并单元格、调整行高列宽（"加宽列"、"自适应列宽"）、移动/复制/填充/排序数据（"移动数据"、"复制到"、"自动填充"、"按某列排序"）。写入单元格数据请使用 lark_sheet_write_cells。仅针对飞书表格。 |
| [Lark Sheet Batch Update](references/lark-sheets-batch-update.md) | 将多个飞书表格写入操作合并为一次批量执行，按顺序依次完成。适合需要连续执行多个写入操作的场景（如先修改结构再写入数据）。仅针对飞书表格。 |
| [Lark Sheet Chart](references/lark-sheets-chart.md) | 管理飞书表格中的图表（柱形图、折线图、饼图、条形图、面积图、散点图、组合图、雷达图等）。当用户需要创建图表、修改图表样式或数据源、查看已有图表配置、删除图表时使用。也适用于用户提到"数据可视化"、"画个图"、"趋势分析"、"对比图"、"占比分析"、"做个图表"等数据可视化相关场景。仅针对飞书表格。 |
| [Lark Sheet Pivot Table](references/lark-sheets-pivot-table.md) | 管理飞书表格中的数据透视表。当用户需要创建透视表、修改透视表的行列字段/聚合方式/筛选条件、查看已有透视表配置、删除透视表时使用。也适用于用户提到"分组汇总"、"交叉分析"、"按XXX统计"、"按字段分组"、"再分下组"、"多维分析"、"数据透视"等场景。仅针对飞书表格。 |
| [Lark Sheet Conditional Format](references/lark-sheets-conditional-format.md) | 管理飞书表格中的条件格式规则（重复值高亮、单元格值比较、数据条、色阶、排名、自定义公式等）。当用户需要创建条件格式、修改已有规则的范围或样式、查看当前条件格式配置、删除规则时使用。也适用于用户提到"高亮"、"标红"、"颜色标记"、"数据条"、"色阶"、"条件样式"等场景。仅针对飞书表格。 |
| [Lark Sheet Filter](references/lark-sheets-filter.md) | 管理飞书表格中的筛选器（filter）。当用户需要筛选数据（按文本/数值/颜色/日期条件过滤行）、查看已有筛选配置、修改或删除筛选器时使用。也适用于"只看"、"筛选出"、"仅保留符合条件的"等场景。仅针对飞书表格。 |
| [Lark Sheet Filter View](references/lark-sheets-filter-view.md) | 管理飞书表格中的筛选视图（filter view）。当用户需要"建一个 XX 视图"、"保存这个筛选状态"、"切换不同筛选"、维护一个 sheet 上多份独立筛选配置时使用。视图与筛选器（filter）相互独立，可在同一 sheet 共存；视图的隐藏行仅在用户进入该视图时本地生效，不影响其他协作者。仅针对飞书表格。 |
| [Lark Sheet Sparkline](references/lark-sheets-sparkline.md) | 管理飞书表格中的迷你图（折线迷你图、柱形迷你图、胜负迷你图）。当用户需要在单元格内嵌入小型图表来展示数据趋势时使用。也适用于"趋势线"、"单元格内图表"、"迷你图"等场景。注意：不等同于被禁用的 SPARKLINE() 公式函数。仅针对飞书表格。 |
| [Lark Sheet Float Image](references/lark-sheets-float-image.md) | 管理飞书表格中的浮动图片。当用户需要在表格中插入浮动图片、调整图片位置和大小、查看已有浮动图片、删除图片时使用。也适用于"插入图片"、"添加 logo"、"放一张图"等场景。注意：如果用户需要将图片嵌入到某个单元格内部（单元格图片），请阅读 lark_sheet_write_cells Skill。仅针对飞书表格。 |

## 公共 flag 速查

各 reference 的每个 shortcut 标题下用一行徽章标注该 shortcut 支持的公共 / 系统 flag，例如：

- `_公共四件套 · 系统：--dry-run_` — URL/token + sheet 定位全 4 个公共 flag，加 `--dry-run`
- `_公共：URL/token（无 sheet 定位） · 系统：--yes、--dry-run_` — 只接 URL/token，常见于 `+batch-update` 等不强制 sheet 定位的 shortcut

徽章里只列名字。type / 必填 / 描述都在本段统一声明：

### 公共 flag（定位资源）

**公共四件套** = `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`。前两者 XOR 互斥（spreadsheet 定位），后两者 XOR 互斥（sheet 定位）。

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--url` | string | XOR | spreadsheet URL；与 `--spreadsheet-token` 二选一 |
| `--spreadsheet-token` | string | XOR | spreadsheet token；与 `--url` 二选一 |
| `--sheet-id` | string | XOR | 工作表 reference_id；与 `--sheet-name` 二选一 |
| `--sheet-name` | string | XOR | 工作表名称；与 `--sheet-id` 二选一 |

### 系统 flag

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--dry-run` | bool | 否 | 零副作用：仅打印请求路径与参数模板，不发起调用；多步操作会输出每个子操作的请求模板 |
| `--yes` | bool | 是（仅 `high-risk-write`） | 二次确认；不带时退出码 10。详见 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) 高风险审批协议 |
| `--print-schema` | bool | 否 | 本地打印复合 JSON flag 的 JSON Schema 并退出，不发起任何调用、不需要其它 required flag。与 `--flag-name <name>` 搭配指定要查哪个 flag；省略 `--flag-name` 时列出该 shortcut 所有可查询的 flag。仅在 shortcut 含复合 JSON flag 时有效。 |
| `--flag-name` | string | 否 | 配合 `--print-schema` 使用，指定要打印 JSON Schema 的 flag 名（不带 `--` 前缀，如 `cells` / `properties` / `operations`）。 |

**Agent 使用提示**：写复合 JSON flag（`--cells` / `--properties` / `--operations` / `--border-styles` / `--sort-keys` 等）时，如果对结构不确定，先跑 `lark-cli sheets <shortcut> --print-schema --flag-name <name>` 把完整 JSON Schema 读出来再构造 payload，比靠 reference 的速查表更精确，也避免因为字段拼写或缺失被服务端拒绝。reference 的 `## Schemas` 段只给一层结构，深层只能靠 `--print-schema` 或 `## Examples` 的真实示例。
