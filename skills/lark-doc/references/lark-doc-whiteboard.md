# lark-doc 画板处理指南

## 两个 Skill 的职责边界

| Skill             | 核心职责                                                      | 约束                              |
|-------------------|-----------------------------------------------------------|---------------------------------|
| `lark-doc`        | 识别画板机会、使用 Mermaid/SVG 创建图表、按需调度 SubAgent、插入简单图表或复杂空白画板 | 简单图由主 Agent 直接写入；复杂图或多图并行时可按需隔离到 SubAgent |
| `lark-whiteboard` | 查询/导出已有画板；复杂图表生成（Mermaid/DSL/SVG 路由、场景选型、渲染验证）；写入已有/空白画板  | 复杂图表生成或已有画板更新时按需读取；宿主支持且上下文较重时可隔离到 SubAgent |

## 画板适用规则

写文档时，核心流程、系统架构、方案对比、风险链路、里程碑、指标趋势、因果归因、组织关系、能力分层等内容，如果图示能明显降低理解成本，可以规划为画板；结构简单或文字更清楚的内容不必强行画板化。

同一篇文档可以有多个画板。确有多个独立图示点时，可拆成多个聚焦画板，而不是把所有信息塞进一张大图。

## 文档与画板协同流程

### 步骤 1：识别画板机会

| 场景                      | 入口                                                        |
|-------------------------|-----------------------------------------------------------|
| 文档中需要思维导图、时序图、类图、饼图、甘特图 | 步骤 2A:使用 mermaid 插入图表                                     |
| 文档中需要插入其他图表/自定义图形       | 步骤 2B: 使用 SVG 插入图表                                        |
| 已有画板需要更新内容              | 先 `docs +fetch` 获取 `board_token`，跳至步骤 3B |
| 只查看 / 下载已有画板            | 切换至 `lark-whiteboard`，不走本流程                               |

> [!IMPORTANT]
> ⚠️ **分别对每个图表进行决策**

如果有多个位置需要插入图表，你需要根据每个图表的内容**分别决定**采用步骤 2A 还是 2B。思维导图、时序图、类图、饼图、甘特图插入 Mermaid 块；其他类型图表使用 SVG；已有画板更新走步骤 3B。

**执行方式（SubAgent 为可选隔离优化，而非硬性前提）**：

- **默认由主 Agent 直接执行**：简单 Mermaid / SVG 图、单张画板、已有画板的轻量更新，或当前会话本身聚焦于画板创作/编辑时，直接写入更高效，且能完整保留主对话上下文与术语约定。
- **按需使用 SubAgent（可选）**：仅当当前运行环境支持 SubAgent 且具备同等工具、网络与认证权限，并满足以下条件之一时使用：
  - 同篇文档需要**并行生成多张**互不依赖的画板；
  - 文档主任务上下文很重，而该画板需要专门视觉设计、加载大量 `lark-whiteboard` 参考文档或进行多轮渲染迭代，隔离上下文能明显减少干扰。
- **降级兜底**：宿主不支持 SubAgent，或 SubAgent 因沙箱、权限或超时失败时，直接回退由主 Agent 按同一流程执行，不得因无法启动 SubAgent 而阻塞任务。

### 步骤 2A: 使用 mermaid 插入图表

```xml

<whiteboard type="mermaid">
    mermaid 代码...
</whiteboard>
```

如果 Mermaid 已在本地文件中，可写成 `<whiteboard type="mermaid" path="@./diagram.mmd"></whiteboard>`；CLI 会在写入前读取文件并展开为内联内容。

### 步骤 2B: 使用 SVG 插入图表

使用 `docs +create` / `docs +update`（或写入本地 XML）插入：

```xml

<whiteboard type="svg">
    <svg...>...
    </svg>
</whiteboard>
```

如果 SVG 已在本地文件中，可写成 `<whiteboard type="svg" path="@./diagram.svg"></whiteboard>`；PlantUML 文件同理使用 `<whiteboard type="plantuml" path="@./sequence.puml"></whiteboard>`。

**SVG 通用约束**：SVG 必须完整自包含，包含 `<svg>` 根节点和 `viewBox`，不引用外部图片、脚本或远程资源。

若按步骤 1 决定交由 SubAgent 产出或插入（可选），需传入以下最小上下文以及下方的 [SVG 设计 Workflow] 章节指南：

- doc token、插入位置（标题 / block_id / command）
- 图表目标、受众、源段落或数据
- 要求读取 `lark-doc-xml.md`；仅做文档内 SVG 插入时不需要读取 `lark-whiteboard`

#### 画板 SVG 设计指南

使用 SVG 插入画板时，最终交付是**画板跨越重排渲染的节点**(你写 SVG → 画板解析)
**核心心智纠正 (重要)**：

- 大多数 AI 如果只考虑“绝对不报错/完美映射”, 最终给出的都是全篇纯白底色加单层 `<rect>` 的方正卡片网格, 极其死板单调, *
  *这将被视为不及格！**
- **SVG 给你了完全的设计自由**, 请大胆使用你脑内的图标路径 (`<path>`), 连接指引 (`流畅的 <path>`), 各种环境氛围点缀,
  大胆一点, 充分信任你的品味, 发挥出你的顶级艺术创造力！

##### SVG 设计 Workflow

###### 1. 想清楚要画什么

- **核心信息是什么？** 能做到一图胜千言, 绝对不要只生成平平无奇的文字表格, 要有设计感
- **内容充实度**：如果用户描述稀疏简略, 利用你的领域知识扩展, 保证信息维度和内容充实, 但不要过度堆砌, 淹没重点
- **视觉层级与隐喻**：这个没有固定的形式, 你自由判断, 比如: 给重要的节点加光环, 加高亮背景；给对比项设计天平或对称结构

###### 2. 写 SVG

> [!IMPORTANT]
> 布局, 配色, 信息密度, 装饰物——**全部由你判断**, 打破单调的 `<rect>` 牢笼, 严禁通篇用矩形和文字应付用户
> 操作边界约束：

- **语言跟随用户**：图表文字的语言与用户 prompt 保持一致, 技术术语用行业里通用的写法, 不机械翻译
- 文字用 `<text>`(不是 `<path>`), 容器宽度留够——画板按 CJK ≈ 1em / Latin ≈ 0.6em 重排
- 连线使用正交折线替代斜直线(`<polyline>` 带水平/垂直折点)视觉效果更好
- 可自由使用 `translate`, `rotate`, `scale`但请尽量避免使用 `skewX` / `skewY` / `matrix(...)` 发生空间级扭曲

###### 画板怎么处理 SVG

画板的 svg-parser 把可识别元素转成可编辑节点, 其余降级为内嵌图片(渲染没问题, 虽然不可编辑, 但是可以正常显示)；但非阴影用途的
`<filter>` / `<pattern>` / `<clipPath>` / `<mask>` 等装饰特性画板不支持（见下方⚠️）
**不需要所有元素都可编辑, 但必须避免使用不支持的装饰特性, 且要兼顾可编辑和美观漂亮**

**可识别的元素**

- 形状：`<rect>` / `<circle>` / `<ellipse>` / `<polygon>`
- 连线：`<line>` / `<polyline>` / `<path>`(自动识别为直线 / 折线 / 曲线)
- 文本：`<text>` / `<tspan>` 画板硬编码 Noto Sans SC **文字必须用 `<text>`**
- 分组：`<g>` / `<a>` / `<use>` 引用 `<symbol>`
- 变换：`translate` / `rotate` / `scale` 正常；`skewX` / `skewY` / `matrix(...)` 降级
- 阴影：`<filter>` 里放 `<feDropShadow>` 或标准 drop/inner primitive 链 (`<feGaussianBlur in="SourceAlpha">` + `<feOffset>` + `<feFlood>` + `<feComposite>` + `<feMerge>`), 会被识别成节点阴影, drop 至多 1 个, inner 至多 1 个; 其余 filter 效果不识别
- 渐变：`<linearGradient>` / `<radialGradient>` 在 `<defs>` 中定义, 通过 `fill="url(#id)"` 引用 (载体限 `<rect>` / `<circle>` / `<ellipse>` / `<polygon>` / `<path>`), 需要至少 2 个 `<stop>`, `gradientUnits` 只支持默认的 `objectBoundingBox` (不写即可)

> [!IMPORTANT]
> ⚠️ **不支持的装饰特性**

- `<pattern>` / `<clipPath>` / `<mask>` / 非阴影用途的 `<filter>` (blur / hue-rotate / 复合合成 / `flood-color=url(...)` / 多个 `<feDropShadow>` 等) → 画板不支持，**请避免使用，否则会导致画板渲染问题**
- 渐变边界：`gradientUnits="userSpaceOnUse"` / `spreadMethod="reflect|repeat"` / stops 少于 2 个 / 复杂 `gradientTransform` 会变成不可编辑图片, 视觉正确但失去可编辑性, 若无必要请沿用默认 `objectBoundingBox`

###### 3.插入后审查

插入画板后，可以从返回值使用 lark-cli 指令，将画板内容导出为 png
图片。若是对设计不满意，可以修改后，删除原来的画板再重新插入，或是调用 [
`../../lark-whiteboard/SKILL.md`](../../lark-whiteboard/SKILL.md) 编辑。

```bash
lark-cli whiteboard +export \
  --whiteboard-token "wbcnxxxxxxxx" \
  --output-type preview \
  --output ./preview.png
```

### 步骤 3B：复杂图表生成或编辑已有画板（调用 `lark-whiteboard`）

读取 [`../../lark-whiteboard/SKILL.md`](../../lark-whiteboard/SKILL.md)，按其创作或编辑工作流写入目标 `board_token`。

- 默认可由主 Agent 直接执行；若按步骤 1 满足隔离条件（文档主任务上下文较重、需多轮渲染调优，或多个互不依赖画板需并行处理）且宿主支持同等权限的 SubAgent，可交由 SubAgent 执行。
- 若 SubAgent 不可用、权限受限或执行失败，直接降级由主 Agent 按同一流程执行。

选择启动 SubAgent 时，传入以下最小上下文（多个画板互不依赖时可并行启动；每个 SubAgent 只负责一个画板或一个 SVG 插入点，不要互相复用上下文）：

- `board_token`
- 图表目标、推荐画板类型、受众
- 与图表直接相关的源段落或数据
- 要求读取 [`../../lark-whiteboard/SKILL.md`](../../lark-whiteboard/SKILL.md)，按其完整流程写入该 `board_token`

### 步骤 4：完成校验

- Mermaid: 确认插入的是 `<whiteboard type="mermaid">`，且内容 mermaid 语法完整
- SVG: 确认插入的是 `<whiteboard type="svg">`，且内容是完整 `<svg ...>...</svg>`
- 不保留空白占位画板；复杂路径只有空白画板而无内容视为任务未完成

---

---

## 关联参考

- 画板查询/创作/修改/渲染写入：[`../../lark-whiteboard/SKILL.md`](../../lark-whiteboard/SKILL.md)
