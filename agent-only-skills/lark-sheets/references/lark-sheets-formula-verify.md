# Lark Sheet Formula Verify（+formula-verify）

> **本文定位**：飞书表格"公式写入后是否真的零错误"的诊断入口。公式的书写规则与 Excel→飞书迁移的语义规则一律以 `references/lark-sheets-formula-translation.md` 为唯一权威，本文不重复；本文聚焦"写完之后如何用一次调用发现公式错误"与 AI 公式的全区间一次异步状态检查交付。
>
> **边界**：本文不讲公式怎么写（去 `references/lark-sheets-formula-translation.md`），也不讲公式怎么写入表格（去 `references/lark-sheets-write-cells.md` / `references/lark-sheets-batch-update.md`）。本文只讲两件事：
>
> - **普通公式**：任务里发生公式落表、批量填充公式、`--copy-to-range` 扩展公式、导入含公式 workbook 时，对本次公式范围逐段跑 `+formula-verify --exit-on-error`；`errors_found` 修复、`partial` 拆分续扫，全部分段 `status='success'` 后才算完成。
> - **AI 公式**（`=AI(...)`）：不要用普通公式的"轮询到 zero-error"逻辑；改用 `+formula-verify --ai-only --range` 按「AI 公式校验」的全区间一次异步状态检查规则交付。

## 为什么需要自检

飞书表格已经实时算好结果，但"算出来"和"算对了"是两件事。常见缺口：

- 公式编译失败 → 单元格落成文本（写入类 shortcut 返回的 `formula_errors[]` 是**编译失败**信号）。
- 公式编译成功但**运行时错误**：`#REF!` / `#DIV/0!` / `#VALUE!` / `#NAME?` / `#NULL!` / `#NUM!` / `#N/A`——这一类只看 `formula_errors[]` 看不到，必须扫单元格值。

`+formula-verify` 把两路信号合并成一份统一 JSON：一次调用聚合公式错误清单 + 编译失败清单 + 每类错误的定位与样本，调用方可据此定位修复。任务只要发生公式落表，就把它作为公式错误码健康检查；限定本次新增 / 修改的公式范围逐段扫描并带 `--exit-on-error`。`status='success'` 仅表示无编译/运行时错误，不判断字段映射、阈值、单位、口径或业务结果是否正确——业务语义哨兵见 `references/lark-sheets-formula-translation.md`。

## 调用契约

最小调用形态：

| 入参 | 含义 |
|---|---|
| `--url` / `--spreadsheet-token` | 表格定位（XOR 二选一，必填） |
| `--sheet-id` / `--sheet-name` | 限定子表（mutually exclusive；省略则扫全部可见子表） |
| `--range` | 限定 A1 范围；省略则用各 sheet 的 `current_region` |
| `--max-locations` | 每类错误样本上限，默认 20 |
| `--exit-on-error` | `status='errors_found'` 时返回非 0 退出码；`partial` 仍需调用方检查 status 并拆分续扫 |
| `--ai-only` | 只检查 `=AI(...)` 异步计算状态；与普通公式 7 类错误扫描分开使用 |

返回核心字段：

- `status` ∈ `success` / `errors_found` / `partial`——**唯一可机读的健康度判据**。
- `total_errors` / `total_formulas` / `scanned_cells`——本次扫描规模指标。
- `has_more`——为 true 表示扫描被内部上限截断（详见后文「截断与续读」），未覆盖完整范围。
- `error_summary[<错误类型>]`——每类错误的 `count` / `locations[]` / `samples[].{address,formula,depends_on}`。
- `compile_errors[]`——合并最近一次写入留下的编译失败清单，与运行时错误并存时同时出现。
- `warning_message`——仅在 `has_more=true` 时出现，告知调用方需要缩小 `--range` / 拆 `--sheet-id` 续读。

## 写入后诊断规则

任何批量公式 / 含公式列写入完成后，都必须对本次新增 / 修改的公式范围逐段调用 `+formula-verify --exit-on-error`。不要等用户显式说"校验一下公式"才执行；只要任务动作包含写公式，这一步就是完成路径的一部分。AI 公式不套这条：`=AI(...)` 是异步计算，按「AI 公式校验」的全区间一次异步状态检查规则交付，不等 `status='success'`。触发场景：

- `+cells-set` / `+csv-put`
- `+cells-set --copy-to-range` / 模板单元格向整列或整块扩展公式
- `+workbook-import`
- `+batch-update` 中含写入子操作
- `+table-put`（任意列含公式时）
- `+workbook-import`（导入的 xlsx 含公式时）

处置规则：

1. `status='success'` → 当前分段无编译/运行时错误；但还必须按 `references/lark-sheets-formula-translation.md` 的业务语义契约核字段、阈值、单位、完整范围和业务哨兵。全部目标分段均为 success 且哨兵值正确后才完成。
2. `status='partial'` → 扫描被内部上限截断；缩小 `--range` 或拆 `--sheet-id` 续扫，未扫描区域仍未知，不能用交付说明代替验证。
3. `status='errors_found'` 且 `compile_errors[]` 非空 → 根据 `compile_errors[].reason` 修正公式语法（飞书函数名 / 范围语法 / 引用样式）；确实无法表达时才降级静态值，并说明原因与不联动风险。
4. `status='errors_found'` 且只剩运行时错误 → 按 `error_summary` 的 `samples[].formula` + `depends_on` 排查根因（零除？空值参与运算？引用越界？日期差写法？数组语义？），修复后重验。
5. 同一处错误连续修复 3 次仍未通过 → 可用 `IFERROR` 兜底或退回纯值，但降级后的目标格已不再是公式；需回读确认没有残留错误公式，并在交付说明写清不随源数据更新。

注意：

- 在 `status='errors_found'` 的状态下调用 `+cells-set --copy-to-range` 继续扩展会把错误复制放大，建议先处理关键错误。
- "编译失败但运行时无报错"不是 zero-error（编译失败的单元格此刻是文本不是公式，源数据一变就再也算不出值）。
- 只靠肉眼读首末 5 行确认不可靠——表中段、隐藏行、合并区里的错误这样根本看不到；`+formula-verify` 可补充这一诊断视角。
- 只验证写入区首行不够：批量填公式后同时抽查首行、中段、尾部和汇总行；目标是发现“只填到前 N 行”“把明细公式写进合计行”“尾部仍是空/错误值”这类问题。
- 修公式时先定位根因格，再看下游链路。不要把被上游错误污染的下游格全部重写；同型公式优先从相邻正确单元格复制/改引用，写完回读下游关键格是否仍有 `#VALUE!` / `#REF!`。
- 查找/匹配公式必须有错误处理：不要裸写 `VLOOKUP` / `XLOOKUP`。未匹配时返回明确文本（如“未匹配到”），不要静默空串，除非用户明确要求空值。
- 排名/排序公式要处理空值、0 值和不参与排名项；这些项应保持空/0，而不是进入通用排名公式得到正整数名次。

## 截断与续读

后端有一个内部硬上限对总扫描单元格数做截断（不暴露给调用方），超过后立即返回 `has_more=true` + `warning_message`，`error_summary` / `compile_errors` 仅覆盖已扫描部分。处理路径：

- 关键输出区优先按 `--sheet-id` / `--sheet-name` 拆成多次调用。
- 同 sheet 内按 `--range` 切片（如先 `A1:Z200` 再 `AA1:AZ200`），逐块诊断。
- 续扫是完成条件的一部分：本次写入的公式范围必须全部拆分扫描到 `success`，不能因时间不足只在交付说明里列未覆盖范围就结束（同处置规则 2）。确实无法在本轮扫完时，按处置规则 5 对未验证公式降级为静态值并声明，而不是留下未验证的活公式。

## AI 公式校验（`--ai-only`）

飞书表格提供一个统一的 **`AI` 公式**（`=AI(prompt, [range])`，用自然语言驱动翻译 / 分类 / 情感分析 / 信息提取 / 总结 / 润色等，写法与清单见 `references/lark-sheets-formula-translation.md`）。AI 公式的写入与普通公式一致（复用 `+cells-set` / `set_cell_range`，无需特殊接口），但**计算是异步的**：写入后要等 AI 算完才有结果。普通的 `+formula-verify` 只扫本地单元格值（7 类 Excel 错误），看不到 AI 公式的计算状态。

`--ai-only` 让 `+formula-verify` 只校验 AI 公式、跳过普通公式的 Excel 错误扫描，专用于写完 AI 公式后的异步状态检查。**它必须是第一校验入口；禁止先用 `+cells-get` / `+csv-get` 轮询 AI 结果。**

- **`--ai-only` 返回字段**（机读判据以这些为准，均为整数）：
  - `ai_formula_total`——后端返回的 AI 公式汇总计数，**不是本次写入的单元格条数**（同一批写入的多个 AI 公式可能只计为 1），`--range` 也不收窄它——**认返回里的单元格定位，不要拿它和本次预期条数做等值比对**。
  - `ai_formula_done`——已算出结果的条数。
  - `ai_formula_pending_count`——仍在后台计算（`pending`）的条数。
  - `ai_formula_failed_count`——失败 / 不支持的条数。
- **异步预期**：少量 AI 公式通常很快算出结果；批量写入后部分公式仍为 `pending`（计算中）属于正常现象，飞书会在后台持续计算。
- **`--exit-on-error` 兼容**：`--ai-only --exit-on-error` 时，若 `ai_formula_failed_count > 0`，返回非 0 退出码，便于脚本 / CI 收敛。
- 可与 `--sheet-id` / `--sheet-name` / `--range` 共存，表示「只在指定范围里校验 AI 公式」。
- **普通公式不要带 `--ai-only`**：带上会跳过 7 类 Excel 错误扫描，普通公式等于没验。

**`--range` 用整个写入区间，不要抽样**：`--ai-only` 是只读操作、成本低，`--range` 应覆盖本次写入的**全部** AI 公式区间（而非代表性子集）——子集抽检会漏掉「只有列尾那批被写坏」的情况。但别把 `--range` 当过滤器用：它只透传给后端，AI-only 汇总不保证按它收窄，失败项要按返回的单元格定位核对是否落在本次写入区间内。区间过大触发截断（`has_more=true`）时按「截断与续读」拆 `--range` / `--sheet-id`。

**必经步骤：一次性公式文本核对（不是轮询）**。写完 AI 公式后，先对种子格 / 首格做**一次** `+cells-get --include formula`，确认引号 / 括号没在 shell / CSV / JSON 层被破坏、单元格里落进去的确实是 `=AI(...)` 公式而非残缺字面量或 `#ERROR`。这一步只做一次、只看文本，被禁止的只是**用 `+cells-get` 反复轮询计算结果**（结果状态一律走 `--ai-only`）。

交付判据（机读）：全写入区间内 `ai_formula_failed_count == 0`；`failed` / `unsupported` 先修完再谈交付。满足后即使仍有 `ai_formula_pending_count > 0` 也可以交付，不必轮询到全部完成；交付时告知用户"AI 公式仍在后台运行，结果会陆续完成"。另外「公式在写入层被破坏、根本没算作 AI 公式」的静默失败不会体现为 `failed`，靠上面那次公式文本核对拦住——不要指望用 `ai_formula_total` 和预期条数对数（该总数未必按 `--range` 收窄）。

`ai_formula_failed_count > 0`，或文本核对暴露出 `#ERROR`、残缺括号（如 `E2)`）、半截函数名、全角括号时，说明公式串在引号层被破坏、没作为公式写进去——不要继续等 pending，回到 `+cells-set` 用 `\"` 转义重写该格（写入范例见 `references/lark-sheets-formula-translation.md` 的 AI 公式章节）。

典型用法：

```bash
# 写入一批 AI 公式后，对整个写入区间校验计算状态
lark-cli sheets +formula-verify --url <表URL> --sheet-name <子表名> --range <整个写入区间> --ai-only
# ai_formula_failed_count==0 即可交付；pending 会在后台继续计算
```

## 常见陷阱

| 坑 | 应对 |
|---|---|
| 错误字符串本地化 | 后端按内部 `error_kind` / `compute_status` 字段识别错误类别，不走字符串匹配；调用方拿到的 7 类英文错误代码由后端统一规范输出，与 locale 无关。 |
| `formatted_value` 可能隐藏错误 | 某些条件格式 / 自定义数字格式会把 `#DIV/0!` 显示成空白。后端直接读 cell `error_kind`，不依赖 `formatted_value`，绕开此类被遮蔽。 |
| 把 `partial` 当全量健康 | `partial` 仅表示**已扫描部分**无错误，剩余区域未知；缩小 ranges 或按 sheet 拆分，直到本次普通公式范围全部 success。 |
| 编译失败 vs 运行时错误 | 同一份报告里 `compile_errors[]` 与 `error_summary` 并存。语义层先解决 `compile_errors[]`、再做运行时自检。 |
