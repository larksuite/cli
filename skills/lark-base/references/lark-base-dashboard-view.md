# 仪表盘视图组件

在仪表盘中展示任务明细、列表或时间安排时，使用 `+dashboard-block-create --type view`。组件归属于 Dashboard，通过 `base_token + dashboard_id + block_id` 管理。创建和更新的视图业务配置放在 `data_config`；外层 `type:view` 是组件类型，创建输入的 `view_type` 是展示类型。读取组件记录时，使用详情返回的 `tableId + viewId` 调用 `+record-list`。

## 读取视图组件数据

先从 `+dashboard-block-list` 确认目标组件的 `type=view`，再读取详情；已有可信组件详情和引用时直接复用：

```bash
lark-cli base +dashboard-block-get \
  --base-token BASE_EXAMPLE_TOKEN --dashboard-id DASHBOARD_EXAMPLE_ID \
  --block-id BLOCK_EXAMPLE_ID --as user
```

视图引用位于 `data.block.data_config`，使用 camelCase；`viewType` 是字符串展示类型，例如 `grid`、`gantt`、`gallery`：

```json
{
  "tableId": "tbl_example",
  "viewId": "vew_example",
  "viewType": "grid"
}
```

`tableId` 和 `viewId` 必须来自目标组件，不使用 `block_id` 或数据表默认视图代替。存量响应中，`tableId` 若是只含一个 ID 的数组，取其中唯一值；`viewId` 若是数字，转成字符串。引用缺失或表引用不能唯一确定时，先核对组件详情，不猜测数据源。

使用组件所属的 `base_token` 和这组真实引用读取记录：

```bash
lark-cli base +record-list \
  --base-token BASE_EXAMPLE_TOKEN --table-id tbl_example --view-id vew_example \
  --format ndjson --output ./view-records.ndjson --as user
```

- 默认省略 `--filter-json`、`--sort-json` 和 `--field-id`，继承视图的筛选、排序与可见字段；现有记录接口还会补充时间轴、封面和分组所需字段。用户要求额外条件或字段时再显式传入，对应部分会覆盖视图配置。
- 返回的是视图范围内的记录明细。页面布局、分组折叠状态、仪表盘全局筛选和点击联动不由这条查询模拟。
- 按 stdout 摘要及 manifest 的 `has_more` 判断完整性；需要全量且 `has_more=true` 时，使用 manifest 的 `next_offset` 作为下一次 `--offset`，写入另一个 artifact。完整读取与一致性检查见 [Record 读取](../SKILL.md#record)。
- 视图不存在或不可访问时，报告该组件读取失败并核对引用和权限；不要去掉 `--view-id` 退回整表，也不要把失败当作空数据。

`+dashboard-block-get-data` 用于图表计算结果；视图组件的数据读取统一走上述记录查询路径。

## 创建

先用 `+table-list` 和 `+field-list` 确认真实表名、字段名及字段类型。将以下配置保存为 `view.json`：

```json
{
  "table_name": "任务表",
  "view_type": "grid",
  "filter": {
    "conjunction": "and",
    "conditions": [
      {"field_name": "状态", "operator": "is", "value": "进行中"}
    ]
  },
  "group_by": [
    {"field_name": "状态", "mode": "integrated", "sort": {"type": "group", "order": "asc"}}
  ],
  "sort_by": [{"field_name": "截止时间", "order": "asc"}],
  "visible_fields": ["任务名称", "状态", "负责人", "截止时间"]
}
```

```bash
lark-cli base +dashboard-block-create \
  --base-token BASE_EXAMPLE_TOKEN --dashboard-id DASHBOARD_EXAMPLE_ID \
  --name "进行中的任务" --type view --data-config @view.json
```

可附加 `--position '{"x":0,"y":0,"w":12,"h":8}'` 指定布局。位置是与 `data_config` 平级的组件属性。

| 配置 | 创建输入 |
|------|----------|
| `table_name` | 必填，非空表名 |
| `view_type` | 必填，非空展示类型；例如 `grid`、`gantt`、`gallery`，完整支持范围由组件服务端决定 |
| `filter` | 可选，使用仪表盘的 `conjunction + conditions[]` 对象协议，见 [Dashboard Block 配置](lark-base-dashboard-block-config.md) |
| `group_by` | 可选数组；每项必须有非空 `field_name`，可指定 `mode` 和 `sort:{type,order}` |
| `sort_by` | 可选数组；每项必须有非空 `field_name` 和 `order:asc/desc`，数组顺序保留 |
| `visible_fields` | 可选字段名数组，保留展示顺序 |

创建、更新默认不接受 `data_config` 顶层的 `timebar`、`card`，即使值为 `{}` 或 `null` 也会报参数错误。`view_type` 的支持范围和校验规则不变，仍由服务端决定各展示类型的有效配置。

视图组件无需 `series`、`count_all`，也不接受图表指标配置。筛选沿用仪表盘公共校验（1～50 条条件，数组值最多 200 项）；按真实字段类型填写值，不将筛选转换为数据表 `+view-set-filter` 的 tuple 协议。

`group_by` 沿用仪表盘现有规范化：`mode`、`sort.type/order` 转小写，`sort.type` 为 `group` 或 `view` 且省略 `order` 时补 `asc`。CLI 不套用普通图表的两个分组维度上限，也不将数据表视图接口的类型限制或数量上限推定为组件限制。表和字段解析、展示类型及分组模式枚举、各配置对不同视图类型的适用性由服务端校验。

### 甘特图

```json
{
  "table_name": "任务表",
  "view_type": "gantt"
}
```

### 画册

```json
{
  "table_name": "任务表",
  "view_type": "gallery"
}
```

## 更新与回读

更新已有组件时，用 `+dashboard-block-update` 提交变动的 `data_config` 字段。无需重复提供外层 `type`，也无需为只改排序而重传 `table_name` 或 `view_type`：

```bash
lark-cli base +dashboard-block-update \
  --base-token BASE_EXAMPLE_TOKEN --dashboard-id DASHBOARD_EXAMPLE_ID \
  --block-id BLOCK_EXAMPLE_ID \
  --data-config '{"sort_by":[{"field_name":"截止时间","order":"desc"}]}'

lark-cli base +dashboard-block-get \
  --base-token BASE_EXAMPLE_TOKEN --dashboard-id DASHBOARD_EXAMPLE_ID \
  --block-id BLOCK_EXAMPLE_ID

# 使用上一步返回的 tableId / viewId 核对排序配置
lark-cli base +view-get-sort \
  --base-token BASE_EXAMPLE_TOKEN --table-id tbl_example --view-id vew_example
```

CLI 对更新做 JSON 对象解析、`timebar/card` 写入限制检查和现有分组规范化，不将创建时的必填字段套到局部更新。空数组、`filter:null` 均保留在请求中；它们的清空行为、嵌套对象合并、更换表或展示类型后的旧配置处理由服务端决定。修改嵌套对象前读取完整现状，只改要求变更的内容并按完整子对象提交；更换表或展示类型时提交适用于目标的完整配置，随后回读验证。

创建、更新响应在 `data.block` 中返回服务端结果。GET 的 `data.block.data_config` 返回 `tableId / viewId / viewType` 等视图引用，不应假定它会原样返回创建、更新输入的 `table_name / view_type / filter / group_by / sort_by / visible_fields` 等业务配置。缺少这些输入字段不代表配置未保存，不要据此重发整份配置。输入字段 `view_type` 和返回字段 `viewType` 均使用字符串展示类型，字段命名保持各自协议，完整支持范围由服务端决定；按返回的 `viewType` 核对展示类型。

修改嵌套配置前或写入后验证时，保持同一 `base_token`，将组件返回的 `tableId / viewId` 分别传入 `--table-id / --view-id`，按需调用：

| 要检查的配置 | 读取命令 |
|--------------|----------|
| 筛选 `filter` | `+view-get-filter` |
| 排序 `sort_by` | `+view-get-sort` |
| 分组 `group_by` | `+view-get-group` |
| 可见字段 `visible_fields` | `+view-get-visible-fields` |

已有的时间轴、卡片配置可分别用 `+view-get-timebar`、`+view-get-card` 检查。读取结果遵循各视图属性接口协议，不能直接当作组件写入的 `data_config` 重发；更新仍按上文的组件输入协议提交变更字段。需要记录明细时按上方读取流程执行。页面展示效果按实际任务验收。

写入限制不改变读取行为；服务端返回的已有 `timebar/card` 配置仍原样输出。独立的数据表视图配置命令不受此限制影响。

`--no-validate` 跳过本地语义校验和规范化，包括上述 `timebar/card` 检查，仍要求合法 JSON 对象；透传不代表服务端支持这些参数。内联 JSON、`@file`、dry-run 和执行共用同一份解析结果。`--dry-run` 只验证请求，不证明服务端已部署。
