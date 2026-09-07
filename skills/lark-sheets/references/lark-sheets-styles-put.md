# Lark Sheet Styles Put（+styles-put）

> **本文定位**：对**已有**表格做美化收尾的默认入口——样式 / 边框 / 合并 / 行高列宽 / 冻结写成一份声明式规格，一次调用交付。样式**取什么值**（配色 / 字号 / 对齐 / 数字格式标准）以 `lark-sheets-visual-standards` 为唯一权威，本文只讲**怎么落地**。
>
> **边界（三分流判定，按操作组合选入口）**：目标是**样式 / 合并 / 行高列宽 / 冻结**的任意组合 → 本命令；**同一个写操作**打多个区域（如多区域清除、批量下拉）→ 用该命令自身的复数形态（`--ranges` / map 入参）；操作链**跨类型且有顺序依赖**（如插列 → 写表头 → 回填数据）→ `+batch-update`。美化收尾不需要也不应该拼 `--operations` 子操作数组。

## 使用场景

写入。对存量表格的多个子表批量应用视觉规格：新表美化、加汇总行后统一版式、按分组合并同类单元格、调列宽行高、冻结表头。整份规格展开为一次批量提交按序执行，与 `+batch-update` 同为 **fail-fast**——失败后哪些子操作已生效不做统一假设，先回读确认再补发（语义同 `lark-sheets-batch-update`「执行语义」）。

⚠️ **失败后不要照抄报错里的 `operations[N]` 去续发**：那个数组是 CLI 从 `--styles` 展开出来的（相邻同样式的 `cell_styles` 还会被合并成更大的矩形），下标与你写的 spec 项没有对应关系，也不是你能直接重发的东西。正确做法：回读受影响区域（`+cells-get --include style` / `+sheet-info`）确认哪些已生效，再重发没落上的部分。样式 / 行高列宽 / 冻结是幂等盖章（整份重发无副作用，这通常就是最省事的解法），只有 `cell_merges` 需要挑出未生效的部分单独发。

**词汇三处同构**：`--styles` 的字段词汇与 `+workbook-create --styles`（建新表同步美化）、`+table-put --styles`（写数据同步美化）完全一致——`cell_styles` / `cell_merges` / `row_sizes` / `col_sizes` / `freeze` 学一次三处通用。区别只有两点：本命令作用于**已有**表格（顶层 `--url` / `--spreadsheet-token` 定位），且 `cell_styles` 的 range 不受「本次写入区域」限制、可指向表内任意区域。

**规格要点**：

- 顶层 `{styles:[...]}`，每项对应一个目标子表，`name` 必须是真实子表名（不确定先 `+workbook-info` 查，禁止猜 `Sheet1`）。
- 每个子表项按固定顺序执行：`cell_merges` → `cell_styles` → `row_sizes` → `col_sizes` → `freeze`；样式盖章允许覆盖含合并区的区域（合并区限制只针对值写入，样式不受限）。
- `row_sizes` / `col_sizes` 只需 `{range, size}`（px，即像素尺寸；`standard` / 行的 `auto` 才需显式 `type`）。尺寸键统一是 `size`。
- 加边框用 `border` 简写：`{"style":"solid","color":"#DDDDDD"}` 应用到四边；只有分侧不同样式才用 `border_styles` 完整形态。
- `freeze` 用 `{rows:N, cols:N}` 冻结前 N 行 / 列，0 或省略表示该维度不冻结；freeze 是整份状态覆盖，全 0（如 `{"rows":0}`）= 两轴全部解冻，与 `+dim-freeze --rows 0 --cols 0` 等价（仅 `+workbook-create` 建新表时全 0 无意义、会被校验拒绝）。

**回读校验**：整份规格执行成功后按编辑准则抽样回读受影响区域（`+cells-get --include style` 或 `+sheet-info` 看合并 / 行高列宽 / 冻结），确认关键样式实际生效。

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+styles-put` | write | 批量 |

## Flags

### `+styles-put`

_公共：URL/token（无 sheet 定位） · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--styles` | string + File + Stdin（复合 JSON） | required | 对**已有**表格应用的视觉规格 JSON：顶层 `{styles:[...]}`，每项对应一个目标子表（`name` 用真实子表名），并至少给 `cell_styles` / `cell_merges` / `row_sizes` / `col_sizes` / `freeze` 之一。字段词汇与 `+workbook-create` / `+table-put` 的 `--styles` 完全同构（cell_styles 用 A1 range + 扁平样式字段，边框用 `border` 简写 {style,weight,color} 四边同款、分侧才用 border_styles；row/col sizes 用行/列范围 + size（px 即像素，standard/auto 才需 type）；merges 用单元格 range；freeze 用 `{rows:N, cols:N}` 冻结前 N 行/列）。整份规格展开为一次批量提交（fail-fast：失败后哪些已生效不做统一假设，先回读确认再补发）；range 不受「本次写入区域」限制，可指向表内任意区域 |

## Schemas

> 复合 JSON flag 字段速查（只列顶层 + 一层嵌套）。深层结构看下方 `## Examples`，或用 `--print-schema` 读完整 JSON Schema（用法见 SKILL.md「公共 flag 速查」与「Agent 使用提示」）。

### `+styles-put` `--styles`


**数组项**（类型 object）：
- `cell_merges` (array<object>?) — 单元格合并操作数组；range 使用 A1 单元格范围，merge_type 默认 all each: { merge_type?: enum, range: string }
- `cell_styles` (array<object>?) — 单元格样式操作数组；每项用 A1 单元格 range 指定范围，字段名与 +cells-set-style 对齐 each: { background_color?: string, border?: object, border_styles?: object, font_color?: string, font_family?: string, …共 14 项 }
- `col_sizes` (array<object>?) — 列宽操作数组；range 使用列范围如 A:C，给 size（px）即像素列宽（type 可省略）；type 为 standard 时不带 size each: { range: string, size?: number, type?: enum }
- `freeze` (object?) — 冻结行列：rows = 冻结前 N 行，cols = 冻结前 N 列（0 或省略 = 该维度不冻结） { cols?: integer, rows?: integer }
- `name` (string) — 子表名
- `row_sizes` (array<object>?) — 行高操作数组；range 使用行范围如 1:3，给 size（px）即像素行高（type 可省略）；type 为 standard/auto 时不带 size each: { range: string, size?: number, type?: enum }

## Examples

### `+styles-put`

表头美化 + 按组合并 + 列宽 + 冻结首行，一次交付：

```bash
lark-cli sheets +styles-put --url "https://example.feishu.cn/sheets/shtXXX" --styles - <<'JSON'
{"styles":[{
  "name": "Sheet1",
  "cell_merges": [{"range":"A5:A8"},{"range":"A9:A12"}],
  "cell_styles": [
    {"range":"A1:F1","font_weight":"bold","background_color":"#1E5BC6","font_color":"#FFFFFF","horizontal_alignment":"center"},
    {"range":"A2:F30","border":{"style":"solid","color":"#DDDDDD"}}
  ],
  "row_sizes":  [{"range":"1:1","size":36}],
  "col_sizes":  [{"range":"A:C","size":120}],
  "freeze":     {"rows":1}
}]}
JSON
```

多子表同一批交付（每个子表一个 styles 项）：

```bash
lark-cli sheets +styles-put --url "..." --styles - <<'JSON'
{"styles":[
  {"name":"明细","cell_styles":[{"range":"A1:H1","font_weight":"bold","background_color":"#F0F0F0"}],"freeze":{"rows":1}},
  {"name":"汇总","cell_styles":[{"range":"A1:D1","font_weight":"bold"}],"col_sizes":[{"range":"A:D","type":"pixel","size":140}]}
]}
JSON
```

### Validate / DryRun / Execute 约束

- `Validate`：`--styles` 必须是合法 JSON、`styles` 非空数组；每项 `name` 必填、至少给 `cell_merges` / `cell_styles` / `row_sizes` / `col_sizes` / `freeze` 之一；`cell_styles` 每项至少一个样式字段；展开后受子操作数（100）与总格数预算约束，超限报错给拆分建议。
- `DryRun`：输出展开后每个子操作的请求模板，不发起调用。
- `Execute`：整份规格合成一次批量请求按序执行；fail-fast。报错会列出失败的子操作及原因，但其中的 `operations[N]` 是 CLI 展开后的内部下标（含 `cell_styles` 合并），不对应 `--styles` 里的项，也不能直接按下标续发——报错会明说这一点并让你先回读再补发。
