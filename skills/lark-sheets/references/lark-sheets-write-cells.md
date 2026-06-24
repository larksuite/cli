# Lark Sheet Write Cells

## 写入边界 + 回读校验（编辑类任务必做）

1. **明确写入边界**：写入前必须能回答"目标 range 的起止行列号是多少？是否落在用户授权范围内？"。除用户明示要修改的区域外，禁止扩张到原数据列以外或新建 Sheet。
2. **完整性断言**：批量写入前先把"预期写入条数"硬编码到代码里（如要填 106 条翻译 → `expected = 106`），写完后回读断言 `actual == expected`。少于预期就继续写，禁止交付半成品。
3. **回读抽样校验**：写完关键值 / 公式后，用 `+csv-get` 或 `+cells-get` 重新读取写入区域，至少抽样 3-5 个代表性单元格（首 / 中 / 末），核对值与预期一致（与本地脚本计算的预期值对照）。公式特定的"先验证模板再 --copy-to-range / 修完再读回"细则见下方相关章节。
4. **护原表 · 派生产物落点（写排名 / 标记 / 汇总 / 改写列时易丢数据）**：派生结果一律写到**真实末列 +1 的全新空列**或新建子表，**禁止复用任何已有原数据列**——哪怕该列看起来"空"，也要先 `+csv-get` 回读确认整列无原始数据再写。三条铁律：① 不把新公式 / 新值写进原数据列（典型反例：把新算的排名公式写进了原本存放另一份原始数据的列，整列原始数据被覆盖丢失）；② 不改写、不合并原表头字段名（典型反例：把几个独立表头字段合并成一列，原字段名丢失）；③ 慎用 `--allow-overwrite`：它一旦让写入区盖到相邻原始列 / 行就是不可逆数据丢失，加它之前必须用 `+sheet-info` / `+csv-get` 核清目标 range 不含任何原始数据。

## 新增列 / 新增行的样式继承（防止视觉风格不一致）

新增列 / 新增行**必须**先用 `+cells-get` 读相邻原列 / 原行的完整样式作为模板，**禁止**只传 `value` 期望默认样式与原表一致——飞书新单元格默认对齐通常是 `H:right, V:bottom`，与多数原表的 `H:center, V:middle` 不一致。

**完整继承清单**（写新列 / 新行时 cells 数组必须同时携带）：

1. `cell_styles.font_size` / `cell_styles.font_weight` / `cell_styles.font_color` / `cell_styles.font_style`（字号 / 粗细 / 颜色 / 斜体等）
2. `cell_styles.horizontal_alignment` / `cell_styles.vertical_alignment`（H-Align / V-Align）—— 漏继承会导致新列对齐与原列不一致（常见）
3. `cell_styles.number_format`（小数位 / 千分位 / 百分比 / 日期格式）—— 漏继承会导致同列数值格式混乱
4. `cell_styles.background_color`（背景色）
5. `border_styles`（四边框）
6. **`merged_cells`（合并范围）**——续写场景必查：用 `+sheet-info --include merges` 读原数据区域的合并信息。**原行有跨列合并**（如标题行 `A1:G1` 合并）时，新行**必须**用 `+cells-{merge|unmerge}` 工具复制相同合并模式到新行（如续写第 3 个周报块的标题行 `A23:G23` 必须合并）。仅传 cells 数组的 5 类样式不够——合并范围要单独靠 `+cells-{merge|unmerge}` 工具落地（典型反例：续写多周记录表时，新增周次的标题行未合并，视觉上与原前几周风格不一致）

**采样模板的正确做法**：
- 表头新列 → 读相邻表头单元格（如新加 D1 → 读 A1/B1/C1 任一）
- 数据新列 → 读相邻数据行单元格（如新加 M5:M100 → 读 L5 / L6 / L7）
- 续写新行 → 读最近一行已有数据（如续写第 20 行 → 读 19 行所有列）

**反模式**（违规）：
- 只传 `{"value": "四级菜单"}` 给 D1，不传 `cell_styles` → D1 默认非加粗、非居中，与 A1/B1/C1 风格断裂
- 新列 M5 写入 `=SUM(F5:L5)` 时只传 `formula`，不传 `cell_styles.horizontal_alignment / vertical_alignment / number_format` → M 列对齐变 `H:right`，数字格式变默认

## 长数字防科学计数法（数值列写入必查）

写入或计算结果可能产生长数字（≥ 12 位整数 / 高精度小数）的列，**必须**在 `cell_styles.number_format` 显式设置非通用格式，否则飞书会自动用科学计数法显示，用户看到的就是"内容被截断 / 看不清原值"。

| 场景 | 必加的 `number_format` |
|---|---|
| 长整数（订单号 / 身份证 / 单据号） | `"0"` 或 `"@"`（强制文本，避免精度丢失） |
| 金额 / 千分位 | `"#,##0.00"` |
| 百分比 | `"0.00%"` |
| 数量 / 计数 | `"0"`（整数） |
| 日期 | `"yyyy-mm-dd"` 或 `"yyyy/m/d"` |

**典型反例**：长数字列（如审批单号、流水号）未设 `number_format`，飞书显示为 `1.23E+15`，用户复制出来已经丢失精度。

## 使用场景

写入。向飞书表格的单元格区域写入值、公式、样式、批注、图片或下拉，也可批量写入 CSV / DataFrame。本 reference 覆盖 6 个 shortcut，按数据来源 + 内容形态选：

| 场景 | 用这个 shortcut | 原因 |
|------|----------------|------|
| 模型手里已经有 CSV 文本（小规模手动构造、从 `+csv-get` 取到后简单加工） | `+csv-put` | 直接传 CSV 文本 + `--start-cell`，不用自己拼二维 cells 数组；必要时自动扩容行列 |
| 列里有数值语义的数据（数字 / 金额 / 百分比 / 日期 / 计数）→ 飞书，要类型保真（来源不限：DataFrame、Counter、dict、list 都算） | `+table-put` | typed 协议（外层 `{"sheets":[{"name":"…","columns":[...],"data":[[...]],"dtypes":{...},"formats":{...}}]}`，**只有这四件套字段**）：`dtypes` 用 pandas dtype 串声明列类型（`int64` / `float64` / `datetime64[ns]` / `bool` / `object`），`formats` 给每列展示格式（千分位 / 百分比 / 日期）。**date 落真日期、金额 / 百分比 / 计数等数值列保精度且带 `number_format`（可排序 / 求和 / 入图表）**、string 保前导零，多 sheet 一次写。**只要列有数值语义就走这里**，不要在本地把数字拼成带 `$` / `%` 的字符串再走 `+csv-put` |
| 写入含样式、批注、图片、数据校验等任意富写入 | `+cells-set` | 唯一支持完整富字段的 shortcut（公式 `+csv-put` 也能写） |
| 只改已有 cell 的样式，不动 value/formula | `+cells-set-style` | 拍平 10 个样式字段为独立 flag；不触发不必要的值写入 |
| 单 cell 嵌入图片 | `+cells-set-image` | 比 `+cells-set` 参数更简短 |
| 大量纯值 + 需要表头样式/边框 | 先用 `+csv-put` 写值，再用 `+cells-set-style` 补样式 | 分工配合，入参最短 |

**优先级**：常规批量写入（纯值或公式）优先 `+csv-put`（最短入参，直接传 CSV 文本）；含样式/批注/图片才用 `+cells-set`。⚠️ 这里"纯值"特指**已是文本、无需保留数值语义**的内容；只要列里是金额 / 百分比 / 日期 / 计数等有数值语义的数据，应优先 `+table-put`（用 typed 协议的 `dtypes` 声明列类型 + `formats` 设展示格式），而不是 `+csv-put`。

⚠️ `+csv-put` 可写值或公式：以 `=` 开头的单元格会被当作公式计算（读回时 `formula` 字段保留、`value` 为计算结果）。**公式内部含逗号 / 引号 / 换行时必须按 RFC 4180 转义**——含逗号的字段整格用双引号包裹、字段内部的引号再翻倍：如 `=COUNTIF(D5:D22,"及格")` 必须写成 `"=COUNTIF(D5:D22,""及格"")"`（外层双引号包裹整格，内部 `"及格"` 的引号翻倍成 `""及格""`）。漏转义会被 CSV 解析器按逗号拆列、整块写入区域错位（如本该 `G4:H6` 错成 `G4:K4`），详见下方 `+csv-put` 示例。**因此含逗号 / 引号 / 换行的公式优先改用 `+cells-set`（JSON 二维数组）写入——`cells[r][c].formula` 字段直接放公式串，零 CSV 转义负担，从根上避免拆列错位**（`+table-put` 的 typed 协议只接受 `columns / data / dtypes / formats` 四件套、没有 `formula` 字段，公式写入只能走 `+cells-set` / `+csv-put`）。此外 `+csv-put` **不会**携带样式/批注/图片，也无法把 `=` 开头的内容当字面量文本写入；需要样式/批注/图片用 `+cells-set`（或"写值 + 补样式"两步法）。

⚠️ **别把本该是数值的列格式化成字符串用 `+csv-put` 写入**：金额 / 百分比 / 市值 / 计数等列，若在本地拼成带 `$` / `%` / 千分位的字符串（如 `"$1,234.50"` / `"+30.5%"`）再 `+csv-put` 灌进去，单元格会变成**文本**——丢失排序 / 求和 / 图表 / 透视能力，且与 `number` 列混排时无法参与计算。正解是 `+table-put --sheets` 完整 payload（外层一定要带 `{"sheets":[...]}`、列名走 `columns`、二维数据走 `data`、列 pandas dtype 走 `dtypes`、列展示格式走 `formats`），数值列用 pandas dtype 串如 `dtypes:{"价格":"float64"}`（百分比同样存小数 `0.305`），并配 `formats:{"价格":"$#,##0.00","完成率":"0.0%"}` 做展示格式，**显示效果完全相同、数值无损**。判断信号：**当你准备把一个数字 format 成字符串再写时，几乎总该用 `+table-put` 而非 `+csv-put`**。

⚠️ 大数据回写走"`+csv-get` 按 `--range` 行窗口分批读到本地 + 本地脚本处理 + `+csv-put` 分批回写"。

## `+cells-set` 写入要点（常用模式 / 公式 / 样式）

> 以下是用 `+cells-set`（及 `+cells-set-style`）做富写入时的常用模式与铁律；选哪个 shortcut 见上方「使用场景」。

`+cells-set` 为一块区域设置值 / 公式 / 批注 / 样式，也支持 `rich_text` 的 `type: "embed-image"` 嵌入单元格图片。**关键：`cells` 二维数组的行列维度必须与 `range`（闭区间）严格一致，否则触发 `InvalidCellRangeError`**——维度计算示例见文末 `## Schemas` 的 `--cells`。

> **单元格图片 vs 浮动图片（最易选错）**：图若**属于某条记录、要随那行排序 / 筛选 / 增删**（凭证 / 证件照 / 每行配图，话里带「对应 / 每行 / 这列」等绑定词）→ **单元格图片**（本工具）：用 `+cells-set-image`（最短）或 `+cells-set` 的 `rich_text` + `type: "embed-image"`。只是自由摆放的装饰（logo / 水印 / 封面）→ 浮动图片，见 lark-sheets-float-image。别因「浮动图更好控制 / 更熟」默认选浮动图——它承载"对应某记录"的图会随增删行 / 排序错位。

常用模式（**必须遵守，禁止逐行写入替代**）：

- 整列公式：先在 `H2` 写一个公式，再用 `--copy-to-range "H2:H100"` 或 `--copy-to-range "H:H"` 向下填充。**禁止对每一行单独调用 `+cells-set` 写入相同结构的公式**
- 整列格式：先在 `J1` 写一个带样式的模板单元格，再用 `--copy-to-range "J:J"`
- 首行样式：先在 `A1` 写一个模板单元格，再用 `--copy-to-range "1:1"`
- 用户说”这列 / 整列 / 这行 / 首行 / 向下复制”时，**必须**使用模板单元格 + `--copy-to-range`
- 多区域写入相同格式/公式结构时，优先写一个模板，再用 `--copy-to-range` 复制到所有目标区域

⚠️ **逐行写入公式是常见低效写法**：对每一行单独调用 `+cells-set` 写公式（如 26 次）既慢又易错，且不会自动平移公式引用。正确做法是 1 次模板写入 + 1 次 `--copy-to-range`（公式引用自动平移）。

💡 **写入公式前先按迁移规则改写**：如果公式来自 Excel 或包含数组场景，先读取并遵循 `lark-sheets-formula-translation` 的规则完成改写，再把最终公式写入 `formula` 字段。

💡 **内容与样式分离写入（推荐）**：当需要同时写入内容和样式时，`cells` 中每个单元格都带上 `cell_styles` / `border_styles` 会导致入参非常冗长。由于同一区域的样式通常高度重复（如整列统一背景色、统一边框），推荐拆成两步：
1. **先写内容**：`+cells-set` 只传 `value` / `formula`，不带样式，`cells` 入参精简
2. **再批量刷样式**：对区域中的一个单元格写入目标样式作为模板，再用 `--copy-to-range` 将样式扩展到整列 / 整行 / 整个区域（`--copy-to-range` 会复制值、公式和样式，所以模板单元格应已包含正确的值）

示例：要对 A2:A100 写入数据并统一设置蓝色背景 + 边框：
```
Step 1: `+cells-set` — range="A2:A100", cells 只含 value（无样式，入参短）
Step 2: `+cells-set` — range="A2", cells 含 value + cell_styles + border_styles（单个模板）, --copy-to-range="A2:A100"
```
这比在 99 个单元格中都重复写样式 JSON 高效得多。

💡 **样式更新是「部分合并」，不是整体覆盖**：`+cells-set-style` / `+cells-batch-set-style`（以及 `+cells-set` 的 `cell_styles` / `border_styles`）只改你**显式传入**的样式属性，未传的属性保留原值。两个实用推论：
- **可分层叠加**：对同一区域先刷字体色、再单独刷背景色、再单独刷边框，后一步不会清掉前一步——美化已有区域时无需一次带齐所有字段，可拆成多次窄调用。
- **`border_styles` 按边合并**：只传 `{"top":{...}}` 只更新上边框，`bottom` / `left` / `right` 保留原状；不必为了「只改一条边」而把四边全部重传。（例外见上方「新增行的边框/样式禁止用 `{}` 跳过」：**全新行**底子里没有边框，仍需把要显示的边都显式传出。）

💡 **大批量数据分批写入（推荐）**：当需要写入大量行（如几十行以上）时，不要试图在一次调用中生成全部 `cells` 数据——`cells` 数组过大会让单次生成的内容过长，容易出错或被截断。应将数据拆分为多批，每批 20-50 行，分多次调用 `+cells-set` 逐批生成并写入（如先写 `A2:D21`，再写 `A22:D41`，依此类推）。每次只生成当前批次的数据，控制单次生成量。

注意：

- 不要把 `cells` 写成字符串化 JSON
- `+cells-set` 默认即覆盖非空 cell（`--allow-overwrite` 默认 true）；若要**保护**非空 cell 不被覆盖，显式传 `--allow-overwrite=false`（遇非空 cell 报错）
- 若目标区域涉及合并单元格，不要向合并区域中的非左上角单元格写入数据；如需写入，应改写合并区域左上角单元格，或先调整/取消合并区域
- **构造 `range` 时行号必须基于逻辑行号**：如果之前通过 `+csv-get` 读取了数据，CSV 中被双引号包裹的多行字段（如 `"2026年3月2日\n星期一"`）是**一个单元格**，不是两行。写入时的行号必须按逻辑记录计算，不能按物理换行符计数，否则 `range` 会整体偏移导致写入到错误位置

> 用户说"样式和原表一致 / 保持原表格式 / 边框继承"时同理：`cell_styles` 只覆盖字体和对齐、**不含边框**，边框必须用独立 `border_styles` 字段传——完整继承清单见上方「新增列 / 新增行的样式继承」。

⚠️ **公式写入必须自己校验结果（后端不会报语法错）**：`+cells-set` 写公式时，即便公式有括号不配对（如 `=IFERROR(VALUE(REGEXEXTRACT(D5, "\d+"))), 0)` 比 IFERROR 多一个 `)`）或用了飞书不支持的函数（如 `GOOGLETRANSLATE` / `CUBEVALUE`），**后端工具也会返回 `updated_cells_count=N, rc=0` 的"成功"**——错误会静默写进单元格显示为 `#VALUE!` / `#NAME?` / `#REF!`。因此：
1. **写完立即读回**：`+cells-set` 后紧跟 `+csv-get`（或 `+cells-get`）读目标范围前几行，检查是否出现 `#VALUE!` / `#NAME?` / `#REF!` / `#N/A` / `#DIV/0!` / `#NUM!`
2. **看到 `#` 开头的错误值**立即修公式：`#NAME?` 多半是函数名拼错或用了飞书不支持的函数（如 `GOOGLETRANSLATE` / CUBE 系列；注意 `UNIQUE` / `FILTER` / `SPLIT` 飞书是支持的）；`#VALUE!` 多半是类型不匹配或括号错位；`#REF!` 是引用错误；`~CIRCULAR~REF~` 是循环引用（公式引用了自身或会闭环）
3. **`--copy-to-range` 扩展前先验证模板**：模板单元格公式自己都算错，`--copy-to-range` 复制到 100 行就是 100 个错误
4. **去重 / 筛选函数**：飞书**支持** `UNIQUE` / `FILTER` / `SPLIT`（原生数组函数，详见 `lark-sheets-formula-translation`），可直接用；`DISTINCT` 不是飞书函数，去重用 `UNIQUE`。大数据量去重 / 分组也可用透视表（`+pivot-{create|update|delete}`，值字段聚合方式选 count）
5. **循环引用预检**：写聚合公式（SUM / AVERAGE / COUNT 等）前必须明确**引用范围不包含目标单元格自身或其传递依赖**。典型反例：在 C3 写 `=SUMIF(B:B,LEFT(B3,9)&"*",C:C)`，B 列匹配 B3 前 9 位时 C3 自己也命中，导致 C3 自引用 → `~CIRCULAR~REF~`。修法：用辅助列 / 显式排除自身（`SUMIFS(C:C, B:B, ..., A:A, "<>"&A3)`）/ 缩小范围避开自己
6. **REGEX 模式覆盖率验证**：公式里的 `REGEXEXTRACT` / `REGEXMATCH` / `REGEXREPLACE` 等正则模式落地前必须用本地脚本在源列上跑一遍命中率统计（`df[col].str.contains(pattern).mean()`）；命中率 < 100% 时必须扩展 pattern 或加多分支（IFS / 多个 IFERROR 串联）兜底，**禁止**只覆盖样本前 N 行就交付（典型反例：用 `REGEXEXTRACT(D5,"长(\d+)")` 只匹配带"长"前缀的尺寸文本，对"宽×高"、"×"、"*"等其它分隔符直接漏匹配）
7. **公式范围与用户指令字面对齐**：用户说"对 F 至 L 列求和"就必须写 `SUM(F2:L2)` 或 `F2+G2+H2+I2+J2+K2+L2`，**不能漏列、多列、错列**。写完用 `+cells-get` 拿回 `formula` 字符串，与用户原话逐字对照（参与求和的列名一致 / 起止列号一致 / 运算符一致），不一致就是违规
8. **量纲 / 单位换算 / 数量乘项预检（公式不报错但结果整体偏倍数）**：从文本提取数字做计算前，先核对**单位是否统一、是否漏乘数量、口径是否一致**——这类错误公式能跑通、无 `#` 报错，回读也看不出（值"像对的"）。必须用本地脚本对 3–5 个代表行**离线手算一遍预期值**，与公式结果逐格比对量级：① 单位不一致先统一再算（典型反例：尺寸 `320CM*337CM` 直接取数相乘除以 1e6 得 0.11，正确是 CM→MM 换算后得 10.78，**差 100 倍**）；② 按"单件×数量"的量必须乘数量列（典型反例：侧面板面积漏乘 F 列数量，F=2 的行只算了一半）；③ 标准值口径对齐（典型反例：营养成分 mg/kg 与 g/100g 口径混用，整列放大 100 倍）。**口径 / 单位 / 数量任一项错，整列计算结果就是错的；这类错误公式不报错、回读也不易看出，必须靠离线手算对照。**

⚠️ **收到 `formula_errors` 反馈后不要只打补丁**：`+cells-set` 返回值里若出现 `formula_errors: [{cell, formula, error_type, detail}]`，说明某些 cell 公式编译失败（`error_type=compile_failed` 通常是函数语法错如 `SPLIT(x)[1]` 的下标取值飞书不支持（SPLIT 本身支持，取第 N 项用 `INDEX(SPLIT(...),N)`）；`non_formula` 是 `=` 开头但解析不通过）。此时**禁止只聚焦修报错点的局部语法**（如仅把 `[1]` 换成 `INDEX(..,1)`），必须：

1. **重新审视整条公式的完整性**：被 formula_errors 标出的那一行，公式除了下标语法错，还可能有其他先天缺陷（字符清洗不全、IFERROR 兜底漏条件、引用列写错），修完语法错后立即整体复核
2. **同步对称修复所有相似列**：如果同一任务涉及多列相似处理（如"算 H 列面积"用 D 列尺寸、"算 I 列面积"用 E 列尺寸），**修完一列必须把同样的清洗/兜底逻辑同步到所有相似列**，禁止出现 H 列用 `SUBSTITUTE(长)+SUBSTITUTE(高)+SUBSTITUTE(×)` 而 I 列只用 `SUBSTITUTE(×)` 这种不对称处理——会导致一列编译通过有值、另一列编译通过但 IFERROR 全返回空，用户看到的是"数据为空"而非"公式错"
3. **修完再读回验证**：不只看 `formula_errors` 为空（这只证明编译通过，不证明运行时有值），必须 `+csv-get` 读目标列前 3-5 行，确认**非空源数据对应的目标列有非空计算结果**
4. **核心心智**：`formula_errors` 是"帮你暴露编译错"的工具，不是"修掉它就收工"的通行证。编译通过 + 运行时 IFERROR 兜底空 = 用户视角的"没算出来"

⚠️ **新增行的边框/样式禁止用 `{}` 跳过**：`cells` 数组里 `{}` 的语义是"**此单元格不做任何修改、保留原状态**"。这在写入**已有行**时是安全的（原有边框/样式保持不变），但在写入**新行**（比如表尾追加汇总行、扩展行）时是灾难：新行底子里本来就没边框，`{}` 不修改 = 保留无边框状态，导致该 cell 视觉断裂。

⚠️ **"汇总行"识别 → 读 `lark-sheets-visual-standards` 拿完整样式规范**：下述双重条件**同时满足**才是汇总行，禁止仅凭"有 AVERAGE"就判定：
- **语义信号**（二选一）：用户 prompt 含"合计/汇总/总计/统计/各科平均分/最下面加一行算…/底部总计"等意图词；或上下文明确是"表尾追加一行做聚合"
- **结构信号**：新行全行都在做聚合（含 `=SUM/AVERAGE/COUNT/MAX/MIN/SUBTOTAL(...)`，支持 IFERROR 包裹），**不是**单个 cell 算个参考值或每行都算的派生列

满足上述时，**不要在本文里猜样式**，直接去读 `lark-sheets-visual-standards` 的「场景一 → 1A. 添加汇总行 / 表头行」章节，按那里的样式要点配齐 `font.bold / horizontal_alignment / background_color / border_styles`。

反例（**不是**汇总行，禁止自动加粗）：
- 用户说"在 H5 帮我算个 AVERAGE 参考"→ 单 cell 计算
- 每行都有 `=AVERAGE(本行区间)` 的派生列 → 属数据列
- 用户明确说"不要加粗/样式和数据行保持一致"→ 遵循用户意图

**正确做法**（二选一）：

- **做法 A（推荐）**：按上方「内容与样式分离写入」两步法——先用模板单元格 + `--copy-to-range` 铺**完整样式**（`cell_styles` + `border_styles` 都要，不能只铺 border，否则新行字体 / 对齐 / 背景色全裸奔），再单独 `+cells-set` 写 value / formula。汇总行的 `cell_styles` 要点（bold / 背景色 / 上边框）见 `lark-sheets-visual-standards` 的「场景一 → 1A. 添加汇总行 / 表头行」。
- **做法 B**：一次写入，但每个 cell（含空白格）都显式带 `cell_styles` + `border_styles`，**不能用 `{}`**。

**判断是不是"新行"**：写入 range 超出 `+csv-get` 返回的 `current_region` 右 / 下边界（如 `current_region=A1:H10`、写 `A11:H11`）即新行，必须按上述做法补边框。

## 富文本单元格：超链接 / @人 / @文档（`rich_text`）

带显示文本的超链接、@人、@文档这类富内容**必须**走 `+cells-set` 的 `rich_text` 字段（`cells[].rich_text` 数组，每段一个对象、带 `type`），**不能**直接传普通字符串——纯字符串只会被当作纯文本存进单元格。完整字段跑 `lark-cli sheets +cells-set --print-schema --flag-name cells`，常用段类型：

- **超链接（带显示文本）**：`{"type":"link","text":"飞书","link":"https://www.feishu.cn"}`。纯 URL 不需要 `rich_text`，直接写普通字符串即可。
- **@人**：`{"type":"mention","mention_token":"<userId>","notify":false}`。**仅支持同租户用户，单次写入最多 50 人。** `notify` **默认 `true`**（会给被 @ 的人发通知），不想发务必显式传 `false`。
- **@文档**：同样 `"type":"mention"`，`mention_token` 传文档 token（如 `shtXXX`）。

`mention_type`（类型编号）等可选字段以 `--print-schema` 输出为准。

> ⚠️ `rich_text` 一旦设置会**忽略**同一 cell 的 `value`；它与 `formula` / `multiple_values` 三者只能选其一作为内容字段（可叠加 `cell_styles` / `note` 等）。

## Dropdown 选项 + 配色（`+dropdown-set` / `+dropdown-update`）

### 选项怎么来：`--options` 与 `--source-range` 二选一

| flag | 选项来源 | 适用场景 |
|---|---|---|
| `--options '["a","b","c"]'` | 写在命令里的固定列表 | 选项集是常量、不需要事后维护 |
| `--source-range ''\''Sheet1'\''!T1:T3'` | 已有单元格里的值 | 选项要跟数据动态同步；想维护一张「枚举值」列后多处引用 |

两个 flag **必须传一个、且只能传一个**——同时传或都不传，CLI 会立刻报错。`--source-range` 用 A1 + sheet 前缀写法（如 `'Sheet1'!T1:T3`，sheet 名按 A1 标准单引号包裹），可以指同 sheet 也可以指其它 sheet（如 `'Refs'!A1:A10`）。

### 配色：默认即上色，三种意图三条线

下拉**默认带胶囊高亮**——什么 flag 都不传时，所有选项按内置 10 色色板循环上色，跟 UI 手动配下拉的默认行为对齐。三种意图：

| 想要的效果 | 怎么传 |
|---|---|
| 默认色板循环上色 | 都不传 `--highlight` / `--colors` |
| 按选项指定具体颜色 | 只传 `--colors '["#hex",...]'`（不需要再传 `--highlight`） |
| 纯白下拉、不要高亮 | 传 `--highlight=false`（注意 `=false` 不能省，单写 `--highlight` 在 cobra 里等价于 true） |

`--colors` 长度**可以短于**选项数（list 模式短于 `--options` 长度，listFromRange 模式短于 `--source-range` 的单元格数），未指定的选项按内置色板循环补色；但**不能长于**——CLI 在 Validate 阶段就会拦截，错误形如 `--colors length (4) must not exceed dropdown source size (3)`。

当 `--highlight=false` 显式关闭高亮时，`--colors` 即使传了也会被忽略（语义自相矛盾，但不报错）。

### 最小用例

**`--options` 模式 — 默认色板（最常见）**：

```
lark-cli sheets +dropdown-set \
  --url https://... --sheet-id <id> \
  --range A2:A100 \
  --options '["待开始","进行中","已完成","已取消"]'
```

**`--options` 模式 — 指定颜色**（4 个选项配 3 个颜色，第 4 个按色板补）：

```
lark-cli sheets +dropdown-set \
  --url https://... --sheet-id <id> \
  --range A2:A100 \
  --options '["待开始","进行中","已完成","已取消"]' \
  --colors '["#bff7d9","#FFE699","#bacefd"]'
```

**`--source-range` 模式**（先在 `'Sheet1'!T1:T3` 维护「男/女/保密」三行，再让 `B2:B21` 引用它）：

```
lark-cli sheets +dropdown-set \
  --url https://... --sheet-id <id> \
  --range B2:B21 \
  --source-range ''\''Sheet1'\''!T1:T3' \
  --colors '["#cce8ff","#ffd6e7","#e6e6e6"]'
```

**纯白下拉**（明确告诉用户"不要彩色"时才用）：

```
lark-cli sheets +dropdown-set \
  --url https://... --sheet-id <id> \
  --range A2:A100 \
  --options '["低","中","高"]' \
  --highlight=false
```

> ⚠️ **`--source-range` 必须带 sheet 前缀**（即使跟 `--range` 同 sheet）。注意一个坑：回读这种 listFromRange 下拉单元格时，`data_validation.range` 看起来不带 sheet 前缀（形如 `$T$1:$T$3`），如果要把读出来的 range 反过来写回 `--source-range`，**必须自己重新补上 sheet 前缀**，否则会被拒。
>
> ⚠️ **sheet 前缀里的表名一律「裸写」，不要加引号**——这条对所有带 sheet 前缀的 range 入参通用（`--source-range`、`+cells-batch-set-style` / `+cells-batch-clear` / `+dropdown-update` 的 `--ranges` 等）。即使表名含点或空格（如 `2025.9`、`一月份 `），也直接写 `2025.9!A1`；**不要**按电子表格习惯写成 `'2025.9'!A1`——引号会被当成表名的一部分，导致 `sheet "'2025.9'" not found`。

`+dropdown-update`（多 range 批量更新）的所有 flag 语义与 `+dropdown-set` 完全一致；只是目标 `--ranges` 由单值变成 JSON 数组（每项带 sheet 前缀），同一份选项 + 配色应用到所有 range。

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+cells-set` | write | 单元格 |
| `+cells-set-style` | write | 单元格 |
| `+cells-set-image` | write | 单元格 |
| `+dropdown-set` | write | 对象 |
| `+csv-put` | write | 单元格 |
| `+table-put` | write | 单元格 |

## Flags

### `+cells-set`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | required | 写入区域（A1 格式） |
| `--cells` | string + File + Stdin（复合 JSON） | required | JSON：2D 数组 `[[{cell},...],...]`，维度与 `--range` 完全一致；每个 cell 可含 `value` / `formula` / `cell_styles` / `note` / `rich_text`（含 `type="embed-image"` 单元格嵌图）等，完整字段跑 `--print-schema` |
| `--allow-overwrite` | bool | optional | 允许覆盖非空 cell（默认 true）；设为 false 时遇非空 cell 报错 |
| `--max-cells` | int | optional | 防爆，默认 50000（隐藏 flag：不在 `--help` 列出，但可正常传入） |
| `--copy-to-range` | string | optional | 复制范围（A1 表示法）：把 --range 中 --cells 写入的内容（值/公式/样式，取决于实际传入字段）复制到该区域，公式引用自动平移（如 C2=B2 → C3=B3）。适合先写一行/一块模板再扩展填充整列/整区域（如 --range A1:G1 写模板、--copy-to-range A1:G100 填充 100 行）。支持整行 3:6、整列 C:E、到列尾 D3:D、到行尾 D3:3；支持英文逗号分隔多个目标区域，如 C1:D2,E5:F6 |

### `+cells-set-style`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | required | 目标范围（A1 格式，如 `A1:B2`） |
| `--background-color` | string | optional | 背景颜色（十六进制，如 `#ffffff`） |
| `--font-color` | string | optional | 字体颜色（十六进制，如 `#000000`） |
| `--font-size` | float64 | optional | 字体大小（px，例：10、12、14） |
| `--font-style` | string | optional | 字体样式（可选值：`normal` / `italic`） |
| `--font-weight` | string | optional | 字重（可选值：`normal` / `bold`） |
| `--font-line` | string | optional | 字体线条样式（可选值：`none` / `underline` / `line-through`） |
| `--horizontal-alignment` | string | optional | 水平对齐（可选值：`left` / `center` / `right`） |
| `--vertical-alignment` | string | optional | 垂直对齐（可选值：`top` / `middle` / `bottom`） |
| `--word-wrap` | string | optional | 换行策略（可选值：`overflow` / `auto-wrap` / `word-clip`） |
| `--number-format` | string | optional | 数字格式（例：文本 `@`、数字 `0.00`、货币 `$#,##0.00`、日期 `mm/dd/yyyy`） |
| `--border-styles` | string + File + Stdin（复合 JSON） | optional | 边框配置 JSON：`{ top: {style,color,weight}, bottom: ..., left: ..., right: ... }`；4 方向结构相同 |

### `+cells-set-image`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | required | 目标单元格（A1 格式，必须单 cell，如 `A1`；起止 cell 须相同） |
| `--image` | string | required | 本地图片路径（支持 PNG / JPEG / JPG / GIF / BMP / JFIF / EXIF / TIFF / BPG / HEIC） |
| `--name` | string | optional | 图片文件名（含扩展名）；省略时取 `--image` 的 basename |

### `+dropdown-set`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | required | 目标范围（A1 格式，如 `A2:A100`） |
| `--options` | string + File + Stdin（复合 JSON） | xor | 下拉选项 JSON 数组，例如 `["opt1","opt2"]`。服务端不限制选项数量，也不限制单个选项长度；含逗号的选项可以接受（写入时会自动转义）。大量选项建议改用 `--source-range`。 |
| `--colors` | string + File + Stdin（简单 JSON） | optional | 下拉胶囊背景色，RGB hex 数组（如 `["#1FB6C1","#F006C2"]`）。长度可短不可长——超长 Validate 拦截（`--colors length (N) must not exceed dropdown source size (M)`），未指定项按内置 10 色色板循环补色。**单独传即生效**；`--highlight=false` 时被忽略。 |
| `--multiple` | bool | optional | 启用多选；默认 `false` |
| `--highlight` | bool | optional | 下拉胶囊背景色高亮开关。**不传 = 开**（按内置 10 色色板循环上色）；`--highlight=false` 关闭得到纯白下拉。配色用 `--colors` 覆盖。 |
| `--source-range` | string | xor | listFromRange 模式的下拉源 range，A1 表示法 + sheet 前缀（如 `'Sheet1'!T1:T3`）。映射到 server `data_validation.range`，搭配 server `data_validation.type='listFromRange'` 自动生效。跟 `--options` 二选一：传 `--options` 走 inline 列表（type=list），传本 flag 走 range 引用（type=listFromRange）。`--colors` 长度规则不变（≤ 源 range 单元格数），`--highlight` / `--multiple` 行为相同。当 `--highlight` 开启且 source 覆盖单元格数超过 2000 时，服务端会将该下拉判为 option-error（这是不支持的组合）；CLI 会向 stderr 输出 warning。如需取消，传 `--highlight=false`。 |

### `+csv-put`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--start-cell` | string | required | 目标区域起点 A1（如 `A1`、`B5`，不带 sheet 前缀；用 `--sheet-id` / `--sheet-name` 指定 sheet）；必须是单个单元格，不接受范围写法；终点按 CSV 实际行列数自动推断 |
| `--csv` | string + File + Stdin（非 JSON 文本） | required | RFC 4180 CSV 文本；可写值或公式（以 = 开头的单元格按公式计算）；不带样式 / 批注 / 图片，需要这些用 +cells-set。 |
| `--allow-overwrite` | bool | optional | 允许覆盖（默认 true）；设为 false 时若目标非空报错 |
| `--range` | string | optional | --start-cell 的别名（与 +csv-get / +cells-set 一致，用 --range 定位）；传区间（如 A1:H17）时自动取其左上角单元格（隐藏 flag：不在 `--help` 列出，但可正常传入） |

### `+table-put`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--sheets` | string + File + Stdin（复合 JSON） | required | Typed 表格协议（pandas-DataFrame-shaped）JSON：顶层 `{"sheets":[...]}`，每个数组项是一张子表 `{name, start_cell?, mode?, header?, allow_overwrite?, columns:["colA","colB",...], data:[[...]], dtypes?:{colA:pandasDtype, ...}, formats?:{colA:numberFormat, ...}}` —— `name` 与外层 `sheets` 数组都不可省。Agents 用 `scripts/sheets_df.py` 的 `df_to_sheet(df, name)` 一行把 DataFrame 转成一项（多子表就 list 拼起来再包 `{"sheets":[...]}`）。`dtypes` 值是 pandas dtype 字符串（`int64`、`float64`、`Int64`、`bool`、`boolean`、`datetime64[ns]`、`object`、...），CLI 端映射成内部 string/number/date/bool —— 省略 `dtypes` 时该列按文本写入（适合原始 CSV-shaped 数据）。`formats[col]` 是 Excel number_format 字符串（如 `#,##0.00`、`0.0%`、`yyyy-mm`）；缺省时 date 列用 `yyyy-mm-dd`，string 列用文本格式 `@`。 |
| `--styles` | string + File + Stdin（复合 JSON） | optional | 类型保真写入后再应用的视觉处理操作 JSON：顶层 `{styles:[...]}`，每项对应一个被写入的子表、含 `name`，并至少给 `cell_styles` / `row_sizes` / `col_sizes` / `cell_merges` 之一。`cell_styles` 用 A1 单元格 range + 扁平样式字段（字段同 +cells-set-style，含 number_format / 颜色 / 对齐 / border_styles）；row/col sizes 用行/列范围 + type/size；merges 用单元格 range + 可选 merge_type。styles 数组的长度/顺序/name 必须与被写入的子表对应（与 --sheets.sheets 一一对应）。完整 cell_styles 字段结构跑 `+table-put --print-schema --flag-name styles`。 |

## Schemas

> 复合 JSON flag 字段速查（只列顶层 + 一层嵌套）。深层结构看下方 `## Examples`，或用 `--print-schema` 读完整 JSON Schema（用法见 SKILL.md「公共 flag 速查」与「Agent 使用提示」）。

### `+cells-set` `--cells`

_【维度】行列数必须与 range 完全一致：'A1:C2'→[[_,_,_],[_,_,_]]（2行×3列），'B5:B7'→[[_],[_],[_]]（3行×1列），'A1'→[[_]]（1×1）_

**二维数组项**（类型 object）：
- `value` (oneOf?) — 静态单元格值（文本、数字、布尔）
- `formula` (string?) — 以 '=' 开头的单元格公式（例如：'=SUM(A1:A10)'）
- `note` (string?) — 单元格批注/备注
- `cell_styles` (object?) — 单元格样式属性，包括字体、颜色、对齐方式和数字格式 { font_color?: string, font_size?: number, font_weight?: enum, font_style?: enum, font_line?: enum, …共 10 项 }
- `border_styles` (object?) — 单元格边框配置，含 top/bottom/left/right 四个方向，每个方向的结构相同（见 top） { top?: object, bottom?: object, left?: object, right?: object }
- `rich_text` (array<object>?) — 富文本内容 each: { type: enum, text: string, style?: object, link?: string, mention_token?: string, …共 17 项 }
- `multiple_values` (array<object>?) — 多值内容，用于支持多选的列表验证单元格 each: { value: oneOf, format?: string }
- `data_validation` (object?) — 数据验证配置 { type: enum, items?: array<string>, range?: string, operator?: enum, values?: array<oneOf>, …共 9 项 }

### `+cells-set-style` `--border-styles`

_单元格边框配置，含 top/bottom/left/right 四个方向，每个方向的结构相同（见 top）_

**顶层字段**：
- `top` (object?) { style?: enum, weight?: enum, color?: string }
- `bottom` (object?) { style?: enum, weight?: enum, color?: string }
- `left` (object?) { style?: enum, weight?: enum, color?: string }
- `right` (object?) { style?: enum, weight?: enum, color?: string }

### `+dropdown-set` `--options`

_列表选项_

**数组项**（类型 string）：
- 标量：string

### `+table-put` `--sheets`

_一个或多个子表的 typed 数据，每个数组元素写入一张子表；支持多 DataFrame → 多子表一次写入_

**数组项**（类型 object）：
- `name` (string) — 目标子表名
- `start_cell` (string?) — 写入起点单元格（A1 记法，如 "B2"），默认 "A1"
- `mode` (enum?) — overwrite（默认）：从 start_cell 起写「表头 + 数据」块；append：把数据追加到子表已有数据下方（默认不重复表头） [overwrite / append]
- `header` (boolean?) — 是否写一行列名表头
- `allow_overwrite` (boolean?) — 为 false 时，若写入会落在非空单元格则拒写以保护原数据（返回 partial_success）
- `columns` (array<string>) — 列名字符串数组，顺序与 `data` 中每行取值一一对应
- `data` (array<array<string|number|boolean|null>>) — 数据行；每行是一个数组，长度必须等于 `columns` 数
- `dtypes` (object?) — 可选
- `formats` (object?) — 可选

### `+table-put` `--styles`


**数组项**（类型 object）：
- `cell_merges` (array<object>?) — 单元格合并操作数组；range 使用 A1 单元格范围，merge_type 默认 all each: { merge_type?: enum, range: string }
- `cell_styles` (array<object>?) — 单元格样式操作数组；每项用 A1 单元格 range 指定范围，字段名与 +cells-set-style 对齐 each: { background_color?: string, border_styles?: object, font_color?: string, font_line?: enum, font_size?: number, …共 12 项 }
- `col_sizes` (array<object>?) — 列宽操作数组；range 使用列范围如 A:C，type 为 pixel/standard，pixel 需要 size each: { range: string, size?: number, type: enum }
- `name` (string) — 子表名
- `row_sizes` (array<object>?) — 行高操作数组；range 使用行范围如 1:3，type 为 pixel/standard/auto，pixel 需要 size each: { range: string, size?: number, type: enum }

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。

### `+cells-set` 的拆分与转介绍

"工具选择"段已讲清纯值（`+csv-put`）vs 富写入（`+cells-set`）。下表补 CLI 侧的 `+cells-set` **兄弟拆分**，以及不属于本 reference 的**跨 reference 转介绍**——避免 agent 用 `+cells-set` 硬扛所有写入场景。

| 写入场景 | 用这个 | 不要用 |
|---------|--------|--------|
| 只改**已有 cell 的样式**，不动 value/formula | `+cells-set-style` | `+cells-set`（会触发不必要的值写入） |
| 把**单张图片嵌入**到某个 cell | `+cells-set-image` | `+cells-set`（参数更繁琐） |
| **插行/列 + 写入** 这种多步组合，且要原子 | `+batch-update`（见 lark-sheets-batch-update） | 多次独立 `+cells-set`（非原子；插入会扰动后续 range） |
| 在**多个不连续 range** 上应用同一组样式 | `+cells-batch-set-style`（见 lark-sheets-batch-update） | 多次 `+cells-set-style`（非原子） |

### `+cells-set`

示例：

```bash
# 纯值（数组形态）；默认即覆盖非空 cell，无需显式传 --allow-overwrite
lark-cli sheets +cells-set --url "https://example.feishu.cn/sheets/shtXXX" \
  --sheet-name "Sheet1" --range "A1:B2" \
  --cells '[[{"value":"name"},{"value":"score"}],[{"value":"alice"},{"value":95}]]'

# 富 cell（公式 + 样式，cells 是二维矩阵每元素一个 cell schema）
lark-cli sheets +cells-set --spreadsheet-token shtXXX --sheet-id "$SID" \
  --range "C2:C10" --cells @rich-cells.json
```

`--cells` 富格式见 `## Schemas` 段（cells 元素含 value / formula / cell_styles / border_styles / data_validation / multiple_values / note / rich_text）；值 / 公式 / 样式 / 批注 / 嵌入图片可同一次写入混合提交。

> 中间想跳过的 cell 用空对象 `{}` 占位（底层语义为"保留原值不变"），`--cells` 维度仍须与 `--range` 完全一致。例：`--range A1:A5 --cells '[[{"value":1}],[{}],[{}],[{}],[{"value":5}]]'` 只写 A1 和 A5。
>
> 跨多个不连续区域散点写入（如 `D2` + `F7` + `J15`）不属于 `+cells-set` 的能力范围——请用 `+batch-update` 把多次 `+cells-set` 打包成单次原子请求。

### `+cells-set-style`

只改样式，不动 value / formula。10 个 cell_styles 字段拍平为独立 flag，边框走 `--border-styles` JSON。

```bash
# 加粗 + 黄底
lark-cli sheets +cells-set-style --url "..." --sheet-name "Sheet1" \
  --range "A1:B2" --font-weight bold --background-color "#FFFF00"

# 配套边框
lark-cli sheets +cells-set-style --url "..." --sheet-id "$SID" \
  --range "A1:D10" --font-size 12 --horizontal-alignment center \
  --border-styles '{"top":{"style":"solid","color":"#000","weight":"thin"},"bottom":{"style":"solid","color":"#000","weight":"thin"}}'
```

### `+cells-set-image`

把单张图片嵌入 cell（必须单 cell 范围）：

```bash
lark-cli sheets +cells-set-image --url "..." --sheet-name "Sheet1" \
  --range "A1" --image ./logo.png
```

### `+csv-put`

示例：

```bash
# 内联 CSV
lark-cli sheets +csv-put --url "https://example.feishu.cn/sheets/shtXXX" \
  --sheet-name "Sheet1" --start-cell "A1" \
  --csv $'name,score\nalice,95\nbob,87'

# 从文件
lark-cli sheets +csv-put --spreadsheet-token shtXXX --sheet-id "$SID" \
  --start-cell "A1" --csv @data.csv
```

> `+csv-put` 比 `+cells-set` 短得多——批量灌值或公式时优先用它。需要样式/批注/图片才换 `+cells-set`。
>
> ✅ `=` 开头的单元格会被当作公式计算（不是字面量文本）：
>
> ```bash
> lark-cli sheets +csv-put --url "..." --sheet-name "Sheet1" \
>   --start-cell "A1" \
>   --csv $'name,score\nalice,=SUM(B2:B10)'
> # ↑ B2 写入公式 =SUM(B2:B10)，读回 formula 保留、value 为计算结果。
> # 反过来：无法用 +csv-put 写「= 开头的字面量文本」（会被当公式）；样式/批注/图片仍用 +cells-set。
> ```
>
> ⚠️ **公式内部含逗号 / 引号必须 RFC 4180 转义**：CSV 用逗号分隔字段，公式里的逗号（如 `COUNTIF(D5:D22,"及格")` 的参数分隔逗号）会被解析器当成字段分隔符，把一格拆成多格、整块二维结构压扁错位。规则：**含逗号的字段整格用双引号包裹，字段内部的引号再翻倍**：
>
> ```bash
> # 从 G4 写一个 2 列 3 行的统计块；=COUNTIF 含逗号 + 内部引号，必须转义
> lark-cli sheets +csv-put --url "..." --sheet-name "Sheet1" \
>   --start-cell "G4" \
>   --csv $'统计项,结果\n成绩总和,=SUM(C5:C22)\n及格人数,"=COUNTIF(D5:D22,""及格"")"'
> # ↑ "=COUNTIF(D5:D22,""及格"")"：外层双引号包裹整格，内部 "及格" 的引号翻倍成 ""及格""。
> # 裸写 =COUNTIF(D5:D22,"及格") 会被 CSV 按逗号拆成两格、写入区域从 G4:H6 错位成 G4:K4。
> ```
>
> 💡 **含逗号 / 引号 / 换行的公式优先用 `+cells-set`（JSON 二维数组）写入**——`cells[r][c].formula` 字段直接放公式串，没有 CSV 转义负担，从根上杜绝拆列错位。`+table-put` 的 typed 协议只有 `columns / data / dtypes / formats` 四件套、没有 `formula` 字段，公式写入用 `+cells-set` 或 `+csv-put`。准备给 `+csv-put` 的公式加逗号时，先考虑换 `+cells-set`：
>
> ```bash
> # 同样的统计块，结构化写入无需任何转义
> lark-cli sheets +cells-set --url "..." --sheet-name "Sheet1" --range "G4:H6" \
>   --cells '[[{"value":"统计项"},{"value":"结果"}],[{"value":"成绩总和"},{"formula":"=SUM(C5:C22)"}],[{"value":"及格人数"},{"formula":"=COUNTIF(D5:D22,\"及格\")"}]]'
> ```

> **定位 + 写入边界（关键，避免误覆盖）**：
> - 定位用 `--start-cell`（锚点 = 左上角单元格）；也接受 `--range` 别名（与 `+csv-get` / `+cells-set` 一致，传区间会自动取左上角）。
> - ⚠️ `--start-cell` / `--range` **只定左上角、不限制写入大小**：CSV 从锚点按自身行列数 auto-expand 铺开。给一个"小 range"**不会**截断数据——超出部分照写，且默认覆盖。这与 `+cells-set --range`（精确矩形、`--cells` 必须与 range 同维）语义相反，别把那套心智搬过来。
> - dry-run 与成功响应都回显 `writes_range`（实际落区，如 `B2:D4`）：**写前先 `--dry-run` 看一眼落区**，确认不会盖到相邻数据。
> - 要保护非空 cell：`--allow-overwrite=false`（落区内出现非空 cell 即报错）。

### `+table-put`（DataFrame → 飞书，类型保真写入）

把结构化数据（DataFrame、list of dict、Counter）类型保真写入**已有**表（写入语义同 `+cells-set`）。协议形状**对齐 pandas `to_json(orient="split")`**：`columns:[列名]` + `data:[[行...]]`，可选 `dtypes:{列名:pandas_dtype}` 决定每列类型（number 保精度、date 落真日期），可选 `formats:{列名:number_format}` 覆盖显示格式（千分位 / 百分比 / 自定义日期）。dtypes 缺失时整张表按 string 写入（带 `@` 文本格式，邮编 / 订单号等含前导零的 id 保真）。

只写入**已有**表（`--url` / `--spreadsheet-token` 二选一必填），不新建工作簿——**要新建表格直接用 `+workbook-create --sheets`**（同协议、一步建表 + 类型保真写入，详见 workbook reference）。读回用镜像命令 `+table-get`（见 read-data reference），输出与 `--sheets` 同构、可 round-trip。

```bash
# sheet 按 name 匹配、缺则新建；多 DataFrame 经 stdin 一次写多 sheet
python export.py | lark-cli sheets +table-put --url "<表URL>" --sheets -
# 某 sheet 带 "mode":"append" 追加到已有数据末尾、默认不重复表头
lark-cli sheets +table-put --spreadsheet-token "<token>" --sheets @payload.json
```

每个 sheet 还可带 `"allow_overwrite": false`（遇非空拒写、保护原数据）、`"header": false`（只写数据不写表头）。完整字段跑 `+table-put --print-schema --flag-name sheets`。

#### DataFrame → 协议（用 `df_to_sheet` helper）

pandas 的 `df.to_json(orient="split", date_format="iso")` 一步完成所有清洗（NaN→null、Timestamp→ISO 字符串、numpy 标量→原生数字），把 dtypes 拼上即可。本 skill 把这段 5 行 helper 打包成可 import 的 [`scripts/sheets_df.py`](../scripts/sheets_df.py)（含 `df_to_sheet` 和 `sheet_to_df`，写入 / 读回成对）：

```python
from sheets_df import df_to_sheet

# 单 sheet（显式 format 覆盖默认显示）
payload = {"sheets": [df_to_sheet(df, "销售", {"营收": "#,##0.00", "毛利率": "0.0%"})]}

# 多 sheet——helper 让每个 sheet 一行，不再重复 boilerplate
payload = {"sheets": [df_to_sheet(df1, "销售"),
                      df_to_sheet(df2, "成本"),
                      df_to_sheet(df3, "利润")]}
```

> **CSV-shaped 全文本数据**（不需要类型保真、含前导零的 id 也要保留）省掉 dtypes 即可，inline 一行写完，不必走 helper（注意保留 `date_format="iso"`，否则 datetime 列会被序列化成 epoch 毫秒数字，CLI 拒绝）：
> ```python
> payload = {"sheets": [{"name": "原始",
>                        **json.loads(df.to_json(orient="split", date_format="iso"))}]}
> ```
> **别把 `to_json + json.loads` 换成 `df.to_dict(orient="split")`**：会留 `numpy.int64` 让 `json.dumps` 后续报 "not serializable"——这一步是清洗的关键。

不用 pandas 也行——typed 协议就是纯 JSON。手写场景：

```python
# Counter / dict / 手拼数据：直接写 columns + data，按需加 dtypes/formats
payload = {"sheets": [{
    "name": "渠道",
    "columns": ["channel", "count", "rate"],
    "data": [["app", 1240, 0.62], ["web", 760, 0.38]],
    "dtypes": {"count": "int64", "rate": "float64"},
    "formats": {"rate": "0.0%"},
}]}
```

> **dtype 速查**：`int64`/`float64`（数值）、`Int64`（含空值的整数，nullable）、`bool`/`boolean`、`datetime64[ns]`（date，默认 `yyyy-mm-dd`）、`object`（string）。pandas dtype 字符串原样塞进 dtypes 即可，CLI 端按前缀匹配（`int*`/`uint*`/`Int*`/`float*` → number 等）。未识别 dtype 兜底为 string。

#### `--styles`（写入时同时套样式）

`--styles` 在 typed 写入后顺带应用视觉处理，省掉一次 `+cells-set-style` 往返。协议与 `+workbook-create --styles` **完全同构**（详见 workbook reference）：顶层 `{styles:[...]}`，数组每项对应一个被写入的子表、含 `name`，并按能力拆成四类可选数组——`cell_styles`（A1 单元格 range + 扁平样式字段，含 `number_format` / 颜色 / 对齐 / `border_styles`，随内容在同一次写入里一并应用）、`cell_merges`、`row_sizes`、`col_sizes`。styles 数组的长度 / 顺序 / name 必须与被写入的子表对应（与 `--sheets.sheets` 一一对应）。

```bash
lark-cli sheets +table-put --url "<表URL>" \
  --sheets '{"sheets":[{"name":"明细","columns":["日期","金额"],"dtypes":{"日期":"datetime64[ns]","金额":"float64"},"formats":{"金额":"#,##0.00"},"data":[["2024-01-15",1234.5]]}]}' \
  --styles '{"styles":[{"name":"明细",
    "cell_styles":[{"range":"A1:B1","font_weight":"bold","background_color":"#f5f5f5","horizontal_alignment":"center"}],
    "cell_merges":[{"range":"A1:B1"}],
    "col_sizes":[{"range":"A:B","type":"pixel","size":120}]}]}'
```

完整字段跑 `+table-put --print-schema --flag-name styles`。

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`+cells-set` 的 `--cells` 必须能解析为 JSON 二维矩阵且行列数与 `--range` 完全一致；`+cells-set-style` 的样式 flag 至少一个非空（或带 `--border-styles`）；`+cells-set-image` 的 `--range` 必须是单 cell（起止 cell 相同）；`+csv-put` 的 `--csv` 必须能按 RFC 4180 解析；`+table-put` 给了 `--styles` 则按子表名 / 顺序 / 数量与 `--sheets.sheets` 对齐校验；防爆参数上限校验。
- `DryRun`：输出目标 range + 推断尺寸 + 是否覆盖非空 cell 警告，零网络副作用。
- `Execute`：写后不自动回读；如需确认，自行调用 `+cells-get --range <写入区域> --include value,formula` 抽样核对。
