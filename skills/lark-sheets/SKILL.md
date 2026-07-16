---
name: lark-sheets
version: 3.0.2
description: "飞书电子表格：创建和操作电子表格。支持创建表格、管理工作表与行列结构（增删/合并/调整尺寸/隐藏/冻结）、读写单元格（值/公式/样式/批注/单元格图片）、查找替换、多操作原子批量更新，以及图表、透视表、条件格式、筛选器、迷你图、浮动图片等对象的创建与维护。当用户需要创建电子表格、管理工作表、批量读写或编辑数据、统计汇总与可视化、表格美化、公式计算（含 Excel 公式迁移）、金融/财务建模（DCF、三张表、预算、Sensitivity 等）等任务时使用。若用户是想按名称或关键词搜索云空间（云盘/云存储）里的表格文件，请改用 lark-drive 的 drive +search 先定位资源。当用户给出 doubao.com 的 /sheets/ URL/token 时，也应直接使用本 skill，不要因为域名不是飞书而回退到 WebFetch；路由依据是 URL 路径模式和 token，而不是域名。"
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

## 飞书表格编辑准则（动手前必守，所有编辑类任务一律生效）

下列准则横切所有飞书表格任务，**动手前先过一遍**——即使你是被索引直接路由进某个工具参考也一律生效。每条只给一句话纲要，展开与边界见括注的 reference。

1. **最小改动**：除任务要改的单元格 / 列外，原表其它单元格、行列结构、Sheet 名、合并区、格式 1:1 保持；中间结果放原数据右侧或新建空白 Sheet，**禁止删 / 改名 / 隐藏 / 移动已存在 Sheet**；改写类任务精确圈定行列，不该转的原值 1:1 保留。
2. **真实写回 + 回读校验**：交付必须是对在线表格的真实写入，写完用 `+csv-get` / `+cells-get` / `+<对象>-list` 回读确认实际生效——**写操作返回 `ok` 只代表请求被接受、不代表结果符合预期**；写公式后查错误码、筛选 / 排序后核对前几行、删除 / 清空后确认已空。禁止只在文本里声称"已完成"。
3. **读全再写**：批量填充 / 补齐 / 修正类任务先确认真实数据末行再写，只探前 N 行会漏写表尾（确定末行流程见 `lark-sheets-read-data`）。
4. **公式优先于硬编码**：能用公式表达的计算（总计 / 占比 / 增长率 / 提取 / 查找）一律写公式而非静态值；**凡可由表内其它单元格推导的派生值默认就用公式，即使用户没说"联动 / 自动更新"**；写任何飞书公式前先读 `lark-sheets-formula-translation`，而且**只要公式真实写入表格，收尾默认就要继续跑 `lark-sheets-formula-verify` 的 `+formula-verify`，直到 `status='success'`**。
5. **续写 / 扩展继承样式**：续写、补齐、复制区块、新增行列时禁止只读值只写值，必须连带 `cell_styles` + `border_styles` + 合并 + 行高一起继承（清单见 `lark-sheets-write-cells`，四边框最易漏）。
6. **多步写入合并 `+batch-update`**：多个连续写入、或同一工具对多区域重复调用，合并为单次原子 `+batch-update`（语义见 `lark-sheets-batch-update`）。
7. **分组汇总用透视表**："按 X 统计 Y / 分组汇总 / 各类数量金额"用 `+pivot-{create|update|delete}`，禁止用 SUMIF / 本地脚本拼一张假透视表。
8. **拆成可验证 checklist**：落地前把指令拆成所有"独立可验证子要点"，逐点 `assert` 全过才交付（多维排序每维一点、多目标每目标一点、范围类核起 / 末 / 边界）；只做第一个要点属违规。
9. **全量处理前置断言条数**：翻译 / 打标 / 批量公式落地等逐条任务，先把预期条数硬编码再 `assert actual == expected`，禁止输出"已完成前 N 条，剩余继续"的半成品。

> 上述准则的实操展开——读取路径、原生工具优先级、脚本配合、易漏陷阱——见下方「执行要点」节；端到端工作流为：了解结构（`+workbook-info`）→ 读数据 → 理解语义 → 原生工具优先 → 写入 → 回读验证。

## 场景 → 命令速查（拿不准命令名先查这里，别按直觉拼）

把高频意图映射到**真实存在**的 shortcut / flag。agent 常从 Excel / Google Sheets / 飞书 OpenAPI 误迁移命令名或 flag，先对照本表，避免一次必然失败的试错。完整 shortcut 见各工具参考。**选定命令后别急着写——先读「动手前读」列指向的 reference 再动手**：命令名对得上不代表用法对，写入 / 清除 / 透视类尤其容易漏掉 reference 里的防错、类型与样式继承规则。

| 你要做的事 | ✅ 正确写法 | 动手前读 | ❌ 不存在（会被 cobra 拒） |
| --- | --- | --- | --- |
| 读数据（纯值 / CSV） | `+csv-get`（范围用 `--range`） | `lark-sheets-read-data` | `+get-range`、`+range-get`、`+cells-read` |
| 读值 + 公式 / 样式 / 批注 | `+cells-get --include value,formula,style,comment,data_validation` | `lark-sheets-read-data` | `+get-cell`、`+cell-get`、`--with-styles`、`--with-merges`、`--include-merged-cells` |
| 写纯文本值（整块 CSV 平铺；列里**没有**需字面保真的数值 / 日期标签 / 编号——点分日期 `12.10`、编号 `001` 会被 csv-put 数值化，不算纯文本） | `+csv-put`（定位用 `--start-cell`，单个左上角锚点格；也接受 `--range` 别名，区间自动取左上角） | `lark-sheets-write-cells` | 把含点分日期(`12.10`)/编号(`001`)的列裸灌 `+csv-put`——会被数值化（`12.10`→`12.1`、`001`→`1`，尾零/前导零丢失），改用 `+table-put` 声明 `dtypes:object` |
| 写带类型的数据到**已有**表（列里有数字 / 金额 / 百分比 / 日期 / 计数等**本质是量值**的数据——不看当下要不要排序 / 求和，量值一律走这里） | `+table-put --sheets` 完整 payload `{"sheets":[{...}]}`（列名走 `columns`、二维数据走 `data`、列 pandas dtype 走 `dtypes`、列展示格式走 `formats`；来源不限 DataFrame——Counter / dict / list 同理；要同时美化加 `--styles` 一步带样式（区域底色 / 边框 / 列宽 / 行高 / 合并），不必事后再刷；payload 里不存在的 sheet 名会自动建子表，详见 write-cells） | `lark-sheets-write-cells` | 在本地把数字拼成 `"$1,234"` / `"30.5%"` 字符串再 `+csv-put`（会落成文本、丢失计算能力；常见借口见下方 ⚠️） |
| **新建**电子表格并写带类型的数据（类型保真需求同上，但目标表还不存在） | `+workbook-create --sheets`（协议与 `+table-put` 同构、一步建表 + typed 写入，无需先建空表再 `+table-put`；date / number 不丢；`--styles` 同样可在建表同一步带全套样式，详见 workbook） | `lark-sheets-workbook` | 用 `--values` 灌日期 / 数字（会落成文本、丢类型） |
| 写公式 / 富写入（样式 · 批注 · 图片 · 富文本），或需精确矩形定位的值 | `+cells-set`（定位用 `--range`；批注 / 图片 / 富文本只能用它，公式也可；**公式落表后继续 `+formula-verify` 收尾**） | `lark-sheets-write-cells` | — |
| 插图：图片**绑定到某条记录**、随行走（凭证 / 证件照 / 商品图 / 头像 / 二维码 / 每行配图） | `+cells-set-image`（单格 `--range`，嵌入单元格内） | `lark-sheets-write-cells` | — |
| 插图：**自由摆放、不绑数据**的装饰 / 标识（logo / 水印 / 封面大图 / banner） | `+float-image-create`（浮动图片，自由定位 + 尺寸 + 层级） | `lark-sheets-float-image` | — |
| 查找 / 替换文本 | `+cells-search`（找，关键字用 `--find`）、`+cells-replace`（替换） | `lark-sheets-search-replace` | `+cells-find`、`+find`、`--query` |
| 看子表结构（合并 / 行高列宽 / 冻结 / 隐藏） | `+sheet-info` | `lark-sheets-sheet-structure` | `+sheet-get`、`+structure-get`、`+sheet-structure-get` |
| 看工作簿 / 子表清单 | `+workbook-info` | `lark-sheets-workbook` | `+sheet-list`、`+workbook-get`、`+workbook-list` |
| 复核某次（AI）编辑改了什么 / 取两个版本间的变更 | `+changeset-get --start-revision <编辑前版本>`（省略 `--end-revision` 取到最新；版本差 ≤ 20） | `lark-sheets-changeset` | — |
| 取当前文档 revision（版本号） | `+revision-get` | `lark-sheets-workbook` | — |
| 导出 xlsx / 单表 csv | `+workbook-export` | `lark-sheets-workbook` | — |
| 导入本地 xlsx/xls/csv 文件为飞书电子表格 | `+workbook-import --file ./x.xlsx`（本地表格文件 → 飞书电子表格的正解；仅要导成多维表格 bitable 时才用 `drive +import --type bitable`） | `lark-sheets-workbook` | `drive +import`（导电子表格时绕了 drive 通道、还要多给 `--type`，应直接用 `+workbook-import`）、把 .xlsx 在本地读成数据再 `+workbook-create` 重灌（多此一举，应直接 `+workbook-import`）、要把文件并入某个**已有在线工作簿**（给它加子表）却用它——import 只会新建独立表，加子表应走 `+sheet-copy` / `+sheet-create` |
| 参考某个**已有在线表**、把多个本地文件 / 数据各作为一张子表**追加**进去（不另起独立表） | 先 `+workbook-info` 拿模板子表 `sheet_id` → `+sheet-copy` 逐张复制模板子表（公式 / 合并 / 分组底色 / 列宽 / 条件格式全继承）再用 `+cells-*` 只改数据；无模板可继承时 `+sheet-create` 建空子表 + `+table-put --sheets/--styles` 写入 | `lark-sheets-workbook` | 把文件 `+workbook-import` / `+workbook-create` 另起一张**独立新表**（目标是并入已有工作簿时就跑偏了；这两条只产新表、不接受已有表定位） |
| 清除内容 / 格式 | `+cells-clear`（范围维度用 `--scope`，取值 content / formats / all） | `lark-sheets-range-operations` | `--type` |
| 批量清除多区域 | `+cells-batch-clear`（`--scope`） | `lark-sheets-batch-update` | `--target` |
| 调整列宽 / 行高 | `+cols-resize` / `+rows-resize`（行、列是两个独立命令） | `lark-sheets-range-operations` | `--dimension`（无此 flag） |
| 分组汇总 / 透视 | `+pivot-create`（默认不传落点 flag → 自动新建子表，零覆盖） | `lark-sheets-pivot-table` | 用 SUMIF / 本地脚本拼一张假透视表 |
| 画图表 / 可视化（柱 / 折线 / 饼 / 条 / 散点 / 组合…） | `+chart-create` | `lark-sheets-chart` | matplotlib / 本地画图再贴图（原生图表可交互、随数据更新） |
| 条件高亮 / 数据条 / 色阶 / 重复值标记 | `+cond-format-create` | `lark-sheets-conditional-format` | `+highlight`、`+conditional-format`、逐格 `+cells-set-style` 硬凑 |
| 筛选 / 只看符合条件的行 | `+filter-create` | `lark-sheets-filter` | pandas filter 后覆盖写回（会毁原数据；要保存多份筛选状态用 `+filter-view-create`） |

> ⚠️ **动手前的触发式必读（按动作判定，不看主场景）**：本次操作只要**涉及样式 / 美化**（底色 / 边框 / 字号 / 对齐 / 数字格式 / 汇总行 / 配色 / 列宽行高），动手前先读 `lark-sheets-visual-standards`；只要**要写飞书公式**，动手前先读 `lark-sheets-formula-translation`（飞书函数与 Excel 有差异，凭直觉迁移易错），**写完后再读 `lark-sheets-formula-verify` 并执行 `+formula-verify` 收尾**。哪怕主任务是"建表 / 展开数据 / 录入"，只要动作里含美化或写公式就适用——别因"这不算专门的美化 / 公式任务"而跳过。
> ⚠️ **两种图片别选错**：图若**绑定某条记录、要随行排序 / 筛选 / 增删**（凭证 / 证件照 / 每行配图，话里带「对应 / 每行 / 这列」等绑定词）→ 单元格图片 `+cells-set-image`；只是自由摆放的装饰（logo / 水印 / 封面）→ 浮动图片 `+float-image-create`。别因「浮动图更好控制 / 更熟」默认选浮动图。
> ⚠️ **纯文本还是数值语义（看数据本质，不看当下用途）**：金额 / 百分比 / 比率 / 计数 / 日期等**本质是量值**的数据 → 一律数值写入，常规二维表用 `+table-put`（`dtypes` 声明类型 + `formats` 设展示格式），版式装不下（多级 / 合并表头的宽表 leaderboard 等）改用 `+cells-set` 传数字（百分比传小数 `0.4`）+ `number_format`，照样显示 `40%` 且数值无损。只有编号 / 身份证 / 单据号这类**本质是标识符**、要字面保真的才用 `+csv-put` 平铺。**几个常见借口都不成立**——"只是 leaderboard / 报表展示不用算""版式复杂""样式以后再刷、先铺文本"都不是把百分比写成 `"40%"` 字符串灌 `+csv-put` 的理由（展示不改变它是数值；类型不能后补，落成文本就回不来）。判据与操作展开见 `lark-sheets-write-cells`「数字还是文本」。
> ⚠️ **要新建子表 / 整表美化 → 别默认「`+csv-put` 写值再事后刷样式」**：`+table-put` / `+workbook-create` 的 `--styles` 能在写数据的**同一步**带全套样式（区域底色 / 边框 / 列宽 / 行高 / 合并），且 `+table-put` 的 payload 里若 sheet 名不在工作簿中会自动新建子表——**纯文本表要新建子表 + 美化时同样走这里**（`--styles` 与列是否 typed 无关），比「`+csv-put` 写值 + 多次 `+cells-batch-set-style` / `+*-resize` 刷样式」少好几次调用（冻结行列等 sheet 级属性仍需 `+dim-freeze` 单独一步）。
> ⚠️ **定位 flag**：`+cells-get` / `+cells-set` / `+csv-get` 用 `--range`；`+csv-put` 规范用 `--start-cell`（单个左上角锚点格），也接受 `--range` 别名（区间自动取左上角），二者择一即可。
> ⚠️ **读取附加信息**一律走 `+cells-get --include …`，**没有** `--with-styles` 这类 flag；**看合并单元格**用 `+sheet-info` 的 `merged_cells`，不要在 `+cells-get` 里找 merge flag。

## 执行要点（读取 / 原生工具 / 陷阱）

准则的实操展开。端到端工作流：了解结构 → 读数据 → 理解语义 → 原生工具优先 → 写入 → 回读验证。

### 读取：按需求选路径（细则见 `lark-sheets-read-data`）

| 用户需求 | 读取路径 |
|---|---|
| "完善 / 补齐 / 填空 / 修正所有 XX"、分析 / 清洗 / 大数据 | 原生优先（公式 / `+pivot` / `+filter`）；表达不了再分批 `+csv-get` 导出 + 脚本处理 + 分批回写（默认覆盖所有对应数据行，不以用户选区为准） |
| "查一下 / 看看 / 统计 / 汇总"等只读 | `+csv-get` 读到上下文 |
| 需要公式 / 样式 / 批注 | `+cells-get` |
| 续写 / 扩展已有内容 | `+csv-get` 看结构 + `+cells-get` 读源区样式 + `+sheet-info --include row_heights,merges`（见准则 5） |

> "补齐 / 填空"类用只读路径探 10 行就写会漏写表尾——写入前先按 `lark-sheets-read-data` 确认真实数据末行（准则 3）。

### 计算：原生工具优先，代码兜底（强化准则 7）

| 用户需求 | 用原生 | 禁止的替代 |
|---|---|---|
| 按 X 统计 Y、分组汇总 | `+pivot-{create\|update\|delete}` | pandas groupby → 写值 |
| 求和 / 计数 / 平均 / 占比 | 公式 | Python 算 → 写静态值 |
| 图表 / 可视化 | `+chart-*` | matplotlib |
| 条件高亮 / 色阶 | `+cond-format-*` | 逐格设样式 |
| 筛选 | `+filter-*` | pandas filter → 覆盖写入 |
| 文本提取 / 转换 / 查找 | 公式（REGEXEXTRACT / TEXT / VLOOKUP 等） | Python → 写静态值 |

只有多步清洗、统计建模、公式试错 3 次仍失败时才用代码。

### 用脚本配合 CLI 时

- **只读 stdout**：CLI 数据走 stdout、诊断走 stderr；解析 JSON 别 `2>&1`（警告混入会解析失败），用管道或单独重定向 stdout。
- **喂 CLI 的 CSV / JSON 用 UTF-8 无 BOM**；临时文件放系统临时目录、勿落项目目录。
- **命令失败先读 stderr 再调整**，别原样重发。
- **回写纯单元格值**：剥离 `值(V-Align: bottom)` 这类"值(样式)"串与残留引号再写；排序优先 `+range-sort` 原生工具，别"读出本地排完再整列写回"。

### 易漏陷阱

- **`+dim-insert` 不继承行高**：只继承值 / 公式 / 边框，新行回落默认高度截断长文本；插行填长文本前读相邻行 `row_height`，用 `+batch-update` 合 `+rows-resize` 补齐。
- **公式容错**：日期 / 查找 / 数值转换公式用 `IFERROR` 包裹；写完读结果列首末各 5 行查 `#VALUE!` / `#REF!` / `#DIV/0!`，然后继续跑 `+formula-verify` 直到 `status='success'`；同一方案试错上限 3 次。
- **循环引用**：聚合公式引用范围不能含目标 cell 自身或其传递依赖。
- **隐藏行列**：`+csv-get` 默认含隐藏行列；设 `--skip-hidden=true` 只看可见，但返回行序号与实际行号不再对应。
- **跨 sheet 对象**：图表 / 条件格式 / 透视表 / 浮动图片可能分布在多个子表，操作前先 `+workbook-info` 掌握全局。
- **NLP 任务分批**：语义理解 / 翻译 / 改写 / 分类等用 NLP 处理（代码只做分批 / 行号映射 / 写回）；数据量大必须分批（通常 30 行 / 批），每批处理完即时写回，单批生成通常 ≤ 300 行，多批用 `+batch-update`。

## References

本 skill 的 reference 分两组：先读**通用方法与规范**（横切所有任务的样式、公式规则，不含具体 shortcut），它们规定了"怎么做对"；再按操作对象进入**工具参考**查具体 shortcut 与调用细节。编辑类任务务必先过一遍通用方法与规范，连同上方「飞书表格编辑准则」对所有工具参考一律生效。

### 通用方法与规范（先读，横切所有任务，不含具体 shortcut）

| Reference | 描述 |
| --- | --- |
| [飞书表格样式与配色规范](references/lark-sheets-visual-standards.md) | 飞书表格样式与配色规范：表头/数据区/汇总行的颜色、字号、对齐、边框、数字格式等取值标准，以及从零新建表格的版式美化、新增汇总行、追加行列继承原表风格、已有区域美化等典型场景的决策流程与样式要点。工具调用参数细节请参考对应的 lark-sheets-write-cells / lark-sheets-range-operations / lark-sheets-batch-update。条件格式（高亮、标红、数据条、色阶）请使用 lark-sheets-conditional-format。 |
| [飞书表格公式生成规则](references/lark-sheets-formula-translation.md) | Excel 公式到飞书表格公式的迁移与生成规则。核心目标不是保留 Excel 原语法，而是按飞书表格可执行规则重写公式，并在结果上尽量对齐 Excel。当用户要求把 Excel 公式改写成飞书表格公式，或需要生成飞书公式（尤其涉及 ARRAYFORMULA、原生数组函数、INDEX/OFFSET、MAP/LAMBDA、日期差、多层范围结果与二次展开）时使用。本文只负责把公式写对，落表后的强制收尾请接 `lark-sheets-formula-verify`。 |

### 按对象的工具参考（含 shortcut）

| Reference | 描述 |
| --- | --- |
| [Lark Sheet Formula Verify](references/lark-sheets-formula-verify.md) | 公式写入 / 批量填充 / `--copy-to-range` 扩展 / 导入含公式工作簿后的强制自检入口。对指定子表（或整本工作簿）扫描公式与单元格值，聚合所有 Excel 错误（#REF! / #DIV/0! / #VALUE! / #NAME? / #NULL! / #NUM! / #N/A），同时合并最近一次写入留下的编译失败（formula_errors），输出统一 JSON 让 AI 一次拿到完整健康度报告。只要任务涉及写公式，落表后就应调用 +formula-verify 收敛到 zero-error；`status='errors_found'` 或 `status='partial'` 时禁止把链路标为完成。 |
| [Lark Sheet Workbook](references/lark-sheets-workbook.md) | 管理飞书表格的工作簿结构（子表列表及元数据）。当用户提到"看看这个表格有什么"、"表格结构"、"有哪些 sheet"、"新建一个 sheet"、"删除这个工作表"、"重命名"、"复制一份"、"移动到前面"时使用。 |
| [Lark Sheet Sheet Structure](references/lark-sheets-sheet-structure.md) | 管理飞书表格的子表结构与布局。适用场景：查看行高、列宽、隐藏行列、合并单元格等布局信息，以及"插入一行"、"删除这列"、"隐藏行"、"冻结表头"、行列分组（大纲折叠/展开）等操作。行列大纲仅在用户明确提到"行分组"、"列分组"、"大纲"、"outline"时才触发，"按XXX分组"等数据分组场景请使用 lark-sheets-pivot-table。如需在表尾追加数据，应先通过此 skill 插入行，再通过 lark-sheets-write-cells 写入。 |
| [Lark Sheet Read Data](references/lark-sheets-read-data.md) | 读取飞书表格中的单元格数据。当用户需要"看看数据"、"分析数据"、"统计/汇总"时使用；也适用于需要查看公式、样式、批注等详细信息的场景。 |
| [Lark Sheet Search & Replace](references/lark-sheets-search-replace.md) | 在飞书表格中搜索和替换文本，支持限定范围、大小写匹配、精确匹配、正则表达式。当用户需要"查找"、"搜索"、"定位"某个值，或"替换"、"批量修改文本"、"把 A 改成 B"时使用。不要用于理解表格结构（应读取数据）、不要用于数据分析（应读取数据后计算）、不要把用户操作动作中的关键词（如"汇总金额""统计数量"）当作搜索词。 |
| [Lark Sheet Write Cells](references/lark-sheets-write-cells.md) | 向飞书表格的指定区域批量写入值、公式、样式、批注或单元格图片。适用场景：填写数据、设置公式、修改格式、添加批注、嵌入单元格图片（如需操作浮动图片，请使用 lark-sheets-float-image）；若只需把一块 CSV 批量铺到表格上（值或公式，不带样式/批注），直接使用 `+csv-put` 更短更快。追加数据需先通过 lark-sheets-sheet-structure 插入行列。只要这次写入真实落了公式，收尾默认继续执行 `lark-sheets-formula-verify`。 |
| [Lark Sheet Range Operations](references/lark-sheets-range-operations.md) | 对飞书表格中指定区域执行结构性操作（不涉及写入单元格数据值）。适用场景：清除内容或格式（"清空"、"删除内容"、"去掉格式"）、合并/取消合并单元格、调整行高列宽（"加宽列"、"自适应列宽"）、移动/复制/填充/排序数据（"移动数据"、"复制到"、"自动填充"、"按某列排序"）。写入单元格数据请使用 lark-sheets-write-cells。 |
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

各 reference 的每个 shortcut 标题下用一行徽章标注该 shortcut 支持的公共 / 系统 flag，例如：

- `_公共四件套 · 系统：--dry-run_` — URL/token + sheet 定位（两组各**必给一个**，详见下方「公共 flag」），加 `--dry-run`
- `_公共：URL/token（无 sheet 定位） · 系统：--yes、--dry-run_` — 只接 URL/token，常见于 `+batch-update` 等不强制 sheet 定位的 shortcut

徽章里只列名字。type / 必填 / 描述都在本段统一声明：

### 公共 flag（定位资源）

**公共四件套** = `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`，分成两组 XOR，**每组都必须给且只能给一个**（XOR = 二选一必填，不是"可选"）：

1. **spreadsheet 定位（必填）**：`--url` 与 `--spreadsheet-token` 二选一，**必须给其中之一**。两个都不给 → 校验报错 `specify at least one of --url or --spreadsheet-token`；两个都给 → 互斥冲突。
   - **`--url` 解析 `/sheets/`、`/spreadsheets/` 与 `/wiki/` 三种链接**（从路径里抽出 token；也可以直接把裸 token 传给 `--spreadsheet-token`）。其它形态的链接不会被解析成表格 token。
   - **`/wiki/` 知识库链接可直接传 `--url`**：会自动定位到链接背后的电子表格；若该链接背后不是电子表格（而是文档 / 多维表格等），则报错。
   - **例外**：`+workbook-create`（新建表 + 可选写入数据）与 `+workbook-import`（把本地文件导入为新表）都产出一张**还不存在**的表格，**不接受任何 spreadsheet / sheet 定位 flag**——`+workbook-create` 只有 `--title` / `--folder-token` / `--values` / `--styles` / `--sheets`，`+workbook-import` 只有 `--file`（必填）/ `--folder-token` / `--name`。
2. **sheet 定位（公共四件套 shortcut 必填）**：`--sheet-id` 与 `--sheet-name` 二选一，**必须给其中之一**。两个都不给 → 校验报错 `specify at least one of --sheet-id or --sheet-name`。
   - ⚠️ **不确定 sheet 名时禁止直接猜 `Sheet1`**：除非用户对话明确说出 sheet 名 / id，或上下文（之前的工具调用 / URL 锚点 `?sheet=xxx`）已经出现过具体值，否则**第一步先调 `+workbook-info --url "..."`**（或 `--spreadsheet-token`）拿 `sheets[].sheet_id` / `sheets[].title` 列表再选。中文环境下子表常叫"数据" / "Sheet"（无数字）/ "工作表 1" / 业务名，猜 `Sheet1` 大概率撞 `sheet not found`，比先查多耗一次失败调用 + 重试。
   - ⚠️ **`--range` 里的 `Sheet1!` 前缀不能替代 sheet 定位**：即使写了 `--range 'Sheet1!A1:B2'`，仍**必须**额外传 `--sheet-id` 或 `--sheet-name`，否则照样报上面的错。
   - ⚠️ **A1 reference 含 `!`**（`--source` / `--range` / `--ranges`）**：整段用单引号包裹**，如 `--range 'Sheet1!A1:B2'`——单引号能挡住 bash 的 history expansion（`!` 被拦成 `event not found`；双引号挡不住；别改用 `set +H`，原因见下方「复合 JSON / 大入参」）。sheet 名含特殊字符（`-` / 空格 / 非 ASCII）需在内部按 A1 标准再包一层单引号时，用 `'\''` 转义保持外层单引号，如 `--source ''\''Sales-2025'\''!A1:D100'`。
   - **例外**：徽章标为 `_公共：URL/token（无 sheet 定位）…_` 的 shortcut（如 `+workbook-info` / `+workbook-export` / `+batch-update` / `+dropdown-update|delete` / `+cells-batch-set-style` / `+cells-batch-clear` / `+sheet-create`）**不接受也不需要** sheet 定位，只给一组 spreadsheet 定位即可。`+pivot-create` 用 `--target-sheet-id` / `--target-sheet-name`（XOR，可都不传，落点细节见 `lark-sheets-pivot-table`）。

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--url` | string | 二选一必填（与 `--spreadsheet-token`） | spreadsheet 或 wiki URL |
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

**参数含特殊字符（`!` / 引号 / 空格 / 非 ASCII）时，用单引号包裹该参数即可，不要起手 `set +H` 之类的 shell 开关来防转义。** `set +H`（关 bash history expansion）在 `sh` / `dash` 下是非法选项（`set: Illegal option -H`）、会让整条命令直接失败；而单引号挡得住 `!` 的 history expansion（否则报 `event not found`），对 bash 与 `sh` / `dash` 一致安全。参数本身含单引号、或 payload 较大时，按上文走 stdin。

**`@file` 接绝对路径会被拒，且被拒后不要照报错提示做。** `@file` 出于安全只接受 cwd 下的相对路径，传 cwd 之外的绝对路径会被拒。此时报错会建议"先 cd 到目标目录，或改用相对路径"——**两条都不要照做**：cd 过去、或把临时文件写进用户项目目录，都会污染工作目录。正解是改用 stdin（`--<flag> - < 文件`）。
