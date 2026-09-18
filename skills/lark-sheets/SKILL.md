---
name: lark-sheets
version: 3.5.2
description: "飞书电子表格：创建和操作电子表格。支持工作表与行列结构（增删/合并/尺寸/隐藏/冻结/分组）、单元格读写（值/公式/样式/批注/单元格图片）、区域复制移动排序填充、查找替换、批量更新，图表、透视表、条件格式、筛选器与筛选视图、下拉列表、迷你图、浮动图片等对象的创建与维护，以及公式校验、历史版本回滚、本地 Excel/CSV 与飞书表格的导入导出。当用户需要创建或编辑表格、统计汇总与可视化、表格美化、公式计算（含 Excel 公式迁移）、金融/财务建模（DCF、三张表、预算、Sensitivity 等）时使用。多维表格（Base/bitable）请改用 lark-base；若用户是想按名称或关键词搜索云空间（云盘/云存储）里的表格文件，请改用 lark-drive 的 drive +search 先定位资源。当用户给出 doubao.com 的 /sheets/ URL/token 时，也应直接使用本 skill，不要因为域名不是飞书而回退到 WebFetch；路由依据是 URL 路径模式和 token，而不是域名。"
metadata:
  requires:
    bins: ["lark-cli"]
    siblings: ["lark-shared"]
  cliHelp: "lark-cli sheets --help"
---

# sheets

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理。**

## 场景 → 命令速查

> 按当前动作选行；下一步必须 Read 该行 reference，读取完成前不得执行命令。只读命中的文档；含公式 / 样式等横切动作时再读对应规范，禁止用目录枚举代替 Read。

| 你要做的事 | ✅ 正确写法 | 动手前读（先 Read 再动手） |
| --- | --- | --- |
| 读数据 | `+csv-get`（纯值/CSV）、`+cells-get`（公式/样式/批注） | 读 `references/lark-sheets-read-data.md` |
| 写入数据 | `+csv-put`（无类型歧义纯文本）、`+table-put`（typed；量值/真日期；标签/编号/前导零/文本数字用 object，禁裸 csv-put）、`+cells-set`（公式/富写入）、`+cells-set-style`（样式）、`+cells-set-image`（单元格图片） | 读 `references/lark-sheets-write-cells.md` |
| 格式继承（新列/新行） | 物理插行 / 插列用 `+dim-insert --inherit-style before\|after`；往已有空白区域扩写用 `+range-copy --paste-type formats` 先铺样式再写值 | 读 `references/lark-sheets-range-operations.md`；插行插列再读 `references/lark-sheets-sheet-structure.md` |
| 工作簿操作 | `+workbook-create`、`+workbook-info`、`+workbook-import`、`+sheet-copy`、`+revision-get`、`+workbook-export` | 读 `references/lark-sheets-workbook.md` |
| 行列操作 | 排序用 `+range-sort` 原子移动整行；合并 / 取消合并用 `+cells-merge` / `+cells-unmerge`；清空内容才用 `+cells-clear`；尺寸用 `+cols-resize` / `+rows-resize` | 读 `references/lark-sheets-range-operations.md`；涉结构布局再读 `references/lark-sheets-sheet-structure.md` |
| 美化收尾 | `+styles-put` | 读 `references/lark-sheets-styles-put.md` |
| 子表结构 | `+sheet-info`、`+dim-insert`；删整行 / 列用 `+dim-delete`，不能用 clear 代替 | 读 `references/lark-sheets-sheet-structure.md` |
| 画图表 / 可视化 / 柱状图 / 折线图 / 饼图 / 趋势 / 占比 | 单图用 `+chart-create-basic`，多图用扁平输入的 `+batch-chart-create`；改已有图的数据源用 `+chart-data-update`、配置用 `+chart-config-update`；只有语义 shortcut 表达不了的单系列 / 单数据点 / 高级字段才用 `+chart-create` / `+chart-update`，且只提交必要的局部 properties。动手前先断言每张图的类型、横轴字段、分组字段和目标张数，画完 `+chart-list` 逐项核；图片迁移成真图表后删除并复查原浮动图片 | 读 `references/lark-sheets-chart.md`；含透视 / 分组汇总再读 `references/lark-sheets-pivot-table.md` |
| 分组汇总 / 透视 | `+pivot-create` | 读 `references/lark-sheets-pivot-table.md` |
| 筛选 / 只看符合条件的行 | `+filter-create` | 读 `references/lark-sheets-filter.md` |
| 查找 / 替换文本 | `+cells-search`、`+cells-replace` | 读 `references/lark-sheets-search-replace.md` |
| 条件格式 / 条件高亮 / 数据条 / 色阶 | 随数据变化的标色用 `+cond-format-create`；固定刷色只用于用户点名要静态着色 | 读 `references/lark-sheets-conditional-format.md` |
| 插图：自由摆放的装饰 | `+float-image-create` | 读 `references/lark-sheets-float-image.md` |
| 迷你图 / 单元格内趋势线 | `+sparkline-create` | 读 `references/lark-sheets-sparkline.md` |
| 批量清除多区域 | `+cells-batch-clear` | 读 `references/lark-sheets-batch-update.md`（high-risk） |
| 复核编辑变更 / 取版本间差异 | `+changeset-get` | 读 `references/lark-sheets-changeset.md` |
| 保存多份筛选状态 / 命名筛选视图 | `+filter-view-create`；视图与 `+filter-create` 相互独立、可在同一子表共存 | 读 `references/lark-sheets-filter-view.md` |
| 查编辑历史 / 回滚到历史版本 | `+history-list` 取版本，`+history-revert`（high-risk，异步）回滚后用 `+history-revert-status` 轮询 | 读 `references/lark-sheets-history.md` |

> ⚠️ 金额 / 百分比 / 比率 / 计数及参与运算的真日期写数字（百分比传 `0.4` + `number_format`）；日期标签、编号、前导零、身份证 / 单据号写文本。`--range` 只写 `A1:B2`，子表另传 `--sheet-id` / `--sheet-name`。

## 飞书表格编辑准则

1. **最小改动**：用户没点名要删 / 改名 / 隐藏时，已有 Sheet 一张不动；补齐只写空格，未要求调整的值 / 结构 / 格式不动。
2. **目标子表与回读断言**：先确认真实末行与目标区域；未点名子表时只从 `resource_type=sheet && is_hidden=false` 的可见网格候选里选，唯一才自动使用，多张不得按 index 猜。涉及"所有 / 每个 sheet"（跨表汇总、批量清洗、合并多张子表）时先 `+workbook-info` 列全再逐个处理，别只做前几张。写后用 `+csv-get` / `+cells-get` / `+<对象>-list` 验首、中、末及用户点名项——返回 `ok` 只表示请求成功。纯 CSV 回写前去掉 `annotated_csv` 的 `[row=N] ` 前缀，`cells-get` 的样式字段与值分开处理，公式必须回读 `formula`。**样式同样要回读**：写过边框 / 底色 / 字体色 / 数字格式 / 行高列宽 / 冻结的，收尾用 `+cells-get --include style` 或 `+sheet-info` 抽查目标区域首、中、末格确认属性真的在——写入返回 `ok` 不代表样式落上了；缺的整份重发（样式是幂等盖章，重发无副作用）。
3. **公式闭环**：可推导值写落格公式，不用静态值代替——用 Python 算好数值再写进单元格，交付的是改输入不重算的死表；Python 只用于推导和验证，落进单元格的必须是引用其他格的公式。写前确认字段语义、阈值边界（以上/至少=`>=`，超过/大于=`>`）、单位/时区和完整源范围，选首中末、空值、边界及一条可手算记录作哨兵；写后逐段 `+formula-verify --exit-on-error`，各段 `status='success'` 且哨兵值正确才算完成（AI 公式例外：异步计算，改用 `+formula-verify --ai-only` 对整个写入区间做一次异步状态检查，不用 `+cells-get` 轮询结果，`failed` 清零后即使仍有 pending 也可交付并说明）；试错 3 次仍失败可降级静态值，交付说明写明「静态值 + 失败原因 + 不随源数据更新」。
4. **完整继承样式**：新增行列时禁止只读值只写值——原表字体、对齐、底色（含奇偶行交替）、四边框都延续到新区域。**物理插入行 / 列**用 `+dim-insert --inherit-style before|after`（原生继承，比补刷可靠）；**往已有空白区域扩写**（如在数据右侧加新列）用 `+range-copy --paste-type formats` 先铺样式再写值；两者都表达不了的非规则样式，才用 `+cells-get --include style` 读源区样式随值写回。无论走哪条路径，插入后都另查行高列宽（行高不随样式继承，插行填长文本前补 `+rows-resize`）、合并与跨列标题并补齐。详见 `references/lark-sheets-write-cells.md`。
5. **原子操作**：排序用 `+range-sort`，`--range` 覆盖完整记录宽度，排序列只写进 `--sort-keys`；删除记录用 `+dim-delete`，清空内容 / 格式才用 `+cells-clear`；禁止读值后用 `+csv-put` 覆盖来模拟排序 / 删除。仅跨类型且有顺序依赖时才用 high-risk `+batch-update`。
6. **标色分流**：数据变化后应自动重算的高亮 / 标红用条件格式，已确定结果的固定标注用静态样式，装饰性美化按视觉规范。两条路径取色字段用同一判据：用户中文语境下的"标红 / 染色 / 标记"指**单元格背景色**，"文字红 / 字体红 / 把字变红"才用字体色，默认无说明时选背景色。条件格式建完先 `+cond-format-list` 验规则与范围，再 `+cond-format-result-get` 抽查哨兵格命中样式。
7. **产物可核对**：用户点名的 sheet 名与数量、表头、标题、图例、文件名、口径逐字保留；回复中每项“已完成”都能定位到产物，缺口逐项声明。
8. **替换与新增**：批量替换 / 删除后搜索确认无残留；新增列要有表头，单位 / 口径另置，不占原表头或数据格。
9. **不编造**：表外数据须有可核验来源，不用常识或名称推断伪造公司、标准值、行情或法规参数；**没有来源就留空**——凭记忆填的数值大概率与真实值对不上，比留空更糟。留空的格在交付说明里逐项列出格址与缺的来源，不要只写一句"部分数据缺失"。

> 🤖 **文本类 NLP 任务首选 AI 公式，别默认退回手工 / Python**：只要对文本列做**翻译 / 情感 / 分类打标签 / 信息提取 / 总结 / 润色**等 NLP，飞书在线表格上优先用原生 `=AI(prompt, range)` 逐列铺开（写法与普通公式一致，见 `references/lark-sheets-formula-translation.md`），一次落表随行自动计算，比逐条读 → 手工判断 → 回写 / Python 调模型再写静态值都更省事。**判定标准是「逐行独立」**：每个目标单元格只依赖同一行输入即为逐行独立，**数据量（哪怕 1 万 +）、分批、判断复杂度都不改变该判定**——大数据量下 AI 公式仍是首选，分批只改公式铺设的批次大小（行数很多时按批串行，量级参考每批几百到一千行），不得改为「用 Python 或规则脚本生成语义结果后静态写回」；Python 只能做清洗 / 行号映射 / 构造公式批次，不得读源文本生成目标语义值。只有单个结果依赖多行输入的跨行任务才走非公式路线。AI 公式异步计算，写完先对种子格 / 首格做**一次** `+cells-get --include formula` 核对文本，随后**第一校验入口必须是** `+formula-verify --ai-only --range <整个写入区间>`，禁止用 `+cells-get` 轮询计算结果；判据为 `ai_formula_failed_count == 0`（`--range` 只透传给后端、不保证收窄汇总口径，按返回的单元格定位核对本次区间，别拿总数对预期条数），满足后即使仍有 pending 也可交付，并告知用户"AI 公式仍在后台运行"。

> 流程：了解结构 →（未点名时先按 visible_grid selection 定位）→ 读数据 → 原生工具写入 → 按用户点名项回读验证 → 在线交付。整理 / 美化 / 加汇总行这类会改变表长或版式的任务，收尾把表头行冻住（原表已有冻结设置的不动）。xlsx 验收只在处理本地 xlsx、或用户点名要本地 xlsx / 下载 / 打印时跑。
## References

reference 分两组：先读**通用方法与规范**（横切所有任务的样式 / 公式规则），再按操作对象进入**工具参考**查具体 shortcut。编辑类任务务必先过通用方法与规范，连同上方「飞书表格编辑准则」对所有工具参考一律生效。

### 通用方法与规范（先读，横切所有任务，不含具体 shortcut）

| Reference | 描述 |
| --- | --- |
| [飞书表格样式与配色规范](references/lark-sheets-visual-standards.md) | 飞书表格样式与配色规范：表头/数据区/汇总行的颜色、字号、对齐、边框、数字格式等取值标准，以及从零新建表格的版式美化、新增汇总行、追加行列继承原表风格、已有区域美化等典型场景的决策流程与样式要点。工具调用参数细节请参考对应的 lark-sheets-write-cells / lark-sheets-range-operations / lark-sheets-batch-update。条件格式（高亮、标红、数据条、色阶）请使用 lark-sheets-conditional-format。 |
| [飞书表格公式生成规则](references/lark-sheets-formula-translation.md) | Excel 公式到飞书表格公式的迁移与生成规则。核心目标不是保留 Excel 原语法，而是按飞书表格可执行规则重写公式，并在结果上尽量对齐 Excel。当用户要求把 Excel 公式改写成飞书表格公式，或需要生成飞书公式（尤其涉及 ARRAYFORMULA、数组语义与逐行填充、原生数组函数、INDEX/OFFSET、MAP/LAMBDA、日期差、多层范围结果与二次展开）时使用。本文负责把公式写对；落表后必须用 `references/lark-sheets-formula-verify.md` 对本次公式范围逐段诊断。 |

### 按对象的工具参考（含 shortcut）

| Reference | 描述 |
| --- | --- |
| [Lark Sheet Formula Verify](references/lark-sheets-formula-verify.md) | 公式写入 / 批量填充 / `--copy-to-range` 扩展 / 导入含公式工作簿后的完成检查。普通公式按本次新增或修改范围逐段扫描，合并编译失败与 7 类运行错误；`partial` 继续拆分，全部 `status='success'` 后完成。AI 公式用 `--ai-only` 对整个写入区间做一次异步状态检查，pending 可说明后交付。 |
| [Lark Sheet Workbook](references/lark-sheets-workbook.md) | 管理飞书表格的工作簿结构（子表列表及元数据）。当用户提到"看看这个表格有什么"、"表格结构"、"有哪些 sheet"、"新建一个 sheet"、"删除这个工作表"、"重命名"、"复制一份"、"移动到前面"时使用。 |
| [Lark Sheet Sheet Structure](references/lark-sheets-sheet-structure.md) | 管理飞书表格的子表结构与布局：查看行高列宽、隐藏、合并、冻结与分组，并执行插入/删除/移动行列等物理结构操作。数据分组统计走 lark-sheets-pivot-table。普通表尾追加优先用 lark-sheets-write-cells 的 `+table-put --mode append` 自动定位末行；只有用户明确要求物理插入行列、继承模板结构或扩容布局时才先用本 reference。 |
| [Lark Sheet Read Data](references/lark-sheets-read-data.md) | 读取飞书表格中的单元格数据。当用户需要"看看数据"、"分析数据"、"统计/汇总"时使用；也适用于需要查看公式、样式、批注等详细信息的场景。 |
| [Lark Sheet Search & Replace](references/lark-sheets-search-replace.md) | 在飞书表格中搜索和替换文本，支持限定范围、大小写匹配、精确匹配、正则表达式。当用户需要"查找"、"搜索"、"定位"某个值，或"替换"、"批量修改文本"、"把 A 改成 B"时使用。不要用于理解表格结构（应读取数据）、不要用于数据分析（应读取数据后计算）、不要把用户操作动作中的关键词（如"汇总金额""统计数量"）当作搜索词。 |
| [Lark Sheet Write Cells](references/lark-sheets-write-cells.md) | 向飞书表格指定区域批量写入值、公式、样式、批注或单元格图片。纯文本可用 `+csv-put`；金额、百分比、日期、布尔、计数和后续参与聚合的列用 `+table-put` 并显式声明 dtypes/formats；公式或富字段用 `+cells-set`。追加数据可直接使用 `+table-put --mode append`；只有明确需要物理插行/列时才先走 lark-sheets-sheet-structure。公式落表后必须运行 lark-sheets-formula-verify。 |
| [Lark Sheet Range Operations](references/lark-sheets-range-operations.md) | 对飞书表格中指定区域执行结构性操作（不涉及写入单元格数据值）。适用场景：清除内容或格式（"清空"、"删除内容"、"去掉格式"）、合并/取消合并单元格、调整行高列宽（"加宽列"、"自适应列宽"）、移动/复制/填充/排序数据（"移动数据"、"复制到"、"自动填充"、"按某列排序"）。写入单元格数据请使用 lark-sheets-write-cells。 |
| [Lark Sheet Styles Put](references/lark-sheets-styles-put.md) | 把一份声明式视觉规格（样式/边框/合并/行高列宽/冻结）一次性应用到已有飞书表格的多个子表，整份规格一次提交。当任务是对存量表做美化收尾、批量刷样式、统一版式时使用。样式取值标准见 lark-sheets-visual-standards；建新表带样式走 lark-sheets-workbook（+workbook-create --styles）、写数据同步带样式走 lark-sheets-write-cells（+table-put --styles），三者共用同一份 --styles 词汇。仅针对飞书表格。 |
| [Lark Sheet Batch Update](references/lark-sheets-batch-update.md) | 将多个飞书表格写入操作合并为一次批量执行，按顺序依次完成。适合需要连续执行多个写入操作的场景（如先修改结构再写入数据）。 |
| [Lark Sheet Chart](references/lark-sheets-chart.md) | 管理飞书表格中的图表（柱形图、折线图、饼图、条形图、面积图、散点图、组合图、雷达图等）。当用户需要创建图表、修改图表样式或数据源、查看已有图表配置、删除图表时使用。也适用于用户提到"数据可视化"、"画个图"、"趋势分析"、"对比图"、"占比分析"、"做个图表"等数据可视化相关场景。 |
| [Lark Sheet Pivot Table](references/lark-sheets-pivot-table.md) | 管理飞书表格中的数据透视表。当用户需要创建透视表、修改透视表的行列字段/聚合方式/筛选条件、查看已有透视表配置、删除透视表时使用。也适用于用户提到"分组汇总"、"交叉分析"、"按XXX统计"、"按字段分组"、"再分下组"、"多维分析"、"数据透视"等场景。 |
| [Lark Sheet Conditional Format](references/lark-sheets-conditional-format.md) | 管理飞书表格中的条件格式规则（重复值高亮、单元格值比较、数据条、色阶、排名、自定义公式等）。当用户需要创建条件格式、修改已有规则的范围或样式、查看当前条件格式配置、删除规则时使用。也适用于用户提到"高亮"、"标红"、"颜色标记"、"数据条"、"色阶"、"条件样式"等场景。 |
| [Lark Sheet Filter](references/lark-sheets-filter.md) | 管理飞书表格中的筛选器（filter）。当用户需要筛选数据（按文本/数值/颜色/日期条件过滤行）、查看已有筛选配置、修改或删除筛选器时使用。也适用于"只看"、"筛选出"、"仅保留符合条件的"等场景。 |
| [Lark Sheet Filter View](references/lark-sheets-filter-view.md) | 管理飞书表格中的筛选视图（filter view）。当用户需要"建一个 XX 视图"、"保存这个筛选状态"、"切换不同筛选"、维护一个 sheet 上多份独立筛选配置时使用。视图与筛选器（filter）相互独立，可在同一 sheet 共存；视图的隐藏行仅在用户进入该视图时本地生效，不影响其他协作者。 |
| [Lark Sheet Sparkline](references/lark-sheets-sparkline.md) | 管理飞书表格中的迷你图（折线迷你图、柱形迷你图、胜负迷你图）。当用户需要在单元格内嵌入小型图表来展示数据趋势时使用。也适用于"趋势线"、"单元格内图表"、"迷你图"等场景。注意：不等同于被禁用的 SPARKLINE() 公式函数。 |
| [Lark Sheet Float Image](references/lark-sheets-float-image.md) | 管理飞书表格中的浮动图片。当用户需要在表格中插入浮动图片、调整图片位置和大小、查看已有浮动图片、删除图片时使用。也适用于"插入图片"、"添加 logo"、"放一张图"等场景。注意：如果用户需要将图片嵌入到某个单元格内部（单元格图片），请阅读 lark-sheets-write-cells。 |
| [Lark Sheet History](references/lark-sheets-history.md) | 查询飞书表格的历史版本并回滚到指定版本。当用户需要查看一张表的编辑历史版本列表、回滚到某个历史版本、或查询回滚的异步状态（进行中/成功/失败）时使用。回滚为异步操作，发起后通过状态查询轮询结果。仅针对飞书表格。 |
| [Lark Sheet Changeset](references/lark-sheets-changeset.md) | 读取两个版本（CS revision）之间的 changeset（原始变更操作清单），用于复核某次编辑——尤其是 AI 编辑——是否真实满足用户诉求。传入起始版本（编辑前基线），可选结束版本（省略取最新），版本差上限 20；返回里最外层带当前表格最新版本号。当用户需要"看看这次改了什么"、"核对 AI 改动"、"对比两个版本的变更"时使用。 |

## 公共 flag 速查

各 reference 的 shortcut 标题下用一行徽章标注支持的公共 / 系统 flag（如 `_公共四件套 · 系统：--dry-run_`）。type / 必填 / 描述在本段统一声明：

### 公共 flag（定位资源）

**公共四件套** = `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`，分成两组 XOR，**每组都必须给且只能给一个**（XOR = 二选一必填，不是"可选"）——`spreadsheet` 指工作簿、`sheet` 指子表；条件格式 / 图表 / 筛选视图 / 透视表 / 迷你图 / 浮动图片这类对象在四件套之外另用各自的 `--*-id` 定位：

1. **spreadsheet 定位（必填）**：`--url`（解析 `/sheets/`、`/spreadsheets/`、`/wiki/` 三种链接；wiki 链接自动定位背后的电子表格）与 `--spreadsheet-token`（裸 token）二选一。**例外**：`+workbook-create` / `+workbook-import` 产出**还不存在**的表，不接受任何定位 flag。
2. **sheet 定位（公共四件套 shortcut 必填）**：`--sheet-id` 与 `--sheet-name` 二选一。
   - ⚠️ **不确定 sheet 名时禁止猜 `Sheet1`**：除非对话或上下文已出现具体值，第一步先 `+workbook-info` 拿 `sheets[].sheet_id/title` 再选——中文表的子表常叫"数据"/"工作表 1"/业务名，猜名大概率撞 `sheet not found`。
   - ⚠️ **`--range` 里的 `Sheet1!` 前缀不能替代 sheet 定位**：仍必须传 `--sheet-id` / `--sheet-name`。
   - ⚠️ **A1 引用含 `!` 时整段用单引号包裹**（`--range 'Sheet1!A1:B2'`，挡 bash history expansion；别用 `set +H`，sh/dash 下非法）。sheet 名要在 A1 里内层再包单引号时用 `'\''` 转义。
   - **例外**：徽章标 `_公共：URL/token（无 sheet 定位）…_` 的 shortcut 不接受 sheet 定位——工作簿级（`+workbook-info` / `+sheet-list` / `+sheet-create` / `+revision-get` / `+changeset-get` / `+history-list|revert|revert-status`）、批量与整表级（`+batch-update` / `+batch-chart-create|update` / `+cells-batch-clear` / `+styles-put` / `+dropdown-update|delete`），以及子表名写在 payload 里的 `+table-put`。`+workbook-export` 只接 `--sheet-id`（无 `--sheet-name`），`+pivot-create` 用 `--target-sheet-id/name`（XOR，可都不传）。徽章是判据，本行只是速记。

```bash
# 统一调用范式：两组定位缺一不可（占位符别原样填；表名先 +workbook-info 查）
lark-cli sheets +csv-get --url "https://.../sheets/shtXXX" --sheet-name "<真实表名>" --range "A1:F30"
```

### 系统 flag

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--dry-run` | bool | 否 | 零副作用：仅打印请求路径与参数模板，不发起调用 |
| `--yes` | bool | 是（仅 `high-risk-write`） | 二次确认；不带时退出码 10。详见 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md) 高风险审批协议 |
| `--print-schema` | bool | 否 | 写复合 JSON flag 前结构不确定就先跑它：本地打印 Schema 并退出（不发起调用、不需要其它 required flag），搭配 `--flag-name` 指定查哪个 flag，省略时列出该 shortcut 可查的 flag。只有含复合 JSON flag 的 shortcut 支持。 |
| `--flag-name` | string | 否 | 配合 `--print-schema`：flag 名不带 `--` 前缀（`cells` / `properties`）。**支持点分路径切片**：`--flag-name properties.snapshot.plotArea.axes` 只打印该子树，大 schema（chart 的 properties 约 1700 行）按需取，别整篇翻页。 |

> **bool flag 语法**：开启可用裸 `--flag`；显式值只用 `--flag=true` 或 `--flag=false`，不得用空格分隔。

> ⚠️ **high-risk-write 命令清单（exit 10 强确认门禁）**：`+batch-update`、`+cells-clear`、`+cells-batch-clear`、`+sheet-delete`、`+dim-delete`、`+dropdown-delete`、`+history-revert`（整表回滚到历史版本），以及各对象删除 `+chart-delete` / `+pivot-delete` / `+cond-format-delete` / `+filter-delete` / `+filter-view-delete` / `+sparkline-delete` / `+float-image-delete`。
>
> **审批协议**：先 `--dry-run` 预览、向用户展示将执行的操作与影响范围，**获得用户明确同意后**再在原命令追加 `--yes` 执行。未经用户同意不得带 `--yes`，也不得在 exit 10 后静默补 `--yes` 重试——那等于禁用门禁。完整协议见 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)。

**Schema 的边界**：`--print-schema` 打印的是 flag 值的内部结构，flag 描述要求外层信封时（如 `--sheets` 的 `{"sheets":[…]}`）schema 里看不到那层，按描述补上；reference 的 `## Schemas` 段也只给一层。图表直接 `+chart-create --print-example <type>` 拿最小可用模板改参。

### flag 内容类型与输出约定（术语速记）

- JSON 类入参分三类：**复合 JSON** = 深层嵌套对象（`--print-schema` 可查）；**简单 JSON** = 一二维标量数组；**非 JSON 文本** = 原样文本（如 CSV）。
- **envelope**：所有 shortcut 返回统一外层 `{ok, identity, data, ...}`；写操作不会自动回读，校验自行调用 `+*-list` / `+*-get` / `+cells-get`。
- **大 payload 走文件 / stdin，不在命令行内联**：Type 标 `File + Stdin` 的 flag 支持 `--flag "@./x.json"`（`@file` 只接受 cwd 下相对路径，绝对路径被拒）与 `--flag -`（stdin）；payload 含换行 / 引号或体量大时一律落文件。**stdin 每次调用只能给一个 flag**——`+table-put` 的 `--sheets` 与 `--styles` 都是大 JSON 时，一个走 `-`、另一个走 `@./x.json`。临时文件不要落进用户项目目录。
- **非 POSIX shell（PowerShell / cmd.exe）适配**：本 skill 全部 `bash` 代码块（heredoc `<<'JSON'`、单引号转义 `'\''`）只适用于 bash / zsh，动手前先判断当前 shell，非 POSIX 环境按下表改写，**不要试错式改引号**——`@file`（cwd 相对路径）是全平台无引号问题的兜底形态：

| 形态 | bash / zsh | PowerShell | cmd.exe |
| --- | --- | --- | --- |
| 大 / 多行 JSON | `--flag - <<'JSON' … JSON` | 先写 UTF-8 无 BOM 文件再 `--flag '@./x.json'`，或 `Get-Content -Raw ./x.json \| lark-cli … --flag -` | 先写文件再 `--flag @./x.json`（cmd 无 heredoc / 管道读文件不可靠） |
| 单行 inline JSON | `--flag '{"a":1}'` | `--flag '{"a":1}'`（PS 单引号同为字面量） | 不要 inline——cmd 会吃掉内层双引号，一律走 `@file` |
