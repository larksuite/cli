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
- 文本、日期和数字字段均可使用下拉或平铺；本节单源协议不支持日期／数字范围组件或多字段映射，仍可只改名称、位置。多数据源切片器按[多数据源配置](#dashboard-多数据源配置)更新，不套用本节单源字段。BaseApp 组件不支持 `type=slicer`。
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

### 多表需求与聚合失败的恢复边界

用户明确要求把多张表的数据放到同一个图表（含漏斗图）时，先按下文多数据源协议构造 `data_sources`，逐源核对真实表、字段、聚合与阶段／维度。多源不是把表物理 JOIN；同一阶段能否汇总仍要核对实际结果。不要默认复制记录、新建辅助汇总表或把全部指标改从一张表读取来模拟多源。辅助表只用于源表及已支持的多源聚合确实无法表达的派生口径；不能用它冒充用户点名的来源。

遇到 `SUM not supported by field` 等拒绝时，核对原字段及公式／查找引用的实际返回类型、单位和目标环境支持的聚合，不因外层是公式就认定非数值，也不因样例像数字就认定可求和。仅按错误定位做有界修正并回读；不要使用 `--no-validate` 绕过服务端限制，不把公式改成手填常量。只有用户要记录数时才可改为 `count_all`，金额不能改成计数。无法保留指定来源与指标口径时，报告受阻的表、字段、聚合及错误，不悄悄删去数据源或宣称完成。进度图不使用普通图表的 `data_sources`，其两侧来源遵循进度图协议。

## Dashboard 多数据源配置

同一 Base 内支持 `column/line/bar/scatter/combo/area/pie/ring/funnel/wordCloud/statistics/ranking/radar/slicer`。使用 `data_sources`（至少 2 个不重复的数据表，无额外数量上限），不可混用顶层单源字段。

CLI 在多源创建和更新时均将来源内 `series[].rollup` 转为大写；更新不补创建默认值。创建本地拒绝相同 Base 写法与表名的重复来源、切片器来源携带 `series/count_all`；省略 Base 与显式 Base 是否指向同一表仍由服务端核对。字段存在性、权限、跨源兼容性及组件适用范围以服务端校验为准。`backgroundOpacity` 本地校验 0–1；`--no-validate` 跳过这些本地语义检查和归一化，但不绕过服务端校验。

### 字段结构

| 字段 | 类型 | 创建 | 更新 |
|------|------|------|------|
| `data_sources` | object[] | 至少 2 个不重复的数据表 | 已有多源：至少 1 项来源补丁；单源转多源：至少 2 个来源的完整目标配置 |
| `data_sources[].table_name` | string | 必填，当前 Base 的表名 | 可按唯一表名定位已有来源；不用于换表 |
| `data_sources[].base_token` | string | 可选，省略继承外层 `--base-token` | 必须属于同一 Base |
| `data_sources[].source_id` | string | 建议省略 | 使用 Get 返回的来源身份 |
| `data_sources[].series` / `count_all` | array / boolean | 图表可均省略，默认记录数；不能同时提供 | 提供时切换统计方式；省略保留 |
| `data_sources[].group_by` | object[] | 每源最多一个维度；必填规则见组件表 | 提供时整体替换；省略保留 |
| `data_sources[].filter` | object | 可选筛选条件 | 对象整体替换，`null` 清空，省略保留 |
| `data_sources[].alias` | string | 可选来源系列别名 | 省略保留，`null` 清除 |
| `multi_source` | object | 可选组件级配置 | 按字段更新，不替换来源数组 |

每个来源只必填 `table_name`；省略 `base_token` 时继承命令外层 `--base-token`，不做全局表名反查，显式跨 Base 不支持。图表的 `series` 和 `count_all:true` 互斥，但创建时均可省略：服务端默认统计记录总数。需要字段求和等才传 `series`，每源最多一个指标，各来源指标模式、rollup、分组数量须一致。

普通图表创建时省略 `group_by`，服务端为每张表独立选择默认类别字段，不生成空分组。柱状图优先单选，其次文本、人员、日期、多选、公式、引用；折线／面积／散点优先日期。需要确定横轴时显式提供 `group_by`，每源最多一个维度；无可用默认字段时必须显式配置。更新省略分组则保留原值，不重新选字段。

`multi_source.series_split` 默认 false（汇总），通常无需传；true 按来源拆分系列。它不是多数据源开关，传 `data_sources` 即启用多源，不需要提交内部 `enableMultiDataSource`。

### 各组件的多源能力

| 组件 | 来源与组件级配置 |
|---|---|
| `column/line/bar/scatter/combo/area/pie/ring/funnel/wordCloud` | 每源记录数或唯一指标、分组和筛选；`multi_source.series_split` 控制汇总／拆系列 |
| `statistics` | 每源记录数或唯一指标；不允许分组或排序；格式用 `multi_source.number_format` 或原生格式，不能混用 |
| `ranking` | 每源恰好一个分组；`multi_source.limit_size` 为 1–500；排序只支持 `value` |
| `radar` | 每源必须一个分组，使用按记录聚合 |
| `slicer` | 每源恰好一个分组，禁止 `series/count_all`；展示和选择模式放在 `multi_source`，不放顶层 |

统一排序用 `multi_source.sort={type:group|value,order:asc|desc}`；不提供时各来源分组排序须一致。`alias` 是来源系列别名，更新时 `alias:null` 清除，不通过改数组顺序改变身份。多源切片器的 `multi_source.displayMode/selectMode/order` 分别为 `dropdown|tiled`、`single|multiple`、`asc|desc`，创建缺省为 `tiled/multiple/asc`；其他组件不接受这三项。

### 创建与更新规则

创建时建议省略 `source_id`，Get 返回每源的 `source_id/base_token/table_name` 和生效配置。后续优先使用返回的 `source_id`，也可用唯一 `table_name`；两者同时提供必须定位同一来源。来源身份不按数组下标推测。

单源转多源使用 `+dashboard-block-update`，向同一组件提交至少两个来源的完整 `data_sources`，沿用创建时的字段和默认值。先 Get 读取原配置，需要保留原统计口径时，将原表的指标、分组和筛选显式放入对应来源；不要将原扁平字段与 `data_sources` 混传。转换保留组件 ID、名称、布局和未修改的样式，不需要删除或重建。只传 `multi_source` 不能开启多源。

已有多源更新 `data_sources` 是 **至少 1 项已有来源的补丁数组，不是整体替换**，无额外数量上限：

- 未列出的来源和未传字段保持不变；不能通过省略来源删除，通过顺序重排，或隐式新增来源／换表。多源转回单源不支持。
- `series/group_by` 数组整体替换；显式传 `series` 或 `count_all:true` 切换该来源指标方式，均省略则保留原指标，不能套用创建默认计数规则。
- `filter` 对象替换全部条件；`filter:null` 清空；省略保留。表与字段须属于对应来源，各来源仍须符合指标、rollup 和维度一致性约束。
- `multi_source` 按字段更新；仅传 `{"multi_source":{"series_split":true}}`、排行榜的 `limit_size` 或切片器选择模式均不改其他来源。指标卡 `number_format` 按子字段合并。
- 切片器展示更新保留联动目标和已选条件；显式更新来源配置或改变单／多选模式会清空已选条件。
- 旧快照自定义来源 ID 可能在更新后归一为 `src_N`；继续操作时使用最新响应身份。

### 可复制模板

柱状图最小配置：

```json
{"data_sources":[{"table_name":"销售表"},{"table_name":"成本表"}]}
```

```bash
lark-cli base +dashboard-block-create \
  --base-token '<BASE_TOKEN>' --dashboard-id '<DASHBOARD_ID>' \
  --name '多数据源柱状图' --type column --as user \
  --data-config '{"data_sources":[{"table_name":"数据表"},{"table_name":"数据表 副本"}]}'

lark-cli base +dashboard-block-get \
  --base-token '<BASE_TOKEN>' --dashboard-id '<DASHBOARD_ID>' \
  --block-id '<BLOCK_ID>' --as user

# 仅改副本横轴，其余来源、指标和筛选保留
lark-cli base +dashboard-block-update \
  --base-token '<BASE_TOKEN>' --dashboard-id '<DASHBOARD_ID>' \
  --block-id '<BLOCK_ID>' --as user \
  --data-config '{"data_sources":[{"table_name":"数据表 副本","group_by":[{"field_name":"文本","mode":"integrated"}]}]}'

# 单源转多源：提交完整目标，保留原表统计口径须显式填写
lark-cli base +dashboard-block-update \
  --base-token '<BASE_TOKEN>' --dashboard-id '<DASHBOARD_ID>' \
  --block-id '<SINGLE_SOURCE_BLOCK_ID>' --as user \
  --data-config '{"data_sources":[{"table_name":"销售表","series":[{"field_name":"销售额","rollup":"SUM"}],"group_by":[{"field_name":"月份"}]},{"table_name":"预算表","series":[{"field_name":"预算额","rollup":"SUM"}],"group_by":[{"field_name":"月份"}]}],"multi_source":{"series_split":true}}'
```

### 使用限制与错误处理

来源配置或回读失败时，按错误消息中的来源身份定位数据表，检查表名、字段和访问权限。错误未明确来源时，不推断某张表异常。

`+dashboard-block-get-data` 读取计算结果，不提供独立的每源计算状态、结果归属或下钻接口。多源配置不支持跨 Base、组件复制、来源增删／重排，以及跨组件大类切换。同大类内的堆积、平滑等子形态使用下文的 `viewModel.chartKind`，不修改来源配置。

`--linkage` 使用更新请求外层的同源目标列表，不属于 `data_config`，不支持多源字段映射。不要把字段映射塞进数据配置。修改多源配置时保留已有合法联动。

创建请求超时或返回网络错误时，先查询组件列表确认是否已创建，再决定是否重试，避免重复创建。批量创建同一仪表盘的组件须串行执行。

## 自定义样式

通过现有 `--data-config` 增加 `viewModel`、`overrideViewModel`，不使用 `--style` 或 `data_config.style`。字段结构与 Web 一致，服务端按组件和公开白名单校验；不要直接提交整个 Web Snapshot。

透视表可在同一次更新中提交数据和样式，例如 `{"rows":[],"viewModel":{"rules":{"theme":{"fontColor":"#3366FF"}}}}` 同时清空行维度并设置字体颜色，无需拆成两次请求。

### 字段结构

| 字段 | 类型 | 创建 | 更新 |
|------|------|------|------|
| `viewModel` | object | 可选，与组件数据配置一起提交 | 可单独提交样式补丁 |
| `viewModel.chartKind` | number | 当前组件大类内的子形态 | 不能用于跨组件类型切换 |
| `viewModel.rules` | object | 主题、系列、坐标轴及组件专用配置 | 未传字段保留，公开叶字段可用 `null` 清除 |
| `overrideViewModel` | object | 仅组合图，配置分类形态与值轴 | 可单独更新组合图样式 |

### 配置入口与组件适用范围

| 能力 | 原生配置入口 |
|---|---|
| 主题配色、单色、默认字体颜色 | `viewModel.rules.theme.colorTheme/singleColorTheme/fontColor`；整体字体改色须同时提交下文的组件字体规则 |
| 组件标题文字 | 创建或更新的 `--name` |
| 图表组件标题颜色、加粗、不透明度 | `name_style.color/bold/opacity`，图表透明度取 0–1，写入 Dashboard 组件元数据 |
| 背景颜色、不透明度、模糊度、字体 | `viewModel.rules.plotArea.background/backgroundOpacity/backgroundBlur/font` |
| 图例显隐、定位、背景和字体 | `viewModel.rules.legend.legend`；Web 图例文字颜色用 `legend.colorVisualMap.discreteLegendSpec.labelStyle.color`，主题覆盖在 `theme.override.legends.discreteLegend` |
| 全系列颜色、透明度、线宽、标记大小和形状 | `viewModel.rules.series.all`，使用 `fill/fillOpacity/strokeWidth/size/shape` 等 Web 字段 |
| 数据标签显隐、内容、字体、位置、格式 | `viewModel.rules.series.allDataLabel`；内容用 `detail.keys`，格式用 `formatConfig.formatCode` |
| 坐标轴标题、刻度、范围、标签格式和网格线 | `viewModel.rules.xyAxes.detail.index0/value0/value1`，仅适用有对应坐标轴的组件 |
| 按已保存系列配置 mark 和标签 | `viewModel.rules.series.rules[系列 key].spec/dataLabel` |
| 同大类子形态 | `viewModel.chartKind`，使用 Web 数值；服务端拒绝跨大类值 |
| 组合图分类形态与系列值轴 | `overrideViewModel.cartesian.combo.baseType/detail`、`seriesYAxis`，仅适用组合图 |
| 指标卡格式、描述和专用字号 | `viewModel.rules.statistics.formatInfo/description/customFontSizes` |
| 进度形状、颜色和格式 | `viewModel.rules.progress`、`viewModel.rules.formatInfo`，完整示例见[进度图协议](lark-base-dashboard-progress.md#形状格式与颜色) |

饼图、雷达图、面积图、漏斗图、词云、排行榜、NPS、倒计时和透视表有对应组件专用规则；不得给其他类型塞入这些节点。支持 Web 子形态不等于新增通用 `stack/subtype/axis/palette` 字段，不能根据常见图表库字段名猜协议。切片器单／多选属于业务配置：单源用 `selectMode`，多源用 `multi_source.selectMode`，不是原生样式字段。

### 同大类子形态切换

`type` 是创建时的组件大类；已有组件切换子形态使用 `+dashboard-block-update --data-config` 中的数值 `viewModel.chartKind`，不传 `--type`，不删除重建。折线平滑还对应 `viewModel.rules.series.all.interpolate:"monotone"`，不是顶层 `smooth` 或 `series[].smooth`。下表为可直接使用的补丁：

| 当前大类 | 目标子形态 | `--data-config` |
|---|---|---|
| `bar` | 百分比堆积条形图 | `{"viewModel":{"chartKind":1116418}}` |
| `line` | 普通直线 | `{"viewModel":{"chartKind":1061124,"rules":{"series":{"all":{"interpolate":"linear"}}}}}` |
| `line` | 平滑折线 | `{"viewModel":{"chartKind":1069828,"rules":{"series":{"all":{"interpolate":"monotone"}}}}}` |
| `area` | 普通面积图 | `{"viewModel":{"chartKind":1052936,"rules":{"series":{"all":{"interpolate":"linear"}}}}}` |
| `area` | 平滑面积图 | `{"viewModel":{"chartKind":1069832,"rules":{"series":{"all":{"interpolate":"monotone"}}}}}` |
| `area` | 平滑堆积面积图 | `{"viewModel":{"chartKind":1070344,"rules":{"series":{"all":{"interpolate":"monotone"}}}}}` |
| `radar` | 普通雷达图 | `{"viewModel":{"chartKind":1441793}}` |
| `radar` | 面积雷达图 | `{"viewModel":{"chartKind":1441796}}` |

服务端切换 `chartKind` 时同步目标子形态的默认系列规则，例如平滑插值、雷达面积填充，并保留配色和标签等无关样式。`interpolate` 只改变插值，不代替用于堆积、百分比或填充模式的 `chartKind`。其他子形态必须使用已确认的 Web 数值，不枚举猜测整数或提交字符串类型名。

Web `operations` 是路径变更记录，不是 CLI 入参。将 `oi` 写到对应的 `data_config` 路径即可，不提交 `operations`、`od` 或 `valid`：

```bash
lark-cli base +dashboard-block-update --as user \
  --base-token '<BASE_TOKEN>' --dashboard-id '<DASHBOARD_ID>' \
  --block-id '<LINE_BLOCK_ID>' \
  --data-config '{"viewModel":{"chartKind":1069828,"rules":{"series":{"all":{"interpolate":"monotone"}}}}}'
```

更新前 Get 确认当前大类；更新后独立 Get 核对 `data_config.viewModel.chartKind` 及已保存规则。`--dry-run` 不写入；退出码为 0 不能代替字段回读与渲染确认。

### 创建与更新规则

创建时将这些原生节点与该组件原有数据配置放在同一个 `data_config`；更新允许只传样式。原生节点逐叶合并，未传项保持原值，数组替换；`null` 清除公开覆盖，空对象不清空。根 `viewModel`、`overrideViewModel`、`viewModel.rules` 不能整体清空。旧数据协议的合并语义不因此改变。

- 进度形状：`bar|semiRing|circle`；精度为 0–9 的整数，普通精度和百分比精度分开；透明度 0–1。
- `chartKind` 使用 Web 原生数值，仅支持当前组件大类内子形态，不支持跨组件切换。
- 指标卡旧 `number_format` 与原生 `viewModel.rules.statistics.formatInfo` 不能混用。
- 公共范围包括主题、绘图区、图例、标签、系列和坐标轴，以及组件专用样式子集。Tooltip、Annotation 不支持。
- Get 回读公开样式覆盖，省略默认 seed 和内部状态，并非完整有效样式。
- `+dashboard-block-get` 可为单源和多源组件返回 `series_descriptors`，包括单源组合图按分组选项拆分的系列；不需要额外查询参数，也不应为获取系列描述先改成多源。每项的 Web 稳定系列 `key` 和可配置的 `storage_paths` 是定向修改的依据，不能按数据源数量推断是否可查询。以本次响应为准：字段缺失、为空或未包含目标系列时，不代表目标不存在或修改成功。查询只读；数据源、指标或分组变化后重新 Get，不沿用旧 key。

### 可复制模板

#### 组件标题与背景

标题文字用 `--name`，标题样式用 `name_style`，组件背景用 `viewModel.rules.plotArea`。三者可在一次更新中提交；不是 `viewModel.title` 或 `overrideViewModel.background`：

```bash
lark-cli base +dashboard-block-update \
  --base-token '<BASE_TOKEN>' --dashboard-id '<DASHBOARD_ID>' --block-id '<BLOCK_ID>' \
  --name '2026 年月销售额' --as user \
  --data-config '{"name_style":{"color":"#1D4ED8","bold":true,"opacity":1},"viewModel":{"rules":{"plotArea":{"background":"#EAF3FF","backgroundOpacity":0.85,"backgroundBlur":0}}}}'
```

#### 背景不透明度与模糊度的单位

`viewModel.rules.plotArea.backgroundOpacity`（不透明度）与 `backgroundBlur`（背景模糊度）都使用 **0–1 的数字比例**，不是整数百分数或像素值。用户说的百分数需先除以 100：0% → `0`，20% → `0.2`，50% → `0.5`，100% → `1`。模糊度 `0` 表示关闭，不能用 `false` 代替。

例如“不透明度 90%、背景模糊度 20%”，只更新这两个字段，保留背景颜色、标题与其他配置：

```bash
lark-cli base +dashboard-block-update --as user \
  --base-token '<BASE_TOKEN>' --dashboard-id '<DASHBOARD_ID>' --block-id '<BLOCK_ID>' \
  --data-config '{"viewModel":{"rules":{"plotArea":{"backgroundOpacity":0.9,"backgroundBlur":0.2}}}}'
```

只要求调整模糊度时仅提交 `backgroundBlur`。不要提交 `20`、`"20%"` 或 `false`：Web 会将数字比例乘以 100 显示，`20` 会显示为 **2000%**，不是 20%。这里的单位规则仅针对上述 `plotArea` 字段，不推断其他同名样式字段。

更新后独立 Get，核对 `data_config.viewModel.rules.plotArea.backgroundOpacity` 为 `0.9`、`backgroundBlur` 为 `0.2`；有 Web 验证条件时再确认界面显示 90% 与 20%。只收到更新成功、或回读到错误值 `20`，都不能报告“模糊度 20% 已生效”；未验证渲染时说明仅完成配置回读。

#### 图例与数值标签

柱、条、折线、面积和组合图的图例显隐使用 `viewModel.rules.legend.legend.visible`（双层 `legend`）；全系列数值标签使用 `viewModel.rules.series.allDataLabel.visible`。两个开关相互独立，布尔值 `true` 开启、`false` 关闭。只修改用户点名的开关，不连带修改另一项。

仅开启图例的 `--data-config`：

```json
{"viewModel":{"rules":{"legend":{"legend":{"visible":true}}}}}
```

例如面积图同时显示图例和每个点的数值标签：

```bash
lark-cli base +dashboard-block-update --as user \
  --base-token '<BASE_TOKEN>' --dashboard-id '<DASHBOARD_ID>' --block-id '<BLOCK_ID>' \
  --data-config '{"viewModel":{"rules":{"legend":{"legend":{"visible":true}},"series":{"allDataLabel":{"visible":true,"detail":{"keys":["value"]}}}}}}'
```

创建时将相同的 `viewModel` 与该图表的数据配置一起放入 `--data-config`。同时隐藏图例和数值标签：

```bash
lark-cli base +dashboard-block-update --as user \
  --base-token '<BASE_TOKEN>' --dashboard-id '<DASHBOARD_ID>' --block-id '<BLOCK_ID>' \
  --data-config '{"viewModel":{"rules":{"legend":{"legend":{"visible":false}},"series":{"allDataLabel":{"visible":false}}}}}'
```

不要使用单层 `rules.legend.visible`、`legend.show/isShow/enabled`、顶层 `showLegend/showDataLabel` 或 `series[].label`。缺省字段、`legend:{}` 或 `legend:null` 不等于关闭；空对象不清空，`null` 清除覆盖后也不能据此断言隐藏。

执行后用独立 Get 核对这两个完整路径及其布尔值；字段缺失不能仅凭“多系列默认显示图例”宣称已显式开启。若已知路径被拒绝或提交值未在回读中体现，先核对组件适用类型和具体错误，报告未验证项；不要循环枚举字段名或靠 `--no-validate` 试探。以上补丁不修改指标、维度、组合图形态或系列配色；其他图表的专用标签使用各自规则，不套用 XY 标签节点。

#### 组件字体颜色与字号

“组件字体”是图表内容的文字，不是 Dashboard 组件标题，也不是折线／柱／扇区等图形配色。按下表先识别组件类型，再构造样式补丁；不能只写 `theme.fontColor`，也不能把所有组件的专用节点拼成一个通用补丁。

以下路径均相对 `data_config.viewModel.rules`；颜色使用字符串，字号使用数值像素（如 `14`，不是 `"14px"`）。颜色和字号分别更新：只改颜色不带字号，只改字号不带颜色。

##### 普通图表的基础映射

| 作用位置 | 颜色字段 | 字号字段 |
|---|---|---|
| 组件字体面板（下文简称 C／S） | C：`theme.fontColor` 与 `plotArea.font.color` | S：`plotArea.font.fontSize`；不存在 `theme.fontSize` |
| 图例文字（L） | `legend.colorVisualMap.discreteLegendSpec.labelStyle.color` | 同一 `labelStyle.fontSize` |
| XY 全系列数值标签（D） | `series.allDataLabel.color` | `series.allDataLabel.fontSize` |
| 坐标轴刻度文字（A） | `xyAxes.detail.<axis>.label.labelColor` | `xyAxes.detail.<axis>.label.labelFontSize` |
| 坐标轴标题文字（A） | `xyAxes.detail.<axis>.title.titleColor` | `xyAxes.detail.<axis>.title.titleFontSize` |

A 只更新当前图中实际存在的轴：`index0`、`value0`，以及已存在的次值轴 `value1`；雷达图没有 `value1`。不要为改字体启用次轴、轴标题或标签。轴标题字号是公开协议的 `titleFontSize`，不要照抄 Web 内部操作里的 `title.labelFontSize`。

C／S 是普通字体面板的入口，不代表所有文字会自动继承；整体修改时还需按组件类型补齐下表的渲染字段。图例颜色不在 `legend.legend.color`，图例字号也不在 `legend.fontSize`；显隐开关仍使用上一节的 `legend.legend.visible`，改字体时不动开关。

##### 各组件的适用字段与边界

| 组件类型 | 修改组件字体颜色 | 修改组件字号 |
|---|---|---|
| `column/bar/line/area/scatter/combo` | C + L + D + A；已有系列标签独立覆盖按下文处理 | S + L + D + A；已有系列标签独立覆盖按下文处理 |
| `radar` | C + L + D，补齐实际角度轴／半径轴的 A | S + L + D，补齐实际轴的 A；不能提交 `value1` |
| `pie/ring` | C + L + `pie.allDataLabel.color` | S + L + `pie.allDataLabel.fontSize`；不要塞入 `xyAxes` |
| `funnel` | C + L + `funnel.seriesRules.baseSeries` 下的 `mainDataLabel.color`、`transformDataLabel.color`、`totalTransformRateMark.color` | S + L + 同三个节点的 `fontSize`；固定 `baseSeries`，不猜 XY 系列 key |
| `statistics` | `theme.fontColor`，对应指标卡正文颜色；不必套普通图表 C／D／A | `statistics.customFontSizes.number/title/compareDesc/compareNumber` 分别控制主数值、内部描述、对比描述、对比数值；只更新用户要求的部分 |
| `ranking` | `theme.fontColor` 为排行榜正文渲染入口；`plotArea.font.color` 对应通用面板值，可一同更新 | 当前 Web 不提供统一固定字号，公开 `ranking` 规则也无字号字段；不能仅写 S 就宣称排行榜字号已变 |
| `nps` | `nps.font.color`，不是只写 C；保留 `opacity/blur` | 当前公开 `nps.font` 只有颜色／透明度／模糊度，没有 `fontSize`，不能承诺固定字号 |
| `progress` | `progress.percentFontColor/curFontColor/targetFontColor/rangeFontColor` 分别控制百分比、当前值、目标值、日期范围文字；按当前模式及用户要求选字段 | 当前公开 `progress` 规则无固定字号字段；不要猜 `progress.fontSize` 或用 `formatInfo.precision` 代替字号 |
| `countdown` | `countdown.text.color` 控制数字文字，`countdown.desc.color` 控制描述；`countdown.block.color` 是数字块背景，不是字体 | 数字与描述随布局自适应，当前公开规则无固定字号字段；不套 S |
| `wordCloud` | 没有已确认的通用组件字体面板映射；词语配色属于 `wordClouds.colorTheme`，仅用户要求修改词语配色时使用，不冒充普通文字样式 | 不承诺整体固定字号；`fillingInitialFontSize/fillingDeltaFontSize` 是填充词布局参数，不是所有词语的字号 |
| `pivotTable/slicer` | 现有公开样式未确认表格／筛选项正文统一改色的映射，不把 C 当作已支持的正文改色 | 未确认正文统一字号映射；仅保存 S 不足以证明正文变化，报告能力缺口，不枚举字段 |
| `view/text/button/image/ai_chart` 等 | 不套用原生图表字体规则；使用各组件已公开的专属协议，缺少入口时说明不支持本项 | 同左，不因其他图表支持字号而扩展到这些组件 |

指标卡的 `customFontSizes.title` 是指标卡内部文字层，不是 Dashboard 组件外部标题。恢复指标卡自适应字号时清除对应公开覆盖，不为改单个数值字号同时重写其他层。普通图表字体操作也不提交 `name_style` 或 `plotArea.title`；Dashboard 标题样式单独使用 `name_style`，服务端会拒绝多数图表的内部 `plotArea.title`。

##### 已有文字覆盖与保留规则

Web 的整体字体操作还会更新已经保存的逐系列 `dataLabel` 覆盖；否则已有单系列颜色／字号可能盖住全系列设置。先 Get 查看公开的 `series.rules`，通过 `series_descriptors` 的身份和可写路径定位：只给已经覆盖该属性的 `dataLabel.color` 或 `dataLabel.fontSize` 同步目标值，不生成不存在的系列覆盖，不修改 `spec.fill/stroke/size`。无法公开定位的目标应报告未验证，不猜 key。

已有 `theme.override.legends.discreteLegend.labelStyle` 或组件专用文字覆盖也需对照回读检查；若其覆盖了本次要改的颜色／字号，只修改相同文字属性，保留背景、透明度、字重和显隐。不要用整节点 `null` 清掉无关样式。

##### XY 示例：深灰色、14px，不动标题和折线颜色

适用于实际只有 `index0/value0` 两根轴的柱、条、折线、面积、散点和组合图；已有 `value1` 或逐系列文字覆盖时按上文补齐。补丁不包含 `name_style`、图形配色或任何 `visible`，不会主动开启／隐藏文字：

```json
{"viewModel":{"rules":{"theme":{"fontColor":"#333333"},"plotArea":{"font":{"color":"#333333","fontSize":14}},"legend":{"colorVisualMap":{"discreteLegendSpec":{"labelStyle":{"color":"#333333","fontSize":14}}}},"series":{"allDataLabel":{"color":"#333333","fontSize":14}},"xyAxes":{"detail":{"index0":{"label":{"labelColor":"#333333","labelFontSize":14},"title":{"titleColor":"#333333","titleFontSize":14}},"value0":{"label":{"labelColor":"#333333","labelFontSize":14},"title":{"titleColor":"#333333","titleFontSize":14}}}}}}}
```

##### 非 XY 示例

饼图／环形图的内容文字改为红色、14px；若只改其中一项，删除另一项属性：

```json
{"viewModel":{"rules":{"theme":{"fontColor":"#FF0000"},"plotArea":{"font":{"color":"#FF0000","fontSize":14}},"legend":{"colorVisualMap":{"discreteLegendSpec":{"labelStyle":{"color":"#FF0000","fontSize":14}}}},"pie":{"allDataLabel":{"color":"#FF0000","fontSize":14}}}}}
```

漏斗图的主数值、阶段转化率和总转化率改为红色、14px：

```json
{"viewModel":{"rules":{"theme":{"fontColor":"#FF0000"},"plotArea":{"font":{"color":"#FF0000","fontSize":14}},"legend":{"colorVisualMap":{"discreteLegendSpec":{"labelStyle":{"color":"#FF0000","fontSize":14}}}},"funnel":{"seriesRules":{"baseSeries":{"mainDataLabel":{"color":"#FF0000","fontSize":14},"transformDataLabel":{"color":"#FF0000","fontSize":14},"totalTransformRateMark":{"color":"#FF0000","fontSize":14}}}}}}}
```

指标卡正文改红色，仅主数值字号改为 26，不修改描述和对比文字字号：

```json
{"viewModel":{"rules":{"theme":{"fontColor":"#FF0000"},"statistics":{"customFontSizes":{"number":26}}}}}
```

NPS 只改文字颜色，不提交不存在的字号字段：

```json
{"viewModel":{"rules":{"nps":{"font":{"color":"#FF0000"}}}}}
```

##### 执行与验收

使用 `+dashboard-block-update --data-config` 提交适用的纯样式补丁，省略数据源、指标、维度、图表子形态、标题和未要求的样式。创建时可与该组件的数据配置一起提交。

独立 Get 核对所有本次写入的属性，并比较图形配色、标题及显隐是否保持。面板值、逐位置渲染规则和 Web 实际显示是不同证据：`theme.fontColor` 或 S 回读成功不等于整组件生效，接口成功也不能证明未开放的字号已支持。没有 Web 渲染证据时说明“配置已回读，渲染待验”，不能宣称视觉验收通过。目标属性缺失或被拒绝时停止并报告，不能通过加标题样式、改图形颜色或盲猜字段代替组件字体修改。

#### 其他样式

进度图仅修改形状和格式，不修改数据源：

```bash
lark-cli base +dashboard-block-update \
  --base-token '<BASE_TOKEN>' --dashboard-id '<DASHBOARD_ID>' \
  --block-id '<BLOCK_ID>' --as user \
  --data-config '{"viewModel":{"rules":{"progress":{"shape":"semiRing"},"formatInfo":{"precision":2,"percentPrecision":1}}}}'
```

公共配色更新（适用图表的全系列 mark）：

```json
{"viewModel":{"rules":{"series":{"all":{"fill":"#3366FF","fillOpacity":0.8}}}}}
```

数据与样式一起创建：

多源配置与样式节点可放在同一个 `data_config`，不必先创建再补开关或样式：

```bash
lark-cli base +dashboard-block-create \
  --base-token '<BASE_TOKEN>' --dashboard-id '<DASHBOARD_ID>' \
  --name '多表柱状图' --type column --as user \
  --data-config '{"data_sources":[{"table_name":"销售表"},{"table_name":"成本表"}],"viewModel":{"rules":{"theme":{"colorTheme":["#3366FF","#34C759"]},"series":{"allDataLabel":{"visible":true,"detail":{"keys":["value"]}}}}}}'
```

指标卡百分比格式的纯样式补丁：

```json
{"viewModel":{"rules":{"statistics":{"formatInfo":{"formatName":"percentage_rounded","precision":2}}}}}
```

组合图默认分类形态可使用 `{"overrideViewModel":{"cartesian":{"combo":{"baseType":"line"}}}}`。单源和多源组合图按系列调整 `combo.detail`、`seriesYAxis` 或 `series.rules` 前，先用普通 `+dashboard-block-get` 读取 `series_descriptors`，根据返回的来源、指标或分组选项身份定位目标项，并只按该项的 `storage_paths` 和 `key` 提交对应配置。先完成数据配置更新，再重新查询系列描述，不在一次请求中猜测尚未生成的 key。

`overrideViewModel.cartesian.combo.detail` 是“稳定系列 key → 形态字符串”的映射，值仅支持 `"column" | "line" | "area" | "scatter"`（柱、线、面积、散点）。例如 `{"<查询返回的系列key>":"line"}`；不能写成 `{"<系列key>":{"type":"line"}}`，也不使用数字、`bar` 或数组。单项 `null` 仅表示清除该系列的公开覆盖，不表示设置某种形态。不要把形态映射与 `viewModel.rules.series.rules.<key>` 的样式对象结构混用。

例如用户要求“销售额为柱、预算额为线”，在 Get 已返回并确认两项 key 均允许写入 `overrideViewModel.cartesian.combo.detail` 后，将下面两个占位 key 替换为原样返回的真实 key，再执行：

```bash
lark-cli base +dashboard-block-update --as user \
  --base-token '<BASE_TOKEN>' --dashboard-id '<DASHBOARD_ID>' --block-id '<BLOCK_ID>' \
  --data-config '{"overrideViewModel":{"cartesian":{"combo":{"detail":{"<GET返回的销售额key>":"column","<GET返回的预算额key>":"line"}}}}}'

lark-cli base +dashboard-block-get --as user \
  --base-token '<BASE_TOKEN>' --dashboard-id '<DASHBOARD_ID>' --block-id '<BLOCK_ID>'
```

两项均显式配置，避免仅依赖 `baseType` 推断销售额必为柱；若用户只要求改一个系列，则只提交那个 key，保留其他覆盖和默认形态。回读应核对每个目标 `combo.detail.<key>` 的字符串值；计算数值正确或 Update 成功不等于形态已验证。已知路径被拒绝、目标 key 未返回或回读不一致时报告具体阻塞，不枚举对象／数字等写法试探，也不创建辅助表或切换数据源绕过。

例如“订单阶段为横轴，按产品线分组，基础版折线、专业版柱、企业版折线”，应保留原单源组件，按以下顺序操作：

1. 确认横轴为订单阶段、系列分组为产品线；需要修改数据配置时先完成该更新，不把“分组请求成功”等同于组合图分组映射正确。
2. 重新 Get，从 `series_descriptors` 逐一定位基础版、专业版、企业版对应项；不得拿旧分组的 key 或按来源拆分的系列冒充分组选项。
3. 按各项返回的可写路径分别设置 `line/column/line`；其他系列、数据源、名称和布局不随之修改。
4. 独立 Get 核对横轴、组合图分组字段／选项及三个系列的形态。仅有 `baseType:line`、更新成功或计算结果有三组数值，不能证明逐选项形态要求已完成。

`source_id`、字段 ID、选项 ID 和显示名称都不是可直接使用的系列 key；不得据此拼接 `dmk-*`，也不用数组下标。`series_descriptors` 缺失、为空、目标项身份不明确或缺少所需可写路径时，停止定向写入，保留组件并报告具体缺失项；不能靠 `--no-validate`、删图重建、复制源表、增加辅助字段、把单源改为多源或统一默认形态来绕过当前任务的系列定位问题。默认形态和全系列样式仍可设置，但不能据此声称指定系列已分别设置成功。

### 使用限制

图表组件标题使用 `name_style`，不是 `plotArea.title.spec`；`bold` 为布尔值，不是 `fontWeight`。图表 `name_style.opacity` 为 0–1；既有视图组件协议按本文视图章节使用。Dashboard 不支持自定义组件外边框，不提交 `plotArea.border`；系列 `strokeWidth` 是图形描边，不是组件外边框。背景模糊使用 0–1 数字比例，20% 写 `backgroundBlur:0.2`，关闭写 `backgroundBlur:0`，不要传整数百分数或 `false`。

创建时可在对应组件的数据配置中同时提供上述样式字段。纯样式更新不回传整份 `data_config`，避免误改指标、筛选及分组。`name_style` 按叶子合并，省略保留，叶子 `null` 清除覆盖。其他样式在允许的叶子传 `null` 清除覆盖；不要清空数据来源或包含数据状态的 `progress/statistics` 整个节点。

进度图格式名支持 `digital/digital_without_separator/scientific/percentage_rounded/cyn_rounded/dollar_rounded`。百分比数值格式与进度百分比精度是两回事，日期进度不接受数值格式、数值单位或 `areaType:number`。百分比数值格式不能同时指定数值单位。

Get 返回已保存的公开样式覆盖，不包含主题合成后的完整有效样式。`--no-validate` 仅跳过本地语义校验，不绕过服务端字段校验和权限检查。

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
