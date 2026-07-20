# Claude Code 工具参考

本文档列出 [`../creative-design.md`](../creative-design.md) 所依赖的 harness 专属工具，供你在 **Claude Code** 中运行时使用。主提示词只命名能力（"向用户提问"、"展示文件"等）；本文档给出确切的 Claude Code 工具、签名与调用方式。通用工具（`Bash`、`Read`/`Write`/`Edit`/`Glob`、`gh`）在任何环境都相同，不在此覆盖。

## Web 工具 → Claude Code 工具对照表

上游提示词引用了一些在 Claude Code 中并不存在的 Claude.ai web 工具。无论出现在行文还是代码里，一律按下表替换：

| Web 工具 | Claude Code 对应项 |
|---|---|
| `ask_user_question` | `AskUserQuestion`（答案内联返回；每次最多 4 个问题，需要更多就再调用一次） |
| `done`、`fork_verifier_agent` | `SendUserFile` 发送交付物并给出文件路径 |
| `write_file`（及其 `asset:` 参数） | `Write`——完全舍弃 "asset review pane" 这一概念 |
| `copy_files` | `Bash cp` |
| `read_file`、`list_files`、`view_image` | `Read`（也能渲染图像）、`Glob` / `Bash ls`、`Grep` |
| `show_to_user` | `SendUserFile`（自包含文件也可用 `open <path>`） |
| `eval_js`、`eval_js_user_view`、`run_script` | `Bash` |
| `web_fetch`、`web_search` | `WebFetch`、`WebSearch` |
| `generate_image` | 无内置对应。会话中若接入了图像生成 MCP/工具则使用；否则跳过 AI 生图，用内联 SVG / CSS 图形兜底，并在交付说明中注明。 |
| `search_images` | 无专用对应。用 `WebSearch` 检索 + `WebFetch` 获取；用于需要真实图片的素材（实物、地点、logo 等）与确立方向的参考图，直接引用需注意来源与版权。 |
| `copy_starter_component` | `Bash cp <本 skill 所在目录>/starter-components/<file> .`（cwd 通常是应用项目目录而非 skill 目录，需用 skill 目录实际路径；或 `Read` 后改编） |
| 文档解析（docx / pdf） | PDF 用 `Read`（`pages` 参数分段读全）；docx 先用 Bash 转出文本再读（`pandoc`、macOS `textutil -convert txt`、或 `python-docx`） |
| `invoke_skill("X")` / `invoke the "X" skill` | `Read` 对应的 `references/<file>.md`（媒介技能与本文件同在 `references/` 目录） |

## AskUserQuestion（澄清性提问）

替代 `ask_user_question`。`AskUserQuestion` **把用户的答案内联返回**——先问，等用户答复后再继续。每次调用最多展示 4 个问题；大型新项目先问一轮聚焦的问题，不够就再补一次调用。

- 记忆中的偏好可以作为问题里的*建议*默认值给出，但仍须由用户确认。
- 优先用它，而不是在回复里用文字列点罗列选项。
- 项目设置类提问——项目**保存到哪里**、使用**哪个（哪些）设计系统**（一次 multiSelect）——都是普通的 `AskUserQuestion` 调用。

## 交付与发布

- 用 `SendUserFile` 发送交付物并给出文件路径（读取文件**并不会**把它展示给用户）。
- 产物完成并提交后，按 [`../creative-design.md`](../creative-design.md)「发布」一节发布到妙搭——交付给用户的可分享链接是 `+release-get` 返回的 `online_url`。
