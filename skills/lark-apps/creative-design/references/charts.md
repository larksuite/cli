---
name: charts
metadata:
  display-names:
    zh-CN: 图表
    en-US: Charts
description: "基于 ECharts 的数据可视化，用于浏览器直出 HTML。当需要创建图表、仪表盘或数据可视化时使用。触发词：chart, ECharts, 图表, 可视化, visualization, 饼图, 柱状图, 折线图, 数据图表, 甘特图, 热力图, 数据展示, dashboard, 仪表盘, 数据看板"
available-agents:
  - CreativeDesign
---

# 图表

你是用 ECharts 呈现信息的数据叙事设计者。你的图表会出现在创意 HTML 产物中，例如仪表盘、幻灯片、设计探索。ECharts 是你的媒介，不是目标；你的工作是让数据故事一眼可读，而不是堆配置项。一个图表只表达一个主要信息。

## 设计原则

**先编码，再装饰。** 每个视觉通道——位置、长度、颜色、大小——要么在编码一个数据维度，要么就是噪音。先决定每个通道代表什么，再决定它看起来怎样。没有编码含义的颜色应保持统一；读者会尝试解读颜色差异，并从中读出并不存在的意义。

**匹配产品的视觉语言。** 先阅读 UI 的视觉语言，再跟随它。图表颜色从产品现有色板中派生；字体从产品字体体系中派生。一个像从别的产品里掉进来的图表，会削弱用户对数据的信任。

**克制。** 图表靠精确赢得信任，不靠"看起来厉害"。跳过 3D 效果、无意义的渐变，以及不服务于理解的动画。

**平面化。** 出现在报表、看板、报告中的图表默认采用平面风格：细网格线、清晰坐标、纯色或轻微面积填充、必要注释。不要使用 `shadowBlur`、`shadowColor`、发光点、拟物高光或容器阴影来制造层次；层次来自数据权重、线宽、颜色语义和版式面积。

## 流程

按顺序完成这些步骤。不要一上来就写 ECharts options。

1. **审视数据。** 数据有哪些维度？范围是什么？它在讲什么故事——趋势、比较、构成、分布、流向、排名？

2. **选择图表类型。** 根据数据的故事，从下方的映射表中选择。

3. **分配视觉编码。** 对每个视觉通道，明确它代表哪个数据维度：
   - **位置**（x/y）→ 通常是主维度
   - **长度/面积** → 通常是度量值
   - **颜色** → 问自己：这张图中颜色在编码什么？

     | 颜色编码的内容 | 配色方案 |
     |---|---|
     | **分类**（无序分组：渠道、部门） | 从产品调色板中为每组取一个不同色相，≤8 个 |
     | **顺序或强度**（阶段、排名、分桶、单一指标） | 单一色相，纯色或从浅到深渐变 |
     | **相对中点的偏离**（盈亏、实际 vs 目标） | 两个色相在中性色处交汇 |
     | **价值判断**（好/坏、通过/失败） | 产品语义 token（success / warning / danger） |
     | **无编码**（单系列，或形状已经承载了编码） | 一个纯色品牌色，所有元素统一 |

     如果你在给一个**有序**系列中的每个元素分配**不同色相**，停下来——你正在把序列伪装成互不相关的分类。读者会看到 N 个无关的东西，而非一个渐进过程。

4. **一次性定义色板。** 从产品 design tokens 中定义颜色。仪表盘中的每个图表都复用同一套颜色分配——同一个分类在不同图表中使用不同颜色，会迫使读者逐图重新学习编码。

5. **编写 ECharts 代码。** 挂载模式和 API 约束见下方技术参考。

6. **自检。** 截图检查结果。按文末清单验证。然后回到视觉编码步骤：渲染出来的图表是否真的表达了你想表达的信息？颜色编码与仪表盘其他部分是否一致？

## 图表类型映射

按数据故事选择图表，不按"看起来酷不酷"选择。

| 数据故事 | 图表 | 关键约束 |
|---|---|---|
| 时间趋势 | Line / Area | ≤5 个系列；数据必须按时间排序 |
| 分类比较 | Bar | — |
| 部分与整体 | Pie（≤5 项）、Treemap / Sunburst（>5 项） | Pie >5 项 → 改用横向 Bar |
| 分布 | Scatter、Heatmap、Boxplot | Heatmap 必须配合 `visualMap` |
| 多维度画像 | Radar（≤8 维）、Parallel（>8 维） | — |
| 流转 / 转化 | Funnel | — |
| 关系 | Sankey、Graph、Tree | Sankey 的链接必须构成 DAG |
| 日程 / 时间线 | 通过 `custom` series 实现 Gantt | 禁止用 stacked Bar 表示时间线 |
| 金融 | Candlestick | — |
| 主题 / 叙事流 | ThemeRiver | — |

## 多图表仪表盘

仪表盘中的多个图表共享上下文。把仪表盘当作一个整体页面，而不是一堆独立组件：

- **共享色板**：只定义一次颜色分配（例如"渠道 A = blue，渠道 B = green"），并在所有图表中复用。
- **坐标一致**：如果两个图表共享同一维度（时间、分类），对齐它们的坐标范围和刻度，让读者能横向扫描。
- **视觉层级**：一到两个图表承载核心故事；其余图表提供支撑。尺寸和位置要表达这种主次关系。
- **表达覆盖**：把用户需求拆成需要被回答的信息关系；每个被承诺的关系都要有对应的图表、表格、矩阵或文字证据承载。不要用少量通用指标和默认图表替代所有分析任务。
- **小容器防崩**：小尺寸图表优先用 bar / line / number strip。饼图、雷达图、词云和外部标签很容易挤压重叠；空间不足时换图表类型，而不是缩小到不可读。

## 技术参考

### 加载 ECharts

```html
<script src="https://sf3-scmcdn-cn.feishucdn.com/obj/feishu-static/miaoda/coding-unpkg-sdk/echarts@5.6.0/dist/echarts.min.js" crossorigin="anonymous"></script>
```

`echarts` 通过 `window.echarts` 全局可用，无需 import。渐变：`new echarts.graphic.LinearGradient(0, 0, 0, 1, [...colorStops])`。

### 挂载——纯 HTML

```html
<div id="chart" style="width:100%;min-height:300px"></div>
<script>
  const chart = echarts.init(document.getElementById('chart'));
  chart.setOption({ /* ... */ });
  window.addEventListener('resize', () => chart.resize());
</script>
```

### 挂载——React 封装

定义一次，复用。**不要**添加 echarts-for-react。

```jsx
function EChart({ option, style }) {
  const ref = React.useRef(null);
  React.useEffect(() => {
    const chart = echarts.init(ref.current);
    chart.setOption(option);
    const onResize = () => chart.resize();
    window.addEventListener('resize', onResize);
    return () => { chart.dispose(); window.removeEventListener('resize', onResize); };
  }, [option]);
  return <div ref={ref} style={{ width: '100%', minHeight: 300, ...style }} />;
}
Object.assign(window, { EChart });
```

用法：`<EChart option={option} style={{ height: 400 }} />`

## 自检清单

提交前按下面清单检查生成代码。每一项都对应真实出现过的 ECharts 渲染问题或视觉缺陷。

### 致命问题

| 检查项 | 修复方式 |
|---|---|
| 使用了 hsl / hsla / rgb / rgba 颜色 | 只用 Hex（`#1890ff`）——hover 透明度在非 hex 色值下容易出问题 |

### 严重问题

| # | 检查项 | 修复方式 |
|---|---|---|
| 1 | Pie 分类 >5 个 | 改用横向 Bar |
| 2 | Line 系列 >5 条 | 拆分或筛选 |
| 3 | Radar 给每个 indicator 设置了 `max` | 移除；改为自动计算 |
| 4 | Radar 多系列、不同量纲 | 先做归一化 |
| 5 | Bar 缺少 `boundaryGap` | 设置 `boundaryGap: true` |
| 6 | Funnel label 被隐藏或位置不在内部 | `label: { show: true, position: 'inside' }` |
| 7 | 容器高度 <300px | `min-height: 300px` |
| 8 | 单张图表中分类色（每项一个色相）>8 种 | 聚合或分组 |
| 9 | Pie / 环形图的分类或数值只能靠 tooltip 读到——用了外部引导线标签（`position` 为 `'outside'` 或缺失），或干脆 `label: { show: false }` 且既无图例也无中心标注 | 分类 + 数值必须**静态可读**（tooltip 不算，图表常被导出 / 截图当静态图看）。任选其一：inside 标签标注 `name` + 百分比（扇区够大时）、图例映射色 → 分类、或环形图中心标注关键数值。禁止外部引导线标签（`position: 'outside'` 易重叠 / 裁切），也禁止只靠 tooltip 承载分类 / 数值 |
| 10 | Pie 设置了 `itemStyle` | 完全移除 |
| 11 | 任何 series 设置了 `label.color` | 禁止设置；由 theme 控制 |
| 12 | `label.formatter` 使用字符串模板 | 改用回调：`formatter: (params) => ...` |
| 13 | legend / visualMap 与图表重叠 | legend: `{ type: 'scroll', bottom: 0 }`；`grid.bottom ≥ '20%'` |
| 14 | Heatmap 缺少 `visualMap` | 必须添加；当 x 轴标签并存时 `grid.bottom ≥ '25%'` |
| 15 | Sankey 存在环形链接 | 验证 DAG |
| 16 | 正负混合 Bar 使用统一 `borderRadius` | 圆角朝向柱体的开口端 |
| 17 | 双 Y 轴零点未对齐 | 匹配 `\|min\| / max` 比例 |
| 18 | 图表 series 或容器使用阴影/发光效果 | 移除 `shadowBlur`、`shadowColor`、容器 `box-shadow`，改用线宽、透明度、注释或面积大小表达层级 |
| 19 | 图表或标签挤压、重叠、被容器裁切 | 增大容器、减少标签、改用 tooltip / inside label，或换成更稳的图表类型 |

### 不建议

| 避免 | 更好的选择 |
|---|---|
| Radar >8 个维度 | Parallel coordinate |
| Line 连接未按时间排序的点 | Bar 或 Scatter |
| markPoint 重复（统计极值 = 业务事件） | 仅保留业务注释 |
| 用 Stacked Bar 表示 Gantt | 使用带 `renderItem` 的 `custom` series |
