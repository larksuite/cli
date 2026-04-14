---
name: lark-whiteboard
version: 0.2.0
description: >
  飞书画板：查询/导出/创作/修改飞书云文档中的画板。支持 whiteboard-cli DSL、Mermaid、PlantUML 创作架构图、流程图等复杂图表并写入画板。
  当用户需要可视化架构、流程、组织关系、时间线、因果、对比等结构化信息时均适用，无论是否提及"画板"。
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli whiteboard --help"
---

> [!IMPORTANT]
> **执行前检查环境**：
> - 运行 `whiteboard-cli --version`，确认版本为 `0.2.x`；未安装或版本不符 → `npm install -g @larksuite/whiteboard-cli@^0.2.0`
> - 运行 `lark-cli --version`，确认可用。
> - 执行任何 `npm install` 前，**必须征得用户同意**。

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

---

## 先确认你的角色

> [!CAUTION]
> **[SubAgent] 被主 Agent 委派，已持有 board_token 和内容要求**
> → 跳过以下全部内容，直接跳至 **[§ 渲染 & 写入画板](#渲染--写入画板)** 章节执行。
> → 执行完毕后返回 `{ "board_token": "xxx", "status": "ok" }` 或 `{ ..., "status": "failed", "error": "原因" }`。

> **[主 Agent] 直接收到用户请求**
> → 继续往下读。

---

## 快速决策（主 Agent）

| 用户需求 | 行动 |
|---|---|
| 查看画板内容 / 导出图片 | [`+query --output_as image`](references/lark-whiteboard-query.md) |
| 获取画板的 Mermaid/PlantUML 代码 | [`+query --output_as code`](references/lark-whiteboard-query.md) |
| 检查画板是否由代码绘制 | [`+query --output_as code`](references/lark-whiteboard-query.md) |
| 修改节点文字/颜色（简单改动）| `+query --output_as raw` → 手动改 JSON → `+update --input_format raw` |
| 用户**已提供** Mermaid/PlantUML 代码，或明确指定用该格式 | 自己生成/使用代码 → [`+update --input_format mermaid/plantuml`](references/lark-whiteboard-update.md) |
| 绘制复杂图表（架构/流程/组织等）| → **[§ 创作 Workflow](#创作-workflow主-agent单个画板)** |
| 修改/重绘已有复杂画板 | → **[§ 修改 Workflow](#修改-workflow主-agent)** |

> **⚠️ 强制规范（通过 stdin 更新）**：
> 数据来源于本地文件时，**必须**使用 `--source - --input_format <格式>`。
> 例：`cat chart.mmd | lark-cli whiteboard +update <token> --source - --input_format mermaid`

## Shortcuts

| Shortcut | 说明 |
|---|---|
| [`+query`](references/lark-whiteboard-query.md) | 查询画板，导出为预览图片、代码或原始节点结构 |
| [`+update`](references/lark-whiteboard-update.md) | 更新画板，支持 PlantUML、Mermaid 或 OpenAPI 原生格式 |

---

## 创作 Workflow（主 Agent，单个画板）

> 此 workflow 用于**独立创作一个画板**（主 Agent 自己执行，无需开 subAgent）。
> 需要在文档中批量创建多个画板时，由 lark-doc 负责调度，见 [`lark-doc references/lark-doc-whiteboard.md`](../lark-doc/references/lark-doc-whiteboard.md)。

**Step 1：获取 board_token**

| 用户给了什么 | 怎么获取 |
|---|---|
| 直接给了 whiteboard token（`wbcnXXX`）| 直接使用 |
| 文档 URL 或 doc_id，文档中已有画板 | `lark-cli docs +fetch --doc <URL> --as user`，从返回的 `<whiteboard token="xxx"/>` 提取 |
| 文档 URL 或 doc_id，需要新建画板 | `lark-cli docs +update --doc <doc_id> --mode append --markdown '<whiteboard type="blank"></whiteboard>' --as user`，从响应 `data.board_tokens[0]` 取得（参数详见 lark-doc SKILL.md）|

**Step 2：渲染 & 写入**

→ 进入 **[§ 渲染 & 写入画板](#渲染--写入画板)** 章节，按流程完成后直接返回结果给用户。

---

## 修改 Workflow（主 Agent）

**Step 1：获取 board_token**（同创作 Workflow Step 1）

**Step 2：判断修改策略**

```
+query --output_as code
  ├─ 返回 Mermaid/PlantUML 代码
  │   → 在原代码上修改 → +update --input_format mermaid/plantuml
  ├─ 无代码（DSL 或其他方式绘制的画板）
  │   ├─ 只改文字/颜色 → +query --output_as raw → 手动改 JSON → +update --input_format raw
  │   └─ 重绘/结构调整 → +query --output_as image → 看图后进入 [§ 渲染 & 写入画板]
  └─ 用户有明确要求 → 以用户要求优先
```

---

## 渲染 & 写入画板

> [!NOTE]
> **[SubAgent] 从这里开始执行。**
> **[主 Agent 创作 Workflow Step 2] 也在这里继续。**

### 渲染路由

根据图表类型选择路径，读对应文件按其完整 workflow 执行（含读 scene 指南、生成内容、渲染审查、交付）：

| 图表类型 | 路径 |
|---|---|
| 思维导图、时序图、类图、饼图、甘特图 | [`routes/mermaid.md`](routes/mermaid.md) |
| 其他图表（Claude / Gemini / GPT / GLM）| [`routes/svg.md`](routes/svg.md) |
| 其他图表（Doubao / Seed / 其他）| [`routes/dsl.md`](routes/dsl.md) |

**图表类型速查**（不确定走哪条路时参考）：

| 图表类型 | routes 路径 | scene 指南 |
|---|---|---|
| 架构图（分层/微服务/前后端） | svg / dsl | [`scenes/architecture.md`](scenes/architecture.md) |
| 流程图（业务流/状态机/审批流） | svg / dsl | [`scenes/flowchart.md`](scenes/flowchart.md) |
| 组织架构图 | svg / dsl | [`scenes/organization.md`](scenes/organization.md) |
| 泳道图（跨角色/跨系统流程） | svg / dsl | [`scenes/swimlane.md`](scenes/swimlane.md) |
| 对比图（方案对比/功能矩阵） | svg / dsl | [`scenes/comparison.md`](scenes/comparison.md) |
| 鱼骨图（因果/根因分析） | svg / dsl | [`scenes/fishbone.md`](scenes/fishbone.md) |
| 飞轮图（增长飞轮/闭环链路） | svg / dsl | [`scenes/flywheel.md`](scenes/flywheel.md) |
| 金字塔图（层级结构/需求层次） | svg / dsl | [`scenes/pyramid.md`](scenes/pyramid.md) |
| 里程碑/时间线 | svg / dsl | [`scenes/milestone.md`](scenes/milestone.md) |
| 柱状图/条形图 | dsl | [`scenes/bar-chart.md`](scenes/bar-chart.md) |
| 折线图/趋势图 | dsl | [`scenes/line-chart.md`](scenes/line-chart.md) |
| 树状图（矩形树图/层级面积） | dsl | [`scenes/treemap.md`](scenes/treemap.md) |
| 漏斗图（转化漏斗/销售漏斗） | dsl | [`scenes/funnel.md`](scenes/funnel.md) |
| 思维导图/时序图/类图/饼图/甘特图 | mermaid | [`scenes/mermaid.md`](scenes/mermaid.md) |

### 产物规范

| 场景 | 产物目录 |
|---|---|
| 主 Agent 独立执行 | `./diagrams/YYYY-MM-DDTHHMMSS/`（本地时间，不含冒号和时区后缀） |
| SubAgent 被委派执行 | `./diagrams/board_{n}/`（n 由委派方传入，保证多个并行 subAgent 产物不冲突） |
| 用户指定路径 | 以用户为准 |

目录内固定文件名：

```
diagram.svg           ← SVG 源码（SVG 路径）
diagram.json          ← PageDetail JSON（SVG 路径）/ DSL JSON（DSL 路径）
diagram.gen.cjs       ← 坐标计算脚本（仅 DSL 脚本构建方式）
diagram.mmd           ← Mermaid 源码（Mermaid 路径）
diagram.png           ← 渲染结果
```

### 写入画板

> [!CAUTION]
> **写入前强制 dry-run**：向已有内容的画板写入时，必须先加 `--overwrite --dry-run` 探测。
> 输出含 `XX whiteboard nodes will be deleted` → 必须向用户确认后才能执行。

```bash
# 第一步：dry-run 探测
npx -y @larksuite/whiteboard-cli@^0.2.0 -i <产物文件> --to openapi --format json \
  | lark-cli whiteboard +update \
    --whiteboard-token <Token> \
    --idempotent-token <10+字符唯一串> \
    --overwrite --dry-run --as user

# 第二步：确认后执行
npx -y @larksuite/whiteboard-cli@^0.2.0 -i <产物文件> --to openapi --format json \
  | lark-cli whiteboard +update \
    --whiteboard-token <Token> \
    --idempotent-token <10+字符唯一串> \
    --yes --as user
```

> `--idempotent-token` 最少 10 字符，建议用时间戳+标识拼接（如 `1744800000-board-2`），SubAgent 并行时用 board_n 区分，避免重试导致重复写入。
> 如需应用身份上传，将 `--as user` 替换为 `--as bot`。

**SubAgent 完成后返回**：
- 成功：`{ "board_token": "<token>", "status": "ok" }`
- 失败：`{ "board_token": "<token>", "status": "failed", "error": "<原因>" }`
