# Lark Sheet Formula Verify（+formula-verify）

> **本文定位**：飞书表格"公式写入后是否真的零错误"的诊断入口。公式的书写规则与 Excel→飞书迁移的语义规则一律以 `references/lark-sheets-formula-translation.md` 为唯一权威，本文不重复；本文聚焦"写完之后如何用一次调用发现公式错误"。
>
> **边界**：本文不讲公式怎么写（去 `references/lark-sheets-formula-translation.md`），也不讲公式怎么写入表格（去 `references/lark-sheets-write-cells.md` / `references/lark-sheets-batch-update.md`）。本文只讲一件事：任务里发生公式落表、批量填充公式、`--copy-to-range` 扩展公式、导入含公式 workbook 时，如何用 `+formula-verify` 做诊断并决定是否修复。

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

任何批量公式 / 含公式列写入完成后，都必须对本次新增 / 修改的公式范围逐段调用 `+formula-verify --exit-on-error`。不要等用户显式说"校验一下公式"才执行；只要任务动作包含写公式，这一步就是完成路径的一部分。触发场景：

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

## AI 公式异步检查

`=AI(prompt, [range])` 是异步计算，不能套用普通公式“立即零错误”的完成条件。写入 AI 公式后：

1. 用 `+formula-verify --ai-only --range <本次 AI 公式范围>` 检查状态；`--range` 只透传给后端，AI-only 汇总不保证按它收窄，认返回里的单元格定位而不是总数；
2. `failed` / `unsupported` 先修复；
3. 只有 `pending` 时可交付，但必须说明仍在后台计算；需要最终结果的任务继续轮询到 pending=0；
4. 普通公式不要带 `--ai-only`，否则会跳过 7 类 Excel 错误扫描。

## 截断与续读

后端有一个内部硬上限对总扫描单元格数做截断（不暴露给调用方），超过后立即返回 `has_more=true` + `warning_message`，`error_summary` / `compile_errors` 仅覆盖已扫描部分。处理路径：

- 关键输出区优先按 `--sheet-id` / `--sheet-name` 拆成多次调用。
- 同 sheet 内按 `--range` 切片（如先 `A1:Z200` 再 `AA1:AZ200`），逐块诊断。
- 续扫是完成条件的一部分：本次写入的公式范围必须全部拆分扫描到 `success`，不能因时间不足只在交付说明里列未覆盖范围就结束（同处置规则 2）。确实无法在本轮扫完时，按处置规则 5 对未验证公式降级为静态值并声明，而不是留下未验证的活公式。

## 常见陷阱

| 坑 | 应对 |
|---|---|
| 错误字符串本地化 | 后端按内部 `error_kind` / `compute_status` 字段识别错误类别，不走字符串匹配；调用方拿到的 7 类英文错误代码由后端统一规范输出，与 locale 无关。 |
| `formatted_value` 可能隐藏错误 | 某些条件格式 / 自定义数字格式会把 `#DIV/0!` 显示成空白。后端直接读 cell `error_kind`，不依赖 `formatted_value`，绕开此类被遮蔽。 |
| 把 `partial` 当全量健康 | `partial` 仅表示**已扫描部分**无错误，剩余区域未知；缩小 ranges 或按 sheet 拆分，直到本次普通公式范围全部 success。 |
| 编译失败 vs 运行时错误 | 同一份报告里 `compile_errors[]` 与 `error_summary` 并存。语义层先解决 `compile_errors[]`、再做运行时自检。 |
