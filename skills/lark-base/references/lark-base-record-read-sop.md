# base record read SOP

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、权限处理和全局参数。

记录读取由 6 个功能组合完成：选路、字段投影、视图预处理、分页与范围、返回结构解释、link 关联读取。

## 1. 读取选路

| 场景 | 使用方式 | 规则 |
|------|------|------|
| 已知 `record_id` | `+record-get` | 只读单条记录，不要用 search/list 反查。 |
| 明确关键词检索 | `+record-search` | 只用于文本关键词检索；金额、状态、日期等结构化条件不要用 search。 |
| 普通明细读取 / 导出 / 查看前 N 条 | `+record-list` | 优先加 `--view-id` 时只读该视图可见记录与可见字段；或者加 `--field-id` 手动裁剪字段；不传 `--view-id` 时会读取全表。 |
| 明确筛选 / 排序 / Top N / Bottom N 且需要原始记录或 `record_id` | 创建带 filter + sort 的临时视图 + `+record-list --view-id` | 让视图完成 filter/sort projection，LLM 不擅长手工筛选排序，建议用视图完成。 |
| 统计 / 聚合结果且不需要 `record_id` | 转到 [`lark-base-data-query.md`](lark-base-data-query.md) | `data-query` 是特殊分析 DSL，不是记录读取工具。 |

## 2. 字段投影

- `FieldListFirst`: 不清楚字段结构时先 `+field-list`，确认筛选字段、排序字段、展示字段、关联字段、业务唯一键字段。
- `UseRealField`: 字段名和字段 ID 必须来自 `+field-list` 返回，不要凭自然语言猜字段名。
- `MinimalProjection`: 每次读取只返回本次任务需要的字段；`+record-list` 用重复 `--field-id`，视图读取用 `+view-set-visible-fields`。
- `FieldScopePriority`: 返回字段优先级为显式投影字段（`+record-list --field-id` / `record-search select_fields`） > 视图可见字段 > 全表字段；需要稳定列范围时必须显式投影。
- `LongFieldAvoidance`: 默认不要读取 `trace`、`raw`、长文本、附件等高噪声字段，除非任务明确需要。
- `BusinessKey`: 后续要定位、更新或解释记录时，投影中必须包含可识别业务字段，例如订单号、日报ID、姓名、编号。

## 3. 视图预处理

适用于结构化筛选、排序、最高/最低、倒数、Top/Bottom N、按条件找记录等场景。

1. `+field-list` 获取字段 ID、字段名和字段类型。
2. `+view-create` 创建临时 `grid` 视图，名称带任务语义，例如 `tmp_query_销售额升序`。
3. `+view-set-filter` 设置筛选条件；空值是否参与必须按用户语义判断。
4. `+view-set-sort` 设置排序条件；最高/最新用降序，最低/最早/倒数用升序。
5. `+view-set-visible-fields` 设置投影字段，只保留业务键、排序字段、筛选解释字段、需要展示或二跳的字段。
6. `+record-list --view-id <view_id> --limit <N>` 读取结果；不要再从未排序全表输出中手动挑选。

## 4. 分页与范围

- `ViewScope`: URL 带 `view_id` 时先判断用户是否要求“该视图下”；全表问题不要误用 URL 视图范围，应该根据需求创建合适的临时视图完成查询任务。
- `ViewIdScope`: `+record-list --view-id` 是作用域参数；仅用于用户指定的视图，或本次任务主动创建的临时筛选 / 排序 / 投影视图。
- `NeedAllPages`: 用户要求全部、导出、统计、最高/最低且未用视图/limit 限定时，必须检查 `has_more` 并串行翻页。
- `LimitWhenScoped`: 用户只要示例、前 N 条、Top/Bottom N，使用 `--limit` 控制结果规模。
- `NoConcurrentList`: `+record-list` 禁止并发调用；分页和多表读取必须串行。
- `DataQueryScope`: `data-query` 的筛选 DSL 与视图筛选不是同一套语法；不要混用。

## 5. 返回结构解释

- `ColumnMapping`: `fields` / `field_id_list` 定义 `data` 每列含义；解释记录前先建立列到字段名的映射。
- `RowMapping`: `record_id_list[i]` 与 `data[i]` 是同一行；需要后续定位、更新或关联时，按下标整理成 `record_id + 字段名:值` 的小表。
- `BusinessMatch`: 后续引用目标记录时按业务字段匹配，不靠肉眼数行号。
- `FieldType`: 按字段类型解释值；数字、货币、日期、人员、formula、lookup、attachment、link 不要当普通文本处理。
- `EmptyValue`: 空值参与筛选或排序前必须明确语义；不要默认把空值当 `0`、空字符串或有效状态。
- `AnswerCheck`: 最终回答前复核答案记录来自读取结果、筛选排序已应用、字段含义和 record_id 映射无误。

## 6. link 关联字段读取

link 字段是关联单元格；读取结果通常是关联表的 `record_id` 数组，不是用户可读名称。

| 步骤 | 做法 |
|------|------|
| 识别 link 字段 | 用 `+field-list` 查看字段类型为 `link`，并读取 `link_table` 确认关联目标表。 |
| 读取当前表 | 在当前表 `+record-list` / `+record-get` 中保留 link 字段和业务键字段。 |
| 解析单元格值 | link 单元格通常形如 `[{"id":"rec..."}]`；提取其中每个 `id` 作为关联表 `record_id`。 |
| 读取关联表 | 到 `link_table` 使用 `+record-get --record-id <rec...>` 或裁剪后的 `+record-list` 读取显示字段。 |
| 建立映射 | 形成 `关联record_id -> 显示字段值` 映射，再回填当前表结果。 |
| 多值处理 | 多个关联值保持原顺序；可去重批量读取，但回答时按原单元格顺序输出。 |

禁止事项：

- 不要把 link 单元格里的 `record_id` 当作最终答案。
- 不要用 `+record-search` 搜索 link `record_id` 来查关联记录。
- 不要凭关联 `record_id` 猜名称、负责人、门店等显示值。
- 不要只看当前表字段名推断关联表结构；跨表读取前必须拿关联表字段结构。

## 7. 命令 help

- `HelpFirst`: 参数、示例、JSON shape 和取值约束以 `lark-cli base +record-get --help`、`+record-search --help`、`+record-list --help` 为准。
- `RecordSearchJson`: 构造 `+record-search --json` 前先看 `+record-search --help`，确认 `keyword/search_fields/select_fields/view_id/offset/limit` 的结构和约束。
- `RecordListProjection`: 构造 `+record-list` 前先看 `+record-list --help`，确认 `--field-id`、`--view-id`、`--offset`、`--limit` 的语义。

## 参考

- [lark-base-view-set-filter.md](lark-base-view-set-filter.md)
- [lark-base-view-set-sort.md](lark-base-view-set-sort.md)
- [lark-base-view-set-visible-fields.md](lark-base-view-set-visible-fields.md)
- [lark-base-data-query.md](lark-base-data-query.md)
