# BaseApp Block `data_config`

本文件说明 BaseApp 组件 `data_config` 的 CLI 映射与操作约束，不复制完整字段 Schema。App 图表的每个 `data_sources[]` 元素复用 [Dashboard Block 配置](lark-base-dashboard-block-config.md) 的字段取值、筛选、分组、排序及规范化规则；区别是 Dashboard 使用扁平单数据源结构，而 App 图表在顶层使用共享的 `base_token` 和多数据源 `data_sources[]`。列表组件是 App 独有协议，不复用 Dashboard 图表结构。本文所称“组件协议”是指 CLI 随版本发布的 API 元数据、本文明确列出的约束及实际服务端校验结果。

## 类型映射

- 图表：`--type column|bar|line|pie|ring|area|combo|scatter|funnel|wordCloud|radar|statistics`
- 富文本：`--type text`（与 Dashboard 文本组件同名同义）
- 列表：`--type list --sub-type standard|grouped|collapsible|card|detail`
- 列表省略 `--sub-type` 时默认 `standard`
- `type/sub_type` 创建后不可修改

## 外层请求字段

- Create 只发送 `name`、`type`、按需发送的 `sub_type` 和 `data_config`。`name`、`type` 必填；图表和列表的 `data_config` 必填，富文本可省略。
- 标准列表未显式指定 `--sub-type` 时，不发送 `sub_type`，由服务端使用 `standard` 默认值；其他列表类型必须发送对应 `sub_type`。
- 图表和富文本不得发送 `sub_type`。
- Update 只发送 `name`、`data_config`，且至少提供一个；未传字段保持不变，不允许修改 `type`、`sub_type`。
- 布局、位置、尺寸、`show_title` 等展示配置不属于本期公开 Create/Update 请求字段，CLI 不提供或提升这些字段。

## 列表配置

列表公共数据源字段为单值 `base_token` 和 `table_name`。每个列表最多关联一个 Base，且该 Base 必须位于 App 所在的同一 Workspace。

按组件协议，各 subtype 使用以下字段组：

- 公共：`base_token`、`table_name`、`filter`、`sort_by`
- `standard/grouped/collapsible`：`columns`、`group_by`
- `card`：`fields`、`card_config`
- `detail`：`fields`、`detail_config`
- `columns` 和 `fields` 都是可选字段。未指定时 CLI 不发送，由服务端使用产品默认字段。
- 只有用户显式指定 `columns` / `fields` 时才发送；显式传 `[]` 表示明确发送空数组，不能作为默认值自动补入。
- `filter`、`sort_by`、`group_by`、`card_config`、`detail_config` 也都是可选字段；未指定时不发送。
- 列表 Create 的 `data_config` 必填，其中只有 `base_token` 和 `table_name` 是顶层必填字段。
- 可选对象一旦传入，其内部必填项仍须满足协议，例如 `filter` 必须包含 `conjunction` 和 1～50 项 `conditions`。

不要添加协议未定义的语义校验，尤其不要假设：

- detail/card 必须有 title；
- grouped/collapsible 必须或只能有一个 group_by；
- fields 存在 role 或 visible 属性。

未知顶层字段会被本地校验拒绝；只有确认 CLI 校验与最新协议不一致时才使用 `--no-validate`。

## 创建示例

```bash
lark-cli base +app-block-create \
  --app-token <app_token> --page-id <page_id> \
  --name "订单列表" \
  --type list --sub-type standard \
  --data-config '{"base_token":"<base_token>","table_name":"订单"}'
```

字段的具体对象结构与必填性以 CLI 当前版本的 API 元数据和服务端校验结果为准，不在这里猜测未公开属性。

## 更新语义

下面是只更新顶层 `filter` 的示例，适用于协议将 `filter` 定义在 `data_config` 顶层的组件（所有列表 subtype 均可使用）。组件类型在创建后不可修改，所以 update 命令不再传 `--type`。App 图表的 `filter` 定义在对应的 `data_sources[]` 元素中；更新图表筛选时必须按图表结构传入完整 `data_sources`，不能把 `filter` 提到顶层。

```bash
lark-cli base +app-block-update \
  --app-token <app_token> --page-id <page_id> --block-id <block_id> \
  --data-config '{"filter":{"conjunction":"and","conditions":[{"field_name":"状态","operator":"is","value":"已完成"}]}}'
```

- CLI 只发送用户显式传入的字段。
- 未传字段由服务端保持不变。
- 不为 update 注入 create 默认值，不先读取后拼成全量配置。
- 数组/对象字段的替换粒度以组件协议和服务端校验结果为准。

## 图表与富文本

**App 图表是多数据源结构（`ChartDataConfig`），与 Dashboard 的扁平单源结构不同。** 顶层用一个 `base_token`（所有数据源共用），`table_name` / `series` / `count_all` / `group_by` / `filter` 下沉到每个 `data_sources[]` 元素里；顶层另有可选的 `data_source_mode` 和 `sort`。每个数据源内部各字段的取值逻辑与 [Dashboard Block 配置](lark-base-dashboard-block-config.md) 完全一致（`series[].rollup` 大写、`group_by[].sort` 小写等），CLI 对每个 `data_sources[]` 元素复用同一套规范化与校验。富文本使用 `--type text`，配置为 `{"text":"..."}`，无数据源；Create 时可省略 `data_config`，等价于空文本。

> **text 内容怎么取**：text 组件没有 `/data` 接口，走 `+app-block-get-data` 会被服务端兜底成通用 500。改用 `+app-block-get --block-id <widget_id>` 直接读 `data_config.text`（Markdown 原文）。图表仍走 `+app-block-get-data --block-id <chart_token>`。

顶层参数：

| 参数 | 必填 | 取值 | 说明 |
|-|-|-|-|
| `base_token` | 是 | `string` | 数据所在 Base 的 token；所有数据源共用同一个值。App 命令不带 `--base-token`，只能写在 data_config 内 |
| `data_sources` | 是 | `ChartDataSourceConfig[]` | 有序数组，至少一项 |
| `data_source_mode` | 否 | `aggregate` / `compare` | `aggregate`（默认）在横轴聚合数据源；`compare` 按数据源拆分系列 |
| `sort` | 否 | `{type: group\|value\|record, order?: asc\|desc}` | 顶层排序；`statistics` 不允许 |

每个 `data_sources[]` 元素：`table_name`（必填）、`series` 与 `count_all=true` 二选一、`group_by`（最多 2 项，`statistics` 不允许）、`filter`。

```json
{
  "base_token": "A2f5boKjfazMzesI9zKbmugTc4T",
  "data_sources": [
    {
      "table_name": "数据表",
      "count_all": true,
      "group_by": [
        { "field_name": "文本", "mode": "integrated", "sort": { "type": "value", "order": "desc" } }
      ]
    }
  ]
}
```

对应命令（单数据源计数柱状图）：

```bash
lark-cli base +app-block-create \
  --app-token <app_token> --page-id <page_id> \
  --name "文本分布" --type column \
  --data-config '{"base_token":"A2f5boKjfazMzesI9zKbmugTc4T","data_sources":[{"table_name":"数据表","count_all":true,"group_by":[{"field_name":"文本","mode":"integrated","sort":{"type":"value","order":"desc"}}]}]}'
```

多数据源示例（两张表各出一条系列，按数据源拆分）：

```bash
lark-cli base +app-block-create \
  --app-token <app_token> --page-id <page_id> \
  --name "销售与成本" --type combo \
  --data-config '{"base_token":"bas_xxx","data_source_mode":"compare","data_sources":[{"table_name":"销售表","group_by":[{"field_name":"月份","sort":{"type":"group","order":"asc"}}],"series":[{"field_name":"销售额","rollup":"SUM"}]},{"table_name":"成本表","group_by":[{"field_name":"月份","sort":{"type":"group","order":"asc"}}],"series":[{"field_name":"成本","rollup":"SUM"}]}],"sort":{"type":"group","order":"asc"}}'
```

Update 语义：传入 `data_sources` 即全量替换整个有序数组；修改 `base_token` 时必须同时传入完整 `data_sources`。请求不得包含 `sub_type`（平滑/堆积/百分比等展示变体走产品默认值）。布局、位置、尺寸和展示配置不属于本期公开 Create/Update 协议。其他请求字段以 CLI 当前版本的 API 元数据和服务端校验结果为准。
