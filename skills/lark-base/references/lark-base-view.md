# View：类型选择与生命周期

View 是同一 Table 的展示与组织方式，共享底层记录；创建视图不会复制记录。只创建满足当前需求的视图，不默认把五种类型全部建一遍。一次性查询用 Record 命令；需要用户长期浏览、处理或共享时创建 View。

## 选择视图

| 类型 | 展示方式与优势 | 何时选用 | 优先配置 |
|---|---|---|---|
| `grid` 表格 | 每行一条记录、每列一个字段，纵向浏览；分组组织行，排序改变行顺序。信息密度高，便于逐项对照与维护。 | 默认选择；明细台账、多字段比较、批量核对。 | 可见字段及顺序；按需筛选、分组、排序。 |
| `kanban` 看板 | 按分组字段横向排列列，每列展示该组的记录卡片；排序控制组内记录顺序。 | 需要按状态、阶段或类别分栏处理事项；优先选用有清晰选项的单选/多选字段作为分组依据。 | 分组字段、卡片可见字段、组内排序；有附件时可选封面。多选分组不可直接当作互斥分区统计。 |
| `gantt` 甘特图 | 左侧为表格明细，右侧为同一批记录的时间条；可直观看到起止时间、持续时间及排期重叠。左侧支持可见字段、筛选、分组和排序。 | 每条记录代表具有时间跨度的任务、项目或其他实体，重点是比较排期。 | `timebar` 绑定开始、结束和标题字段；左侧通常只留 1–3 个关键字段，为时间轴留空间。 |
| `calendar` 日历 | 将记录按时间字段定位到日历日期格，以事项形式展示；方便回答“某天有哪些事”。 | 发布计划、活动、预约、任务日期等，重点是按日/周/月浏览。 | `timebar` 绑定开始、结束和标题字段；配置展示字段和筛选。不要套用表格的通用分组、排序。 |
| `gallery` 画册 | 记录直接以卡片排列，不按状态分栏；重点内容可配附件封面。 | 产品、素材、案例、人员等需要逐卡浏览的集合；不要求每条记录有图片。 | 卡片展示字段、可选封面、筛选与排序。 |

选择捷径：**比较字段 → grid；按状态/类别处理 → kanban；比较时间跨度 → gantt；按日期找事项 → calendar；浏览卡片内容 → gallery。** Gantt 和 Calendar 都能展示时间相关实体，区别是前者强调跨度与重叠，后者强调日期位置。

## 当前 CLI 配置范围

“✓”表示该类型有对应 CLI 配置入口；不等于支持界面中的所有设置。

| 操作 | grid | kanban | gantt | calendar | gallery |
|---|:---:|:---:|:---:|:---:|:---:|
| 创建、查询、改名、删除 | ✓ | ✓ | ✓ | ✓ | ✓ |
| `+view-set-filter` 筛选 | ✓ | ✓ | ✓ | ✓ | ✓ |
| `+view-set-visible-fields` 字段显隐/顺序 | ✓ | ✓ | ✓ | ✓ | ✓ |
| `+view-set-sort` 排序 | ✓ | ✓ | ✓ | — | ✓ |
| `+view-set-group` 分组 | ✓ | ✓ | ✓ | — | — |
| `+view-set-timebar` 时间条 | — | — | ✓ | ✓ | — |
| `+view-set-card` 卡片封面 | — | ✓ | — | — | ✓ |

- 每个 `set` 都有对应 `get`：例如 `+view-get-group`、`+view-get-timebar`。修改已有配置时先读取，只在已知原状态的基础上构造目标配置。
- `visible_fields` 是最终可见字段的完整有序列表，遗漏字段会隐藏；主字段可能被服务端固定在首位。隐藏展示字段不删除字段或记录。
- `sort_config` 最多 10 项；`group_config` 最多 3 项，具体分组字段须适用于目标视图。看板通常先配置一个明确的分类字段。`group_config[].desc` 控制分组排序，记录顺序使用 `sort_config`。
- `timebar` 必须提供 `start_time`、`end_time`、`title`；前两者引用日期/时间字段，`title` 引用用于标识事项的字段。不要用创建时间代替用户真正要求的任务排期；记录也需要有相应时间值。
- `card` 当前通过 `cover_field` 选择附件字段或用 `null` 清除封面。截图中的卡片紧凑模式、字段名开关、行高、填色等，不据此臆造 CLI 参数。
- `form` 不属于这里的五种创建类型，表单使用 Form 命令。

## Few-shot：按目的创建与配置

以下是相互独立的选型示例，不是一套必须全执行的步骤。`BASE_TOKEN`、`TABLE_ID` 使用已解析的真实资源坐标；字段名示例假定目标表已有相应字段，配置前用 `+field-list` 核实类型与名称。`--view-id` 接受真实 ID 或名称；示例使用新建视图的唯一名称，名称不唯一时使用实际返回 ID。

### 明细台账：grid

需求：“按项目分组查看任务，优先显示最早截止的任务。”

```bash
lark-cli base +view-create --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --json '{"name":"任务明细","type":"grid"}' --as user
lark-cli base +view-set-visible-fields --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务明细" --json '{"visible_fields":["任务名称","项目","状态","截止时间"]}' --as user
lark-cli base +view-set-group --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务明细" --json '{"group_config":[{"field":"项目","desc":false}]}' --as user
lark-cli base +view-set-sort --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务明细" --json '{"sort_config":[{"field":"截止时间","desc":false}]}' --as user
```

### 按状态处理任务：kanban

需求：“待办、进行中、已完成各一列，每列按截止时间排列。”前置：状态字段是包含相应选项的单选字段。

```bash
lark-cli base +view-create --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --json '{"name":"任务看板","type":"kanban"}' --as user
lark-cli base +view-set-group --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务看板" --json '{"group_config":[{"field":"状态","desc":false}]}' --as user
lark-cli base +view-set-visible-fields --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务看板" --json '{"visible_fields":["任务名称","负责人","截止时间"]}' --as user
lark-cli base +view-set-sort --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务看板" --json '{"sort_config":[{"field":"截止时间","desc":false}]}' --as user
```

### 比较任务排期：gantt

需求：“查看任务开始到结束的排期，左侧只保留任务和负责人。”

```bash
lark-cli base +view-create --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --json '{"name":"任务排期","type":"gantt"}' --as user
lark-cli base +view-set-timebar --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务排期" --json '{"start_time":"开始时间","end_time":"结束时间","title":"任务名称"}' --as user
lark-cli base +view-set-visible-fields --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "任务排期" --json '{"visible_fields":["任务名称","负责人"]}' --as user
```

### 按日期浏览活动：calendar

需求：“在日历上查看每项活动的安排。”

```bash
lark-cli base +view-create --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --json '{"name":"活动日历","type":"calendar"}' --as user
lark-cli base +view-set-timebar --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "活动日历" --json '{"start_time":"活动开始","end_time":"活动结束","title":"活动名称"}' --as user
```

### 浏览产品卡片：gallery

需求：“以图片卡片浏览产品，展示名称、分类和价格。”前置：产品图片是附件字段。

```bash
lark-cli base +view-create --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --json '{"name":"产品画册","type":"gallery"}' --as user
lark-cli base +view-set-card --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "产品画册" --json '{"cover_field":"产品图片"}' --as user
lark-cli base +view-set-visible-fields --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "产品画册" --json '{"visible_fields":["产品名称","分类","价格"]}' --as user
```

### 通用生命周期：发现、筛选、改名、清理

```bash
# 发现与检查：已有目标视图时直接配置它
lark-cli base +view-list --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --as user
lark-cli base +view-get --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "$VIEW_ID" --as user

# 只展示进行中的记录；复杂条件见下方筛选参考
lark-cli base +view-set-filter --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "$VIEW_ID" --json '{"logic":"and","conditions":[["状态","intersects",["进行中"]]]}' --as user

# 改名用 --name；创建用 --json 中的 name
lark-cli base +view-rename --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "$VIEW_ID" --name "进行中任务" --as user

# 用户明确要求删除该视图且目标已确认时执行
lark-cli base +view-delete --base-token "$BASE_TOKEN" --table-id "$TABLE_ID" --view-id "$VIEW_ID" --as user --yes
```

创建支持单对象或对象数组，`type` 默认 `grid`；批量创建逐项执行，中途失败时前面的视图可能已创建。超时、异常或同名冲突后先用 `+view-list` 确认状态，复用已创建的目标，避免整批盲重试。删除 View 移除该展示配置，不是删除底层记录。

配置的 `--json` 使用对应对象：`{"group_config":[...]}`、`{"sort_config":[...]}`、`{"visible_fields":[...]}`，不要把裸数组或 `group_by/property` 塞进创建请求。清除排序/分组分别传 `{"sort_config":[]}` / `{"group_config":[]}`；清除封面传 `{"cover_field":null}`。需要确认最终展示时，再读取对应配置或按视图读取记录。

筛选详细写法见 [View filter](lark-base-view-set-filter.md)；该文档继续路由公共条件协议。
