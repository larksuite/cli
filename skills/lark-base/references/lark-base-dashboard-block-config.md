# Base Dashboard Block 配置

Block 的 `data_config` 字段因 `type` 不同而变化。本文档是 Dashboard block `data_config` 的单一事实来源（SSOT），包含组件类型、字段结构、筛选格式、约束和可复制模板。普通图表使用扁平单数据源配置；倒计时使用下文的 fixed/field 专属配置；进度图使用嵌套的 `progress`，见[进度图创建、更新和获取](lark-base-dashboard-progress.md)。BaseApp 图表的外层结构不同，但每个 `data_sources[]` 元素复用本文的字段取值、筛选、分组、排序及规范化规则（限普通图表）；创建或更新 App 组件时，还必须读取 [BaseApp Block data_config](lark-base-app-block-data-config.md) 了解共享 `base_token`、多数据源封装，以及 App 独有的列表组件协议。倒计时和进度图仅支持 Dashboard。

图表联动是与 `data_config` 平级的顶层 `linkage`，通过 `--linkage` 更新。配置同源目标、追加或清除前读取 [图表联动](lark-base-dashboard-linkage.md)，不要把它放进 `data_config`。

## 支持的组件类型（`type` 枚举）

| type 值 | 说明 |
|---------|------|
| `column` | 柱状图 |
| `bar` | 条形图 |
| `line` | 折线图 |
| `pie` | 饼图 |
| `ring` | 环形图 |
| `area` | 面积图 |
| `combo` | 组合图 |
| `scatter` | 散点图 |
| `funnel` | 漏斗图 |
| `wordCloud` | 词云 |
| `radar` | 雷达图 |
| `ranking` | 排行榜 |
| `statistics` | 指标卡 |
| `nps` | NPS 图 |
| `pivotTable` | 透视表 |
| `slicer` | 单数据源、单字段的离散值切片器（仅 Dashboard） |
| `countdown` | 倒计时（固定目标时间或日期字段取值） |
| `progress` | 进度图（数值／日期，使用独立的 `progress` 配置） |
| `view` | 视图组件（筛选、分组、排序、可见字段），见 [视图组件](lark-base-dashboard-view.md) |
| `text` | 文本（支持 Markdown） |
| `button` | 按钮 |

## 字段类型与操作符速查（AI 决策用）

> 先用 `+field-list` / `+field-get` 确认字段 `type`；本节使用当前字段接口里的 canonical 类型名：`number`、`text`、`select`、`datetime`、`checkbox`、`user`。NPS 使用的 `Rating` 是 Dashboard 服务端识别的评分字段语义，不属于当前字段操作符速查里的通用筛选类型。

```
text: is, isNot, contains, doesNotContain, isEmpty, isNotEmpty
number: is, isNot, isGreater, isGreaterEqual, isLess, isLessEqual, isEmpty, isNotEmpty
select（multiple=false）: is, isNot, isEmpty, isNotEmpty
select（multiple=true）: is, isNot, contains, doesNotContain, isEmpty, isNotEmpty
datetime: is, isGreater, isLess, isEmpty, isNotEmpty
checkbox: is (value: true/false)
user / created_by / updated_by: is, isNot, isEmpty, isNotEmpty
```

`isGreaterEqual` / `isLessEqual` 不是全局不支持：它们可用于 `number`，但不能用于 `datetime` / `created_at` / `updated_at`。日期范围必须用 `isGreater` / `isLess` 配合 `ExactDate`；不要把数字字段的操作符集合套到日期字段上。

## 普通图表 data_config 结构

以下指标、分组和排序规则适用于普通图表；`slicer` 使用下方独立契约，不需要指标；`progress` 使用[进度图独立契约](lark-base-dashboard-progress.md)；`view` 使用[视图组件配置](lark-base-dashboard-view.md)，无需指标且不受图表分组数量限制。

| 字段 | 类型 | 说明 |
|------|------|------|
| `table_name` | string | 关联数据表名称 |
| `series` | `[{ "field_name": "xxx", "rollup": "SUM" }]` | 指标/Y 轴（与 `count_all` 二选一）。rollup 支持 `SUM` / `MAX` / `MIN` / `AVERAGE` |
| `count_all` | boolean | COUNTA 聚合，统计所有记录数（与 `series` 二选一） |
| `group_by` | `[{ "field_name": "xxx", "mode": "integrated", "sort": {...} }]` | X 轴分组维度。`mode` 和 `sort` 的要求因组件类型而异，见下方说明 |
| `filter` | object | 筛选条件 |
| `filter.conjunction` | `"and"` / `"or"` | 筛选逻辑 |
| `filter.conditions` | `[{ "field_name", "operator", "value" }]` | 筛选条件数组，value 类型因字段类型而异（见下方 filter 格式规则） |
| `category_range` | `[min, detractorMax, passiveMax, max]` | NPS 三段边界，仅 `nps` 类型支持；首尾必须等于 Rating 字段量程，首尾匹配由服务端按字段元数据校验 |

### slicer 切片器契约

创建使用 `+dashboard-block-create --type slicer --data-config '…'`。`data_config` 必填，公开字段为：

| 字段 | 创建 | 更新 |
|------|------|------|
| `table_name` | 必填，当前 Base 的表名 | 省略保留；换表须同时提供新 `group_by` |
| `group_by` | 必填，恰好一个有 `field_name` 的对象 | 提供时整体替换，仍恰好一个字段 |
| `group_by[0].mode` | 可选，`integrated` / `enumerated` | 随 `group_by` 整体替换，不继承被省略的子字段 |
| `filter` | 可选对象，省略表示不筛选；不接受 `null` | 同表省略保留，对象整体替换，`null` 清空 |
| `displayMode` | `dropdown` / `tiled` | 可单独提供 |
| `selectMode` | `single` / `multiple` | 可单独提供 |
| `order` | `asc` / `desc` | 可单独提供 |

创建时 `displayMode`、`selectMode`、`order` 必须一起提供或一起省略。全部省略时，CLI 补为 `tiled + multiple + asc`；部分提供或显式非法值会报错，不补缺项。更新不补创建默认值。`order` 不接受数字 `1/2` 或大写 `ASC/DESC`，创建、更新与获取统一使用小写字符串。

最小创建配置：

```json
{
  "table_name": "订单表",
  "group_by": [{ "field_name": "地区" }]
}
```

完整创建配置：

```json
{
  "table_name": "订单表",
  "group_by": [{ "field_name": "地区", "mode": "integrated" }],
  "filter": {
    "conjunction": "and",
    "conditions": [
      { "field_name": "状态", "operator": "is", "value": ["已完成"] }
    ]
  },
  "displayMode": "dropdown",
  "selectMode": "multiple",
  "order": "asc"
}
```

更新使用 `+dashboard-block-update --block-id <block_id> --data-config '…'`，并照常提供 `--base-token` 和 `--dashboard-id`。不传 `--type`，目标类型及字段有效性由服务端校验。常用补丁如下：

| 意图 | `--data-config` 内容 |
|------|----------------------|
| 只改排序 | `{"order":"desc"}` |
| 只改展示 | `{"displayMode":"dropdown"}` |
| 只改单选 | `{"selectMode":"single"}` |
| 清空筛选 | `{"filter":null}` |
| 切换表和字段 | `{"table_name":"客户表","group_by":[{"field_name":"客户地区"}]}` |

- 同表更新未传字段保留原值。`filter` 对象替换全部条件，不追加旧条件；不能用 `conditions: []` 清空。普通图表更新也支持 `filter:null`。
- 只有 `table_name` 解析到不同表 ID 才视为换表，此时必须同时提供新表的唯一 `group_by`；未传 `filter` 清除旧表筛选，提供时按新表校验。传回原表名按同表更新处理。
- `data_config: {}` 对受支持切片器不修改配置；完全省略 `--data-config` 可只改名称或位置。`--data-config null`、数组或非对象值会报错，包括使用 `--no-validate` 时。
- 不接受 `count_all`、`series`、顶层 `sort`、`group_by[].sort` 或内部快照字段。底层固定计数由服务端处理。无需先读取再回传完整配置。
- 文本、日期和数字字段均可使用下拉或平铺；日期／数字范围组件、多数据源或多字段映射的切片器不支持数据配置更新，但仍可只改名称、位置。BaseApp 组件不支持 `type=slicer`。
- 联动配置由服务端保留。表、字段或选择模式改变时清理默认选中项；只改排序、展示、筛选或同字段分组模式时保留。默认选中项、联动列表和字段类型元数据不属于可写 `data_config`。
- `+dashboard-block-get` 返回当前配置；创建和更新响应也使用服务端回读结果。无筛选时省略 `filter`。范围模式或展示配置不完整时，获取可能只返回数据源配置，不代表其数据配置可更新；CLI 不补展示默认值。
- `--no-validate` 跳过本地语义校验和默认值处理，保留原输入，由服务端校验；JSON 仍须为合法对象。`--data-config @file.json` 与内联 JSON 使用相同规则，dry-run 与执行使用同一份解析后的配置。

### text 类型特殊结构

`text` 类型组件用于展示富文本内容，**不需要数据源配置**（无 `table_name`、`series`、`group_by`、`filter`）。

| 字段 | 类型 | 说明 |
|------|------|------|
| `text` | string | **必填**。支持 Markdown 语法，详见下方说明 |

**支持的 Markdown 语法：**

| 语法 | 示例 | 效果 |
|------|------|------|
| 一级标题 | `# 标题` | 大标题 |
| 二级标题 | `## 标题` | 中标题 |
| 三级标题 | `### 标题` | 小标题 |
| 加粗 | `**文字**` | **文字** |
| 斜体 | `*文字*` | *文字* |
| 删除线 | `~~文字~~` | ~~文字~~ |
| 有序列表 | `1. 项目` | 1. 项目 |
| 无序列表 | `- 项目` | - 项目 |

> **注意**：以上未提及的 Markdown 语法（如链接、图片、代码块、表格等）均不支持。

### pivotTable 类型特殊结构

透视表使用独立的行、列和值协议，不使用 `series` / `group_by` / `count_all`。Create 必须传 `table_name`；`rows`、`columns`、`values` 缺省为空数组。字段名必须来自真实表结构。

```json
{
  "table_name": "订单",
  "rows": [{"field_name":"地区","group_type":"RAW"}],
  "columns": [{"field_name":"日期","group_type":"DATE_YEAR_MONTH"}],
  "values": [{"field_name":"销售额","rollup":"SUM","name":"销售额合计"}],
  "sort": [{
    "group_ref":{"area":"rows","index":0},
    "sort_type":"VALUE",
    "value_ref":{"area":"values","index":0},
    "order":"desc"
  }]
}
```

- `group_type` 支持 `RAW` 及 `DATE_YEAR`、`DATE_YEAR_QUARTER`、`DATE_YEAR_MONTH`、`DATE_YEAR_MONTH_DAY`、`DATE_YEAR_WEEK`、`DATE_MONTH_DAY`、`DATE_QUARTER`、`DATE_MONTH`、`DATE_WEEKDAY`。
- `rollup` 支持 `SUM`、`COUNT`、`COUNT_DISTINCT`、`AVERAGE`、`MAX`、`MIN`、`MEDIAN`。
- `field_key` 是透视表配置项的稳定标识，不替代 `field_name`。Create 的 `rows` / `columns` / `values` 只传 `field_name`，禁止提交 `field_key`。Update 保留已有配置项时，同时携带 Get 返回的 `field_key`、`field_name` 以及当前 `group_type` 或 `rollup`；需要替换字段、分组方式或聚合方式时省略原 `field_key`，由服务端生成新的配置项标识。
- `sort` 是排序数组，可同时声明多条字段或指标排序；传 `[]` 清空排序。Create 使用 `{area,index}` 引用本次请求中的数组项。Update 可使用 `group_field_key` / `value_field_key` 引用更新后的配置项，也可使用 `{area,index}`；同一引用位置只能选择一种方式。
- Update 顶层字段是局部更新，但显式传入的 `rows` / `columns` / `values` 各自整体替换。切换 `table_name` 时三组数组必须全部显式提供。
- `view_name` 与 `filter` 互斥；传 `null` 可清除对应的数据源约束。

### button 类型特殊结构

`button` 类型组件不要求 `table_name`、`series` 或 `count_all`。按钮文案使用 `label`：

```json
{"label":"运行工作流"}
```

创建时可直接使用 `--type button --data-config '{"label":"运行工作流"}'`，无需 `--no-validate`。CLI 不对按钮专属字段增加必填或非空限制，由服务端校验；`data_config` 仍须是合法 JSON 对象，且 `number_format` 仍仅限 `statistics` 类型。

更新按钮文案时只传 `--data-config '{"label":"新的按钮文案"}'`，无需补齐数据源或指标字段。

## countdown 倒计时

复用 `+dashboard-block-create` / `+dashboard-block-update` / `+dashboard-block-get`。创建要求完整配置，更新接受局部配置，获取返回当前模式的完整配置。创建参数 `--type countdown` 是组件类型；`data_config.type` 是可切换的 `fixed` / `field` 模式。

倒计时公开字段使用 **camelCase**：`targetTime`、`aggregateType`、`additionalDatasources`、`tableName`、`countAll`、`groupBy`、`fieldName`。这些字段不使用普通图表的 snake_case，CLI 不改写字段名、枚举大小写或补充默认值。

### 创建固定时间倒计时

```bash
lark-cli base +dashboard-block-create \
  --base-token BASE_EXAMPLE_TOKEN --dashboard-id DASHBOARD_EXAMPLE_ID \
  --name "发布倒计时" --type countdown \
  --data-config '{"type":"fixed","targetTime":1798732800000}'
```

`targetTime` 必填，为非负整数毫秒时间戳；允许 `0` 和过去时间。fixed 不接受 `aggregateType` 或 `additionalDatasources`，包括 null 和空数组。Base 至少需要一张表供底层渲染使用，该占位表不出现在公开配置中。

### 创建字段倒计时

把下面的完整 JSON 保存为 `countdown.json`，传给 `--data-config @countdown.json`：

```json
{
  "type": "field",
  "aggregateType": "MIN",
  "additionalDatasources": [
    {
      "tableName": "任务表",
      "countAll": true,
      "groupBy": [{ "fieldName": "截止时间", "mode": "integrated" }],
      "filter": {
        "conjunction": "and",
        "conditions": [{ "fieldName": "状态", "operator": "isNot", "value": "已完成" }]
      }
    }
  ]
}
```

| 字段 | 约束 |
|------|------|
| `aggregateType` | 必填，`MIN` 取最早日期，`MAX` 取最晚日期 |
| `additionalDatasources` | 必填，恰好一个数据源 |
| `tableName` | 必填，真实非空表名 |
| `countAll` / `series` | 必须二选一；`countAll` 只能为 true；`series` 为 1～20 项，每项必填 `fieldName` 和 `rollup: SUM\|MAX\|MIN\|AVERAGE` |
| `groupBy` | 必填，恰好一个日期字段；`fieldName` 必须引用日期、创建时间或修改时间字段 |
| `groupBy[].mode` | 可选，`integrated` / `enumerated`；省略时后端使用 `integrated` |
| `groupBy[].sort` | 可选，`type: group\|value\|view` 必填，`order: asc\|desc` 可选 |
| `filter` | 可选，`conjunction: and\|or`，`conditions` 为 1～50 项；每项使用 `fieldName`、`operator` 和按操作符需要的 `value` |

日期字段提供目标时间，`aggregateType` 决定取最早还是最晚日期；`series` 是数据源指标配置。使用 series 时，例如将 `countAll:true` 替换成 `"series":[{"fieldName":"工期","rollup":"SUM"}]`。field 不接受 `targetTime`。

筛选操作符与值沿用普通图表的字段类型规则，但字段名键使用 `fieldName`。CLI 校验结构；表名解析、字段类型以及筛选值是否合法由后端判断。创建不接受 `filter:null`。

### 局部更新和切换模式

```bash
# 已有 fixed 模式：只改目标时间
lark-cli base +dashboard-block-update \
  --base-token BASE_EXAMPLE_TOKEN --dashboard-id DASHBOARD_EXAMPLE_ID \
  --block-id BLOCK_EXAMPLE_ID --data-config '{"targetTime":1798732800000}'

# 已有 field 模式：只清空筛选
lark-cli base +dashboard-block-update \
  --base-token BASE_EXAMPLE_TOKEN --dashboard-id DASHBOARD_EXAMPLE_ID \
  --block-id BLOCK_EXAMPLE_ID --data-config '{"additionalDatasources":[{"filter":null}]}'
```

| 更新场景 | 行为 |
|----------|------|
| 省略 `type` | 沿用当前模式；显式传相同模式也按局部更新处理 |
| fixed 只传 `targetTime` | 保留其他配置；field 只传 targetTime 会被拒绝 |
| field 只传 `aggregateType` | 保留数据源、日期字段、指标、筛选 |
| 提供 `additionalDatasources` | 必须恰有一项，按唯一对象的字段合并；`[{}]` 不修改数据源 |
| 提供 `groupBy` / `series` | 数组整体替换；groupBy 项仍需 fieldName，省略 mode 使用 integrated，省略 sort 不继承旧排序 |
| 同模式、同表更新筛选 | 省略 filter 保留；对象整体替换；null 清空；空 conditions 不表示清空 |
| 切换指标方式 | countAll:true 移除旧 series；非空 series 移除旧 countAll；两者同传、countAll:false 或空 series 均拒绝 |
| 切换到 fixed | 显式传 `{"type":"fixed","targetTime":...}` |
| 切换到 field | 显式传完整 field 配置，包括 aggregateType 和唯一数据源；不继承历史隐藏筛选 |
| field 更换表 | 提供完整新数据源：tableName、唯一日期 groupBy、countAll:true 或非空 series；可省略 type/aggregateType，省略新 filter 表示不筛选 |

只有解析后的表 ID 改变才算换表；传回同一表名仍按局部更新。`data_config:{}` 不修改配置；省略整个 data_config 可以仅修改 name 或 position。data_config 本身不能为 null、数组或其他非对象值。

CLI 保留补丁中的省略字段、空对象、null、0、false，后端结合当前组件验证模式切换、换表完整性和字段引用后提交。更新前获取配置用于确认现状，提交时只发送需要修改的字段。从获取结果中删除 filter 再提交不能清空筛选，必须显式使用 filter:null。

### 获取与写入响应

`+dashboard-block-get` 的 `data.block.data_config` 返回完整的当前模式配置。fixed 返回 type/targetTime；field 返回 type/aggregateType/additionalDatasources。创建和更新也回读完整 block，分别附带 `data.created:true` / `data.updated:true`。

回读会归一化默认 mode、排序或筛选值，不保证与请求 JSON 完全相同。清空后省略 filter；隐藏固定时间、占位数据源、units/title/animation 等未开放展示配置不属于公开 data_config。读取配置使用 `+dashboard-block-get`。

## group_by 详细说明

### mode 枚举

| mode | 含义 | 适用场景 |
|------|------|----------|
| `integrated` | 聚合分组（默认） | 绝大部分场景，按字段值分组统计 |
| `enumerated` | 多值拆分统计 | 多选、人员等多值字段，将每个选项/人员拆开独立统计 |

> 多选、人员等多值字段默认用 `enumerated`；其他字段默认用 `integrated`。

### sort 排序

| sort.type | 含义 | 典型场景 |
|-----------|------|----------|
| `group` | 按横轴值排序 | 按月份升序、按品类名字母序 |
| `value` | 按纵轴值排序 | 按销售额从大到小 |
| `view` | 按数据源记录顺序 | 保持原表行序（不常用） |

`sort.order`：`asc`（升序）/ `desc`（降序）

只要写 `sort` 对象，就需要明确排序方向。CLI 会把 `sort.type` 为 `group` 或 `view` 且缺少 `order` 的情况规范化为 `order:"asc"`；`sort.type:"value"` 必须显式写 `order:"asc"` 或 `order:"desc"`，因为指标值排序方向会改变业务含义。

如果表中行序就是业务顺序，首次创建 block 时就一次性设置 `sort:{"type":"view","order":"asc"}` 保留行序，避免创建后再二次更新排序条件。

### ranking 排行榜专属契约

排行榜只支持一个分组和一个指标，公开字段固定为 `table_name`、`series`/`count_all`、`group_by`、`filter`、`limit_size`：

- `group_by` 必填且长度严格为 1；`mode` 仅支持 `integrated` / `enumerated`。
- `series` 长度严格为 1，且与 `count_all:true` 二选一；`rollup` 仅支持 `SUM` / `MAX` / `MIN` / `AVERAGE`。
- 排序只写在 `group_by[0].sort`，`type` 只能为 `value`，`order` 为 `asc` / `desc`。创建时省略排序默认按指标值降序。
- `limit_size` 是 Top N，取值为 `1..500` 的整数，创建时省略默认 `10`。
- 不支持顶层 `sort`、公开 `ranking` 对象或头像开关。

更新 `ranking` 时，`data_config` 是顶层 patch：只传 `limit_size` 只改 Top N；只传 `group_by` 只替换唯一分组和排序；只传 `series` 或 `count_all:true` 只切换指标；只传 `filter` 只替换筛选。切换 `table_name` 时必须在同一 patch 提供新的 `group_by` 以及 `series` 或 `count_all:true`；未传 `filter` 保留原筛选，未传 `limit_size` 保留原 Top N。

示例 — 柱状图按销售额降序：

```json
{
  "table_name": "订单表",
  "series": [{ "field_name": "金额", "rollup": "SUM" }],
  "group_by": [{ "field_name": "类别", "mode": "integrated", "sort": {"type": "value", "order": "desc"} }]
}
```

## filter 格式规则

**基本结构：**

```json
{
  "filter": {
    "conjunction": "and",
    "conditions": [
      { "field_name": "字段名", "operator": "操作符", "value": "值" }
    ]
  }
}
```

**多条件示例（and/or）：**

```json
{
  "filter": {
    "conjunction": "and",
    "conditions": [
      { "field_name": "状态", "operator": "is", "value": "已完成" },
      { "field_name": "金额", "operator": "isGreater", "value": 1000 }
    ]
  }
}
```

**操作符：**

| 操作符 | 含义 | 是否需要 value |
|--------|------|---------------|
| `is` | 等于 | 是 |
| `isNot` | 不等于 | 是 |
| `contains` | 包含 | 是 |
| `doesNotContain` | 不包含 | 是 |
| `isEmpty` | 为空 | 否 |
| `isNotEmpty` | 不为空 | 否 |
| `isGreater` | 大于 | 是 |
| `isGreaterEqual` | 大于等于 | 是 |
| `isLess` | 小于 | 是 |
| `isLessEqual` | 小于等于 | 是 |

**各字段类型的 value 格式：**

| 字段类型 | value 类型 | 适用操作符 | 示例 |
|----------|-----------|-----------|------|
| `text` | string | is, isNot, contains, doesNotContain, isEmpty, isNotEmpty | `{"field_name":"姓名","operator":"contains","value":"张"}` |
| `number` | number | is, isNot, isGreater, isGreaterEqual, isLess, isLessEqual, isEmpty, isNotEmpty | `{"field_name":"金额","operator":"isGreater","value":0}` |
| `select` (`multiple=false`) | string（选项名） | is, isNot, isEmpty, isNotEmpty | `{"field_name":"状态","operator":"is","value":"已完成"}` |
| `select` (`multiple=true`) | string[]（选多个）/ string（选单个） | is, isNot, contains, doesNotContain, isEmpty, isNotEmpty | 多选传数组如 `["标签1","标签2"]`；单选传单个字符串 |
| `datetime` / `created_at` / `updated_at` | `["ExactDate", Unix 毫秒时间戳]` | is, isGreater, isLess, isEmpty, isNotEmpty | `{"field_name":"创建日期","operator":"isGreater","value":["ExactDate",1704038400000]}` |
| `checkbox` | boolean | is | `{"field_name":"已审核","operator":"is","value":true}` |
| `user` / `created_by` / `updated_by` | string 或 string[]（用户 ID，格式 `ou_xxx`）。不知道 `open_id` 时先用 `lark-cli contact +search-user --query "<姓名/邮箱/手机号>" --as user` 查 id。 | is, isNot, isEmpty, isNotEmpty | `{"field_name":"负责人","operator":"is","value":"ou_xxxxxxxxxxxxxxxx"}` |
| 所有类型（为空/不为空） | 不需要 value | isEmpty, isNotEmpty | `{"field_name":"备注","operator":"isEmpty"}` |

> `value` 类型因字段而异，可为 `string | number | boolean | string[] | ["ExactDate", number]`，需按上表构造。

### 日期筛选

图表 `data_config.filter` 筛选 `datetime` / `created_at` / `updated_at` 字段时：

- 有值条件只能使用 `is`、`isGreater` 或 `isLess`，不得使用 `isGreaterEqual` 或 `isLessEqual`。
- `value` 必须写成 `["ExactDate", <Unix 毫秒时间戳>]`，不得直接传裸时间戳。
- `isEmpty` / `isNotEmpty` 不传 `value`。

日期区间示例：

```json
{
  "filter": {
    "conjunction": "and",
    "conditions": [
      {
        "field_name": "派单日期",
        "operator": "isGreater",
        "value": ["ExactDate", 1785686400000]
      },
      {
        "field_name": "派单日期",
        "operator": "isLess",
        "value": ["ExactDate", 1786032000000]
      }
    ]
  }
}
```

## 约束与本地校验

- 必填与互斥
  - 普通图表类型必填：`table_name`；countdown 使用上文的专属 fixed/field 结构
  - slicer 类型必填：`table_name`、唯一 `group_by`；使用上方切片器专属契约
  - text 类型必填：`text`
  - button 类型不要求 `table_name`、`series`、`count_all`；按钮专属字段由服务端校验
  - 互斥：`series` 与 `count_all` 二选一，且至少提供其一（普通图表；切片器禁止这两个字段）
  - nps 类型必填：`table_name`、长度为 1 的 `group_by`；`group_by[0].mode` 可省略，省略时按 `integrated` 处理，显式传入时也只能为 `integrated`；不支持 `group_by[0].sort` 和 `series`；`count_all` 可省略，出现时只能为 `true`
  - pivotTable 类型必填：`table_name`；使用 `rows` / `columns` / `values`，不使用普通图表的 `group_by` / `series` / `count_all`
  - text 类型**不支持**：`series`、`count_all`、`group_by`、`filter`
- 长度/结构
  - 普通图表的 `group_by` 最多 2 个；每项 `field_name` 必填；切片器恰好一个
  - `group_by[].sort.type` 取值 `group|value|view`；`order` 取值 `asc|desc`
- 规范化（CLI 自动处理；`--no-validate` 时不生效，`data_config` 原样透传给后端）
  - `series[].rollup` 自动转成大写（如 `sum` → `SUM`）
  - `group_by[].sort.type/order` 自动转成小写
  - `group_by[].sort.type` 为 `group` 或 `view` 且缺少 `order` 时，自动补 `order:"asc"`；`value` 排序不会自动补方向
- 本地校验（可通过 `--no-validate` 跳过）
  - `+dashboard-block-create` 默认对 `data_config` 做轻量校验；失败会聚合错误并给出修复建议
  - `+dashboard-block-update` 不带 `--type`，所以一般不做按组件类型的强校验；检测到透视表专属字段时会校验行列、指标和排序引用。所有更新都会对可解析的 `filter` 条件做轻量校验，包括 `conjunction`、字段引用、`operator` 和必需的 `value`，并与 create 一样拦截非法 `number_format` 子字段（见下方 number_format 小节）
  - 仅需传入合法 JSON；CLI 不会擅自改写你的业务含义

## 可复制模板

**按意图选择模板：**
- 比较不同类别数值 → 柱状图 / 条形图
- 看趋势变化 → 折线图 / 面积图
- 看占比分布 → 饼图 / 环形图 / 词云
- 多指标对比 → 组合图
- 看两变量关系 → 散点图
- 看流程转化 → 漏斗图
- 看多维度评分 → 雷达图
- 显示单个指标 → 指标卡（统计数字或记录数）
- 统计满意度评分分布 → NPS 图（一个 Rating 字段 + 可选分段）
- 按行列维度交叉汇总多个指标 → 透视表
- 查看单维度 Top N → 排行榜
- 按单字段离散值筛选仪表盘 → 切片器，使用上方 slicer 模板
- 展示距离指定日期或日期字段目标的时间 → 倒计时（见 countdown 小节）

最小柱状图：

```json
{
  "table_name": "表名",
  "series": [{ "field_name": "数值字段", "rollup": "SUM" }],
  "group_by": [{ "field_name": "分组字段", "mode": "integrated" }]
}
```

最小饼图/环形图（按分类字段统计行数占比）：

```json
{
  "table_name": "表名",
  "count_all": true,
  "group_by": [{ "field_name": "分类字段", "mode": "integrated" }]
}
```

折线图（按月趋势）：

```json
{
  "table_name": "表名",
  "series": [{ "field_name": "金额", "rollup": "SUM" }],
  "group_by": [{ "field_name": "月份", "mode": "integrated", "sort": {"type":"group","order":"asc"} }]
}
```

条形图（横向柱状图）：

```json
{
  "table_name": "表名",
  "series": [{ "field_name": "数值字段", "rollup": "SUM" }],
  "group_by": [{ "field_name": "分组字段", "mode": "integrated" }]
}
```

面积图（趋势填充）：

```json
{
  "table_name": "表名",
  "series": [{ "field_name": "数值字段", "rollup": "SUM" }],
  "group_by": [{ "field_name": "时间字段", "mode": "integrated", "sort": {"type":"group","order":"asc"} }]
}
```

组合图（柱+线等多指标对比）：

```json
{
  "table_name": "表名",
  "series": [
    { "field_name": "指标1", "rollup": "SUM" },
    { "field_name": "指标2", "rollup": "SUM" }
  ],
  "group_by": [{ "field_name": "分类字段", "mode": "integrated" }]
}
```

散点图（两变量相关性）：

```json
{
  "table_name": "表名",
  "series": [{ "field_name": "Y轴字段（数值/指标）", "rollup": "SUM" }],
  "group_by": [{ "field_name": "X轴字段（分类/维度）", "mode": "integrated" }]
}
```

漏斗图（流程转化）：

先判断用户要看的数值语义：

- **当前数量**：统计每个当前状态/阶段下有多少记录，例如“各环节当前数量”“当前阶段分布”。源表有状态/阶段字段时，直接用 `count_all:true` + `group_by`。
- **累计数量**：统计到达该阶段及其后续阶段（后缀和）的累计数量，例如“流程转化”“从 A 到 B 各环节转化”。此口径假设流程单向、无跳阶/回退、记录不删除；不满足时须用状态变更历史，不能对当前快照累加。如果表中已有累计数量字段或阶段汇总表，直接用该字段画漏斗图；否则先计算累计数量，创建并写入 helper 汇总表后再画图。

当前数量：

```json
{
  "table_name": "表名",
  "count_all": true,
  "group_by": [{ "field_name": "状态字段", "mode": "integrated" }]
}
```

累计数量：

```json
{
  "table_name": "流程汇总表名",
  "series": [{ "field_name": "累计数量", "rollup": "SUM" }],
  "group_by": [{ "field_name": "阶段字段", "mode": "integrated", "sort": {"type":"view","order":"asc"} }]
}
```

如果只有当前状态数据但用户要看流程转化，需要先按业务阶段顺序计算每个阶段的累计数量，再创建 helper 汇总表（如：阶段、累计数量），用 `+record-batch-create` 一次写入后，按“累计数量”模板创建漏斗图。helper 表行序就是业务顺序时，首次创建 block 时一次性设置好 `group_by.sort`。

> ⚠️ 注意:helper 汇总表仅用于源表无法直接聚合出目标形态的场景（如上面的累计数量漏斗图）。只要能在源表上直接用 `group_by` + `rollup`（含 `AVERAGE`）算出，就不需要新建 helper 表。

词云（文本频率）：

```json
{
  "table_name": "表名",
  "count_all": true,
  "group_by": [{ "field_name": "文本字段", "mode": "integrated" }]
}
```

雷达图（多维度评分）：

```json
{
  "table_name": "表名",
  "series": [
    { "field_name": "维度1", "rollup": "SUM" },
    { "field_name": "维度2", "rollup": "SUM" },
    { "field_name": "维度3", "rollup": "SUM" }
  ],
  "group_by": [{ "field_name": "分类字段", "mode": "integrated" }]
}
```

排行榜（按销售额取 Top 10）：

```json
{
  "table_name": "订单表",
  "series": [{ "field_name": "金额", "rollup": "SUM" }],
  "group_by": [{ "field_name": "负责人", "mode": "integrated", "sort": {"type":"value","order":"desc"} }],
  "limit_size": 10
}
```

排行榜只更新 Top N：

```json
{"limit_size": 20}
```

指标卡（统计数字）：

```json
{
  "table_name": "数据表",
  "series": [{ "field_name": "数字", "rollup": "SUM" }]
}
```

NPS 图（按 Rating 评分字段统计记录数）：

```json
{
  "table_name": "问卷结果",
  "group_by": [{ "field_name": "满意度评分", "mode": "integrated" }],
  "category_range": [0, 6, 8, 10]
}
```

NPS 的 `group_by[0].field_name` 必须指向 Base 的评分字段（Dashboard 内部识别为 `Rating` 语义）。调用方可通过 Base 字段详情或界面字段配置确认评分字段的最小值与最大值；CLI 只能做轻量 JSON 校验，字段类型、字段量程、`category_range` 首尾是否等于评分字段最小值和最大值由服务端按字段元数据校验。

`category_range` 可省略，服务端会按 Rating 字段自身量程生成默认分段。显式传入时数组长度必须为 4，且首尾必须等于 Rating 字段最小值和最大值。

指标卡（统计记录数）：

```json
{
  "table_name": "数据表",
  "count_all": true
}
```

### statistics 指标卡数值格式 number_format（可选）

仅 `type: statistics` 支持在 `data_config` 里加可选 `number_format`，控制数值展示格式与精度；不传时服务端会补 `{"formatName":"digital"}`，`precision` 保持省略。其它组件类型不支持该字段：create 会被 CLI 直接拒绝（显式 `--no-validate` 可跳过），避免把后端严格 schema 错误延迟到请求阶段；update 不带 `--type`，由服务端结合组件现有类型裁决。

- `formatName`（string，可选）：必须精确匹配下表 5 个枚举之一，**区分大小写**（不同于 `series[].rollup` 会被自动转成大写，这里不做规范化，`DIGITAL` 会被拒绝）。
- `precision`（integer，可选）：小数位数，`0` 到 `9` 的整数；`2.5` 这类非整数会被本地拒绝。

| formatName | 含义 | 示例（precision=2） |
|------------|------|--------------------|
| `digital` | 千分位数字（不传 `number_format` 时的服务端默认值） | `1,234.56` |
| `digital_without_separator` | 无千分位数字 | `1234.56` |
| `percentage_rounded` | 百分比 | `1,234.56%` |
| `cyn_rounded` | 人民币金额 | `¥1,234.56` |
| `dollar_rounded` | 美元金额 | `$1,234.56` |

指标卡（金额，保留 2 位小数）：

```json
{
  "table_name": "订单表",
  "series": [{ "field_name": "金额", "rollup": "SUM" }],
  "number_format": { "formatName": "dollar_rounded", "precision": 2 }
}
```

> **更新时 `number_format` 按子字段合并**：例如现有 `{"formatName":"digital","precision":2}` 时只传 `{"number_format":{"precision":0}}`，服务端会保留 `formatName:"digital"` 并把精度改为 `0`。其它顶层 key 的更新策略见 [lark-base-dashboard.md](lark-base-dashboard.md)。

文本组件（Markdown 富文本）：

```json
{
  "text": "# 🚀 一级标题\n这是一个 **加粗** *斜体* ~~删除线~~ 的示例。\n\n## 📌 二级标题\n1. 有序列表项 1\n2. 有序列表项 2\n\n### 📌 三级标题\n- 无序列表项 1\n- 无序列表项 2"
}
```

> **注意**：text 类型组件不需要 `table_name`、`series`、`group_by`、`filter` 等数据源相关字段。

## 常见错误与修复

- 同时存在 `series` 与 `count_all`
  - 现象：后端/本地校验报互斥错误
  - 修复：见「关键约束」章节的二选一规则
- 缺少 `table_name`
  - 现象：本地校验缺少必填字段
  - 修复：指定数据源表名（使用表名，非表 ID）
- `series[].rollup` 大小写/取值不合法
  - 现象：本地校验提示枚举不支持
  - 修复：改为 `SUM|MAX|MIN|AVERAGE` 中之一（不区分大小写，CLI 会统一为大写；计数请使用 `count_all:true`）
- `group_by` 超出 2 个或字段名为空
  - 修复：保留前 2 个，或补齐 `field_name`
- 排序枚举不合法
  - 修复：`group_by.sort.type` 仅能为 `group|value|view`；`order` 为 `asc|desc`
- filter 写法不规范
  - 修复：`conjunction` 取 `and|or`；`conditions[].operator` 必须在本页表格列举的范围内；除 `isEmpty/isNotEmpty` 外需提供 `value`

## 坑点

- **`count_all` 与 `series` 二选一** — 两者不能同时使用
- **filter `value` 类型因字段而异** — 文本/单选为 string，数字为 number，日期为毫秒时间戳，多选/人员可为 string[]，复选框为 boolean；`isEmpty`/`isNotEmpty` 不需要 value
- **`data_config` 结构随 `type` 变化** — 不同组件类型的字段不同，创建前务必确认类型对应的字段
- **表名用 name，不是 ID** — `table_name` 对应的是表名称（如「订单表」），不是 `table_id`
