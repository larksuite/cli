# Dashboard 图表联动

用户希望点击源图表的数据点后，其他图表随之筛选时，使用
`+dashboard-block-update --linkage`。本期只配置同一 Base、同一仪表盘中
数据源相同的组件，写入仅接受 `target_block_ids`。

先检查 `+dashboard-block-update --help` 是否包含 `--linkage`。参数缺失时说明当前 CLI 版本未接入，需要升级到已发布的支持版本；不要把 Skill 已更新当作 binary 已更新。

## 选择入口

- 点击图表的数据点影响其他组件：本指引的图表联动。
- 设置仪表盘整体筛选：[全局筛选](lark-base-dashboard-global-filter.md)。
- 点击按钮执行流程：[按钮工作流](lark-base-dashboard-button-rule.md)。
- 查询图表计算结果：`+dashboard-block-get-data`。

联动属于源组件的顶层 `linkage` 配置，与 `data_config` 平级。
组件的固定 `data_config.filter` 可以与联动并存；固定筛选不能代替点击交互。

## 支持范围与输入

源图表支持 `pie`、`column`、`bar`、`line`、`area`、`scatter`、`radar`、
`funnel`、`combo`、`ranking`、`wordCloud`。当前 `ring` 不支持发起；
`pivot_table` 受更新入口限制，也不能作为本期发起方。
目标继续受服务端组件类型检查约束：普通图表、透视表和表格视图可作为候选，
倒计时、AI 图表、切片器、富文本、按钮、布局容器和源自身不可作为目标。
布局内符合条件的子组件可作为目标；源仍须能由普通 Block Get/Update 定位。

同源指实际数据表 ID 集合完全一致，顺序无关。图表可以使用不同维度、指标和
固定筛选。多数据源时必须逐个目标比较，不能只判断集合相交、子集或所有目标的并集。
同名表、同名字段都不能证明同源；数据源身份、组件存在性和类型由服务端验证。

`--linkage` 支持内联 JSON 和 `@file`；文件内容就是联动对象，不再包一层 `linkage`。
仅允许 `target_block_ids`，其值必须是数组，最多 100 个非空且不重复的目标 ID。
`field_mappings` 即使为空也不接受。`--no-validate` 不跳过联动输入约束。

| 请求 | 语义 |
|---|---|
| 省略 `--linkage` | 保留现有联动和映射 |
| 提供非空 `target_block_ids` | 整体替换当前源图表的目标列表；本次配置无字段映射 |
| `{"target_block_ids":[]}` | 清除该源图表的联动和字段映射 |
| `{}`、`null`、缺少目标列表 | 参数错误 |
| 携带 `field_mappings` | 本期写入不支持，参数错误 |

## 配置与修改流程

1. 确定真实 `base_token`、`dashboard_id` 和源、目标 `block_id`。
   已有可信 ID 时直接使用；缺少 ID 时用 `+dashboard-block-list --page-size 100`
   定位，按 `has_more/page_token` 翻页。所有目标必须来自同一仪表盘。
2. 用 `+dashboard-block-get` 读取源的 `type`、`data_config` 和 `linkage`，
   按需读取候选目标。源、目标使用表的名称必须来自真实返回，不猜测数据源。
3. 根据用户意图构造最终完整目标列表。“新增目标”保留原有目标后加入新目标；
   “移除目标”提交剩余列表。两个方向的联动分别属于各自源组件。
4. 执行源组件的 Update。只修改联动时仅传 `--linkage`，不整包写回 Get 响应。
   创建新的图表时先串行完成创建、取得实际 ID，再配置联动。
5. 检查写响应中的 `data.block.linkage.target_block_ids` 与最终目标一致。
   清除后通常省略 `linkage`。结果不确定或需要确认持久化时，再执行一次 Get。

例如，A 原来联动 B，现在要新增 C，应提交 B 和 C；只提交 C 会移除 A → B。
修改 A 不会更改 X → A 等其他源组件的配置。

```bash
# 读取源组件发起的联动
lark-cli base +dashboard-block-get \
  --base-token '<base_token>' --dashboard-id '<dashboard_id>' \
  --block-id '<source_block_id>'

# 替换为完整的同源目标列表；占位 ID 须替换为真实组件 ID
lark-cli base +dashboard-block-update \
  --base-token '<base_token>' --dashboard-id '<dashboard_id>' \
  --block-id '<source_block_id>' \
  --linkage '{"target_block_ids":["<target_block_id_1>","<target_block_id_2>"]}'

# 清除当前源的联动
lark-cli base +dashboard-block-update \
  --base-token '<base_token>' --dashboard-id '<dashboard_id>' \
  --block-id '<source_block_id>' --linkage '{"target_block_ids":[]}'
```

也可以将 `{"target_block_ids":["<target_block_id>"]}` 保存为工作区文件，
用 `--linkage @linkage.json` 提交。追加 `--dry-run` 只验证并预览请求，
不访问真实数据源，也不证明部署、权限或联动生效。

## 回读、历史配置与恢复

- Get 返回当前图表发起的联动，不返回其他图表指向它的反向关系。
- 回读若含历史 `field_mappings`，不要删掉该字段后把“追加目标”当成等价更新。
  本期可以明确重设为同源且无映射的联动，或清除；需要保留、编辑映射的任务超出本期范围。
- 跨表或多数据源不一致时报告不支持，重新选择符合条件的目标。
  不自动复制表、改数据源、修改固定筛选或绕过参数限制来模拟联动。
- 更新失败可能已有部分配置写入。先 Get 源组件确认当前状态，再决定是否重试；
  不将一次错误解释成“所有修改都未发生”，也不反复盲目重试。
- 源或目标随后换表时需要重新确认联动有效性。若一次请求同时换表和配置联动，
  服务端必须基于最终数据源校验；服务端能力尚未确认时，分两步修改并重新读取。

## 交互验收

配置回读证明保存结果。用户要求实际点击效果时，还需在仪表盘页面选择源图表的
数据点，确认指定目标按预期变化、无关图表不受影响，并取消选择确认恢复。
`+dashboard-block-get-data` 不会模拟选择数据点，不能单独证明交互联动通过。
无法进行页面验证时，明确交付的是已回读的配置，点击效果尚未验证。
