---
name: lark-slides
version: 1.0.0
description: "飞书幻灯片：创建和编辑幻灯片。创建演示文稿、读取幻灯片内容、管理幻灯片页面（创建、删除、读取、局部替换）。当用户需要创建或编辑幻灯片、读取或修改单个页面时使用。当用户给出 doubao.com 的 /slides/ URL/token 时，也应直接使用本 skill，不要因为域名不是飞书而回退到 WebFetch；路由依据是 URL 路径模式和 token，而不是域名。不负责：云文档内容编辑（走 lark-doc）、云文档里的独立画板对象（走 lark-whiteboard）、上传或下载普通文件（走 lark-drive）。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli slides --help"
---

# slides (v1)

> 本技能文档较长，务必使用 Read 工具阅读两次，必须阅读完整全文。

## 权威经验

**权威经验是全局硬约束和高频易错点，必须牢记并严格遵守。**

- 你有充足的时间完成这个 PPT，质量永远比速度重要。
- PPT 的尺寸是 960x540，必须严格确保主体内容在页面边界内。
- !!!禁止交付无图产物!!! 必须使用大量图片增强视觉效果!!! 禁止重复使用同一张图!!!
- 封面页的主视觉必须是 `<img>`（来自生图工具或搜图工具），不要使用 `<shape>` 或 `<icon>` 拼出封面视觉。
- 禁止用 `<shape>` 和 `<line>` 拟形具体物项，必须使用生图工具生成的 `<img>`。
- 禁止在 `headline` 或 `title` 下方放置用于分隔或装饰的 `rect` 或 `<line>`。
- 禁止在任何页面内部使用无意义的装饰线条或色块条带，页面任何一边都不要使用贴边窄条。
- 生图工具的指令参数必须以“不要出现任何文字和颜色色号”结尾，避免生成的图片上出现干扰文字。
- 禁止使用 emoji 图标，任何位置都不能出现。
- 字号必须显式设置 `<content>` 的 `fontSize` 属性，不要依赖 `textType` 的默认字号兜底，这些兜底值明显偏大。
- 大数字、字号大或字数多的 `<content>` 必须设置 `wrap="true" autoFit="normal-auto-fit"` 属性自动换行和缩排，避免文字溢出。
- 文字颜色必须用 `<content>` 的 `color` 属性而不是 `fontColor` 属性。
- 文字行间距必须设置 `<content>` 的 `lineSpacing="multiple:xx"` 或 `lineSpacing="fixed:xx"` 而不是 `lineSpacing="xx"`。
- 图片必须用 `<img>` 而不是 `<image>`。
- IconPark 图标必须填充颜色（设置 `<fill><fillColor color="rgba(R,G,B,A)"/></fill>`）并和背景有足够对比。
- 绘制图表时原生图表（柱状、条形、折线、面积、饼（环）、雷达、组合图）用 `<chart>`，其他（漏斗图、金字塔图、象限图、矩阵图等）用 `<shape>` + `<line>` 模拟。
- 隐藏 `<chart>` 的图例只能通过不写或删除 `<chartLegend>` 实现，`<chartLegend>` 不支持 `position="none"`。
- 表格优先用 `rect` 和 `text` 模拟，其他用 `<table>`，没有 `<shape type="table">`。
- 必须设置 `<table>` 的 `width` 和 `height` 固定表格大小，同时设置需要保留列宽或行高的 `<col>` 的 `width` 和 `<tr>` 的 `height`，其余自动分配。
- `<td>` 直接子元素只有 `<fill>`（背景）、`<content>`（文字）和边框配置（一般不用），不能嵌套 `<shape>`、`<img>`、`<icon>`。
- `<shape type="rect">` 只是形状不是容器，`<icon>`、`<img>`、`<shape type="text">` 和其他 `<shape>` 必须与它平级靠坐标叠放。
- 填充渐变颜色必须用 `<fill><fillColor color="linear-gradient(135deg, rgba(R,G,B,A) 0%, rgba(R,G,B,A) 100%)"/></fill>`。
- 编辑页面前必须阅读 [`workflow/slides_editing.md`](references/workflow/slides_editing.md)。
- 绘制图表前必须阅读 [`xml/slides_chart_demo.xml`](references/xml/slides_chart_demo.xml)。
- 当用户要求无损复述历史上下文时，必须无损复述以上权威经验、必读的技能文档（需要重新阅读）和易错元素语法（尤其是 `<table>` 和 `<chart>`）。

## 豆包设计原则

适用范围：

- 普通内容页的设计必须以豆包设计原则为最高准则，除非用户要求使用模板或直接提供设计方案。
- 不适用于 `title-cover`、`section-divider`、`conclusion`、`quote-highlight` 和 `big-number`。

核心要求：

- 必须采用信息密度极高的图文卡片布局，追求充实饱满、图文丰富、可逐行细读的版面，宁可密而满，不要空而疏。
- **!!!信息密度极高!!! 图多!!! 卡多!!! 字多!!!**

排版布局：

- 卡片布局：卡片按多行网格铺满页面，版面对称、均衡、不留白。网格数、图文比例按内容变化，避免每页雷同。使用更多卡片做细分承载，避免在单张卡片里堆砌大量文字（例如 8 张 50 字卡片优于 2 张 200 字卡片），多个要点必须拆分为多张子卡片。
- 卡片样式：方角卡片 + 半透明填充 + 无边框 + 卡片贴边窄条（可选）；所有卡片必须使用相同的配色方案（少量需强调的卡片除外），禁止同页出现彩虹卡片（卡片颜色超过 3 种）。
- 卡片结构：视觉锚点（关键词、编号或 IconPark 图标）+ 标题 + 内容（包括文字、图片、图表、子卡片）。
- 文字卡片：多数页面必须满足 6-8 张文字卡片、200-400 文字数量，字数不足时必须扩写成长句或段落，文字卡片不要留白，必须充实饱满。文字卡片不是短标签，而是“标题 + 完整说明”，像浓缩的分析文稿。文字内容不得不用列表、分栏、关键词或短句时，必须保证层次清晰，更建议拆分为多张子卡片。
- 图片卡片：多数页面必须满足 1-3 张图片卡片，缺少图片时必须用生图工具补充配图，图片卡片与文字卡片组成网格，确保图文丰富。
- 图表卡片：数据信息不要在文字卡片中罗列，必须在图表卡片中可视化（包括表格、图表、时间线、流程图等），图表卡片与其他卡片组成网格，展现数据驱动。
- 间距要求：所有边距都要左右对称，页面和内部内容的边距至少 40px（内容不要贴边），卡片和内部文字的边距至少 5px（文字不要贴边），卡片之间保持 20-40px 的间距。
- 文字对齐：正文默认左对齐，只在封面、结尾或大号数字场景中使用居中；表格里的文字左对齐、数字右对齐、仅关键词或短句时居中对齐。

视觉风格：

- 美学：干净、明亮、清爽但信息饱满；靠卡片和对齐网格在高密度下维持秩序感；同排卡片文字数量应相近以保持观感整齐。
- 字体：全篇以无衬线体（思源黑体）为主，封面或关键强调可少量使用衬线体。
- 字号：标题 28-36pt、正文 12-14pt、注释 10-12pt，常规关键指标 16-32pt、核心指标用 36-52pt 数字，下面配 10-14pt 标签与简短解读，需要容纳更多文字时允许使用更小的字号。
- 图标：内嵌 IconPark 图标（可用关键词或编号替代）作为视觉锚点，让高密度文字也有图形节奏，而不是成片纯文字块。
- 配色：克制颜色数量，确保所有页面都只使用同样的 1 个背景色（偏好浅米白）、1 个主色、1 个强调色和 1 个辅助色；偏好莫兰迪配色，禁止彩虹配色（比如蓝配橙）。

## Quick Reference

**本表只定位「场景 → 用哪条命令、读哪份文档」。参数以「执行前必做」里对应的文档和 `lark-cli slides +<verb> --help` 为准，不要凭记忆或按别的命令类比补参数。**

| 用户需求 | 优先动作 | 关键文档 / 命令 |
|----------|----------|-----------------|
| 新建 PPT | 先规划 `slide_plan.json`，再按页数选择一步或两步创建 | `planning-layer.md`、`visual-planning.md`、`asset-planning.md`、`cli/lark-slides-create.md`、`slides +create`、`slides +add-slide`、`xml/lark-slides-add-slide.md`（两步创建逐页添加） |
| 用户要求使用模板，或提供 PPTX 文件要求修改、美化 | 将模板导入为 Slides 再编辑 | `workflow/template-editing.md` |
| 编辑单个标题、文本块、图片或局部元素 | 块级替换/插入，**只动点名的 block，同页其他元素不受影响**；不改页序 | `slides +replace-slide`、`cli/lark-slides-replace-slide.md` |
| 一页改动很多（批量字体/配色）、要改页面背景、要删掉若干元素 | 整页覆盖，`slide_id` 和页序不变；带原 `id` 写回的元素保留 id，不带 `id` 的会作为新元素插入并拿到新 id；**代价是没写进 `--content` 的元素会被删除，所以改个别元素不要用它** | `slides +update-slide`、`lark-slides-update-slide.md` |
| 给已有 PPT 追加或插入页面 | 一次一页，`--slide` 支持 `@file` 绕开 shell 转义 | `slides +add-slide`、`xml/lark-slides-add-slide.md` |
| 删除页面 | 按 `slide_id` 单页删除，删前先回读确认 | `slides +delete-slide`、`xml/lark-slides-delete-slide.md` |
| 读取或分析已有 PPT | 解析 slides/wiki token，用 shortcut 回读全文 XML 或读取单页 XML，保存 `xml_presentation_id`、`slide_id`、`revision_id` | `slides +xml-get`、`xml_presentation.slide.get`、`cli/lark-slides-xml-presentations-get.md` |
| 查看或回滚历史版本 | 先用 `+history-list` 找 `history_version_id`，再 `+history-revert`，必要时 `+history-revert-status` 轮询 | [`cli/lark-slides-history.md`](references/cli/lark-slides-history.md) |
| 获取幻灯片页面截图 | 按页码用 `--slide-number`，按 ID 用 `--slide-id`；单张用 `--output`，批量或全量用 `--output-dir`，每批最多 10 页串行执行；截图目录复用同一任务的 deck/task 标识，后续读取返回的实际路径 | `slides +screenshot`、`cli/lark-slides-screenshot.md` |
| 上传或使用图片 | 先上传为 `file_token`，禁止直接写 http(s) 外链 | `slides +media-upload`、`cli/lark-slides-media-upload.md`，或 `+create --slides` 的 XML 里写 `<img src="@./path">` 占位符 |
| 绘制图表 | 原生图表（柱状、条形、折线、面积、饼（环）、雷达、组合图）用 `<chart>`，其他（漏斗图、金字塔图、象限图、矩阵图等）用 `<shape>` + `<line>` 模拟 | `xml/xml-schema-quick-ref.md`、`xml/slides_chart_demo.xml` |
| 绘制表格 | 优先用 `rect` 和 `text` 模拟，其他用 `<table>` | `xml/xml-schema-quick-ref.md` |
| 使用图标 | 禁止盲猜 iconType，必须先检索 IconPark，再写 `<icon iconType="...">`，图标必须填充颜色并和背景有足够对比，禁止使用 emoji 图标 | `iconpark_tool.py search → resolve`、`xml/iconpark.md` |
| 创建失败、空白页、3350001、布局异常 | 先回读状态，再按排障清单修复，不假设原操作原子成功 | `workflow/error-handling.md`、`workflow/validation-xml.md` |

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，认证、权限和全局参数均以 lark-shared 为准。**

**CRITICAL — 查看或回滚历史版本前，MUST 先读取 [`cli/lark-slides-history.md`](references/cli/lark-slides-history.md)。回滚接口只接受 `history_version_id`，不要把 `revision_id` 直接传给 `+history-revert`。**

**CRITICAL — 生成任何 XML 之前，MUST 先用 Read 工具读取 [xml/xml-schema-quick-ref.md](references/xml/xml-schema-quick-ref.md)，禁止凭记忆猜测 XML 结构。**

**CRITICAL — 新建演示文稿或大幅改写页面时，MUST 先生成 `.lark-slides/plan/<deck-or-task-id>/slide_plan.json`，再生成 XML。先创建对应目录，规划层规则和中间产物生命周期见 [planning-layer.md](references/planning-layer.md)。仅替换一个标题、插入一个块等小型已有页编辑可豁免。**

**CRITICAL — 新建演示文稿或大幅改写页面时，生成 XML 前 MUST 读取 [visual-planning.md](references/visual-planning.md)，确保 `layout_type`、`visual_focus`、`text_density` 实际改变页面几何、主视觉和文本量。**

**CRITICAL — 新建演示文稿或大幅改写页面时，规划 `asset_need` MUST 遵循 [asset-planning.md](references/asset-planning.md)：只做元数据规划，必须有 `fallback_if_missing`，不得要求真实搜索、下载或上传素材。**

**CRITICAL — 将完整 `<slide>` XML 提交给 `slides +create`、`slides +add-slide` 或 `slides +update-slide` 之前，MUST 先把待提交 XML 保存到本地文件并运行唯一版式准出入口 [`scripts/xml_lint.py`](scripts/xml_lint.py)；`summary.error_count` 必须为 0 才能调用接口。**

**CRITICAL — 创建、大幅改写或整页写回后，MUST 按 [workflow/validation-xml.md](references/workflow/validation-xml.md) 做显式验证：回读全文 XML、核对页数和关键元素，并使用 [`scripts/xml_lint.py`](scripts/xml_lint.py) 统一检查 XML、越界、重叠、空白页和内容稀疏风险。**

**CRITICAL — 创建前自检或失败排障时，MUST 按 [workflow/error-handling.md](references/workflow/error-handling.md) 检查 XML 转义、结构、shell 截断、图片 token、3350001 和布局风险。**

**编辑已有幻灯片页面**：单个标题、文本块、图片或局部元素优先用 [`+replace-slide`](references/cli/lark-slides-replace-slide.md)（块级替换/插入，不动页序）；一页里改动很多（例如批量换字体）、要改背景、或要删掉若干元素时用 [`+update-slide`](references/lark-slides-update-slide.md) 整页覆盖（`slide_id` 和页序不变，但没写进 `--content` 的元素会被删除）；**多页大改就对每一页各跑一次 `+update-slide`**。选择 action 和完整读-改-写流程见 [`workflow/slides_editing.md`](references/workflow/slides_editing.md)。

**用户要求使用模板**：按 [workflow/template-editing.md](references/workflow/template-editing.md) 处理。

## 身份选择

飞书幻灯片通常是用户自己的内容资源。**默认应优先显式使用 `--as user`（用户身份）执行 slides 相关操作**，始终显式指定身份。

- **`--as user`（推荐）**：以当前登录用户身份创建、读取、管理演示文稿。执行前先完成用户授权：

```bash
lark-cli auth login --domain slides
```

- **`--as bot`**：仅在用户明确要求以应用身份操作，或需要让 bot 持有/创建资源时使用。使用 bot 身份时，要额外确认 bot 是否真的有目标演示文稿的访问权限。

**执行规则**：

1. 创建、读取、增删 slide、按用户给出的链接继续编辑已有 PPT，默认都先用 `--as user`。
2. 如果出现权限不足，先检查当前是否误用了 bot 身份；不要默认回退到 bot。
3. 只有在用户明确要求"用应用身份 / bot 身份操作"，或当前工作流就是 bot 创建资源后再做协作授权时，才切换到 `--as bot`。

## 执行前必做

> **重要**：`references/xml/slides_xml_schema_definition.xml` 是此 skill 唯一正确的 XML 协议来源；其他 md 仅是对它和 CLI schema 的摘要。

高频只读：

- [xml/xml-schema-quick-ref.md](references/xml/xml-schema-quick-ref.md)
- [planning-layer.md](references/planning-layer.md)（新建 / 大幅改写）
- [visual-planning.md](references/visual-planning.md)（新建 / 大幅改写）
- [asset-planning.md](references/asset-planning.md)（新建 / 大幅改写）
- [workflow/validation-xml.md](references/workflow/validation-xml.md)（创建 / 大幅改写后）

调用相关命令前必须读取相关的文档以了解命令的使用方式：

- 创建：[`cli/lark-slides-create.md`](references/cli/lark-slides-create.md)、[`xml/lark-slides-add-slide.md`](references/xml/lark-slides-add-slide.md)（逐页添加 / 给已有 PPT 追加页面）
- 删除页面：[`xml/lark-slides-delete-slide.md`](references/xml/lark-slides-delete-slide.md)
- 阅读：[`cli/lark-slides-xml-presentations-get.md`](references/cli/lark-slides-xml-presentations-get.md)
- 编辑：[`workflow/slides_editing.md`](references/workflow/slides_editing.md)、[`cli/lark-slides-replace-slide.md`](references/cli/lark-slides-replace-slide.md)、[`lark-slides-update-slide.md`](references/lark-slides-update-slide.md)
- 历史版本：[`cli/lark-slides-history.md`](references/cli/lark-slides-history.md)
- 截图：[`cli/lark-slides-screenshot.md`](references/cli/lark-slides-screenshot.md)
- 图片：[`cli/lark-slides-media-upload.md`](references/cli/lark-slides-media-upload.md)
- 图表：[`xml/slides_chart_demo.xml`](references/xml/slides_chart_demo.xml)
- 图标：[`xml/iconpark.md`](references/xml/iconpark.md)、[`scripts/iconpark_tool.py`](scripts/iconpark_tool.py)
- 排障：[`workflow/error-handling.md`](references/workflow/error-handling.md)
- 完整协议：[`xml/slides_xml_schema_definition.xml`](references/xml/slides_xml_schema_definition.xml)


## Workflow

### Design Ideas

不要生成无设计感的幻灯片。纯白背景 + 标题 + bullets 只能作为极简临时稿，不能作为正式交付。

开始写 XML 前，先在 `slide_plan.json` 里确定 deck 级视觉策略：

- **主题化配色**：配色必须服务本次主题、行业和受众，不要默认蓝色商务风。如果把同一套颜色换到另一个完全不同主题仍然成立，说明配色不够具体。
- **主次比例**：选择 1 个主色承担约 60-70% 视觉权重，1 个辅助色承担结构和分区，1 个强调色只用于关键数字、结论或行动点。不要让所有颜色权重相同。
- **背景一致性**：先确定全 deck 的背景策略，默认保持同一明暗基调和底色体系；无论深浅，都要保证内容和背景对比充足。
- **统一 motif**：选择一个可复用视觉母题贯穿全文，例如编号节点、卡片处理方式、半出血图片区域、标题、页脚。不要每页换一套装饰语言。

每页至少要有一个视觉元素：图片、图标、图表、表格、流程、对比结构或大号数字。文本框本身不算主视觉。

常见页面形态：

- **双栏结构**：左文右图或左图右文，视觉区域占 35-45% 宽度。
- **图标行**：图标在色块或圆形底中，右侧是短标题和一句解释。
- **网格**：适合能力、模块、风险、行动项，每格内容保持同等层级。
- **半出血视觉**：图片占据左/右半屏，文字覆盖或贴边排布。
- **大数字卡片**：核心指标用大数字，下面配标签与简短解读。
- **对比列**：before/after、方案 A/B、问题/解法用左右并列，标题和基线严格对齐。
- **时间线/流程图**：步骤用节点和箭头表达，流程方向必须一眼可见。

常见错误必须避免：

- 不要所有页面复用同一种标题 + 三 bullets 版式。
- 不要用低对比文字或低对比图标，例如浅灰字压在浅色背景上。
- 不要让装饰线穿过文字，或让页脚、来源、编号挤压主体内容。
- 不要把素材缺失表现为空白图片框；必须按 `fallback_if_missing` 生成替代图片。
- 不要在任何位置使用 emoji 图标。


### 生成流程

```text
Step 1: 需求分析 & 读取知识
  - 分析主题、受众、页数、风格；
  - 若用户要求使用模板，按 workflow/template-editing.md 处理
  - 读取 xml/xml-schema-quick-ref.md；新建 / 大幅改写时还要读取 planning-layer.md、visual-planning.md、asset-planning.md
  - 涉及图表读取 xml/slides_chart_demo.xml

Step 2: 生成大纲 → 写入 slide_plan.json
  - 生成结构化大纲
  - 新建 / 大幅改写必须先创建目录并写入 `slide_plan.json`
  - plan 字段、路径命名和 `asset_need` 结构按 planning-layer.md / asset-planning.md 执行

Step 3: 按 slide_plan.json 生成 XML → 创建
  - 逐页消费 plan：key_message 定主结论，layout_type 定几何，visual_focus 定主视觉，text_density 定文本量
  - 缺少真实素材时必须用 `fallback_if_missing` 生成替代图片，不要留空
  - 读 cli/lark-slides-create.md 定一步创建还是两步创建，并据此构造 `slides +create`；两步创建再读 xml/lark-slides-add-slide.md 用 `+add-slide` 逐页添加
  - 图片按 cli/lark-slides-media-upload.md 处理；复杂 XML、转义和 3350001 排查按 workflow/error-handling.md 执行

Step 4: 审查 & 交付
  - 创建完成后，必须用 `slides +xml-get --presentation <xml_presentation_id>` 读取全文 XML，并按 workflow/validation-xml.md 做显式验证记录，包括 XML 文本重叠检查
  - 失败或部分成功按 workflow/error-handling.md 处理；局部问题优先用 `+replace-slide` 修正
  - 没问题 → 交付：使用 NotifyHuman 工具交付 PPT 链接
```

> 渐变色必须使用 `rgba()` 格式并带百分比停靠点，如 `linear-gradient(135deg,rgba(15,23,42,1) 0%,rgba(56,97,140,1) 100%)`。使用 `rgb()` 或省略停靠点会导致服务端回退为白色。

### 大纲模板

生成大纲时使用以下格式：

```text
[PPT 标题] — [定位描述]，面向 [目标受众]

页面结构（N 页）：
1. 封面页：[标题文案]
2. [页面主题]：[要点1]、[要点2]、[要点3]
3. [页面主题]：[要点描述]
...
N. 结尾页：[结尾文案]

风格：[配色方案]，[排版风格]
```

## 核心概念

### URL 格式与 Token

| URL 格式 | 示例 | Token 类型 | 处理方式 |
|----------|------|-----------|----------|
| `/slides/` | `https://example.larkoffice.com/slides/xxxxxxxxxxxxx` | `xml_presentation_id` | URL 路径中的 token 直接作为 `xml_presentation_id` 使用 |
| `/wiki/` | `https://xxx.feishu.cn/wiki/wikcn_EXAMPLE_NODE_TOKEN_123456` | `wiki_token` | ⚠️ **不能直接使用**，需要先查询获取真实的 `obj_token` |

> 带 `--presentation` 的 slides shortcut 都会自动解析以上两种 URL；直接调用原生 API 时仍需手动解析 wiki 链接。

### Wiki 链接特殊处理（关键！）

知识库链接（`/wiki/TOKEN`）不能直接当 `xml_presentation_id`。直接调用原生 API 前，先用 Wiki shortcut 查询节点，确认 `data.obj_type == "slides"`，再用 `data.obj_token` 作为真实 presentation ID。

```bash
lark-cli wiki +node-get --node-token 'https://xxx.feishu.cn/wiki/wikcn_EXAMPLE_NODE_TOKEN_123456' --as user --format json
```

节点解析必须与后续 Slides 操作使用相同身份；下游明确使用 `--as bot` 时，这里也改为 `--as bot`。

带 `--presentation` 的 slides shortcut 都会自动解析 `/wiki/` URL 并校验 `obj_type`；手动调用 `xml_presentations.*` / `xml_presentation.slide.*` 时才需要自己做这一步。

### 资源关系

```text
Wiki Space (知识空间)
└── Wiki Node (知识库节点, obj_type: slides)
    └── obj_token → xml_presentation_id

Slides (演示文稿)
├── xml_presentation_id (演示文稿唯一标识)
├── revision_id (版本号)
└── Slide (幻灯片页面)
    └── slide_id (页面唯一标识)
```

## Shortcuts 与 API

Shortcut 是对常用操作的高级封装（`lark-cli slides +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+create`](references/cli/lark-slides-create.md) | 创建 PPT，可选一步添加页面 |
| [`+add-slide`](references/xml/lark-slides-add-slide.md) | 向已有演示文稿追加或插入**一页**（`--before-slide-id` 控制位置），XML 支持 `@file` / stdin，`<img src="@./path">` 占位符自动上传 |
| [`+delete-slide`](references/xml/lark-slides-delete-slide.md) | 按 `slide_id` 删除**一页** |
| [`+xml-get`](references/cli/lark-slides-xml-presentations-get.md) | 读取全文 XML，用 `--presentation` 指定演示文稿的 `xml_presentation_id`，用 `--output` 把 XML 存到本地文件（必须是 CWD 内的相对路径，如 `.lark-slides/plan/<deck>/readback.xml`） |
| [`+screenshot`](references/cli/lark-slides-screenshot.md) | 把幻灯片页面截图保存为本地图片；用 `--slide-number` 指定页码（从 1 开始，多页重复传入）或用 `--slide-id` 指定页面；单张用 `--output .lark-slides/screenshots/<deck-or-task-id>/page-01`，批量用 `--output-dir .lark-slides/screenshots/<deck-or-task-id>`（一次最多 10 页）；后续必须读取返回的 `output` / `screenshots[].path` |
| [`+media-upload`](references/cli/lark-slides-media-upload.md) | 上传本地图片到指定演示文稿，返回 `file_token`（用作 `<img src="...">`），最大 20 MB |
| [`+replace-slide`](references/cli/lark-slides-replace-slide.md) | 对已有幻灯片页面进行块级替换/插入（`block_replace` / `block_insert`），自动注入 id 和 `<content/>`，不改变页序 |
| [`+update-slide`](references/lark-slides-update-slide.md) | 把一整页 XML 交给已有页面，页面变成 `--content` 描述的样子；能一次改样式/插入/删除/备注/背景，`slide_id` 和页序不变。**没写进 `--content` 的元素会被删除** |

没有 Shortcut 覆盖时使用原生 API。高频资源：`slides +xml-get` 读取全文；`xml_presentation.slide.create/delete/get/replace` 管理单页。

```bash
lark-cli schema slides.<resource>.<method>   # 调用 API 前必须先查看参数结构
lark-cli slides <resource> <method> [flags] # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `--data` / `--params` 参数结构，不要猜测字段格式。

## 核心规则

1. **先规划再写 XML**：新建演示文稿或大幅改写页面时，必须先写入 `.lark-slides/plan/<deck-or-task-id>/slide_plan.json`；模板、风格和大纲只能作为规划输入，不能绕过规划层
2. **创建流程**：新建演示文稿用 `slides +create`，一步创建还是两步创建按 [`cli/lark-slides-create.md`](references/cli/lark-slides-create.md) 判断
3. **`<slide>` 直接子元素只有 `<style>`、`<data>`、`<note>`**：文本和图形必须放在 `<data>` 内
4. **文本通过 `<content>` 表达**：必须用 `<content><p>...</p></content>`，不能把文字直接写在 shape 内；不要混淆 XML 元素 `<content>` 和 `--parts` 的 JSON 字段：编写 `--parts` 时，`block_replace` 装载 XML 使用标准字段 `replacement`，`block_insert` 使用 `insertion`
5. **保存关键 ID**：后续操作需要 `xml_presentation_id`、`slide_id`、`revision_id`
6. **删除谨慎**：删除不可逆，删前先回读确认 `slide_id`
7. **编辑已有页面优先原链接更新**：修改单个 shape/img 用 `+replace-slide`（`block_replace` / `block_insert`），不要整页重建；一页改动很多或要改背景用 `+update-slide` 整页覆盖（保 `slide_id` 和页序），多页整页重建就对每页各跑一次 `+update-slide`，不要用 `slides +create` 新建整份 PPT；追加/插入单页用 `+add-slide`、删除单页用 `+delete-slide`，只有这些 shortcut 未覆盖的参数才手动调 `slide.create` / `slide.delete`
8. **`<img src>` 只能用上传到飞书 drive 的 `file_token`，禁止使用 http(s) 外链 URL**：飞书 slides 渲染端不会代理外链图片，外链 src 在 PPT 里通常不显示或显示破图。流程必须是「先把图存到本地 → 用 `slides +media-upload` 上传，或在 `+create --slides` 的 XML 里写 `<img src="@./path">` 占位符自动上传 → 拿 `file_token` 写进 `<img src>`」。如果用户给了网图链接，先 `curl`/下载到 CWD 内再走上传流程，不要直接把外链 URL 塞进 `src`。**图片最大 20 MB**（slides upload API 不支持分片上传）。

> **注意**：如果 md 内容与 `xml/slides_xml_schema_definition.xml` 或 `lark-cli schema slides.<resource>.<method>` 输出不一致，以后两者为准。
