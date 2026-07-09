# 改写增强工作流

用户提供已有文档链接或 token，需要改写、润色、补充或重排版时，遵循本工作流。

## 核心方法论 — Code-Act Loop
通过自适应的 **Code-Act Loop** 驱动文档改写，而非固定模板式的工作流。每次任务都循环执行：
1. **Plan（规划）** — 根据用户目标和文档当前状态，评估下一步该做什么
2. **Execute（执行）** — 由主 Agent 自己运行 `lark-cli docs` 命令推进改写；仅画板渲染按需隔离到 SubAgent（见步骤二）
3. **Observe（观察）** — 检查命令输出，验证正确性，确认内容是否满足用户目标
4. **Iterate（迭代）** — 如需调整，回到 Plan 继续循环

## 核心原则：精准手术优于全量覆盖
1. **精准手术**：只改用户指定的 block，不改其他 block。
2. **全量覆盖**：如果用户明确要改整篇，才用 `overwrite` 命令。
3. **保真约束**：改写时原文里的 `<cite type="user">`（@人）、`<cite type="doc">`（@文档）、`<img>`、`<source>`、`<whiteboard>`、`<sheet>`、`<bitable>`、`<synced_reference>` 等行内组件和资源块一律原样保留（含所有 token / user-id / doc-id 属性），不许替换成纯文本姓名、链接或占位符。

## 工作流程

### 步骤一：分析与画板识别（串行）

1. **选择读取范围**（节省上下文的关键）：
   - 用户只改某一节 / 文档较大 → 先 `docs +fetch --scope outline --max-depth 2` 拿目录，再 `docs +fetch --scope section --start-block-id <目标标题id> --detail with-ids` 精读该节（`section` 会自动展开到下一个同级/更高级标题前，不用手动算结束 block id）
   - 需要精确跨节区间 → `docs +fetch --scope range --start-block-id xxx --end-block-id yyy`（或 `--end-block-id -1` 读到末尾）
   - 用户只给了模糊关键词 → `docs +fetch --scope keyword --keyword xxx --context-before 1 --context-after 1 --detail with-ids`
   - 用户明确要改整篇 → `docs +fetch --detail with-ids`
   - 详见 [`lark-doc-fetch.md`](../lark-doc-fetch.md) 中「选 `--scope`（读取范围）」小节
2. 系统性评估：用户想改什么、现有文档风格是什么、哪些内容需要保留、哪些问题影响理解
3. **画板识别**：逐章节扫描，判断是否有段落用图明显比文字更易懂（流程 / 架构 / 时间线 / 对比 / 占比等，见 `lark-doc-style.md` 的画板原则）。默认用文字，只有确需图示才记录需要插图的章节（block ID）、推荐画板类型、mermaid/SVG路径和源内容片段
4. 向用户简要说明改进计划（包含识别出的画板机会）

### 步骤二：定向改写（单 Agent 串行）

5. **优先处理步骤一识别出的画板候选段落**：读取并按 [lark-doc-whiteboard.md](../lark-doc-whiteboard.md) 选型和插入；正文本身不交给 SubAgent
6. 由主 Agent **顺序逐节**改写，**不按章节拆给并行 Agent**，避免上下文割裂、重复矛盾和全文级约束失效：
   - 沿用或轻微调整已有文档风格，除非用户要求彻底重排版
   - 优先通过重写段落、调整标题、补充小标题提升可读性；叙述内容保持成段，**不要默认改成列表**，只有确属并列要点 / 步骤才用列表（见 `lark-doc-style.md`）
   - 富 block 是可选表达手段，不因固定比例而添加，取舍遵循 `lark-doc-style.md` 的写作原则；画板类需求只走第 5 步

### 步骤三：验证（串行）

7. 获取更新后文档局部内容，检查是否符合用户目标和已有风格
8. 检查是否满足用户目标并保留原有关键内容。再按 `lark-doc-style.md` 的「写完自检」快速核对，发现问题则定向修正

### 步骤四：专项校验（按需执行）

9. 仅当用户预期需要校验字数时，才读取并执行 [`lark-doc-word-stat.md`](../lark-doc-word-stat.md) 的「字数遵循校验」；否则跳过本项，不读取该 workflow。若执行了专项校验，向用户呈现结果

**上下文节省提示**：主 Agent 改某节时如需重新读取，优先用 `docs +fetch --scope section --start-block-id <章节标题id>`（自动覆盖整节），或 `--scope range --start-block-id xxx --end-block-id yyy` 精确区间，只拉当前章节，不要重复拉全文。
