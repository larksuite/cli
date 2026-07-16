# Lark Sheet Read Data

## 列格式多样性预探（写公式 / 排序 / 筛选前必做）

> 本节给出"写公式 / 排序 / 筛选前先探清列格式多样性"的正确流程，是主 SKILL.md「飞书表格编辑准则」准则 3（读全再写）在 read_data 工具层的落地。

对参与后续**计算 / 排序 / 筛选 / 公式提取**的列，**必须**先 sample **至少 50 行**（小表则全量），识别该列所有值类型变体后再设计公式 / 条件。只看前 10 行不够，因为下列差异通常潜伏在表尾或中段：

- **日期列同时出现多种格式**：`YYYYMM`、`YYYY-MM-DD`、`YYYY/M/D`、带时间戳、文本"未知"
- **数值列混入公式文本 / 单位 / 注释**：`1000+200=1200`、`100元`、`/（合同未明确）`、`#N/A`
- **空值与 0 / "0" 混杂**
- **大小写 / 全角半角差异**（"办公费" vs "办公费 "、"Sales" vs "sales"）

预探后必须在公式 / 筛选条件里用 `IFERROR` / `IFS` / 提取数值的辅助列处理所有变体；不能为了通过 head(10) 的样本就直接落地。一旦设计的逻辑只覆盖 sample 中出现的格式，就属于违规。

⚠️ **大数字（15 位以上的身份证 / 参考号 / 流水号）做去重 / 比较时禁止用 `+csv-get` 的显示值**：`+csv-get` 返回的是**格式化显示值**，15 位以上数字会被显示成 `1.04E+14` 这类科学计数法——多个本不相同的号在显示层全变成同一个 `1.04E+14`，拿去判重会**整列误判为重复**。比较 / 去重 / 匹配大数字时必须改用 `+cells-get`（取原始精确值）或把该列读为文本，禁止用 csv-get 的科学计数显示值（反例：大批长参考号被显示成科学计数后，互不相同的号全变成同一个值，被当成整列重复并错误高亮）。

## 使用场景

读取。从飞书表格中读取单元格数据。本 reference 覆盖 4 个 shortcut，按读取目的选择：

| 读取目的 | 用这个 shortcut | 数据去向 | 说明 |
|---------|----------------|---------|------|
| 快速查看纯值数据、批量处理 | `+csv-get` | 对话上下文 | 返回 CSV 文本（每行带 `[row=N]` 前缀）；大表请按 `--range` 行窗口分批读（截断时看 `has_more`） |
| 按列类型结构化读出（喂 DataFrame / round-trip 回 `+table-put`） | `+table-get` | 对话上下文 | 返回 typed 协议（`columns:[列名]` + `data` + `dtypes`/`formats` + `range`），输出形状对齐 pandas split；可一行 `pd.DataFrame(sheet["data"], columns=sheet["columns"]).astype(sheet["dtypes"])` 还原 DataFrame，或直接 round-trip 回 `+table-put`。不带 `--range` 时读**完整 used range**（跨过表中部空行 / 空列），每个子表回传实际读取范围 `range` 供完整性校验。注意这与下文 `current_region` "遇表中部空行截断"不矛盾：`+table-get` 读的是子表物理 used range（飞书记录的已用矩形，含中间空行），`current_region` 是从锚点连通扩展、遇整行空行就断 |
| 查看公式、样式、批注、数据验证 | `+cells-get` | 对话上下文 | 返回单元格完整信息，token 开销较大 |
| 查看某区域的下拉框（数据验证）选项 | `+dropdown-get` | 对话上下文 | 返回该 A1 范围已配置的下拉列表选项 |

**选择原则**：
- 只看值或做数据处理 → `+csv-get`；大表分批读取，避免一次拉全表撑爆上下文
- 要按列类型结构化读出（喂 DataFrame / round-trip 回 `+table-put`）→ `+table-get`
- 需要公式/样式/批注 → `+cells-get`
- 只想知道某区域下拉框有哪些选项 → `+dropdown-get`

⚠️ **大数据优先落盘、别灌进上下文**：`+csv-get` / `+cells-get` 都受调用方 Bash / 终端的单命令 stdout 输出上限约束（常见默认约 30000 字符，超过会被截断或转存为文件）。纯值分析优先 `+csv-get --format csv` 按 `--range` 行窗口（`A1:Z500` / `A501:Z1000` …）分批重定向到文件 + 本地脚本处理 + `+csv-put` 分批回写；若确实要让结果直接进上下文又不想触发转存，给任一命令把 `--max-chars`（默认 500000）调小到略低于该上限（如 `25000`），CLI 改为优雅截断 + `has_more` 分页。

**`+csv-get` 返回值核心设计**：
- `annotated_csv` — **CSV 数据唯一入口**。每一逻辑行前加 `[row=N] ` 前缀（N = 真实表格行号）。任何需要行号的下游操作（合并、写入、清空、格式化、插入/删除、条件格式、筛选、图表/透视表范围、搜索替换等），**行号一律直接从 `[row=N]` 读取**。若需要纯 CSV（如喂给本地脚本做解析），去前缀即可：`line.replace(/^\[row=\d+\] /, '')`。
- `col_indices` — **定位列字母唯一入口**。在表头中找到目标字段是第 j 个（0-based），用 `col_indices[j]` 取列字母。**禁止手数逗号**——列数超过 10 时极易 off-by-one（例如把 W 误判为 X）。
- `row_indices` — 程序化引用的备用数组。LLM 推理请用 `annotated_csv` 的前缀，不要查这个数组里的 index（把行号当数值用容易心算出错）。
- `current_region` — 从请求范围扩展到被空行空列包围的连续数据区域（等价于 Excel Ctrl+Shift+*），适合先读少量行探表头。⚠️ 它**遇表中部整行空行 / 整列空列就截断**，可能小于真实数据范围（漏掉空行之后的行）；**不能**直接当整表末行用，判断整表是否读全要拿 `+workbook-info` 的物理 `row_count` / `column_count` 当上界交叉核对（见下方「按 row_count 盲读空行」与「确定数据范围的正确流程」）。

注意：

- `+csv-get` 和 `+cells-get` 支持分页/截断，注意检查 `has_more` / `truncated` 标志；两者在处理返回数据之前都必须先读 `warning_message`（上游 schema 要求先读它再用其它字段，内含定位与截断续读提示），`+cells-get` 还要用每个 range 的 `actual_range` / `row_indices` / `col_indices` 判断真实位置
- 隐藏行列默认包含在返回结果中（`--skip-hidden=false`），如需只看可见数据设为 `true`。读取原语本身不标注哪些行列被隐藏：若要识别隐藏区间（以决定是否过滤、或如何解读混入的隐藏数据），用 `+sheet-info --include hidden_rows,hidden_cols` 取隐藏行列集合，再结合 `+csv-get` / `+cells-get` 返回的 `row_indices` / `col_indices` 判断每行 / 每列是否隐藏

**常见配置错误（必须注意）**：
- **全量读取导致上下文溢出**：不要对大表（数百行以上）直接用 `+csv-get` 或 `+cells-get` 读取全部数据到上下文。大表场景必须分批读取：用 `--range` 切行窗口逐块读（`+csv-get` / `+cells-get` 单次返回量由 `--max-chars` 自动兜底，截断时返回 `has_more`）；过大时考虑导出到本地文件后用脚本处理再分批回写
- **了解结构 ≠ 读取全量数据**：探表不用读全表，但必须同时探两个方向的表头：
  - **横向（列头）**：先读前几行，且**列范围必须覆盖所有列**——用 `+workbook-info` 拿总列数，`range` 末列填到最后一列（例如总列数是 N，则 `range: "A1:[列N]10"`）。列范围截短会遗漏右侧字段、后续写入列定位错误。
  - **纵向（行标）**：若左侧 1-2 列是行标签（日期/类别/编号枚举每行含义，典型交叉表/透视布局），**必须再读 `A:A` 或 `A:B` 把行标列读到底**，拿全部行标。只读前几行会看不全表尾的行，导致批量写入漏改——这是"只改前 N 行、其余未更新"的主要成因。扁平列表（每行独立记录、列是字段）可跳过这一步，但仍要按下方「确定数据范围的正确流程」用 `+workbook-info` 的物理 `row_count` 交叉核对末行（`current_region` 遇空行会截断，不能单独兜底）。
  - 数据量大或会进入上下文上限时，分批读 + 本地处理 + 分批回写，不要一口气拉全表到上下文。
- **`+cells-get` 滥用**：当只需要数据值时，使用 `+csv-get`（token 开销约为 `+cells-get` 的 1/5）。只有确实需要公式、样式或批注时才用 `+cells-get`
- **忽略分页标志**：读取返回 `has_more=true` 时，说明还有更多数据。如果任务需要完整数据，必须继续分页读取，不能只处理第一页就开始写入
- **直接按 `+cells-get` 返回二维数组下标推导真实位置**：`ranges[n].cells[i][j]` 里的 `i/j` 只是返回数组下标，不等于真实表格行列。定位真实行号必须用 `ranges[n].row_indices[i]`，定位真实列字母必须用 `ranges[n].col_indices[j]`；若 `--skip-hidden=true`、请求范围越界被裁剪，或最后一行是部分返回，错误地自己数下标会立刻错位
- **CSV 行号计数错误**：`+csv-get` 返回的 CSV 遵循 RFC 4180 标准，被双引号 `"..."` 包裹的字段中的换行符属于**字段内容的一部分**（即单元格内换行），不代表新的一行。计算行号时必须按**逻辑记录**计数，而非按物理换行符 `\n` 计数
- **手动数列确定列号**：禁止通过在 CSV 表头中手动数逗号/字段来确定目标列的列字母。当列数超过 10 时，手动计数极易产生 off-by-one 偏移（例如把 W 列误判为 X 列）。**必须使用 `col_indices`**：先在 CSV 表头中找到目标字段名是第 j 个字段（0-based），再用 `col_indices[j]` 获取该列的实际列字母
- **用数据列的值推导行号（常被巧合掩盖）**：CSV 中常见"序号 / ID / 编号 / No."等形似行号的列，其值与实际表格行号**没有任何绑定关系**——序号可能跳号（1,2,3,5,6...）、可能从非 1 开始、可能有重复或被中途重置。此规则适用于**所有需要行号的下游操作**：合并单元格、区间写入/清空/格式化、插入/删除行、条件格式范围、筛选器范围、图表数据源、透视表范围、搜索替换范围等等——**凡是要把行号填进任何工具参数的场景，行号一律从 `annotated_csv` 中目标行开头的 `[row=N]` 前缀直接读取**，禁止用"序号=行号"、"表头占 1 行所以数据从第 2 行开始"、"第 N 个序号就在第 N+1 行"等心算，也禁止先心算再"事后核对"。**危险特征**：前几十行中序号恰好等于表格行号（典型成因：表头 +1 与一次跳号 -1 的偏移互相抵消形成巧合），模型一旦把这个巧合当作规律，会在后续所有行沿用；而中间再出现跳号时，从该行起整块区域全部错位，且错位不自查很难发现。**正确工作流**：①在 `annotated_csv` 里定位目标逻辑行（按字段内容匹配）；②直接读取该行开头的 `[row=N]` 前缀得到真实表格行号；③把这个行号填进下游工具参数。区间操作时，起始行用 start 行的 `[row=N]`、结束行用 end 行的 `[row=N]`。**自检**：动手前，在 `annotated_csv` 靠后位置再抽 1~2 行，核对 `[row=N]` 是否与首列"序号"一致——不一致（典型：`[row=57] 58,...`）即说明有跳号/隐藏行，更要严格从 `[row=N]` 取值，不要被序号列迷惑
- **`row_count` 与 `current_region` 都不能单独定末行**：`+workbook-info` 的 `row_count` 是 sheet 的**网格物理行数**（常是 200 / 1000 等默认值），通常**大于**真实数据末行——直接按它把 `--range` 拉到 `S200` 会读回大片空行，浪费上下文。反过来，`+csv-get` 返回的 `current_region` 是从锚点扩展、被空行空列围住的连续块，**遇表中部整行空行就截断**，可能**小于**真实数据范围（漏掉空行之后的行，典型反例：1–80 行有数据、81 行空、82 行起还有数据，`current_region` 只到 80，82 行起整段被漏读）。正确做法：把 `row_count` 当**上界**、`current_region` 当**起点参考**，在二者之间按下方「确定数据范围的正确流程」确认真实末行（含跨过中间空行的核对），不要只信其一。
- **current_region 当作纯数据范围**：`current_region` 返回的是从请求范围向四周扩展到被空行空列包围的**连续非空区域**，等价于 Excel 的 Ctrl+Shift+\*。它包含该区域内**所有非空行**——不仅包含数据行，还可能包含标题行、汇总行（如"总计"）、签名行（如"编制人/审批人"）、脚注等非数据内容。**严禁直接将 `current_region` 的末尾行作为数据范围的结束行**。正确做法见下方「确定数据范围的正确流程」

### 确定数据范围的正确流程（排序、筛选、批量写入等操作前必做）

当后续操作需要精确的数据范围（如排序、筛选、删除、批量写入）时，仅靠 `current_region` 探测到的范围是不够的——它**两头都可能不准**：表中部有整行空行时会被截断（末行偏小、漏数据），表尾有汇总 / 签名行时又会偏大。必须同时确认数据的**起始行**和**结束行**。具体步骤：

1. **确认起始行**：读取前 5~10 行，识别表头行位置，数据起始行 = 表头行 + 1
2. **确认结束行**（关键步骤，不可跳过）：
   - **先防截断（漏数据）**：拿 `+workbook-info` 的物理 `row_count` 当上界，与 `current_region` 末行对比。若 `current_region` 末行 **远小于** `row_count`（差出很多空间），不要直接采信——在 `current_region` 末行之后再探一段（如往下读到 `row_count`，或分段扫到首个连续空白区），确认空行之后确实没有数据；典型反例：`row_count=327`、`current_region` 只到第 80 行，第 81 行空、82 行起还有数据，只读到 80 就漏了一大段。
   - **再排尾部非数据行**：读取确认到的末行附近若干行（建议末尾 5~10 行），逐行排除：
     - **汇总行**：内容为"合计"、"总计"、"小计"、"总计:"等
     - **签名/审批行**：内容为"编制人"、"审核人"、"部门负责人"等
     - **空行或分隔行**：整行为空或仅有边框
     - **备注/脚注行**：注释性文字、说明文字等
3. **最终数据范围** = 起始行 ~ 最后一条有效数据行（跨过中间空行、排除尾部非数据行）

**示例**：`current_region` 返回 `A1:N51`，读取 Row 48~51 发现：

- Row 49: 序号=47, 姓名=xxx, 有正常数据 → ✅ 数据行
- Row 50: "总计", 有合并单元格 → ❌ 汇总行
- Row 51: "总经理：...", "编制人：..." → ❌ 签名行
- **正确数据范围 = A3:N49**（而非 A3:N51）

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+cells-get` | read | 单元格 |
| `+dropdown-get` | read | 对象 |
| `+csv-get` | read | 单元格 |
| `+table-get` | read | 单元格 |

## Flags

### `+cells-get`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | required | A1 范围，如 `A1:F10`（不带 sheet 前缀；用 `--sheet-id` / `--sheet-name` 指定 sheet） |
| `--include` | string_slice | optional | 要返回的信息类别，逗号分隔多个（可选值：`value` / `formula` / `style` / `comment` / `data_validation`） |
| `--max-chars` | int | optional | 单次返回字符上限，默认 500000（兜底防爆）。大数据通常宜重定向落盘做分析；仅当要让结果直接进上下文、又不触发文件转存时才调小（如 25000），以 has_more 分页 |
| `--skip-hidden` | bool | optional | 跳过隐藏行列，默认 `false` |

### `+dropdown-get`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | required | A1 范围，如 `A2:A100`（不带 sheet 前缀；用 `--sheet-id` / `--sheet-name` 指定 sheet） |

### `+csv-get`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | required | A1 范围，如 `A1:F30`（不带 sheet 前缀；用 `--sheet-id` / `--sheet-name` 指定 sheet） |
| `--max-chars` | int | optional | 单次返回字符上限，默认 500000（兜底防爆）。大数据通常宜重定向落盘做分析；仅当要让结果直接进上下文、又不触发文件转存时才调小（如 25000），以 has_more 分页 |
| `--include-row-prefix` | bool | optional | 是否在每行前加 `[row=N]` 前缀，默认 `true` |
| `--skip-hidden` | bool | optional | 跳过隐藏行列，默认 `false` |

### `+table-get`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--sheet-id` | string | optional | 只读该子表（按 id）；省略则读所有子表 |
| `--sheet-name` | string | optional | 只读该子表（按名）；省略则读所有子表 |
| `--range` | string | optional | 读取的 A1 范围；省略则读每个子表的完整 used range（会跨过表中部的整行空行 / 整列空列，不会被截断） |
| `--no-header` | bool | optional | 把第一行当数据而非表头（列名取 col1/col2 …） |

## Examples

### `+csv-get`

公共四件套：`--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（前两者 XOR，后两者 XOR）。

示例：

```bash
# 简单读（sheet 定位必填：--sheet-name 或 --sheet-id 必给一个；range 的 Sheet1! 前缀不能替代它）
lark-cli sheets +csv-get --url "https://example.feishu.cn/sheets/shtXXX" --sheet-name "Sheet1" --range "A1:F30"

# 用 sheet-name 模糊定位（运行时框架会先解析到 sheet-id）
lark-cli sheets +csv-get --spreadsheet-token shtXXX --sheet-name "销售明细" --range "A1:F30"
```

输出契约（envelope.data）：

- `annotated_csv` — 含 `[row=N]` 前缀的 CSV 主入口
- `col_indices` / `row_indices` — 列字母 / 行号映射数组
- `current_region` — 从锚点扩展到被空行空列包围的连续区域的 A1 范围。⚠️ **它不是整表真实边界**：遇表中部整行空行 / 整列空列会截断、可能小于真实数据范围；表尾的汇总 / 签名 / 脚注又可能让它大于纯数据范围。判断整表是否读全须拿 `+workbook-info` 的物理 `row_count` 当上界交叉核对（见上方「`row_count` 与 `current_region` 都不能单独定末行」）
- `row_count` / `col_count` — **本次返回的行 / 列数**（= `actual_range` 的尺寸，随 `--range` 变），**不是整表物理总行列数**；整表物理尺寸取 `+workbook-info`
- `has_more` — 当前 `--range` 是否因 `--max-chars` 被截断（截断后续读接着用 `--range`）；它**只反映本次 range 内是否读完**，`has_more=false` **不代表整表已读全**（range 之外的数据不在判断内）

> 要按列类型结构化读出（喂 DataFrame、或 round-trip 回 `+table-put`）用 `+table-get`（见下）；`+csv-get` 给的是带 `[row=N]` 前缀的纯值快照，下游需要行号/列坐标时直接从前缀与 `col_indices` 取。

### `+cells-get`

示例：

```bash
# 读 A1:F10 的公式 + 样式（sheet 定位必填）
lark-cli sheets +cells-get --url "https://example.feishu.cn/sheets/shtXXX" --sheet-name "Sheet1" \
  --range "A1:F10" --include formula,style
```

> ⚠️ 调用方在 `cells[i][j]` 中**不能**用下标推真实行列：必须读 `ranges[n].row_indices[i]` / `ranges[n].col_indices[j]`。

### `+table-get`（飞书 → DataFrame，类型保真读出）

`+table-put`（写入侧，见 write-cells reference）的镜像：把表格读回与 `--sheets` 完全同构的 typed 协议（`sheets[]` + `columns:[列名]` + `data:[[行]]` + `dtypes:{列名:pandas_dtype}` + `formats?:{列名:number_format}` + `range`），可直接喂回 `+table-put` 或一行还原 DataFrame。

**默认（不带 `--range`）读取整张子表的完整 used range**：会跨过表中部的整行空行 / 整列空列，覆盖到真实数据边界。每个子表都回传实际读取的 `range`（如 `A1:F10`）——`+table-get` 不返回分页 / 截断标志，这个 `range` 是判断是否读全的唯一信号：拿它和源 xlsx 行列数、关键末行 / 末日期交叉核对，确认读取完整。仍要精确控制范围时显式传 `--range`。

列类型从每列 `number_format` 推断（日期格式→`date`/`datetime64[ns]`、数值→`number`/`float64`、bool→`bool`），`date` 列的序列号转回 ISO `yyyy-mm-dd`——日期、数字往返不丢类型。**列类型只在该列所有非空值一致时才定（`number` / `date` / `bool`）；一列混了类型（如数字列混入「暂无」、日期列混入裸数字）会降为 `string`（dtypes 输出 `object`），让 `dtypes` 与 `data` 里每个值自洽——能 round-trip 回 `+table-put`、不让 pandas `astype` 崩。降级是无损的（脏值原样保留为文本）；若要把零星脏值转成数值列，交给调用方在 pandas 侧做（`to_numeric(errors='coerce')`），那里原始值仍在、可追溯。** 默认读所有子表、第一行当表头（`--no-header` 把首行当数据、列名取 `col1` / `col2` …）。

```bash
# 默认读所有子表 → sheets[]（与 +table-put 的 --sheets 同构，可喂回或转 DataFrame）
lark-cli sheets +table-get --url "<表URL>"
# 可选：--sheet-name / --sheet-id 限定只读某一个子表（不给则读全部）
lark-cli sheets +table-get --url "<表URL>" --sheet-name "销售"
```

#### 输出 → DataFrame（用 `sheet_to_df` helper）

输出形状对齐 pandas split：`columns` 是列名数组、`data` 是二维数据、`dtypes` 是 `{列名: pandas_dtype_str}` 映射。直接喂给 `pd.DataFrame(...).astype(...)` 就能一次性还原所有列类型（不必逐列 `to_datetime` / `to_numeric`）。本 skill 把这段 2 行 helper 打包成可 import 的 [`scripts/sheets_df.py`](../scripts/sheets_df.py)（含 `df_to_sheet` 和 `sheet_to_df`，写入 / 读回成对）：

```python
from sheets_df import sheet_to_df

# 单 sheet
df = sheet_to_df(out["data"]["sheets"][0])

# 多 sheet——按名字取
sheets = {s["name"]: sheet_to_df(s) for s in out["data"]["sheets"]}
df_sales = sheets["销售"]
```

> 显示格式（千分位、百分比、自定义日期）在 `sheet["formats"]`，pandas 不消费；改完数据 round-trip 回去时透传给 `+table-put` 即可，飞书侧显示不变。

#### round-trip：读 → 改 → 写回（写读对偶）

`sheet_to_df` 和 `df_to_sheet` 一对镜像 helper（[`scripts/sheets_df.py`](../scripts/sheets_df.py)）让 round-trip 三段读 / 改 / 写各一行：

```python
import json, subprocess
from sheets_df import df_to_sheet, sheet_to_df

# 1. 读
out = json.loads(subprocess.check_output(
    ["lark-cli","sheets","+table-get","--url",URL,"--sheet-name","销售"]))
sheet = out["data"]["sheets"][0]
df = sheet_to_df(sheet)

# 2. 改（pandas 操作）
df["营收"] = df["营收"] * 1.1

# 3. 写回（formats 是飞书侧显示格式，pandas 不消费，透传保留显示）
payload = {"sheets": [df_to_sheet(df, sheet["name"], formats=sheet.get("formats"))]}
subprocess.run(["lark-cli","sheets","+table-put","--url",URL,"--sheets","-"],
               input=json.dumps(payload).encode(), check=True)
```

`sheet_to_df(sheet)` 消费 `(columns, data, dtypes)`，`df_to_sheet(df, name, formats=...)` 重新生成同样三个字段——读 / 写完全对偶，只有 `formats` 需要手工透传一次。

### Validate / DryRun / Execute 约束

- `Validate` 阶段只做 XOR 检查、Enum 合法性、防爆参数上限校验；**禁止**联网（如不能用 `--sheet-name` 提前去查 `sheet-id`）。
- `DryRun` 输出请求模板：`--sheet-name` 在 dry-run 输出里生成为 `<resolve:销售明细>` 占位符，不实际解析。
- `Execute` 阶段才进行 sheet-name → sheet-id 解析与 API 调用。
