# 布局系统

## 布局决策

> 不要靠关键词猜布局。先分析信息结构，再决定布局策略。

| 判断条件 | 布局策略 |
|----------|----------|
| 元素有明确上下层级（用户层→服务层→数据层） | **Flex 分层** |
| 空间位置承载信息（地理方位、拓扑坐标、角度） | **纯绝对定位**（脚本计算坐标） |
| 多个独立模块平级互联，无上下级 | **混合布局（岛屿式）** |
| 不确定 | **默认 Flex 分层**（最安全） |

| 布局策略 | 适用图表 |
|----------|----------|
| 纯绝对定位 | 鱼骨图、柱状图、折线图、拓扑图、地图路线 |
| Flex 骨架 | 架构层级图、卡片墙、组织架构图、对比表 |
| 混合（岛屿式） | 系统集成图、飞轮图、流程图 |

**读代码画架构图**：扫目录结构（按层分 → Flex；按功能模块分 → 看依赖方向）→ grep import（单向→Flex；网状→混合）→ 拿不准→默认 Flex。

> **flex 容器内的 `x/y` 会被完全忽略！**

❌ 致命错误：
```json
{ "type": "frame", "layout": "vertical", "children": [
  { "type": "rect", "x": 100, "y": 0, "text": "成都" },
  { "type": "rect", "x": 540, "y": 0, "text": "康定" }
]}
```
✅ 正确：用 `layout: "none"` 或放在顶层 nodes 用 x/y。

**构建方式**：

| 布局类型 | 做法 |
|----------|------|
| 纯 Flex | 直接写 JSON |
| 混合布局 | 直接写 JSON + 估高辅助 |
| 纯绝对定位 | 写脚本生成 JSON（node xxx.js） |
| 需要精确避让 | 脚本 + `--layout` 两阶段 |

---

## 网格方法论

核心理念：**先画网格，再填内容**。

先回答三个问题：
1. **信息分几行几列？** 每组一行或一列
2. **每格多大？** 等宽还是有主次？
3. **行列间距多大？** 分区间 24-32px，同区内 12-16px

---

## 布局模式选择

| 模式 | 适用场景 | DSL 映射 |
|------|---------|---------|
| grid | 架构图、对比表、卡片墙、看板 | vertical frame 嵌套 horizontal frame |
| flow | 流程图、审批流 | vertical frame，主流程居中 |
| tree | 组织架构、模块依赖 | 根节点居中，子节点横向展开 |
| free | 系统集成、拓扑图、鱼骨图 | `layout: "none"` + x/y |

大多数图表用 grid 模式。只有节点位置本身有含义时才用 free。

> 以上都是布局策略名称，不是 DSL 的 `layout` 属性值。DSL 的 layout 只支持 `'horizontal'`、`'vertical'`、`'none'` 三种。

---

## DSL 与 CSS Flexbox 属性映射

| DSL 属性 | 对应的 CSS 心智模型 | 限制 |
|-----------------------|-----------------------------------|--------|
| `layout: 'horizontal'` | `flex-direction: row` | 不写 layout = 绝对定位 |
| `layout: 'vertical'` | `flex-direction: column` | 同上 |
| `layout: 'none'` | `position: absolute`（子节点用 x/y） | 子节点不能用 `fill-container` |
| `width/height: 'fill-container'` | `flex: 1`（主轴）/ `align-self: stretch`（交叉轴） | 祖先必须有确定尺寸 |
| `width/height: 'fit-content'` | `width/height: auto` | — |
| `alignItems` | 同 CSS `align-items` | 仅 `'start'`/`'center'`/`'end'`/`'stretch'`（无 flex- 前缀）|
| `justifyContent` | 同 CSS `justify-content` | 仅 `'start'`/`'center'`/`'end'`/`'space-between'`/`'space-around'` |
| `gap` | 同 CSS `gap` | 必须显式写（不写节点会粘连） |
| `padding` | 同 CSS `padding` | 必须显式写。支持 `number` / `[v,h]` / `[t,r,b,l]` |

`alignItems` 默认值为 `'start'`（CSS Flexbox 默认 `stretch`）。需要等高卡片时必须显式写 `alignItems: 'stretch'`。

DSL 的语法是严格白名单，不能写原生 CSS 属性（不支持 `alignSelf`、`flexWrap`、`margin` 等）。

---

## DSL 注意事项

1. **frame 必须写 layout 属性**，不写时子节点全堆在左上角。
2. **fill-container 死锁陷阱**：使用 `fill-container` 时，祖先链中必须有固定宽度（或高度），否则和 `fit-content` 形成死锁，尺寸退化为 0。
   ```json
   // 死锁：horizontal 父 width fit-content + 子 width fill-container
   { "type": "frame", "layout": "horizontal", "width": "fit-content", "children": [
     { "type": "rect", "width": "fill-container" }
   ]}
   // 正确：祖先在对应轴有固定尺寸
   { "type": "frame", "layout": "horizontal", "width": 1200, "children": [
     { "type": "rect", "width": "fill-container" }
   ]}
   ```
3. **含文字节点高度用 fit-content**，引擎不支持 overflow，写死高度会截断文字。
4. **Shape 节点有内边距**：rect/ellipse/diamond/triangle 各边 12px；cylinder 垂直 +42px。
5. **不支持 flex-wrap**，需要换行时用嵌套 frame 模拟。
6. **图层顺序**：数组中越靠后的节点层级越高。需要叠加标注时放在数组最后。

---

## 布局选择指南

| 你要表达的关系 | 怎么排 | DSL 写法 |
|-------------|-------|---------|
| 先后顺序、层级从上到下 | 纵向堆叠 | `layout: 'vertical'` |
| 并列、同等重要、可对比 | 横向等分 | `layout: 'horizontal'` + `alignItems: 'stretch'` + `width: 'fill-container'` |
| 区域有名称，名称在侧边 | 侧标签 + 内容并排 | 横向 frame: [text(标签), frame(内容)] |
| 多个大分区，各自独立 | 分区纵向排列 | 纵向 frame 包多个彩色 frame |
| 一行放不下，需要换行 | 嵌套横向 frame 模拟换行 | 纵向 frame 包多个横向 frame |
| 节点位置本身有含义（拓扑、地图） | 绝对定位 | `layout: 'none'` + x/y |

这些可以自由嵌套组合。比如：纵向堆叠(标题) + 分区纵向排列(多个层) + 每个层内横向等分(节点)。

---

## 布局示例

### 纵向堆叠（标题 + 内容）

```json
{
  "type": "frame", "layout": "vertical", "gap": 28, "padding": 32,
  "width": 1200, "height": "fit-content",
  "children": [
    { "type": "text", "width": "fill-container", "height": "fit-content",
      "text": "图表标题", "fontSize": 24, "textAlign": "center" },
    ...内容...
  ]
}
```

### 横向等分（并列元素）

```json
{
  "type": "frame", "layout": "horizontal", "gap": 16, "padding": 0,
  "width": "fill-container", "height": "fit-content",
  "alignItems": "stretch",
  "children": [
    { "type": "rect", "width": "fill-container", "height": "fit-content",
      "textAlign": "center", "verticalAlign": "middle", "text": "A" },
    { "type": "rect", "width": "fill-container", "height": "fit-content",
      "textAlign": "center", "verticalAlign": "middle", "text": "B" }
  ]
}
```

`alignItems: 'stretch'` + `width: 'fill-container'` = 等宽等高。

### 侧标签 + 内容

```json
{
  "type": "frame", "layout": "horizontal", "gap": 24, "padding": 0,
  "width": "fill-container", "height": "fit-content",
  "alignItems": "center",
  "children": [
    { "type": "text", "width": 160, "height": "fit-content",
      "text": "区域名称", "fontSize": 20, "textColor": "#1F2329", "textAlign": "right" },
    { "type": "frame", "width": "fill-container", "height": "fit-content",
      ...区域内容...
    }
  ]
}
```

不要用 frame 的 `title` 属性做标签——渲染为极小标题栏，不可读。

### 分区纵向排列

把内容划分为几个大区域，每个区域用不同颜色区分（颜色从 style 文件的色板选取）：

```json
{
  "type": "frame", "layout": "vertical", "gap": 28, "padding": 0,
  "width": "fill-container", "height": "fit-content",
  "children": [
    { "type": "frame", "borderRadius": 8,
      "layout": "horizontal", "gap": 16, "padding": 20, ...区域1... },
    { "type": "frame", "borderRadius": 8,
      "layout": "horizontal", "gap": 16, "padding": 20, ...区域2... }
  ]
}
```

### 模拟换行

一行放不下时，拆成多个横向 frame：

```json
{
  "type": "frame", "layout": "vertical", "gap": 8, "padding": 0,
  "children": [
    { "type": "frame", "layout": "horizontal", "gap": 8, "padding": 0,
      "children": [item1, item2, item3, item4] },
    { "type": "frame", "layout": "horizontal", "gap": 8, "padding": 0,
      "children": [item5, item6] }
  ]
}
```

---

## 绝对定位

当节点位置本身有含义（拓扑图、地图、时间线轴）时用绝对定位。大多数图表优先用 Flex。

### 混合布局

模块内部用 Flex 自动排版，模块之间用绝对定位自由摆放。每个模块是一个带 x/y 的 flex frame：

```json
{
  "type": "frame", "id": "module-a", "x": 100, "y": 100,
  "width": 300, "height": "fit-content",
  "layout": "vertical", "gap": 8, "padding": 16,
  "children": [
    { "type": "rect", "width": "fill-container", "height": "fit-content", "text": "内容1" },
    { "type": "rect", "width": "fill-container", "height": "fit-content", "text": "内容2" }
  ]
}
```

### 两阶段绘图

先出骨架图导出坐标，再基于坐标补充连线和注解：

```bash
npx -y @larksuite/whiteboard-cli@^0.1.0 -i skeleton.json -o step1.png -l coords.json
```

`coords.json` 包含每个带 id 节点的精确坐标（absX, absY, width, height）。

---

## 常用间距和尺寸

| 参数 | 常用范围 | 说明 |
|------|---------|------|
| 整图宽度 | 1000-1400px | — |
| 分区之间间距 | 24-32px | — |
| 同分区内节点间距 | 12-16px | — |
| 有连线的节点间距 | >= 40px | 给箭头留空间 |
| 分区内边距 | 16-24px | — |
| 侧标签宽度 | 120-180px | — |

---

## 等大卡片

一排卡片需要等宽等高时，不要写固定像素：

```json
{
  "type": "frame", "layout": "horizontal", "gap": 16, "padding": 0,
  "alignItems": "stretch",
  "children": [
    { "type": "rect", "width": "fill-container", "height": "fit-content", "text": "A" },
    { "type": "rect", "width": "fill-container", "height": "fit-content", "text": "B" }
  ]
}
```

`alignItems: 'stretch'` + `width: 'fill-container'` = 等宽等高。
