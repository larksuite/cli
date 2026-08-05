# Lark Sheet Formula Verify（+formula-verify）

> **本文定位**：飞书表格"公式写入后是否真的零错误"的自检入口，也是所有写公式任务的**强制收尾步骤**。公式的书写规则与 Excel→飞书迁移的语义规则一律以 `lark-sheets-formula-translation` 为唯一权威，本文不重复；本文聚焦"写完了之后怎么用一次调用确认 zero-error"。
>
> **边界**：本文不讲公式怎么写（去 `lark-sheets-formula-translation`），也不讲公式怎么写入表格（去 `lark-sheets-write-cells` / `lark-sheets-batch-update`）。本文只讲一件事：**只要任务里发生了公式落表、批量填充公式、`--copy-to-range` 扩展公式、导入含公式 workbook，收尾就必须用 `+formula-verify` 自检到 zero-error 才能交付**。

## 为什么需要自检

飞书在线表格已经实时算好结果，但"算出来"和"算对了"是两件事。常见缺口：

- 公式编译失败 → 单元格落成文本（写入类 shortcut 返回的 `formula_errors[]` 是**编译失败**信号）。
- 公式编译成功但**运行时错误**：`#REF!` / `#DIV/0!` / `#VALUE!` / `#NAME?` / `#NULL!` / `#NUM!` / `#N/A`——这一类只看 `formula_errors[]` 看不到，必须扫单元格值。

`+formula-verify` 把两路信号合并成一份统一 JSON：一次调用聚合全表错误清单 + 编译失败清单 + 每类错误的定位与样本，AI 一眼就能定位修复，链路也能据 `status` 强制收敛到 `success`。

## 调用契约

最小调用形态：

| 入参 | 含义 |
|---|---|
| `--url` / `--spreadsheet-token` | 表格定位（XOR 二选一，必填） |
| `--sheet-id` / `--sheet-name` | 限定子表（mutually exclusive；省略则扫全部可见子表） |
| `--range` | 限定 A1 范围；省略则用各 sheet 的 `current_region` |
| `--max-locations` | 每类错误样本上限，默认 20 |
| `--exit-on-error` | `status='errors_found'` 时返回非 0 退出码（CI 网关用） |

返回核心字段：

- `status` ∈ `success` / `errors_found` / `partial`——**唯一可机读的健康度判据**。
- `total_errors` / `total_formulas` / `scanned_cells`——本次扫描规模指标。
- `has_more`——为 true 表示扫描被内部上限截断（详见后文「截断与续读」），未覆盖完整范围。
- `error_summary[<错误类型>]`——每类错误的 `count` / `locations[]` / `samples[].{address,formula,depends_on}`。
- `compile_errors[]`——合并最近一次写入留下的编译失败清单，与运行时错误并存时同时出现。
- `warning_message`——仅在 `has_more=true` 时出现，告知调用方需要缩小 `--range` / 拆 `--sheet-id` 续读。

## 写入收尾收敛规则

任何批量公式 / 含公式列写入完成后调用 `+formula-verify` 直到 `status='success'` 才能交付。不要等用户显式说"校验一下公式"才想到这里；**只要任务动作包含写公式，这一步默认就该做**。触发场景：

- `+cells-set` / `+csv-put`
- `+cells-set --copy-to-range` / 模板单元格向整列或整块扩展公式
- `+workbook-import`
- `+batch-update` 中含写入子操作
- `+table-put`（任意列含公式时）
- `+workbook-import`（导入的 xlsx 含公式时）

收敛规则：

1. `status='success'` → 通过；可以把链路标完成。
2. `status='partial'` → 扫描被内部上限截断。先缩小 `--range` 或拆 `--sheet-id` 续扫，**不允许**把 `partial` 当作 `success`。
3. `status='errors_found'` 且 `compile_errors[]` 非空 → **先解决编译失败**：根据 `compile_errors[].reason` 修正公式语法（飞书函数名 / 范围语法 / 引用样式），用 `+cells-set` 重写后再调一次 `+formula-verify`。
4. `status='errors_found'` 且只剩运行时错误 → 按 `error_summary` 的 `samples[].formula` + `depends_on` 排查根因（零除？空值参与运算？引用越界？日期差写法？数组语义？），修复后重新自检。
5. 同一处错误连续修复 3 次仍未通过 → 改用 `IFERROR` 包裹兜底，或退回纯值写入；不要在 `errors_found` 状态下扩展 `+cells-set --copy-to-range`、追加批量写入。

注意：

- 在 `status='errors_found'` 的状态下调用 `+cells-set --copy-to-range` 继续扩展会把错误复制放大。
- "编译失败但运行时无报错"不是 zero-error（编译失败的单元格此刻是文本不是公式，源数据一变就再也算不出值）。
- 跳过自检直接交付、靠肉眼读首末 5 行确认是不可靠的——表中段、隐藏行、合并区里的错误这样根本看不到。

## 截断与续读

后端有一个内部硬上限对总扫描单元格数做截断（不暴露给调用方），超过后立即返回 `has_more=true` + `warning_message`，`error_summary` / `compile_errors` 仅覆盖已扫描部分。处理路径：

- 把工作簿按 `--sheet-id` / `--sheet-name` 拆成多次调用。
- 同 sheet 内按 `--range` 切片（如先 `A1:Z200` 再 `AA1:AZ200`），逐块自检。
- 每块都跑到 `has_more=false` 且 `status='success'` 才算通过。

## 常见陷阱

| 坑 | 应对 |
|---|---|
| 错误字符串本地化 | 后端按内部 `error_kind` / `compute_status` 字段识别错误类别，不走字符串匹配；调用方拿到的 7 类英文错误代码由后端统一规范输出，与 locale 无关。 |
| `formatted_value` 可能隐藏错误 | 某些条件格式 / 自定义数字格式会把 `#DIV/0!` 显示成空白。后端直接读 cell `error_kind`，不依赖 `formatted_value`，绕开此类被遮蔽。 |
| 把 `partial` 当 `success` | `partial` 仅表示**已扫描部分**无错误，剩余区域未知。必须续扫直到 `has_more=false` 且 `status='success'` 才能算通过。 |
| 编译失败 vs 运行时错误 | 同一份报告里 `compile_errors[]` 与 `error_summary` 并存。语义层先解决 `compile_errors[]`、再做运行时自检。 |
