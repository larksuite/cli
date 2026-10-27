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

## 形状、格式与颜色

数据继续放在 `data_config.progress`，样式放在 `data_config.viewModel.rules`，不增加 `style`。通用合并与清空规则读取 [自定义样式](lark-base-dashboard-block-config.md#自定义样式)。数值、日期模式都支持 `shape=bar|semiRing|circle`；服务端同步对应形态，不要求调用方同时写 `chartKind`。

一次创建半环数值进度图，并独立设置数值精度和进度百分比精度：

```bash
lark-cli base +dashboard-block-create \
  --base-token '<BASE_TOKEN>' --dashboard-id '<DASHBOARD_ID>' \
  --name '半环进度' --type progress --as user \
  --data-config '{"progress":{"type":"value","current":{"type":"custom","value":37.125},"target":{"type":"custom","value":100}},"viewModel":{"rules":{"progress":{"shape":"semiRing","color":"#3366FF","opacity":0.4},"formatInfo":{"formatName":"digital","precision":2,"percentPrecision":1}}}}'
```

`precision` 控制当前／目标数值，`percentPrecision` 控制进度百分比，两者均为 0–9 的整数。上例数值为 37.13，进度为 37.1%。`formatName=percentage_rounded` 将数值本身按百分比显示：例如 current=0.37125、target=1、precision=2 时显示 37.13%／100.00%，不是把输入改成 37.125／100。

以下都是 `+dashboard-block-update --data-config` 的纯样式补丁，命令还需提供 Base、Dashboard、Block ID 及 `--as user`：

| 场景 | 补丁 |
|---|---|
| 修改形状 | `{"viewModel":{"rules":{"progress":{"shape":"circle"}}}}` |
| 修改百分比精度 | `{"viewModel":{"rules":{"formatInfo":{"percentPrecision":3}}}}` |
| 当前值／目标值文字颜色 | `{"viewModel":{"rules":{"progress":{"curFontColor":"#3366FF","curFontOpacity":0.6,"targetFontColor":"#34C759","targetFontOpacity":1}}}}` |
| 百分比文字颜色 | `{"viewModel":{"rules":{"progress":{"percentFontColor":"#F54A45","percentFontOpacity":1}}}}` |
| 日期范围文字颜色 | `{"viewModel":{"rules":{"progress":{"rangeFontColor":"#8E44AD","rangeFontOpacity":0.6}}}}` |

按进度百分比配置色区间：

```json
{
  "viewModel": {
    "rules": {
      "progress": {
        "isAreaColor": true,
        "areaType": "percent",
        "colors": [
          {"range":[0,30],"color":"#F54A45","opacity":1},
          {"range":[30,70],"color":"#FFC60A","opacity":1},
          {"range":[70,100],"color":"#34C759","opacity":1}
        ]
      }
    }
  }
}
```

百分比区间使用 0–100，不归一到 0–1；`opacity` 使用 0–1。数值模式也可用 `areaType:number` 按当前绝对值匹配，例如 current=75、target=200 时使用范围 `[50,100]`，而不是按 37.5% 匹配。每段起点必须小于终点，区间须有序且不重叠；`colors` 在更新时整体替换。切回单色可传 `isAreaColor:false` 并设置 `color/opacity`。

日期进度通过原有 `progress.type=date/current/target/dateFormat` 配置数据，也可同时设置形状、单色／百分比色区间和文字颜色；不支持数值格式、数值单位及 `areaType:number`。

### 与 Web 的快照一致性

服务端将当前值和目标值分别写入 `cur/target` 指标；外层 `dataCondition` 与目标条件一致。创建和目标值更新遵循这一规则，只更新当前值不改主条件；日期模式隐藏数值条件同样使用目标值。调用方只提交公开 `progress`，不要手写 `rangeDefinition` 或补发 `user_change`。此约束需要配套服务端，旧组件不会因部署自动重建。

### 百分比格式与进度百分比不是同一项

先确认用户要改的是哪一种显示，不为满足回执关键词而改变业务单位：

| 用户要求 | 配置位置与含义 |
|---|---|
| 进度完成率保留一位小数 | `viewModel.rules.formatInfo.percentPrecision:1`；只控制 current / target 得到的进度百分比精度 |
| 当前值／目标值本身显示为百分比 | `viewModel.rules.formatInfo.formatName:"percentage_rounded"`，配合 `precision` 控制数值小数位；输入仍为比例小数 |
| 当前值／目标值是金额、数量 | 保留对应数值格式；不能仅为显示完成率而改为百分比格式 |

例如 current=37.125、target=100，`formatName:"digital",precision:2,percentPrecision:1` 表示数值 37.13／100.00、进度 37.1%；current=0.37125、target=1 且用户要百分比数值时，`formatName:"percentage_rounded",precision:2,percentPrecision:1` 表示数值 37.13%／100.00%、进度 37.1%。不要同时把比例乘以 100 再套百分比格式。

只调整完成率精度的最小补丁：

```json
{"viewModel":{"rules":{"formatInfo":{"percentPrecision":1}}}}
```

明确要求当前值／目标值显示为百分比时的补丁：

```json
{"viewModel":{"rules":{"formatInfo":{"formatName":"percentage_rounded","precision":1,"percentPrecision":1}}}}
```

独立 `+dashboard-block-get` 回读 `data_config.viewModel.rules.formatInfo`，分别核对格式、数值精度与进度百分比精度，并核对 current／target 的来源与单位。保留返回 JSON 作为配置证据；需要实际显示效果时另验页面，配置成功不等于渲染已验证。日期模式不写这些数值格式字段。

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
