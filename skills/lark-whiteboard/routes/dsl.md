# DSL 路径

> **这是画板，不是网页。** 画板是无限画布上自由放置元素，flex 布局是可选增强。

## Workflow

```
Step 1: 路由 & 读取知识
  - 读对应 scene 指南 — 了解结构特征和布局策略
  - 确定布局策略（见下方快速判断）和构建方式
  - 读 references/ 核心模块 — 语法、布局、配色、排版、连线

Step 2: 生成完整 DSL（含颜色）
  - 按 content.md 规划信息量和分组
  - 按 layout.md 选择布局模式和间距
  - 推荐使用图标让图表更直观，运行 `npx -y @larksuite/whiteboard-cli@^0.2.11 --icons` 查看可用图标
  - 按 style.md 上色（用户没指定时用默认经典色板）
  - 按 schema.md 语法输出完整 JSON
  - 连线参考 connectors.md，排版参考 typography.md

  注意：部分图形（鱼骨/飞轮/柱状/折线等）要按 scene 指南的脚本模板写 CommonJS 脚本生成 JSON：
    1. 创建产物目录 ./diagrams/YYYY-MM-DDTHHMMSS/
    2. 将脚本保存为 diagram.gen.cjs（必须 .cjs 后缀，脚本用 require() 写，.js 在 ESM 项目下会崩），执行 node diagram.gen.cjs 产出 diagram.json
    3. 用产出的 diagram.json 进入 Step 3

Step 3: 渲染 & 审查 → 交付
  - 渲染前自查（见下方检查清单）
  - 渲染 PNG（仅用于预览验证，不是最终产物）：npx -y @larksuite/whiteboard-cli@^0.2.11 -i diagram.json -o diagram.png
  - 检查：信息完整？布局合理？配色协调？文字无截断？连线无交叉？
  - 有问题 → 按症状表修复 → 重新渲染（最多 2 轮）
  - 2 轮后仍有严重问题 → 考虑走 Mermaid 路径兜底
  - 写入画板：用 whiteboard-cli 将 diagram.json 转换为 OpenAPI 格式并 pipe 给 +update：
      npx -y @larksuite/whiteboard-cli@^0.2.11 -i diagram.json --to openapi --format json \
        | lark-cli whiteboard +update --whiteboard-token <board_token> \
            --source - --input_format raw --idempotent-token <时间戳+标识> --as user
      → 完整 dry-run / 确认流程见 SKILL.md [§ 写入画板](../SKILL.md#写入画板)
  - 交付：向用户报告 board_token 写入成功
```

**布局策略快速判断**（详见 `references/layout.md`）：

先定**主布局**，再定子布局：**结构化信息**优先用 Flex，**关系链路**优先用 Dagre，**灵活定位**用绝对布局。

> **构建方式是强约束**：当 scene 指南要求"脚本生成"时，必须先写脚本（`.cjs`，CommonJS）并用 `node` 执行来产出 JSON 文件。

## 模块索引

### 核心参考（必读）

| 模块     | 文件                       | 说明                            |
| -------- | -------------------------- | ------------------------------- |
| DSL 语法 | `references/schema.md`     | 节点类型、属性、尺寸值          |
| 内容规划 | `references/content.md`    | 信息提取、密度决策、连线预判    |
| 布局系统 | `references/layout.md`     | 网格方法论、Flex 映射、间距规则 |
| 排版规则 | `references/typography.md` | 字号层级、对齐、行距            |
| 连线系统 | `references/connectors.md` | 拓扑规划、锚点选择              |
| 配色系统 | `references/style.md`      | 多色板、视觉层级                |

### 场景指南（按类型选读一个）

| 图表类型    | 文件                     | 适用场景                               |
| ----------- | ------------------------ | -------------------------------------- |
| 架构图      | `scenes/architecture.md` | 分层架构、微服务架构                   |
| 组织架构图  | `scenes/organization.md` | 公司组织、树形层级                     |
| 泳道图      | `scenes/swimlane.md`     | 跨角色流程、跨系统交互流程             |
| 对比图      | `scenes/comparison.md`   | 方案对比、功能矩阵                     |
| 鱼骨图      | `scenes/fishbone.md`     | 因果分析、根因分析                     |
| 柱状图      | `scenes/bar-chart.md`    | 柱状图、条形图                         |
| 折线图      | `scenes/line-chart.md`   | 折线图、趋势图                         |
| 树状图      | `scenes/treemap.md`      | 矩形树图、层级占比                     |
| 漏斗图      | `scenes/funnel.md`       | 转化漏斗、销售漏斗                     |
| 金字塔图    | `scenes/pyramid.md`      | 层级结构、需求层次                     |
| 循环/飞轮图 | `scenes/flywheel.md`     | 增长飞轮、闭环链路                     |
| 里程碑      | `scenes/milestone.md`    | 时间线、版本演进                       |
| 流程图      | `scenes/flowchart.md`    | 业务流、状态机、带条件判断的链路       |
| 图片展示    | `scenes/photo-showcase.md` | 用户显式要求图片/配图/插图时（需先完成 `references/image.md` 的图片准备） |

## 渲染前自查

- [ ] 不同分组用了不同颜色？同组节点样式完全一致？
- [ ] 外层浅色背景、内层白色节点？
- [ ] 所有节点有边框（borderWidth=2）？文字在背景上清晰可读？
- [ ] 连线用灰色（#BBBFC4），不用彩色？
- [ ] frame 都写了 layout 属性？gap 和 padding 都显式设置了？
- [ ] 含文字节点 height 用 fit-content？connector 在顶层 nodes 数组？

## 症状→修复表

| 看到的问题         | 改什么                              |
| ------------------ | ----------------------------------- |
| 文字被截断         | height 改为 fit-content             |
| 文字溢出容器右侧   | 增大 width，或缩短文字              |
| 节点重叠粘连       | 增大 gap                            |
| 节点挤成一团       | 增大 padding 和 gap                 |
| 连线穿过节点       | 调整 fromAnchor/toAnchor 或增大间距 |
| 大面积空白         | 缩小外层 frame 宽度                 |
| 文字和背景色太接近 | 调整 fillColor 或 textColor         |
| 布局整体偏左/偏右  | 调整绝对定位的 x 坐标使内容居中     |

## 关键约束速查

1. **含文字节点的 height 必须用 `'fit-content'`** — 写死数值会截断文字
2. **`fill-container` 仅在 flex 父容器中生效** — `layout: 'none'` 下宽度退化为 0
3. **`layout: 'none'` 的容器必须有固定宽高** — 不要写成 `fit-content`
4. **connector 必须放在顶层 nodes 数组** — 不能嵌套在 frame children 里
5. **flex 容器内的 x/y 会被完全忽略** — 需要自由定位时用 `layout: 'none'`
6. **Dagre 子容器默认为不透明节点** — 需穿透时声明 `layout: "dagre"` + `layoutOptions: { isCluster: true }`
