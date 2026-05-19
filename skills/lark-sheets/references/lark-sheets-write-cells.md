# Lark Sheet Write Cells

## 写入边界 + 回读校验（编辑类任务必做）

1. **明确写入边界**：写入前必须能回答"目标 range 的起止行列号是多少？是否落在用户授权范围内？"。除用户明示要修改的区域外，禁止扩张到原数据列以外或新建 Sheet。
2. **完整性断言**：批量写入前先把"预期写入条数"硬编码到代码里（如要填 106 条翻译 → `expected = 106`），写完后回读断言 `actual == expected`。少于预期就继续写，禁止交付半成品。
3. **回读抽样校验**：写完关键值 / 公式后，用 `+csv-get` 或 `+cells-get` 重新读取写入区域，至少抽样 3-5 个代表性单元格（首 / 中 / 末），核对值与预期一致（与 本地脚本 计算的预期值对照）。公式特定的"先验证模板再 copy_to_range / 修完再读回"细则见下方相关章节。

## 新增列 / 新增行的样式继承（防止视觉风格不一致）

新增列 / 新增行**必须**先用 `+cells-get` 读相邻原列 / 原行的完整样式作为模板，**禁止**只传 `value` 期望默认样式与原表一致——飞书新单元格默认对齐通常是 `H:right, V:bottom`，与多数原表的 `H:center, V:middle` 不一致。

**完整继承清单**（写新列 / 新行时 cells 数组必须同时携带）：

1. `cell_styles.font_size` / `cell_styles.font_weight` / `cell_styles.font_color` / `cell_styles.font_style`（字号 / 粗细 / 颜色 / 斜体等）
2. `cell_styles.horizontal_alignment` / `cell_styles.vertical_alignment`（H-Align / V-Align）—— 漏继承会导致新列对齐与原列不一致（高频）
3. `cell_styles.number_format`（小数位 / 千分位 / 百分比 / 日期格式）—— 漏继承会导致同列数值格式混乱
4. `cell_styles.background_color`（背景色）
5. `border_styles`（四边框）
6. **`merged_cells`（合并范围）**——续写场景必查（高频致命错误）：用 `+sheet-info --info_type=merged_cells_infos` 读原数据区域的合并信息。**原行有跨列合并**（如标题行 `A1:G1` 合并）时，新行**必须**用 `+cells-{merge|unmerge}` 工具复制相同合并模式到新行（如续写第 3 个周报块的标题行 `A23:G23` 必须合并）。仅传 cells 数组的 5 类样式不够——合并范围要单独靠 `+cells-{merge|unmerge}` 工具落地（典型反例：续写多周记录表时，新增周次的标题行未合并，视觉上与原前几周风格不一致）

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

写入。为一块单元格区域设置值、公式、批注/备注和/或格式。也支持通过 `rich_text` 中 `type: "embed-image"` 在单元格内嵌入图片（单元格图片）。关键：数组维度必须严格匹配——`cells` 二维数组必须与 `range` 的行列维度完全一致，range 是闭区间，否则会触发 `InvalidCellRangeError`。计算示例：区域 `A1:D3` = 3 行 × 4 列 = `[[r1c1,r1c2,r1c3,r1c4],[r2c1,r2c2,r2c3,r2c4],[r3c1,r3c2,r3c3,r3c4]]`；区域 `A41:N48` = 8 行 × 14 列 = 8 个数组且每个数组 14 个单元；单个单元格 `A1` = `[[cell]]`；单列区域 `B5:B7` = `[[cell1],[cell2],[cell3]]`。空单元请使用 `{}`。**如果填写的区域存在大量重复内容，务必优先使用 `copy_to_range` 字段复制，可大幅减少 `cells` 长度。**

> **单元格图片 vs 浮动图片**：
> - **单元格图片**（本工具）：图片嵌入在单元格内部，属于单元格内容，随单元格移动。通过 `rich_text` 中 `type: "embed-image"` 写入。
> - **浮动图片**：图片悬浮在单元格上方，可自由定位和调整大小，不属于单元格内容。→ 使用 lark_sheet_float_image Skill。

高频模式（**必须遵守，禁止逐行写入替代**）：

- 整列公式：先在 `H2` 写一个公式，再用 `copy_to_range: "H2:H100"` 或 `copy_to_range: "H:H"` 向下填充。**禁止对每一行单独调用 `+cells-set` 写入相同结构的公式**
- 整列格式：先在 `J1` 写一个带样式的模板单元格，再用 `copy_to_range: "J:J"`
- 首行样式：先在 `A1` 写一个模板单元格，再用 `copy_to_range: "1:1"`
- 用户说”这列 / 整列 / 这行 / 首行 / 向下复制”时，**必须**使用模板单元格 + `copy_to_range`
- 多区域写入相同格式/公式结构时，优先写一个模板，再用 `copy_to_range` 复制到所有目标区域

⚠️ **逐行写入公式是最常见的致命错误**：对每一行单独调用 `+cells-set` 写入公式（如调用 26 次），会快速耗尽轮次上限导致操作不完整。正确做法是 1 次模板写入 + 1 次 `copy_to_range` = 2 次调用完成。

💡 **写入公式前先按迁移规则改写**：如果公式来自 Excel 或包含数组场景，先读取并遵循 `lark-sheets-formula-translation` skill 的规则完成改写，再把最终公式写入 `formula` 字段。

💡 **内容与样式分离写入（推荐）**：当需要同时写入内容和样式时，`cells` 中每个单元格都带上 `cell_styles` / `border_styles` 会导致入参非常冗长。由于同一区域的样式通常高度重复（如整列统一背景色、统一边框），推荐拆成两步：
1. **先写内容**：`+cells-set` 只传 `value` / `formula`，不带样式，`cells` 入参精简
2. **再批量刷样式**：对区域中的一个单元格写入目标样式作为模板，再用 `copy_to_range` 将样式扩展到整列 / 整行 / 整个区域（`copy_to_range` 会复制值、公式和样式，所以模板单元格应已包含正确的值）

示例：要对 A2:A100 写入数据并统一设置蓝色背景 + 边框：
```
Step 1: `+cells-set` — range="A2:A100", cells 只含 value（无样式，入参短）
Step 2: `+cells-set` — range="A2", cells 含 value + cell_styles + border_styles（单个模板）, copy_to_range="A2:A100"
```
这比在 99 个单元格中都重复写样式 JSON 高效得多。

💡 **大批量数据分批写入（推荐）**：当需要写入大量行（如几十行以上）时，不要试图在一次调用中生成全部 `cells` 数据——`cells` 数组过大会导致模型生成内容过长而超时。应将数据拆分为多批，每批 20-50 行，分多次调用 `+cells-set` 逐批生成并写入（如先写 `A2:D21`，再写 `A22:D41`，依此类推）。每次调用只需生成当前批次的数据，控制单次生成量，避免超时。

注意：

- 不要把 `cells` 写成字符串化 JSON
- 如果目标区域中已有值、公式或样式需要被覆盖，显式设置 `allow_overwrite=true`
- 若目标区域涉及合并单元格，不要向合并区域中的非左上角单元格写入数据；如需写入，应改写合并区域左上角单元格，或先调整/取消合并区域
- **构造 `range` 时行号必须基于逻辑行号**：如果之前通过 `+csv-get` 读取了数据，CSV 中被双引号包裹的多行字段（如 `"2026年3月2日\n星期一"`）是**一个单元格**，不是两行。写入时的行号必须按逻辑记录计算，不能按物理换行符计数，否则 `range` 会整体偏移导致写入到错误位置

⚠️ **"样式与原表一致"必须包含 `border_styles`（高频致命错误）**：当用户说"样式和原表一致"、"保持原表格式"、"边框继承"等要求时，cells 里的 `cell_styles` **不能只传 `font_size` / `horizontal_alignment` / `vertical_alignment`**——这几项只覆盖字体和对齐，**不包含边框**。边框必须用独立的 `border_styles` 字段传（或在源 cell 用 `+cells-get` 读出来再原样复制）。
- **反模式**：`cells=[[{cell_styles:{font_size:16, horizontal_alignment:"center", vertical_alignment:"middle"}}]]`（字体+对齐都有，但**新 cell 仍然没边框**，视觉上与原表断裂）
- **正确做法**：`cell_styles` + `border_styles` 一起传，`border_styles` 覆盖 top/bottom/left/right 四条边（或至少 data 区该加的几条），确保视觉连续
- 特别是**新列/新行**场景，新 cell 底子里本来就没边框，如果不显式传 `border_styles`，copy_to_range 复制的模板也没边框 → 整列/整行无边框

⚠️ **公式写入必须自己校验结果（后端不会报语法错）**：`+cells-set` 写公式时，即便公式有括号不配对（如 `=IFERROR(VALUE(REGEXEXTRACT(D5, "\d+"))), 0)` 比 IFERROR 多一个 `)`）或函数名拼错（`=UNIQUE(...)` 飞书不支持），**后端工具也会返回 `updated_cells_count=N, rc=0` 的"成功"**——错误会静默写进单元格显示为 `#VALUE!` / `#NAME?` / `#REF!`。因此：
1. **写完立即读回**：`+cells-set` 后紧跟 `+csv-get`（或 `+cells-get`）读目标范围前几行，检查是否出现 `#VALUE!` / `#NAME?` / `#REF!` / `#N/A` / `#DIV/0!` / `#NUM!`
2. **看到 `#` 开头的错误值**立即修公式：`#NAME?` 多半是函数名拼错或飞书不支持（UNIQUE/DISTINCT 等）；`#VALUE!` 多半是类型不匹配或括号错位；`#REF!` 是引用错误；`~CIRCULAR~REF~` 是循环引用（公式引用了自身或会闭环）
3. **`copy_to_range` 扩展前先验证模板**：模板单元格公式自己都算错，`copy_to_range` 复制到 100 行就是 100 个错误
4. **飞书不支持的函数**：`UNIQUE` / `DISTINCT` / `FILTER`（部分）—— 对应"去重"场景改用透视表（`+pivot-{create|update|delete}`，值字段聚合方式选 count）
5. **循环引用预检（高频致命错误）**：写聚合公式（SUM / AVERAGE / COUNT 等）前必须明确**引用范围不包含目标单元格自身或其传递依赖**。典型反例：在 C3 写 `=SUMIF(B:B,LEFT(B3,9)&"*",C:C)`，B 列匹配 B3 前 9 位时 C3 自己也命中，导致 C3 自引用 → `~CIRCULAR~REF~`。修法：用辅助列 / 显式排除自身（`SUMIFS(C:C, B:B, ..., A:A, "<>"&A3)`）/ 缩小范围避开自己
6. **REGEX 模式覆盖率验证**：公式里的 `REGEXEXTRACT` / `REGEXMATCH` / `REGEXREPLACE` 等正则模式落地前必须用 本地脚本 在源列上跑一遍命中率统计（`df[col].str.contains(pattern).mean()`）；命中率 < 100% 时必须扩展 pattern 或加多分支（IFS / 多个 IFERROR 串联）兜底，**禁止**只覆盖样本前 N 行就交付（典型反例：用 `REGEXEXTRACT(D5,"长(\d+)")` 只匹配带"长"前缀的尺寸文本，对"宽×高"、"×"、"*"等其它分隔符直接漏匹配）
7. **公式范围与用户指令字面对齐**：用户说"对 F 至 L 列求和"就必须写 `SUM(F2:L2)` 或 `F2+G2+H2+I2+J2+K2+L2`，**不能漏列、多列、错列**。写完用 `+cells-get` 拿回 `formula` 字符串，与用户原话逐字对照（参与求和的列名一致 / 起止列号一致 / 运算符一致），不一致就是违规

⚠️ **收到 `formula_errors` 反馈后不要只打补丁（高频致命错误）**：`+cells-set` 返回值里若出现 `formula_errors: [{cell, formula, error_type, detail}]`，说明某些 cell 公式编译失败（`error_type=compile_failed` 通常是函数语法错如 `SPLIT(x)[1]` 飞书不支持；`non_formula` 是 `=` 开头但解析不通过）。此时**禁止只聚焦修报错点的局部语法**（如仅把 `[1]` 换成 `INDEX(..,1)`），必须：

1. **重新审视整条公式的完整性**：被 formula_errors 标出的那一行，公式除了下标语法错，还可能有其他先天缺陷（字符清洗不全、IFERROR 兜底漏条件、引用列写错），修完语法错后立即整体复核
2. **同步对称修复所有相似列**：如果同一任务涉及多列相似处理（如"算 H 列面积"用 D 列尺寸、"算 I 列面积"用 E 列尺寸），**修完一列必须把同样的清洗/兜底逻辑同步到所有相似列**，禁止出现 H 列用 `SUBSTITUTE(长)+SUBSTITUTE(高)+SUBSTITUTE(×)` 而 I 列只用 `SUBSTITUTE(×)` 这种不对称处理——会导致一列编译通过有值、另一列编译通过但 IFERROR 全返回空，用户看到的是"数据为空"而非"公式错"
3. **修完再读回验证**：不只看 `formula_errors` 为空（这只证明编译通过，不证明运行时有值），必须 `+csv-get` 读目标列前 3-5 行，确认**非空源数据对应的目标列有非空计算结果**
4. **核心心智**：`formula_errors` 是"帮你暴露编译错"的工具，不是"修掉它就收工"的通行证。编译通过 + 运行时 IFERROR 兜底空 = 用户视角的"没算出来"

⚠️ **新增行的边框/样式禁止用 `{}` 跳过（高频致命错误）**：`cells` 数组里 `{}` 的语义是"**此单元格不做任何修改、保留原状态**"。这在写入**已有行**时是安全的（原有边框/样式保持不变），但在写入**新行**（比如表尾追加汇总行、扩展行）时是灾难：新行底子里本来就没边框，`{}` 不修改 = 保留无边框状态，导致该 cell 视觉断裂。

⚠️ **"汇总行"识别 → 读 `lark-sheets-visual-standards` 拿完整样式规范**：下述双重条件**同时满足**才是汇总行，禁止仅凭"有 AVERAGE"就判定：
- **语义信号**（二选一）：用户 prompt 含"合计/汇总/总计/统计/各科平均分/最下面加一行算…/底部总计"等意图词；或上下文明确是"表尾追加一行做聚合"
- **结构信号**：新行全行都在做聚合（含 `=SUM/AVERAGE/COUNT/MAX/MIN/SUBTOTAL(...)`，支持 IFERROR 包裹），**不是**单个 cell 算个参考值或每行都算的派生列

满足上述时，**不要在本 skill 里猜样式**，直接去读 `lark-sheets-visual-standards` 的"汇总行规范"章节，按那里的规则配齐 `font.bold / horizontal_alignment / background_color / border_styles`。

反例（**不是**汇总行，禁止自动加粗）：
- 用户说"在 H5 帮我算个 AVERAGE 参考"→ 单 cell 计算
- 每行都有 `=AVERAGE(本行区间)` 的派生列 → 属数据列
- 用户明确说"不要加粗/样式和数据行保持一致"→ 遵循用户意图

**正确做法**（二选一）：

**做法 A（推荐）：两步走——先铺样式、再覆内容**

```
Step 1: 用模板单元格 + copy_to_range 铺"完整样式"（不是只铺 border）到新区域
  `+cells-set` — range="A11", cells=[[{
    border_styles: {...},
    cell_styles: { /* 按行性质填充：数据行继承数据区样式；汇总行见 lark_sheet_visual_standards */ }
  }]], copy_to_range="A11:H11"

Step 2: 再用 `+cells-set` 单独写具体 value/formula（不再传样式，避免覆盖）
  `+cells-set` — range="A11", cells=[[{value: "平均分"}]]
  `+cells-set` — range="C11:F11", cells=[[{formula: "=AVERAGE(C2:C10)"}, {formula: "=AVERAGE(D2:D10)"}, ...]]
```

⚠️ **Step 1 `cell_styles` 禁止留空**：只铺 border、不铺 `cell_styles`，等于新行从格式上"裸奔"——没字体、没对齐、没背景色。如果新行是汇总行，这意味着 bold 丢失，用户感受"没做样式"。Step 1 的 `cell_styles` 要么继承源区块（`+cells-get` 读相邻已有行样式后复用），要么按汇总行规范（见 `lark-sheets-visual-standards`）配齐。

**做法 B：一次写入但每个 cell 都显式带样式**

```
`+cells-set` — range="A11:H11", cells=[[
  {value: "平均分", cell_styles: {...}, border_styles: {...}},
  {value: "",      cell_styles: {...}, border_styles: {...}},   ← B11 不能是 {}，要显式带 border
  {formula: "=AVERAGE(C2:C10)", cell_styles: {...}, border_styles: {...}},
  {formula: "=AVERAGE(D2:D10)", cell_styles: {...}, border_styles: {...}},
  ...
]]
```

**判断是不是"新行"**：`+csv-get` 返回的 `current_region` 是 `A1:H10`，你要写入的 range 是 `A11:H11`（超出 `current_region` 右/下边界），就是新行——必须按上述做法处理边框。

## 工具选择

本 skill 提供以下 CLI shortcut，按数据来源 + 内容形态选：

| 场景 | 用这个 shortcut | 原因 |
|------|----------------|------|
| 模型手里已经有 CSV 文本（小规模手动构造、从 `+csv-get` 取到后简单加工） | `+csv-put` | 直接传 CSV 文本 + start_cell，不用自己拼二维 cells 数组；必要时自动扩容行列 |
| 写入含公式、样式、批注、图片、数据校验等任意富写入 | `+cells-set` | 唯一支持完整字段的 shortcut |
| 只改已有 cell 的样式，不动 value/formula | `+cells-set-style` | 拍平 10 个样式字段为独立 flag；不触发不必要的值写入 |
| 单 cell 嵌入图片 | `+cells-set-image` | 比 `+cells-set` 参数更简短 |
| 大量纯值 + 需要表头样式/边框 | 先用 `+csv-put` 写值，再用 `+cells-set-style` 补样式 | 分工配合，入参最短 |

**优先级**：常规纯值写入优先 `+csv-put`（最短入参，直接传 CSV 文本）；含公式/样式/批注/图片才用 `+cells-set`。

⚠️ `+csv-put` 只写纯值，**不会**携带公式/样式/批注/图片；公式字符串以 `=` 开头会被当作字面量文本落地。如果数据里需要公式或样式，**必须**用 `+cells-set`（或"写值 + 补样式"两步法）。

⚠️ 大数据回写走"`+csv-get --max-rows N` 分批读到本地 + 本地脚本处理 + `+csv-put` 分批回写"。

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+cells-set` | write | 单元格 |
| `+cells-set-style` | write | 单元格 |
| `+cells-set-image` | write | 单元格 |
| `+dropdown-set` | write | 对象 |
| `+csv-put` | write | 单元格 |

## Flags

### `+cells-set`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | 是 | 写入区域 A1 格式 |
| `--cells` | string + File + Stdin（复合 JSON） | 是 | JSON：`{"values": [[...], ...]}`；可含 `formula` / `cell_styles` / `comments` / `embed_image` 富信息 |
| `--allow-overwrite` | bool | 否 | 允许覆盖非空 cell；默认 false 时遇非空 cell 报错 |
| `--max-cells` | int + Hidden | 否 | 防爆，默认 50000 |

### `+cells-set-style`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | 是 | 目标范围 A1 格式（如 `A1:B2`） |
| `--background-color` | string | 否 | 背景颜色（十六进制，如 `#ffffff`） |
| `--font-color` | string | 否 | 字体颜色（十六进制，如 `#000000`） |
| `--font-size` | number | 否 | 字体大小（px，例：10、12、14） |
| `--font-style` | string + Enum | 否 | 字体样式 enum：`normal` / `italic` |
| `--font-weight` | string + Enum | 否 | 字重 enum：`normal` / `bold` |
| `--font-line` | string + Enum | 否 | 字体线条样式 enum：`none` / `underline` / `line-through` |
| `--horizontal-alignment` | string + Enum | 否 | 水平对齐 enum：`left` / `center` / `right` |
| `--vertical-alignment` | string + Enum | 否 | 垂直对齐 enum：`top` / `middle` / `bottom` |
| `--word-wrap` | string + Enum | 否 | 换行策略 enum：`overflow` / `auto-wrap` / `word-clip`（默认 `overflow`） |
| `--number-format` | string | 否 | 数字格式（例：文本 `@`、数字 `0.00`、货币 `$#,##0.00`、日期 `mm/dd/yyyy`） |
| `--border-styles` | string + File + Stdin（复合 JSON） | 否 | 边框配置 JSON：`{ top: {style,color,weight}, bottom: ..., left: ..., right: ... }`；4 方向结构相同 |

### `+cells-set-image`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | 是 | 目标 cell A1（必须单 cell，如 `A1`；起止 cell 须相同） |
| `--image` | string | 是 | 本地图片路径（支持 PNG / JPEG / JPG / GIF / BMP / JFIF / EXIF / TIFF / BPG / HEIC） |
| `--name` | string | 否 | 图片文件名（含扩展名）；省略时取 `--image` 的 basename |

### `+dropdown-set`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | 是 | 目标范围 A1 格式（如 `A2:A100`） |
| `--options` | string + File + Stdin（复合 JSON） | 是 | 选项 JSON 数组 `["opt1","opt2"]`；最多 500 项，每项 ≤100 字符，不含逗号 |
| `--colors` | string + File + Stdin（简单 JSON） | 否 | RGB hex 颜色数组（如 `["#1FB6C1","#F006C2"]`），长度必须与 `--options` 一致 |
| `--multiple` | bool | 否 | 启用多选；默认 `false` |
| `--highlight` | bool | 否 | 选项配色显示；默认 `false` |

### `+csv-put`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | 是 | 目标区域起点 A1（如 `Sheet1!A1`）；自动按 CSV 行列数推断终点 |
| `--csv` | string + File + Stdin（非 JSON 文本） | 是 | RFC 4180 CSV 文本；只写纯值，不带公式/样式/批注 |
| `--allow-overwrite` | bool | 否 | 允许覆盖；默认 false 时若目标非空报错 |

## Schemas

> 复合 JSON flag（如 `--cells` / `--properties` / `--operations` / `--border-styles` / `--sort-keys`）的字段速查：只列顶层字段 + 一层嵌套结构。深层结构看 `## Examples` 段的真实示例；要拿完整 JSON Schema 跑 `lark-cli sheets <shortcut> --print-schema --flag-name <name>`。先 `--print-schema`（不带 `--flag-name`）会列出该 shortcut 所有可查询的 flag。

### `+cells-set` `--cells`


**顶层字段**：
- `border_styles` (object?) — 单元格边框配置，含 top/bottom/left/right 四个方向，每个方向的结构相同（见 top） { bottom?: object, left?: object, right?: object, top?: object }
- `cell_styles` (object?) — 单元格样式属性，包括字体、颜色、对齐方式和数字格式 { background_color?: string, font_color?: string, font_line?: enum, font_size?: number, font_style?: enum, …共 10 项 }
- `data_validation` (object?) — 数据验证配置 { help_text?: string, items?: array<string>, operator?: enum, range?: string, support_multiple_values?: boolean, …共 7 项 }
- `formula` (string?) — 以 '=' 开头的单元格公式（例如：'=SUM(A1:A10)'）
- `multiple_values` (array<object>?) — 多值内容，用于支持多选的列表验证单元格 each: { format?: string, value: oneOf }
- `note` (string?) — 单元格批注/备注
- `rich_text` (array<object>?) — 富文本内容 each: { attachment_name?: string, attachment_token?: string, attachment_uri?: string, file_size?: number, image_height?: number, …共 17 项 }
- `value` (oneOf?) — 静态单元格值（文本、数字、布尔）

### `+cells-set-style` `--border-styles`

_单元格边框配置，含 top/bottom/left/right 四个方向，每个方向的结构相同（见 top）_

**顶层字段**：
- `bottom` (object?) { color?: string, style?: enum, weight?: enum }
- `left` (object?) { color?: string, style?: enum, weight?: enum }
- `right` (object?) { color?: string, style?: enum, weight?: enum }
- `top` (object?) { color?: string, style?: enum, weight?: enum }

### `+dropdown-set` `--options`

_数据验证配置_

**顶层字段**：
- `help_text` (string?) — 验证失败时显示的提示文本
- `items` (array<string>?) — 列表选项（type='list' 时必填）
- `operator` (enum?) — 比较运算符（type='number'/'date'/'textLength' 时必填） [equal / notEqual / greaterThan / greaterThanOrEqual / lessThan / lessThanOrEqual / between / notBetween]
- `range` (string?) — 源数据区域（type='listFromRange' 时必填，格式：'SheetName!A1:A10'）
- `support_multiple_values` (boolean?) — 列表验证是否支持多选（type='list'/'listFromRange' 时可选，默认 false）
- `type` (enum) — 数据验证类型：list（下拉列表）、listFromRange（引用范围下拉列表）、number（数字）、date（日期）、textLength（文本长度）、… [list / listFromRange / number / date / textLength / checkbox]
- `values` (array<oneOf>?) — 比较值（operator 为 'between'/'notBetween' 时需要两个值，其它运算符需要一个值）

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。

### `+cells-set` 的拆分与转介绍

"工具选择"段已讲清纯值（`+csv-put`）vs 富写入（`+cells-set`）。下表补 CLI 侧的 `+cells-set` **兄弟拆分**，以及不属于本 skill 的**跨 skill 转介绍**——避免 agent 用 `+cells-set` 硬扛所有写入场景。

| 写入场景 | 用这个 | 不要用 |
|---------|--------|--------|
| 只改**已有 cell 的样式**，不动 value/formula | `+cells-set-style` | `+cells-set`（会触发不必要的值写入） |
| 把**单张图片嵌入**到某个 cell | `+cells-set-image` | `+cells-set`（参数更繁琐） |
| **插行/列 + 写入** 这种多步组合，且要原子 | `+batch-update`（跨 skill） | 多次独立 `+cells-set`（非原子；插入会扰动后续 range） |
| 在**多个不连续 range** 上应用同一组样式 | `+cells-batch-set-style`（跨 skill） | 多次 `+cells-set-style`（非原子） |

### `+cells-set`

示例：

```bash
# 纯值（数组形态）
lark-cli sheets +cells-set --url "https://example.feishu.cn/sheets/shtXXX" \
  --sheet-name "Sheet1" --range "A1:B2" --allow-overwrite \
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
  --sheet-name "Sheet1" --range "A1" --allow-overwrite \
  --csv $'name,score\nalice,95\nbob,87'

# 从文件
lark-cli sheets +csv-put --spreadsheet-token shtXXX --sheet-id "$SID" \
  --range "A1" --csv @data.csv --allow-overwrite
```

> `+csv-put` 比 `+cells-set` 短得多——只想批量灌纯值时优先用它。需要公式/样式才换 `+cells-set`。

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`+cells-set` 的 `--cells` 必须能解析为 JSON 二维矩阵且行列数与 `--range` 完全一致；`+cells-set-style` 的样式 flag 至少一个非空（或带 `--border-styles`）；`+cells-set-image` 的 `--range` 必须是单 cell（起止 cell 相同）；`+csv-put` 的 `--csv` 必须能按 RFC 4180 解析；防爆参数上限校验。
- `DryRun`：输出目标 range + 推断尺寸 + 是否覆盖非空 cell 警告，零网络副作用。
- `Execute`：写后调用 `+cells-get --range <写入区域> --include value,formula` 抽样回读，envelope.meta.verification 给出"预期 vs 实际"对比。
