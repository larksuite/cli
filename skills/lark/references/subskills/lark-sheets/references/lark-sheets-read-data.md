# Lark Sheet Read Data

## 列格式多样性预探（写公式 / 排序 / 筛选前必做）

> 对应 `lark-sheets-core-operations` 的 **R3 计算复现**——本节是 R3 在 read_data 工具层的具体落地。

对参与后续**计算 / 排序 / 筛选 / 公式提取**的列，**必须**先 sample **至少 50 行**（小表则全量），识别该列所有值类型变体后再设计公式 / 条件。只看前 10 行不够，因为下列差异通常潜伏在表尾或中段：

- **日期列同时出现多种格式**：`YYYYMM`、`YYYY-MM-DD`、`YYYY/M/D`、带时间戳、文本"未知"
- **数值列混入公式文本 / 单位 / 注释**：`1000+200=1200`、`100元`、`/（合同未明确）`、`#N/A`
- **空值与 0 / "0" 混杂**
- **大小写 / 全角半角差异**（"办公费" vs "办公费 "、"Sales" vs "sales"）

预探后必须在公式 / 筛选条件里用 `IFERROR` / `IFS` / 提取数值的辅助列处理所有变体；不能为了通过 head(10) 的样本就直接落地。一旦设计的逻辑只覆盖 sample 中出现的格式，就属于违规。

## 使用场景

读取。从飞书表格中读取单元格数据。本 reference 覆盖 3 个 shortcut，按读取目的选择：

| 读取目的 | 用这个 shortcut | 数据去向 | 说明 |
|---------|----------------|---------|------|
| 快速查看纯值数据、批量处理 | `+csv-get` | 对话上下文 | 返回 CSV 文本（加 `--rows-json` 改为结构化 rows `{row_number, values:{列字母→值}}`）；大表请按 `--range` 行窗口分批读（截断时看 `has_more`） |
| 查看公式、样式、批注、数据验证 | `+cells-get` | 对话上下文 | 返回单元格完整信息，token 开销较大 |
| 查看某区域的下拉框（数据验证）选项 | `+dropdown-get` | 对话上下文 | 返回该 A1 范围已配置的下拉列表选项 |

**选择原则**：
- 只看值或做数据处理 → `+csv-get`；大表分批读取，避免一次拉全表撑爆上下文
- 要结构化、按 `row_number` / 列字母定位的输出 → `+csv-get --rows-json`（默认 CSV 串更省 token，超大表批量仍用默认）
- 需要公式/样式/批注 → `+cells-get`
- 只想知道某区域下拉框有哪些选项 → `+dropdown-get`

⚠️ 超大数据请走"`+csv-get` 按 `--range` 行窗口（如 `A1:Z500` / `A501:Z1000` …）分批读到本地文件 + 本地脚本处理 + `+csv-put` 分批回写"。

**`+csv-get` 返回值核心设计**：
- `annotated_csv` — **CSV 数据唯一入口**。每一逻辑行前加 `[row=N] ` 前缀（N = 真实表格行号）。任何需要行号的下游操作（合并、写入、清空、格式化、插入/删除、条件格式、筛选、图表/透视表范围、搜索替换等），**行号一律直接从 `[row=N]` 读取**。若需要纯 CSV（如喂给本地脚本做解析），去前缀即可：`line.replace(/^\[row=\d+\] /, '')`。
- `col_indices` — **定位列字母唯一入口**。在表头中找到目标字段是第 j 个（0-based），用 `col_indices[j]` 取列字母。**禁止手数逗号**——列数超过 10 时极易 off-by-one（例如把 W 误判为 X）。
- `row_indices` — 程序化引用的备用数组。LLM 推理请用 `annotated_csv` 的前缀，不要查这个数组里的 index（把行号当数值用容易心算出错）。
- `current_region` — 从请求范围扩展到被空行空列包围的连续数据区域（等价于 Excel Ctrl+Shift+*），适合先读少量行探表头、同时获知整表实际范围。

注意：

- `+csv-get` 和 `+cells-get` 支持分页/截断，注意检查 `has_more` / `truncated` 标志；使用 `+cells-get` 时，在读取 `cells` 之前还必须先看 `warning_message`，并用每个 range 的 `actual_range` / `row_indices` / `col_indices` 判断真实位置
- 隐藏行列默认包含在返回结果中（`--skip-hidden=false`），如需只看可见数据设为 `true`

**常见配置错误（必须注意）**：
- **全量读取导致上下文溢出（高频致命错误）**：不要对大表（数百行以上）直接用 `+csv-get` 或 `+cells-get` 读取全部数据到上下文。大表场景必须分批读取：用 `--range` 切行窗口逐块读（`+csv-get` / `+cells-get` 单次返回量由 `--max-chars` 自动兜底，截断时返回 `has_more`）；过大时考虑导出到本地文件后用脚本处理再分批回写
- **了解结构 ≠ 读取全量数据**：探表不用读全表，但必须同时探两个方向的表头：
  - **横向（列头）**：先读前几行，且**列范围必须覆盖所有列**——用 `+workbook-info` 拿总列数，`range` 末列填到最后一列（例如总列数是 N，则 `range: "A1:[列N]10"`）。列范围截短会遗漏右侧字段、后续写入列定位错误。
  - **纵向（行标）**：若左侧 1-2 列是行标签（日期/类别/编号枚举每行含义，典型交叉表/透视布局），**必须再读 `A:A` 或 `A:B` 把行标列读到底**，拿全部行标。只读前几行会看不全表尾的行，导致批量写入漏改——这是"只改前 N 行、其余未更新"的主要成因。扁平列表（每行独立记录、列是字段）可跳过这一步，但仍要靠 `current_region` 兜底。
  - 数据量大或会进入上下文上限时，分批读 + 本地处理 + 分批回写，不要一口气拉全表到上下文。
- **`+cells-get` 滥用**：当只需要数据值时，使用 `+csv-get`（token 开销约为 `+cells-get` 的 1/5）。只有确实需要公式、样式或批注时才用 `+cells-get`
- **忽略分页标志**：读取返回 `has_more=true` 时，说明还有更多数据。如果任务需要完整数据，必须继续分页读取，不能只处理第一页就开始写入
- **直接按 `+cells-get` 返回二维数组下标推导真实位置（高频错误）**：`ranges[n].cells[i][j]` 里的 `i/j` 只是返回数组下标，不等于真实表格行列。定位真实行号必须用 `ranges[n].row_indices[i]`，定位真实列字母必须用 `ranges[n].col_indices[j]`；若 `--skip-hidden=true`、请求范围越界被裁剪，或最后一行是部分返回，错误地自己数下标会立刻错位
- **CSV 行号计数错误（高频致命错误）**：`+csv-get` 返回的 CSV 遵循 RFC 4180 标准，被双引号 `"..."` 包裹的字段中的换行符属于**字段内容的一部分**（即单元格内换行），不代表新的一行。计算行号时必须按**逻辑记录**计数，而非按物理换行符 `\n` 计数
- **手动数列确定列号（高频致命错误）**：禁止通过在 CSV 表头中手动数逗号/字段来确定目标列的列字母。当列数超过 10 时，手动计数极易产生 off-by-one 偏移（例如把 W 列误判为 X 列）。**必须使用 `col_indices`**：先在 CSV 表头中找到目标字段名是第 j 个字段（0-based），再用 `col_indices[j]` 获取该列的实际列字母
- **用数据列的值推导行号（高频致命错误，常被巧合掩盖）**：CSV 中常见"序号 / ID / 编号 / No."等形似行号的列，其值与实际表格行号**没有任何绑定关系**——序号可能跳号（1,2,3,5,6...）、可能从非 1 开始、可能有重复或被中途重置。此规则适用于**所有需要行号的下游操作**：合并单元格、区间写入/清空/格式化、插入/删除行、条件格式范围、筛选器范围、图表数据源、透视表范围、搜索替换范围等等——**凡是要把行号填进任何工具参数的场景，行号一律从 `annotated_csv` 中目标行开头的 `[row=N]` 前缀直接读取**，禁止用"序号=行号"、"表头占 1 行所以数据从第 2 行开始"、"第 N 个序号就在第 N+1 行"等心算，也禁止先心算再"事后核对"。**危险特征**：前几十行中序号恰好等于表格行号（典型成因：表头 +1 与一次跳号 -1 的偏移互相抵消形成巧合），模型一旦把这个巧合当作规律，会在后续所有行沿用；而中间再出现跳号时，从该行起整块区域全部错位，且错位不自查很难发现。**正确工作流**：①在 `annotated_csv` 里定位目标逻辑行（按字段内容匹配）；②直接读取该行开头的 `[row=N]` 前缀得到真实表格行号；③把这个行号填进下游工具参数。区间操作时，起始行用 start 行的 `[row=N]`、结束行用 end 行的 `[row=N]`。**自检**：动手前，在 `annotated_csv` 靠后位置再抽 1~2 行，核对 `[row=N]` 是否与首列"序号"一致——不一致（典型：`[row=57] 58,...`）即说明有跳号/隐藏行，更要严格从 `[row=N]` 取值，不要被序号列迷惑
- **按 `row_count` 盲读空行（高频低效）**：`+workbook-info` 的 `row_count` 是 sheet 的**网格物理行数**（常是 200 / 1000 等默认值），不是数据末行；按它把 `--range` 拉到 `S200`（实际数据可能只到 `S32`）会读回大片空行，浪费上下文又干扰判断。真实数据末行以 `+csv-get` 返回的 `current_region` 为准（它就是数据边界），再按下方「确定数据范围的正确流程」确认末行。
- **current_region 当作纯数据范围（高频致命错误）**：`current_region` 返回的是从请求范围向四周扩展到被空行空列包围的**连续非空区域**，等价于 Excel 的 Ctrl+Shift+\*。它包含该区域内**所有非空行**——不仅包含数据行，还可能包含标题行、汇总行（如"总计"）、签名行（如"编制人/审批人"）、脚注等非数据内容。**严禁直接将 `current_region` 的末尾行作为数据范围的结束行**。正确做法见下方「确定数据范围的正确流程」

### 确定数据范围的正确流程（排序、筛选、批量写入等操作前必做）

当后续操作需要精确的数据范围（如排序、筛选、删除、批量写入）时，仅靠 `current_region` 探测到的范围是不够的——必须同时确认数据的**起始行**和**结束行**。具体步骤：

1. **确认起始行**：读取前 5~10 行，识别表头行位置，数据起始行 = 表头行 + 1
2. **确认结束行**（关键步骤，不可跳过）：读取 `current_region` 末尾附近的若干行（建议读取末尾 5~10 行），逐行检查内容，排除非数据行：
   - **汇总行**：内容为"合计"、"总计"、"小计"、"总计:"等
   - **签名/审批行**：内容为"编制人"、"审核人"、"部门负责人"等
   - **空行或分隔行**：整行为空或仅有边框
   - **备注/脚注行**：注释性文字、说明文字等
3. **最终数据范围** = 起始行 ~ 最后一条有效数据行（排除非数据行）

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

## Flags

### `+cells-get`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | required | A1 范围，如 `A1:F10`（不带 sheet 前缀；用 `--sheet-id` / `--sheet-name` 指定 sheet） |
| `--include` | string_slice | optional | 要返回的信息类别，逗号分隔多个（可选值：`value` / `formula` / `style` / `comment` / `data_validation`） |
| `--max-chars` | int | optional | 防爆，默认 200000（隐藏 flag：不在 `--help` 列出，但可正常传入） |
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
| `--max-chars` | int | optional | 防爆，默认 200000（隐藏 flag：不在 `--help` 列出，但可正常传入） |
| `--include-row-prefix` | bool | optional | 是否在每行前加 `[row=N]` 前缀，默认 `true` |
| `--skip-hidden` | bool | optional | 跳过隐藏行列，默认 `false` |
| `--rows-json` | bool | optional | 返回结构化 rows（`{row_number, values:{列字母→值}}`）而非 CSV 文本，默认 `false` |

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
- `current_region` — 自动扩展到非空连续区域的 A1 范围。它是**真实数据边界**，**优先于 `+workbook-info` 的 `row_count`**（`row_count` 是网格物理行数，常是 200 / 1000 等默认值、远大于实际数据；按它盲读会拉回大片空行）
- `has_more` — 是否截断；截断后续读用 `--range` 接着读

**加 `--rows-json`：返回结构化 rows（而非 CSV 字符串）**

```bash
lark-cli sheets +csv-get --url "https://example.feishu.cn/sheets/shtXXX" --sheet-name "Sheet1" --range "A1:G20" --rows-json
```

`--rows-json` 下的输出契约（替换 `annotated_csv` / `col_indices` / `row_indices`）：

- `rows` — 数组，每元素 `{row_number, values}`。`row_number` 是真实表格行号（整数，下游需要行号的操作直接取它）；`values` 按**列字母** key（如 `values["D"]`，绝对列字母）。**所有逻辑行都在 `rows` 里**。引号内换行已解析进单元格值，无需自己按 RFC-4180 拆行。
- `data_not_fully_read` — **仅当没读全时出现**：`{read_through_row, data_extends_through_row, unread_rows, reread_range}`。出现即表示真实数据超出本次读取范围；批量写入前必须按 `reread_range` 重读全区，否则漏行。
- 其余字段（`current_region` / `actual_range` / `has_more`）同上。

### `+cells-get`

示例：

```bash
# 读 A1:F10 的公式 + 样式（sheet 定位必填）
lark-cli sheets +cells-get --url "https://example.feishu.cn/sheets/shtXXX" --sheet-name "Sheet1" \
  --range "A1:F10" --include formula,style
```

> ⚠️ 调用方在 `cells[i][j]` 中**不能**用下标推真实行列：必须读 `ranges[n].row_indices[i]` / `ranges[n].col_indices[j]`。

### Validate / DryRun / Execute 约束

- `Validate` 阶段只做 XOR 检查、Enum 合法性、防爆参数上限校验；**禁止**联网（如不能用 `--sheet-name` 提前去查 `sheet-id`）。
- `DryRun` 输出请求模板：`--sheet-name` 在 dry-run 输出里生成为 `<resolve:销售明细>` 占位符，不实际解析。
- `Execute` 阶段才进行 sheet-name → sheet-id 解析与 API 调用。
