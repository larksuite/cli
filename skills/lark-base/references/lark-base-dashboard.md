# Dashboard（仪表盘/数据看板）模块指引

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

Dashboard 是 Base 中的数据可视化看板，可以把表格数据变成**组件**（图表、指标卡等）进行展示。

## 核心概念

- **Dashboard（仪表盘）**：容器，包含多个组件
- **Block（组件）**：仪表盘中的单个可视化元素（柱状图、折线图、饼图、指标卡等）
- **data_config**：组件的数据源配置（表名、字段、分组等）

## 能力速览

| 你想做什么 | 用这些命令 | 关键文档 |
|------|-----------|---------|
| 创建/删除/改名称 | `+dashboard-create/delete/update` | 本页下方「仪表盘管理」 |
| 在仪表盘里添加组件 | `+dashboard-block-create` | 先定位 dashboard、表和字段，再读 [Dashboard Block 配置](lark-base-dashboard-block-config.md) 构造 `data_config` |
| 修改组件 | `+dashboard-block-update` | 先读 block 现状，再读 [Dashboard Block 配置](lark-base-dashboard-block-config.md) 决定替换哪些顶层 key |
| 创建/更新时指定组件精确位置大小 | `+dashboard-block-create/update --position` | 本页下方「精确布局 --position vs +dashboard-arrange」 |
| 查看仪表盘有哪些组件 | `+dashboard-get` 或 `+dashboard-block-list` | 本页下方「查看仪表盘」 |
| 读取图表计算结果 | `+dashboard-block-get-data` | 返回图表最终数据协议；需要 block 元数据先用 `+dashboard-block-get` |
| 智能重排组件布局 | `+dashboard-arrange` | 用户明确要求重排，或本次会话新建仪表盘的收尾整理；无法指定 `x/y/w/h`、精确位置或尺寸 |

## 精确布局 --position vs +dashboard-arrange

create/update 可选 `--position`，用 12 列栅格坐标精确指定单个组件的落点与大小：`{"x","y","w","h"}`，`x`/`y` 为左上角坐标（>=0），`w` 为宽度（1..12，且 `x+w<=12`），`h` 为高度（>=1）。它与 `name`/`type`/`data_config` 平级挂在请求体顶层。

> [!IMPORTANT]
> - **四个 key 必须齐全且都是数字**：`position` 按整体提交、不做逐字段合并，所以只传 `{"x":6}` 表达的不是"只挪位置不改大小"，而是一个缺了三项的位置。本地会拒绝残缺对象（含显式 `null`）。
> - 坐标**取值不做本地校验**：越界、负值或重叠坐标会原样发给服务端，由服务端自动重排。调用方仍应优先规划 12 列范围内且不重叠的坐标，避免自动重排改变预期落点。
> - 不传 `--position`：create 由服务端自动装箱，update 保持当前布局不变。
> - 只有用户明确给出 `x/y/w/h`、具体行列/顺序、每个组件宽高或可直接换算的尺寸比例时才用 `--position`。"调整布局""美化""撑满""铺满"本身不算精确约束，没有组件级坐标或尺寸时优先用 `+dashboard-arrange` 整盘编排。
> - 命令成功即视为写入成功，一般无需仅为读回位置再调用 `+dashboard-block-get` / `+dashboard-block-list`；成功响应不代表最终渲染位置已经过读回验证。

## statistics 指标卡数值格式

`statistics` 组件可在 create/update 的 `data_config.number_format` 中设置 `formatName` 和 `precision`。create 会校验组件类型和子字段；update 不接收 `--type`，只校验 `number_format` 子字段，再由服务端结合现有 block 类型裁决。枚举、精度范围、更新语义和可复制模板读取 [Dashboard Block 配置](lark-base-dashboard-block-config.md)。

## 典型场景工作流

### 场景 1：从 0 到 1 创建仪表盘

从 0 到 1 创建仪表盘时，按用户需求规划组件的类型和数量，并注意以下要点：

- 聚合方式：创建指标卡或分布图时优先把聚合写进 `data_config`，只有 Top N、字段取值探索、复杂筛选校验或 helper 汇总表场景才先用 `+data-query`。
- Dry-run 边界：已按模板构造的简单指标卡、分布图、趋势图不需要逐个 `--dry-run` 后再真实创建；只有在调试 JSON、检查请求体、复杂自造 `data_config` 或处理 API validation 错误时才 dry-run。
- 验证方式：创建接口成功返回即表示写入成功。只有结果不确定时才用一次 `+dashboard-get` 或 `+dashboard-block-list` 确认仪表盘和组件存在；不要仅为确认创建而逐组件调用 `+dashboard-block-get-data`。
- 布局方式：用户没有给出组件级坐标或尺寸时，创建完成后用一次 `+dashboard-arrange` 整盘编排即可；只有用户明确给出可执行的精确布局约束时才在 create 中带 `--position`，此时通常不再需要 arrange。

示例：搭建一个销售数据分析仪表盘

```bash
# 第 1 步：创建空白仪表盘
lark-cli base +dashboard-create --base-token xxx --name "销售数据分析"
# 记录返回的 dashboard_id

# 第 2 步：获取数据源信息
lark-cli base +table-list --base-token xxx
lark-cli base +field-list --base-token xxx --table-id <table_id>

# 第 3 步：规划应该创建哪些组件（根据用户需求确定组件类型和数量）
# 例如：总销售额（指标卡）、月度趋势（折线图）、负责人 Top N（排行榜）

# 第 4 步：顺序创建每个组件（必须串行执行，不能并发）
# 重要：创建组件前，先确定 dashboard_id、组件 name/type 和真实表字段
# 再阅读 lark-base-dashboard-block-config.md 了解 data_config 结构、组件类型和 filter 规则

# 第 1 个组件
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

# 继续创建其他组件...

# 排行榜组件：省略 limit_size 和 sort 时分别默认 10、value desc
lark-cli base +dashboard-block-create \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --name "负责人销售额 Top 10" \
  --type ranking \
  --data-config '{"table_name":"订单表","series":[{"field_name":"金额","rollup":"SUM"}],"group_by":[{"field_name":"负责人"}]}'

# 第 5 步：组件创建完成后，可按需使用 arrange 智能重排（未使用 --position 时可选）
# 默认布局可能不够美观，arrange 会根据组件数量和类型自动优化布局
# 若任一组件使用了显式 --position，跳过此步骤；除非用户明确同意放弃精确布局
# 若用户没有要求美化/重排，也可跳过；这不影响仪表盘和组件是否已创建成功
lark-cli base +dashboard-arrange \
  --base-token xxx \
  --dashboard-id blk_xxx
```

### 场景 2：在已有仪表盘上添加新组件

```bash
# 第 1 步：列出仪表盘，定位到当前仪表盘
lark-cli base +dashboard-list --base-token xxx
# 获取目标 dashboard_id

# 第 2 步：根据用户诉求规划组件类型和数据源
# 建议先查看当前仪表盘已有组件，避免重复创建，或作为参考
lark-cli base +dashboard-get --base-token xxx --dashboard-id blk_xxx

# 第 3 步：获取数据源信息
lark-cli base +table-list --base-token xxx
lark-cli base +field-list --base-token xxx --table-id <table_id>

# 第 4 步：顺序创建每个新组件（必须串行执行，不能并发）
# 重要：先确定 dashboard_id、组件 name/type 和真实表字段
# 再阅读 lark-base-dashboard-block-config.md 了解 data_config 结构
lark-cli base +dashboard-block-create \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --name "新组件名" \
  --type column \
  --data-config '{...}'
```

### 场景 3：编辑已有组件

> [!IMPORTANT]
> `+dashboard-block-update` **不能修改组件的 `type`**（图表类型），只能更新 `name`、`data_config` 和可选的 `position`。
> 如需更换组件类型，必须先删除再重新创建。

```bash
# 第 1 步：列出仪表盘，定位到当前仪表盘
lark-cli base +dashboard-list --base-token xxx

# 第 2 步：列出组件，获取到目标组件
lark-cli base +dashboard-block-list --base-token xxx --dashboard-id blk_xxx
# 获取目标 block_id
# 提示：查看已有组件可作为参考，或检查是否重复创建相似组件

# 第 3 步：获取组件当前详情
lark-cli base +dashboard-block-get --base-token xxx --dashboard-id blk_xxx --block-id chtxxxxxxxx

# 第 4 步：根据用户编辑诉求准备更新
# 如果编辑诉求涉及数据源变更，需要先获取数据源信息
lark-cli base +table-list --base-token xxx
lark-cli base +field-list --base-token xxx --table-id <table_id>

# 第 5 步：执行更新
# 重要：先读取当前 block 的 name/type/data_config
# 再阅读 lark-base-dashboard-block-config.md 了解 data_config 更新规则
lark-cli base +dashboard-block-update \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --block-id chtxxxxxxxx \
  --data-config '{...}' \
  --position '{...}'   # 可选，只在需要调整布局时传

# 排行榜只修改 Top N；不会覆盖分组、指标、筛选或排序
lark-cli base +dashboard-block-update \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --block-id chtxxxxxxxx \
  --data-config '{"limit_size":20}'

```

### 场景 4：重排仪表盘布局

当用户要求调整布局、重排、美化、撑满或铺满，但没有给出组件级坐标或尺寸时使用。定位仪表盘后用一次 `+dashboard-arrange` 整盘编排即可（对本次会话从零新建的仪表盘，在建完组件后编排一次）。

> [!CAUTION]
> - 排列结果是**服务端智能推荐**，不一定完全符合用户预期
> - `+dashboard-arrange` 无法指定 `x/y/w/h`、精确位置或尺寸，排列逻辑是**自适应**的；只有用户明确给出可执行的组件级坐标、行列或尺寸约束时才改用 `--position`
> - **不建议**在已有仪表盘上自动调用，除非用户明确要求
> - 用户只要求一般性重排、美化、撑满或铺满时，用 `+dashboard-arrange` 整盘编排
> - 编排结果不理想时，可结合用户反馈再调整；不要为了凑效果去探测 raw `lark-cli api`、源码或未公开布局参数

```bash
# 第 1 步：列出仪表盘，定位到目标仪表盘
lark-cli base +dashboard-list --base-token xxx

# 第 2 步：执行智能重排
lark-cli base +dashboard-arrange \
  --base-token xxx \
  --dashboard-id blk_xxx
```

### 场景 5：读取仪表盘或组件现状

**选择查询方式：**
- 想看仪表盘整体结构（含主题、所有组件名称和类型）→ 用 **方式 A**
- 只想快速查看有哪些组件 → 用 **方式 B**
- 想看某个组件的详细 data_config 配置 → 用 **方式 C**
- 想看某个图表/指标卡实际算出来的数据 → 用 **方式 D**

用户要求读取“全部图表”或“完整仪表盘”时，先用方式 B 分页枚举所有 block：使用 `--page-size 100`；若返回 `has_more=true`，继续把本页返回的 `page_token` 传给 `--page-token`，直到 `has_more=false`。收齐后再对每个 block 收口，不能只返回 get-data 成功的子集：

1. 图表或指标卡：使用方式 D 读取计算结果。
2. `text`：使用方式 C，正文位于 `data_config.text`；text 没有计算结果，但属于完整仪表盘内容。
3. get-data 返回不支持的图表类型：先用方式 C 读取真实 `data_config`，确认 `table_name`、维度、指标、聚合与筛选，再按 [Record 查询与分析 SOP](lark-base-record-query-and-analysis-sop.md) 使用 `+data-query` 重建同口径结果。字段必须来自真实配置和表结构，不得猜测；无法等价重建时明确报告限制，不能静默省略该 block。

```bash
# 第 1 步：列出仪表盘，定位到当前仪表盘
lark-cli base +dashboard-list --base-token xxx

# 第 2 步：根据用户诉求查看详情

# 方式 A：查看仪表盘整体情况（包含所有组件列表）
lark-cli base +dashboard-get --base-token xxx --dashboard-id blk_xxx

# 方式 B：列出所有组件
lark-cli base +dashboard-block-list \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --page-size 100

# 方式 C：查看某个组件的详细配置
lark-cli base +dashboard-block-get --base-token xxx --dashboard-id blk_xxx --block-id chtxxxxxxxx

# 方式 D：查看某个图表组件的计算结果（AI 友好的 chart protocol）
lark-cli base +dashboard-block-get-data --base-token xxx --block-id chtxxxxxxxx

# 最后：把获取到的现状信息整理好告诉用户
```

需要读取多个组件的计算结果时，先用方式 B 获取真实 `block_id`（使用 `--page-size 100`；若 `has_more=true`，继续把返回的 `page_token` 传给 `--page-token`，直到 `has_more=false`），再按 [lark-base-dashboard-block-get-data.md](lark-base-dashboard-block-get-data.md) 的多组件范式，在一个 shell 工具调用内串行读取；不要把每个 block 拆成独立模型轮次。文本组件没有计算结果，应跳过。

## 组件类型选择

组件 `type` 决定展示形式：

| 用户想看什么 | 选什么 type | 说明 |
|-------------|------------|------|
| 数据趋势（时间变化） | line | 折线图组件 |
| 类别比较（谁高谁低） | column | 柱状图组件 |
| 占比分布（各部分比例） | pie | 饼图组件 |
| 单个关键指标 | statistics | 指标卡组件 |
| 单维度 Top N 排名 | ranking | 排行榜组件，单分组、单指标 |
| 富文本说明/标题/注释 | text | 文本组件（支持 Markdown） |

详细组件类型和 data_config 完整规则：[Dashboard Block 配置](lark-base-dashboard-block-config.md)

## 常见问题

**Q: 创建组件的命令和 data_config 怎么写？**
A:
1. 先确定 `dashboard_id`、组件 `name`、组件 `type` 和真实表字段
2. 再读 [Dashboard Block 配置](lark-base-dashboard-block-config.md) 了解：
   - 全部组件类型的可复制模板
   - filter 筛选条件格式
   - 字段类型与操作符对应表

**Q: 为什么组件创建失败了？**
A: 常见原因：
- `table_name` 用了 table_id 而不是表名（必须用表名称，如「订单表」）
- `series` 和 `count_all` 同时存在（必须二选一，互斥）
- 字段名拼写错误（必须用 `+field-list` 获取的真实字段名，禁止猜测）
- 组件创建并发执行（必须串行，等上一个完成再执行下一个）

**Q: 可以一次创建多个组件吗？**
A: 不可以，必须串行执行。等上一个 `+dashboard-block-create` 完成后再执行下一个。

**Q: 组件的 `type` 创建后能改吗？**
A: 不能。`+dashboard-block-update` 只能修改 `name`、`data_config` 和 `position`，不能修改 `type`。

**Q: 更新组件的命令和 data_config 怎么写？**
A:
1. 先读取当前 block，确认 `block_id`、当前 `type` 和已有 `data_config`
2. 再读 [Dashboard Block 配置](lark-base-dashboard-block-config.md) 了解 data_config 结构

**data_config 更新策略（顶层 key merge）**：
- 只传入需要修改的顶层字段（如 `series`、`filter`）
- 未传的顶层字段（如 `group_by`）自动保留原值
- 但每个传入的字段内部通常是**全量替换**（如传新 `filter` 会完整覆盖旧 `filter`）；`number_format` 例外，按子字段合并，见 [Dashboard Block 配置](lark-base-dashboard-block-config.md) 的 number_format 小节

**Q: 查看已有组件有什么用？**
A: 在「添加新组件」或「编辑组件」前查看已有组件可以：
- 了解当前仪表盘已有哪些可视化
- 避免重复创建相似的组件
- 参考已有组件的 data_config 结构作为模板

**Q: 我想直接拿图表算好的结果给 AI 分析，应该用什么？**
A: 用 `+dashboard-block-get-data`。它返回图表协议 JSON（常见字段包括 `dimensions`、`measures`、`main_data`，指标卡可能还有 `comparison_data`、`trend_data`），不返回 block 名称、类型、布局或 `data_config`；需要这些元数据时先用 `+dashboard-block-get`。

## 写入前检查

- 创建 block 前必须知道 `base_token`、`dashboard_id`、组件 `name/type` 和 `data_config`。
- 更新 block 前必须知道 `base_token`、`dashboard_id`、`block_id`，并读过当前 block。
- `data_config` 中使用表名和字段名，不使用 table_id / field_id；名称必须来自 `+table-list` / `+field-list` 的真实返回。
