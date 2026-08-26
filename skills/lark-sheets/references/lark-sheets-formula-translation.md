# 飞书表格公式生成规则

> **本文定位**：飞书公式正确性的**唯一权威**——书写任何飞书公式、或把 Excel 公式迁移到飞书前，先读本文。涵盖公式书写约定（绝对引用、范围语法）、投影 vs spill、`ARRAYFORMULA` / 数组语义与逐行填充、高风险引用函数、日期差、不支持函数清单。
> **边界**：本文只讲"公式怎么写对"；公式**怎么写入表格**（`+cells-set` / 模板单元格 + `--copy-to-range` / 容错回读）见 `lark-sheets-write-cells`。公式写入完成后可用 `lark-sheets-formula-verify` 做诊断；不要把"翻译对了"误当成"结果一定正确"。本文不含 shortcut，通用编辑准则见主 SKILL.md「飞书表格编辑准则」。

**核心原则一：飞书不像 Excel 365 那样默认 spill（溢出展开）。** 某个参数要求单值、实际传入的却是区域时，飞书默认取"投影"（按公式所在行/列取对应的那一个值）；只有当求值处于**数组公式上下文内部**——最外层套了 `ARRAYFORMULA`，或公式里已有原生数组函数（`FILTER` / `XLOOKUP` / `SORT` 等，见下方清单）——才逐项展开。`ARRAYFORMULA` 与"逐行标量公式 + `--copy-to-range` 填充"导出后都保真，按需要选：前者一条公式覆盖整片、写起来短；后者每格是独立公式，导出后在 Excel 里能单格编辑。

**核心原则二：`LAMBDA` 系高阶函数（`MAP` / `REDUCE` / `SCAN` / `BYROW` / `BYCOL` / `MAKEARRAY`）在飞书内算得对，但导出 `.xlsx` 会静默算错。** 导出时飞书只把 LAMBDA 体内联展开成普通数组表达式，**不保留高阶语义**，全程不报错：

- `REDUCE(0,A2:A6,LAMBDA(acc,x,acc+x))`（归约求和）→ `=0+A2:A6`，归约整个丢失
- `BYROW(A2:B6,LAMBDA(r,SUM(r)))`（逐行求和）→ `=SUM(A2:B6)`，5 个结果塌成 1 个
- `MAP(A2:A6,LAMBDA(x,IF(x>0,x,0)))` → `=A2:A6>0`，`IF` 整个消失

唯一例外是 `MAP` 且 LAMBDA 体为**纯运算符或单参函数**时展开恰好等价（`LAMBDA(a,b,a*b)` → `=A2:A6*B2:B6` ✓）。**除此之外一律改走 `ARRAYFORMULA` / 逐行填充 / 辅助列**——同样的逐项逻辑写成 `=ARRAYFORMULA(IF(A2:A6>0,A2:A6,0))` 导出后完整保留，写成 `MAP(...LAMBDA(...IF...))` 就丢。这类错误公式不报错、飞书里回读也是对的，只有导出后才暴露，靠事后检查发现不了。

## 公式书写约定（写任何公式都先满足）

- **绝对引用 `$`**：向下 / 向右填充前判断哪些引用要锁定——用户指定的固定 cell（`$C$3`）、要固定的数据范围（`$A$2:$B$5`）、锁列不锁行（`$A2`）、锁行不锁列（`B$1`）。填充前检查是否需固定汇率 / 税率 / 查找表 / 权重表，以及同列 / 同行公式结构是否一致。
- **公式字符串用飞书范围语法**：写 `H:H`、`A2:B5`，**禁止** `H2:H` / `2:2`。要在公式里引用整行，用显式范围（如 `$A2:$Z2`）替代禁用的 `2:2`。这与 CLI 工具参数（如 `--range` / `--copy-to-range`）的 A1 表示法写法不同：参数侧合法的 `D3:D`、`1:1`、`3:6` 在公式串里反而非法。**公式串 ≠ CLI 参数**，两套规则别互相照搬，混用会导致调用失败或公式报错。
- **产物要导出 xlsx 交付时优先 Excel 兼容函数**：同一计算能用 Excel 兼容函数（SUMIFS / TEXT / MID / FIND 等）表达就不用飞书特有函数（MAP / REGEXEXTRACT / ARRAYFORMULA 等）——特有函数在导出后的 xlsx 里可能无法重算；确需使用时，导出后核对重算正常再交付。

## 翻译后建议：代码复现校验

公式语法翻译完之后，建议用本地脚本在源数据上独立复现一份"等价计算结果"再写入。流程：

1. **挑 3-5 个代表性输入行**（首行 / 中段 / 末行 / 含空值 / 含异常格式各一）
2. **用 Python 复现 Excel 原公式的语义**（不是飞书译文的语义，而是用户原本想要的结果）
3. **写入飞书译文公式后回读这几行的实际值**
4. **三方对照**：`Excel 原公式语义 == Python 复现 == 飞书译文回读值`；不一致时优先排查（数组语义？日期差？范围引用？），无法修完时在交付说明标明风险。

**理由**：Excel→飞书的语法翻译很容易在 spill / 数组 / 日期差 / 范围引用上出现等价性偏差，仅靠语法转换通过不足以保证业务结果正确。

## 落表后的默认交接

本文解决的是"公式怎么写对"，不是"写进表里后一定能零错误运行"。因此：

1. 按本文完成公式改写后，用 `lark-sheets-write-cells` / `lark-sheets-batch-update` 把公式真实写入表格。
2. 公式一旦落表，可进入 `lark-sheets-formula-verify` 做诊断。
3. `+formula-verify` 的 `errors_found` / `partial` 是风险信号；关键输出区优先修复，非关键区可在交付说明记录。

**静态值改公式（"让统计表跟随源数据变化"类任务）额外一步**：改写前先快照原静态值，公式写完后逐格与快照 diff。不一致时先尝试口径变体（`>` / `>=`、取整方式、匹配列）逼近原值；仍不一致不算失败——原静态值可能对应旧数据或含未声明口径——但必须在交付说明中给出 diff 表与所用口径的解释，禁止不声明差异直接交付。

## 决策流程

1. 最终结果是**标量**（单值）→ 直接写普通公式
2. 最终结果是**一维或二维数组**：
   - 公式中**包含**飞书原生数组函数（如 FILTER、XLOOKUP、MAP 等）→ 直接写，数组语义会自动传播到整个公式，包括原生数组函数外层接的标量运算（如 `+1`、`*100`）
   - 公式中**不包含**任何原生数组函数，只是在对区域做标量计算 → 用 `ARRAYFORMULA` 包住整个表达式，或写成**单行标量公式再向下 / 向右填充**（`--copy-to-range`）
3. Excel 依赖 `ROW(range)` 逐项驱动 `SUBTOTAL/INDIRECT/OFFSET` → 拆成辅助列：辅助列每行写单行标量式（`=SUBTOTAL(103,INDIRECT("E"&ROW(E16)))`）向下填充，再对辅助列做聚合；但结果要随筛选联动时保持单条 `MAP(...LAMBDA(...))`，见下方「Excel 隐式逐项求值」
4. 内层 `INDEX/INDIRECT/OFFSET` 返回范围，外层 `SUMIF/COUNTIF/SUMIFS` 还要继续吃这些范围 → 同样拆辅助列逐行算，再聚合
5. 公式意图是"对多个区域分别计算再汇总"（例如用 INDIRECT/OFFSET 对每行生成一个范围，再对所有范围聚合）→ 飞书不能直接返回"区域的列表"，必须明确降维：用 `VSTACK` 垂直合并、`HSTACK` 水平合并、`TOCOL/TOROW` 展平，或先把各段结果落到辅助区域再用普通聚合函数汇总
6. 算日期差 → 不要写 `DAY(end-start)`，用 `DAYS`、`DATEDIF` 或直接 `end-start`

## 飞书的投影行为（不是默认 spill）

触发条件是**参数要求单值、实际传入的却是区域**，此时飞书取"投影"而不是"spill"：

- 单列区域 → 按当前公式所在行取值
- 单行区域 → 按当前公式所在列取值
- 二维区域 → 只有当前公式位置能映射到该区域时才取值，否则报错
- 数组常量 `{...}` 或函数返回矩阵，在普通标量上下文里通常只取左上角

**例外是数组公式上下文内部**：最外层套了 `ARRAYFORMULA`、或公式里已有原生数组函数时，同一个区域会逐项展开，不再投影。

因此（以下均指普通公式，即不在数组公式上下文里）：
- `=A1:A2` 在飞书普通公式里不会 spill，只会投影到当前行
- `=ABS(A2:B2)` 不会得到一整行，要写 `=ARRAYFORMULA(ABS(A2:B2))`，或在 A、B 两格分别写 `=ABS(A2)` / `=ABS(B2)`
- `=TRUNC({1.1111,2.222},{1,2})` 要得到一整行，写 `=ARRAYFORMULA(TRUNC({1.1111,2.222},{1,2}))`

## 没有原生数组函数时：ARRAYFORMULA 或逐行填充

**前提：本节适用于公式中没有任何原生数组函数的情况。** 若公式中已有原生数组函数（如 FILTER、XLOOKUP、MAP 等），数组语义会自动传播到整个公式的求值过程（见下一节）。

以下运算与函数**只按标量求值**，直接喂整段区域不会逐项展开：

- 算术运算：`+ - * / ^ %`
- 比较运算：`= <> > >= < <=`
- 标量数学函数：`ABS ROUND INT TRUNC MOD LOG LN SQRT SIN COS TAN ...`
- 文本函数：`LEN LEFT RIGHT MID UPPER LOWER TRIM TEXT VALUE ...`
- 日期函数：`YEAR MONTH DAY DATE TIME EDATE EOMONTH ...`
- 条件函数：`IF IFS IFERROR IFNA NOT ISNUMBER ISTEXT ISBLANK ...`
- 引用函数（高风险）：`INDEX OFFSET COLUMN ROW MATCH`

**两条等价做法，导出 `.xlsx` 后都保真，按需要选一条：**

- **`ARRAYFORMULA(<整个表达式>)`**：一条公式覆盖整片，写起来短。`=ARRAYFORMULA(A2:A100*B2:B100)` ✓、`=ARRAYFORMULA(IF(A2:A100>0,B2:B100,""))` ✓
- **逐行标量式 + 填充**：首行写 `=A2*B2` / `=IF(A2>0,B2,"")`，再用 `--copy-to-range` 铺到整列，引用随行递增。每格是独立公式，导出后在 Excel 里能单格编辑

`MAP` 只在 LAMBDA 体是**纯运算符或单参函数**时可用（如 `=MAP(A2:A100,B2:B100,LAMBDA(a,b,a*b))`）；体内出现 `IF`、多参函数或字符串拼接就改用上面两条路，理由见开头核心原则二。

### 公式中有原生数组函数时，整个公式已进入数组模式

飞书的数组语义会在整个公式求值过程中累积传播：一旦某个原生数组函数运行，后续所有运算符和函数也会自动逐元素处理，无论它们出现在哪一层。

因此以下写法直接成立，不必再包 `ARRAYFORMULA`、也不必拆成逐行填充：

- `=FILTER(A2:A10,B2:B10="x")+1` ✓
- `=XLOOKUP(E2:E10,A2:A10,B2:B10)*100` ✓
- `=ABS(FILTER(A2:A10,B2:B10>0))` ✓
- `=MAP(A2:A10,LAMBDA(x,x*2))-1` ✓

## 原生数组函数清单

以下函数按数组语义工作，可直接返回整片结果，不必拆成逐行填充；且它们在 Excel 侧同样存在，可安全使用：

`CELL` `CHOOSECOLS` `CHOOSEROWS` `DROP` `EXPAND` `FILTER` `FREQUENCY` `GROWTH` `HSTACK` `LINEST` `LOGEST` `LOOKUP` `MINVERSE` `MMULT` `MUNIT` `RANDARRAY` `SEQUENCE` `SORT` `SORTBY` `SUMPRODUCT` `SWITCH` `TAKE` `TEXTSPLIT` `TOCOL` `TOROW` `TRANSPOSE` `TREND` `UNIQUE` `VSTACK` `WRAPCOLS` `WRAPROWS` `XLOOKUP`

`BYCOL` `BYROW` `MAKEARRAY` `MAP` `REDUCE` `SCAN` 同样是原生数组函数，但受核心原则二约束——导出 `.xlsx` 会丢高阶语义，默认改走 `ARRAYFORMULA` / 逐行填充 / 辅助列。

`ARRAYFORMULA` 不在上面这份清单里——它的作用是给**本来只按标量求值**的表达式套上数组语义，而不是自己返回数组。导出 `.xlsx` 时它会被翻译成 Excel 原生数组公式（`=ARRAYFORMULA(IF(A2:A6>2,B2:B6,""))` → `=IF(A2:A6>2,B2:B6,"")`，作用范围覆盖整片），语义完整保留，可安全使用。

> **注意：`SWITCH` 在飞书里被当作原生数组函数处理，这与 Excel 行为不同——把区域喂给它会逐项展开。**

## 跨电子表格取数不要用公式

飞书公式没有跨工作簿引用的通用写法（Excel 的外部链接迁过来也不成立）。需要另一份电子表格的数据时，先把那份数据读出来（`+csv-get` 等）落到本表的一张子表，再在本表内用普通引用计算——既避开跨表引用限制，也保证导出后公式仍可用。

## INDEX / OFFSET / COLUMN / ROW / MATCH 是高风险函数

这组函数容易让人误以为会自动把多值铺开，但在飞书里不能这样假设。

**高风险信号：**

- 行号 / 列号 / 偏移量本身是数组
- 结果本来应该是一行或一块二维区域
- 外层还有算术、比较、`IF` 等继续处理它

更稳的写法：整体包一层 `=ARRAYFORMULA(INDEX(...))` / `=ARRAYFORMULA(ROW(...))`；或退回**当前行的标量式再向下填充**——首行写 `=INDEX($A$2:$A$100,ROW(A1))`，向下填充时 `ROW(A1)` 自动递增为 1、2、3…

**例外：** 如果返回值只是立刻交给聚合函数消费，直接写即可：

- `=SUM(INDEX(A1:B2,0,1))` ✓

## Excel 隐式逐项求值，飞书里要拆辅助列

**典型特征：**

- 外层是 `SUMPRODUCT`、`SUM` 等聚合
- 内层用了 `SUBTOTAL`、`INDIRECT`、`OFFSET` 等更偏"单值/单引用"的函数
- Excel 会把中间结果逐项带进去算
- 飞书里直接照抄，往往不能得到同样的逐项语义

同类本质也包括：`INDEX/INDIRECT/OFFSET` 先返回范围，外层再把这些范围交给 `SUMIF`、`COUNTIF`、`AVERAGEIF`、`SUMIFS` 等范围感知函数 —— 飞书里这些外层函数不会自动二次展开内层范围。

这时要把"遍历"落到**辅助列**上，分两步：

```excel
辅助列首行（如 Z16）：=单行计算逻辑          # 例：=SUBTOTAL(103,INDIRECT("E"&ROW(E16)))
                        用 --copy-to-range 铺满 Z16:Z387（引用随行递增）
汇总格：              =SUM(Z16:Z387)         # 需要时可隐藏辅助列
```

辅助列全是普通标量公式，导出 `.xlsx` 后逐格原样保留，也避开了 `LAMBDA` 系高阶函数的导出陷阱。

**例外：结果要随筛选联动时，保持单条公式。** `SUBTOTAL` 的意义就在于筛选变化后重新计算，这类需求写成

```excel
=SUMPRODUCT(MAP(ARRAYFORMULA(ROW($E$16:$E$387)),LAMBDA(row,SUBTOTAL(103,INDIRECT("E"&row)))))
```

筛选状态本身导出 `.xlsx` 就不会保留，所以这个场景是飞书内专用，不受核心原则二的导出约束。

其余同类场景走辅助列：

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

内层 `INDEX/INDIRECT/OFFSET` 返回的是二维范围，外层还想继续对这些范围做范围计算时，不要假设飞书会"再展开一层"。改用辅助列逐行算再聚合（见上一节），别把二次展开压进单条数组公式。

### 真正的三维或更高维结果不能直接返回

典型触发场景：想把多个不同区域或不同条件的结果合并展示，例如：
- 对 A 列、B 列、C 列分别做 FILTER，想把三列结果并排展示
- 对多个月份分别生成数据行，想把所有月份上下堆叠展示

飞书无法直接返回"多个区域的集合"，必须先决定降维方式：

- 上下堆叠：`=VSTACK(slice1, slice2, slice3)`
- 左右拼接：`=HSTACK(slice1, slice2, slice3)`
- 压成单列：`=TOCOL(...)`
- 压成单行：`=TOROW(...)`
- 只保留聚合值：把各 slice 分别落到辅助区域，再用 `SUM` / `SUMPRODUCT` 等普通聚合函数汇总（`REDUCE` 受核心原则二约束，不要用）

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
- spill 区域未知 → 回到源公式重写，或用 `TAKE` / `DROP` 截取

### 结构化引用

Excel：`=SUM(Table1[Amount])`

飞书不支持结构化引用，改成显式 A1 区域：`=SUM(A2:A100)`

### 老式 CSE 花括号

Excel：`{=A1:A10*B1:B10}`（Ctrl+Shift+Enter 输入）

飞书改为：`=ARRAYFORMULA(A1:A10*B1:B10)`——导出 `.xlsx` 后正好还原成 Excel 的 CSE 数组公式；或首行写 `=A1*B1` 再向下填充

## 日期序列与日期差

飞书日期序列：`0 = 1899-12-30`，`1 = 1899-12-31`，没有 Excel 的 1900 年闰年兼容问题。

**错误写法（不要用）：**

- `=DAY(B2-A2)` ✗ — 差值会被当成日期序列号再拆字段
- `=MONTH(B2-A2)` ✗
- `=YEAR(B2-A2)` ✗

**正确写法：**

- 天数差：`=DAYS(B2,A2)` 或 `=DATEDIF(A2,B2,"D")` 或 `=B2-A2`
- 月份差：`=DATEDIF(A2,B2,"M")`
- 年份差：`=DATEDIF(A2,B2,"Y")`
- 工作日差：`=NETWORKDAYS(A2,B2)`

## 飞书不支持的函数

> 本段是"飞书不支持函数"的**唯一权威清单**。以下函数在飞书里不存在或被禁用，禁止主动使用；用户明确要求时应拒绝并提供替代方案：

- `STOCKHISTORY` — 实时股票数据，飞书无等价函数，需手动导入数据
- `WEBSERVICE` — 外部 HTTP 请求，飞书无等价函数
- CUBE 系列（`CUBEVALUE`、`CUBEMEMBER`、`CUBESET`、`CUBERANK` 等）— OLAP cube 函数，飞书不支持
- `GOOGLEFINANCE`、`GOOGLETRANSLATE` 等 Google 特有函数 — 无等价函数
- `FORECAST.ETS` 系列（`FORECAST.ETS`、`FORECAST.ETS.STAT` 等）— 飞书不支持
- `INFO`、`RTD` — 系统信息 / 实时数据函数，飞书不支持
- `PIVOT` — 用 `+pivot-{create|update|delete}` 透视表对象替代
- `AMORDEGRC`、`PHONETIC`、`DETECTLANGUAGE` — 飞书不支持
- `LET`、命名自定义函数（名称管理器里定义的 LAMBDA）、独立调用的 `LAMBDA`（如 `=LAMBDA(x,x+1)(5)`）— 会报 `#NAME?`；改用嵌套 IF / 辅助列。**例外**：`LAMBDA` 作为 `MAP` / `REDUCE` / `BYROW` / `BYCOL` / `SCAN` / `MAKEARRAY` 的内联参数时飞书**支持**，但受核心原则二约束（导出 `.xlsx` 丢高阶语义），默认仍走逐行填充 / 辅助列

## 代表性改写示例

- 基础逐项计算
  - Excel: `=A2:A100*B2:B100`
  - 飞书: `=ARRAYFORMULA(A2:A100*B2:B100)`；或首行 `=A2*B2` + `--copy-to-range` 向下填充
- 条件判断
  - Excel: `=IF(A2:A100>0,B2:B100,"")`
  - 飞书: `=ARRAYFORMULA(IF(A2:A100>0,B2:B100,""))`；或首行 `=IF(A2>0,B2,"")` + 向下填充（LAMBDA 体含 `IF`，不能用 `MAP`）
- 原生数组函数（无需改动）
  - Excel: `=FILTER(A2:C100,B2:B100="East")`
  - 飞书: `=FILTER(A2:C100,B2:B100="East")`
- 原生数组函数 + 标量运算（无需改动，数组语义自动传播）
  - Excel: `=XLOOKUP(E2:E10,A2:A10,B2:B10)*100`
  - 飞书: `=XLOOKUP(E2:E10,A2:A10,B2:B10)*100`
- 高风险引用函数
  - Excel: `=INDEX(A1:D2,{2,1},0)`
  - 飞书: `=ARRAYFORMULA(INDEX(A1:D2,{2,1},0))`（`col_num=0` 取整行必须包在 `ARRAYFORMULA` 里才成立，裸写会报 `#VALUE!`）
- 日期差
  - 错误: `=DAY(B2-A2)`
  - 推荐: `=DAYS(B2,A2)` 或 `=DATEDIF(A2,B2,"D")` 或 `=B2-A2`
- Excel 隐式逐项求值
  - Excel: `=SUMPRODUCT(SUBTOTAL(103,INDIRECT("E"&ROW($E$16:$E$387))))`
  - 飞书: `=SUMPRODUCT(MAP(ARRAYFORMULA(ROW($E$16:$E$387)),LAMBDA(row,SUBTOTAL(103,INDIRECT("E"&row)))))`（`SUBTOTAL` 要随筛选联动，保持单条公式）
- 多层范围 / 二次展开
  - 错误思路: `=SUMIF(INDIRECT("E"&ROW($E$16:$E$387)),">0")`
  - 飞书: 辅助列 `Z16` 写 `=SUMIF(INDIRECT("E"&ROW(E16)),">0")` 向下填充到 `Z387`
- 三维降二维（保留所有层）
  - 飞书: `=VSTACK(slice1,slice2,slice3)` 或 `=HSTACK(slice1,slice2,slice3)`
- 三维降二维（只保留聚合值）
  - 飞书: 各 slice 落到辅助区域后 `=SUM(辅助区域)`（不要用 `REDUCE`）
