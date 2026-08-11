# Base 数据表查询与分析 SOP

数据表记录查询和分析任务先读本 SOP，包括记录预览、筛选、排序、去重、统计、聚合、TopN、多值计算、Link 或多表关联、复杂行级计算、全局结论和查询后写入。先区分需要 LLM 理解原文的语义分析与可程序化计算的确定性分析，再按任务所需数据规模与计算复杂度选择对应路径。用户直接要求解释、编写或排错 `+data-query` 命令或 DSL 时，直接读 [data-query guide](lark-base-data-query-guide.md)。

## 分流决策

1. 明确所有需要参与分析的表及其 `records_count`。
2. 如果结论必须依赖 LLM 理解原始内容，例如开放文本打标、情绪或意图识别、主题归纳、语义分类、相似性判断或实体消歧，进入下文“LLM 语义分析”路径。
3. 对于其余确定性查询，任一分析表超过 2000 行时，先从任务意图中为所有大表提取可在单表内独立执行的谓词，例如日期范围、状态和关键词，再按下文将谓词逐表下推，并用 `--field-id '<一个简单标量字段>' --limit 2000 --output <probe>.ndjson --minimal-stdout` 探测。目标是每张表都达到 `has_more=false`；任一表无法压缩到 2000 行以内时，转 [lark-base-data-analysis-cloud.md](lark-base-data-analysis-cloud.md) 用云端的数据分析能力。
4. 所有分析表都不超过 2000 行后：若只有一张表且短 jq 可清晰完成筛选、计数、简单分组/聚合/排序、TopN 可以使用 jq。
5. 其余确定性任务比如多表、日历计算和复杂数据分析，在 Python 可用时使用 Python，否则进入 [Cloud SOP](lark-base-data-analysis-cloud.md)。

进入 Cloud 后先由 Cloud SOP 在原始记录查询与聚合查询之间选路；只有选定 `+data-query` 时才读取 data-query guide。

## 执行与交付

分析输入默认采用 `--output x.ndjson`；NDJSON 未显式传 `--limit` 时默认读取最多 2000 条，正式分析通常沿用该范围。窄投影探测、快速预览或用户明确要求前 N 条时再设置较小的 `--limit`。`--format json` 和 Markdown 适用于向用户即时展示的小结果。

缩小大表记录范围时，展示文本关键词用 `+record-search`，日期、状态、数字、空值、选项、人员和关联等结构化条件用 `+record-list --filter-json`。

### 单表谓词下推常用 example

`+record-list` / `+record-search` 的 `--filter-json '<filter-json>'` 支持使用 tuple condition 下推单表谓词。以下示例用注释说明各条件的含义；实际传参时删除注释并使用标准 JSON：

```jsonc
{
  "logic": "and", // 所有 conditions 同时成立；任意一个成立时使用 "or"
  "conditions": [
    ["标题", "==", "Launch plan"], // 文本全等
    ["标题", "intersects", "urgent"], // 文本包含目标片段
    ["金额", ">=", 100], // 数字比较；支持 ==、!=、>、>=、<、<=
    ["状态", "intersects", ["进行中", "暂停"]], // Select 集合相交：包含“进行中”或“暂停”任意一个选项
    ["状态", "disjoint", ["已终止"]], // Select 集合无交集
    ["已完成", "==", true], // Checkbox
    ["负责人", "intersects", [{"id": "ou_xxx"}]], // 负责人包含某个人；intersects 表示包含数组中任意一个人员
    ["关联项目", "intersects", [{"id": "rec_xxx"}]], // 关联项目包含某个 record_id；intersects 表示包含数组中任意一条关联
    ["备注", "non_empty"], // 非空判断；标量 null 和多值空数组都是空
    ["业务日期", "==", "ExactDate(2026-08-07)"], // 具体一天：按 Base 时区匹配 2026-08-07 当天
    ["发生时间", ">", "ExactDate(2024-01-31 23:59:59.999)"], // 日期不支持 >=；用 > 前一天最后一毫秒表达含当天的下界
    ["发生时间", "<", "ExactDate(2024-03-01 00:00:00)"] // 2024 年 2 月范围上界：小于 3 月 1 日零点
  ]
}
```

全表分析的常规资源链路是 `+table-list` 确认目标表与规模，对所有参与分析的表并发执行 `+field-list` 读取所需 schema，再用 `+record-list` 导出记录；已有可信的 `table_id` 时可直接并发读取各表 `+field-list`。`+view-get` 可按需读取，作为用户持久化访问习惯的可选参考；其中的 filter、sort 与字段范围可辅助理解用户常用的查询范围和排序偏好，并结合当前任务确定最终口径。

1. 每次读取使用任务所需的最小投影，并包含 JOIN、解释、回查或写入需要的业务 key。
2. 全局结论以 `has_more=false` 的完整导出或 Cloud 聚合结果为依据；`has_more=true` 时继续收敛单表谓词或选择 Cloud 路径。
3. 确定性分析选定一个分析引擎直接读取 NDJSON；模型上下文仅接收预览或最终小结果。
4. Base 标量空值很常见；聚合前按用户口径确定空值是排除、按零计入还是进入分母。用户未指定且不同处理会实质改变结论时，说明空值数量、采用的口径及其影响；任务涉及业务键、展开、JOIN 或金额分摊时，同样明确目标粒度及与口径直接相关的重复或总量守恒。
5. 最终结果保留真实表、查询范围和计算口径，展示用户可读字段；内部 ID 用于连接或定位。

`+table-list` / `+base-block-list` 返回的 `records_count` 表示整表行数；manifest 的 `records_count` 表示本次查询实际导出的行数。

## 复用本轮 NDJSON

Agent 上下文曾下载过当前表的 NDJSON 时，按以下规则判断是否复用：

1. 短时间内继续分析或表中数据低频变化时，谓词下推口径一致且已有列覆盖计算需求即可优先复用。
2. 间隔较长或表中数据高频变化时，批量提取 manifests 的 `base_token/table_id/rev`，并发执行 `+table-list` 校验最新 `rev`；版本一致且谓词口径未变时复用，否则重新导出对应表。

> 例：本轮已按“日期在 2026 年”导出 `orders.ndjson`，用户继续要求按负责人聚合；谓词和所需列未变，直接复用。若间隔较长或该表频繁写入，manifest `rev=42` 与 `+table-list` 最新 `rev` 相同则复用，最新 `rev=43` 则重新导出。

## LLM 语义分析

先用任务中明确且不改变分析口径的确定性条件缩小数据范围；只有剩余判断必须依赖语义理解时，才将必要原文加载到模型上下文。

开放文本打标、情绪或意图识别、主题归纳、语义分类、相似性判断和实体消歧等任务必须理解原文，最终判断由当前 LLM 在本地上下文中逐条完成。代码只用于确定性范围筛选、分批、结果持久化和最终汇总；除非用户明确要求规则法，不用关键词命中、词频、正则或规则打分替代语义判断。

1. 先把日期、状态、来源等不改变任务语义的确定性范围条件下推到 Base，只导出 `record_id`、判断所需原文和最终解释所需的最小字段集。
2. 在读取正文前，先看 manifest 的 `record_file_size_bytes`；结合 `records_count` 以及所选字符串列的 `null_count`、`max_length` 判断正文相对当前上下文的规模，拿不准时先读取前 3 行再决定读取范围。
3. 文件较小且上下文充足时，将必要记录读入上下文并直接完成语义分析；文件较大但任务仍必须理解全部原文时，先向用户说明原因和预计耗时，在确认后按文本体量分批处理。各批沿用同一判断口径，将 `record_id`、结构化判断和必要依据持续写入本地 artifact，最后统一汇总。

## Manifest

`--output <path>.ndjson` 生成 `<path>.ndjson` 与 `<path>.manifest.json`；记录写入 NDJSON，stdout 返回 manifest，`--minimal-stdout` 只保留文件位置、文件字节数、`records_count` 和 `has_more`。

分析 artifact 使用相对路径输出到当前工作目录，例如 `--output ./records.ndjson`。

```json
{
  "record_file": "/path/records.ndjson",
  "record_file_size_bytes": 18432,
  "manifest_file": "/path/records.manifest.json",
  "records_count": 137,
  "has_more": false,
  "columns": {
    "record_id": {"physical_type": "string", "stats": {"max_length": 15}},
    "状态": {
      "field_id": "fld_status",
      "field_type": "select",
      "physical_type": "array<string>",
      "stats": {"empty_count": 3, "max_length": 2, "avg_length": 1.1},
      "example": ["进行中"]
    }
  }
}
```

- manifest `columns` 是 NDJSON 物理 schema 的权威来源，包含 `field_id`、`field_type`、`physical_type`、`stats` 以及可选的真实 example 或 hint；它不替代完整 Base field schema，选项配置、数字格式、Link 目标表或 formula/lookup 定义影响任务时读取 `+field-list`。全空列按 hint 跳过，任务必须使用时显式 cast。
- `stats` 只统计本次导出的 records；`null_count` 只计 JSON `null`，字符串长度按 Unicode 字符计数，数字 `avg` 排除 null，多值 `avg_length` 按全部 records（含 `[]`）计算。

| 列类别 | `stats` |
| --- | --- |
| 普通字符串 | `null_count, max_length` |
| 数字 | `null_count, min, max, avg` |
| 日期 | `null_count, min, max` |
| checkbox | `true_count` |
| Location | `null_count` |
| 多值列 | `empty_count, max_length, avg_length` |
| 系统 `record_id` | `max_length` |

- stdout 的 `records_count` 和 `has_more` 描述本次导出；确认后无需在分析代码中重读 manifest 或重新统计 NDJSON 行数。
- `record_file_size_bytes` 是 NDJSON artifact 的实际字节数，用于选择一次读取、预览或分批方式；确定性计算由 jq/Python 直接读取文件。
- manifest 的 `rev` 是导出首个响应页返回的 table revision；与 `+table-list` 返回的最新 `rev` 比较，可判断本轮 NDJSON 是否仍对应当前表版本。
- `query_context` 保存导出查询范围；复用本轮 NDJSON 时结合原查询上下文确认谓词下推口径保持一致。
- 仅在需要 `columns`、example、hint 或执行 artifact 复用判断时读取 `manifest_file`；满足复用条件后直接继续分析现有 NDJSON。
- `ignored_fields` 和 `record_not_found` 仅在 stdout 返回时关注。

## 数据库专家快速心智模型

- Base table 是面向协作的反范式宽表；本地分析将每个导出表作为关系输入，不假设数据库级约束。
- 每行是一条 record；系统 `record_id` 是表内真正的主键，由 Base 系统生成并维护，契约保证 `NOT NULL` 和 `UNIQUE`，分析代码无需再次检查空值或唯一性，也不可把它作为普通字段更新。Base 的“主字段”只是主要展示字段，不是主键。
- NDJSON 业务列一律使用字段 `name` 作为 key，不使用 `field_id`；字段重命名会改变 key，对应的 `field_id` 仅记录在 manifest 列元数据中。
- 除 `record_id` 外，不假设任何列满足 `NOT NULL`、`UNIQUE` 或业务键约束；仅当某列实际作为业务键参与关联或去重时处理空值和重复值。
- checkbox 在 NDJSON 中始终为 `true` 或 `false`，上游空值会在导出时规范化为 `false`；其他标量列可空并使用 `null`。多值列始终非空，没有元素时用 `[]`；这些是序列化契约，不是业务约束。
- NDJSON 的读取结构以 manifest `physical_type` 和下表为准，不等同于写记录时的 CellValue；`lark-base-cell-value.md` 在读写形态不一致的类型下提供对照说明。formula 和 lookup 在当前 NDJSON 中统一为字符串，不保留计算结果的原始类型。
- 将 `physical_type` 和上述 CellValue 结构视为输入契约；一次性分析代码直接读取，不再逐格验证 `record_id`、数组或 struct 的运行时形状。
- 未显式指定 sort 时不保证行顺序。

### Physical type 快速参考

| `field_type` | `physical_type` | 示例与语义 |
| --- | --- | --- |
| 系统 `record_id` | `string` | `"rec_xxx"`；系统主键 |
| `text`、`formula`、`lookup`、`auto_number`、`not_support` | `string|null` | `"进行中"`；formula、lookup 不保留结果的原始类型 |
| `datetime`、`created_at`、`updated_at` | `string|null` | `"2026-08-05T10:30:00.000+08:00"`；RFC3339，固定三位毫秒 |
| `number` | `number|null` | `12.5`；JSON 整数和小数均为 number |
| `checkbox` | `boolean` | `true`；上游空值已规范化为 `false` |
| `select` | `array<string>` | `["进行中", "高优"]`；单选、多选读取均为名称数组 |
| `location` | `struct<lng number, lat number, full_address string>|null` | `{"lng":116.39,"lat":39.90,"full_address":"北京市"}`；非空 Location 的三个成员均非空 |
| `user`、`group_chat`、`created_by`、`updated_by` | `array<struct<id string, name string>>` | `[{"id":"ou_xxx","name":"张三"}]` |
| `link` | `array<struct<id string>>` | `[{"id":"rec_xxx"}]`；schema 的 `table_id` 指定目标表，`id` 是目标 `record_id` |
| `attachment` | `array<struct<file_token string, size number, name string>>` | `[{"file_token":"box_xxx","size":1024,"name":"report.pdf"}]` |

### 日期字段读取

日期字段以带 offset 的 RFC3339 字符串序列化，并有两种分析语义：

- **instant semantics**：计算真实时长、先后顺序或跨时区比较时，解析完整 RFC3339 值，以其表示的绝对时刻计算。
- **local-calendar semantics**：按来源 Base 的日、周、月等本地日历分组时，使用序列化值中的本地日期，不先转 UTC，也不按 manifest `timezone` 重复换算。

例如，`2026-03-20T23:30:00.000-05:00` 与 `2026-03-21T12:30:00.000+08:00` 表示同一时刻；前者若是来源 Base 的值，本地日报归入 3 月 20 日，而时长或排序计算应把它解析为绝对时刻。只构造任务实际需要的日期表示，并在分析引擎中使用具备 datetime 功能的列。

## 读取与关系建模

仅在 SOP 已选择 Python 路径后，按实际实现方式只读一份示例：

- [Python 标准库示例](lark-base-data-analysis-python-stdlib.md)
- [pandas 示例](lark-base-data-analysis-pandas.md)

两份示例使用相同的五类场景：加载与日期解析、集合谓词、单数组展开、Link JOIN、多数组共现。场景语义和粒度规则以本 SOP 为准，示例只提供对应实现的最短代码。

标准库足以清晰表达任务时直接使用；DataFrame 能明显简化计算时再选 pandas。已选择 pandas 但环境未安装时，网络可用且存在 `uv` 或 `pip` 才按需安装，优先使用 `uv run --no-project --with pandas python analyze.py`。

将 Base 反范式宽表映射为关系模型时，可将标量列视为 record attributes，将多值列视为以 `record_id` 为关联键的 nested relation，将 Link 视为跨表 adjacency list。多值列通过 lateral `explode` / `UNNEST` 切换粒度；Link 规范化为 bridge relation 后再 `merge` / `join` / `JOIN`；同类来源表先投影到 conformed fact schema，再用 `concat` / `UNION ALL` 纵向合并。

## 常见分析模式

### 单表简单筛选与统计：jq

NDJSON 每行是一条 record。单表短筛选、计数和简单聚合可直接用 jq；下面筛选“状态”包含“进行中”的记录，并统计记录数和金额合计：

默认导出后使用本地 `jq -s`，同一 artifact 可反复查询而无需重新下载。表达式很短且只执行一次，或本地 jq 不可用时，可改用 `--jq-records '<expr>'` 等价 `jq -s '<expr>' records.ndjson`。使用 CLI 内置 jq 处理 NDJSON 记录时必须使用 `--jq-records`；通用 `--jq` 不支持 ndjson。

```bash
lark-cli base +record-list \
  --base-token <base_token> \
  --table-id <table_id> \
  --field-id 状态 \
  --field-id 金额 \
  --output records.ndjson \
  --minimal-stdout &&
jq -s '
  map(select((.["状态"] | index("进行中")) != null)) as $records
  | ($records | map(.["金额"] | select(. != null))) as $amounts
  | {
      records_count: ($records | length),
      amount_sum: (
        if ($amounts | length) > 0 then ($amounts | add) else null end
      )
    }
' records.ndjson
```

### 多值列：nested relation 与目标粒度

Base 的反范式宽表会把零到多个 Select、人员、群组、Link 或附件元素嵌入一条 source record。多值单元格默认按无重复、无序集合建模：元素顺序不承担稳定业务语义，同一 source record 内可将元素视为唯一，因此其元素数等于去重元素数；跨 source record 出现的同一元素仍是不同事实或关系边。分析时将数组视为以 `record_id` 为 correlation key 的 nested relation，并先确定 target grain：

- **record grain**：包含、交集、子集和元素数量等问题直接使用集合谓词，不做 expansion。
- **record-element grain**：通过 lateral `explode` / `UNNEST` 规范化为 `(source_record_id, element)` bridge relation。inner expansion 会丢弃空数组来源，outer expansion 会保留来源 record；回到 record 口径时按 `source_record_id` 聚合或去重。
- **entity grain**：两侧分别规范化为 bridge relation，再按稳定 element key JOIN。人员和群组以 `id` 连接、以 `name` 展示；Select 以名称作为元素键，仅当字段共享同一业务值域时才可连接。

使用列 `stats` 中的 `empty_count`、`avg_length` 和 `max_length` 做 expansion cardinality 与数据倾斜预估：单数组 inner expansion 的估算行数为 `records_count × avg_length`，outer expansion 还需加上 `empty_count`；结合 `max_length` 识别极端 fan-out 或 hot record。任务确实需要元素粒度且估算规模可控时，可以直接展开。

#### 多数组、fan-out 与 row-local Cartesian product

同一 source record 中的独立数组默认建立为彼此独立的 lateral pipeline，分别展开并聚合回 target grain 后再连接，避免 many-to-many fan-out 和重复计量。只有问题明确要求分析元素组合或共现时，才同时展开形成 row-local Cartesian product。

两个数组同时展开的准确 cardinality 为 `Σᵢ(|Aᵢ| × |Bᵢ|)`；可用 `records_count × avg_length_a × avg_length_b` 估算执行规模，并结合两列的 `max_length` 判断极端 fan-out。平均长度乘积不反映列间相关性，只用于成本估算。Base schema 不提供不同多值列之间的 positional contract；仅当额外业务契约明确声明位置对应语义时，才按 ordinality ZIP。

### Link：跨表 adjacency list

- Link 字段的完整 schema 以 `+field-list` 为准，其中 `table_id` 声明唯一目标 table；NDJSON 的 `[{"id":"rec_xxx"}]` 表示指向该表目标 `record_id` 的零到多条有向边。以 `table_id` 确定目标表，缺少可信 schema 时先补充 `+field-list`。
- 将 Link 规范化为 `(source_record_id, target_record_id)` edge/bridge relation，再按 `target_record_id = 目标表.record_id` 执行外键式 JOIN。需要反向遍历时复用同一 edge relation 反向分组或连接；NDJSON 不隐含自动反向关系。
- 多跳 Link 通过逐跳组合 edge relation 完成 traversal，并始终在各自 record-id domain 内连接。最终展示目标表的用户可读 attributes；已有 Link 时使用 Link edge relation，其他关联使用经过验证的 business key。

### 跨表同类实体与指标

- 多表 users 等重复实体的事实分析，先把各表投影为 `(source_table, source_record_id, entity_id, metric...)` 的 conformed long fact schema，再 `UNION ALL` 并聚合到 entity grain。需要横向比较时，各表先聚合到相同 entity grain 再 JOIN，避免原始事实之间产生 many-to-many fan-out。
- 没有 Link 时只能使用经过验证的 business key 关联。名称相似匹配属于 entity resolution，不属于普通 JOIN；应作为独立阶段输出匹配依据、置信度和未决项。
