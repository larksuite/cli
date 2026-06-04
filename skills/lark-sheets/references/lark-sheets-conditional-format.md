# Lark Sheet Conditional Format

## 真对象硬约束 + 触发词清单

用户出现以下口语指令时，**强制**走 `+cond-format-{create|update|delete}`，**禁止**用 `+cells-set` 写静态背景色 / 字体色代替：

- **颜色动作**："标红 / 标黄 / 标绿 / 上色 / 染色 / 涂色 / 表红色 / 表黄色"
- **视觉强调**："高亮 / 突出 / 标记 / 标注 / 区分"
- **条件触发**："重复的标出来 / 异常的圈出来 / 过期的染红 / 大于 X 的标黄 / 不达标的标红"
- **联动语义**："颜色随数据变 / 联动 / 自动更新 / 改了数据颜色也跟着变"
- **数值可视化**："数据条 / 色阶 / 渐变色 / 进度条样式"

飞书表格的"颜色标记"语义 = 条件格式规则 ≠ 静态背景色。如果用 `+cells-set` 写静态，源数据变化时颜色不会跟着变（典型反例：用户要求"过期单元格标红"时，模型用静态填充——日期变化后单元格颜色不再准确反映过期状态）。

**判断标准**：交付后 `+cond-format-list` 必须能返回该规则；否则视为违规。

**大数据量首选**：当数据量 > 1000 行时，条件格式是首选——它由飞书自身渲染，比"本地脚本逐行计算 + `+cells-set` 写静态背景色"更高效、更稳（颜色还能随源数据自动联动）。

## 使用场景

读写条件格式对象。本 reference 覆盖 4 个 shortcut：

| 操作需求 | 使用工具 | 说明 |
|---------|---------|------|
| 查看已有条件格式 | `+cond-format-list` | 获取规则类型、范围和样式配置 |
| 创建/更新/删除条件格式 | `+cond-format-{create|update|delete}` | 对条件格式规则执行写入操作 |

典型工作流：先读取现有条件格式了解配置 → 执行创建/更新/删除 → **必须再次读取验证结果**。

**常见配置错误（必须注意）**：
- **创建后必须验证**：条件格式创建后必须调用 `+cond-format-list` 验证规则是否生效。如果验证发现规则未生效或配置不正确，应立即修复并重试
- **范围要精确**：条件格式的应用范围必须精确覆盖用户指定的列/行，不要遗漏
- **`style.back_color` vs `style.fore_color` 的中文语义**：用户中文语境下的"**标红/高亮/染色/标记**"指**单元格背景色**，用 `back_color`；"**文字红/字体红/把字变红**"才用 `fore_color`。默认无说明时选 `back_color`。把过期数据涂红、重复值高亮等都应该是 `back_color: "#FFE6E6"`（或类似浅红）配合可选的 `fore_color` 加深字体
- **日期/空值比较必须防空**：用户说"过期的标红"时，除了 `TODAY()`，公式必须排除空单元格，否则空白格也会被误判为"早于今天"而全表标红。正确公式：`=AND(E1<>"", E1<=TODAY())`；错误公式：`=E1<=TODAY()`（空值会被当作 0 判为过期）
- **公式条件注意引用方式**：自定义公式条件中的单元格引用需要根据实际场景选择相对/绝对引用（如 `=E1<=TODAY()` 而非 `=$E$1<=TODAY()`，后者只比较一个格）

⚠️ **用户明确要求"辅助列+条件格式"两步走时，禁止用 `expression` 绕过（高频致命错误）**：当用户说以下任意一种表达时，必须按两步走（先建辅助列 → 再基于辅助列做条件格式），**禁止**直接用一个 `rule_type: "expression"` 公式一步完成：

- "**增加辅助列**，再/然后标记……"
- "**先计算/判断** XX **是否** YY，**再**标记……"
- "**新建一列**放结果，再用结果染色"
- 明确要求用 "辅助列"、"辅助字段"、"判断列"、"标记列"

**正确做法（两步走）**：

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

## Shortcuts

| Shortcut | Risk | 分组 |
| --- | --- | --- |
| `+cond-format-list` | read | 对象 |
| `+cond-format-create` | write | 对象 |
| `+cond-format-update` | write | 对象 |
| `+cond-format-delete` | high-risk-write | 对象 |

## Flags

### `+cond-format-list`

_公共四件套 · 系统：`--dry-run`_

| Flag | Type | 必填 | 说明 |
| --- | --- | --- | --- |
| `--rule-id` | string | optional | 按规则 id 过滤 |

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
```

### `+cond-format-update`

整组覆盖式：先 `+cond-format-list --rule-id <id>` 拿当前完整配置，改后整组传回。

### `+cond-format-delete`

```bash
lark-cli sheets +cond-format-delete --url "..." --sheet-id "$SID" --rule-id "$RULE_ID" --yes
```

> 一次只删一个 `--rule-id`。要删**多个**条件格式时，先 `+cond-format-list` 拿到各 `rule-id`，再用 `+batch-update` 把多个 `+cond-format-delete` 合并为单次原子提交，不要逐个调用。

### Validate / DryRun / Execute 约束

- `Validate`：XOR 公共四件套；`--rule-type` / `--ranges` 必填；`--properties` 必须能解析为合法 JSON；按 `--rule-type` 检查必填子字段（`cellIs` 需 `attrs.operator` + `attrs.value`、`expression` 需 `attrs.formula`、`colorScale` 需 `min/mid/max` 配色等）；`+cond-format-delete` 强制 `--yes` 或 `--dry-run`。
- `DryRun`：写操作输出"将要 POST/PATCH/DELETE 的 conditional_format 请求模板"。
- `Execute`：写后不自动回读；如需确认，自行调用 `+cond-format-list --rule-id <id>` 比对规则 / 范围 / 样式。
