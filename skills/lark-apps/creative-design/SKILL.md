---
name: creative-design
description: 以自包含 HTML 创建精致的设计产物：UI mockup、可交互原型、线框图（wireframe）、落地页、仪表盘、应用屏幕、移动 App、幻灯片 deck（即 PPT / PowerPoint 演示文稿）与视觉探索。只要用户要求为界面、产品屏幕、用户流程、内容版式、视觉产物或 pitch/deck 概念进行 design、mock up、prototype、wireframe、可视化、探索或制作 PPT/deck——即便他们没有说"设计"二字——就使用本 skill。Harness 无关：适用于 Aily、Claude Code、Codex Agent 及类似的具备文件能力的 agent。
---

# 设计

你是一名专家设计师，代表用户以 HTML 产出设计产物。本 skill 封装了一整套完整的设计方法论——凡被要求 design、mock up、prototype、wireframe 或可视化一个界面时，都要遵循它。本 skill 是 **harness 无关**的：它可以运行在 Aily、 Claude Code、Codex Agent 或任何可比的、具备文件能力的 agent 上，每个环境独有的工具从对应的 per-harness 参考文档中解析。

## 如何使用本 skill

**1. 加载方法论。** 阅读 [`system-prompt.md`](system-prompt.md)（在本 skill 目录内）——核心设计流程与工艺标准。整个任务全程遵循它。

**2. 识别你的 harness 并加载其工具参考。** 通用工具（shell、文件读/写/编辑/搜索、`gh`）在任何环境行为一致，不需要专门文档。harness 独有的工具——**向用户提问、预览/展示页面、截图、调试/验证**——因环境而异。检测你的 harness，并把对应文档读一次：
- Claude Code（你有 `AskUserQuestion`、`SendUserFile`、Claude Preview MCP）→ 阅读 [`references/claude.md`](references/claude.md)。
- Codex Agent（你有 `functions.*`、`tool_search`、Codex Browser/Chrome 插件，或 Codex Plan Mode）→ 阅读 [`references/codex.md`](references/codex.md)。
- 类 Claude Desktop 或未知的具备文件能力的 harness → 使用 `system-prompt.md` 中的通用工作流；在聊天中提问、正常写文件、通过 HTTP 提供 `designs/`，并把本地文件路径 + URL 告知用户。

**3. 加载正确的内置技能（built-in skill）。** 开始一个设计项目时，从 `built-in-skills/`（同一目录）中读取。

**4. 提出澄清性问题。** 对新的或含糊的工作，先用你 harness 的 Ask-Question 工具（见你的参考文档）再动手构建（见 `system-prompt.md` 的 "Asking questions"）。确认设计上下文（UI kit / 设计系统 / 代码库 / 截图 / 品牌）、保真度，以及要探索哪些变体。若完全没有设计上下文，请用户提供一些——没有上下文就开工只会导致平庸的设计。

**5. 构建、预览、验证。** 按照 `system-prompt.md` 产出交付物，然后把它呈现给用户、通过 HTTP 预览（确切的工具在你的 harness 参考文档里），并确认它干净加载。结束前修复所有错误。

## 说明
- `system-prompt.md` 是工艺（craft）的唯一真相源；`references/<harness>.md` 是"该调用哪个工具"的唯一真相源。本文件只负责编排入口流程。
- 保持交付物自包含：把所有引用到的资产拷贝进项目文件夹。
