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
| 按列类型结构化读出（喂 DataFrame / round-trip 回 `+table-put`） | `+table-get` | 对话上下文 | 返回 typed 协议（`columns:[列名]` + `data` + `dtypes`/`formats` + `range`），输出形状对齐 pandas split；可一行 `pd.DataFrame(sheet["data"], columns=sheet["columns"]).astype(sheet["dtypes"])` 还原 DataFrame，或直接 round-trip 回 `+table-put`。不带 `--range` 时读**完整 used range**（跨过表中部空行 / 空列），每个子表回传读取范围 `range`；被 `max_chars` 裁掉时**该子表**带 `truncated: true` 与 `truncation_warning`，预算耗尽导致后续整表未读时**顶层**也带同组字段，`--output-path` 落盘模式另看 stdout 回执的 `complete` / `truncated`——**先看截断字段再用数据；三层都没报也不等于逻辑读全**，仍要用返回数据实际行数、关键末行与源数据交叉核对（详见下文）。注意这与下文 `current_region` "遇表中部空行截断"不矛盾：`+table-get` 读的是子表物理 used range（飞书记录的已用矩形，含中间空行），`current_region` 是从锚点连通扩展、遇整行空行就断 |
| 查看公式、样式、批注、数据验证 | `+cells-get` | 对话上下文 | 返回单元格完整信息，token 开销较大 |
| 查看某区域的下拉框（数据验证）选项 | `+dropdown-get` | 对话上下文 | 返回该 A1 范围已配置的下拉列表选项 |

**选择原则**：
- 只看值或做数据处理 → `+csv-get`；大表分批读取，避免一次拉全表撑爆上下文
- 要按列类型结构化读出（喂 DataFrame / round-trip 回 `+table-put`）→ `+table-get`
- 需要公式/样式/批注 → `+cells-get`
- 只想知道某区域下拉框有哪些选项 → `+dropdown-get`

## 读表理解脚本（Agent 优先入口）

当目标是"先理解表格内容 / 结构 / 子表边界"，且本地存在 `scripts/lark_*.py`（只随仓库版 skill 分发，二进制内嵌版不含 `scripts/`），可优先用这组只读脚本，再决定是否直接调用上述 shortcut。脚本是可选捷径，不是必经入口——脚本不可用时直接按下表右列的 CLI 等价路径执行：如果任务很小，或需要公式 / 样式 / 批注 / 精确原始值等脚本未覆盖的信息，可以直接用 CLI 做等价或更精细读取。

| 脚本 | 底层 shortcut | 适用场景 |
| --- | --- | --- |
| `scripts/lark_inspect_workbook.py` | `+workbook-info` / `+sheet-info` / `+csv-get` | 在线表格第一步预检：拿 sheet 清单、布局、预览、`current_region` |
| `scripts/lark_detect_subtables.py` | `+workbook-info` / `+sheet-info --include merges,hidden_rows,hidden_cols` / 小窗口 `+csv-get` | 同一 sheet 可能有多个表格区域、汇总块、备注块时，在**已知且未截断的窗口**内识别候选子表 range |
| `scripts/lark_profile_table.py` | `+csv-get` / `+sheet-info --include hidden_rows,hidden_cols`（默认包含隐藏行列时；必要时再手工 `+cells-get` / `+table-get`） | 对**已确认且未截断的候选 range**做表头、数据范围、列类型、特殊行画像，并输出 `summary` / `field_map` / `risk_warnings` / `write_hints` |

`lark_profile_table.py` 是**启发式画像**，不是最终判定器：它能降低手工数行列和漏看特殊行的风险，但表头、多行标题、数据末行、列类型、特殊行和追加列都可能需要二次确认。批量写入、公式、排序、筛选、去重、透视/图表等操作前，不能只凭 profile 结果直接写；必须把 profile 输出与任务语义、样本值、必要的 CLI 补读一起核对。

`lark_profile_table.py` 的使用口径：

| 任务类型 | 建议 |
| --- | --- |
| 只读取或修改用户明确指定的单个单元格 / 很小范围，且不需要理解整表 | 可直接用 CLI |
| 批量写入、公式 / 计算、排序、筛选、删除、仅保留、去重、lookup / 匹配、条件高亮、透视表、图表、汇总 | 优先对目标区域运行 `lark_profile_table.py`；若已用等价 CLI 明确确认表头、数据范围、字段列、列类型和特殊行，可跳过脚本。去重 / lookup 若目标列含 `long_numeric_like_id`、前导 0 或格式化数字，profile 只能定位列，比较值必须改用 `+cells-get` 或 `+table-get` |
| 多块表、表头不确定、存在合并 / 汇总 / 空行 / 备注块、选区是单格但任务语义是整表 | 先 `lark_detect_subtables.py` 或补充 CLI 确认候选范围，再对目标 range 跑 `lark_profile_table.py` |
| 需要公式、样式、批注、数据验证、精确原始值、长数字 ID 精确比较 | 先用脚本形成结构化理解，再按需补 `+cells-get` / `+table-get` / 分批 `+csv-get` |

推荐链路（大表先定窗口，脚本不接受截断结果）：

```bash
python scripts/lark_inspect_workbook.py --url "<表格URL>"
# 先用 +workbook-info 和小窗口 +csv-get 确认真实 sheet、列边界和起始区域；大表按行窗口推进。
python scripts/lark_detect_subtables.py --url "<表格URL>" --sheet-name "<子表名>" --range "A1:H200"
python scripts/lark_profile_table.py --url "<表格URL>" --sheet-name "<子表名>" --range "A1:H200"
```

`lark_detect_subtables.py` / `lark_profile_table.py` 的 `+csv-get` 命中 `has_more` 会以错误退出并报告已读取的 `actual_range`，绝不基于半截数据给出候选范围或画像。遇到此错误，以 `actual_range` 为已完成窗口，缩小列数或从其末行之后继续读；跨窗口的候选范围、汇总行和写入落点必须再用 CLI 核对，不能把单个窗口结果当整表结论。

脚本只读，不做任何写入。它们的输出用于降低 token 和定位错误；后续需要公式、样式、批注、精确原始值时，仍按本文件规则直接调用 `+cells-get` / `+table-get` / `+csv-get`。写入前如果使用了 `lark_profile_table.py`，至少读取并使用这些字段：`summary.header_row`、`summary.data_range`、`summary.data_row_segments`、`field_map`、`risk_warnings`、`visibility`、`write_hints.safe_append_col` 和 `special_rows`。仅当 `risk_warnings` 不含 `data_range_has_gaps` 时，才可把 `data_range` 当连续写入范围；有缺口时按 `data_row_segments` 分段读写。

脚本关键 flag：

| Flag | 脚本 / 默认 | 何时调整 |
| --- | --- | --- |
| `--skip-hidden` | profile / detect，关闭（默认包含隐藏行列） | 只分析可见数据时开启；此时必须使用 profile 的 `data_row_segments`，不要把连续 `data_range` 直接用于写入。 |
| `--max-chars` | inspect `8000`；profile / detect `25000` | 输出过大时缩小范围或降低值；profile / detect 若截断会报错并给 `actual_range`，按窗口继续。 |
| `--header-scan-rows` | profile `20` | 表头前有多行标题、说明或空行时提高；过大时结合 `possible_multi_row_header` 补读确认，不要仅凭评分结果写入。 |
| `--max-sheets` | inspect `3` | 未指定 sheet 时仅前 N 个 sheet 带 layout / preview，其余仍返回摘要并在 warnings 说明。 |
| `--max-merge-components` | detect `2000` | 超限会跳过 gap 合并并告警；需缩小窗口或人工复核子表边界。 |
| `--gap-rows` / `--gap-cols` | detect `1` / `0` | 子表被切碎或粘连时调整；每次调整后复核候选范围。 |

detect 最多确认 10 个跨窗口合并锚点；超限会在 `warnings` 中说明跳过的数量。遇到该 warning，缩小扫描窗口后再复核受影响的子表边界。

`lark_profile_table.py` 输出触发补读的规则：

- `risk_warnings` 非空时，不要把画像当最终事实；按下表补读或调整，不在表内的 warning 也先保守复核。

| Warning | 必做动作 |
| --- | --- |
| `mixed_value_types` / `long_numeric_like_id` / `formula_or_value_errors` | 补 `+cells-get` 或 `+table-get`，确认原始值、类型和公式。 |
| `duplicate_headers` / `unnamed_columns` / `header_not_detected` / `header_row_not_first` / `many_empty_cells` | 补 `+csv-get` 读取表头附近和空值样本，确认真正表头与字段列。 |
| `data_range_not_detected` / `special_rows_present` / `empty_rows_present` | 补 `+csv-get` 读取尾部和特殊行样本，确认有效数据末行。 |
| `possible_multi_row_header` | 补读表头上下各 1-2 行；必要时 `+sheet-info --include merges` 核对跨列合并。 |
| `hidden_rows_in_range` / `hidden_columns_in_range` | 写入前用 `+sheet-info --include hidden_rows,hidden_cols` 确认是覆盖还是跳过隐藏内容。 |
| `data_range_has_gaps` | 不按连续 `data_range` 写；用 `summary.data_row_segments` 对每个实际读取行段单独读写。 |
| `data_range_has_col_gaps` | 返回的列不连续（`--skip-hidden` 跳过了隐藏列）；不要把 `data_range` 当连续列区写回，按 `summary.data_col_segments` 分列段处理，否则缺口右侧的值会整体错位。 |

- `write_hints.safe_append_col` 只是候选追加列，不代表绝对安全。新增列或覆盖区域前，必须用 `+csv-get` / `+cells-get` / `+sheet-info` 核对该列为空、没有隐藏列/公式/样式/对象依赖，且符合用户要求的落点。该字段已自动跳过隐藏列（跳过的列名列在 `write_hints.skipped_hidden_cols`）——注意 `--skip-hidden` 下隐藏列根本不出现在返回网格里，若它们正好都贴在数据右边缘，`data_range_has_col_gaps` 也不会告警，所以这层跳过是唯一的保护，别绕过它自己按「最后一列 +1」推落点。

⚠️ **大数据优先落盘、别灌进上下文**：`+csv-get` / `+cells-get` 都受调用方 Bash / 终端的单命令 stdout 输出上限约束（常见默认约 30000 字符，超过会被截断或转存为文件）。纯值分析优先用 `+csv-get` 按 `--range` 行窗口（`A1:Z500` / `A501:Z1000` …）分批重定向到文件 + 本地脚本处理 + `+csv-put` 分批回写；若确实要让结果直接进上下文又不想触发转存，给任一命令把 `--max-chars`（默认 500000）调小到略低于该上限（如 `25000`），CLI 改为优雅截断 + `has_more` 分页。

> **落盘不等于读全**：`--output-path` 只是把上限从 stdout 口径放宽到有界的 2000 万字符（读取链路非流式，该上限是内存保护），不是无限。stdout 回执带 `complete` 字段——`complete:false` 时另有 `truncated` 与提示，文件里只有半截数据；多子表读取还会给 `unread_sheets` 列出预算耗尽前没读到的子表。**拿到回执先看 `complete`，不要默认整表已落全。**

**`+csv-get` 返回值核心设计**：
- `annotated_csv` — **CSV 数据唯一入口**。每一逻辑行前加 `[row=N] ` 前缀（N = 真实表格行号）。任何需要行号的下游操作（合并、写入、清空、格式化、插入/删除、条件格式、筛选、图表/透视表范围、搜索替换等），**行号一律直接从 `[row=N]` 读取**。若需要纯 CSV（如喂给本地脚本做解析），去前缀即可：`line.replace(/^\[row=\d+\] /, '')`。
- `col_indices` — **定位列字母唯一入口**。在表头中找到目标字段是第 j 个（0-based），用 `col_indices[j]` 取列字母。**禁止手数逗号**——列数超过 10 时极易 off-by-one（例如把 W 误判为 X）。
- `row_indices` — 程序化引用的备用数组。LLM 推理请用 `annotated_csv` 的前缀，不要查这个数组里的 index（把行号当数值用容易心算出错）。
- `current_region` — 从请求范围扩展到被空行空列包围的连续数据区域（等价于 Excel Ctrl+Shift+*），适合先读少量行探表头。⚠️ 它**遇表中部整行空行 / 整列空列就截断**，可能小于真实数据范围（漏掉空行之后的行）；**不能**直接当整表末行用，判断整表是否读全要拿 `+workbook-info` 的物理 `row_count` / `column_count` 当上界交叉核对（见下方「按 row_count 盲读空行」与「确定数据范围的正确流程」）。

注意：

- `+csv-get` 和 `+cells-get` 支持分页/截断，注意检查 `has_more` / `truncated` 标志；两者在处理返回数据之前都必须先读 `warning_message`（上游 schema 要求先读它再用其它字段，内含定位与截断续读提示），`+cells-get` 还要用每个 range 的 `actual_range` / `row_indices` / `col_indices` 判断真实位置
- 隐藏行列默认包含在返回结果中（`--skip-hidden=false`），如需只看可见数据设为 `true`。读取原语本身不标注哪些行列被隐藏：若要识别隐藏区间（以决定是否过滤、或如何解读混入的隐藏数据），用 `+sheet-info --include hidden_rows,hidden_cols` 取隐藏行列集合，再结合 `+csv-get` / `+cells-get` 返回的 `row_indices` / `col_indices` 判断每行 / 每列是否隐藏
- 要判断单元格内容是否被行高列宽挤到显示不全（排版检查、调整行高列宽前），给 `+cells-get` 加 `--include truncation`：会按字号 / 自动换行 / 行高列宽估算并返回被截断单元格的 `isRowTruncated` / `isColTruncated`（未返回视为未截断）。有额外计算开销，仅需要时才开

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
| `--include` | string_slice | optional | 要返回的信息类别，逗号分隔多个。`truncation` 会额外按行高列宽 / 字号 / 自动换行估算每个单元格是否被截断显示，返回 `isRowTruncated` / `isColTruncated`（有额外计算开销，仅排版检查 / 调整行高列宽前才开）（可选值：`value` / `formula` / `style` / `comment` / `data_validation` / `conditional_format` / `truncation`） |
| `--max-chars` | int | optional | 单次返回字符上限，默认 500000（兜底防爆）。要整表无截断直接用 --output-path 落盘（上限自动放宽到 2000 万字符——读取链路非流式，此上限是内存保护；更大就显式给 --max-chars）；仅当要让结果直接进上下文、又不落盘时才调小（如 25000），按 has_more 分页。 传 0 表示「不自设上限」，等价于不传（仍是 500000 / 落盘时 2000 万），不会退回底层工具那个更小的默认截断。 |
| `--output-path` | string | optional | 把完整读取结果写入本地路径（如 `./out.json`），文件内容为 data 载荷的 JSON；stdout 只回一个含 output_path/字节数的确认信息。**一旦设置，字符上限自动放宽到有界的 2000 万字符**（覆盖 --max-chars 默认），并非无限——读取链路非流式，该上限是内存保护；显式 --max-chars 优先。stdout 回执带 `complete` 字段（命中上限时另有 `truncated` 与提示），据此判断文件是否完整，不要默认整表已落全。省略时按常规把结果打到 stdout。 |
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
| `--range` | string | optional | A1 范围，如 `A1:F30`（不带 sheet 前缀；用 `--sheet-id` / `--sheet-name` 指定 sheet）。**可省略：缺省读取整个子表**（按表格实际边界裁剪，返回的 actual_range 标注实际读取范围）；大表配合 --max-chars / --output-path 控制体量 |
| `--max-chars` | int | optional | 单次返回字符上限，默认 500000（兜底防爆）。要整表无截断直接用 --output-path 落盘（上限自动放宽到 2000 万字符——读取链路非流式，此上限是内存保护；更大就显式给 --max-chars）；仅当要让结果直接进上下文、又不落盘时才调小（如 25000），按 has_more 分页。 传 0 表示「不自设上限」，等价于不传（仍是 500000 / 落盘时 2000 万），不会退回底层工具那个更小的默认截断。 |
| `--output-path` | string | optional | 把完整读取结果写入本地路径（如 `./out.json`），文件内容为 data 载荷的 JSON；stdout 只回一个含 output_path/字节数的确认信息。**一旦设置，字符上限自动放宽到有界的 2000 万字符**（覆盖 --max-chars 默认），并非无限——读取链路非流式，该上限是内存保护；显式 --max-chars 优先。stdout 回执带 `complete` 字段（命中上限时另有 `truncated` 与提示），据此判断文件是否完整，不要默认整表已落全。⚠️ 落盘的是 data 载荷的 **JSON**（`+csv-get` 也一样，CSV 文本是 JSON 里的一个字段），不是直接可用的 .csv 文件；要纯 CSV 文件请把 stdout 重定向到文件。 省略时按常规把结果打到 stdout。 |
| `--include-row-prefix` | bool | optional | 是否在每行前加 `[row=N]` 前缀，默认 `true` |
| `--skip-hidden` | bool | optional | 跳过隐藏行列，默认 `false` |

### `+table-get`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--sheet-id` | string | optional | 只读该子表（按 id）；省略则读所有子表 |
| `--sheet-name` | string | optional | 只读该子表（按名）；省略则读所有子表 |
| `--range` | string | optional | 读取的 A1 范围；省略则读每个子表的完整 used range（会跨过表中部的整行空行 / 整列空列，不会被截断） |
| `--max-chars` | int | optional | 单次返回字符上限，默认 500000（兜底防爆）。底层工具即使不传也有约 50000 的默认截断，故此处显式发送以放宽；要整表读取请用 --output-path 落盘（上限自动放宽到有界的 2000 万字符，非无限；回执 complete 字段说明是否完整）。 传 0 表示「不自设上限」，等价于不传（仍是 500000 / 落盘时 2000 万），不会退回底层工具那个更小的默认截断。 |
| `--output-path` | string | optional | 把完整读取结果写入本地路径（如 `./out.json`），文件内容为 data 载荷的 JSON；stdout 只回一个含 output_path/字节数的确认信息。**一旦设置，字符上限自动放宽到有界的 2000 万字符**（覆盖 --max-chars 默认），并非无限——读取链路非流式，该上限是内存保护；显式 --max-chars 优先。stdout 回执带 `complete` 字段（命中上限时另有 `truncated` 与提示），据此判断文件是否完整，不要默认整表已落全。省略时按常规把结果打到 stdout。 |
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

# 全量读：省略 --range 即读整个子表（按实际边界裁剪，返回 actual_range 标注实读范围），
# 无需先 +workbook-info 探行列再拼 range；大表配合 --max-chars / --output-path
lark-cli sheets +csv-get --spreadsheet-token shtXXX --sheet-name "销售明细"
```

输出契约（envelope.data）：

- `annotated_csv` — 含 `[row=N]` 前缀的 CSV 主入口
- `col_indices` / `row_indices` — 列字母 / 行号映射数组
- `current_region` — 从锚点扩展到被空行空列包围的连续区域的 A1 范围。⚠️ **它不是整表真实边界**：遇表中部整行空行 / 整列空列会截断、可能小于真实数据范围；表尾的汇总 / 签名 / 脚注又可能让它大于纯数据范围。判断整表是否读全须拿 `+workbook-info` 的物理 `row_count` 当上界交叉核对（见上方「`row_count` 与 `current_region` 都不能单独定末行」）
- `actual_range` — **本次实际读到的 A1 范围**。续读 / 校验覆盖度一律以它为准：`actual_range` 小于请求范围时，哪怕 `has_more=false` 也说明只拿到部分窗口，不能把 `row_count` 当成"已读全"
- `row_count` / `col_count` — **本次返回的行 / 列数**（= `actual_range` 的尺寸，随 `--range` 变），**不是整表物理总行列数**；整表物理尺寸取 `+workbook-info`
- `has_more` — 当前 `--range` 是否因 `--max-chars` 被截断（截断后续读接着用 `--range`）；它**只反映本次 range 内是否还有后续页**，`has_more=false` **不代表整表或该窗口已读全**——仍要结合 `actual_range` 看实际覆盖到哪里

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

**默认（不带 `--range`）读取整张子表的完整 used range**：会跨过表中部的整行空行 / 整列空列，覆盖到真实数据边界。每个子表都回传实际读取的 `range`（如 `A1:F10`）。**截断信号分三层**：① 子表数据被 `max_chars` 裁掉时，该子表带 `truncated: true` + `truncation_warning`；② 字符预算耗尽导致后续整表一行未读时，**顶层**也带同组字段（按提示改用 `--sheet-name` 单表重跑或提高 `--max-chars`）；③ `--output-path` 落盘模式以 stdout 回执的 `complete`（命中上限时另有 `truncated`）判断文件完整。**任何一层都没报截断，也不等于逻辑读全**——used range 探测在特殊布局（大段整空行 / 空列）下可能偏窄：拿 `range` 连同返回 `data` 的实际行数、关键末行 / 末日期，与源数据行列数（`+workbook-info` / 源 xlsx）交叉核对，确认覆盖真实边界。仍要精确控制范围时显式传 `--range`；分段续读时配 `--no-header`，表头行与各段 dtypes 需自行拼接对齐。

列类型从每列 `number_format` 推断（日期格式→`date`/`datetime64[ns]`、数值→`number`/`float64`、bool→`bool`），`date` 列的序列号转回 ISO `yyyy-mm-dd`——日期、数字往返不丢类型。**列类型只在该列所有非空值一致时才定（`number` / `date` / `bool`）；一列混了类型（如数字列混入「暂无」、日期列混入裸数字）会降为 `string`（dtypes 输出 `object`），让 `dtypes` 与 `data` 里每个值自洽——能 round-trip 回 `+table-put`、不让 pandas `astype` 崩。降级是无损的（脏值原样保留为文本）；若要把零星脏值转成数值列，交给调用方在 pandas 侧做（`to_numeric(errors='coerce')`），那里原始值仍在、可追溯。** 默认读所有子表、第一行当表头（`--no-header` 把首行当数据、列名取 `col1` / `col2` …）。

```bash
# 默认读所有子表 → sheets[]（与 +table-put 的 --sheets 同构，可喂回或转 DataFrame）
lark-cli sheets +table-get --url "<表URL>"
# 可选：--sheet-name / --sheet-id 限定只读某一个子表（不给则读全部）
lark-cli sheets +table-get --url "<表URL>" --sheet-name "销售"
```

#### 输出 → DataFrame（用 `sheet_to_df` helper）

输出形状对齐 pandas split：`columns` 是列名数组、`data` 是二维数据、`dtypes` 是 `{列名: pandas_dtype_str}` 映射。直接喂给 `pd.DataFrame(...).astype(...)` 就能一次性还原所有列类型（不必逐列 `to_datetime` / `to_numeric`）。本 skill 把这段 2 行 helper 打包成可 import 的 [`scripts/sheets_df.py`](../scripts/sheets_df.py)（含 `df_to_sheet` 和 `sheet_to_df`，写入 / 读回成对；它在本 skill 的 `scripts/` 目录下，运行目录不在该目录时先把它加入 `sys.path` 再 import）：

```python
import sys; sys.path.insert(0, "scripts")  # helper 在 skill 根的 scripts/ 下；cwd 不在 skill 根时填该目录的实际路径
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
import sys; sys.path.insert(0, "scripts")  # 同上：sheets_df 在 skill 的 scripts/ 目录
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
