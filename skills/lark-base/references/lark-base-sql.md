# Base SQL

## 选用原则

Base SQL 是实验性的 PostgreSQL 风格只读子集，支持一个 Base 内的跨表查询。环境准入见 [主 Skill 的记录分析](../SKILL.md#记录分析)；进入 SQL 流程后，通过 Catalog 的 `data.supportBaseSQLExecute=true` 确认目标 Base 支持执行。

单条 SQL 返回的结果行数上限以服务端配置为准，明细和分组结果共用此上限，超限会报错。需要完整结果且行数或上限未知、或执行超限时，使用 Record 完整分页后计算。

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

按下方特性白名单编写 SQL。`--sql` 接收字符串、`@file.sql` 或 stdin。

```bash
lark-cli base +sql-query --base-token '<base_token>' \
  --sql 'SELECT "状态", COUNT(*) AS "数量" FROM "任务" GROUP BY "状态"' --as user > ./counts.json
```

若分析指定 View，先用 `+view-get-filter --base-token <base_token> --table-id <table_id> --view-id <view_id>` 和 `+view-get-sort --base-token <base_token> --table-id <table_id> --view-id <view_id>` 理解范围与顺序，再转换为 SQL；范围与顺序以实际 View 配置为准。需要在本地处理时，用 Record 的 `--view-id` 读取。

### 3. 写入：切换到 Record Cell Schema

**SQL 只读，写入通过 Record API 完成。** 需要读后写时，分析还须形成修改计划：目标表、去重后的 `rec_id` 范围和每条记录的变更动作。SQL 使用表名，Record 写入使用 `table_id` 与记录 ID；从 Catalog 获取目标表 ID，以查询中的 `rec_id` 定位记录。

```bash
lark-cli base +sql-query --base-token '<base_token>' \
  --sql 'SELECT rec_id, "标题", "工时" FROM "任务" WHERE "工时" >= 8' --as user > ./targets.json
```

执行修改计划时，使用主 Skill 的 [记录新增和更新](../SKILL.md#2-新增记录或更新记录单元格) 或删除工具，转换为逐记录新增 / 更新 / 删除的操作，可批量提交。

**SQL Cell Schema 与主 Skill 的 Record Cell Schema 是两套结构。** 写入前按 Record Cell Schema 映射字段值。覆盖、递增或数组追加都必须生成符合 Record Cell Schema 的最终单元格值；必要时用 `+record-get` 读取所需单元格。提交数组按整体替换处理；追加时先合并已有值，再提交完整数组。

批量大小、串行写入和 `ignored_fields` 处理遵守主 Skill；以写入响应确认结果，无需立即回读。

## PG 特性白名单

以下列出支持的签名；示例注释说明类型与组合规则。CTE、聚合回连和窗口组合参见 [差异提醒](#差异提醒)。

- **Select Lists / Predicates：** `SELECT expr [AS alias]` / `SELECT *`、表别名、`DISTINCT` / `DISTINCT ON (...)`、`WHERE`；`AND OR NOT`；`= != <> < <= > >=`、`[NOT] IN`、`[NOT] BETWEEN`、`IS [NOT] NULL`、`[NOT] LIKE/ILIKE`。
- **GROUP BY / ORDER BY：** 表达式、别名、序号；`HAVING`；`ASC/DESC`；非负整数 `LIMIT / OFFSET`；`GROUPING SETS` / `ROLLUP` / `CUBE`。
- **Aggregate Functions：** `COUNT(*) / COUNT(expr)`；`COUNT(DISTINCT expr)` 支持数值、TEXT、TIMESTAMP、rec_id、OPTION；`SUM/AVG([DISTINCT] expr)` 仅 DOUBLE；`MIN/MAX(expr)` 支持数值、TEXT、TIMESTAMP；`ARRAY_AGG([DISTINCT] expr)` 仅数值/TEXT。
- **Conditional / Mathematical / String Expressions：** searched `CASE WHEN ... THEN ... [ELSE ...] END`（可嵌套、多 WHEN、返回 NULL）；二元 `+ - * / %`；`ABS/CEIL/FLOOR(x)`、`ROUND(x[, n])`；`LOWER/UPPER/TRIM/LENGTH(text)`、`CONCAT(text, ...)`、`REPLACE(text, from, to)`、文本 `||`；`NULLIF` 与类型兼容的 `COALESCE`。
- **Type Conversion：** `CAST(text AS DOUBLE)` / `text::DOUBLE`；INT 聚合输入可用 `x * 1.0` 形成 DOUBLE。
- **Date/Time：** `TIMESTAMP '...'`、`CURRENT_TIMESTAMP`；`EXTRACT(YEAR/MONTH/DAY/HOUR/MINUTE/SECOND FROM ts)`；`DATE_TRUNC('year'/'quarter'/'month'/'week'/'day', ts)`；`ts +/- INTERVAL 'n unit'`（n 可负、可小数；unit 为 millisecond/second/minute/hour/day/week）；`ts - ts`。
- **Arrays / Table Functions：** `ARRAY[...]`、`xs @> ys`、`xs <@ ys`、`xs && ys`、`CARDINALITY(xs)`；`FROM t CROSS JOIN UNNEST(t.collection) AS u`，也可逗号连接。
- **Joined Tables / Subqueries：** `[INNER] JOIN ... ON`、`LEFT [OUTER] JOIN ... ON`、`CROSS JOIN`；可连续 JOIN、数值 < / > 连接、ON 附加 AND；`FROM (SELECT ...) AS q`；顶层非递归 `WITH ... AS (SELECT ...)`（支持 CTE 列名列表）；标量子查询、`IN (SELECT ...)`、单相关键等值 `EXISTS / NOT EXISTS`；`UNION / UNION ALL`。多键相关条件按下方执行差异规划。
- **Window Functions：** `ROW_NUMBER()/RANK()/DENSE_RANK() OVER ([PARTITION BY ...] ORDER BY ...)`、`FIRST_VALUE(数值列) OVER (...)`、命名 `WINDOW`；`COUNT(*) / SUM/AVG/MIN/MAX(数值列) OVER ()`；`COUNT(*) / COUNT(数值列) / SUM/AVG/MIN/MAX(数值列) OVER (PARTITION BY ...)`。
- **Session Information：** `CURRENT_USER_ID()` 返回 USER_ID。

### Catalog 类型与谓词

SQL 表/列名使用 Catalog `data.tables` 的 `name`，类型取 `sqlType.kind` / `element`；公式、Lookup、系统字段也按此类型处理。所有可查询类型均可投影、判空。

| 类型 | 谓词与边界 |
| --- | --- |
| TEXT | `= != <>`、`[NOT] IN`、`[NOT] LIKE/ILIKE` |
| INT / DOUBLE | `= != <> < <= > >=`、`[NOT] IN`、`[NOT] BETWEEN` |
| BOOL | `= TRUE/FALSE` |
| TIMESTAMP | 等值、范围比较与同类型时间表达式 |
| OPTION | `= !=`、`[NOT] IN`；字面量使用已有选项名。OPTION 是独立类型，按其类型使用算子；与 TEXT 混合的 UNION 分支会触发类型错误 |
| RECORD_ID | 等值；系统 `rec_id` 可与记录 ID 字符串比较，如 `rec_id = 'rec_example'` |
| USER_ID / GROUP_ID | 支持同类型比较，如 `"负责人".userId = CURRENT_USER_ID()`；与 TEXT 字面量比较会被拒绝 |
| DURATION | 仅投影、判空；时间戳相减返回该类型，响应为毫秒 |
| UNSUPPORTED | 通过 Record API 读取 |

### LIKE 模式匹配

1. `%` 匹配零个或多个字符，`_` 匹配单个字符（包括中文）；模式匹配覆盖整个字符串。
2. `LIKE / NOT LIKE` 区分大小写，`ILIKE / NOT ILIKE` 忽略大小写。
3. 默认使用反斜杠转义：`\%`、`\_` 匹配字面量 `%`、`_`。
4. 模式支持 TEXT 表达式，例如 `LIKE CONCAT('a', '%')`。

### 数值、聚合与排序的差异

```sql
SELECT "状态",
       SUM(CASE WHEN "完成" = TRUE THEN 1.0 ELSE 0.0 END) AS done, -- SUM/AVG 仅 DOUBLE
       ARRAY_AGG("标题") AS titles,
       COUNT(DISTINCT "标题") AS n
FROM "任务"
GROUP BY "状态";

SELECT 5 / 2 AS ratio,                  -- 整数相除返回 2.5
       0.0 - "工时" AS negative_hours,  -- 用二元减法取负值
       LENGTH("标题") AS bytes         -- UTF-8 字节数；字符串函数接收 TEXT
FROM "任务"
ORDER BY "工时" ASC;
```

### 集合谓词与展开

```sql
-- COLLECTION<T> / ARRAY<T>：元素类型匹配；OPTION 元素必须是已有选项名。
WHERE "标签" @> ARRAY['高优']             -- 包含全部
WHERE "标签" <@ ARRAY['高优', '外部依赖'] -- 被包含
WHERE "标签" && ARRAY['外部依赖']        -- 相交
WHERE CARDINALITY("标签") >= 2
WHERE "标签" IS NULL

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
WHERE u.name LIKE '张%';
-- 对象集合统一用 UNNEST 后读取成员：USER 为 userId/name，GROUP 为 id/name，FILE 为 id/name。
-- USER_ID/GROUP_ID 按专用 ID 类型处理；FILE 的 id/name 为 TEXT。

SELECT a.rec_id, b.rec_id AS project_id
FROM "任务" AS a JOIN "项目" AS b ON a."关联项目" @> ARRAY[b.rec_id];
```

### 日期：物理表层运算与月份键

```sql
WITH q AS (
  SELECT rec_id,
         "日期" - INTERVAL '1 day' AS previous_day,
         EXTRACT(YEAR FROM "日期") * 12 + EXTRACT(MONTH FROM "日期") AS month_key
  FROM "数据表"
)
SELECT rec_id, previous_day, month_key FROM q;
-- 月/年 INTERVAL 的月份比较可用连续 month_key 替代；精确日期位移需另行处理日历边界。
-- 时间计算按 Base 时区；TIMESTAMP/DURATION 返回毫秒。时间范围连接差异见下文。
```

### 综合示例：CTE、聚合回连与窗口

```sql
WITH details AS (
  SELECT t.rec_id, t."标题" AS title, t."工时" AS hours,
         "负责人".name AS owner_name
  FROM "任务" AS t
  WHERE t."完成" = TRUE
), grouped AS (
  SELECT title, SUM(hours) AS total_hours, COUNT(*) AS record_count
  FROM details
  GROUP BY title
  HAVING COUNT(*) > 1
), ranked AS (
  SELECT title, total_hours, record_count,
         RANK() OVER (ORDER BY total_hours DESC) AS group_rank
  FROM grouped
)
SELECT d.rec_id, d.title, d.owner_name, d.hours,
       r.total_hours, r.record_count, r.group_rank
FROM details AS d
JOIN ranked AS r ON d.title = r.title
WHERE r.group_rank <= 3
ORDER BY r.group_rank, d.rec_id;
-- 中文表/字段名加双引号；对象成员在物理表层投影后传入 CTE。
```

支持 CTE 重复引用、自连接、分叉回连，以及与 `FROM (SELECT ...)` 组合；计算列筛选、聚合回连、ARRAY_AGG 后 UNNEST、窗口结果的外层 WHERE/JOIN/再次聚合均可用。相关组合缺陷与预算见 [差异提醒](#差异提醒)。

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

**SQL Cell Schema 按 Catalog 类型及投影解析；Record Cell Schema 的 NDJSON 按配套 manifest 解析。** JSON/NDJSON 控制输出封装；单元格结构仍按各自 Cell Schema 解析。写入使用 [主 Skill 的 Record Cell Schema](../SKILL.md#2-新增记录或更新记录单元格)。

`--dry-run` 校验本地输入并展示请求；执行语义通过实际查询确认。失败或结果异常时查 [差异提醒](#差异提醒)。

## 明确拒绝的特性

以下签名已确认被拒绝；其余扩展语法以命令返回为准。

- **连接与子查询：** `WITH RECURSIVE`、`LATERAL`、`RIGHT/FULL/NATURAL JOIN`、`USING`、`INTERSECT/EXCEPT`。
- **聚合与窗口：** 聚合内 `ORDER BY`、显式 `ROWS/RANGE` frame、`LAG/LEAD`、`BOOL_AND/BOOL_OR`、总体/样本标准差及方差函数。
- **集合与表函数：** `GENERATE_SERIES`、`UNNEST WITH ORDINALITY`、LEFT/INNER JOIN UNNEST、`ARRAY_POSITION`、数组下标/切片、数组 `||`、`ARRAY_CAT`。
- **时间与类型转换：** `EXTRACT(EPOCH)`、`TO_CHAR`、`TO_NUMBER`、月/年或多单位 INTERVAL；`INT/BOOL → DOUBLE` 和 `DOUBLE → TEXT` 的 CAST。
- **排序与谓词：** `NULLS FIRST/LAST`、`IS DISTINCT FROM`、正则运算符、`ANY`、`BETWEEN SYMMETRIC`；LIKE 的 `ESCAPE` 子句、`LIKE ANY(...)`、`COLLATE`。
- **数值与文本函数：** `TRUNC/SQRT/POWER/^/SIGN`、`GREATEST/LEAST`、`STRING_AGG/SUBSTRING/REGEXP_REPLACE/CONCAT_WS/SPLIT_PART/POSITION`。

人员对象成员在物理表层投影后传入 CTE；位置字段通过 Record API 读取。

## 差异提醒

### 空值语义与类型推导

- **类型推导：** 裸 NULL 用于日期函数、时间运算或 `COALESCE` 首参时会触发类型检查错误。使用已知类型的列或表达式；`COALESCE` 的备用值与其类型一致。
- **集合：** 空 COLLECTION 表示为 NULL；`CARDINALITY(NULL)` → NULL（不是 0）。
- **聚合：** `ARRAY_AGG` 的结果数组省略 NULL 元素。
- **排序：** ASC 将 NULL 排在最前，DESC 将 NULL 排在最后。

### 查询组合

| 查询组合 | 实际行为与推荐写法 |
| --- | --- |
| CTE 透传 TIMESTAMP 后外层 ± INTERVAL | 当前实现缺陷：有效 TIMESTAMP 的外层运算会返回 NULL。临时绕过：在物理表层先计算时间边界再传出。外层 EXTRACT / DATE_TRUNC 可直接使用 |
| 物理表 JOIN 按文本键聚合的 CTE，再 LIMIT/OFFSET | 分页可能遗漏匹配行；使用公共明细 CTE 聚合后回连并返回有界完整结果，或分别查询聚合结果和明细再合并 |
| 时间范围条件放 JOIN ON | 范围条件可能保留额外匹配行；物理表使用 CROSS JOIN + WHERE 表达时间范围，再分组计算 |
| 多键相关 EXISTS / NOT EXISTS | 相关条件可能丢失部分匹配；使用常量范围加单个等值相关键，或按完整连接键去重后 JOIN |
| Link 集合关联谓词放 CROSS JOIN 后的 WHERE | 可能触发记录 ID 集合算子错误；关联集合条件使用 JOIN ON，时间等筛选条件放 WHERE |
| SUM(x) OVER (ORDER BY ...) | 返回整个分区的总和；累计值使用物理表 CROSS JOIN + WHERE 范围约束后分组汇总，或在本地计算 |
| 多阶段时间条件的 CTE/连接链 | 可能丢失阶段约束或返回执行错误；拆成逐阶段查询，将上一阶段实际结果作为下一阶段的范围与时间边界 |

查询预算：JOIN 上限为 10，计算子查询上限为 8，结果行数上限由服务端配置决定。透传 CTE 的优化与计算节点预算分别处理。需要完整明细时使用完整分页路径。

### 处理失败与能力边界

SQL 错误的定位信息来自响应中的 phase/span/action；保留 SQL、类型与 log_id 便于诊断，分享时脱敏业务值。

- 结果行数超限：转 Record 完整分页；字节数超限：精简返回字段。
- JOIN/子查询预算超限：拆为有界查询或本地计算；Catalog 超限：转 Record / Field 路径。
- USER_ID/GROUP_ID 的指定 ID 文本筛选：读取 ID 后在本地比较。
