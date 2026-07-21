# 画板创作/修改工作流

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

## 修改 Workflow

**Step 1：获取 board_token**（同创作 Workflow Step 1）

**Step 2：判断修改策略**

```
+query --output_as code
  ├─ 返回 Mermaid/PlantUML 代码
  │   → 在原代码上修改 → +update --input_format mermaid/plantuml
  ├─ 无代码（SVG/DSL 或其他方式绘制的画板）
  │   ├─ 需纯新增（思维导图、流程图、时序图、类图、饼图、甘特图）图表节点
  │   │    → +query --output_as image → 看图 → +query --output_as raw → 确定新节点坐标和层级  → [§ 渲染 & 写入画板]
  │   └─ 其他改动（几何变动/增删元素/结构调整/混合编辑等）
  │       → [`../routes/svg-edit.md`](../routes/svg-edit.md)（视觉高保真还原，大部分场景适用）
  └─ 用户有明确要求 → 以用户要求优先
```

---

## 渲染 & 写入画板

### 渲染路由

**先自报身份**：按训练来源，你属于哪一家？`Claude` / `Gemini` / `GPT` / `GLM` / `Doubao 或 Seed` / `Other`。身份决定 SVG / DSL 分流（不同家族对这两种表达语言的能力不均衡），不要跳过这一步。

**然后按图表类型 × 身份选路径**，读对应文件按其完整 workflow 执行（含读 scene 指南、生成内容、渲染审查、交付）：

按上到下匹配, 命中即停:

| 图表类型               | 身份                                  | 路径                                             |
|--------------------|-------------------------------------|------------------------------------------------|
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

- Mermaid / PlantUML / SVG 产物直接用对应的 `mermaid` / `plantuml` / `svg` 写入。
- 只有 DSL 产物或已明确需要 OpenAPI 原生节点格式时，才先用 `npx -y @larksuite/whiteboard-cli@^0.2.13 --to openapi --format json` 转换，再用 `raw` 写入。

具体命令示例、`--overwrite`、`--idempotent-token` 和 `--as user/bot` 的使用方式，统一参考 [`whiteboard +update`](./lark-whiteboard-update.md)。
