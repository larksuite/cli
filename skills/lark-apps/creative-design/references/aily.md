# Aily 工具参考

本文档列出 [`../creative-design.md`](../creative-design.md) 所依赖的 harness 专属工具，供你在 **Aily** 中运行时使用。主提示词只命名能力（"向用户提问"、"展示文件"等）；本文档给出 Aily 的调用方式。通用工具（`Bash`、文件读/写/编辑、grep/glob 搜索）在任何环境都相同，不在此覆盖。

## Web 工具 → Aily 对应项

上游提示词引用了一些在 Aily 中并不存在的 Claude.ai web 工具。无论出现在行文还是代码里，一律按下表替换：

| Web 工具 | Aily 对应项 |
|---|---|
| `ask_user_question` | `ask_user`（向用户抛出结构化决策问题；先问，等用户答复后再继续）。 |
| `done`、`fork_verifier_agent` | 用 `submit` 交付结果并给出文件路径。 |
| `write_file`（及其 `asset:` 参数） | Aily 的「创建/编辑本地文件」工具。不存在 asset review pane；舍弃这一概念。 |
| `copy_files` | `Bash cp`。 |
| `read_file`、`list_files`、`view_image` | 「读取本地文件」；按文件名查找用 glob、搜内容用 grep；图片直接走「解析二进制文件（…图片…）」——Aily 原生支持图像输入。 |
| `show_to_user` | 用 `submit` 交付并给出绝对本地文件路径。 |
| `eval_js`、`eval_js_user_view`、`run_script` | 脚本用 `Bash`。 |
| `web_fetch`、`web_search` | `fetch`、`web_search`。用于时效性事实、内容素材补充或用户要求的查询。 |
| `generate_image` | `aily-image-generate_workbench`（Seedream V4.5 模型）：支持文生图、图生图（给参考图）、信息图（infographic）、图片编辑、组图（一次生成多张风格统一、角色连贯的图像序列）。 |
| `search_images` | `doubao_image_search`（按关键词搜索图片，适合找参考图、素材图）。 |
| `copy_starter_component` | `Bash cp <本 skill 所在目录>/starter-components/<file> .`（cwd 通常是应用项目目录而非 skill 目录，需用 skill 目录实际路径；或读取后改编）。 |
| 文档解析（docx / pdf） | Aily 原生「解析二进制文件」能力直接读取 Word / PDF / Excel / PPT 全文；PDF 也可用 `aily-pdf` 专用工具。 |
| `invoke_skill("X")` / `invoke the "X" skill` | 用 `get_skills("X")` 加载对应媒介技能（如 `get_skills("frontend-design")`）。这些技能同时以本地文件形式随本 skill 附带在 `references/<X>.md`，`get_skills` 取不到时直接读该文件。 |

## 提出澄清性问题

用 `ask_user` 提出聚焦的结构化问题——它把用户的决策内联返回，先问、等答复后再继续。它最适合高影响力的承重决策：交付格式、保真度、设计上下文、参考应用、变体数量。一轮提问保持简明、可执行。不要虚构假的工具名。

## 交付与发布

- 用 `submit` 提交交付结果，并给出绝对本地文件路径。
- 产物完成并提交后，按 [`../creative-design.md`](../creative-design.md)「发布」一节发布到妙搭——交付给用户的可分享链接是 `+release-get` 返回的 `online_url`。

## Aily 专属注意事项

- **优先用专用工具而非手搓。** 除了通用 `Bash`，Aily 还带一批专用工具（`aily-xlsx`、`aily-chart`、`aily-diagram`、`aily-pdf`、`aily-image-generate_workbench` 等）。涉及表格、图表、流程图、PDF、图像生成时，优先用对应专用工具，而不是用 `Bash` 从零脚本化。
- **图像素材优先走生成 / 搜索。** [`../creative-design.md`](../creative-design.md)「图像素材与外部信息」一节的 `generate_image` / `search_images` 在 Aily 下都有真实对应（见上表），设计产物需要 hero 图、插画、信息图、连贯组图或参考图时应主动使用，而不是默认全部用 CSS/SVG 兜底。搜索到 / 生成的图片先落到本地，再用 `lark-cli apps +file-upload` 上传、在代码中引用返回的远端 URL，不提交 git。
- `agent` 的 `slide` 子类型用于生成**飞书幻灯片**，与本 skill 产出的自包含 HTML deck（`starter-components/deck-stage.js`）是两条不同路径，不要混用——本 skill 的 deck 始终是 HTML。
- 交付统一走 `submit`；需要跨轮次保留项目上下文时可用 `aily-work-memory`。
