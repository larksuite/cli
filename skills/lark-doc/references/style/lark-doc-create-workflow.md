# 从零创作工作流

用户提供主题、需求或简要说明，需要生成一份新的飞书文档时，遵循本工作流。

## 核心方法论 — Code-Act Loop

通过自适应的 **Code-Act Loop** 驱动文档创作，而非固定模板式的工作流。每次任务都循环执行：

1. **Plan（规划）** — 根据用户目标和文档当前状态，评估下一步该做什么
2. **Execute（执行）** — 运行相应的 `lark-cli docs` 命令，或 **spawn** Agent 子任务并行推进
3. **Observe（观察）** — 检查命令输出，验证正确性，确认内容是否满足用户目标
4. **Iterate（迭代）** — 如需调整，回到 Plan 继续循环

循环在文档达到质量标准且满足用户需求时结束。不要试图一次性产出完美内容——迭代打磨效果更好。根据用户实际需求灵活决定文档结构和版块，而不是套用固定模板。


## 典型 Code-Act Loop 流程

### 步骤一：规划与初始创建（串行）

1. 分析用户需求：受众、目的、范围
2. 设计大纲：根据任务自然选择结构。可以是短文、纪要、FAQ、方案、报告、清单或其他形式；不要默认套固定章节、固定开头或固定富 block 配比
3. `docs +create --api-version v2` 创建文档。长文档可**只建骨架**：标题 + 各级标题 + 每节一句占位摘要；短文档可以一次写入完整内容
   - ⚠️ 创建较长文档时，**不要**一次性把完整章节内容塞进 `--content`。超长 `--content` 容易触发字符/参数限制。
   - 完整内容留到步骤二，由各 Agent 用 `block_insert_after --block-id <章节标题 block_id>` 分段写入。
   - ⚠️ **`@file` 路径限制**：`--content @file` 只接受当前工作目录下的相对路径，传绝对路径（如 `@/tmp/xxx.md`）会报 `unsafe file path`。需要落盘时，将文件写在 cwd 下，用完自行清理。

### 步骤二：分段撰写（并行 Agent）

4. Spawn Agent 并行撰写各章节。每个 Agent 需收到：
   - 文档 token、负责的章节范围、用户目标、目标读者和已有风格线索
   - `lark-doc-xml.md` 的完整路径（Agent 须先读取）；仅在需要使用富 block 或用户要求美化时提供 `lark-doc-style.md`
   - 使用 `block_insert_after --block-id <章节标题 block_id>` 写入对应章节内容

### 步骤三：整合审查与画板识别（串行）

5. `docs +fetch --api-version v2 --detail with-ids` 获取文档，审查整体效果
6. 评估内容是否满足用户目标：事实是否完整、结构是否清楚、语气是否匹配、是否保留必要素材
7. **画板意图识别**：逐章节扫描，按 `lark-doc-style.md`「画板意图识别」表判断是否有段落适合用图表达。重要信息优先画板化，记录需要插图的章节、推荐画板类型、mermaid/SVG 路径和用于画图的源内容

### 步骤四：画板处理与润色（并行 Agent）

8. **优先处理步骤三识别出的画板需求**：
   参考 [lark-doc-whiteboard.md](../lark-doc-whiteboard.md)中的方式，插入图表画板。
9. Spawn 内容改写 Agent 定向润色：
   - 文字密集且不易读时，优先拆段、改列表、增加小标题或调整顺序；只有确实存在行列数据、并列对比或强提醒信息时，才考虑 `<table>` / `<grid>` / `<callout>`
   - 需要明显分隔的主题可补充 `<hr/>`，不强制章节间都使用
   - 本地图片使用 `docs +media-insert` 插入


## Agent 子任务要求

内容改写 Agent 必须收到：文档 token、章节范围（标题/block ID）、`lark-doc-xml.md` 路径、用户目标/风格要求、具体的 `docs +update` command 和 `--block-id`。只有在需要使用富 block 或用户要求美化时，才提供 `lark-doc-style.md` 路径。

Mermaid 图由主 Agent 直接插入 `<whiteboard type="mermaid">...</whiteboard>`，无需 SubAgent。

SVG SubAgent 必须收到：文档 token、插入位置（标题/block ID）、图表目标、源内容片段、`lark-doc-xml.md` 路径，以及[lark-doc-whiteboard.md](../lark-doc-whiteboard.md) 中的 "SVG 设计 Workflow" 指南。它只负责插入一个 `<whiteboard type="svg">...</whiteboard>`，不改其他正文，也不读取 `lark-whiteboard`。

已有画板更新 SubAgent 必须收到：board_token、图表目标、推荐画板类型、源内容片段、[`../../../lark-whiteboard/SKILL.md`](../../../lark-whiteboard/SKILL.md) 路径。它只负责写入画板，不改文档正文。
