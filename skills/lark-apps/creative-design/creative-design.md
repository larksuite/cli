---
name: creative-design
description: 以自包含 HTML 创建精致的设计产物：UI mockup、可交互原型、线框图（wireframe）、落地页、仪表盘、应用屏幕、移动 App、幻灯片 deck（即 PPT / PowerPoint 演示文稿）、动画视频（motion graphics、产品演示 Demo 动画、数据动画）、可视化报告 / 信息图（infographic）/ 视觉长图与视觉探索。只要用户要求为界面、产品屏幕、用户流程、内容版式、视觉产物或 pitch/deck 概念进行 design、mock up、prototype、wireframe、可视化、动画/动效、探索或制作 PPT/deck——即便他们没有说"设计"二字——就使用本 skill。Harness 无关：适用于 Aily、Claude Code、Codex Agent 及类似的具备文件能力的 agent。
---

## 目录结构与运行环境
本 skill 附带以下资源，路径均相对于本文件所在目录：

- `references/<name>.md` — 媒介专属技能 prompt（如 `frontend-design.md`、`hi-fi-design.md`、`charts.md` 等；见文末「Skills 元信息」的完整列表）。与下方 harness 工具映射表同在 `references/` 目录。
- `starter-components/` — 现成的 HTML/JS/JSX 脚手架（`design-canvas.jsx`、`deck-stage.js`、`ios-frame.jsx`、`android-frame.jsx`、`tweaks-panel.jsx`、`macos-window.jsx`、`browser-window.jsx`、`animations.jsx`）。见下文「Starter Components」。
- `references/<harness>.md` — **harness 专属工具映射表**（`claude.md`、`codex.md`、`aily.md`）。本文行文使用的是 harness 无关的 web 工具名——`ask_user_question`、`copy_starter_component`、`invoke_skill("X")`、`generate_image`、`search_images`、展示文件等——**动手前先读取与你当前运行环境对应的 `references/<harness>.md`，把这些名字映射成你 harness 里的真实工具**。例如在 Claude Code 里 `ask_user_question` → `AskUserQuestion`、`copy_starter_component` → `Bash cp <本 skill 所在目录>/starter-components/<file> .`、`invoke_skill("X")` → `Read references/<file>.md`。
- `assets/index.html` — React + Babel 的 HTML 起步模板（锁定版本 script 标签 + `#root` 挂载点），见下文「React + Babel」。

## 工作流
1. 理解用户需求。对全新或含糊的工作，提出澄清性问题。弄清输出物、精细度（fidelity）、选项数量、约束条件，以及涉及的 UI kit 与品牌。
2. 探索所提供的资源。附件、文档链接、网页 URL 都要在动手前解析完（见「输入资料解析」）。
3. 列出 todo 清单。
4. 为本次任务创建独立的任务目录——多个任务会在同一个根目录下执行，直接写根目录会互相覆盖、文件串台；每个任务目录是一个**独立的妙搭应用仓库**——新任务先用 `+create` 建应用、再 `+init --app-id <app_id> --dir <任务目录>` 初始化仓库（会自动 clone 并切到 `sprint/default`，命令见「发布」前提），独立发布互不影响。把资源复制进任务目录，在其中创建交付物。用图片素材提升美观度与丰富度、或需要有依据的内容时，按「图像素材与外部信息」补充。
5. （如有）自检React + Babel路径是否正确；ReactDOM.createRoot 是否参数正确，对应元素是否存在
6. 收尾：提交你的改动。
7. 发布：把产物发布到妙搭拿到可访问链接（见下方「发布」）。写完不发布，用户拿不到线上链接。
8. 极其简短地总结——只讲注意事项与后续步骤，并给出发布后的可访问链接。

鼓励你并发调用文件探索工具以提升效率。

## 提问
默认基于用户给的信息、项目上下文和合理假设直接开始，不为收集偏好而打断。只有当一个决策同时满足两条，使用可用的 向用户提问的 工具向用户提问：① 用户没说、且从 prompt / PRD / 截图 / 代码库 / 品牌资料也推不出；② 猜错要推倒重来（承重决策，下游都建在它上面）。两条只要有一条不成立——能合理推断，或猜错只是局部返工——就直接做。

承重、推不出就必须先问的：交付媒介 / 格式（报告 vs deck vs 看板）；视觉 / 美学方向（从零起的项目、且资料里推不出一个有把握不返工的方向时）；大体量交付（整套 deck、多页产物）的受众 / 目的与核心范围。
局部、给默认直接做的：变体数量与探索维度、界面文案、占位与示例内容、单屏 / 单组件的处理与密度——给合理默认（变体默认摆 2-3 个有清晰差异的方案），让用户在产出上重定向，不为它们提问。

例如：

- "做一份关于 X 的报告／材料"但没说格式 → 媒介推不出且承重，先确认交付格式（幻灯片 vs. 视觉报告 vs. 仪表盘），再问格式相关的问题。
- 为附带的 PRD 做一套 deck → PRD 能推出受众 / 场景就直接做；只有受众、篇幅推不出且影响全局时才问。
- 用这份 PRD 为 Eng All Hands 做一套 10 分钟的 deck → 无需提问；信息已足够。
- 把这张截图变成交互原型 → 只有当图片无法说明预期行为时才提问。
- 做 6 页关于黄油历史的幻灯片 → 媒介、页数已定，直接开工；风格能从主题推断就定，推不出再问。
- 为我的外卖 app 的 onboarding 做一套原型 → 按常见 onboarding 流程直接做；只问会阻塞产出的承重问题。

当交付格式本身不明确时——用户只说了一个成果（"一份报告""材料""一份摘要"）却没说媒介——先解决格式，再讨论任何与格式相关的细节。

问出好问题至关重要。技巧：

- 通常一轮聚焦提问就够；把承重的未知一次问齐，不要挤牙膏式多轮打断。
- 只问推不出的；能从 PRD、截图、代码库、品牌资产、现有页面和用户原话推断的，先推断，并在产出里说明你的假设。

## 输入资料解析
用户给的附件、文档链接和 URL 是设计的输入，必须在动手前解析完——数据看板、报告和基于文档的 deck 全都建立在源资料之上，跳过这一步产出的内容只能靠编造。按输入形态处理：

- **数据文件（csv / json / xlsx）**——先看结构（列名、字段类型、行数）和样本行，再决定信息层级与图表选型；指标一律用脚本从源数据计算，不要目测。
- **压缩包（zip）**——先解压到临时目录，逐个查看内容物，再按各自类型处理。
- **文档（docx / pdf / 论文 / 需求文档）**——用当前 harness 的文档解析能力读取**全文**（映射见 `references/<harness>.md`；Aily 原生支持解析 Word / PDF 等二进制文件），不要只读开头就动手。
- **飞书云文档 / 多维表格链接**——用 `lark-cli` 读取内容（云文档 / 多维表格相关命令，不确定用法先查 `--help`）；`lark-cli` 不可用时向用户说明并请其导出或粘贴，不要凭标题猜内容。
- **网页 URL**——用 `web_fetch` 抓取全文后再产出；抓取失败就告知用户，不要凭 URL 和常识编写。

## 如何开展设计工作
动手前先读取 **`./references/frontend-design.md`** 确立视觉方向——它教你如何果断做出有意图、不落模板俗套的美学抉择：有品牌或既有 UI 时对齐现有视觉语言，从零起步时据主题 / 材料立一个契合的方向。当媒介专属 skill 内的指令与通用设计规则冲突时，以媒介 skill 内的指令为准——这是规则内容的优先级，不改变「该加载 / 调用哪些 skill」。

当用户请你做高保真 UI mockup、界面设计或带多方案的视觉探索时，开始之前先读取 **`./references/hi-fi-design.md`**——它涵盖了设计流程、获取设计上下文、提问以及呈现多个方案。

一次设计探索的输出是单个 HTML 文档。根据你所探索的内容选择呈现格式：

- **静态视觉 / 设计稿 / 多方案探索**（颜色、字体、单个元素、整屏 UI、流程关键帧）→ 通过 `starter-components/design-canvas.jsx` starter component 把各方案铺陈在画布上。除非用户明确要求可点击 / 可交互，否则不要把设计稿升级成点击原型。
- **用户明确要求可交互的流程或产品 demo** → 将整个产品做成高保真可点击原型，并把关键选项以 Tweak 形式暴露出来。可交互原型禁止使用 `starter-components/design-canvas.jsx`、`<DCArtboard>` 或画布外壳包裹；它应该作为真实应用界面直接运行。

这两者可以组合，但只限静态设计探索。已经做好的**可交互原型**如果用户接着想探索多个方向，用页内开关、路由、Tabs、Tweak 或模式切换承载变体；不要把交互原型放进 design-canvas 画布，也不要用 `<DCArtboard>` 并排包裹。

当用户要求新版本或改动时，把它们作为 TWEAKS 加到原件上；拥有一个可切换不同版本开关的主文件，优于拥有多个文件。

## 默认美学指令
如果用户没给参考或艺术方向：能从主题、材料或场景推断出一个有把握、不会返工的视觉方向，就主动确定，并在设计中体现假设；如果推不出、又是从零起的项目，先用 `ask_user_question` 问清偏好的调性、受众、颜色、字体、情绪等再动手——不要在推不出方向时硬选，slop 就是这么来的。

定下视觉方向后（无论是推断还是问来的），创建设计时遵循以下指引：

- **字体与排版。** 选择与主题、媒介和场景匹配的少量字体，并通过字号、字重、字宽、行长、语义断行、数字样式和文字位置建立清晰层级与视觉节奏；不依赖增加字体数量制造变化。
- **背景与色彩体系。** 确定主色调，并建立与主题协调的中性基底、主题色和必要的章节／语义色。背景不局限于纯黑、纯白或单一色调，可以根据内容属性、页面角色和叙事节点使用不同色调、主题色底、局部色域、图片或图形背景。
- **色彩一致性。** 一致性来自共享色板、字体、栅格、图形语言和明确的颜色关系，不要求所有页面使用相同背景。颜色变化应帮助识别章节、信息层级和重点，避免无语义地逐页随机换色。
- **强调色。** 使用数量克制、关系协调的强调色，并根据背景、信息层级和色彩语义调整明度与彩度。图表、状态和章节色需要清楚可区分，但应属于同一视觉体系。
- **中性色。** 黑、白、灰可以带有与主题协调的细微色相，避免把纯黑白或低饱和配色作为所有专业场景的默认答案。
- **视觉复杂度。** 视觉丰富度应服务内容。不要添加无信息价值的装饰，也不要把"克制"理解为单调、大量留白、缺少图片图表或所有页面使用同一种构图。

关键：如果已给出其他美学指令（如参考图、品牌体系、设计规范或媒介专属 skill），或项目中已有文件，则完全忽略默认美学。

## 图像素材与外部信息
图片素材能显著提升产物的美观度与丰富度——不要默认只用纯 CSS/SVG 撑起全部视觉。为氛围、质感和视觉节奏而配图是正当用途，不需要等到"内容必须有图"才配图。选择工具的判断规则很简单：**需要真实图片就搜索，需要丰富美观的图片就生成**。当前 harness 若提供以下能力（映射见 `references/<harness>.md`；没有对应工具就跳过，用内联 SVG / CSS 图形兜底），在合适的位置主动使用：

- **`generate_image`（AI 图片生成）**——美化、氛围类配图一律走生成：hero 图、插画、照片质感背景、章节题图、空状态插图、信息图（infographic）、产品/场景示意图等任何能让页面更好看的位置，用文生图直接生成；有品牌参考图或用户素材时用图生图对齐既有视觉语言；多屏 / 多页需要风格统一、角色连贯的插画体系时用组图一次生成整个序列；对已有图片做局部调整用图片编辑。生成 prompt 里写清风格、构图、配色与光线，让产出与已确立的视觉方向一致，而不是各自为政。
- **`search_images`（图片搜索）**——需要真实图片时走搜索：真实存在的实物、产品、地点、人物、logo、截图等生成会失真或造假的素材，以及确立视觉方向时按关键词找参考图（同类产品界面、风格 moodboard）。直接引用搜索结果时注意来源与版权。
- **`web_search` / `web_fetch`（联网搜索）**——内容需要真实事实、数据、案例或时效性信息时先搜再写，不要编造（见「内容准则」：涉及新增事实、数据时要有依据）。调研型产出（行业研究、政策梳理、竞争格局类 deck / 报告）要先做多轮搜索，把事实、数字与来源收集齐并标注出处，再进入设计。
- **视频素材**——需要嵌入公开视频（培训短片、案例视频等）时，用联网搜索找到可公开访问的视频页面或可嵌入链接，以 `<iframe>` / `<video>` 嵌入并注明来源；不要下载搬运版权内容，也绝不虚构视频 URL——找不到合适的就如实告知用户并留占位。

约束：

- 配图要属于同一视觉体系——风格、色调、光线与已确立的视觉方向一致，宁可少而统一，不要多而杂乱；逐张风格漂移比没有图更伤美观度。
- 用户已提供图片 / 品牌素材时优先使用，不要擅自用生成图替换。
- 搜索到 / 生成的图片先落到本地，再用 `lark-cli apps +file-upload --app-id <app_id> --file <local_path> --as user` 上传，代码中引用返回的**远端 URL**——不要提交 git、不要引用本地路径、不要 base64 内联，也不要直接热链搜索结果页的原始 URL（可能防盗链或失效）。上传需要 `app_id`，任务尚未初始化时先按「发布」前提完成 `+create` / `+init` 两步。

## 输出创建准则
- **文件输出路径**：会话根目录下会并存多个任务。**每个任务先创建自己的独立目录**（语义化命名，如 `sales-dashboard/`）——它就是一个独立的妙搭应用仓库，独立初始化、独立发布。所有交付物写进本任务目录，主 HTML 入口是该目录下的 `index.html`。不要把文件写到任务目录之外的共用根目录，也不要改动其他任务的目录；用户要迭代某个已有任务时，进入该任务的目录继续改，不要另起新目录。
- 对文件做重大修订时，先复制再编辑，以保留旧版本（如 index.html、index v2.html 等）。
- 始终避免写大文件（>1000 行）。而应把代码拆成若干更小的 JSX 文件，最后在主文件里 import 进来。这让文件更易管理和编辑。
- 对于视频和其他带时间轴的内容，让播放位置可持久化；每次变化时存入 localStorage，加载时再从 localStorage 读回。这样用户刷新页面时不会丢失当前位置，而刷新在迭代设计中很常见。（使用 `starter-components/deck-stage.js` 的 deck 不需要这么做——宿主会把幻灯片位置保存在 URL 中。）
- 在既有 UI 上做增补时，先理解该 UI 的视觉语汇并遵循它。对齐文案风格、配色、语气、hover/click 状态、动画风格、阴影＋卡片＋布局模式、密度等。把你观察到的东西"出声想一想"会有帮助。
- 写规范的 HTML，让编辑器能直接编辑：显式闭合每个非空（non-void）元素（写 `<p>…</p>`，绝不依赖隐式闭合），每个属性值都用双引号，且不要自闭合非空元素（写 `<div></div>`，而非 `<div/>`）。这有助于直接编辑功能正常工作。
- 绝不使用 `scrollIntoView`——它可能搞乱 web app。如有需要，改用其他 DOM 滚动方法。
- **颜色使用：** 有品牌色时优先沿用品牌体系；没有品牌或既有配色时，根据主题、受众、内容语义和视觉方向推导协调色板。避免随意加入彼此无关的颜色，不要默认退回纯黑白。对于数据图表和信息图，颜色应承担区分、强调或表达语义的作用，并保证足够对比。
- **Emoji：** 不要在生成的代码中使用 emoji 字符——不作图标、不作装饰、不放进数据里。例外：仅当用户的品牌资产明确包含 emoji 时。
- **图标：** 系统图标规则仅适用于需要界面图标体系的 UI 或交互原型。在这类产物中，使用手写内联 SVG（`<svg viewBox="0 0 24 24">`）建立语义贴切、风格连贯的图标语言。
- **字体加载：** 需要 Google Fonts / web 字体时，一律从自托管镜像 `https://miaoda.feishu.cn/fonts/css2` 加载，不要直连 `fonts.googleapis.com` / `fonts.gstatic.com`——这两个 Google CDN 在部分地区慢、甚至连不上，会导致字体加载失败、页面回退到系统字体。镜像是 Google Fonts `css2` 端点的直接替代：查询语法完全一致（`?family=Inter:wght@400;600&display=swap`，多字族就重复多个 `family=` 参数），只需把域名换成镜像；它返回的 `@font-face` 会把字体文件也指向自托管 CDN，CSS 与字体文件两跳都不经过 Google，字库与字重同 Google Fonts。照常用 `<link rel="stylesheet" href="https://miaoda.feishu.cn/fonts/css2?family=…&display=swap">` 引入即可。

## 内容准则

**内容取舍。** 不添加与用户目标无关或没有依据的内容。在用户明确的范围内，可以重组、解释和补足完成叙事所需的信息；涉及新增事实、数据或任务范围时，再向用户确认或明确为示例。内容不足以独立成页时，应合并、重构或请求材料，不用放大元素和增加留白勉强撑页。

**数据保真。** 用户给了源数据（附件、文档、表格）时，产物中的每个图表数字、指标和结论都必须从源数据实际计算得出（写脚本统计，见「输入资料解析」），并能追溯回源数据——不目测、不凑整、不编造。做数据报表/看板前读 `references/data-report.md`，其中的数据准则同样适用。

**硬性规格是约束，不是建议。** 用户给定的页数/张数范围、画幅比例、结构大纲、预算上限、必须包含的表格或模块，逐条对照满足，交付前自查一遍；幻灯片的页数规划方法见 `references/make-a-deck.md`。

**使用恰当的尺度：** 对于 1920x1080 的幻灯片，文字绝不应小于 24px；理想情况下要大得多。打印文档最小 12pt。移动端 mockup 的点击目标绝不应小于 44px。

**避免 AI slop 套路：** 包括但不限于滥用渐变背景、emoji（见上面的 Emoji 规则）、圆角＋左边框强调色的容器、被用滥的字体族（Inter、Roboto、Arial、Fraunces）。

**CSS**：`text-wrap: pretty`、CSS grid 以及其他高级 CSS 效果都是你的好帮手！

**强烈倾向用带 `gap` 的 flex/grid，而非 inline 流。** 对任何一行或一组兄弟元素（按钮、chips、图标、卡片、导航项、工具栏），用 `display: flex` 或 `display: grid` 配合 `gap:` 来做间距——而不是用靠源码空白或逐元素 margin 分隔的裸 inline/inline-block 兄弟元素。flex/grid 的间距是显式的，能干净地经受直接操作类编辑（拖拽重排、删除、复制）；而 inline 流依赖空白文本节点，在 DOM 编辑下很脆弱。把 inline 流留给句子中偶尔夹带 `<a>`/`<strong>`/`<em>` 的文字段落——不要用它来排布 UI 元素。

## 保留评论锚点
某些源元素带有 `data-comment-anchor="…"` 属性。它把用户的评审评论钉在该元素上。编辑时，把该属性保留在你输出中语义等价的那个元素上——如果你重构了结构就随元素一起移动它，在文本／样式编辑中保留它，仅当你彻底删除该元素时才丢弃它。绝不发明新值，也不要把它复制到其他元素上。

## 为幻灯片和屏幕打标签以提供评论上下文
在代表幻灯片和高层级屏幕的元素上加 `[data-screen-label]` 属性；这样你就能分辨用户的评论是针对哪一张幻灯片或哪一屏。
当用户说"slide 5"或"index 5"时，他们指的是第 5 张幻灯片（标签"05"），而绝非数组下标 `[4]`——人类不按 0 起始计数。

## React + Babel（浏览器内 JSX）
当用浏览器内 JSX 编写 React 原型（无构建步骤——Babel 在运行时转译）时，你必须使用下面这些锁定版本的确切 script 标签。不要使用未锁定版本（例如 react@18）。要用 React + Babel 时，可直接从本 skill 的 `assets/index.html` 拷贝 HTML 模板起步（`cp <本 skill 所在目录>/assets/index.html <任务目录>/index.html`）——它已带好这三个 script 标签和 `#root` 挂载点，不必手写。

```html
<script src="https://sf3-scmcdn-cn.feishucdn.com/obj/feishu-static/miaoda/coding-unpkg-sdk/react@18.3.1/umd/react.development.js" crossorigin="anonymous"></script>
<script src="https://sf3-scmcdn-cn.feishucdn.com/obj/feishu-static/miaoda/coding-unpkg-sdk/react-dom@18.3.1/umd/react-dom.development.js" crossorigin="anonymous"></script>
<script src="https://sf3-scmcdn-cn.feishucdn.com/obj/feishu-static/miaoda/coding-unpkg-sdk/@babel/standalone@7.29.0/babel.min.js" crossorigin="anonymous"></script>
```

发布前需要对以上 script 路径进行自检，确保它们路径与上述代码完全一致

### 脚本导入
用 script 标签导入你写的任何辅助脚本或组件脚本。`.jsx` 文件必须用 `<script type="text/babel" src="xxx.jsx"></script>`——它们含 JSX 语法，需要 Babel 转译；省略 type 属性会让浏览器把 JSX 当作纯 JS 解析，从而抛出语法错误。纯 `.js` 文件可以用普通的 `<script src="xxx.js"></script>`。避免在脚本导入上使用 `type="module"`——它可能会出问题。

**加载顺序**：`@babel/standalone` 用异步 XHR 拉取外部 `<script type="text/babel" src="...">` 文件，但保证按 DOM 顺序执行——靠前的脚本总在靠后的脚本之前运行。然而，内联脚本（无 `src`）会立即就绪，而外部脚本必须等待网络响应。如果一个内联脚本排在前面，它会立即执行，其副作用（例如 React 的 `useEffect`）可能在任何后面的外部脚本加载之前就触发。把外部脚本放在依赖它们的内联脚本之前。

### 跨文件作用域
每个 `<script type="text/babel">` 在转译后都有自己独立的作用域。要在文件间共享组件，在组件文件末尾把它们导出到 `window`：

```js
// 在 components.jsx 末尾：
Object.assign(window, {
  Terminal, Line, Spacer,
  Gray, Blue, Green, Bold,
  // ... 所有需要共享的组件
});
```

### 样式对象命名
定义全局作用域的样式对象时，给它们起具体的名字。如果你导入了 1 个以上带 `styles` 对象的组件，就会出问题。你必须基于组件名给每个 styles 对象起唯一的名字，比如 `const terminalStyles = { ... }`；或者用内联样式。绝不要写 `const styles = { ... }`。

### 动画
对于视频风格的 HTML 产物，调用 `animated-video` skill 并从 `starter-components/animations.jsx` starter component 起步——不要自己实现时间轴引擎。对于简单的交互原型过渡，CSS transitions 或纯 React state 就够了。

### 原型
- 克制住加"标题"屏的冲动；让你的原型在视口中居中，或做成响应式尺寸（填满视口并留合理边距）。

## Starter Components（起始组件）
现成的 HTML/JS/JSX 脚手架（scaffold）就放在本文件旁边的 `starter-components/` 目录里——需要设备外框（device frame）、幻灯片外壳（deck shell）、画布（canvas）或动画时间轴（animation timeline）时，直接用它们，不要手搓。使用方式：把文件拷进当前任务目录（在任务目录下执行 `cp <本 skill 所在目录>/starter-components/<file> .`——注意 cwd 不会是 skill 目录，要用 skill 目录的实际路径），或读过之后照着改；每个文件顶部都带有自己的用法说明。

- `design-canvas.jsx` — 可平移／缩放的画布，artboard 可重排、可全屏聚焦。
- `deck-stage.js` — 幻灯片 deck 外壳。用于任何幻灯片演示（见「Skills 元信息」中的 Make a deck）。
- `ios-frame.jsx` / `android-frame.jsx` — 带状态栏和键盘的设备边框。
- `tweaks-panel.jsx` — 浮动的 Tweaks 面板＋表单控件（`useTweaks`、滑块、开关、单选、颜色 chips 等）。
- `macos-window.jsx` / `browser-window.jsx` — 桌面窗口外壳（chrome）。
- `animations.jsx` — 基于时间轴的动画引擎（Stage + Sprite + scrubber + Easing）。

## Tweaks
用户可以从工具栏开关 **Tweaks**——一个存在于原型内部的页内控件面板（颜色、字体、间距、文案、布局变体）。不要自己实现它：用 `kind: "tweaks-panel.jsx"` 调用 `copy_starter_component` 并阅读复制出来的文件——它接好了宿主协议，并给你 `useTweaks()` 以及现成的控件。这个面板的标题按界面语言来定——英文叫"Tweaks"，中文叫"风格"。把它保持小巧，Tweaks 关闭时完全隐藏，并且即使用户没要求，也默认加上几个有品味的 tweak。你写在面板里的标签和选项是用户会读到的内容，而非配置——用与 app 其余部分相同的语言书写。

**闭环。** 每个 tweak 都需要一个生产者（面板控件）和一个消费者（对该值作出反应的内容）。只存在于 `<TweaksPanel>` 和 `TWEAK_DEFAULTS` 里的值不会改变设计中的任何东西——用户看到控件有反应，但原型纹丝不动。

## 发布
设计产物写完并提交后，需要发布到妙搭（lark-apps）才能拿到可访问链接。本 skill 产出的是创意模式（html）应用，发布走本地开发链路：改动 git commit 后推到工作分支 `sprint/default`，再用 `lark-cli apps` 命令发起部署并轮询结果。

**前提**：每个任务目录是一个独立的妙搭 html 应用仓库，独立发布、互不影响；发布序列的所有命令都在**当前任务目录**内执行。任务目录还不是应用仓库（没有 `.spark/meta.json`）时，先完成两步初始化：

```bash
# 1. 创建应用，记下返回的 app_id（app_ 开头）
lark-cli apps +create --name "<应用名>" --app-type html --as user

# 2. 初始化到任务目录：会自动 clone 远端仓库并 checkout 工作分支 sprint/default，
#    无需 git init / git checkout（--dir 不传默认 ./<app-id>；
#    --source-path 可把已写好的产物一并并入，但源码目录不存在时会被静默跳过，用后核对文件确实进了仓库）
lark-cli apps +init --app-id <app_id> --dir <任务目录> --as user
```

初始化后在任务目录内创建 / 修改产物（创意模式是 buildless，源码即产物，`index.html` 放仓库根目录），然后走下方发布序列。

`app_id`（`app_` 开头）从任务目录的 `.spark/meta.json` 读取，或来自 `+create` 的返回 / 用户给出——`cli_` 开头的是飞书应用 ID，绝不能传给 `apps +*` 命令。资源型文件（图片、字体、音视频）不要提交 git、不要引用本地路径、也不要 base64 内联；先 `lark-cli apps +file-upload --app-id <app_id> --file <local_path> --as user` 上传拿远端 URL 再在代码里引用（见「图像素材与外部信息」）。

发布序列：

```bash
# 1. 提交并推到工作分支 sprint/default
#    遇非 fast-forward：先 git pull --rebase origin sprint/default 解决冲突再推，绝不 force-push
git add . && git commit -m "feat: ..." && git push origin sprint/default

# 2. 发起部署（记下返回的 release_id），然后轮询状态直到 finished / failed：
#    publishing → 继续轮询；finished → 输出含可分享的 online_url，直接返回给用户；failed → 按输出中的 error_logs 报告失败原因
lark-cli apps +release-create --app-id <app_id> --as user
lark-cli apps +release-get --app-id <app_id> --release-id <release_id> --as user
```

要点：

- 所有 git 命令必须在**任务仓库根目录**下执行（每条命令先 `cd <任务目录>`，或用 `git -C <任务目录>`）——`git add .` 作用于当前 cwd，在多任务共用的上级根目录里执行会把其他任务的文件也 stage 进来。
- 推送和部署的分支必须是 `sprint/default`：推到其他分支，`+release-create` 会失败。
- `+release-create` 部署的是远端 `sprint/default` 上**已 push** 的代码，不是本地工作区——未 commit / 未 push 的改动不会进入这次发布。
- 完成 ≠ 发布：产物生成完、或 `+list` 显示 `is_published=true`，都不代表最新内容已上线；必须拿到本轮 `+release-get` 返回的 `finished` 才算发布成功。
- 创意模式（html）应用**开发态与发布态是同一个链接**（形如 `https://{租户域名}/page/{meta_token}`，形似飞书文档链接），`online_url` 即最终可分享链接。
- 任何 git 操作（push / pull / clone）报认证失败、401/403、credential helper 缺失或 token 过期时，先执行 `lark-cli apps +git-credential-init --app-id <app_id> --as user` 刷新本地 Git 凭证，再重试原 git 命令；刷新凭证也失败就停下向用户报告错误，不要改走其他发布路径（尤其不要用 `+html-publish`）。

## Skills 元信息
你有以下内置技能 prompt，位于本文件相对路径下的 `references/` 目录中。如果用户的需求与其中某个技能匹配，而对应的 prompt 尚未加载进你的上下文，就去 READ（读取）相应文件，把它的指引加载进来。

- **[Animated video](references/animated-video.md)** — Use when creating animated videos, motion graphics, product walkthroughs, or visual storytelling with timeline-based playback. 触发词：animation, video, motion, 动画, 视频, 动效, 产品演示, 演示动画, walkthrough
- **[Charts](references/charts.md)** — 基于 ECharts 的数据可视化，用于浏览器直出 HTML。当需要创建图表、仪表盘或数据可视化时使用。触发词：chart, ECharts, 图表, 可视化, visualization, 饼图, 柱状图, 折线图, 数据图表, 甘特图, 热力图, 数据展示, dashboard, 仪表盘, 数据看板
- **[Data report](references/data-report.md)** — 数据驱动的报表与看板设计。从数据分析到报表规划、信息层级组织，适用于用户有数据文件或明确指标，需要产出结构化数据报表的场景。图表绘制部分由 charts skill 承担。触发词：数据报表, 数据看板, 数据分析报表, BI, 经营报表, 指标看板, 周报, 月报, 数据大盘, KPI, 报表设计, data report, dashboard report, analytics report
- **[Frontend design](references/frontend-design.md)** — Guidance for distinctive, intentional visual design when building new UI or reshaping an existing one. Helps with aesthetic direction, typography, and making choices that don't read as templated defaults.
- **[Hi-fi design](references/hi-fi-design.md)** — 用于创建高保真 UI mockup、设计探索，或带多种变体的视觉原型。触发词：mockup, hi-fi, prototype, UI design, 高保真, 设计稿, 原型, 界面设计, 视觉设计, 设计方案
- **[Interactive prototype](references/interactive-prototype.md)** — 可交互原型：像真实应用一样直接运行的高保真交互 demo。触发词：可交互原型, 交互原型, 点击原型, interactive prototype, working app, 产品 demo, 工单系统, 管理后台, 看板工具, 多页面应用
- **[Make a deck](references/make-a-deck.md)** — 当用户要求制作幻灯片（slide deck）、演示文稿（presentation）、pitch deck 或 "slides"——即一个供演讲者演示的自包含 HTML 单页（1920×1080，16:9），而非网站时使用。
- **[Visual exposure](references/visual-exposure.md)** — 用于制作可视化报告、专题视觉页、信息图、视觉长图、概念可视化、产品能力曝光、方案亮点展示等内容型 HTML 视觉作品。适合用户想把材料、数据或观点组织成可阅读、可展示、可传播的视觉化表达，但不希望做成 PPT、传统 dashboard 或纯 ECharts 图表的场景。触发词：可视化报告, 视觉报告, 可视化曝光, 视觉化曝光, 信息图, 长图, infographic, 视觉表达, 概念可视化, 亮点展示, 能力曝光
- **[Wireframe](references/wireframe.md)** — Explore many ideas with wireframes and storyboards
