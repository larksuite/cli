# Dashboard 进度图创建、更新和获取

使用现有 `+dashboard-block-create`、`+dashboard-block-update` 和 `+dashboard-block-get`。
创建提交完整配置，更新只提交变化字段，获取返回完整的生效配置。外层组件
`type` 是 `progress`；`data_config.progress.type` 是 `value` 或 `date`。

## 创建数值进度

先用 `+table-list`、`+field-list` 确认表名和字段名；两侧可以引用不同表和不同筛选。

```bash
lark-cli base +dashboard-block-create \
  --base-token <base_token> --dashboard-id <dashboard_id> \
  --name "回款进度" --type progress \
  --data-config '{"progress":{"type":"value","current":{"type":"field","table_name":"订单表","series":[{"field_name":"回款金额","rollup":"SUM"}]},"target":{"type":"custom","value":100000}}}'
```

`current`、`target` 均必填，各自支持：

| 来源 | 完整配置 |
|---|---|
| 自定义数值 | `{"type":"custom","value":100}`，允许有限的 0、负数和小数 |
| 记录数 | `{"type":"field","table_name":"订单表","count_all":true}` |
| 字段聚合 | `{"type":"field","table_name":"订单表","series":[{"field_name":"金额","rollup":"SUM"}]}` |

每侧的 `count_all:true` 与 `series` 必须二选一；`series` 恰好一项，聚合仅支持
`SUM/MAX/MIN/AVERAGE`。记录数不使用字段 `COUNTA`。`field` 来源可带 `filter`：

```json
{
  "type": "field",
  "table_name": "订单表",
  "count_all": true,
  "filter": {
    "conjunction": "and",
    "conditions": [{"field_name": "状态", "operator": "is", "value": ["已完成"]}]
  }
}
```

筛选对象使用 `and/or` 和[通用筛选规则](lark-base-dashboard-block-config.md)，条件 1～50 项，
每个值数组至多 200 项；不支持嵌套筛选、分组、视图范围和额外数据源。
创建时不筛选就省略 `filter`，不接受 `filter:null`。

## 创建日期进度

```bash
lark-cli base +dashboard-block-create \
  --base-token <base_token> --dashboard-id <dashboard_id> \
  --name "本月进度" --type progress \
  --data-config '{"progress":{"type":"date","current":{"type":"relative","value":"today"},"target":{"type":"relative","value":"month"}}}'
```

| 位置 | 完整配置 |
|---|---|
| 当前时间 | `{"type":"custom","value":1790000000000}` 或 `{"type":"relative","value":"today"}`；相对值支持 `today/yesterday` |
| 目标范围 | `{"type":"custom","range":[1789000000000,1791000000000]}` 或 `{"type":"relative","value":"month"}`；相对值支持 `month/quarter/year` |
| `dateFormat` | 可省略；服务端默认 `YYYY-MM-DD`，完整枚举见下文 |

`dateFormat` 的完整枚举为 `YYYY-MM-DD`、`YYYY/MM/DD`、`MM-DD`、`MM/DD/YYYY`、`DD/MM/YYYY`。
两侧可以混合相对时间和自定义时间。时间戳是安全整数毫秒值；范围必须包含两个端点且起点小于终点。
相对日期保持相对语义。日期进度不支持 `field` 来源；数值进度中的日期字段聚合也不会自动变成日期模式。
纯自定义数值和日期模式创建时，Base 仍必须至少有一张表，无需填写顶层 `table_name`。

## 局部更新

```bash
lark-cli base +dashboard-block-update \
  --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id> \
  --data-config '{"progress":{"target":{"value":200000}}}'
```

上例要求 target 当前为自定义数值。CLI 不先获取完整配置再合并，也不补创建默认值；
服务端根据已有组件判断省略的模式、来源和表。更新没有外层 `--type`。

| 更新场景 | `--data-config` 示例 | 行为 |
|---|---|---|
| 清空当前值筛选 | `{"progress":{"current":{"filter":null}}}` | current 必须为字段来源；保留指标和 target |
| 修改字段聚合 | `{"progress":{"current":{"series":[{"field_name":"金额","rollup":"MAX"}]}}}` | 整体替换 series；移除 count_all；同表筛选保留 |
| 改为记录数 | `{"progress":{"current":{"count_all":true}}}` | 移除 series；保留同表筛选 |
| 修改日期格式 | `{"progress":{"dateFormat":"YYYY/MM/DD"}}` | 当前必须为日期模式；两侧保留 |
| 修改相对日期范围 | `{"progress":{"target":{"value":"quarter"}}}` | 日期 target 当前必须为 relative |
| 不修改 | `{}`、`{"progress":{}}`、`{"progress":{"current":{}}}` | 不改变对应配置 |

同来源、同表时，未传 `filter` 保留原值，传对象整体替换，`null` 清空。即使提交了完整对象，
省略筛选也不表示清空。筛选条件、`series`、`range` 都整体替换；空条件和半个范围均无效。
不传 `dateFormat` 保留已有格式；恢复默认值须显式传 `YYYY-MM-DD`。
除筛选清空外，不用 `null` 删除必填配置；未知字段和跨模式字段由服务端拒绝。

## 切换来源、表和模式

| 操作 | 必须提交 | 保留内容 |
|---|---|---|
| 某侧 field → custom | `{"type":"custom","value":200}` | 另一侧 |
| 某侧 custom → field | `type:"field"`、表名、count_all 或完整 series | 另一侧；不启用隐藏的旧查询 |
| 某侧换表 | 新表名、新统计方式 | 另一侧；未传筛选则清除该侧旧表筛选 |
| 日期某侧 relative ↔ custom | 显式新 type 和完整 value 或 range | 另一侧和 dateFormat |
| 整体 value ↔ date | 显式 `progress.type` 和完整 current、target | 展示配置；切到 date 时省略格式使用默认值 |

不能根据传入字段隐式切换来源。传相同类型或同一张表仍按局部更新处理。
两侧共享表时，修改一侧的表不影响另一侧。`count_all:false`、空 `series`、两种指标同时传入均无效。

## 获取与输入文件

```bash
lark-cli base +dashboard-block-get \
  --base-token <base_token> --dashboard-id <dashboard_id> --block-id <block_id>
```

从成功响应的 `data.block.data_config.progress` 读取完整生效配置。创建和更新的响应也返回服务端
回读结果。清空后省略 `filter`，相对日期不会展开为某个固定日期；隐藏查询和固定值不返回。
默认日期格式及筛选值可能被归一化，不保证逐字等于输入。这里读取配置，不计算进度百分比。

创建和更新都支持 `--data-config @config.json`。`--dry-run` 展示实际请求体；
`--no-validate` 可跳过本地语义校验，但仍检查 JSON 对象语法，服务端也仍会校验。
进度图仅在 Dashboard 支持，不因此开放 BaseApp 页面组件。
