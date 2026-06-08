# 飞书表格公式生成规则

> **本文定位**：飞书公式正确性的**唯一权威**——书写任何飞书公式、或把 Excel 公式迁移到飞书前，先读本文。涵盖公式书写约定（绝对引用、范围语法）、投影 vs spill、ARRAYFORMULA / 数组语义、高风险引用函数、日期差、不支持函数清单。
> **边界**：本文只讲"公式怎么写对"；公式**怎么写入表格**（`+cells-set` / 模板单元格 + `--copy-to-range` / 容错回读）见 `lark-sheets-write-cells` 与 `lark-sheets-core-operations`。本文不含 shortcut，铁律见 `lark-sheets-core-operations`。

**核心原则：飞书不像 Excel 365 那样默认 spill（溢出展开）。飞书普通公式遇到区域时默认"投影"（只取当前行/列对应的单个值），必须显式使用 `ARRAYFORMULA` 或原生数组函数才能逐项展开。**

## 公式书写约定（写任何公式都先满足）

- **绝对引用 `$`**：向下 / 向右填充前判断哪些引用要锁定——用户指定的固定 cell（`$C$3`）、要固定的数据范围（`$A$2:$B$5`）、锁列不锁行（`$A2`）、锁行不锁列（`B$1`）。填充前检查是否需固定汇率 / 税率 / 查找表 / 权重表，以及同列 / 同行公式结构是否一致。
- **公式字符串用飞书范围语法**：写 `H:H`、`A2:B5`，**禁止** `H2:H` / `2:2`。这与 CLI 工具参数（如 `--range`）的 A1 表示法（`A1:D3`、`1:1`）写法不同，两者混淆会导致调用失败或公式报错。

## 翻译后必做：代码复现校验

公式语法翻译完之后，**必须**用本地脚本在源数据上独立复现一份"等价计算结果"再写入。流程：

1. **挑 3-5 个代表性输入行**（首行 / 中段 / 末行 / 含空值 / 含异常格式各一）
2. **用 Python 复现 Excel 原公式的语义**（不是飞书译文的语义，而是用户原本想要的结果）
3. **写入飞书译文公式后回读这几行的实际值**
4. **三方对照**：`Excel 原公式语义 == Python 复现 == 飞书译文回读值`，全部一致才交付；不一致先排查（数组语义？日期差？范围引用？）

**理由**：Excel→飞书的语法翻译很容易在 spill / 数组 / 日期差 / 范围引用上出现等价性偏差，仅靠语法转换通过不足以保证业务结果正确。

## 决策流程

1. 最终结果是**标量**（单值）→ 通常不需要 `ARRAYFORMULA`
2. 最终结果是**一维或二维数组**：
   - 公式中**包含**飞书原生数组函数（如 FILTER、XLOOKUP、MAP 等）→ 无需加 `ARRAYFORMULA`，数组语义会自动传播到整个公式，包括原生数组函数外层接的标量运算（如 `+1`、`*100`）
   - 公式中**不包含**任何原生数组函数，但在对区域做标量计算 → 加 `ARRAYFORMULA(<整个表达式>)`
3. Excel 依赖 `ROW(range)` 逐项驱动 `SUBTOTAL/INDIRECT/OFFSET` → 改用 `MAP(ARRAYFORMULA(ROW(...)), LAMBDA(r, ...))`
4. 内层 `INDEX/INDIRECT/OFFSET` 返回范围，外层 `SUMIF/COUNTIF/SUMIFS` 还要继续吃这些范围 → 改用 `MAP(..., LAMBDA(...))` 或 `REDUCE(..., LAMBDA(...))`
5. 公式意图是"对多个区域分别计算再汇总"（例如用 INDIRECT/OFFSET 对每行生成一个范围，再对所有范围聚合）→ 飞书不能直接返回"区域的列表"，必须明确降维：用 `VSTACK` 垂直合并、`HSTACK` 水平合并、`TOCOL/TOROW` 展平，或 `REDUCE` 归约成标量
6. 算日期差 → 不要写 `DAY(end-start)`，用 `DAYS`、`DATEDIF` 或直接 `end-start`

## 飞书的投影行为（不是默认 spill）

飞书普通公式对引用区域默认"投影"而不是"spill"：

- 单列区域 → 按当前公式所在行取值
- 单行区域 → 按当前公式所在列取值
- 二维区域 → 只有当前公式位置能映射到该区域时才取值，否则报错
- 数组常量 `{...}` 或函数返回矩阵，在普通标量上下文里通常只取左上角

因此：
- `=A1:A2` 在飞书普通公式里不会 spill，只会投影到当前行
- `=ABS(A2:B2)` 不会得到一整行，要写 `=ARRAYFORMULA(ABS(A2:B2))`
- `=TRUNC({1.1111,2.222},{1,2})` 要得到一整行，写 `=ARRAYFORMULA(TRUNC({1.1111,2.222},{1,2}))`

## ARRAYFORMULA 使用规则

**前提：以下规则适用于公式中没有任何原生数组函数的情况。** 若公式中已有原生数组函数（如 FILTER、XLOOKUP、MAP 等），数组语义会自动传播到整个公式的求值过程，后续标量运算无需额外包 `ARRAYFORMULA`（见下一节）。

需要加 `ARRAYFORMULA` 的典型场景（公式中无原生数组函数时）：

- 算术运算：`+ - * / ^ %`
- 比较运算：`= <> > >= < <=`
- 标量数学函数：`ABS ROUND INT TRUNC MOD LOG LN SQRT SIN COS TAN ...`
- 文本函数：`LEN LEFT RIGHT MID UPPER LOWER TRIM TEXT VALUE ...`
- 日期函数：`YEAR MONTH DAY DATE TIME EDATE EOMONTH ...`
- 条件函数：`IF IFS IFERROR IFNA NOT ISNUMBER ISTEXT ISBLANK ...`
- 引用函数（高风险）：`INDEX OFFSET COLUMN ROW MATCH`

### 公式中有原生数组函数时，整个公式已进入数组模式

飞书的数组语义会在整个公式求值过程中累积传播：一旦某个原生数组函数运行，后续所有运算符和函数也会自动逐元素处理，无论它们出现在哪一层。

因此，以下写法**无需**额外包 `ARRAYFORMULA`：

- `=FILTER(A2:A10,B2:B10="x")+1` ✓
- `=XLOOKUP(E2:E10,A2:A10,B2:B10)*100` ✓
- `=ABS(FILTER(A2:A10,B2:B10>0))` ✓
- `=MAP(A2:A10,LAMBDA(x,x*2))-1` ✓

对比：**没有原生数组函数**时必须加：

- `=A2:A100*B2:B100` → `=ARRAYFORMULA(A2:A100*B2:B100)` ✓
- `=IF(A2:A100>0,B2:B100,"")` → `=ARRAYFORMULA(IF(A2:A100>0,B2:B100,""))` ✓

## 飞书原生数组函数清单

以下函数按数组语义工作，通常**不需要额外包 `ARRAYFORMULA`**：

`ARRAYFORMULA` `ARRAY_CONSTRAIN` `BYCOL` `BYROW` `CELL` `CHOOSECOLS` `CHOOSEROWS` `DROP` `EXPAND` `FILTER` `FLATTEN` `FREQUENCY` `GROWTH` `HSTACK` `IMPORTDATA` `IMPORTFEED` `IMPORTHTML` `IMPORTRANGE` `IMPORTXML` `LINEST` `LOGEST` `LOOKUP` `MAKEARRAY` `MAP` `MINVERSE` `MMULT` `MUNIT` `QUERY` `RANDARRAY` `REDUCE` `REGEXEXTRACT` `SCAN` `SEQUENCE` `SORT` `SORTBY` `SORTN` `SPLIT` `SUMPRODUCT` `SWITCH` `TAKE` `TEXTSPLIT` `TOCOL` `TOROW` `TRANSPOSE` `TREND` `UNIQUE` `VSTACK` `WRAPCOLS` `WRAPROWS` `XLOOKUP`

> **注意：`SWITCH` 在飞书里被当作原生数组函数处理，这与 Excel 行为不同，不需要额外包 `ARRAYFORMULA`。**

## IMPORTRANGE 跨工作簿引用限制

用 `IMPORTRANGE` 跨电子表格引用数据时有两条硬上限：

- **嵌套最多 5 层**：被引用的表里若又用 `IMPORTRANGE` 继续引下一张表，整条引用链最多 5 层。
- **每个工作表最多 100 个 `IMPORTRANGE` 引用**。

超限会让引用失效或报错。设计大量跨表汇总前先估算引用数，必要时先把数据落地到本表再计算。

## INDEX / OFFSET / COLUMN / ROW / MATCH 是高风险函数

这组函数容易让人误以为会自动把多值铺开，但在飞书里不能这样假设。

**高风险信号：**

- 行号 / 列号 / 偏移量本身是数组
- 结果本来应该是一行或一块二维区域
- 外层还有算术、比较、`IF` 等继续处理它

更稳的写法：

- `=ARRAYFORMULA(INDEX(...))`
- `=ARRAYFORMULA(OFFSET(...))`
- `=ARRAYFORMULA(COLUMN(...))`
- `=ARRAYFORMULA(ROW(...))`

**例外：** 如果返回值只是立刻交给聚合函数消费，不需要额外包：

- `=SUM(INDEX(A1:B2,0,1))` ✓

## Excel 隐式逐项求值，飞书里要显式写 MAP

**典型特征：**

- 外层是 `SUMPRODUCT`、`SUM` 等聚合
- 内层用了 `SUBTOTAL`、`INDIRECT`、`OFFSET` 等更偏"单值/单引用"的函数
- Excel 会把中间结果逐项带进去算
- 飞书里直接照抄，往往不能得到同样的逐项语义

同类本质也包括：`INDEX/INDIRECT/OFFSET` 先返回范围，外层再把这些范围交给 `SUMIF`、`COUNTIF`、`AVERAGEIF`、`SUMIFS` 等范围感知函数 —— 飞书里这些外层函数不会自动二次展开内层范围。

这时不要只会补 `ARRAYFORMULA`，要显式写"遍历"。最常用模板：

```excel
=SUMPRODUCT(
  MAP(
    ARRAYFORMULA(ROW(目标范围)),
    LAMBDA(r, 单行计算逻辑)
  )
)
```

同类场景也优先考虑 `MAP`：

- `INDIRECT("A"&ROW(...))`
- `OFFSET(...,ROW(...)-ROW(...),...)`
- `SUBTOTAL(...)`
- `SUMIF(内层返回范围, ...)`
- `COUNTIF(内层返回范围, ...)`
- `SUMIFS(内层返回范围, ...)`
- 任何"希望对每一行 / 每一列各算一次"的模式

## 多层范围结果与三维以上结果

飞书公式结果只能是二维区域，不能是"数组的数组"。

### 多层范围不能自动二次展开

内层 `INDEX/INDIRECT/OFFSET` 返回的是二维范围，外层还想继续对这些范围做范围计算时，不要假设飞书会"再展开一层"。改用：

- `MAP(..., LAMBDA(...))` 显式逐项算
- `REDUCE(..., LAMBDA(...))` 显式累加/归约

### 真正的三维或更高维结果不能直接返回

典型触发场景：想把多个不同区域或不同条件的结果合并展示，例如：
- 对 A 列、B 列、C 列分别做 FILTER，想把三列结果并排展示
- 对多个月份分别生成数据行，想把所有月份上下堆叠展示

飞书无法直接返回"多个区域的集合"，必须先决定降维方式：

- 上下堆叠：`=VSTACK(slice1, slice2, slice3)`
- 左右拼接：`=HSTACK(slice1, slice2, slice3)`
- 压成单列：`=TOCOL(...)`
- 压成单行：`=TOROW(...)`
- 只保留聚合值：`=REDUCE(slice1, {slice2,slice3}, LAMBDA(acc,x,acc+x))`

不要替用户"偷定"第三维展示方式；如果用户没有明确说明怎么展示，至少先把结果改写成可见的二维形状。

## 不能机械照抄的 Excel 语法

### `@` 隐式交叉

Excel：`=@A1:A10`（强制单值，取当前行对应的值）

飞书没有 `@` 运算符。飞书普通公式对引用区域默认就有投影语义，去掉 `@` 即可：

- Excel: `=@A1:A10`
- 飞书: `=A1:A10`

### `#` spill range

Excel：`=A1#`（引用 A1 公式溢出的整片区域）

飞书没有此语法，迁移方式：

- spill 区域已知 → 改成明确范围
- spill 区域未知 → 回到源公式重写，或用 `TAKE` / `DROP` / `ARRAY_CONSTRAIN`

### 结构化引用

Excel：`=SUM(Table1[Amount])`

飞书不支持结构化引用，改成显式 A1 区域：`=SUM(A2:A100)`

### 老式 CSE 花括号

Excel：`{=A1:A10*B1:B10}`（Ctrl+Shift+Enter 输入）

飞书改为：`=ARRAYFORMULA(A1:A10*B1:B10)`

## 日期序列与日期差

飞书日期序列：`0 = 1899-12-30`，`1 = 1899-12-31`，没有 Excel 的 1900 年闰年兼容问题。

**高频错误写法（不要用）：**

- `=DAY(B2-A2)` ✗ — 差值会被当成日期序列号再拆字段
- `=MONTH(B2-A2)` ✗
- `=YEAR(B2-A2)` ✗

**正确写法：**

- 天数差：`=DAYS(B2,A2)` 或 `=DATEDIF(A2,B2,"D")` 或 `=B2-A2`
- 月份差：`=DATEDIF(A2,B2,"M")`
- 年份差：`=DATEDIF(A2,B2,"Y")`
- 工作日差：`=NETWORKDAYS(A2,B2)`

## 飞书不支持的函数

> 本段是"飞书不支持函数"的**唯一权威清单**（`lark-sheets-core-operations` 不再单列，统一指向这里）。以下函数在飞书里不存在或被禁用，禁止主动使用；用户明确要求时应拒绝并提供替代方案：

- `STOCKHISTORY` — 实时股票数据，飞书无等价函数，需手动导入数据
- `WEBSERVICE` — 外部 HTTP 请求，飞书无等价函数
- CUBE 系列（`CUBEVALUE`、`CUBEMEMBER`、`CUBESET`、`CUBERANK` 等）— OLAP cube 函数，飞书不支持
- `GOOGLEFINANCE`、`GOOGLETRANSLATE` 等 Google 特有函数 — 无等价函数
- `FORECAST.ETS` 系列（`FORECAST.ETS`、`FORECAST.ETS.STAT` 等）— 飞书不支持
- `INFO`、`RTD` — 系统信息 / 实时数据函数，飞书不支持
- `PIVOT` — 用 `+pivot-{create|update|delete}` 透视表对象替代
- `AMORDEGRC`、`PHONETIC`、`DETECTLANGUAGE` — 飞书不支持

## 代表性改写示例

- 基础逐项计算
  - Excel: `=A2:A100*B2:B100`
  - 飞书: `=ARRAYFORMULA(A2:A100*B2:B100)`
- 条件判断
  - Excel: `=IF(A2:A100>0,B2:B100,"")`
  - 飞书: `=ARRAYFORMULA(IF(A2:A100>0,B2:B100,""))`
- 原生数组函数（无需改动）
  - Excel: `=FILTER(A2:C100,B2:B100="East")`
  - 飞书: `=FILTER(A2:C100,B2:B100="East")`
- 原生数组函数 + 标量运算（无需改动，数组语义自动传播）
  - Excel: `=XLOOKUP(E2:E10,A2:A10,B2:B10)*100`
  - 飞书: `=XLOOKUP(E2:E10,A2:A10,B2:B10)*100`
- 高风险引用函数
  - Excel: `=INDEX(A1:D2,{2,1},0)`
  - 飞书: `=ARRAYFORMULA(INDEX(A1:D2,{2,1},0))`
- 日期差
  - 错误: `=DAY(B2-A2)`
  - 推荐: `=DAYS(B2,A2)` 或 `=DATEDIF(A2,B2,"D")` 或 `=B2-A2`
- Excel 隐式逐项求值
  - Excel: `=SUMPRODUCT(SUBTOTAL(103,INDIRECT("E"&ROW($E$16:$E$387))))`
  - 飞书: `=SUMPRODUCT(MAP(ARRAYFORMULA(ROW($E$16:$E$387)),LAMBDA(row,SUBTOTAL(103,INDIRECT("E"&row)))))`
- 多层范围 / 二次展开
  - 错误思路: `=SUMIF(INDIRECT("E"&ROW($E$16:$E$387)),">0")`
  - 飞书: `=MAP(ARRAYFORMULA(ROW($E$16:$E$387)),LAMBDA(r,SUMIF(INDIRECT("E"&r),">0")))`
- 三维降二维（保留所有层）
  - 飞书: `=VSTACK(slice1,slice2,slice3)` 或 `=HSTACK(slice1,slice2,slice3)`
- 三维降二维（只保留聚合值）
  - 飞书: `=REDUCE(slice1,{slice2,slice3},LAMBDA(acc,x,acc+x))`
