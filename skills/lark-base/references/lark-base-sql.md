# Base SQL

## 选用原则

Base SQL 是实验性的 PostgreSQL 风格只读子集，支持一个 Base 内的跨表查询。环境准入见 [主 Skill 的记录分析](../SKILL.md#记录分析)；进入 SQL 流程后，通过 Catalog 的 `data.supportBaseSQLExecute=true` 确认目标 Base 支持执行。

单条 SQL 返回的结果行数上限以服务端配置为准，明细和分组结果共用此上限，超限会报错。需要完整结果且行数或上限未知、或执行超限时，使用 Record 完整分页后计算。

按下方支持的算子与类型编写 SQL，结合请求次数、传输量和计算成本选择服务端或本地计算。

## 标准操作流程

### 1. Catalog：确认可用性并探索表结构

```bash
lark-cli base +sql-catalog --base-token '<base_token>' --as user > ./catalog.json
```

使用默认 JSON；Catalog 响应局部示例：

```jsonc
{
  "ok": true,
  "identity": "user",
  "data": {
    "supportBaseSQLExecute": true, // 仅 boolean true 才可执行 sql-query；false、缺失或类型不符时回退 Record / data-query
    "tables": [{
      "baseToken": "<base_token>",
      "id": "<table_id>",
      "name": "数据表", // SQL 使用表名，不使用 table_id
      "fields": {
        "字段名": {
          "id": "<field_id>",
          "name": "字段名",
          "baseType": "Text",
          "sqlType": {"kind": "TEXT"},
          "nullable": false
        }
      }
    }]
  }
}
```

### 2. SQL：分析并在需要时规划修改

按下方白名单编写 SQL；相互独立的查询可并行执行。`--sql` 接收字符串、`@file.sql` 或 stdin。

```bash
lark-cli base +sql-query --base-token '<base_token>' \
  --sql 'SELECT "状态", COUNT(*) AS "数量" FROM "任务" GROUP BY "状态"' --as user > ./counts.json &
lark-cli base +sql-query --base-token '<base_token>' \
  --sql 'SELECT "状态", SUM("工时") AS "总工时" FROM "任务" GROUP BY "状态"' --as user > ./hours.json &
wait
```

若分析指定 View，先用 `+view-get-filter --base-token <base_token> --table-id <table_id> --view-id <view_id>` 和 `+view-get-sort --base-token <base_token> --table-id <table_id> --view-id <view_id>` 理解范围与顺序，再转换为白名单 SQL；范围与顺序以实际 View 配置为准。需要在本地处理时，用 Record 的 `--view-id` 读取。

### 3. 写入：切换到 Record Cell Schema

需要读后写时，分析还须形成修改计划：目标表、去重后的 `rec_id` 范围和每条记录的变更动作。聚合结果不是待修改记录，需另行定位真实记录 ID；不能用 LIMIT 截断结果冒充完整范围。

```bash
lark-cli base +sql-query --base-token '<base_token>' \
  --sql 'SELECT rec_id, "标题", "工时" FROM "任务" WHERE "工时" >= 8' --as user > ./targets.json
```

SQL 暂不支持 INSERT / UPDATE / DELETE。执行修改计划时，使用主 Skill 的 [记录新增和更新](../SKILL.md#2-新增记录或更新记录单元格) 或删除工具，转换为逐记录新增 / 更新 / 删除的操作，可批量提交。

**SQL Cell Schema 与主 Skill 的 Record Cell Schema 是两套结构。** 即使部分字段长得相似，也不能直接把 SQL 返回行当成写入 payload。覆盖、递增或数组追加都必须生成符合 Record Cell Schema 的最终单元格值；必要时用 `+record-get` 读取所需单元格。提交数组是整体替换，不会自动追加或合并。

```bash
lark-cli base +record-batch-update --base-token '<base_token>' --table-id '<table_id>' \
  --json '{"update_records":{"<record_id>":{"工时":9,"标签":["高优"],"负责人":[{"id":"ou_example"}]}}}' --as user
lark-cli base +record-batch-create --base-token '<base_token>' --table-id '<table_id>' \
  --json '{"create_records":[{"标题":"任务 B","工时":8}]}' --as user
```

批量大小、串行写入和 `ignored_fields` 处理遵守主 Skill；以写入响应确认结果，无需立即回读。

## SQL 特性白名单

以下列出支持的签名；示例注释说明 Base 类型与组合差异。CTE、聚合回连和窗口组合参见 [已知问题](#当前试验性版本已知问题)。

- **Select Lists / Predicates：** 显式 `SELECT expr [AS alias]`、表别名、`DISTINCT`、`WHERE`；`AND OR NOT`；`= != <> < <= > >=`、`[NOT] IN`、`[NOT] BETWEEN`、`IS [NOT] NULL`、`[NOT] LIKE/ILIKE`。
- **GROUP BY / ORDER BY：** 表达式、别名、序号；`HAVING`；`ASC/DESC`；非负整数 `LIMIT / OFFSET`。
- **Aggregate Functions：** `COUNT(*) / COUNT(expr)`；`COUNT(DISTINCT expr)` 支持数值、TEXT、TIMESTAMP、rec_id；`SUM/AVG([DISTINCT] expr)` 仅 DOUBLE；`MIN/MAX(expr)` 支持数值、TEXT、TIMESTAMP；`ARRAY_AGG([DISTINCT] expr)` 仅数值/TEXT。
- **Conditional / Mathematical / String Expressions：** searched `CASE WHEN ... THEN ... [ELSE ...] END`（可嵌套、多 WHEN、返回 NULL）；二元 `+ - * / %`；`ABS/CEIL/FLOOR(x)`、`ROUND(x[, n])`；`LOWER/UPPER/TRIM/LENGTH(text)`、`CONCAT(text, ...)`、文本 `||`。
- **Date/Time：** `TIMESTAMP '...'`、`CURRENT_TIMESTAMP`；`EXTRACT(YEAR/MONTH/DAY/HOUR/MINUTE/SECOND FROM ts)`；`DATE_TRUNC('year'/'quarter'/'month'/'week'/'day', ts)`；`ts +/- INTERVAL 'n unit'`（n 可负、可小数；unit 为 millisecond/second/minute/hour/day/week）；`ts - ts`。
- **Arrays / Table Functions：** `ARRAY[...]`、`xs @> ys`、`xs <@ ys`、`xs && ys`、`CARDINALITY(xs)`；`FROM t CROSS JOIN UNNEST(t.collection) AS u`，也可逗号连接。
- **Joined Tables / Subqueries：** `[INNER] JOIN ... ON`、`LEFT [OUTER] JOIN ... ON`、`CROSS JOIN`；可连续 JOIN、数值 < / > 连接、ON 附加 AND；`FROM (SELECT ...) AS q`；顶层非递归 `WITH ... AS (SELECT ...)`；物理表间相关等值 `EXISTS`。
- **Window Functions：** `ROW_NUMBER()/RANK() OVER ([PARTITION BY ...] ORDER BY ...)`；`COUNT(*) / SUM/AVG/MIN/MAX(数值列) OVER ()`；`COUNT(*) / COUNT(数值列) / SUM/AVG/MIN/MAX(数值列) OVER (PARTITION BY ...)`。
- **Session Information：** `CURRENT_USER_ID()` 返回 USER_ID。

### Catalog 类型与谓词

表/列名使用 Catalog `data.tables` 的 `name`；类型取 `sqlType.kind` / `element`。以下为子句片段，替换实际名称与选项后组合为查询。所有可查询类型均可投影、判空；NULL 不等于空字符串。公式、Lookup、系统字段也按 Catalog 类型处理。

```sql
-- TEXT：= != <>、[NOT] IN、[NOT] LIKE/ILIKE
WHERE "标题" ILIKE '%计划%' -- LIKE 区分大小写；用 \% / \_ 匹配字面量通配符
-- INT / DOUBLE：= != <> < <= > >=、[NOT] IN、[NOT] BETWEEN
WHERE "工时" >= 8.0
-- BOOL：= TRUE/FALSE
WHERE "完成" = TRUE
-- OPTION：= !=、[NOT] IN；值为已有选项名
WHERE "状态" IN ('Todo', 'Doing')
-- TIMESTAMP：等值/范围比较，同类型时间表达式 <= >=
WHERE "日期" >= TIMESTAMP '2026-09-15 00:00:00'
-- RECORD_ID：等值；系统 rec_id 可与记录 ID 字符串比较
WHERE rec_id = 'rec_example'
-- USER_ID / GROUP_ID：同类型 ID 比较；字符串表示不等于 TEXT，不支持 ID 文本字面量谓词
WHERE "负责人".userId = CURRENT_USER_ID() -- 指定 open_id 筛选转本地
-- UNSUPPORTED 不用于 SQL；DURATION 仅投影、判空，返回毫秒
SELECT "结束时间" - "开始时间" AS duration FROM "任务";
```

### 数值、聚合与排序的差异

```sql
SELECT "状态",
       SUM(CASE WHEN "完成" = TRUE THEN 1.0 ELSE 0.0 END) AS done, -- SUM/AVG 仅 DOUBLE
       ARRAY_AGG("标题") AS titles, -- 丢弃 NULL，与 PG 不同
       COUNT(DISTINCT "标题") AS n
FROM "任务"
GROUP BY "状态";
-- CASE 所有分支类型相同。
-- COUNT DISTINCT：数值、TEXT、TIMESTAMP、rec_id。
-- MIN/MAX：数值、TEXT、TIMESTAMP；ARRAY_AGG：数值、TEXT。

SELECT 5 / 2 AS ratio,                  -- 2.5，整数除法也不截断
       0.0 - "工时" AS negative_hours,  -- 用二元减法取负值
       LENGTH("标题") AS bytes         -- UTF-8 字节数，不是字符数；字符串函数仅接收 TEXT
FROM "任务"
ORDER BY "工时" ASC;                    -- NULL 最前；DESC 时最后，与 PG 相反
-- 显式投影、必须有表；跨请求无快照。
```

### 集合的空值、谓词与展开

```sql
-- COLLECTION<T> / ARRAY<T>：元素类型匹配；OPTION 元素必须是已有选项名。
WHERE "标签" @> ARRAY['高优']             -- 包含全部
WHERE "标签" <@ ARRAY['高优', '外部依赖'] -- 被包含
WHERE "标签" && ARRAY['外部依赖']        -- 相交
WHERE CARDINALITY("标签") >= 2
WHERE "标签" IS NULL                    -- 空 COLLECTION 为 NULL；CARDINALITY(NULL) 为 NULL，不用 = 0

SELECT t.rec_id, u
FROM "任务" AS t CROSS JOIN UNNEST(t."标签") AS u;
-- 单参数 FROM UNNEST；标量成员用 u，OPTION 谓词留在原集合列。
-- 数字公式按 Catalog 类型处理；COLLECTION<DOUBLE> 使用集合操作。
```

### 对象成员与关联 ID

```sql
SELECT rec_id, "负责人".name, "官网".link
FROM "任务"
WHERE "负责人".name LIKE '张%'           -- USER/GROUP 的 name，URL 的 link/text 均为 TEXT
  AND "官网".link LIKE 'https://example.com%';
-- 引用最多两段：t."工时" 或 "负责人".userId。

SELECT t.rec_id, u.userId, u.name
FROM "任务" AS t CROSS JOIN UNNEST(t."协作人") AS u
WHERE u.name LIKE '张%';                -- COLLECTION<USER>

SELECT t.rec_id, g.id, g.name
FROM "任务" AS t CROSS JOIN UNNEST(t."相关群聊") AS g; -- COLLECTION<GROUP>

SELECT t.rec_id, f.id, f.name
FROM "任务" AS t CROSS JOIN UNNEST(t."附件") AS f
WHERE f.name = 'sample.txt';            -- COLLECTION<FILE>；id/name 为 TEXT

SELECT a.rec_id, b.rec_id AS project_id
FROM "任务" AS a JOIN "项目" AS b ON a."关联项目" @> ARRAY[b.rec_id];
-- LEFT JOIN 后可用右侧 rec_id IS NULL 判断未匹配。
```

### CTE：成员与时间计算

```sql
WITH q AS (
  SELECT rec_id, "负责人".name AS name,
         EXTRACT(YEAR FROM "日期") AS year, -- INT
         "日期" - INTERVAL '1 day' AS previous_day
  FROM "任务"
)
SELECT rec_id, name, year, previous_day FROM q;
-- 对象成员与时间表达式在原表层计算后传出；相关测试记录见下文。
-- 时间计算按 Base 时区，TIMESTAMP/DURATION 返回毫秒。
-- 时间范围连接的已知问题见下文。
```

### CTE：分阶段聚合与回连

```sql
WITH q AS (
  SELECT "标题" AS title, "工时" AS hours FROM "任务"
), g AS (
  SELECT title, SUM(hours) AS total, COUNT(*) AS n
  FROM q GROUP BY title HAVING COUNT(*) > 1
)
SELECT title, total / n AS average FROM g WHERE total > 8.0;
-- 可再 SELECT SUM(total)、COUNT(*)、MAX(n) FROM g：再次聚合、数分组、分组计数取最值。
-- 计算列/CASE 后筛选、DISTINCT 后 COUNT、LIMIT/OFFSET 后再筛选/排序/聚合也可用。

WITH q AS (
  SELECT rec_id, "标题" AS title FROM "任务" -- 公共明细 CTE
), g AS (
  SELECT title, COUNT(*) AS n FROM q GROUP BY title
)
SELECT q.rec_id, g.n FROM q JOIN g ON q.title = g.title;
-- 公共明细 CTE 供聚合和回连复用；相关测试记录见下文。

WITH g AS (
  SELECT "标题" AS title, ARRAY_AGG("工时") AS xs FROM "任务" GROUP BY "标题"
)
SELECT title, CARDINALITY(xs) AS n FROM g;
-- 原始集合经 CTE 透传后可以 UNNEST，再按展开元素分组 COUNT。
```

### CTE：复用与层数

```sql
WITH q AS (
  SELECT rec_id, "工时" AS hours FROM "任务"
), a AS (
  SELECT rec_id, hours + 1.0 AS x FROM q
), b AS (
  SELECT rec_id, hours * 2.0 AS y FROM q
)
SELECT a.rec_id, a.x, b.y FROM a JOIN b ON a.rec_id = b.rec_id;
-- 顶层非递归、后引用前；可重复引用、自连接、分叉回连、LEFT JOIN 后右侧 ID 判空。
-- CTE 与普通 FROM (SELECT ...) AS d 可相互组合。
-- 线性 8 个 CTE 已验证，9 个触发子查询预算；显式输出列。

SELECT a.rec_id FROM "任务" AS a
WHERE EXISTS (SELECT b.rec_id FROM "任务" AS b WHERE b.rec_id = a.rec_id);
-- 相关等值 EXISTS：物理表间。
```

### 窗口：分阶段计算与后续操作

```sql
WITH g AS (
  SELECT "标题" AS title, SUM("工时") AS total FROM "任务" GROUP BY "标题"
)
SELECT title, total, RANK() OVER (ORDER BY total DESC) AS rank
FROM g ORDER BY total DESC;             -- 先聚合、下一层排名

WITH r AS (
  SELECT rec_id, "标题" AS title, ROW_NUMBER() OVER (ORDER BY rec_id) AS rn
  FROM "任务"
)
SELECT rec_id, rn FROM r
WHERE title = '任务 A'                  -- 可按非窗口列筛选
ORDER BY rn LIMIT 3;                    -- 可投影、排序/LIMIT、按记录 ID JOIN
-- 窗口值不能用于外层 WHERE、JOIN ON 或再次聚合。
-- RANK 并列跳号，最终行顺序另用 ORDER BY。
-- SUM OVER (ORDER BY ...) 仍为整组值，不是 PG 默认累计 frame。
```

## SQL Cell Schema

对象成员类型（查询返回结构）：

```text
USER  = {userId: USER_ID, name: TEXT}
GROUP = {id: GROUP_ID, name: TEXT}
FILE  = {id: TEXT, name: TEXT}
URL   = {link: TEXT, text: TEXT}
```

`USER_ID` 为 open_id，`GROUP_ID` 为 open_chat_id，FILE 的 `id` 为附件 file_token，均以字符串返回。谓词范围见上方类型说明。

**SQL Cell Schema**：`+sql-query` 的单元格结构由 Catalog 的 SQL 类型及查询投影决定。必须和 `Record Cell Schema` 加以区分！以下为直接投影的默认响应示例；集合类型返回数组，对象类型返回对象。

```jsonc
{
  "ok": true,
  "identity": "user",
  "data": {
    "rows": [{
      "rec_id": "rec_example", // RECORD_ID
      "标题": "任务 A", // TEXT
      "金额": 100.5, // DOUBLE
      "完成": false, // BOOL
      "状态": "Todo", // OPTION
      "标签": ["高优", "外部依赖"], // COLLECTION<OPTION>
      "日期": 1774317600000, // TIMESTAMP：Unix 毫秒
      "单值人员": {"userId": "ou_123", "name": "示例人员"}, // USER
      "多值人员": [{"userId": "ou_123", "name": "示例人员"}], // COLLECTION<USER>
      "单值群聊": {"id": "oc_123", "name": "示例群聊"}, // GROUP
      "多值群聊": [{"id": "oc_123", "name": "示例群聊"}], // COLLECTION<GROUP>
      "关联任务": ["rec_target"], // COLLECTION<RECORD_ID>
      "附件": [{"id": "file_example", "name": "sample.txt"}], // COLLECTION<FILE>
      "官网": {"link": "https://example.com", "text": "示例网站"}, // URL
      "备注": null // NULL
    }]
  }
}
```

检查退出码、`ok` 后读 `data.rows`；空数组表示成功但无结果。Catalog 表结构在 `data.tables`。

**SQL Cell Schema 按 Catalog 类型及投影解析；Record Cell Schema 的 NDJSON 按配套 manifest 解析。** JSON/NDJSON 仅控制输出封装，不会将两套 Cell Schema 相互转换。写入使用 [主 Skill 的 Record Cell Schema](../SKILL.md#2-新增记录或更新记录单元格)。

SQL 仍受调用身份的访问权限约束；不尝试通过切换身份绕过权限或能力限制。

`--dry-run` 仅校验本地输入、展示请求，不验证 SQL 执行。失败或结果异常时查 [常见 SQL 失败模式](#常见-sql-失败模式)。

## 常见 SQL 失败模式

以下记录已确认的失败与替代写法；未列入白名单不等于已证明服务端不支持。

### 已确认拒绝的写法

#### 查询、排序与筛选

1. **按指定列选择代表行的去重不支持。** 使用普通 DISTINCT，或在本地选择代表行。
2. **显式指定 NULL 排序位置不支持。** 可用 CASE 生成空值排序键，再按业务字段排序。
3. **NULL 安全的相异比较、正则匹配、数组量化比较不支持。** NULL 使用判空与逻辑条件组合，集合使用成员谓词；正则转本地处理。
4. **对称区间比较不支持。** 先确定上下界顺序，再使用普通 BETWEEN。

#### 表达式、类型与时间

1. **通用类型转换（`CAST(...)` / `expr::type`）、空值处理函数不支持。** 条件选择使用 searched CASE；函数返回 INT 而后续需要 DOUBLE 时，暂无已验证的通用转换写法，有限值域可用 CASE 映射到 DOUBLE 分支。
2. **时间点提取 epoch、时间/数字格式化、格式化文本转数字不支持。** 在本地处理返回的毫秒时间值和文本。
3. **按月或年加减 INTERVAL、多单位组成的 INTERVAL 不支持。** 日历月/年运算转本地处理，固定天数不能等价替代；多单位固定时长可先换算为一个白名单单位。
4. **位置经度子字段投影不支持。** 读取完整位置对象后在本地访问属性。

#### 聚合与窗口

1. **聚合内部排序不支持。** 有序收集转本地处理。
2. **显式窗口 frame、命名窗口、紧密排名、前后行取值、窗口首值函数不支持。** 依赖 frame 或相邻行的计算转本地处理。
3. **分组集合、层级汇总、立方体分组不支持。** 可拆为多个普通 GROUP BY 查询，再合并结果。
4. **布尔 AND/OR 聚合、总体/样本标准差及方差函数不支持。** 布尔聚合可通过条件计数改写，并处理 NULL 与空集；统计量可转本地计算。

#### 子查询、JOIN 与集合运算

1. **标量子查询、子查询成员判断、NOT EXISTS 不支持。** 可用相关 EXISTS、JOIN、CTE 或派生表等价改写，并保持重复行和 NULL 语义。
2. **CTE 列名列表、物化提示、横向相关派生表不支持。** 列别名写在 SELECT 中；依赖计算可拆分查询。
3. **带序号的集合展开、通过内连接或左连接展开集合不支持。** 需要序号或保留空集合主记录时，可在本地处理。
4. **右连接、全外连接、自然连接、同名列隐式连接不支持。** 右连接可交换表后使用左连接，同名列连接改为显式 ON；全外连接可拆分后在本地合并。
5. **序列生成表函数不支持。** 日历或数值序列在本地构造。
6. **并集、交集、差集查询运算不支持。** 分别查询后在本地处理，保留各运算的去重与重复行语义。

#### 其他数学、字符串与数组函数

1. **截断、平方根、幂函数与幂运算符、符号函数不支持。** 符号判断可用 CASE，其他运算可转本地处理。
2. **GREATEST/LEAST 条件表达式不支持。** 可用 CASE 改写，并处理 NULL。
3. **字符串聚合、替换、子串截取、正则替换、带分隔符拼接、按分隔符取段、子串位置查找不支持。** 使用白名单的普通拼接或转本地处理。
4. **数组元素位置查找、下标访问、切片、数组拼接运算符及拼接函数不支持。** 在本地处理数组。

### 当前试验性版本已知问题

以下保留原有测试记录，结论限定于记录中的测试组合。遇到对应问题时，选择记录中的等价写法或转本地计算。

| 测试组合 | 测试中观察到的结果 |
| --- | --- |
| CTE / 派生表透传人员对象后，外层读取成员 | 执行错误；原表层先提取成员再传出可用 |
| CTE 透传时间列后，外层 INTERVAL 加减、EXTRACT / DATE_TRUNC | 测试中加减返回 NULL，函数调用报错；原表层计算再传出通过 |
| 原表直接 JOIN 按文本键聚合的 CTE / 派生表 | 已有测试记录返回错误空集；上方公共 CTE 写法通过 |
| 带 LIMIT 的公共 CTE，明细与聚合结果等值 ON 回连 | 测试组合返回执行错误；同义 CROSS JOIN + 等值 WHERE 已验证正确 |
| CTE 时间范围自连接：`b.dt <= a.dt AND b.dt >= a.dt - INTERVAL '1 day'` | 测试中 ON 返回错误空集或计入范围外记录；CROSS JOIN + WHERE 忽略下界，预计算下界后测试仍失败 |
| ARRAY_AGG 后 UNNEST | 执行错误；ARRAY_AGG 后 CARDINALITY 可用 |
| 外层使用 CTE 的相关 EXISTS | 执行错误；物理表间相关等值 EXISTS 可用 |
| CTE 与输出列同名，使用未限定列名 | 内部类型检查错误；限定为 `q.q` 可用 |
| `SUM(x) OVER (ORDER BY ...)` | 返回整组总和，而非 PG 默认累计值；三行 `SUM(1.0)` 返回 `3,3,3`，而非 `1,2,3` |

### 处理失败与能力边界

错误输出到 stderr；按结构化 `error.type`、`error.code`、`retryable`、`log_id` 及实际返回的恢复提示判断。SQL 定位信息从实际错误响应中读取，包括 message 中的 phase/span/action。

| 失败阶段或现象 | 下一步 |
| --- | --- |
| CLI 输入错误 | 检查 SQL 文本与文件输入；无需重查 Catalog |
| 未知表、列或 schema 变化 | 用原身份重读 Catalog，核对名称与可见性后修改 SQL |
| 类型不匹配或选项无效 | 对照 `sqlType`、集合元素类型、合法选项名修正表达式 |
| 公开 ID 文本操作被拒绝 | 仅使用同类型 ID 比较或直接投影；指定 ID 的文本筛选转本地处理 |
| SQL 结构、字段算子或组合不支持 | 按 phase/span 定位表达式，改为已支持的等价写法；没有可靠等价写法时转本地计算 |
| scope / 资源权限不足 | 通过 `lark-shared` 或原生权限流程修复原调用身份；不切换 Owner 或忽略权限条件 |
| Catalog、查询复杂度或结果预算超限 | 结果行数超限按上方选用原则处理；字节数超限可精简返回字段；复杂度超限可拆为有界查询或本地计算；Catalog 超限转 Record / Field 路径 |
| 限流或可重试暂态故障 | 遵守返回的重试时间并有限退避；不连续原样重试，也不换身份规避限流 |

保留 SQL、类型和 log_id 作为诊断依据，分享时脱敏业务值。
