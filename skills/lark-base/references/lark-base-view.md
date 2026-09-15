# View：类型选择与生命周期

所有 View 编辑前必读本参考，包括创建、改名、配置修改和删除。

View 是同一 Table 的展示与组织方式，共享底层记录；创建视图不会复制记录。只创建满足当前需求的视图，不默认把五种类型全部建一遍。一次性查询用 Record 命令；需要用户长期浏览、处理或共享时创建 View。

## 选择视图

**Grid 是最常用的默认视图，方便查看、录入和修改数据。没有明确的特殊展示需求时使用 Grid，不因数据包含状态、日期或附件字段就自动创建其他类型。只有用户明确需要分栏处理、时间跨度比较、日历定位或卡片浏览时，才分别选择 Kanban、Gantt、Calendar 或 Gallery。**

| 类型 | 展示方式与优势 | 何时选用 | 优先配置 |
|---|---|---|---|
| `grid` 表格 | 每行一条记录、每列一个字段，采用熟悉的表格形式，方便查看、录入和修改数据；可通过筛选、分组、排序调整展示。 | 最常用的默认视图；日常读写数据，没有特殊展示需求时优先使用。 | 可见字段及顺序；按需筛选、分组、排序。 |
| `kanban` 看板 | 按分组字段横向排列列，每列展示该组的记录卡片；排序控制组内记录顺序。 | 需要按状态、阶段或类别分栏处理事项；优先选用有清晰选项的单选/多选字段作为分组依据。 | 分组字段、卡片可见字段、组内排序；有附件时可选封面。多选分组不可直接当作互斥分区统计。 |
| `gantt` 甘特图 | 左侧为表格明细，右侧为同一批记录的时间条；可直观看到起止时间、持续时间及排期重叠。左侧支持可见字段、筛选、分组和排序。 | 每条记录代表具有时间跨度的任务、项目或其他实体，重点是比较排期。 | `timebar` 绑定开始、结束和标题字段；左侧通常只留 1–3 个关键字段，为时间轴留空间。 |
| `calendar` 日历 | 将记录按时间字段定位到日历日期格，以事项形式展示；方便回答“某天有哪些事”。 | 发布计划、活动、预约、任务日期等，重点是按日/周/月浏览。 | `timebar` 绑定开始、结束和标题字段；配置展示字段和筛选。不要套用表格的通用分组、排序。 |
| `gallery` 画册 | 记录直接以卡片排列，不按状态分栏；重点内容可配附件封面。 | 产品、素材、案例、人员等需要逐卡浏览的集合；不要求每条记录有图片。 | 卡片展示字段、可选封面、筛选与排序。 |

选择捷径：**日常读写数据、无特殊展示需求 → grid；按状态/类别处理 → kanban；比较时间跨度 → gantt；按日期找事项 → calendar；浏览卡片内容 → gallery。** Gantt 和 Calendar 都能展示时间相关实体，区别是前者强调跨度与重叠，后者强调日期位置。

## Few-shot：按目的创建与配置

以下是相互独立的选型示例，不是一套必须全执行的步骤。`BASE_TOKEN`、`TABLE_ID` 使用已解析的真实资源坐标；字段名示例假定目标表已有相应字段，配置前用 `+field-list` 核实类型与名称。`--view-id` 接受真实 ID 或名称；示例使用新建视图的唯一名称，名称不唯一时使用实际返回 ID。

### 表格查看与维护：grid

需求：“按项目分组查看任务，优先显示最早截止的任务。”

```bash
# 默认视图；支持筛选、字段显隐、分组、排序；不支持时间条和卡片封面。
# 创建 JSON 支持对象/数组，type 默认 grid；不要塞入 group_by/property，form 走 Form 命令。
lark-cli base +view-create --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --json '{"name":"任务明细","type":"grid"}' --as user
# visible_fields 是完整有序列表；遗漏即隐藏，不删除数据，主字段可能固定在首位。
lark-cli base +view-set-visible-fields --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务明细" --json '{"visible_fields":["任务名称","项目","状态","截止时间"]}' --as user
# group_config 最多 3 项，字段须适用于目标视图；空数组清除分组。
lark-cli base +view-set-group --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务明细" --json '{"group_config":[{"field":"项目","desc":false}]}' --as user
# sort_config 最多 10 项；空数组清除排序。配置 JSON 用对象包装，不传裸数组。
lark-cli base +view-set-sort --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务明细" --json '{"sort_config":[{"field":"截止时间","desc":false}]}' --as user
```

### 按状态处理任务：kanban

需求：“待办、进行中、已完成各一列，每列按截止时间排列。”前置：状态字段是包含相应选项的单选字段。

```bash
# 支持筛选、字段显隐、分组、排序、卡片封面；不支持时间条。
# 优先按一个单选/多选字段分栏；group 的 desc 排列分组，sort 排列组内记录。
lark-cli base +view-create --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --json '{"name":"任务看板","type":"kanban"}' --as user
lark-cli base +view-set-group --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务看板" --json '{"group_config":[{"field":"状态","desc":false}]}' --as user
lark-cli base +view-set-visible-fields --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务看板" --json '{"visible_fields":["任务名称","负责人","截止时间"]}' --as user
lark-cli base +view-set-sort --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务看板" --json '{"sort_config":[{"field":"截止时间","desc":false}]}' --as user
```

### 比较任务排期：gantt

需求：“查看任务开始到结束的排期，左侧只保留任务和负责人。”

```bash
# 支持筛选、字段显隐、分组、排序、时间条；不支持卡片封面。
# timebar 必填开始、结束、标题；起止字段须为日期/时间且记录有值，按业务排期选择。
lark-cli base +view-create --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --json '{"name":"任务排期","type":"gantt"}' --as user
lark-cli base +view-set-timebar --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务排期" --json '{"start_time":"开始时间","end_time":"结束时间","title":"任务名称"}' --as user
# 左侧通常保留 1–3 个关键字段，为时间轴留空间。
lark-cli base +view-set-visible-fields --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务排期" --json '{"visible_fields":["任务名称","负责人"]}' --as user
```

### 按日期浏览活动：calendar

需求：“在日历上查看每项活动的安排。”

```bash
# 支持筛选、字段显隐、时间条；不支持通用分组、排序和卡片封面。
# timebar 必填开始、结束、标题；起止字段须为日期/时间且记录有值。
lark-cli base +view-create --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --json '{"name":"活动日历","type":"calendar"}' --as user
lark-cli base +view-set-timebar --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "活动日历" --json '{"start_time":"活动开始","end_time":"活动结束","title":"活动名称"}' --as user
```

### 浏览产品卡片：gallery

需求：“以图片卡片浏览产品，展示名称、分类和价格。”前置：产品图片是附件字段。

```bash
# 支持筛选、字段显隐、排序、卡片封面；不支持分组和时间条。
# cover_field 使用附件字段；传 null 清除封面。
lark-cli base +view-create --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --json '{"name":"产品画册","type":"gallery"}' --as user
lark-cli base +view-set-card --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "产品画册" --json '{"cover_field":"产品图片"}' --as user
lark-cli base +view-set-visible-fields --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "产品画册" --json '{"visible_fields":["产品名称","分类","价格"]}' --as user
```

### 通用生命周期：发现、筛选、改名、清理

```bash
# 五种视图均支持查询、改名、删除；已有目标视图时直接配置它。
# 修改已有配置先读对应 get（如 +view-get-group）；需要验收时再读回。
# 批量创建逐项执行，可能部分成功；异常或同名冲突后先 list 确认，避免盲重试。
lark-cli base +view-list --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --as user
lark-cli base +view-get --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "$VIEW_ID" --as user

# 只展示进行中的记录；复杂条件见下方筛选参考
lark-cli base +view-set-filter --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "$VIEW_ID" --json '{"logic":"and","conditions":[["状态","intersects",["进行中"]]]}' --as user

# 改名用 --name；创建用 --json 中的 name
lark-cli base +view-rename --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "$VIEW_ID" --name "进行中任务" --as user

# 用户明确要求且目标已确认时删除视图；不删除底层记录。
lark-cli base +view-delete --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "$VIEW_ID" --as user --yes
```

筛选详细写法见 [View filter](lark-base-view-set-filter.md)；该文档继续路由公共条件协议。
