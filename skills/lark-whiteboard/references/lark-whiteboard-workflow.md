# 画板创作/编辑工作流

## 创作 Workflow

> 此 workflow 用于**独立创作一个画板**。
> 需要在文档中批量创建多个画板时，由 lark-doc 负责调度，见 `lark-doc` 技能的 `references/lark-doc-whiteboard.md`。

**Step 1：获取 board_token**

| 用户给了什么 | 怎么获取 |
|---|---|
| 直接给了 whiteboard token（`wbcnXXX`）| 直接使用 |
| 文档 URL 或 doc_id，文档中已有画板 | `lark-cli docs +fetch --doc <URL> --as user`，从返回的 `<whiteboard token="xxx"/>` 提取 |
| 文档 URL 或 doc_id，需要新建画板 | `lark-cli docs +update --doc <doc_id> --command append --content '<whiteboard type="blank"></whiteboard>' --as user`，从响应 `data.new_blocks[0].block_token` 取得（`block_type == "whiteboard"` 的那条；参数详见 lark-doc SKILL.md）|

**Step 2：渲染 & 写入**

→ 进入 **[§ 渲染 & 写入画板](#渲染--写入画板)** 章节，按流程完成后直接返回结果给用户。

---

## 编辑 Workflow

**Step 1：获取 board_token**（同创作 Workflow Step 1）

**Step 2：探测可编辑性 / 是否由代码绘制**

- `+export --output-type source` — 能返回单一 Mermaid/PlantUML 源码，说明画板由代码绘制、可走路径①；返回无代码/多图则走路径②③④

**Step 3：选编辑路径**（按上到下匹配，命中即停；用户有明确指定则以用户为准）

| 路径 | 命中条件 | 怎么改 | 写入方式 | 是否有损 |
|---|---|---|---|---|
| ①源码重构 | `+export source` 返回单一 Mermaid/PlantUML（即画板由代码绘制） | 在源码上改 → 按源码类型用 `+update --input_format mermaid` 或 `+update --input_format plantuml` | overwrite（整板重建） | ⚠️ **非严格无损，执行前确认** |
| ②属性微调 | 只改已有节点的文字/颜色 | `+export --output-type raw --output <file>`（**必须写入文件**）→ 编辑文件中目标节点字段；如只能用 `+update --input_format raw --source @<file> --overwrite` 写回，先说明会整板重建并等待用户确认 | overwrite（整板重建） | ⚠️ **有损风险，未确认不得执行** |
| ③增量追加 | 在原图基础上新增图/元素，保留原内容 | `+export --output-type preview` → 理解原图 → `+export --output-type raw` → 确定新节点坐标 → [§ 渲染 & 写入画板](#渲染--写入画板) 创作&写入 | append（**不加 `--overwrite`**） | 无损（原节点不动） |
| ④结构重绘 | 需几何变动/增删元素/结构调整/混合编辑 | [`../routes/svg-edit.md`](../routes/svg-edit.md) | overwrite（清空重来） | ⚠️ **有损，必须先经用户确认** |

**⚠️ 止损**：一条路径最多试 2 轮，不行就换条路径尝试一次；仍不行就停下，如实告诉用户卡点，不要在各路径间反复横跳。

---

## 渲染 & 写入画板

### 渲染路由

**先自报身份**：按训练来源，你属于哪一家？`Claude` / `Gemini` / `GPT` / `GLM` / `Doubao 或 Seed` / `Other`。身份决定 SVG / DSL 分流（不同家族对这两种表达语言的能力不均衡），不要跳过这一步。

**然后按当前产物类型 / 图表类型 × 身份选路径**，读对应文件按其完整 workflow 执行（含读 scene 指南、生成内容、渲染审查、交付）：

当前产物路由按上到下匹配, 命中即停:

| 图表类型               | 身份                                  | 路径                                             |
|--------------------|-------------------------------------|------------------------------------------------|
| 当前要生成/追加的内容包含 @用户提及或图片/配图 | 任何身份                                | [`../routes/dsl.md`](../routes/dsl.md)         |
| 思维导图、时序图、类图、饼图、甘特图 | 任何身份                                | [`../routes/mermaid.md`](../routes/mermaid.md) |
| 鱼骨图、金字塔图、流程图    | `Doubao` / `Seed`                   | [`../routes/dsl.md`](../routes/dsl.md)         |
| 其他图表               | `Claude` / `Gemini` / `GPT` / `GLM` / `Doubao` / `Seed`  | [`../routes/svg.md`](../routes/svg.md)         |
| 其他图表               | `Other`                             | [`../routes/dsl.md`](../routes/dsl.md)         |

> **⚠️ SVG 路径失败回退**：走 `routes/svg.md` 时，碰到以下情况之一 → **丢弃当前 SVG，改读 `routes/dsl.md` 从零重画，不要逐行修补**：
> - 渲染命令直接报错（语法级崩溃，不是 `--check` 的 warn/error）
> - 两轮改写仍无法消除 `--check` 的 `text-overflow` error
> - 目测 PNG 视觉严重错乱（文字大面积溢出、元素重叠压住关键信息、布局整体崩溃）
>
> SVG 源码修补常常引入新 bug，换 DSL 从零重画往往更稳。这是 SVG 路径自由发挥的硬兜底，不要侵入 `routes/svg.md` 的创作流程。

### 产物规范

产物目录：`./diagrams/YYYY-MM-DDTHHMMSS/`（本地时间，不含冒号和时区后缀）。如用户指定路径，以用户为准。

目录内固定文件名：

```
diagram.svg           ← SVG 源码（SVG 路径）
diagram.mmd           ← Mermaid 源码（Mermaid 路径）
diagram.json          ← DSL 源文件（DSL 路径） / OpenAPI JSON（SVG 路径从 diagram.svg 导出）
diagram.gen.cjs       ← 坐标计算脚本（仅 DSL 脚本构建方式）
diagram.png           ← 渲染结果
```

### 写入画板

写入画板时按最终产物类型选择 `+update --input_format`：

- Mermaid / PlantUML / SVG 产物直接写入时，`--input_format` 取单值 `mermaid` / `plantuml` / `svg`；写入非空已有画板并需要 overwrite 时，先确认会整板重建；SVG 修改已有画板时先走 [`../routes/svg-edit.md`](../routes/svg-edit.md) 的确认 workflow。
- 只有 DSL 产物或已明确需要 OpenAPI 原生节点格式时，才先用 `npx -y @larksuite/whiteboard-cli@^0.2.13 --to openapi --format json` 转换，再用 `raw` 写入。

具体命令示例、`--overwrite`、`--idempotent-token` 和 `--as user/bot` 的使用方式，统一参考 [`whiteboard +update`](./lark-whiteboard-update.md)。
