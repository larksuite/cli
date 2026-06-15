# Dashboard（仪表盘/数据看板）模块指引

Dashboard 是 Base 中的数据可视化看板，可以把表格数据变成组件（chart/block）展示。

## 核心概念

- **Dashboard（仪表盘）**：可视化容器，ID 通常为 `blk...`，属于 Base 资源目录里的 block。
- **Dashboard block / Chart（组件/图表）**：仪表盘内的单个可视化组件，`block_id` 通常为 `cht...`。
- **data_config**：图表组件的数据源和统计配置；只有创建/更新 chart/statistics 组件时才需要读 [lark-base-dashboard-block-data-config.md](lark-base-dashboard-block-data-config.md)。

## 读操作命令速查

| 用户目标 | 命令 | 说明 |
|---|---|---|
| 列出仪表盘 | `+dashboard-list` | 定位 dashboard_id；需要更全资源目录时可用 `+base-block-list` |
| 查看仪表盘整体 | `+dashboard-get` | 返回 dashboard 元数据和组件列表 |
| 列出组件 | `+dashboard-block-list` | 只要组件清单时优先用它 |
| 查看组件元数据 | `+dashboard-block-get` | 返回组件 name/type/data_config/layout 等 |
| 读取图表最终数据 | `+dashboard-block-get-data` | 返回图表计算结果；不需要 `--dashboard-id`，也不返回 name/type/data_config |

## 读取路径

1. 先 `+dashboard-list` 或从 URL `table=blk...` 定位 dashboard。
2. 看整体结构用 `+dashboard-get`；只看组件清单用 `+dashboard-block-list`。
3. 要解释某个组件配置时用 `+dashboard-block-get`；要分析图表算出的数据时用 `+dashboard-block-get-data`。
4. text block 没有图表计算结果，不要调用 `+dashboard-block-get-data`。

## 组件类型速查

| 用户想看什么 | type | 说明 |
|---|---|---|
| 数据趋势（时间变化） | `line` | 折线图 |
| 类别比较（谁高谁低） | `column` | 柱状图 |
| 占比分布 | `pie` | 饼图 |
| 单个关键指标 | `statistics` | 指标卡 |
| 富文本说明/标题/注释 | `text` | 文本组件，通常不需要 table/field/data_config 复杂模板 |

## 注意

- dashboard 的 ID 是 Base 资源目录层的 `blk...`；dashboard 内组件的 `block_id` 通常是 `cht...`，不要混用。
- `+dashboard-block-get-data` 只适合 chart/statistics 等有计算结果的组件；需要元数据先用 `+dashboard-block-get`。
- 创建/更新/删除/重排等写操作读 [lark-base-dashboard-write.md](lark-base-dashboard-write.md)。
