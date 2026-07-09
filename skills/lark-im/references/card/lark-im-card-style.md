# Card Style Guide

选择组件组合和视觉样式的决策指南。字段写法见 `card-2.0-schema.md`。

---

## 好看的标准（P0–P7，唯一裁判基准）

**先读这一节。** 下面的「意图→组件」表和「视觉规范」都是为这套标准服务的手段；构造和自检卡片时**以 P0–P7 为准**。

**目标函数**：一张好卡片 = 让收件人在**约 2 秒一瞥**内 get 到「这是什么 + 最重要的是什么 + 要不要操作」，且观感**有序、克制、不嘈杂**。高效传达与视觉舒适在此统一。

**用力分配**：P0 必过（前置闸）→ P1–P3 强约束（阻断）→ P4–P5 基础卫生 → P6–P7 加分。

每条都附**结构化验证句**——卡片不能渲染成图，只能对 JSON 结构推理，所以验证靠「数结构」而非「眯眼看」。

| | 准则 | 可操作要求 | 结构化验证（自检句） |
|---|---|---|---|
| **P0** | **符合诉求**（前置闸·阻断） | 精确承载用户要的信息/意图/操作，不缺、不多、不跑题；意图类型与组件组合匹配 | 把诉求拆成信息点清单，逐点在 JSON 里找到承载组件；操作诉求逐个找到交互组件。有缺=不过 |
| **P1** | **层级**（强约束·阻断） | header 承载「这是什么」；body 内**有且仅有一个**最强焦点（最大字号/最重色/指标卡大数字），其余为支撑；标题用 `**加粗**`、次要信息用 grey | 列出所有文本的「字号+粗细+颜色」三元组，能否排出主>次>辅三层；焦点是否唯一 |
| **P2** | **分组**（强约束·阻断） | 同主题字段收进同一容器（`column_set`/`interactive_container`/背景块），不同主题分容器；块边界靠容器底色/描边/间距，**而非一路 `hr` 平铺** | 数顶层视觉块个数；是否存在「多主题挤在同一无分隔 markdown / 一路 hr 平铺」反模式 |
| **P3** | **复杂度适中**（强约束·阻断·双边带） | 下限：不得纯文字流水账，至少有分块+层级+适度色彩/图标；上限：视觉块 2–5、主色系 ≤3、组件不堆砌、焦点唯一 | ①是否 >1 个视觉块且含≥1 个非纯文本结构元素（背景块/指标卡/图标/表格）；②块数 ≤5、主色系 ≤3。两端都满足才过 |
| **P4** | **对比**（基础卫生） | 标题与正文字号或粗细至少差一档；强调用色/放大；正文不滥用 `#/##/###`（数值焦点放大除外，见 P1） | 标题与正文是否在「字号或粗细」上至少差一档 |
| **P5** | **对齐**（基础卫生） | 间距优先交容器 `vertical_spacing`/`horizontal_spacing`/`padding`，**不滥用散设 margin 造成疏密无规律**；间距值收敛到一套档位（2/4/8/12px）；顶层容器间距一致 | 是否存在无规律的散落 margin；间距取值种类是否 ≤4 |
| **P6** | **语义一致**（加分） | 红=降/警/失败、绿=升/成/通过、grey=次要；主色系起始色由 header 决定、取邻近色环；同色同义 | 同一颜色是否对应同一语义；header 模板色与块色是否同色系 |
| **P7** | **健壮**（加分） | 并列/指标列默认 `weighted` 或 `none`、**慎用 `stretch`**（防移动端拉伸）；需要时配 `config.style.color` 的 light/dark；不靠固定像素宽硬排 | 是否存在 stretch 拉伸风险；深浅色是否都可读 |

---

## 意图 → 组件组合

### 通知类（无交互或只读）

| 用户意图 | 推荐组件组合 | header.template |
|---|---|---|
| 纯文字通知 / 系统公告 | `column_set`（通知正文，带 `blue-50` 背景）+ `button(open_url)` | `blue` |
| 活动公告（带主视觉图） | `img`（主图）+ `markdown`（时间/地点）+ `column_set`（详情对）+ `button(open_url)` | `turquoise` / `blue` |
| 成功 / 完成状态通知 | `column_set`（关键字段，带 `green-50` 背景）+ `markdown`（结论加粗） | `green` |
| 审批结果反馈（已通过 / 已拒绝） | `column_set`（申请信息）+ `column_set`（审批结论 + icon，带 `green-50`/`red-50` 背景） | `green` / `red` |
| 生日 / 节日祝福 | `img`（主图）+ `column_set`（人名/日期）+ `button(open_url)` | `orange` |
| 产品 / 功能上线推广 | `img`（主图）+ `markdown`（亮点）+ `column_set`（功能高亮块）+ `button(open_url)` | `blue` / `violet` |
| 多图展示（图集、AI 生成图） | `img_combination` 或 多个 `img` + `markdown`（说明）+ `button(callback)` | `default` |

### 提醒 + 操作类

| 用户意图 | 推荐组件组合 | header.template |
|---|---|---|
| 提醒 + 一键操作 | `column_set`（详情，带 `yellow-50` 背景）+ `button(callback)` | `yellow` |
| 任务清单 / 待办跟踪 | `checker` × N（每项带 `behaviors: callback`）+ `button(callback)`（全部完成操作） | `blue` |
| 告警触发（需立即处理） | `column_set`（告警指标，带 `red-50` 背景）+ `column_set`（描述 + input 快速备注）+ `button(callback)` | `red` |
| 告警已解决 / 状态变更 | `column_set`（解决时间 / 负责人，带 `green-50` 背景）+ `markdown`（结论加粗） | `green` |
| 审批待处理（含备注输入） | `column_set`（申请信息，带 `grey-50` 背景）+ `column_set`（input 审批意见）+ `button(callback)` × 2（通过 / 拒绝） | `default` |
| 日历 / 日程提醒（含参与人） | `column_set`（时间 / 地点，带 `yellow-50` 背景）+ `person_list`（参与人）+ `button(callback)` | `yellow` |
| 危险操作确认 | `column_set`（说明，带 `red-50` 背景）+ `button(callback)` + `confirm` 弹窗配置 | `red` |

### 数据 / 报告类

| 用户意图 | 推荐组件组合 | header.template |
|---|---|---|
| 日报 / 工作汇报 | `column_set`（指标，带背景色）+ `interactive_container`（进展分块，带描边）× N；内容过长的块用 `collapsible_panel` 折叠次要细节 | `blue` / `default` |
| 数据看板（含图表） | `column_set`（指标，带 `blue-50` 背景）+ `chart` + `table`（根节点，不可嵌套）+ `markdown`（说明） | `blue` |
| 排行榜 | `column_set` 固定列宽（序号 + 头像 `img` + 名字 + 指标）循环条目 | `grey` |
| 订单 / 工单详情 | `div.fields`（字段对）或 `column_set`（需彩色背景块时）+ `button(callback)` | `orange` |

### 表单 / 收集类

| 用户意图 | 推荐组件组合 | header.template |
|---|---|---|
| 纯文字表单收集 | `form`（内含 `input` + `button(form_action_type: submit)`） | `blue` |
| 带下拉选择的表单（单选） | `form`（内含 `select_static` / `select_person` + `input` + `button`） | `wathet` |
| 带多选的表单 | `form`（内含 `multi_select_static` / `multi_select_person` + `input` + `button`） | `wathet` |
| 含日期 / 时间的表单 | `form`（内含 `date_picker` / `picker_time` / `picker_datetime` + `input` + `button`） | `blue` |
| 设备 / 服务反馈 | `form`（内含 `select_static`（满意度）+ `input`（备注）+ `button`） | `yellow` |
| 多步骤进度 / 引导 | `column_set`（横向步骤，带 `blue-50` 背景）+ `markdown`（当前状态）+ `button` | `blue` |

### 推荐 / 选择类

| 用户意图 | 推荐组件组合 | header.template |
|---|---|---|
| 推荐列表（带图卡片，可点击） | `interactive_container`（内含 `img` + `markdown`）× N + `button(open_url)` | `blue` |
| AI 引导选项 / 功能菜单 | `markdown`（欢迎语）+ `interactive_container`（内含 `markdown` 选项说明）× N | 无 header |
| Bot 功能引导 / 教程 | `column_set`（步骤说明，带背景）+ `button` × 2（主操作 / 次操作） | `blue` |
| 服务台 / 多操作入口 | `column_set`（说明，带背景）+ `button` × N（≤3 个主操作，`type` 区分主次）；次要操作超过 3 个时改用 `overflow`（折叠菜单） | 无 header |

### 社交 / 互动类

| 用户意图 | 推荐组件组合 | header.template |
|---|---|---|
| 工作圈 / 社交分享 | `img_combination`（多图）+ `markdown`（正文）+ `button(open_url)` × 2 | `blue` |
| 成交 / 业绩公告 | `img`（庆祝图）+ `markdown`（成绩）+ `column_set`（关键数字） | `green` |

---

## 视觉规范（实现 P0–P7 的具体战术）

组件选型只解决「有没有」，下面各条是落地上面 P0–P7 的具体手段，括号标注它主要服务的原则。

> **P3 特例 — 数据看板类**：`chart + table + column_set + markdown` 是四种不同组件各出现一次，不算「堆砌」，P3 上限照常满足；但仍须保证每类只出现一次。

### 0. Header 图标（服务 P3 · 视觉质感底线）

**几乎所有卡片都应配 header icon**——这是提升「精致感」成本最低的一步，缺失会让 header 显得空洞、平价。

```json
"header": {
  "title": { "tag": "plain_text", "content": "卡片标题" },
  "template": "blue",
  "icon": { "tag": "standard_icon", "token": "mail_colorful" }
}
```

- `token` 从 `resource/icons.md` 按场景选取；彩色图标用 `*_colorful` 后缀，单色用普通名称。
- 常用速查：通知 `notice_colorful`、告警 `warning_colorful`、审批 `approve_colorful`、日历 `calendar_colorful`、数据 `chart_colorful`、任务 `todo_colorful`、AI `myai_colorful`。

### 1. 配色纪律（服务 P6 语义一致）

- **邻近色环**：`Red → Carmine → Orange → Yellow → Green → Turquoise → Wathet → Blue → Violet → Purple →（回到）Red`。一张卡只能取色环上**相邻**的颜色，严禁跳跃（❌ blue + green + red）。
- **最多 3 种主色系**（不含 grey / white）。
- **起始色由 header 决定**：
  - header `blue` → blue / violet / purple
  - header `green` → green / turquoise / wathet
  - header `red` → red / carmine / orange
  - 无 header → 默认 blue / violet / purple
- **深浅语义**（写法 `blue-50`、`blue-600`、`grey-500`）：
  - `-50` 区块背景 · `-100` 标签背景 · `-500` 正文文字 · `-600`/`-700` 强调文字

### 2. 间距纪律（服务 P5 对齐 · 视觉决定性因素）

- **body padding 推荐**：`"padding": "12px 12px 20px 12px"`（上右下左；底部 20px 留白更舒适）。
- **优先不用 `markdown` / `column` 的 `margin` 控间距**：交给父容器的 `vertical_spacing` / `horizontal_spacing` / `padding` 统一管理，多数情况显式置 `0px`；仅在需要精细缩进（如层级左缩进）时才设非零值。
- 容器内 `vertical_spacing` 推荐值：`2px`（高亮块内标题↔正文）/ `4px`（正文段落、列表项）/ `8px`（需拉开的元素）。
- **容器间智能 margin**：某个顶级容器若**不是** body 最后一个元素 → 设 `"margin": "0px 0px 12px 0px"`；若**是**最后一个 → `"0px"` 或不设，避免卡片底部多余留白。

### 3. 指标卡模式（服务 P1 焦点 · 出现 KPI / 数值 / 统计词时强制使用）

触发：内容含 `KPI/ROI/CTR/UV/PV/DAU/GMV/转化率/增长率/总数/营收` 等数值类信息。

- 多个指标并列放进一个 `column_set`，`flex_mode` **默认用 `"none"`、慎用 `"stretch"`**（防移动端拉伸变形，P7）；仅在各列内容等宽、确认移动端不变形时才用 stretch。
- 数值：用 `##` 放大（**唯一允许用 markdown 标题的特例**），可配 `<font>` 上色。
- 描述：`<font color='grey'>` + `text_size: "notation"`。
- 居中 `text_align: "center"`；列背景 `background_style: "grey-50"`；`padding: "12px"`；`vertical_spacing: "2px"`。

```json
{
  "tag": "column_set",
  "flex_mode": "none",
  "horizontal_spacing": "12px",
  "columns": [
    { "tag": "column", "width": "weighted", "weight": 1,
      "background_style": "grey-50", "corner_radius": "8px",
      "padding": "12px", "vertical_spacing": "2px",
      "elements": [
        { "tag": "markdown", "content": "## <font color='blue'>5,483</font>", "text_align": "center" },
        { "tag": "markdown", "content": "<font color='grey'>GMV($)</font>", "text_align": "center", "text_size": "notation" }
      ] }
  ]
}
```

### 4. 描边卡片模式（服务 P2 分组 · 进展 / 事项 / 列表项分块展示）

用 `interactive_container` 给每个事项块加描边 + 圆角，视觉上比彩色底色更轻盈，适合进展/工单/任务列表等「多条目」场景。

```json
{
  "tag": "interactive_container",
  "width": "fill",
  "has_border": true,
  "border_color": "blue-100",
  "corner_radius": "8px",
  "background_style": "blue-50",
  "padding": "12px 12px 12px 12px",
  "vertical_spacing": "4px",
  "margin": "0px 0px 12px 0px",
  "elements": [
    {
      "tag": "markdown",
      "content": "**<font color='blue'>事项标题</font>**"
    },
    {
      "tag": "markdown",
      "content": "事项正文内容……",
      "text_size": "normal"
    }
  ]
}
```

- `border_color` 跟随主色系（蓝系用 `blue-100`，绿系用 `green-100`）。
- 不需要交互时可省略 `behaviors`；需要点击回调时加 `"behaviors": [{"type":"callback","value":{...}}]`。
- **不能在内部放 `form` 或 `table`**。

### 5. 高亮块模式（服务 P2 分组 · 多分类信息成块展示）

两层结构：外层 `column_set` 管布局，内层 `column` 管样式（彩色背景）。

- 每个 `column` 设 `background_style` 用浅色（如 `blue-50` / `green-50`），`padding: "12px 12px 12px 12px"`，`vertical_spacing: "4px"`，`weight: 1`。
- 块内首行用 `**<font color='blue'>分类标题</font>**` 着色加粗，正文紧随。
- **布局选择**：分类 ≤ 3 个且内容简短 → 水平，优先用 `flex_mode: "bisect"`（2 列）或 `"trisect"`（3 列）；各列字数严格等宽且已确认移动端不变形时才用 `stretch`（慎用，见 §9）；**分类 ≥ 4 个、奇数、或任一块内容 > 3 行 → 垂直**（每块独占一行）。配色按上面第 1 条邻近色环依次取色。
- ⚠️ **版本依赖**：`column.background_style` 需客户端 **≥ v7.9**，旧版静默丢背景。要求强健壮性时改用 `interactive_container` 的 `background_style`（无版本限制）替代 column 背景色。

### 6. Header 三件套（服务 P1 层级 · 语境补全）

header 有三层能力，**尽量用满**（至少用 `title` + `icon`；`subtitle` 和 `text_tag_list` 按实际诉求取舍）——这是成本最低、语境最清晰的一步：

- `title`：这是什么（必填）
- `subtitle`：一句上下文（谁发 / 什么时间 / 什么状态），≤1 行，`plain_text`
- `text_tag_list`：状态标签，≤3 个，颜色语义与 P6 保持一致（`blue`=信息、`yellow`=待处理、`red`=紧急、`green`=完成）

```json
"header": {
  "title":    { "tag": "plain_text", "content": "发版审批" },
  "subtitle": { "tag": "plain_text", "content": "2026-06-25 · 后端服务" },
  "template": "blue",
  "icon": { "tag": "standard_icon", "token": "approve_colorful" },
  "text_tag_list": [
    { "tag": "text_tag", "text": { "tag": "plain_text", "content": "待审批" }, "color": "yellow" }
  ]
}
```

**禁止**：在 `header.title` 里写 emoji；把 subtitle 信息改塞进 body 第一行 markdown，让 header 空洞；严肃场景（审批/告警/财务）在 title 或 body 标题里用装饰性 emoji。

### 7. 字段对用 `div.fields`，不要用 `column_set` 模拟（服务 P5 对齐）

详情型"label: value"（订单字段、审批信息、日程详情）首选 `div.fields`——原生对齐，结构更轻：

```json
{
  "tag": "div",
  "fields": [
    { "is_short": true, "text": { "tag": "lark_md", "content": "**提交人**\n张三" } },
    { "is_short": true, "text": { "tag": "lark_md", "content": "**部门**\n研发中台" } },
    { "is_short": true, "text": { "tag": "lark_md", "content": "**提交时间**\n2026-06-25 10:30" } },
    { "is_short": true, "text": { "tag": "lark_md", "content": "**优先级**\n<font color='red'>P0</font>" } }
  ]
}
```

`is_short: true` 的字段自动两两并排，对齐由组件保证。`column_set` 留给**需要彩色背景块 / 不等宽 / 嵌套复杂结构**的场景，不要用它模拟简单字段对。

### 8. 长文本必须设 `lines` 截断（服务 P3 复杂度上限）

凡接收动态数据的文本字段，必须设最大行数避免卡片被撑爆：

| 位置 | 字段 | 推荐上限 |
|---|---|---|
| `div.text` | `lines` | 正文 ≤4，次要说明 ≤2 |
| `person_list` | `lines` | ≤2 |
| `table.header_style` | `lines` | ≤1 |
| `collapsible_panel` | 默认折叠 | 长文本优先用折叠面板而非截断 |

不设 `lines` 的动态文本 = P3 上限的隐患。

### 9. `flex_mode` 决策表（服务 P7 健壮）

| 场景 | 推荐 flex_mode | 原因 |
|---|---|---|
| 指标卡并列（内容不等长） | `none` + `width: weighted` | 防移动端拉伸；各列按比例压缩 |
| 2 列等宽内容（字数相近） | `bisect` | 语义最清晰的两等分 |
| 3 列等宽内容 | `trisect` | 三等分，不写 weight |
| 多 tag / 多图标横排，允许换行 | `flow` | 窄屏自动折行，不挤压 |
| 明确要求两端对齐撑满且内容等宽 | `stretch` | 慎用：移动端窄屏内容过长时会拉伸变形 |

> `stretch` 只在各列字数高度相近、且已确认移动端不变形时使用；其余场景默认 `none`。

### 10. `chart` 配色纳入 P6 纪律

`chart.color_theme` 必须与全卡色系保持一致：

- **默认**：`brand`（单色系，跟随飞书品牌色）或 `primary`（主色单色系），安全选项。
- **禁止**：`rainbow`——会把色环上的跳跃色全打进图表，直接击穿 P6 的"主色系 ≤3 + 邻近色环"约束。
- **例外**：数据维度 ≥4 个系列、且各系列无主次关系（如区域对比图）时，可用 `complementary` 或在 `chart_spec` 里自定义与主色系邻近的颜色数组。

### 11. `laser` 样式的克制规则（服务 P6 语义一致）

`button.type: "laser"` 和 `background_style: "laser"` 是高饱和渐变效果：

- **允许**：AI 生成类、节日庆祝类、营销推广类，每卡 **≤1 处**，且位置在主操作按钮或视觉焦点块。
- **禁止**：审批、告警、财务、工单、日程等严肃场景——laser 在这些场景里显得轻浮廉价。
- **默认不用**；Step 1 设计方案里若要用，需显式说明"×× 场景适合 laser 风格"并得到确认。
