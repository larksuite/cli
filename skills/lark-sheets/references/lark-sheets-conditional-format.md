# Lark Sheet Conditional Format

## 真对象硬约束 + 触发词清单

用户出现以下口语指令时，**强制**走 `+cond-format-{create|update|delete}`，**禁止**用 `+cells-set` 写静态背景色 / 字体色代替：

- **颜色动作**："标红 / 标黄 / 标绿 / 上色 / 染色 / 涂色 / 表红色 / 表黄色"
- **视觉强调**："高亮 / 突出 / 标记 / 标注 / 区分"——**限带条件语义的**（按值 / 规则决定哪些格上色）；纯装饰性无条件上色（斑马纹、整行固定底色）不在强制范围，按视觉规范直接设背景色
- **条件触发**："重复的标出来 / 异常的圈出来 / 过期的染红 / 大于 X 的标黄 / 不达标的标红"
- **联动语义**："颜色随数据变 / 联动 / 自动更新 / 改了数据颜色也跟着变"
- **数值可视化**："数据条 / 色阶 / 渐变色 / 进度条样式"

飞书表格的"颜色标记"语义 = 条件格式规则 ≠ 静态背景色。如果用 `+cells-set` 写静态，源数据变化时颜色不会跟着变（典型反例：用户要求"过期单元格标红"时，模型用静态填充——日期变化后单元格颜色不再准确反映过期状态）。

**判断标准**：交付后 `+cond-format-list` 必须能返回该规则，否则条件格式未生效。

**大数据量首选**：当数据量 > 1000 行时，条件格式是首选——它由飞书自身渲染，比"本地脚本逐行计算 + `+cells-set` 写静态背景色"更高效、更稳（颜色还能随源数据自动联动）。

## 使用场景

读写条件格式对象，并读取条件格式**计算后的单元格样式结果**。本 reference 覆盖这些 shortcut：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 查看已有条件格式规则 | `+cond-format-list` | 获取规则类型、范围和样式配置；用于确认规则对象已存在 |
| 创建/更新/删除条件格式规则 | `+cond-format-create` / `+cond-format-update` / `+cond-format-delete` | 对条件格式规则执行写入操作 |
| 验证条件格式计算结果 | `+cond-format-result-get` | 读取命中后的 `cell_styles`，确认条件格式是否真的作用到哨兵单元格 |
| 常规读数时临时带上条件格式 | `+cells-get --include conditional_format` | 与 `+cond-format-result-get` 等价地合并条件格式样式，但仍归属普通单元格读取入口 |

典型工作流：先读取现有条件格式了解配置 → 执行创建/更新/删除 → **必须先用 `+cond-format-list` 验证规则对象，再用 `+cond-format-result-get` 抽查计算结果**。

**常见配置错误（必须注意）**：
- **创建后必须两段验证**：条件格式创建后先调用 `+cond-format-list` 验证规则对象（rule_type / ranges / style / attrs）是否存在且配置正确；再调用 `+cond-format-result-get --range "<哨兵范围>"` 读取命中后的 `cell_styles`，验证条件格式是否真的按计算结果作用到单元格。如果任一阶段不符合预期，应立即修复并重试
- **验证要覆盖哨兵格**：不要只确认规则对象存在；还要按用户规则抽查 2-3 个应命中 / 不应命中的单元格/行（含边界行、空值、重复值、非图例状态），用 `+cond-format-result-get` 读取 `cell_styles.background_color` / `font_color` / `font_weight` 等结果，确认公式、范围、颜色语义能解释这些哨兵。若规则存在但哨兵样式/命中逻辑不对，继续修正
- **范围要精确**：条件格式的应用范围必须精确覆盖用户指定的列/行，不要遗漏
- **`style.back_color` vs `style.fore_color` 的中文语义**：用户中文语境下的"**标红/染色/标记**"指**单元格背景色**，用 `back_color`；"**文字红/字体红/把字变红**"才用 `fore_color`。默认无说明时选 `back_color`。用户说"**标红**"用标准红 `back_color: "#FF0000"`；说"**高亮/突出**"才用浅色底（如 `#FFE6E6`）配合可选的 `fore_color` 加深字体——把"标红"做成浅粉会被认为没按要求标色
- **日期/空值比较必须防空**：用户说"过期的标红"时，除了 `TODAY()`，公式必须排除空单元格，否则空白格也会被误判为"早于今天"而全表标红。正确公式：`=AND(E1<>"", E1<=TODAY())`；错误公式：`=E1<=TODAY()`（空值会被当作 0 判为过期）
- **公式条件注意引用方式**：自定义公式条件中的单元格引用需要根据实际场景选择相对/绝对引用（如 `=E1<=TODAY()` 而非 `=$E$1<=TODAY()`，后者只比较一个格）
- **`duplicateValues` 只按单列判重**：用户说的"多字段完全相同才算重复"无法直接表达——先建辅助键列（把参与判重的列用分隔符拼成一个键），再用引用该键列的 `expression`（如 `COUNTIF` 键列 >1）把规则应用到数据区整行；只想标记键列本身时才用 `duplicateValues`

⚠️ **用户明确要求"辅助列+条件格式"两步走时，禁止用 `expression` 绕过**：当用户说以下任意一种表达时，必须按两步走（先建辅助列 → 再基于辅助列做条件格式），**禁止**直接用一个 `rule_type: "expression"` 公式一步完成：

- "**增加辅助列**，再/然后标记……"
- "**先计算/判断** XX **是否** YY，**再**标记……"
- "**新建一列**放结果，再用结果染色"
- 明确要求用 "辅助列"、"辅助字段"、"判断列"、"标记列"

**正确做法（两步走）**：

Step 1 的 `+cells-set` 及 `--copy-to-range` 等 flag 以 `references/lark-sheets-write-cells.md` 为准。

```
Step 1: `+cells-set` 在新列写判断公式（形成"是/否"或布尔辅助列）
  range="H2", cells=[[{formula: "=IF(A2>B2, \"是\", \"否\")"}]], --copy-to-range="H2:H100"

Step 2: 基于辅助列值做条件格式（用 cellIs 或引用辅助列的 expression）
  `+cond-format-{create|update|delete}` create
    rule_type: "expression"
    ranges: ["A2:H100"]  // 整行高亮
    attrs: [{formula: ["=$H2=\"是\""]}]  // 引用辅助列
    style: {back_color: "#FFECEC"}
```

**错误做法（一步走绕过辅助列）**：

```
`+cond-format-{create|update|delete}` create
  rule_type: "expression"
  ranges: ["2:145"]
  attrs: [{formula: ["=$O2>$H2"]}]   ← 虽然逻辑等价，但产物里缺辅助列 → 不满足用户明确要求的"辅助列"诉求
```

为什么禁止一步走：用户明确要求辅助列是有**业务意图**的——让人肉眼能在表里看到"是/否"列；条件格式只是视觉辅助。一步 expression 虽然效果对了，但用户打开表格看不到辅助列，被视为"操作不完整/未采用公式"。

`expression` 单独使用的场景是：用户**没有**明确要求辅助列、只要"标红符合条件的行"时。

⚠️ **创建条件格式前必须读数据行确认列对应**：仅读首行表头（`+csv-get range="A1:Z1"`）不够——如果表头语义含糊（比如"时间"、"日期"这种多列同义词），formula 里引用的列字母可能张冠李戴。必须再读 3-5 行**数据样本**（如 `range="A2:Z6"`）确认：①列名对应的实际值；②字段含义匹配用户描述；③数据类型是日期/数字/文本。特别是比较类条件格式（`=$A2>$B2` 这种），列字母选错整条规则就废了。

## 辅助判断列与动态高亮示例

需要展示规则判断并动态标记结果时，可采用「判断公式 → 辅助列 → 条件格式」。这也是导入不支持元素时的一个候选方案；是否替代按 `references/lark-sheets-workbook.md` 的通用原则选择，不要求采用本例。此实现用于提示，不会阻止录入。

采用本例时，先从源文件或用户要求读取判断规则、作用范围和空值策略，不从已丢失配置的在线表猜规则。按 `references/lark-sheets-write-cells.md` 确认真实表头行、首条数据行及空白辅助列；辅助列只放在允许新增的范围内。公式保留实际业务条件与数据范围，涉及唯一性时覆盖实际输入全区间。

示例仅演示「允许空白；录入值为非负数字且不超过同一行上限」：已确认 B 列是上限、C 列是录入值，字段表头在第 2 行、数据在第 3–12 行，F 是原末列、G 全空且允许新增。按真实结构和源规则改写，不能照抄字母与行号；要求必填时空值须返回「非法」，不能沿用示例的空字符串分支。

```bash
# 先继承表头与数据样式；不覆盖上方合并标题
lark-cli sheets +range-copy --url "<表格URL>" --sheet-name "<真实表名>" \
  --source-range "F2:F12" --target-range "G2" --paste-type formats
lark-cli sheets +cells-set --url "<表格URL>" --sheet-name "<真实表名>" \
  --range "G2" --cells '[[{"value":"输入校验"}]]'

# 先验证种子公式，再用同一模板扩展；IFERROR 把输入错误归为非法
lark-cli sheets +cells-set --url "<表格URL>" --sheet-name "<真实表名>" \
  --range "G3" --cells '[[{"formula":"=IFERROR(IF(C3=\"\",\"\",IF(AND(ISNUMBER(C3),C3>=0,C3<=B3),\"合法\",\"非法\")),\"非法\")"}]]'
lark-cli sheets +formula-verify --url "<表格URL>" --sheet-name "<真实表名>" \
  --range "G3" --exit-on-error
lark-cli sheets +cells-set --url "<表格URL>" --sheet-name "<真实表名>" \
  --range "G3" --cells '[[{"formula":"=IFERROR(IF(C3=\"\",\"\",IF(AND(ISNUMBER(C3),C3>=0,C3<=B3),\"合法\",\"非法\")),\"非法\")"}]]' \
  --copy-to-range "G3:G12"
lark-cli sheets +formula-verify --url "<表格URL>" --sheet-name "<真实表名>" \
  --range "G3:G12" --exit-on-error

# 锁辅助列、让行号随数据行变化，范围不含表头
lark-cli sheets +cond-format-create --url "<表格URL>" --sheet-name "<真实表名>" \
  --rule-type expression --ranges '["A3:G12"]' \
  --properties '{"attrs":[{"formula":["=$G3=\"非法\""]}],"style":{"back_color":"#FF0000"}}'
lark-cli sheets +cond-format-list --url "<表格URL>" --sheet-name "<真实表名>"
lark-cli sheets +cond-format-result-get --url "<表格URL>" --sheet-name "<真实表名>" \
  --range "A3:G12"
```

公式诊断覆盖整个辅助列，`partial` 分段续查；诊断通过还要用哨兵核对业务结果：合法值、非法值、上下界、按规则允许的空值，以及涉及唯一性时的重复记录。核对辅助列与条件格式结果一致，排除既有规则 / 静态底色造成的假命中；在独立测试副本中修改输入，确认提示和颜色随之更新，不为测试改写用户原始记录。

采用该方案时，在交付说明中写明实际判断规则和使用限制，例如「提示非法值，不会阻止录入」，并列出与原实现的差异。

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+cond-format-list` | read | 对象 |
| `+cond-format-result-get` | read | 对象 |
| `+cond-format-create` | write | 对象 |
| `+cond-format-update` | write | 对象 |
| `+cond-format-delete` | high-risk-write | 对象 |

## Flags

### `+cond-format-list`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--rule-id` | string | optional | 按规则 id 过滤 |

### `+cond-format-result-get`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--range` | string | required | A1 范围，如 `A1:F10`（不带 sheet 前缀；用 `--sheet-id` / `--sheet-name` 指定 sheet） |
| `--max-chars` | int | optional | 单次返回字符上限，默认 500000（兜底防爆） |

### `+cond-format-create`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--properties` | string + File + Stdin（复合 JSON） | required | 规则配置 JSON，含 `style`（命中样式，必填）和 `attrs?`（规则参数列表，因 `rule_type` 不同结构而异）/ `has_ref?`。`rule_type` 和 `ranges` 已拎为独立 flag |
| `--rule-type` | string | required | 条件格式规则类型；优先级高于 `--properties` 中同名字段（可选值：`duplicateValues` / `uniqueValues` / `cellIs` / `containsText` / `timePeriod` / `containsBlanks` / `notContainsBlanks` / `dataBar` / `colorScale` / `rank` / `aboveAverage` / `expression` / `iconSet`） |
| `--ranges` | string + File + Stdin（简单 JSON） | required | 应用条件格式的 A1 范围 JSON 数组（如 `["A1:A100","C2:C50"]`）；优先级高于 `--properties` 中同名字段 |

### `+cond-format-update`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--rule-id` | string | required | 目标规则 id |
| `--properties` | string + File + Stdin（复合 JSON） | required | 规则配置 JSON，结构同 `+cond-format-create` 的 `--properties`；update 是整组覆盖式 |
| `--rule-type` | string | required | 条件格式规则类型；优先级高于 `--properties` 中同名字段（可选值：`duplicateValues` / `uniqueValues` / `cellIs` / `containsText` / `timePeriod` / `containsBlanks` / `notContainsBlanks` / `dataBar` / `colorScale` / `rank` / `aboveAverage` / `expression` / `iconSet`） |
| `--ranges` | string + File + Stdin（简单 JSON） | required | 应用条件格式的 A1 范围 JSON 数组（如 `["A1:A100","C2:C50"]`）；优先级高于 `--properties` 中同名字段 |

### `+cond-format-delete`

_公共四件套 · 系统：`--yes`、`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--rule-id` | string | required | 目标规则 id |

## Schemas

> 复合 JSON flag 字段速查（只列顶层 + 一层嵌套）。深层结构看下方 `## Examples`，或用 `--print-schema` 读完整 JSON Schema（用法见 SKILL.md「公共 flag 速查」与「Agent 使用提示」）。

### `+cond-format-create` `--properties` / `+cond-format-update` `--properties`

_创建/更新的条件格式属性_

**顶层字段**：
- `rule_type` (enum) — 条件格式规则类型 [duplicateValues / uniqueValues / cellIs / containsText / timePeriod / containsBlanks / notContainsBlanks / dataBar / colorScale / rank / aboveAverage / expression / iconSet] — ⚠️ 已拎为独立 flag `--rule-type`，请勿在此 JSON 内重复填写（同名以独立 flag 为准）
- `ranges` (array<string>) — 应用条件格式的 A1 范围列表 — ⚠️ 已拎为独立 flag `--ranges`，请勿在此 JSON 内重复填写（同名以独立 flag 为准）
- `style` (object) — 命中规则时应用的单元格样式 { back_color?: string, fore_color?: string, text_decoration?: enum, font?: enum }
- `attrs` (array<oneOf>?) — 规则参数列表
- `has_ref` (boolean?) — 可选

## Examples

公共四件套：所有 shortcut 顶部排列 `--url` / `--spreadsheet-token` / `--sheet-id` / `--sheet-name`（XOR）。

### `+cond-format-list`

```bash
# 列出当前 sheet 全部条件格式规则（拿 rule_id 供 update/delete）
lark-cli sheets +cond-format-list --url "..." --sheet-id "$SID"
```

### `+cond-format-create`

`--rule-type` / `--ranges` 是独立 flag（不要再放 `--properties`）；`style` / `attrs` 等结构走 `--properties`：

```bash
# 重复值高亮
lark-cli sheets +cond-format-create --url "..." --sheet-id "$SID" \
  --rule-type duplicateValues --ranges '["A1:A100"]' \
  --properties '{"style":{"back_color":"#FFD7D7"}}'

# 数据条
lark-cli sheets +cond-format-create --url "..." --sheet-id "$SID" \
  --rule-type dataBar --ranges '["B2:B100"]' \
  --properties @rule.json

# 创建后先确认规则对象存在
lark-cli sheets +cond-format-list --url "..." --sheet-id "$SID"

# 再抽查条件格式计算结果：读取哨兵单元格的命中样式
lark-cli sheets +cond-format-result-get --url "..." --sheet-id "$SID" \
  --range "B2:B10"
```

### `+cond-format-result-get`

用于读取条件格式**计算后的样式结果**，不是读取规则对象。创建 / 更新条件格式后必须用它抽查哨兵单元格。

CLI 会对白名单字段做输出裁剪：顶层只保留警告、分页和返回单元格计数；每个 range 只保留请求/实际范围、真实行列坐标、截断标记和二维 `cells`；每个 cell 只保留 `cell_styles`，不返回 `value` / `formula` / `note` / `data_validation` / `border_styles` 等其它单元格数据。`cell_styles` 是底层开启条件格式计算后得到的最终合并样式，不包含 `rule_id` 或独立的命中标记。

```bash
# 读取 B2:B10 的条件格式命中样式，返回 cell_styles.background_color / font_color 等
lark-cli sheets +cond-format-result-get --url "..." --sheet-id "$SID" \
  --range "B2:B10"

# 如果只想在普通读取里临时合并条件格式，也可用 +cells-get --include conditional_format
lark-cli sheets +cells-get --url "..." --sheet-id "$SID" \
  --range "B2:B10" --include conditional_format
```

### `+cond-format-update`

整组覆盖式：先 `+cond-format-list --rule-id <id>` 拿当前完整配置，改后整组传回。

### `+cond-format-delete`

```bash
lark-cli sheets +cond-format-delete --url "..." --sheet-id "$SID" --rule-id "$RULE_ID" --yes
```

> 一次只删一个 `--rule-id`。要删**多个**条件格式时，先 `+cond-format-list` 拿到各 `rule-id`，再用 `+batch-update` 把多个 `+cond-format-delete` 合并为单次批量提交（fail-fast，失败处置见 `references/lark-sheets-batch-update.md`），不要逐个调用。

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`--rule-type` / `--ranges` 必填；`--properties` 必须能解析为合法 JSON；按 `--rule-type` 检查必填子字段（`cellIs` 需 `attrs.operator` + `attrs.value`、`expression` 需 `attrs.formula`、`colorScale` 需 `min/mid/max` 配色等）；`+cond-format-delete` 强制 `--yes` 或 `--dry-run`。
- `DryRun`：写操作输出"将要 POST/PATCH/DELETE 的 conditional_format 请求模板"。
- `Execute`：写后不自动回读；create/update 后必须调用 `+cond-format-list --rule-id <id>` 比对规则 / 范围 / 样式，并用 `+cond-format-result-get --range <2–3 个哨兵格>` 核对实际生效的单元格样式；delete 后 list 确认目标 id 不存在。
