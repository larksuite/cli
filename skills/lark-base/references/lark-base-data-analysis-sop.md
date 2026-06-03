# Base data analysis SOP

Base 数据查询与分析任务的执行契约。覆盖记录读取、筛选、排序、Top/Bottom N、聚合统计、分组聚合、多表关联、临时分析和查询后写入前的目标定位。

本文只管查询选路和正确性边界；具体操作前先读真实结构和现状，复杂 JSON 再跳到 reference：

- `+data-query`: entry guide [lark-base-data-query-guide.md](lark-base-data-query-guide.md), full DSL SSOT [lark-base-data-query.md](lark-base-data-query.md)
- 视图筛选: [lark-base-view-set-filter.md](lark-base-view-set-filter.md)
- 记录读取: `+record-list` / `+record-search` / `+record-get`，先确认字段 ID、字段名、分页和投影范围

## 0. Hard Rules

- 全局问题不能用默认 `+record-list --limit N` 片面地回答。
- `jq` / shell / 本地代码是在个人电脑或当前运行环境中处理已返回数据，只适合小范围结果；超过 200 行默认不推荐本地统计、排序或求极值，应改用 Base 云端查询服务的 filter/sort/aggregate。
- “最高、最低、最新、最早、Top、Bottom、总数、全部、异常、最大、最小、最多、最少、优先级最高”等全局语义，必须在 Base 云端查询服务中完成筛选、排序或聚合。
- 一次性原始记录查询优先用 `+record-list` / `+record-search` 的 filter/sort；聚合分析优先用 `+data-query`。
- `+record-search` 用于关键词检索字段的展示文本；金额、状态、日期、空值、关联等结构化条件继续用 `--filter-json` 表达。
- 不要依赖已有视图，除非用户明确指定该视图，或你已读取并验证其 filter/sort/projection 符合当前问题。
- 交付输出必须使用用户可读的真实字段值；内部 ID、`record_id`、关联记录 ID、open_id、编码字段只可作为连接键或定位键，不能替代最终输出，除非用户明确要求输出这些键值。
- 每次读取必须做最小投影，并包含后续解释、回查或写入需要的业务 key。

## 1. Intent -> Tool Path

| 用户意图 | 首选路径 | 关键规则 |
| --- | --- | --- |
| 看几条、预览、示例 | `+record-list --limit N --field-id ...` | 保持局部语义；不要推广为全局结论 |
| 已知 `record_id` | `+record-get` | 直接读取；不要 search/list 反查 |
| 明确关键词 | `+record-search --keyword ... --search-field ... --field-id ...` | 必须显式指定 `--search-field`；可叠加 `--filter-json` |
| 按条件找原始记录 | `+record-list --filter-json ...` | `filter-json` 与视图筛选结构一致，支持文本、数字、日期、选项、人员、群组、关联等值 |
| 排序 / TopN 原始记录 | `+record-list --filter-json ... --sort-json ... --limit N` | 最高/最新用 `desc:true`，最低/最早用 `desc:false`；数组顺序表达优先级；最多 10 个排序条件 |
| 聚合 / 分组 / 分组排序 | `+data-query` | 使用 filters/dimensions/measures/sort/limit |
| 聚合后输出逐条记录 | `+data-query` 得到业务 key 或候选字段组合 -> `+record-list --filter-json` / `+record-get` 回查 | `+data-query` 维度行按字段组合去重且不返回 `record_id` |
| 多表 / 多跳关联 | 以候选数最小的事实表为驱动表，沿业务 key 或 link `record_id` 逐跳回查 | 读出 link 单元格里的关联 `record_id` 后，到被关联表批量 `+record-get` 展示字段 |
| 查询后写入 / 视图化 | 先用本 SOP 得到可复核的目标记录 id 集合 | 再进入记录写入或视图配置；高价值可复用查询可沉淀为持久视图 |

## 2. Execution Patterns

### 2.1 结构化原始记录与 TopN

使用 `+record-list` 的 filter/sort 路径：

1. `+field-list` 确认筛选字段、排序字段、展示字段、业务 key。
2. 筛选只用 `--filter-json` 或 `--filter-json @file`。
3. 排序用 `--sort-json`。
4. `--field-id` 做最小投影，`--limit` 控制返回数量。

Example: string/number 条件 + TopN：

```bash
lark-cli base +record-list \
  --base-token <base_token> \
  --table-id <table_id> \
  --filter-json '{"logic":"and","conditions":[["Title","==","Launch plan"],["Score",">=",80]]}' \
  --sort-json '[{"field":"Updated","desc":true}]' \
  --field-id Name \
  --field-id Title \
  --field-id Score \
  --limit 20
```

Example: 复杂筛选从文件读取：

```bash
lark-cli base +record-list \
  --base-token <base_token> \
  --table-id <table_id> \
  --filter-json @filter.json \
  --sort-json '[{"field":"Priority","desc":true}]' \
  --field-id Name \
  --field-id Tags \
  --limit 50
```

`filter-json` 与视图筛选结构一致。下面只列常用 fewshot；字段类型、operator、value 形状拿不准，或需要人员、群组、关联、空值、地理位置、formula / lookup 等完整筛选时，先读 [lark-base-view-set-filter.md](lark-base-view-set-filter.md)，再把同样的 filter JSON 传给 `--filter-json`。

文本 `==`：字段值等于目标文本。
```json
{"logic":"and","conditions":[["Title","==","Launch plan"]]}
```

文本包含 / like：文本字段包含目标片段；operator 写 `intersects`。
```json
{"logic":"and","conditions":[["Title","intersects","urgent"]]}
```

数字 `==`：字段值等于目标数字。
```json
{"logic":"and","conditions":[["Score","==",95]]}
```

日期 `==`：字段值等于目标日期；datetime / created_at / updated_at 用 `ExactDate(...)`。
```json
{"logic":"and","conditions":[["Due Date","==","ExactDate(2026-06-02)"]]}
```

选项 `==`：字段值匹配单个选项；选项值使用选项名数组，单个选项也写数组。
```json
{"logic":"and","conditions":[["Priority","==",["P0"]]]}
```

选项 `intersects`：字段值与给定选项集合有交集，常用于多选或“命中任一选项”。
```json
{"logic":"and","conditions":[["Tags","intersects",["P0","Blocked"]]]}
```

`--sort-json` 传排序数组，数组顺序就是优先级，`desc:true` 为降序，`desc:false` 为升序，最多 10 个排序条件。

### 2.2 关键词检索后叠加结构化条件

使用 `+record-search` 做关键词命中，结构化条件仍用 `--filter-json` 下推：

```bash
lark-cli base +record-search \
  --base-token <base_token> \
  --table-id <table_id> \
  --keyword Alice \
  --search-field Name \
  --filter-json '{"logic":"and","conditions":[["Status","!=","Done"]]}' \
  --sort-json '[{"field":"Updated","desc":true}]' \
  --field-id Name \
  --field-id Status \
  --limit 20
```

不要把 `+record-search` 当成金额、状态、日期、空值、关联字段的结构化筛选入口；这些条件继续写成 `--filter-json`。

### 2.3 聚合分析与 TopN

使用 `+data-query`：

- 让 Base 云端查询服务完成 filters、dimensions、measures、sort、pagination.limit。
- `pagination.limit` 是 Base 云端查询服务中的结果限制，不是本地分页扫描。
- 常用聚合 fewshot 先读 [lark-base-data-query-guide.md](lark-base-data-query-guide.md)；字段类型、日期 value、DSL shape 以 [lark-base-data-query.md](lark-base-data-query.md) 为准。
- `+data-query` 可返回聚合结果或维度字段行；维度字段行按字段组合去重且不返回 `record_id`，不能当逐条原始记录结果使用。
- 需要输出逐条记录、记录定位或完整行级字段时，先用 `+data-query` 得到业务 key、分组值或候选字段组合，再用 `+record-list --filter-json` / `+record-get` 回查。

Example: 分组计数：

```bash
lark-cli base +data-query \
  --base-token <base_token> \
  --dsl '{"datasource":{"type":"table","table":{"tableId":"<table_id>"}},"dimensions":[{"field_name":"Status","alias":"status"}],"measures":[{"field_name":"Status","aggregation":"count","alias":"count"}],"shaper":{"format":"flat"}}'
```

Example: 过滤后汇总并取 TopN：

```bash
lark-cli base +data-query \
  --base-token <base_token> \
  --dsl '{"datasource":{"type":"table","table":{"tableId":"<table_id>"}},"dimensions":[{"field_name":"Owner","alias":"owner"}],"measures":[{"field_name":"Amount","aggregation":"sum","alias":"total_amount"}],"filters":{"type":1,"conjunction":"and","conditions":[{"field_name":"Status","operator":"is","value":["Done"]}]},"sort":[{"field_name":"total_amount","order":"desc"}],"pagination":{"limit":10},"shaper":{"format":"flat"}}'
```

### 2.4 视图化与复用

一次性查询先用 `+record-list` / `+record-search` 的 filter/sort 验证。需要用户长期打开、共享或复用时，再把同一套 filter/sort 沉淀为视图。

Example: 将已验证的筛选排序写入视图：

```bash
lark-cli base +view-set-filter \
  --base-token <base_token> \
  --table-id <table_id> \
  --view-id <view_id> \
  --json @filter.json

lark-cli base +view-set-sort \
  --base-token <base_token> \
  --table-id <table_id> \
  --view-id <view_id> \
  --json '{"sort_config":[{"field":"Priority","desc":true}]}'
```

手动配置和视图配置的优先级：

1. `--filter-json` 覆盖 `--view-id` 保存的 view filter JSON。
2. `--sort-json` 覆盖 `--view-id` 保存的 view sort config。
3. 没有手动 filter/sort 时，`--view-id` 使用视图自身保存的 filter/sort。

### 2.5 关系查询与回查

- link 单元格通常是关联表 `record_id` 数组，不是用户可读内容，只是连接键。
- 先用 `+field-list` 确认 link 字段的 `link_table`、业务唯一键和展示字段。
- 从驱动表拿到候选记录后，用关联 `record_id` 到关联表 `+record-get` 批量读取记录内容。
- 多跳关系逐跳建立 `record_id/key -> 用户可读字段` 映射；最终用户可读的信息。

禁止：

- 把 link `record_id` 当最终输出。
- 用 `+record-search` 搜 link `record_id`。
- 基于 ID、自增编号、link 值做语义猜测；禁止依赖字段先验、样本记忆补全交付输出。

## 3. Range & Pagination Contract

- `+record-list` 默认页、固定 `--limit`、本地 `jq`、shell 管道、手工浏览输出，都只覆盖已读取范围；超过 200 行不要把本地处理当作推荐路径。
- `has_more=true`、存在下一页 offset/page token、或返回行数等于 page size，都表示可能还有未读取数据。
- 对全局问题，只有 Base 云端查询服务已经通过 filter/sort/aggregate 收敛目标范围，或 `+data-query` 已在云端完成聚合、排序和限制时，才可以用有限返回形成结论。
- 必须全量导出时，按 `+record-list` 分页语义串行翻页；不要并发调用 `+record-list`。

## 4. Final Answer Check

形成交付输出前必须能确认：

- 问题范围是局部样例、单点定位、全局原始记录、聚合分析、多表关联，还是查询后写入。
- 筛选、排序、聚合是否发生在 Base 云端查询服务中，而不是本地 `jq` / shell 中。
- 如果使用 `jq` / shell，本地输入是否是 200 行以内的小范围结果；超过 200 行是否已改用 Base 云端查询服务查询。
- 如果使用 `+record-list` / `+record-search`，是否处理了 `has_more`，且投影包含业务 key 和解释字段。
- 如果涉及关系查询，是否按 `record_id` 或业务 key 精确回查，交付输出是否来自关联表真实字段。
- 交付输出能追溯到表、字段、筛选条件、排序/聚合条件和连接键。

任一项无法确认时，继续查询或明确说明只能得到局部结论。
