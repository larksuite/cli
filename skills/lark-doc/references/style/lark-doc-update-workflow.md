# 改写增强工作流

用户提供已有文档链接或 token，需要改写、润色、补充或重排版时，遵循本工作流。

## 核心方法论 — Code-Act Loop
通过自适应的 **Code-Act Loop** 驱动文档改写，而非固定模板式的工作流。每次任务都循环执行：
1. **Plan（规划）** — 根据用户目标和文档当前状态，评估下一步该做什么
2. **Execute（执行）** — 运行相应的 `lark-cli docs` 命令，或 **spawn** Agent 子任务并行推进
3. **Observe（观察）** — 检查命令输出，验证正确性，确认内容是否满足用户目标
4. **Iterate（迭代）** — 如需调整，回到 Plan 继续循环

## 核心原则：精准手术优于全量覆盖
1. **精准手术**：只改用户指定的 block，不改其他 block。
2. **全量覆盖**：如果用户明确要改整篇，才用 `overwrite` 命令。
3. **保真约束**：改写时原文里的 `<cite type="user">`（@人）、`<cite type="doc">`（@文档）、`<img>`、`<source>`、`<whiteboard>`、`<sheet>`、`<bitable>`、`<synced_reference>` 等行内组件和资源块一律原样保留（含所有 token / user-id / doc-id 属性），不许替换成纯文本姓名、链接或占位符。

## 工作流程

### 步骤一：分析与画板识别（串行）

1. **选择读取范围**（节省上下文的关键）：
   - 用户只改某一节 / 文档较大 → 先 `docs +fetch --api-version v2 --scope outline --max-depth 2` 拿目录，再 `docs +fetch --api-version v2 --scope section --start-block-id <目标标题id> --detail with-ids` 精读该节（`section` 会自动展开到下一个同级/更高级标题前，不用手动算结束 block id）
   - 需要精确跨节区间 → `docs +fetch --api-version v2 --scope range --start-block-id xxx --end-block-id yyy`（或 `--end-block-id -1` 读到末尾）
   - 用户只给了模糊关键词 → `docs +fetch --api-version v2 --scope keyword --keyword xxx --context-before 1 --context-after 1 --detail with-ids`
   - 用户明确要改整篇 → `docs +fetch --api-version v2 --detail with-ids`
   - 详见 [`lark-doc-fetch.md`](../lark-doc-fetch.md) "意图引导：选择正确的 --scope"
2. 系统性评估：用户想改什么、现有文档风格是什么、哪些内容需要保留、哪些问题影响理解
3. **画板意图识别**：逐章节扫描，按 `lark-doc-style.md`「画板意图识别」表判断哪些段落的信息适合用图表达。重要信息优先画板化，记录需要插图的章节（block ID）、推荐画板类型、mermaid/SVG路径和源内容片段
4. 向用户简要说明改进计划（包含识别出的画板机会）

### 步骤二：定向改写（并行 Agent）

5. **优先处理步骤一识别出的画板候选段落**：
   参考 [lark-doc-whiteboard.md](../lark-doc-whiteboard.md)中的方式，插入图表画板。
6. Spawn 内容改写 Agent 在不重叠的章节上并行改进，各 Agent 收到文档 token 和特定 block ID：
   - 沿用或轻微调整已有文档风格，除非用户要求彻底重排版
   - 可以通过重写段落、调整标题、拆分列表、补表格/分栏/callout 等方式提升可读性
   - 富 block 是可选表达手段，不因固定比例而添加；画板类需求只走第 5 步

### 步骤三：验证（串行）

7. 获取更新后文档局部内容，检查是否符合用户目标和已有风格
8. 检查是否满足用户目标并保留原有关键内容；如仍有明显问题则定向修正，向用户呈现结果

## Agent 子任务要求

内容改写 Agent 必须收到：文档 token、章节范围（标题/block ID）、`lark-doc-xml.md` 路径、用户目标/风格要求、具体的 `docs +update` command 和 `--block-id`。只有在需要使用富 block 或用户要求美化时，才提供 `lark-doc-style.md` 路径。

Mermaid 图由主 Agent 直接插入 `<whiteboard type="mermaid">...</whiteboard>`，无需 SubAgent。

SVG SubAgent 必须收到：文档 token、插入位置（标题/block ID）、图表目标、源内容片段、`lark-doc-xml.md` 路径，以及[lark-doc-whiteboard.md](../lark-doc-whiteboard.md) 中的 "SVG 设计 Workflow" 指南。它只负责插入一个 `<whiteboard type="svg">...</whiteboard>`，不改其他正文，也不读取 `lark-whiteboard`。

已有画板更新 SubAgent 必须收到：board_token、图表目标、推荐画板类型、源内容片段、[`../../../lark-whiteboard/SKILL.md`](../../../lark-whiteboard/SKILL.md) 路径。它只负责写入画板，不改文档正文。

**上下文节省提示**：Agent 如需在自己负责的章节内重新读取内容，优先用 `docs +fetch --api-version v2 --scope section --start-block-id <章节标题id>`（自动覆盖整节），或 `--scope range --start-block-id xxx --end-block-id yyy` 精确区间，只拉自己的章节，不要重复拉全文。
