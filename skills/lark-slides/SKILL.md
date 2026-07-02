---
name: lark-slides
version: 1.0.0
description: "飞书幻灯片：创建和编辑幻灯片。创建演示文稿、读取幻灯片内容、管理幻灯片页面（创建、删除、读取、局部替换）。当用户需要创建或编辑幻灯片、读取或修改单个页面时使用。当用户给出 doubao.com 的 /slides/ URL/token 时，也应直接使用本 skill，不要因为域名不是飞书而回退到 WebFetch；路由依据是 URL 路径模式和 token，而不是域名。不负责：云文档内容编辑（走 lark-doc）、云文档里的独立画板对象（走 lark-whiteboard，注意 slide 内嵌的流程图/架构图仍属本 skill）、上传或下载普通文件（走 lark-drive）。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli slides --help"
---

# slides (v1)

## Quick Reference

| 用户需求 | 优先动作 | 关键文档 / 命令 |
|----------|----------|-----------------|
| 新建 PPT | 先规划 `slide_plan.json`，再按复杂度选择一步或两步创建 | `planning-layer.md`、`visual-planning.md`、`asset-planning.md`、`slides +create` |
| 已有 PPT 大幅改写 | 多页整页重建用 `+replace-pages`，单页局部编辑用 `+replace-slide` | `xml_presentations.get`、`lark-slides-replace-pages.md`、`lark-slides-edit-workflows.md` |
| 编辑单个标题、文本块、图片或局部元素 | 优先块级替换/插入，不改页序 | `slides +replace-slide`、`lark-slides-replace-slide.md` |
| 读取或分析已有 PPT | 解析 slides/wiki token，回读全文或单页 XML，保存 `xml_presentation_id`、`slide_id`、`revision_id` | `xml_presentations.get`、`xml_presentation.slide.get` |
| 获取幻灯片页面截图 | 用 `slide_id` 或页号指定页面 | `slides +screenshot`、`lark-slides-screenshot.md` |
| 上传或使用图片 | 先上传为 `file_token`，禁止直接写 http(s) 外链 | `slides +media-upload`，或 `+create --slides` 的 `@./path` 占位符 |
| 在 slide 中绘制柱/条/折线/面积/雷达/饼等有数据序列的图表 | 使用原生 `<chart>` 元素 | `xml-schema-quick-ref.md` |
| 在 slide 中绘制流程图、时序图、架构图、散点图、漏斗图或装饰图案 | 必须先用 Read 工具读取参考文档，再生成 `<whiteboard>` 元素 | [`lark-slides-whiteboard.md`](references/lark-slides-whiteboard.md) |
| 使用语义图标 | 先检索 IconPark，再写 `<icon iconType="...">` | `iconpark_tool.py search → resolve`、`iconpark.md` |
| 创建失败、空白页、3350001、布局异常 | 先回读状态，再按排障清单修复，不假设原操作原子成功 | `troubleshooting.md`、`validation-checklist.md` |

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，认证、权限和全局参数均以 lark-shared 为准。**

**CRITICAL — 生成任何 XML 之前，MUST 先用 Read 工具读取 [xml-schema-quick-ref.md](references/xml-schema-quick-ref.md)，禁止凭记忆猜测 XML 结构。**

**CRITICAL — PPT 生成与模板编辑硬约束：PPT 的尺寸是 960x540，确保主体内容在页面边界内。多用生图，辅助搜图，必须要图文并茂。不要为了画出一个具象物体而堆叠 3 个以上仅用于拟形的 shape。生成背景图时必须在 prompt 中明确要求不要出现任何文字。用户指定 PPT 模板时，用 lark-drive 技能导入成 lark slides，回读理解每页版式后，直接在该 slides 上编辑，可以填改文字和图片、按需增删模板页，必须严格沿用原版式和字体，只改内容不做设计，完成后回读并微调，凝练文字或缩减字号消除文字溢出，调整 shape 顺序或位置避免文字遮挡。**

**CRITICAL — 新建演示文稿或大幅改写页面时，MUST 先生成 `.lark-slides/plan/<deck-or-task-id>/slide_plan.json`，再生成 XML。先创建对应目录，规划层规则和中间产物生命周期见 [planning-layer.md](references/planning-layer.md)。仅替换一个标题、插入一个块等小型已有页编辑可豁免。**

**CRITICAL — 新建演示文稿或大幅改写页面时，生成 XML 前 MUST 读取 [visual-planning.md](references/visual-planning.md)，确保 `layout_type`、`visual_focus`、`text_density` 实际改变页面几何、主视觉和文本量。**

**CRITICAL — 新建演示文稿或大幅改写页面时，规划 `asset_need` MUST 遵循 [asset-planning.md](references/asset-planning.md)：只做元数据规划，必须有 `fallback_if_missing`，不得要求真实搜索、下载或上传素材。**

**CRITICAL — 创建或大幅改写后，MUST 按 [validation-checklist.md](references/validation-checklist.md) 做显式验证：回读全文 XML、核对页数和关键元素、检查空白/破损页、明显溢出、布局风险；XML 语法和文本重叠静态检查优先使用 [`scripts/xml_text_overlap_lint.py`](scripts/xml_text_overlap_lint.py)。**

**CRITICAL — 创建前自检或失败排障时，MUST 按 [troubleshooting.md](references/troubleshooting.md) 检查 XML 转义、结构、shell 截断、图片 token、3350001 和布局风险。**

**编辑已有幻灯片页面**：单个标题、文本块、图片或局部元素优先用 [`+replace-slide`](references/lark-slides-replace-slide.md)（块级替换/插入，不动页序）；已有 Slides 的多页大改优先用 [`+replace-pages`](references/lark-slides-replace-pages.md) 在原 presentation 内批量重建页面，避免 `slides +create` 生成新链接。选择 action 和完整读-改-写流程见 [`lark-slides-edit-workflows.md`](references/lark-slides-edit-workflows.md)。

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

> **重要**：`references/slides_xml_schema_definition.xml` 是此 skill 唯一正确的 XML 协议来源；其他 md 仅是对它和 CLI schema 的摘要。

高频只读：

- [xml-schema-quick-ref.md](references/xml-schema-quick-ref.md)
- [planning-layer.md](references/planning-layer.md)（新建 / 大幅改写）
- [visual-planning.md](references/visual-planning.md)（新建 / 大幅改写）
- [asset-planning.md](references/asset-planning.md)（新建 / 大幅改写）
- [validation-checklist.md](references/validation-checklist.md)（创建 / 大幅改写后）

按需再读：

- 创建：[`lark-slides-create.md`](references/lark-slides-create.md)
- 编辑：[`lark-slides-edit-workflows.md`](references/lark-slides-edit-workflows.md)、[`lark-slides-replace-slide.md`](references/lark-slides-replace-slide.md)、[`lark-slides-replace-pages.md`](references/lark-slides-replace-pages.md)
- 截图：[`lark-slides-screenshot.md`](references/lark-slides-screenshot.md)
- 图片：[`lark-slides-media-upload.md`](references/lark-slides-media-upload.md)
- 流程图 / 时序图 / 架构图 / 装饰图案：[`lark-slides-whiteboard.md`](references/lark-slides-whiteboard.md)
- 图标：[`iconpark.md`](references/iconpark.md)、[`scripts/iconpark_tool.py`](scripts/iconpark_tool.py)
- 排障：[`troubleshooting.md`](references/troubleshooting.md)
- 完整协议：[`slides_xml_schema_definition.xml`](references/slides_xml_schema_definition.xml)

## Workflow

> **这是演示文稿，不是文档。** 每页 slide 是独立的视觉画面，信息密度要低，排版要留白。

### Design Ideas

不要生成无设计感的幻灯片。纯白背景 + 标题 + bullets 只能作为极简临时稿，不能作为正式交付。

开始写 XML 前，先在 `slide_plan.json` 里确定 deck 级视觉策略：

- **主题化配色**：配色必须服务本次主题、行业和受众，不要默认蓝色商务风。如果把同一套颜色换到另一个完全不同主题仍然成立，说明配色不够具体。
- **主次比例**：选择 1 个主色承担约 60-70% 视觉权重，1-2 个辅助色承担结构和分区，1 个强调色只用于关键数字、结论或行动点。不要让所有颜色权重相同。
- **背景一致性**：先确定全 deck 的背景策略，默认保持同一明暗基调和底色体系；只有分节、转场或强调页才有意改变背景，并必须通过相同主色、纹理、边栏或 motif 让变化看起来属于同一套设计。无论深浅，都要保证正文、图标和线条对比充足。
- **统一 motif**：选择一个可复用视觉母题贯穿全文，例如粗侧边栏、圆形图标底、半出血图片区、编号节点、卡片左上角色块或大号数字。不要每页换一套装饰语言。

每页至少要有一个视觉元素：图片、图标、图表、表格、流程、对比结构、大号数字、示意图或由 shape 组成的抽象视觉。文本框本身不算主视觉。

可优先考虑这些页面形态：

- **双栏结构**：左文右图或左图右文，视觉区域占 35-45% 宽度。
- **图标行**：图标在色块或圆形底中，右侧是短标题和一句解释。
- **2x2 / 2x3 网格**：适合能力、模块、风险、行动项，每格内容保持同等层级。
- **半出血视觉**：图片或抽象形状占据左/右半屏，文字覆盖或贴边排布。
- **大数字卡片**：关键指标用 60-72pt 数字，下面配 10-14pt 标签。
- **对比列**：before/after、方案 A/B、问题/解法用左右并列，标题和基线严格对齐。
- **时间线/流程图**：步骤用节点和箭头表达，流程方向必须一眼可见。

字体和间距建议：

- 标题 36-44pt，关键结论可更大；正文 14-18pt；注释 10-12pt。
- 正文默认左对齐；只在封面、结尾或大号数字场景中使用居中。
- 页面边距至少 40px；内容块之间保持 24-40px 间距，并在同一 deck 内保持一致。
- 卡片内边距要真实留出空间，不要让文字贴边；对齐 shape 和文字时要考虑文本框 padding。

常见错误必须避免：

- 不要所有页面复用同一种标题 + 三 bullets 版式。
- 不要用低对比文字或低对比图标，例如浅灰字压在浅色背景上。
- 不要让装饰线穿过文字，或让页脚、来源、编号挤压主体内容。
- 不要把素材缺失表现为空白图片框；必须按 `fallback_if_missing` 生成 XML-native 视觉。
- 不要留下占位文案、示例公司名、示例日期或与用户主题无关的内容。

### 创建方式选择

| 场景 | 推荐方式 |
|------|----------|
| 简单 XML（1-3 页、结构简单、几乎无复杂中文和特殊字符） | `slides +create --slides '[...]'` 一步创建 |
| 复杂 XML（多页、含中文、大段文本、复杂布局、嵌套引号、特殊字符较多） | **两步创建**：先 `slides +create` 创建空白 PPT，再用 `xml_presentation.slide create` 逐页添加 |
| 已有 PPT 继续追加或插入页面 | 使用 `xml_presentation.slide create`，必要时配合 `before_slide_id` |

> [!WARNING]
> `--slides '[...]'` 的风险点主要在 shell 参数传递，而不是单纯页数。即使只有 1 页，只要 XML 足够复杂，也建议使用两步创建法。

> [!IMPORTANT]
> `slides +create --slides` 底层会逐页创建，不是原子操作。中途失败时先记录 `xml_presentation_id`，回读确认当前状态，再继续修复或追加。

```text
Step 1: 需求澄清 & 读取知识
  - 澄清主题、受众、页数、风格
  - 读取 xml-schema-quick-ref.md；新建 / 大幅改写时还要读取 planning-layer.md、visual-planning.md、asset-planning.md

Step 2: 生成大纲 → 用户确认 → 写入 slide_plan.json
  - 生成结构化大纲供用户确认
  - 新建 / 大幅改写必须先创建目录并写入 `.lark-slides/plan/<deck-or-task-id>/slide_plan.json`
  - plan 字段、路径命名和 `asset_need` 结构按 planning-layer.md / asset-planning.md 执行

Step 3: 按 slide_plan.json 生成 XML → 创建
  - 逐页消费 plan：key_message 定主结论，layout_type 定几何，visual_focus 定主视觉，text_density 定文本量
  - 缺少真实素材时必须用 `fallback_if_missing` 生成 XML-native 兜底视觉；不要留空
  - 创建方式按“创建方式选择”判断；图片、复杂 XML、转义和 3350001 排查按 lark-slides-create.md、media-upload.md、troubleshooting.md 执行

Step 4: 审查 & 交付
  - 创建完成后，必须用 xml_presentations.get 读取全文 XML，并按 validation-checklist.md 做显式验证记录，包括 XML 文本重叠检查
  - 失败或部分成功按 troubleshooting.md 处理；局部问题优先用 `+replace-slide` 修正
  - 没问题 → 交付：告知用户演示文稿 ID 和访问方式
```

### jq 命令模板（编辑已有 PPT 时使用）

新建 PPT 推荐用 `+create --slides`。以下 jq 模板适用于向已有演示文稿追加页面的场景，可以避免手动转义双引号：

```bash
# 追加到末尾
lark-cli slides xml_presentation.slide create \
  --as user \
  --params '{"xml_presentation_id":"YOUR_ID"}' \
  --data "$(jq -n --arg content '<slide xmlns="http://www.larkoffice.com/sml/2.0">
  <style><fill><fillColor color="BACKGROUND_COLOR"/></fill></style>
  <data>
    在这里放置 shape、line、table、chart、whiteboard 等元素
  </data>
</slide>' '{slide:{content:$content}}')"

# 插到指定页之前：before_slide_id 必须在 --data body 里，与 slide 同级
# ⚠️ 不要把 before_slide_id 写进 --params —— CLI 会当未知 query 参数静默下发，服务端忽略，新页跑到末尾
lark-cli slides xml_presentation.slide create \
  --as user \
  --params '{"xml_presentation_id":"YOUR_ID"}' \
  --data "$(jq -n --arg content '<slide ...>...</slide>' --arg before 'TARGET_SLIDE_ID' \
    '{slide:{content:$content}, before_slide_id:$before}')"
```

> 渐变色必须使用 `rgba()` 格式并带百分比停靠点，如 `linear-gradient(135deg,rgba(15,23,42,1) 0%,rgba(56,97,140,1) 100%)`。使用 `rgb()` 或省略停靠点会导致服务端回退为白色。

### 大纲模板

生成大纲时使用以下格式，交给用户确认：

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
| `/wiki/` | `https://example.larkoffice.com/wiki/wikcnxxxxxxxxx` | `wiki_token` | ⚠️ **不能直接使用**，需要先查询获取真实的 `obj_token` |

> `+replace-slide` 和 `+media-upload` shortcut 会自动解析以上两种 URL；直接调用原生 API 时仍需手动解析 wiki 链接。

### Wiki 链接特殊处理（关键！）

知识库链接（`/wiki/TOKEN`）不能直接当 `xml_presentation_id`。直接调用原生 API 前，先查询 wiki 节点，确认 `node.obj_type == "slides"`，再用 `node.obj_token` 作为真实 presentation ID。

```bash
lark-cli wiki spaces get_node --as user --params '{"token":"wiki_token"}'
```

Shortcut `+replace-slide` 和 `+media-upload` 会自动解析 `/wiki/` URL；手动调用 `xml_presentations.*` / `xml_presentation.slide.*` 时才需要自己做这一步。

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
| [`+create`](references/lark-slides-create.md) | 创建 PPT（可选 `--slides` 一步添加页面，支持 `<img src="@./local.png">` 占位符自动上传） |
| [`+media-upload`](references/lark-slides-media-upload.md) | 上传本地图片到指定演示文稿，返回 `file_token`（用作 `<img src="...">`），最大 20 MB |
| [`+replace-slide`](references/lark-slides-replace-slide.md) | 对已有幻灯片页面进行块级替换/插入（`block_replace` / `block_insert`），自动注入 id 和 `<content/>`，不改变页序 |
| [`+replace-pages`](references/lark-slides-replace-pages.md) | 在原演示文稿内批量重建多个页面：先创建新页到旧页前，再删除旧页；适合已有 Slides 的多页大改，不新建链接 |

没有 Shortcut 覆盖时使用原生 API。高频资源：`xml_presentations.get` 读取全文；`xml_presentation.slide.create/delete/get/replace` 管理单页。

```bash
lark-cli schema slides.<resource>.<method>   # 调用 API 前必须先查看参数结构
lark-cli slides <resource> <method> [flags] # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `--data` / `--params` 参数结构，不要猜测字段格式。

## 核心规则

1. **先规划再写 XML**：新建演示文稿或大幅改写页面时，必须先写入 `.lark-slides/plan/<deck-or-task-id>/slide_plan.json`；风格和大纲只能作为规划输入，不能绕过规划层
2. **创建流程**：简单短 XML（1-3 页、结构简单、特殊字符少）可用 `slides +create --slides '[...]'` 一步创建；复杂内容、含图片/中文大段文本/嵌套引号/较多特殊字符，或超过 10 页时，默认先 `slides +create` 创建空白 PPT，再用 `xml_presentation.slide.create` 逐页添加
3. **`<slide>` 直接子元素只有 `<style>`、`<data>`、`<note>`**：文本和图形必须放在 `<data>` 内
4. **文本通过 `<content>` 表达**：必须用 `<content><p>...</p></content>`，不能把文字直接写在 shape 内
5. **保存关键 ID**：后续操作需要 `xml_presentation_id`、`slide_id`、`revision_id`
6. **删除谨慎**：删除操作不可逆，且至少保留一页幻灯片
7. **编辑已有页面优先原链接更新**：修改单个 shape/img 用 `+replace-slide`（`block_replace` / `block_insert`），不要整页重建；已有 Slides 的多页整页重建用 `+replace-pages`，不要用 `slides +create` 新建整份 PPT；只有没有 shortcut 覆盖的特殊单页整页操作才手动 `slide.create` + `slide.delete`
8. **`<img src>` 只能用上传到飞书 drive 的 `file_token`，禁止使用 http(s) 外链 URL**：飞书 slides 渲染端不会代理外链图片，外链 src 在 PPT 里通常不显示或显示破图。流程必须是「先把图存到本地 → 用 `slides +media-upload` 上传或 `+create --slides` 的 `@./path` 占位符自动上传 → 拿 `file_token` 写进 `<img src>`」。如果用户给了网图链接，先 `curl`/下载到 CWD 内再走上传流程，不要直接把外链 URL 塞进 `src`。**图片最大 20 MB**（slides upload API 不支持分片上传）。

> **注意**：如果 md 内容与 `slides_xml_schema_definition.xml` 或 `lark-cli schema slides.<resource>.<method>` 输出不一致，以后两者为准。
