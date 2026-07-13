# Whiteboard 画板元素

`<whiteboard>` 放在 `<data>` 内，内部可放 **SVG** 或 **Mermaid**，用于绘制流程图、时序图、架构图、散点图、漏斗图、自定义图标、装饰图案等 `<chart>` 和 `<shape>` 难以覆盖的视觉内容。

普通柱状图、条形图、折线图、面积图、雷达图、饼图 / 环图和组合图应优先使用原生 `<chart>`。除非用户明确要求像素级自定义，或图表类型确实不受 `<chart>` 支持，否则不要用 `<whiteboard>` + SVG / Mermaid 重画这些标准图表。

> 前置条件：使用本文档前先阅读 [lark-slides SKILL.md](../SKILL.md)。

---

## `<chart>` 还是 `<whiteboard>`？

**先判断内容类型，再进入本文档：**

| 场景 | 推荐元素 |
|------|---------|
| 有结构化数据序列的柱/条/折线/面积/雷达/饼/环/组合图 | `<chart>` — 原生渲染，支持 legend / tooltip / 系列配色 |
| 散点图、漏斗图（`<chart>` 不支持）或其他非原生数据视觉 | `<whiteboard>` SVG |
| 流程图、时序图、架构图、类图、ER 图等拓扑图 | `<whiteboard>` Mermaid 或 SVG |
| 自定义图标、徽标、示意性图形（需要 path/polygon 精确控制） | `<whiteboard>` SVG |
| 进度条、波浪背景、装饰图案、像素级自定义可视化 | `<whiteboard>` SVG |

> 适合 `<chart>` 的内容就用 `<chart>`，不要用 SVG / Mermaid 手绘——原生渲染更省力、结构更稳定，也更容易被回读和后续编辑。

---

## whiteboard 公共属性

| 属性 | 必需 | 说明 |
|------|------|------|
| `topLeftX` | 是 | 左上角 X 坐标（slide 坐标系，slide 默认宽 960） |
| `topLeftY` | 是 | 左上角 Y 坐标（slide 坐标系，slide 默认高 540） |
| `width` | 是 | 画板宽度（像素） |
| `height` | 是 | 画板高度（像素） |

> SVG 模式下 `<svg>` 需声明 `xmlns="http://www.w3.org/2000/svg"`；内容大小由子元素包围盒决定，`width`/`height`/`viewBox` 不影响渲染（仅当元素属性使用百分比值时需要 `viewBox` 提供计算基准）。Mermaid 模式不需要额外属性。

SVG 内的坐标相对于 whiteboard 自身左上角（0,0），与 slide 坐标系无关。

---

## SVG 还是 Mermaid？

选择分三步：**先排除原生 `<chart>`，再判断 whiteboard 类型，最后看当前模型身份**。

### 第一步：先确认是否应该使用 `<chart>`

如果内容是柱状图、条形图、折线图、面积图、雷达图、饼图 / 环图或组合图，返回使用原生 `<chart>`，不要继续套用本文档的 SVG / Mermaid 路径。

### 第二步：whiteboard 类型优先判断

以下类型**推荐 Mermaid**，自动布局、代码简洁；如需精确匹配品牌配色或自定义节点样式，可改用 SVG：

| 图表类型 | Mermaid 关键字 |
|----------|--------------|
| 流程图、决策树、架构图 | `flowchart TD` / `flowchart LR` |
| 时序图 | `sequenceDiagram` |
| 类图 | `classDiagram` |
| 甘特图 | `gantt` |
| 状态图 | `stateDiagram-v2` |
| 思维导图 | `mindmap` |
| ER 图 | `erDiagram` |

### 第三步：非原生图表与装饰元素按模型身份选路径

上表以外的场景（散点图、漏斗图、进度条、时间线、波浪背景、星点纹理等）需要精确控制坐标和配色，SVG 表达力更强，但各模型生成 SVG 的能力有差异：

| 模型身份 | 路径 |
|----------|------|
| Claude / Gemini / GPT / GLM | **SVG** — 精确控制坐标、颜色、透明度 |
| Doubao / Seed / Other | **Mermaid** — 用 `gantt`、`flowchart` 等近似表达；确实无法用 Mermaid 表达时才回退到简单 SVG 矩形/线条 |

> **先自报身份再选路径**：在决定使用 SVG 之前，确认当前模型属于哪一类。不要跳过这一步。

---

## 模式一：SVG

### ⚠️ 设计品质要求

在 slide 里嵌入 `<whiteboard>` 的目的是**表达原生 `<chart>` 或基础 `<shape>` 难以覆盖的视觉关系**，不是把标准数据图表手绘一遍。

- **不要只用矩形加文字应付**：通篇纯白底色 + 方块 + 黑字等于白做，这是不及格输出
- **非原生数据视觉必须有坐标系**：散点、漏斗等仍要有必要的坐标轴、刻度、数值标注或分段说明，不要只画点或色块
- **字号必须有层级**：标题 ≠ 标签 ≠ 数值，混用同一字号会消灭视觉焦点
- **配色要与 slide 主题呼应**：深色 slide 背景下图表用透明底或深色卡片；浅色背景下避免再加纯白底块
- **每个 whiteboard 都是设计机会**：主动用圆角、半透明填充、清晰分组、节点状态等细节拉开与默认模板的差距
- **写 SVG 前先判断背景亮度**：背景亮度 < 30% 时，装饰元素"对比不足"比"过强"危害更大，宁重勿轻；
- **装饰层次用亮度跳跃，不用线性叠透明度**：`α=0.04→0.08→0.12` 的等差递增在深色底上几乎看不出差异（相邻层亮度差 ≈20）；正确做法是非线性跳跃如 `0.10→0.40→0.70→1.0`，相邻层亮度差 ≥60。

### 语法

```xml
<whiteboard width="400" height="300" topLeftX="500" topLeftY="120">
  <svg xmlns="http://www.w3.org/2000/svg">
    <rect x="50" y="50" width="80" height="200" rx="4" fill="rgba(59,130,246,0.85)"/>
    <text x="90" y="270" text-anchor="middle" font-size="12" fill="rgba(100,116,139,1)">ABC</text>
  </svg>
</whiteboard>
```

`<svg>` 需声明 `xmlns="http://www.w3.org/2000/svg"`；`width`/`height`/`viewBox` 无需填写，若元素属性使用百分比值则需额外声明 `viewBox`。

### ⚠️ 渲染包围盒规则

whiteboard 渲染时以**所有子元素的几何包围盒合并结果**为内容区域，自适应缩放到容器。

`<svg>` 上的 `width`、`height`、`viewBox` 不影响内容区域的计算，但 `viewBox` 有一个实际用途：**为百分比属性提供计算基准**。若元素使用 `width="50%"` 等百分比值，必须声明 `viewBox` 才能正确解析；绝对坐标元素则无需关心。推荐统一使用绝对坐标，避免引入百分比依赖。

### 支持的 SVG 元素

| 元素 | 说明 | 典型用途 |
|------|------|---------|
| `<rect>` | 矩形，支持 `rx` 圆角 | 卡片、进度条、分段色块 |
| `<circle>` | 圆 | 节点、装饰点、环形图 |
| `<ellipse>` | 椭圆 | 自定义轮廓图形 |
| `<line>` | 直线 | 轴线、分隔线、连接线 |
| `<path>` | 任意路径（支持 Q/C 曲线） | 波浪、曲线、弧形 |
| `<text>` | 文本，支持中文 | 标签、数值 |
| `<polygon>` | 多边形 | 箭头、星形、面积填充 |
| `<g>` | 分组 | 批量变换、语义分组 |
| `<linearGradient>` | 线性渐变定义，配合 `fill="url(#id)"` 使用 | 渐变背景、渐变填充 |

**颜色：** 统一用 `rgba(R,G,B,A)`，对深浅背景都友好。  
**虚线：** `stroke-dasharray="4,4"` 用于网格线 / 坐标轴。  
**变换：** `transform="translate(x,y)"` / `rotate(deg cx cy)` / `scale(n)` 均支持。

---
### 元素计算

SVG 中只要涉及批量定位、等间距排布或数据映射，**建议额外运行一个 Python 脚本把坐标算出来再填入 SVG**，而不是手动估值。适用范围包括散点、漏斗、装饰性点阵、等间距圆、重复图案等；普通柱状图、折线图、饼图仍应回到原生 `<chart>`。

> **主动去算**：写 SVG 之前先运行脚本，把输出当注释贴在 `<svg>` 开头，再照着填坐标。估值几乎每次都需要反复调整，跳过这步反而更慢。

**散点图 / 装饰性点阵范式**

```python
W, H = 360, 260
origin_x, origin_y = 50, 216  # 左下角，SVG Y 轴向下
cw, ch = 290, 184

points = [(12, 40), (28, 80), (45, 65)]
x_min, x_max, y_min, y_max = 0, 50, 0, 100
for i, (xv, yv) in enumerate(points):
    x = round(origin_x + (xv - x_min) / (x_max - x_min) * cw)
    y = round(origin_y - (yv - y_min) / (y_max - y_min) * ch)
    print(f"point-{i}: cx={x} cy={y}")
```

**装饰性元素（等间距范式）**

```python
n, total_w, cy, r = 8, 340, 40, 4
step = total_w / (n - 1)
for i in range(n):
    print(f"circle-{i}: cx={round(i * step)} cy={cy} r={r}")
```

**最大包围盒 → whiteboard 尺寸**

所有元素坐标算完后，汇总出整体包围盒，直接作为 whiteboard 的 `width`/`height`：

```python
# 每个元素登记 (x, y, w, h)，含 stroke 外扩
elements = [
    (10, 20, 80, 160),   # item-0
    (107, 10, 80, 170),  # item-1
    (204, 40, 80, 140),  # item-2
    (0, 0, 300, 1),      # x-axis
]

xs = [x for x, y, w, h in elements]
ys = [y for x, y, w, h in elements]
x2 = [x + w for x, y, w, h in elements]
y2 = [y + h for x, y, w, h in elements]

wb_w = max(x2) - min(xs)
wb_h = max(y2) - min(ys)
print(f"whiteboard width={wb_w} height={wb_h}")
```

输出即 `<whiteboard width=... height=...>` 的值，无需手动估算。

---
### 布局模式

**全屏装饰层**
```xml
<whiteboard width="960" height="540" topLeftX="0" topLeftY="0">
  <svg xmlns="http://www.w3.org/2000/svg">
    ...
  </svg>
</whiteboard>
```

> ⚠️ 全屏装饰 whiteboard 必须放在所有 `<shape>` / `<img>` / `<table>` 之前，否则会遮挡文字内容。XML 中元素位置越靠后，渲染层级越高。

**侧栏图表（与文字 shape 并排）**
```xml
<!-- 左侧文字 -->
<shape type="text" topLeftX="60" topLeftY="120" width="500" height="340">...</shape>
<!-- 右侧图表 -->
<whiteboard width="340" height="340" topLeftX="580" topLeftY="120">
  <svg xmlns="http://www.w3.org/2000/svg">
    ...
  </svg>
</whiteboard>
```

**底部装饰条**
```xml
<whiteboard width="960" height="100" topLeftX="0" topLeftY="440">
  <svg xmlns="http://www.w3.org/2000/svg">
    ...
  </svg>
</whiteboard>
```

---

### 禁止使用的 SVG 特性

以下特性在 slide `<whiteboard>` 渲染端不支持或行为不可预测，必须避免：

| 禁止 | 原因 | 替代方案 |
|------|------|---------|
| `<radialGradient>` | 渲染失败 | 用 `<linearGradient>` 或 `rgba()` 透明度模拟深浅层次 |
| `<filter>`（阴影、模糊等） | 渲染失败 | 用半透明 `<rect>` 叠加模拟阴影 |
| `<clipPath>` / `<mask>` | 渲染失败 | 调整元素坐标和尺寸自然裁切 |
| `<pattern>` | 渲染失败 | 手动铺 `<circle>` / `<rect>` 点阵 |
| `skewX` / `skewY` / `matrix(...)` | 空间扭曲，降级渲染 | 用 `rotate` + `translate` 替代 |
| `<image>` 外链 URL | 不支持外链 | 先上传得到 file_token，再用 `<img>` 元素 |

---


## 模式二：Mermaid

### 语法

```xml
<whiteboard topLeftX="72" topLeftY="60" width="816" height="360">
  <mermaid>
    <![CDATA[
        flowchart TD
            A[检查 lark-cli 与 jq] --> B[编写每页 slide XML]
            B --> C[通过 jq 生成 slides JSON]
            C --> D[执行 slides +create]
            D --> E[读取 xml_presentation_id]
            E --> F[回读并验证创建结果]
    ]]>
  </mermaid>
</whiteboard>
```

**关键点：**
- 内容用 `<![CDATA[...]]>` 包裹——Mermaid 语法里的 `[`、`>`、`-->` 是 XML 特殊字符，CDATA 避免转义问题
- whiteboard 只需 `topLeftX`、`topLeftY`、`width`、`height`

### 支持的 Mermaid 图表类型

| 类型 | 关键字 | 适用场景 |
|------|--------|---------|
| 流程图 | `flowchart TD` / `flowchart LR` | 业务流程、决策树、工作流 |
| 时序图 | `sequenceDiagram` | 系统交互、API 调用链 |
| 甘特图 | `gantt` | 项目计划、里程碑 |
| 类图 | `classDiagram` | 对象关系、架构设计 |
| ER 图 | `erDiagram` | 数据库结构 |
| 状态图 | `stateDiagram-v2` | 状态机、生命周期 |
| 思维导图 | `mindmap` | 主题梳理、知识架构 |
| 用户旅程 | `journey` | 用户体验路径 |

### Mermaid 布局建议

Mermaid 图表会自动撑满 whiteboard 区域。建议：
- 流程图留足高度，节点较多时适当增加 height（比如 400-480）
- 避免一页放超过 15 个节点，内容太密时考虑分页
- 推荐尺寸参考：

| 图表类型 | 建议 width | 建议 height |
|---------|-----------|------------|
| 流程图（5-8 节点） | 720-816 | 300-400 |
| 时序图（3-5 参与者） | 720-816 | 320-420 |
| 甘特图 | 816 | 280-360 |
| 思维导图 | 816 | 380-480 |

---

## 注意事项 & 已知问题

### z-order（SVG 模式）

whiteboard 在 XML 中的位置决定渲染层级：在 shape 前 → 在下层；在 shape 后 → 在上层。全屏装饰 whiteboard 应放在所有 shape 之前。

### Mermaid CDATA 必要性

Mermaid 语法包含 `[`、`>`、`-->`，不用 CDATA 直接写会破坏 XML 解析。始终使用 `<![CDATA[ ... ]]>`。

---

## 快速自检清单

**SVG 模式——结构检查：**
- [ ] `<svg>` 声明了 `xmlns="http://www.w3.org/2000/svg"`
- [ ] whiteboard 的 `width`/`height` 由所有元素的最大包围盒（含 stroke 外扩）计算得出，不手动估值
- [ ] `topLeftX + width ≤ 960`，`topLeftY + height ≤ 540`
- [ ] 无 `<radialGradient>` / `<filter>` / `<clipPath>`
- [ ] 文字 `y` 坐标为 baseline 位置，最小值 ≥ font-size（避免被裁切）

**SVG 模式——视觉品质检查：**
- [ ] 非原生数据视觉有必要的坐标轴、网格线、数值标注或分段说明，没有"裸点"或无解释色块
- [ ] 字号有层级：标题 > 数值 > 轴标签，非全部相同
- [ ] 单一数据系列用同一颜色，多系列用不同颜色且对比充足
- [ ] 轴标签与图表元素互不遮挡，留有足够空间
- [ ] 坐标推导有注释（写明 originX/Y、chartW/H、数据映射公式）

**Mermaid 模式：**
- [ ] 内容包在 `<![CDATA[...]]>` 内
- [ ] CDATA 结束符 `]]>` 不出现在 Mermaid 代码本身中
- [ ] `topLeftX + width ≤ 960`，`topLeftY + height ≤ 540`
- [ ] 节点数量合理（单图不超过 15-20 个节点）

**通用：**
- [ ] XML 标签全部闭合，属性引号完整
- [ ] 如果失败，检查是否是偶发 5001000，重试一次

---

## 参考

- [lark-slides SKILL.md](../SKILL.md)
