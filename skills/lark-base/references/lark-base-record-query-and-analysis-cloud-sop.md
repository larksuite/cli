# Base Record 查询与分析 Cloud SOP

统一数据分析 SOP 将任务路由到 Cloud 时使用本 SOP。覆盖记录读取、筛选、排序、Top/Bottom N、聚合统计、分组聚合、多表关联和查询后写入前的目标定位。

本文只管查询选路和正确性边界；先按下方 Intent -> Tool Path 选择原始记录查询或聚合查询，再读真实结构和现状：

- 视图筛选: [lark-base-view-set-filter.md](lark-base-view-set-filter.md)
- 记录读取: `+record-list` / `+record-search` / `+record-get`，先确认字段 ID、字段名、分页和投影范围

## 0. 执行约定

- “最高、最低、最新、最早、Top、Bottom、总数、全部、异常、最大、最小、最多、最少、优先级最高”等全局语义，在本路径中由 Base 云端查询服务完成筛选、排序或聚合。
- 一次性原始记录查询优先用 `+record-list` / `+record-search` 的 filter/sort；聚合分析优先用 `+data-query`。
- `+record-search` 用于关键词检索字段的展示文本；金额、状态、日期、空值、关联等结构化条件继续用 `--filter-json` 表达。
- 不要依赖已有视图，除非用户明确指定该视图，或你已读取并验证其 filter/sort/projection 符合当前问题。
- 内部 ID、`record_id`、关联记录 ID、open_id 和编码字段用于连接或定位；交付输出使用用户可读的真实字段值，用户明确要求 ID 时一并展示。
- 每次读取必须做最小投影，并包含后续解释、回查或写入需要的业务 key。

## 1. Intent -> Tool Path

| 用户意图 | 首选路径 | 关键规则 |
| --- | --- | --- |
| 看几条、预览、示例 | `+record-list --limit N --field-id ...` | 保持局部语义 |
| 已知 `record_id` | `+record-get` | 直接读取 |
| 明确关键词 | `+record-search --keyword ... --search-field ... --field-id ...` | 必须显式指定 `--search-field`；可叠加 `--filter-json` |
| 按条件找原始记录 | `+record-list --filter-json ...` | `filter-json` 与视图筛选结构一致，支持文本、数字、日期、选项、人员、群组、关联等值 |
| 排序 / TopN 原始记录 | `+record-list --filter-json ... --sort-json ... --limit N` | 最高/最新用 `desc:true`，最低/最早用 `desc:false`；数组顺序表达优先级；最多 10 个排序条件 |
| 聚合 / 分组 / 分组排序 | `+data-query` | 读取 [data-query DSL reference](lark-base-data-query.md)，使用 filters/dimensions/measures/sort/limit |
| 聚合后输出逐条记录 | `+data-query` 得到业务 key 或候选字段组合 -> `+record-list --filter-json` / `+record-get` 回查 | `+data-query` 维度行按字段组合去重且不返回 `record_id` |
| 多表 / 多跳关联 | 以候选数最小的事实表为驱动表，沿业务 key 或 Link 逐跳回查 | 读出 Link 单元格的 `id`（目标表 `record_id`）后，到被关联表批量 `+record-get` 展示字段 |
| 查询后写入 / 视图化 | 先用本 SOP 得到可复核的目标记录 id 集合 | 再进入记录写入或视图配置；高价值可复用查询可沉淀为持久视图 |

## 2. Execution Patterns

### 2.1 结构化原始记录与 TopN

使用 `+record-list` 的 filter/sort 路径：

1. `+field-list` 确认筛选字段、排序字段、展示字段、业务 key。
2. 筛选使用 `--filter-json '<filter-json>'`。
3. 排序用 `--sort-json`。
4. `--field-id` 做最小投影，`--limit` 控制返回数量。

Example: 结构化筛选 + TopN；示例展示文本包含、数字比较和 Select 集合相交三个常用谓词：

```bash
lark-cli base +record-list \
  --base-token <base_token> \
  --table-id <table_id> \
  --filter-json '{"logic":"and","conditions":[["Title","intersects","Launch plan"],["Score",">=",80],["Status","intersects",["Doing"]]]}' \
  --sort-json '[{"field":"Updated","desc":true}]' \
  --field-id Name \
  --field-id Title \
  --field-id Score \
  --limit 20
```

常用 `filter-json` condition fewshot 统一见 [Base Record 查询与分析 SOP](lark-base-record-query-and-analysis-sop.md)；完整协议见 [Base Filter 条件结构](lark-base-filter-condition.md)。

`--sort-json` 传排序数组，数组顺序就是优先级，`desc:true` 为降序，`desc:false` 为升序，最多 10 个排序条件。

### 2.2 关键词检索后叠加结构化条件

使用 `+record-search` 做关键词命中，结构化条件仍用 `--filter-json` 下推：

```bash
lark-cli base +record-search \
  --base-token <base_token> \
  --table-id <table_id> \
  --keyword Alice \
  --search-field Name \
  --filter-json '{"logic":"and","conditions":[["Status","intersects",["Doing"]]]}' \
  --sort-json '[{"field":"Updated","desc":true}]' \
  --field-id Name \
  --field-id Status \
  --limit 20
```

金额、状态、日期、空值和关联字段等结构化条件使用 `--filter-json`；`+record-search` 处理展示文本关键词。

### 2.3 聚合分析与 TopN

使用 `+data-query`：

- 让 Base 云端查询服务完成 filters、dimensions、measures、sort、pagination.limit。
- `pagination.limit` 是 Base 云端查询服务中的结果限制，不是本地分页扫描。
- 读取 [data-query DSL reference](lark-base-data-query.md) 中与当前查询有关的 fewshot、字段和协议。
- `+data-query` 可返回聚合结果或维度字段行；维度字段行按字段组合去重且不返回 `record_id`，不能当逐条原始记录结果使用。
- 需要输出逐条记录、记录定位或完整行级字段时，先用 `+data-query` 得到业务 key、分组值或候选字段组合，再用 `+record-list --filter-json` / `+record-get` 回查。

Example: 分组计数：

```bash
lark-cli base +data-query \
  --base-token <base_token> \
  --dsl '{"datasource":{"type":"table","table":{"tableId":"<table_id>"}},"dimensions":[{"field_name":"Status","alias":"status"}],"measures":[{"field_name":"Status","aggregation":"count","alias":"count"}],"shaper":{"format":"flat"}}'
```

Example: 汇总后取 TopN；需要过滤时按 `+data-query` 的 LiteQuery DSL reference 增加 `filters`：

```bash
lark-cli base +data-query \
  --base-token <base_token> \
  --dsl '{"datasource":{"type":"table","table":{"tableId":"<table_id>"}},"dimensions":[{"field_name":"Owner","alias":"owner"}],"measures":[{"field_name":"Amount","aggregation":"sum","alias":"total_amount"}],"sort":[{"field_name":"total_amount","order":"desc"}],"pagination":{"limit":10},"shaper":{"format":"flat"}}'
```

### 2.4 视图化与复用

一次性查询先用 `+record-list` / `+record-search` 的 filter/sort 验证。需要用户长期打开、共享或复用时，再把同一套 filter/sort 沉淀为视图。

Example: 将已验证的筛选排序写入视图：

```bash
lark-cli base +view-set-filter \
  --base-token <base_token> \
  --table-id <table_id> \
  --view-id <view_id> \
  --json '{"logic":"and","conditions":[["Priority","intersects",["P0"]]]}'

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

- Link 单元格中的元素形如 `{"id":"rec_xxx"}`；`id` 是目标表的 `record_id`，用于关系连接。
- 先用 `+field-list` 确认 link 字段的 `link_table`、业务唯一键和展示字段。
- 从驱动表拿到候选记录后，用 Link 元素的 `id` 到目标表 `+record-get` 批量读取记录内容。
- 多跳关系逐跳建立 `record_id/key -> 用户可读字段` 映射，交付目标表返回的真实业务字段。

## 3. Range & Pagination Contract

- `+record-list` 默认页、固定 `--limit` 和手工浏览输出都只覆盖已读取范围；模型上下文接收云端收敛后的最终小结果。
- `has_more=true` 说明可能还有未读取数据，需要更新 offset 后继续读取，多次读取仍未读取完成时，采用其他方法完成任务需求，避免无限循环。
- 对全局问题，只有 Base 云端查询服务已经通过 filter/sort/aggregate 收敛目标范围，或 `+data-query` 已在云端完成聚合、排序和限制时，才可以用有限返回形成结论。
- 需要完整原始记录但云端能力无法把结果安全收敛到可返回范围时，明确说明能力边界；不要用手工分页、拆分下载或采样伪装成全局分析。
