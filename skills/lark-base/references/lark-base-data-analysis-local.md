# Base 数据分析 SOP

所有表格数据分析先读本 SOP，按任务所需数据规模与计算复杂度，依次选择 lark-cli 内置 jq、Python 或 Base Cloud。

## 分流决策

1. 明确范围：任务范围只包含必要表；复用本次任务已有且仍可信的 `table_id`、`records_count`、字段类型和 Link 目标表，仅用 `+table-list`、`+base-block-list` 或 `+field-list` 补齐缺失信息。定义 `local_max_records = max(各必要表经过任务语义允许的单表谓词下推后实际需导出的完整记录数)`；`local_max_records <= 2000` 时可继续本地分析。
2. 先判断内置 jq 路径；同时满足以下条件时直接执行：
   1. 只有一张必要表。
   2. 任务是一个短 jq 表达式可清晰完成的筛选、计数、简单分组/聚合/排序、TopN 或 record-local 集合谓词；日期日历语义、窗口、多表关联、实体消歧和需要多阶段中间结果的分析不走 jq。
   3. 该表经过任务语义允许的谓词下推后，实际需导出的完整记录不超过 2000 条。整表 `records_count <= 2000` 时直接导出；否则组合用户任务已隐含且保持任务语义等价的单表谓词，文本关键词用 `+record-search`，结构化条件用 `+record-list --filter-json`。先投影一个简单标量字段，以 `--limit 2000 --output <probe>.ndjson --minimal-stdout` 探测，直到 `has_more=false` 或没有可继续下推的谓词。
   4. 确认范围完整后，复用相同查询参数，投影任务必要字段并用 `--jq-records '<expr>'` 返回最终小结果。jq 在 lark-cli 内执行，不要求外部 jq、共享文件系统或 Python；生成的 NDJSON 与 manifest artifact 保持原始完整内容。
3. jq 路径不适用时，才检查 Python：
   1. Python 可运行并能读取 lark-cli 生成的 artifact，且第 1 步定义的最大单表导出量不超过 2000 条时，导出必要字段的 NDJSON，使用 Python 标准库或 pandas 完成单表复杂计算、多表关联、多值展开、窗口和日历分析。
   2. 任一表较大或 `records_count` 缺失时，按第 2 步相同的方法下推单表谓词并窄投影探测；所有必要表均达到 `has_more=false` 后再正式导出。
   3. Python 标准库足以清晰表达任务时直接使用；DataFrame 能明显简化计算时再选 pandas。若已选 pandas 但环境未安装，网络可用且存在 `uv` 或 `pip` 时按需安装；优先使用 `uv run --no-project --with pandas python analyze.py`，只有 `pip` 时在隔离的虚拟环境中安装。
4. jq 无法完成，且 Python 不可用或谓词下推后的最大单表导出量仍超过 2000 条时，转读 [lark-base-data-analysis-cloud.md](lark-base-data-analysis-cloud.md)。
5. 模型上下文仅接收最终小结果，NDJSON 正文保留在 artifact 文件中。任务涉及业务键、展开、JOIN 或金额分摊时，明确目标粒度并检查与口径直接相关的空值、重复或总量守恒；选定一个分析引擎完成计算。内部 ID 用于连接或定位。

`+table-list` / `+base-block-list` 返回的 `records_count` 表示整表行数；manifest 的 `records_count` 表示本次查询实际导出的行数。

## Manifest

`--output <path>.ndjson` 生成 `<path>.ndjson` 与 `<path>.manifest.json`；记录写入 NDJSON，stdout 返回 manifest。`--jq` 只处理 manifest；确认本次范围完整后，可用 `--jq-records '<expr>'` 对完整导出的 records 数组执行一次查询并以结果替代 stdout，不改变两个 artifact 文件。

```json
{
  "record_file": "/path/records.ndjson",
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
- manifest 的 `rev` 是导出首个响应页返回的 table revision，主要用于接手之前保存的 artifact 时识别其版本。
- `query_context` 保存导出查询范围，主要用于接手之前保存的 NDJSON/manifest；本轮刚完成的查询直接使用当前上下文。
- 仅在需要 `columns`、example 或 hint 时读取 `manifest_file`；同表同投影的重复分析可用 `--minimal-stdout`。
- `ignored_fields` 和 `record_not_found` 仅在 stdout 返回时关注。

## 数据库专家快速心智模型

- Base table 是面向协作的反范式宽表；本地分析将每个导出表作为关系输入，不假设数据库级约束。
- 每行是一条 record；系统 `record_id` 是表内真正的主键，由 Base 系统生成并维护，契约保证 `NOT NULL` 和 `UNIQUE`，分析代码无需再次检查空值或唯一性，也不可把它作为普通字段更新。Base 的“主字段”只是主要展示字段，不是主键。
- NDJSON 业务列一律使用字段 `name` 作为 key，不使用 `field_id`；字段重命名会改变 key，对应的 `field_id` 仅记录在 manifest 列元数据中。
- 除 `record_id` 外，不假设任何列满足 `NOT NULL`、`UNIQUE` 或业务键约束；仅当某列实际作为业务键参与关联或去重时处理空值和重复值。
- checkbox 在 NDJSON 中始终为 `true` 或 `false`，上游空值会在导出时规范化为 `false`；其他标量列可空并使用 `null`。多值列始终非空，没有元素时用 `[]`；这些是序列化契约，不是业务约束。
- 业务字段沿用 `lark-base-cell-value.md` 定义的同一套 Base CellValue 结构；formula 和 lookup 在当前 NDJSON 中统一为字符串，不保留计算结果的原始类型。
- 将 `physical_type` 和上述 CellValue 结构视为输入契约；一次性分析代码直接读取，不再逐格验证 `record_id`、数组或 struct 的运行时形状。
- 未显式指定 sort 时不保证行顺序。

### Physical type 快速参考

| `field_type` | `physical_type` | 示例与语义 |
| --- | --- | --- |
| 系统 `record_id` | `string` | `"rec_xxx"`；系统主键 |
| `text`、`formula`、`lookup`、`auto_number`、`not_support` | `string|null` | `"进行中"`；formula、lookup 不保留结果的原始类型 |
| `datetime`、`created_at`、`updated_at` | `string|null` | `"2026-08-05T10:30:00+08:00"`；RFC3339 |
| `number` | `number|null` | `12.5`；JSON 整数和小数均为 number |
| `checkbox` | `boolean` | `true`；上游空值已规范化为 `false` |
| `select` | `array<string>` | `["进行中", "高优"]`；单选、多选读取均为名称数组，不含 option id |
| `location` | `struct<lng number, lat number, full_address string>|null` | `{"lng":116.39,"lat":39.90,"full_address":"北京市"}`；非空 Location 的三个成员均非空 |
| `user`、`group_chat`、`created_by`、`updated_by` | `array<struct<id string, name string>>` | `[{"id":"ou_xxx","name":"张三"}]` |
| `link` | `array<struct<id string>>` | `[{"id":"rec_xxx"}]`；schema 的 `table_id` 指定目标表，`id` 是目标 `record_id` |
| `attachment` | `array<struct<file_token string, size number, name string>>` | `[{"file_token":"box_xxx","size":1024,"name":"report.pdf"}]` |

### 日期字段读取

日期字段以带 offset 的 RFC3339 字符串序列化。参与日历、区间、排序或窗口计算时，将它转换为所用语言的原生或事实标准日期类型；在数据分析引擎中转换为具备 datetime 功能的列。按日/月分组直接使用值中表达的 Base 本地日期，不按 manifest `timezone` 重新换算。

## 读取与关系建模

仅在 SOP 已选择 Python 路径后，按实际实现方式只读一份示例：

- [Python 标准库示例](lark-base-data-analysis-python-stdlib.md)
- [pandas 示例](lark-base-data-analysis-pandas.md)

两份示例使用相同的五类场景：加载与日期解析、集合谓词、单数组展开、Link JOIN、多数组共现。场景语义和粒度规则以本 SOP 为准，示例只提供对应实现的最短代码。

将 Base 反范式宽表映射为关系模型时，可将标量列视为 record attributes，将多值列视为以 `record_id` 为关联键的 nested relation，将 Link 视为跨表 adjacency list。多值列通过 lateral `explode` / `UNNEST` 切换粒度；Link 规范化为 bridge relation 后再 `merge` / `join` / `JOIN`；同类来源表先投影到 conformed fact schema，再用 `concat` / `UNION ALL` 纵向合并。

## 常见分析模式

### 单表简单筛选与统计：jq

NDJSON 每行是一条 record。单表短筛选、计数和简单聚合可直接用 jq；下面筛选“状态”包含“进行中”的记录，并统计记录数和金额合计：

```bash
lark-cli base +record-list \
  --base-token <base_token> \
  --table-id <table_id> \
  --field-id 状态 \
  --field-id 金额 \
  --limit 2000 \
  --output records.ndjson \
  --jq-records '
  map(select((.["状态"] | index("进行中")) != null))
  | {
      records_count: length,
      amount_sum: (map(.["金额"] // 0) | add // 0)
    }
'
```

### 多值列：nested relation 与目标粒度

Base 的反范式宽表会把零到多个 Select、人员、群组、Link 或附件元素嵌入一条 source record。分析时将数组视为以 `record_id` 为 correlation key 的 nested relation，并先确定 target grain：

- **record grain**：包含、交集、子集和元素数量等问题直接使用集合谓词，不做 expansion。
- **record-element grain**：通过 lateral `explode` / `UNNEST` 规范化为 `(source_record_id, element)` bridge relation。inner expansion 会丢弃空数组来源，outer expansion 会保留来源 record；回到 record 口径时按 `source_record_id` 聚合或去重。
- **entity grain**：两侧分别规范化为 bridge relation，再按稳定 element key JOIN。人员和群组以 `id` 连接、以 `name` 展示；Select 只有名称而没有 option id，仅当字段共享同一业务值域时才可按名称连接。

使用列 `stats` 中的 `empty_count`、`avg_length` 和 `max_length` 做 expansion cardinality 与数据倾斜预估：单数组 inner expansion 的估算行数为 `records_count × avg_length`，outer expansion 还需加上 `empty_count`；结合 `max_length` 识别极端 fan-out 或 hot record。任务确实需要元素粒度且估算规模可控时，可以直接展开。

#### 多数组、fan-out 与 row-local Cartesian product

同一 source record 中的独立数组默认建立为彼此独立的 lateral pipeline，分别展开并聚合回 target grain 后再连接，避免 many-to-many fan-out 和重复计量。只有问题明确要求分析元素组合或共现时，才同时展开形成 row-local Cartesian product。

两个数组同时展开的准确 cardinality 为 `Σᵢ(|Aᵢ| × |Bᵢ|)`；可用 `records_count × avg_length_a × avg_length_b` 估算执行规模，并结合两列的 `max_length` 判断极端 fan-out。平均长度乘积不反映列间相关性，只用于成本估算。仅当 schema 明确声明两个数组具有位置对应语义时，才按 ordinality ZIP。

### Link：跨表 adjacency list

- Link 字段的完整 schema 以 `+field-list` 为准，其中 `table_id` 声明唯一目标 table；NDJSON 的 `[{"id":"rec_xxx"}]` 表示指向该表目标 `record_id` 的零到多条有向边。以 `table_id` 确定目标表，缺少可信 schema 时先补充 `+field-list`。
- 将 Link 规范化为 `(source_record_id, target_record_id)` edge/bridge relation，再按 `target_record_id = 目标表.record_id` 执行外键式 JOIN。需要反向遍历时复用同一 edge relation 反向分组或连接；NDJSON 不隐含自动反向关系。
- 多跳 Link 通过逐跳组合 edge relation 完成 traversal，并始终在各自 record-id domain 内连接。最终展示目标表的用户可读 attributes；已有 Link 时使用 Link edge relation，其他关联使用经过验证的 business key。

### 跨表同类实体与指标

- 多表 users 等重复实体的事实分析，先把各表投影为 `(source_table, source_record_id, entity_id, metric...)` 的 conformed long fact schema，再 `UNION ALL` 并聚合到 entity grain。需要横向比较时，各表先聚合到相同 entity grain 再 JOIN，避免原始事实之间产生 many-to-many fan-out。
- 没有 Link 时只能使用经过验证的 business key 关联。名称相似匹配属于 entity resolution，不属于普通 JOIN；应作为独立阶段输出匹配依据、置信度和未决项。
