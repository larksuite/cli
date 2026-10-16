# Dashboard（仪表盘/数据看板）模块指引

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

Dashboard 是 Base 中的数据可视化看板，可以把表格数据变成**组件**（图表、指标卡等）进行展示。

## 核心概念

- **Dashboard（仪表盘）**：容器，包含多个组件
- **Block（组件）**：仪表盘中的单个可视化元素（柱状图、折线图、饼图、指标卡等）
- **data_config**：组件的数据源配置（表名、字段、分组等）

## 能力速览

| 你想做什么 | 用这些命令 | 关键文档 |
|------|-----------|---------|
| 创建/删除/修改名称或主题 | `+dashboard-create/delete/update` | 本页下方「仪表盘管理」 |
| 管理仪表盘分享 | `+dashboard-share-get` / `+dashboard-share-update` | 本页下方「仪表盘管理」 |
| 在仪表盘里添加组件 | `+dashboard-block-create` | 先定位 dashboard、表和字段，再读 [Dashboard Block 配置](lark-base-dashboard-block-config.md) 构造 `data_config` |
| 在仪表盘里展示数据明细、甘特图或画册 | `+dashboard-block-create --type view`；更新/获取用组件命令 | [视图组件](lark-base-dashboard-view.md)，筛选、分组、排序、可见字段四类配置直接放在 `data_config` |
| 修改组件 | `+dashboard-block-update` | 先读 block 现状，再读 [Dashboard Block 配置](lark-base-dashboard-block-config.md) 决定替换哪些顶层 key |
| 删除组件 | `+dashboard-block-delete` | 先定位仪表盘和目标组件 |
| 新建组件时带上规划好的坐标 | `+dashboard-block-create --position` | 先读 [仪表盘布局与视觉规范](lark-base-dashboard-visual-standards.md) 规划坐标，再按本页下方「仪表盘布局工作流」操作 |
| 查看仪表盘有哪些组件 | `+dashboard-get` 或 `+dashboard-block-list` | 本页下方「查看仪表盘」；**要读组件坐标只能用 `+dashboard-block-list`** |
| 读取图表计算结果 | `+dashboard-block-get-data` | 返回图表最终数据协议；需要 block 元数据先用 `+dashboard-block-get` |
| 布局收尾：提交全部组件坐标 / 重排 / 美化 | `+dashboard-layout`，**每次新建、挪动、改大小后最后一定调用；只删除组件时不调** | 涉及布局必须先读 [仪表盘布局与视觉规范](lark-base-dashboard-visual-standards.md)；命令用法见本页下方「仪表盘布局工作流」 |
| 配置、替换或清除同源图表联动 | `+dashboard-block-update --linkage`；Get 回读 | [图表联动](lark-base-dashboard-linkage.md)，提交源组件的完整目标列表 |
| 查询或设置仪表盘跟随访问者权限统计开关 | 按目标环境已发布的接口调用 | [仪表盘跟随访问者权限统计开关](lark-base-dashboard-advperm.md) |
| 读取、设置或清空仪表盘全局筛选 | 按目标环境已发布的接口调用 | [仪表盘全局筛选](lark-base-dashboard-global-filter.md) |
| 按钮组件绑定、解绑或查询工作流 | 专用 `button_rule` GET/PUT；完整定义复用 `+workflow-get` | [仪表盘按钮绑定工作流](lark-base-dashboard-button-rule.md) |
| 读取视图组件的记录明细 | `+dashboard-block-get` 取得 `tableId / viewId`，再用 `+record-list --view-id` | [视图组件数据读取](lark-base-dashboard-view.md#读取视图组件数据)；继承视图配置并按需分页 |

## 仪表盘管理

使用 `+dashboard-list` 定位仪表盘，`+dashboard-get` 读取详情；`+dashboard-create` / `+dashboard-delete` 创建或删除仪表盘，`+dashboard-update` 修改名称或主题。

使用 `+dashboard-share-get` / `+dashboard-share-update` 管理分享启停、访问范围和返回源 Base 入口；更新前先读取现状，每次只修改一个字段，显式 `false` 会被保留。

## 精确布局：48 列坐标

全链路统一 **48 列栅格**，你不需要做任何坐标换算。`{"x","y","w","h"}`：`x`/`y` 为左上角坐标（>=0），`w` 为宽度（`1<=w<=48` 且 `x+w<=48`），`h` 为高度（>=1）。坐标一律按 48 列写，不做任何换算。

CLI 写坐标时（包括不传坐标、由服务端自动装箱时）统一以 48 列与下游通信，读回时把服务端返回的像素坐标换算成 48 列，所以 `+dashboard-block-list` / `+dashboard-block-get` **读到的 `position` 和写出去的坐标始终是同一套栅格**，不需要也不应该自己做任何换算。服务端暂未返回像素坐标时，CLI 会按服务端的取整坐标折算，并在 stderr 说明精度有损（每个坐标可能与实际相差最多 2 格）。`+dashboard-block-create` / `+dashboard-block-update` 回包里的 `position` 同样已换算成 48 列，与你提交的坐标一致，不是服务端改了你的坐标。

## 仪表盘布局工作流（唯一流程）

> [!IMPORTANT]
> **只要本次操作涉及布局——新建组件、追加组件、调整位置或大小、重排、美化——在规划任何坐标、调用 `--position` 或 `+dashboard-layout` 之前，必须先完整读取 [仪表盘布局与视觉规范](lark-base-dashboard-visual-standards.md)，按其中的布局模式、排布原则、组件尺寸和硬约束规划全部组件坐标。未读该文件不要规划坐标。** 只删除组件不涉及布局，不需要读。

仪表盘布局只有**一条流程**，不区分新建还是追加、整盘还是局部，五步都不能省：

1. **读现有组件**：`+dashboard-block-list --page-size 100`（`has_more=true` 时按 `page_token` 读完）。读到的 `position` 已是 48 列坐标，直接照着规划；`+dashboard-get` 不返回坐标，不能拿来排版。本轮新建的仪表盘没有组件，跳过这一步。
2. **完整读取 [仪表盘布局与视觉规范](lark-base-dashboard-visual-standards.md)**：布局模式、排布原则与骨架、组件尺寸、硬约束和自检清单都只在那份文件里，本页不重复。
3. **先规划好全部组件的坐标**：为盘上**所有**组件（现有的 + 本轮要新建的）定好 `x/y/w/h`。按场景决定能动哪些组件（详见视觉规范开头的「适用范围」）：
   - **新建看板且用户没提布局要求**（或用户要求整盘重排、美化）：按视觉规范整盘规划，并按其自检清单核对。
   - **用户明确给了坐标、位置、顺序或同一行等要求**：按用户的来，给了顺序就按先行后列的阅读顺序排列；不强制分栏。
   - **在已有看板上追加组件**：现有组件保持原坐标，不强制改成分栏；新组件放进现有空位或接在现有组件下方，只为让新组件所在的行铺满而调整同一行的组件。**最底下是表格时，新组件插在表格上方**，不要接在表格下面：优先并入表格上方的最后一行（收窄该行组件让它铺满），容不下时在表格上方单独占一行、表格整体下移。
   - **定点修改（移动、调整大小、放进某个空位）**：只改目标组件，其他组件按读到的坐标原样提交。

   「非目标组件未经确认不得更新」管的是 data_config、名称和删除；坐标能动哪些组件按上面的场景决定。
4. **新建组件时带坐标**：按第 3 步的规划，用 `+dashboard-block-create --position '{"x":..,"y":..,"w":..,"h":..}'` 逐个串行创建。
5. **最后一定调用 `+dashboard-layout`**，`--layout` 传**盘上全部组件**的坐标——包括没动的现有组件和第 4 步刚建的组件。

`+dashboard-layout` 依次做：本地校验每项坐标的范围 → 读现状 → 校验 `--layout` 覆盖了盘上全部组件 → 只把坐标变了的组件一次批量写入（全都没变就不写）→ **每次都**调用服务端兜底整理 → 输出整盘最终坐标。

**兜底整理会挪动组件**：无论这次写没写坐标，命令最后都会让服务端按前端布局规则整理一遍：填补盘上**所有**空洞（包括原本就有、你没动过的空洞），处理重叠和越界。为此它可能挪动或拉宽你没碰过的组件；它不替你决定盘面结构，也不会居中或对齐。应对方式：

- 想让某些组件保持原位，就提交一个本身没有空洞、没有重叠的布局。定点移动一个组件后，它原来的位置会空出来，整理会把下方的组件上移补位；这是命令的固定行为，其他组件仍按原坐标提交，不要自己抢先去挪。
- 提交后以 stdout 的 `layout` 为最终坐标；`tidied_block_ids` 非空时，stderr 会逐个说明哪个组件从哪移到了哪。不要为了撤销整理结果重复提交，整理每次都会再跑一遍。
- 向用户汇报布局时按最终坐标说；整理挪动了用户要求保持不动的组件时，如实说明。

```bash
lark-cli base +dashboard-layout \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --layout '[{"block_id":"blk_a","x":0,"y":0,"w":16,"h":8},{"block_id":"blk_b","x":0,"y":8,"w":16,"h":8},{"block_id":"blk_c","x":16,"y":0,"w":32,"h":16}]'
```

> [!IMPORTANT]
> - **`--layout` 必须覆盖盘上全部组件**：漏了组件、写了盘上没有的 `block_id`、同一个 `block_id` 出现两次，都会直接报错并列出具体 `block_id`，一格都不写。漏交时按报错把缺的组件按 `+dashboard-block-list` 读到的坐标补进去再提交。
> - `--layout` 只接受坐标数组 `[{"block_id","x","y","w","h"}]`，四个坐标键齐全且为整数；支持 `@file` 和 `-`（stdin）。每项坐标在本地校验范围：`x>=0`、`y>=0`、`w>=1`、`h>=1` 且 `x+w<=48`，不满足就直接报错并列出该项，一格都不写。组件之间的空洞和重叠不在本地校验，提交前自己核对：任何场景都不能重叠；新建看板还要按视觉规范的自检清单核对无空洞。
> - **即使每个组件都已用 `--position` 建好、看起来不需要再动，也要调这一步**：兜底整理只在这一步执行。整盘只有 1 个组件时同样如此。**唯一例外是只删除组件**：删完不调，见场景 3「删除组件」。
> - **布局改动不要绕开 `+dashboard-layout`**：不要逐个组件用 `+dashboard-block-update --position`（包括加 `--no-validate`）写坐标来代替它；`+dashboard-layout` 收尾之后也不要再单独改坐标，需要挪动时改好全部坐标再调一次 `+dashboard-layout`。
> - 兜底整理失败时命令会报错，但前面的批量写入已经生效，stderr 会写明哪些组件已写入；按提示用同一份 `--layout` 重新调用即可，坐标已一致的组件不会重复写入。
> - `--position` 的四个 key 必须齐全且都是数字：`position` 按整体提交、不做逐字段合并，只传 `{"x":6}` 不是"只挪位置不改大小"，而是一个缺了三项的位置，本地会拒绝（含显式 `null`）。不传 `--position` 时 create 由服务端自动放置、update 保持当前布局；无论传没传，最后都用 `+dashboard-layout` 收尾。
> - 布局本身就是你的活：用户说"调整布局""美化""撑满""铺满"时，按视觉规范自己规划每个组件的 `x/y/w/h`；不要因为用户没给坐标就放弃规划。
> - 命令成功即视为写入成功，不需要为读回位置再调 `+dashboard-block-get` / `+dashboard-block-list`：`+dashboard-layout` 的 stdout 已给出最终坐标。

`+dashboard-layout` 的 stdout：

| 字段 | 含义 |
|---|---|
| `written_block_ids` | 实际写入的组件（坐标与现状不同的那些） |
| `tidied_block_ids` | 被兜底整理挪动过的组件，可能包括你没改的；stderr 逐个说明从哪移到哪 |
| `layout` | 兜底整理后整盘的最终坐标（48 列），以它为准 |
| `grid_cols` | 坐标口径，恒为 48 |

`+dashboard-block-create` 每次成功后、带 `--position` 的 `+dashboard-block-update` 成功后，stderr 会提示最后用 `+dashboard-layout` 提交全部组件坐标收尾；没有这条提示不代表可以省掉收尾。

## statistics 指标卡数值格式

`statistics` 组件可在 create/update 的 `data_config.number_format` 中设置 `formatName` 和 `precision`。create 会校验组件类型和子字段；update 不接收 `--type`，只校验 `number_format` 子字段，再由服务端结合现有 block 类型裁决。枚举、精度范围、更新语义和可复制模板读取 [Dashboard Block 配置](lark-base-dashboard-block-config.md)。

## 典型场景工作流

### 场景 1：从 0 到 1 创建仪表盘

从 0 到 1 创建仪表盘时，按用户需求规划组件的类型和数量，并注意以下要点：

- 聚合方式：创建指标卡或分布图时优先把聚合写进 `data_config`，只有 Top N、字段取值探索、复杂筛选校验或 helper 汇总表场景才先用 `+data-query`。
- Dry-run 边界：已按模板构造的简单指标卡、分布图、趋势图不需要逐个 `--dry-run` 后再真实创建；只有在调试 JSON、检查请求体、复杂自造 `data_config` 或处理 API validation 错误时才 dry-run。
- 验证方式：创建接口成功返回即表示写入成功。只有结果不确定时才用一次 `+dashboard-get` 或 `+dashboard-block-list` 确认仪表盘和组件存在；不要仅为确认创建而逐组件调用 `+dashboard-block-get-data`。
- 布局方式：**在创建第一个组件之前**，先完整读取 [仪表盘布局与视觉规范](lark-base-dashboard-visual-standards.md)，按它规划好全部组件的 `x/y/w/h`；创建每个组件时用 `--position` 带上规划好的坐标，全部建完后**一定**调用一次 `+dashboard-layout`，把全部组件的坐标交上去收尾。用户给了明确坐标就用用户的，没给就自己规划，不要跳过规划。

示例：搭建一个销售数据分析仪表盘

```bash
# 第 1 步：创建空白仪表盘
lark-cli base +dashboard-create --base-token xxx --name "销售数据分析"
# 记录返回的 dashboard_id

# 第 2 步：获取数据源信息
lark-cli base +table-list --base-token xxx
lark-cli base +field-list --base-token xxx --table-id <table_id>

# 第 3 步：规划组件 + 规划全部组件的坐标（两件事一起做完，不要建到一半再想布局）
# 例如：总销售额（指标卡）、月度趋势（折线图）、负责人 Top N（排行榜）、品类占比（饼图）、渠道对比（柱状图）
# 先完整读 lark-base-dashboard-visual-standards.md，再按它定下每个组件的 x/y/w/h（48 列栅格）
# 下面的坐标示例：左侧边栏区块放指标卡和排行榜，右侧主区块放趋势图和两张对比图

# 第 4 步：顺序创建每个组件（必须串行执行，不能并发）
# 重要：创建组件前，先确定 dashboard_id、组件 name/type 和真实表字段
# 再阅读 lark-base-dashboard-block-config.md 了解 data_config 结构、组件类型和 filter 规则

# 第 1 个组件
lark-cli base +dashboard-block-create \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --name "总销售额" \
  --type statistics \
  --data-config '{"table_name":"订单表","series":[{"field_name":"金额","rollup":"SUM"}]}' \
  --position '{"x":0,"y":0,"w":12,"h":8}'

# 第 2 个组件（等上一个完成后再执行）
lark-cli base +dashboard-block-create \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --name "月度趋势" \
  --type line \
  --data-config '{"table_name":"订单表","series":[{"field_name":"金额","rollup":"SUM"}],"group_by":[{"field_name":"月份","mode":"integrated"}]}' \
  --position '{"x":12,"y":0,"w":36,"h":16}'

# 继续创建其他组件（每个都带上第 3 步规划的 --position）：品类占比饼图 (12,16,18,16)、渠道对比柱状图 (30,16,18,16)

# 排行榜组件：省略 limit_size 和 sort 时分别默认 10、value desc
lark-cli base +dashboard-block-create \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --name "负责人销售额 Top 10" \
  --type ranking \
  --data-config '{"table_name":"订单表","series":[{"field_name":"金额","rollup":"SUM"}],"group_by":[{"field_name":"负责人"}]}' \
  --position '{"x":0,"y":8,"w":12,"h":24}'

# 第 5 步：组件全部创建完成后，一定调用 +dashboard-layout，--layout 传全部组件的坐标
# （block_id 取第 4 步各次创建的返回值；坐标与第 3 步的规划一致）
# 命令只写坐标变了的组件，stdout 的 layout 是最终坐标
lark-cli base +dashboard-layout \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --layout '[{"block_id":"<总销售额>","x":0,"y":0,"w":12,"h":8},{"block_id":"<负责人销售额 Top 10>","x":0,"y":8,"w":12,"h":24},{"block_id":"<月度趋势>","x":12,"y":0,"w":36,"h":16},{"block_id":"<品类占比>","x":12,"y":16,"w":18,"h":16},{"block_id":"<渠道对比>","x":30,"y":16,"w":18,"h":16}]'
```

### 场景 2：在已有仪表盘上添加新组件

与新建走同一条流程：先读现有组件，**完整读取 [仪表盘布局与视觉规范](lark-base-dashboard-visual-standards.md)**，按其中的规则为**盘上全部组件**规划坐标，新组件创建时带 `--position`，最后用 `+dashboard-layout` 提交**全部组件**的坐标。

> [!IMPORTANT]
> - 规划前必须用 `+dashboard-block-list` 读到现有组件占了哪些格子；`+dashboard-get` 不返回坐标。
> - 追加不强制改成分栏：现有组件保持原坐标，新组件放进现有空位或接在现有组件下方，只为让新组件所在的行铺满而调整同一行的组件；用户指定了位置、顺序或同一行时按用户的来。
> - **最底下是表格（`view` / `pivotTable`）时，新组件插在表格上方**，不要接在表格下面：优先并入表格上方的最后一行，按列基线收窄该行组件让它铺满；容不下时新组件在表格上方单独占一行（占用表格原来的 `y`，表格的 `y` 加上这一行的高度），这一行只有它一个组件时可以占满 `w=48`。
> - 最后一步的 `--layout` 要带上**盘上每一个组件**，没动的现有组件按读到的坐标原样写进去；漏交会报错并列出缺的 `block_id`。

```bash
# 第 1 步：列出仪表盘，定位到当前仪表盘
lark-cli base +dashboard-list --base-token xxx
# 获取目标 dashboard_id

# 第 2 步：读现有组件（必须用 block-list：+dashboard-get 不返回坐标）
# 既用于避免重复创建，也用于确认现有组件占了哪些格子
lark-cli base +dashboard-block-list --base-token xxx --dashboard-id blk_xxx --page-size 100
# 例如读到：blk_a (0,0,24,16)、blk_b (24,0,24,16)、blk_c (0,16,24,16)——最后一行右侧空着

# 第 3 步：获取数据源信息
lark-cli base +table-list --base-token xxx
lark-cli base +field-list --base-token xxx --table-id <table_id>

# 第 4 步：规划全部组件的坐标，再顺序创建每个新组件（必须串行执行，不能并发）
# 例如新建一个柱状图补在 blk_c 右侧 (24,16,24,16)
# 重要：先确定 dashboard_id、组件 name/type 和真实表字段
# 再阅读 lark-base-dashboard-block-config.md 了解 data_config 结构
lark-cli base +dashboard-block-create \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --name "新组件名" \
  --type column \
  --data-config '{...}' \
  --position '{"x":24,"y":16,"w":24,"h":16}'

# 第 5 步：一定调用 +dashboard-layout，--layout 传盘上全部组件的坐标（含没动的 blk_a/blk_b/blk_c）
lark-cli base +dashboard-layout \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --layout '[{"block_id":"blk_a","x":0,"y":0,"w":24,"h":16},{"block_id":"blk_b","x":24,"y":0,"w":24,"h":16},{"block_id":"blk_c","x":0,"y":16,"w":24,"h":16},{"block_id":"<第 4 步新建的>","x":24,"y":16,"w":24,"h":16}]'
```

### 场景 3：编辑或删除已有组件

> [!IMPORTANT]
> `+dashboard-block-update` **不能修改组件的 `type`**（图表类型），可以更新 `name`、`data_config`、`linkage` 和可选的 `position`。
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
  --data-config '{...}'
# 改位置或大小属于布局改动（定点修改）：先完整读 lark-base-dashboard-visual-standards.md，只改目标组件的坐标，
# 其他组件按 +dashboard-block-list 读到的坐标原样写进 --layout，用 +dashboard-layout 一次提交收尾；
# 不要在这里逐个用 --position 改坐标来代替 +dashboard-layout

# 排行榜只修改 Top N；不会覆盖分组、指标、筛选或排序
lark-cli base +dashboard-block-update \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --block-id chtxxxxxxxx \
  --data-config '{"limit_size":20}'

```

#### 删除组件

```bash
# 删除前先确认 block_id，删除是高风险写操作，需要 --yes
lark-cli base +dashboard-block-list --base-token xxx --dashboard-id blk_xxx --page-size 100
lark-cli base +dashboard-block-delete \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --block-id chtxxxxxxxx \
  --yes
```

> [!IMPORTANT]
> **删完不要调用 `+dashboard-layout`，也不要挪动其余组件。** 只删除组件不算布局改动。删除会在盘上留下空位，这是正常的——用户要的是删掉一个组件，不是重新排版，擅自重排会把他们其余组件的位置全部改掉。
>
> 只有用户明确说了"删掉 X 并把布局整理一下"，才按场景 4 重排。

### 场景 4：重排仪表盘布局

当用户要求调整布局、重排、美化、撑满或铺满时使用。流程与新建、追加相同：读全部现有组件 → **完整读取 [仪表盘布局与视觉规范](lark-base-dashboard-visual-standards.md)**，按 query 和其中的规则为全部组件重新规划坐标（位置和宽高都可以改）→ 用 `+dashboard-layout` 一次提交全部组件的坐标。用户没给坐标不等于不用规划。

> [!CAUTION]
> - 先收齐现有组件的 `block_id` 与 `type`，再规划坐标；不要凭印象猜 `block_id`
> - `--layout` 要带上盘上每一个组件；漏交、多交或重复都会报错并列出具体 `block_id`，一格都不写
> - 提交前按视觉规范的自检清单核对：无空洞、无重叠、不越界
> - 不要为了凑效果去探测 raw `lark-cli api`、源码或未公开布局参数

```bash
# 第 1 步：列出仪表盘，定位到目标仪表盘
lark-cli base +dashboard-list --base-token xxx

# 第 2 步：收齐现有组件（分页读完）
lark-cli base +dashboard-block-list --base-token xxx --dashboard-id blk_xxx --page-size 100

# 第 3 步：为全部组件规划 48 列坐标后一次提交
lark-cli base +dashboard-layout \
  --base-token xxx \
  --dashboard-id blk_xxx \
  --layout '[{"block_id":"blk_a","x":0,"y":0,"w":16,"h":8},{"block_id":"blk_b","x":0,"y":8,"w":16,"h":8},{"block_id":"blk_c","x":16,"y":0,"w":32,"h":16}]'
```

### 场景 5：读取仪表盘或组件现状

**选择查询方式：**
- 想看仪表盘整体结构（含主题、所有组件名称和类型）→ 用 **方式 A**
- 只想快速查看有哪些组件 → 用 **方式 B**
- **要读组件的坐标（`x/y/w/h`）→ 只能用方式 B**
- 想看某个组件的详细 data_config 配置 → 用 **方式 C**
- 想看某个图表/指标卡实际算出来的数据 → 用 **方式 D**
- 想看视图组件中的记录 → 用 **方式 C** 取得 `tableId / viewId`，按 [视图组件数据读取](lark-base-dashboard-view.md#读取视图组件数据) 调用 `+record-list --view-id`

用户要求读取“全部图表”或“完整仪表盘”时，先用方式 B 分页枚举所有 block：使用 `--page-size 100`；若返回 `has_more=true`，继续把本页返回的 `page_token` 传给 `--page-token`，直到 `has_more=false`。收齐后再对每个 block 收口，不能只返回 get-data 成功的子集：

1. 图表或指标卡：使用方式 D 读取计算结果。
2. `view`：使用方式 C 取得真实视图引用，再用 `+record-list --view-id` 读取记录，继承视图配置并按需分页。
3. `text`：使用方式 C，正文位于 `data_config.text`；text 没有计算结果，但属于完整仪表盘内容。
4. get-data 返回不支持的图表类型：先用方式 C 读取真实 `data_config`，确认 `table_name`、维度、指标、聚合与筛选，再按 [记录分析](../SKILL.md#记录分析) 使用 `+data-query` 重建同口径结果。字段必须来自真实配置和表结构，不得猜测；无法等价重建时明确报告限制，不能静默省略该 block。

```bash
# 第 1 步：列出仪表盘，定位到当前仪表盘
lark-cli base +dashboard-list --base-token xxx

# 第 2 步：根据用户诉求查看详情

# 方式 A：查看仪表盘整体情况（主题 + 组件清单）
# 注意：返回的组件清单只有 block_id / 名称 / 类型，不含坐标
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

> [!IMPORTANT]
> **组件坐标只能从方式 B（`+dashboard-block-list`）读。** `+dashboard-get` 只返回组件清单（`block_id`、名称、类型），**不含 `x/y/w/h`**。
> 规划布局前必须走方式 B：拿 `+dashboard-get` 的结果去排版，等于不知道已有组件在哪，只能凭空猜位置，排出来必然与现有组件重叠。

需要读取多个组件的数据时，先用方式 B 获取真实 `block_id` 和类型（使用 `--page-size 100`；若 `has_more=true`，继续把返回的 `page_token` 传给 `--page-token`，直到 `has_more=false`），再按 [lark-base-dashboard-block-get-data.md](lark-base-dashboard-block-get-data.md) 的多组件范式分流：图表读取计算结果，视图组件读取记录，文本组件读取 `data_config.text`。同类读取可在一个 shell 工具调用内串行执行。

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
A: 外层组件 `type` 不能修改。`+dashboard-block-update` 可以修改 `name`、`data_config`、`linkage` 和 `position`；倒计时内部的 `data_config.type` 可以在 fixed / field 之间切换，但必须提交完整目标模式配置。

**Q: 更新组件的命令和 data_config 怎么写？**
A:
1. 先读取当前 block，确认 `block_id`、当前 `type` 和已有 `data_config`
2. 再读 [Dashboard Block 配置](lark-base-dashboard-block-config.md) 了解 data_config 结构

**普通图表 data_config 更新策略（顶层 key merge）**：
- 只传入需要修改的顶层字段（如 `series`、`filter`）
- 未传的顶层字段（如 `group_by`）自动保留原值
- 但每个传入的字段内部通常是**全量替换**（如传新 `filter` 会完整覆盖旧 `filter`）；`number_format` 例外，按子字段合并，见 [Dashboard Block 配置](lark-base-dashboard-block-config.md) 的 number_format 小节

**倒计时**使用 camelCase 的 fixed/field 专属配置：更新时省略 type 沿用当前模式，唯一 additionalDatasources 对象按字段合并，groupBy/series 数组整体替换。同模式同表下 filter 省略保留、对象替换、null 清空。切换模式要求完整目标配置，换表要求完整新数据源；获取和写入响应均返回当前模式完整配置。构造 JSON 前读 [Dashboard Block 配置](lark-base-dashboard-block-config.md) 的 countdown 小节。

**Q: 查看已有组件有什么用？**
A: 在「添加新组件」或「编辑组件」前查看已有组件可以：
- 了解当前仪表盘已有哪些可视化
- 避免重复创建相似的组件
- 参考已有组件的 data_config 结构作为模板

**Q: 我想直接拿图表算好的结果给 AI 分析，应该用什么？**
A: 用 `+dashboard-block-get-data`。它返回图表协议 JSON（常见字段包括 `dimensions`、`measures`、`main_data`，指标卡可能还有 `comparison_data`、`trend_data`），不返回 block 名称、类型、布局或 `data_config`；需要这些元数据时先用 `+dashboard-block-get`。

图表联动使用独立顶层 `linkage`，更新按完整目标列表替换；省略保留、空数组清除。操作前读 [图表联动](lark-base-dashboard-linkage.md)。只改联动时验收保存的目标列表，要求点击效果时再做页面交互验证；`get-data` 不模拟点击。

## 写入前检查

- 创建 block 前必须知道 `base_token`、`dashboard_id`、组件 `name/type` 和 `data_config`。
- 更新 block 前必须知道 `base_token`、`dashboard_id`、`block_id`，并读过当前 block。
- `data_config` 中使用表名和字段名，不使用 table_id / field_id；名称必须来自 `+table-list` / `+field-list` 的真实返回。
