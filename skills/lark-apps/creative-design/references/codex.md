# Codex Agent 工具参考

本文档列出 [`../creative-design.md`](../creative-design.md) 所依赖的 harness 专属工具，供你在 **Codex Agent** 中运行时使用。主提示词只命名能力（"向用户提问"、"展示文件"等）；本文档给出 Codex 的调用方式。通用工具（shell、文件读/写/编辑/搜索、`gh`）不在此覆盖。

## Web 工具 → Codex 对应项

| Web 工具 | Codex 对应项 |
|---|---|
| `ask_user_question` | 在 Codex Plan Mode 下，若 `functions.request_user_input` 可用则使用它；否则在聊天中提出简明问题并等待用户答复。 |
| `done`、`fork_verifier_agent` | 在最终回复中呈现交付物的文件路径。 |
| `write_file`（及其 `asset:` 参数） | Codex 的常规文件编辑工具。不存在 asset review pane；舍弃这一概念。 |
| `copy_files` | Shell `cp`。 |
| `read_file`、`list_files`、`view_image` | Codex 的常规文件读取/搜索工具。 |
| `show_to_user` | 提供绝对本地文件路径；有帮助时，用 Markdown 以绝对路径嵌入图片。 |
| `eval_js`、`eval_js_user_view`、`run_script` | 脚本用 Shell。 |
| `web_fetch`、`web_search` | 若存在则用 Codex 的 web 工具；用于时效性事实、内容素材补充或用户要求的网络查询。 |
| `generate_image` | 无内置对应。会话中若接入了图像生成工具则使用；否则跳过 AI 生图，用内联 SVG / CSS 图形兜底，并在交付说明中注明。 |
| `search_images` | 无专用对应。若有 web 工具则用其检索图片，用于需要真实图片的素材与确立方向的参考图；没有就跳过。 |
| `copy_starter_component` | Shell `cp <本 skill 所在目录>/starter-components/<file> .`（cwd 通常是应用项目目录而非 skill 目录，需用 skill 目录实际路径；或读取后改编）。 |
| 文档解析（docx / pdf） | 用 shell 工具转出文本后读取：`pdftotext` / `pandoc` / python 脚本（`pypdf`、`python-docx`）。 |
| `invoke_skill("X")` / `invoke the "X" skill` | 阅读对应的 `references/<file>.md`（媒介技能与本文件同在 `references/` 目录）。 |

## 提出澄清性问题

当 Codex 处于 **Plan Mode** 且 `functions.request_user_input` 可用时，用它来提出聚焦的结构化问题。它最适合高影响力的设计决策，如范围、保真度、设计上下文、参考应用、变体数量。

若 `request_user_input` 不可用，或会话不在 Plan Mode，就直接在聊天中问同样的问题并等待用户回答。一轮提问保持简明、可执行。不要虚构假的工具名。

## 交付与发布

- 在最终回复中给出交付物的绝对本地文件路径。
- 产物完成并提交后，按 [`../creative-design.md`](../creative-design.md)「发布」一节发布到妙搭——交付给用户的可分享链接是 `+release-get` 返回的 `online_url`。
