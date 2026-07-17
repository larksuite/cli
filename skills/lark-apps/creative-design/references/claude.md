# Claude Code 工具参考

本文档列出 `SKILL.md` 所依赖的 harness 专属工具，供你在 **Claude Code** 中运行时使用。主提示词只命名能力（"向用户提问"、"预览"、"截图"、"调试"）；本文档给出确切的 Claude Code 工具、签名与调用方式。通用工具（`Bash`、`Read`/`Write`/`Edit`/`Glob`、`gh`）在任何环境都相同，不在此覆盖。

## Web 工具 → Claude Code 工具对照表

上游提示词引用了一些在 Claude Code 中并不存在的 Claude.ai web 工具。无论出现在行文还是代码里，一律按下表替换：

| Web 工具 | Claude Code 对应项 |
|---|---|
| `ask_user_question` | `AskUserQuestion`（答案内联返回；每次最多 4 个问题，需要更多就再调用一次） |
| `done`、`fork_verifier_agent` | `SendUserFile` + Claude Preview MCP；深度检查用 `Agent` 子代理（提示词：[`../agents/fork-verifier-agent.md`](../agents/fork-verifier-agent.md)）——见下文"验证与调试" |
| `write_file`（及其 `asset:` 参数） | `Write`——完全舍弃 "asset review pane" 这一概念 |
| `copy_files` | `Bash cp` |
| `read_file`、`list_files`、`view_image` | `Read`（也能渲染图像）、`Glob` / `Bash ls`、`Grep`；对图像文件使用 `Read` 之前，先运行下文的 vision probe |
| `show_to_user` | `SendUserFile`（自包含文件也可用 `open <path>`）；对最终交付物，还要给出已服务的 `http://localhost:<port>/...` URL。只有 vision probe 通过后才可展示截图；否则只提供截图文件路径而不读取它（见"展示文件与预览"）。 |
| `eval_js`、`eval_js_user_view`、`run_script` | `Bash`；页面内 JS 用 Claude Preview MCP 的 `preview_eval` |
| `web_fetch`、`web_search` | `WebFetch`、`WebSearch` |
| `copy_starter_component` | `Bash cp starter-components/<file> .`（或 `Read` 后改编） |
| `invoke_skill("X")` / `invoke the "X" skill` | `Read` 对应的 `built-in-skills/<file>.md` |

## AskUserQuestion（澄清性提问）

替代 `ask_user_question`。`AskUserQuestion` **把用户的答案内联返回**——先问，等用户答复后再继续。每次调用最多展示 4 个问题；大型新项目先问一轮聚焦的问题，不够就再补一次调用。

- 记忆中的偏好可以作为问题里的*建议*默认值给出，但仍须由用户确认。
- 优先用它，而不是在回复里用文字列点罗列选项。
- 项目设置类提问——项目**保存到哪里**、使用**哪个（哪些）设计系统**（一次 multiSelect；见 [`use-design-system.md`](../built-in-skills/use-design-system.md)）——都是普通的 `AskUserQuestion` 调用。

## 展示文件与预览

要把交付物呈现给用户，用 `SendUserFile` 并传入文件路径（任何文件类型都适用——HTML、图像、文本）。读取文件**并不会**把它展示给用户。

**对最终的设计/原型交付物，把预览当作交付的一部分，而不仅是内部自查。** Claude Code 没有可以打开的、用户可见的共享浏览器（Claude Preview MCP 由 agent 驱动），所以要通过交接让结果可见：用 `SendUserFile` 发送交付物，并给出已服务的 `http://localhost:<port>/<file>.html` URL，让用户在自己的浏览器里打开并与实时原型交互。若 vision probe 通过，再展示一张最终的 `preview_screenshot`（它会内联渲染在对话记录里）。若探测未通过，把截图存盘、只报告其路径，不要读取或嵌入它。这一步在验证完成后做，除非用户要求不做。

在浏览器中打开原型——无论是给用户交互，还是你自己预览/截图——**始终通过 HTTP 提供服务并加载 `http://localhost:<port>/<file>.html` URL；不要直接从 `file://` 打开 HTML。** 多文件原型（加载 `<script type="text/babel" src="…jsx">` 组件的 HTML 入口）只能通过 HTTP 工作——浏览器会拦截跨源的本地脚本读取——自包含单文件也走同一个已服务的 URL，保证预览与截图始终一致、可靠。

为整个 `designs/` 目录只起一次服务（所有项目共用一个服务器）并复用。通过 Claude Preview MCP 预览，它按 `.claude/launch.json` 中的命名配置提供服务：定义单个 `designs` 服务器来服务整个 `designs/` 目录（`python3 -m http.server 4311 --directory designs`），让所有项目共享同一个服务器。

## 视觉输入探测（vision probe）

**每个会话只探测一次**——模型/提供方不会在会话中途更换，所以缓存结论，并在之后的每个设计任务里复用，而不是反复探测。在第一个会把图像字节送入主对话的动作之前完成探测：对 PNG/JPG/WebP 调用 `Read`、调用 `preview_screenshot`，或让子代理对截图做视觉判断。

1. 使用本 skill 随附的已提交探针图——一个带深色 X/边框的彩色小方块。无需生成或写入任何东西；只要解析出它的绝对路径：

   ```text
   <skill>/agents/assets/vision-probe.png
   ```

2. 用 [`../agents/vision-probe-agent.md`](../agents/vision-probe-agent.md) 中的提示词生成一个 `Agent` 子代理，只传入那个绝对路径。让它运行在**与本会话相同的模型/提供方**上（即默认设置），其结论才能反映主 agent 的能力。这次探测是有意隔离的：拒绝图像输入的提供方应当在这个用完即弃的子任务里失败，而不是等真实设计截图已经进入主任务之后才失败。
3. 只有最终回复严格等于 `VISION_OK` 才视为支持图像输入。其他任何结果——`VISION_UNSUPPORTED`、Agent/工具报错、没有可用的最终回复、或夹带多余文字——都意味着本会话余下时间进入**非视觉模式**。

在非视觉模式下，不要对 PNG/JPG/WebP 文件调用 `Read`，也不要调用 `preview_screenshot` 或任何会把图像内容返回给模型的工具。你仍可以用 Chrome/Playwright/无头浏览器命令把截图文件写到磁盘；报告路径，让用户自行打开。

## 验证与调试

交付物就绪后，呈现它（`SendUserFile`），通过已服务的 URL 预览，确认它干净加载，并在结束前修复所有错误。用户始终应当落在一个不会崩溃的视图上。

通过 Claude Preview MCP 预览：

1. `mcp__Claude_Preview__preview_start`，参数 `{name: "designs"}`（`.claude/launch.json` 里的 `designs` 配置）。
2. 打开 `http://localhost:<port>/<file>.html`。
3. 用 `mcp__Claude_Preview__preview_console_logs` 捕捉 JS 错误。
4. 任何截图检查之前先运行 vision probe。若返回 `VISION_OK`，用 `mcp__Claude_Preview__preview_screenshot` 检查布局；若未通过，跳过视觉截图检查，改做下文的文本检查。
5. 交付物就绪后，交接结果：用 `SendUserFile` 发送文件，并给出已服务的 URL 让用户直接打开交互。若使用了非视觉模式，要说明：当前模型/提供方不接受图像输入、视觉审查被跳过、截图只保存为文件路径。

非视觉模式下，用文本与 DOM 证据来验证：确认 HTTP URL 能加载、console 日志没有阻断性错误、预期的根元素存在、主容器宽高非零、可见文本存在、网络/资源加载失败不存在或已有解释。检查空白页时，用页面内 JS——如 `document.body.innerText.trim()`、`document.querySelectorAll('*').length`、关键元素的 `getBoundingClientRect()` 值——而不是截图。

对深度或定向检查（"screenshot and check the spacing"），先运行 vision probe。若返回 `VISION_OK`，生成一个 `Agent` 子代理去加载文件、截图、探查 JS 并回报——当你不想让这些占用自己的上下文时很有用。使用 [`../agents/fork-verifier-agent.md`](../agents/fork-verifier-agent.md) 中的提示词，传入项目目录、文件路径、已服务的 URL，再加一句明确说明"图像输入受支持"。若探测未通过，不要让子代理检查截图；改用上文的文本与 DOM 检查，并告诉用户视觉审查被跳过。

**预览 harness 的坑（React + Babel 原型）**——这些是 Claude Preview MCP 的怪癖，不是你代码的问题：

- `preview_click` 够不到 React 委托的 `onClick`（React 18 `createRoot` 从根容器做事件委托）。要触发处理器，用 `preview_eval`：找到节点，读取其 `__reactProps$*` 键，然后调用 `el[propKey].onClick({stopPropagation(){},preventDefault(){}})`。真实浏览器点击没有问题；这只发生在 harness 层。
- 全局 `keydown` 监听器**可以**通过 `window.dispatchEvent(new KeyboardEvent('keydown',{key:'k',metaKey:true,bubbles:true}))` 触发——用它来测试 ⌘K / Esc / 快捷键。
- 页面内 `location.reload()` 或反复自定义 resize 之后，截图表面会失去同步（窗口缩成角落里的小块）。用 `preview_resize` 先切到某个预设尺寸再切回你的尺寸即可重新同步；优先用 `location.href = …` 而不是 `reload()`。

**若 preview MCP 不可用，**按文件类型降级。完全自包含的单文件可以用 `open <path>`（`file://`）打开；多文件原型（`<script src="…jsx">`）在 `file://` 下**不会**加载，需要 HTTP——自己启动服务器（`python3 -m http.server 4311 --directory .`）并打开 URL，或生成一个 `Agent` 去验证。绝不把用户留在一个组件静默加载失败的视图上。
