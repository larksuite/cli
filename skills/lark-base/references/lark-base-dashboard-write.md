# Dashboard 写操作指引

本文档只管 dashboard / dashboard block 写操作流程。图表组件的 `data_config` 结构、模板和筛选规则见 [lark-base-dashboard-block-data-config.md](lark-base-dashboard-block-data-config.md)。

## 写操作速查

| 用户目标 | 命令 | 是否读 data_config SSOT |
|---|---|---|
| 创建/重命名/删除仪表盘 | `+dashboard-create/update/delete` | 否 |
| 添加 chart/statistics 组件 | `+dashboard-block-create` | 是 |
| 更新 chart/statistics 的数据配置 | `+dashboard-block-update` | 是 |
| 添加/更新 text 组件 | `+dashboard-block-create/update --type text` | 否，`data_config` 只传 `{"text":"..."}` |
| 删除组件 | `+dashboard-block-delete` | 否 |
| 重排/美化布局 | `+dashboard-arrange` | 否 |

## 写前定位

- 写操作前先定位真实 `base_token`、`dashboard_id`、表名/字段名；不要凭用户口述猜。
- 创建/改图前先 `+table-list` 拿表，再用 `+field-list-batch --compact --table-id <表1> --table-id <表2>` 一次取相关表字段，不要逐表调用。
- 更新组件前先 `+dashboard-block-get` 读取当前 block 的 `name/type/data_config`，只改目标字段。

## 创建仪表盘并添加多个组件

```bash
# 1. 创建空白仪表盘
lark-cli base +dashboard-create --base-token xxx --name "销售数据分析"
# 记录 dashboard_id

# 2. 获取数据源结构
lark-cli base +table-list --base-token xxx
lark-cli base +field-list-batch --base-token xxx --compact --table-id tbl_a --table-id tbl_b

# 3. 规划组件（根据用户需求确定组件类型和数量）
# 例如：总销售额（statistics）、月度趋势（line）、品类占比（pie）

# 4. 串行创建每个组件；chart/statistics 的 data_config 先读 lark-base-dashboard-block-data-config.md
lark-cli base +dashboard-block-create \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --name "总销售额" \
  --type statistics \
  --data-config '{"table_name":"订单表","series":[{"field_name":"金额","rollup":"SUM"}]}'

# 第 2 个组件（等上一个完成后再执行）
lark-cli base +dashboard-block-create \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --name "月度趋势" \
  --type line \
  --data-config '{"table_name":"订单表","series":[{"field_name":"金额","rollup":"SUM"}],"group_by":[{"field_name":"月份","mode":"integrated"}]}'

# 5. 需要美化布局时再执行 arrange
lark-cli base +dashboard-arrange --base-token xxx --dashboard-id blk_xxx
```

## 在已有仪表盘添加组件

```bash
# 1. 定位 dashboard 并查看现状，避免重复创建；已有组件也可作为 data_config 参考
lark-cli base +dashboard-list --base-token xxx
lark-cli base +dashboard-get --base-token xxx --dashboard-id blk_xxx

# 2. 获取相关表字段
lark-cli base +table-list --base-token xxx
lark-cli base +field-list-batch --base-token xxx --compact --table-id tbl_a --table-id tbl_b

# 3. 串行创建组件
lark-cli base +dashboard-block-create \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --name "新组件名" \
  --type column \
  --data-config '{...}'
```

## 编辑已有组件

- `+dashboard-block-update` **不能修改组件 `type`**；如需换图表类型，删除旧 block 后用 `+dashboard-block-create` 新建。
- block 换数据源表（`table_name`）时，通常也应删除旧 block 后新建，避免旧字段上下文残留。
- `+dashboard-block-update` 适合同一数据源内改 `name` / `series` / `filter` / `group_by` 等顶层字段；未传顶层字段会保留，传入字段内部是全量替换。

```bash
# 先列出组件，精确定位目标 block；查看已有组件可避免重复创建，也可参考 data_config 结构
lark-cli base +dashboard-block-list --base-token xxx --dashboard-id blk_xxx
lark-cli base +dashboard-block-get --base-token xxx --dashboard-id blk_xxx --block-id cht_xxx
lark-cli base +dashboard-block-update \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --block-id cht_xxx \
  --data-config '{...}'
```

## 删除、重排和 text 组件

- 删除具名图表：`+dashboard-list` → `+dashboard-block-list` 精确匹配名称 → `+dashboard-block-delete`；长 `block_id` 用变量传参，避免手抄截断。
- 布局/重排/撑满/排列美观：直接用 `+dashboard-arrange`；不要尝试用 `+dashboard-block-update` 修改 layout，layout 不是 `data_config`。`+dashboard-arrange` 是服务端智能布局，无法指定具体位置（如第一排放 A、第二排放 B）；不建议在已有仪表盘上自动调用，除非用户明确要求。
- text 组件：如果只是新增/更新说明文案，不需要读取 table/field；确认 dashboard/block 后直接写 `data_config={"text":"..."}`。

## 写后验证

- 创建/更新后用 `+dashboard-block-get` 回读元数据；需要验证图表计算结果时再用 `+dashboard-block-get-data`。
- 多组件创建要串行执行；不要并发创建 dashboard block。
