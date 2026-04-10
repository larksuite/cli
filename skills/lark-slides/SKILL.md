---
name: lark-slides
version: 1.0.0
description: "飞书幻灯片：创建和编辑幻灯片，接口通过 XML 协议通信。创建演示文稿、读取幻灯片内容、管理幻灯片页面（创建、删除、读取、局部替换）。当用户需要创建或编辑幻灯片、读取或修改单个页面时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli slides --help"
---

# slides (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

**CRITICAL — 生成任何 XML 之前，MUST 先用 Read 工具读取 [xml-schema-quick-ref.md](references/xml-schema-quick-ref.md)，禁止凭记忆猜测 XML 结构。**

**CRITICAL — 如果用户提到“模板”“套用模板”“参考某种主题/风格/版式”，或用户需求明显落在已有场景模板内（如工作汇报、产品介绍、商业计划书、培训、晋升汇报等），MUST 先读取 [template-index.json](references/template-index.json) 或运行 [`scripts/template-tool.py`](scripts/template-tool.py) 做模板检索；默认给出 2-3 个最匹配模板候选供用户选择。只有确定要复用某类版式时，才进一步读取 [template-catalog.md](references/template-catalog.md) 或按页型裁切 `references/templates/*.xml` 片段；不要默认阅读全文模板 XML。**

> [!NOTE]
> `scripts/template-tool.py` / `scripts/layout-lint.py` 需要 Python 3，但它们是可选辅助路径，不是强制依赖。没有 Python 时，退回 “`template-index.json` → `template-catalog.md` → 按范围读取 XML 片段” 的纯文档路径。

**编辑已有幻灯片页面**：优先用 [`+replace-slide`](references/lark-slides-replace-slide.md)（块级替换/插入，不动页序）；选择 action 和完整读-改-写流程见 [`lark-slides-edit-workflows.md`](references/lark-slides-edit-workflows.md)。

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

## 快速开始

一条命令创建包含页面内容的 PPT（推荐）：

```bash
lark-cli slides +create --title "演示文稿标题" --slides '[
  "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><style><fill><fillColor color=\"rgb(245,245,245)\"/></fill></style><data><shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"100\"><content textType=\"title\"><p>页面标题</p></content></shape><shape type=\"text\" topLeftX=\"80\" topLeftY=\"200\" width=\"800\" height=\"200\"><content textType=\"body\"><p>正文内容</p><ul><li><p>要点一</p></li><li><p>要点二</p></li></ul></content></shape></data></slide>"
]'
```

也可以分两步（先创建空白 PPT，再逐页添加），详见 [+create 参考文档](references/lark-slides-create.md)。

> [!WARNING]
> `--slides '[...]'` 适合简单页面批量创建，但并不等同于“10 页以内都安全”。如果 slide XML 含中文、大段文本、复杂布局、嵌套引号或较多特殊字符，shell 传参时可能出现转义或截断问题，导致内容丢失、页面空白或布局异常。遇到复杂页面时，优先改用“两步创建法”。

> [!IMPORTANT]
> `slides +create --slides` 底层是“先创建空白 PPT，再逐页调用 `xml_presentation.slide.create`”。这不是原子操作；中途某一页失败时，前面已创建成功的页面会保留。skill 必须把这种“部分成功”风险提前告诉用户，并在失败后先记录 `xml_presentation_id`，回读确认当前状态，再决定是否在现有 PPT 上继续修复或追加。

> 以上是最小可用示例。更丰富的页面效果（渐变背景、卡片、图表、表格等），参考下方 Workflow 和 XML 模板。

## 执行前必做

> **重要**：`references/slides_xml_schema_definition.xml` 是此 skill 唯一正确的 XML 协议来源；其他 md 仅是对它和 CLI schema 的摘要。

### 必读（每次创建前）

| 文档 | 说明 |
|------|------|
| [xml-schema-quick-ref.md](references/xml-schema-quick-ref.md) | **XML 元素和属性速查，必读** |

### 选读（需要时查阅）

| 场景 | 文档 |
|------|------|
| 需要了解详细 XML 结构 | [xml-format-guide.md](references/xml-format-guide.md) |
| 需要快速筛模板、做低成本路由 | [template-index.json](references/template-index.json) |
| 需要匹配 PPT 模板/主题风格 | [template-catalog.md](references/template-catalog.md) |
| 需要按页型抽摘要或裁切 XML 片段 | [`scripts/template-tool.py`](scripts/template-tool.py) |
| 需要做本地布局风险检查 | [`scripts/layout-lint.py`](scripts/layout-lint.py) |
| 需要 CLI 调用示例 | [examples.md](references/examples.md) |
| 需要参考真实 PPT 的 XML | [slides_demo.xml](references/slides_demo.xml) |
| 需要用 table/chart 等复杂元素 | [slides_xml_schema_definition.xml](references/slides_xml_schema_definition.xml)（完整 Schema） |
| 需要编辑已有 PPT 的单个页面 | [lark-slides-edit-workflows.md](references/lark-slides-edit-workflows.md) |
| 需要了解某个命令的详细参数 | 对应命令的 reference 文档（见下方参考文档章节） |

## Workflow

> **这是演示文稿，不是文档。** 每页 slide 是独立的视觉画面，信息密度要低，排版要留白。

### 创建方式选择

| 场景 | 推荐方式 |
|------|----------|
| 简单 XML（1-3 页、结构简单、几乎无复杂中文和特殊字符） | `slides +create --slides '[...]'` 一步创建 |
| 复杂 XML（多页、含中文、大段文本、复杂布局、嵌套引号、特殊字符较多） | **两步创建**：先 `slides +create` 创建空白 PPT，再用 `xml_presentation.slide create` 逐页添加 |
| 已有 PPT 继续追加或插入页面 | 使用 `xml_presentation.slide create`，必要时配合 `before_slide_id` |

> [!WARNING]
> `--slides '[...]'` 的风险点主要在 shell 参数传递，而不是单纯页数。即使只有 1 页，只要 XML 足够复杂，也建议使用两步创建法。

```text
Step 1: 需求澄清 & 读取知识
  - 澄清用户需求：主题、受众、页数、风格偏好
  - 如果需求明显落在已有模板场景内，主动提示用户“可以直接基于现成模板生成”，并给出 2-3 个最匹配模板候选（模板名 + 适用场景 + 风格/色调 + 简短推荐理由）
  - 默认不要把完整模板目录直接贴给用户；除非用户明确要求看更多，否则只展示 2-3 个候选
  - 候选优先选场景强相关模板；只有没有明显场景模板时，才用 `light_general.xml` / `dark_general.xml` 这类通用模板兜底
  - 如果用户没有明确风格，根据主题推荐（见下方风格判断表）
  - 如果用户要求“模板/主题/风格参考”，或主题属于常见模板场景：
    · 先读 template-index.json，或运行 `python3 skills/lark-slides/scripts/template-tool.py search --query "<主题>" --limit 3` 做低成本模板匹配
    · 需要人类可读说明时，再读 template-catalog.md 组织候选文案
    · 锁定模板后，优先运行 `template-tool.py summarize` 看 `<theme>` / 页型摘要；需要具体布局时，再用 `template-tool.py extract` 或按 range 读取 XML 片段
    · 复用模板的 theme、配色、页面流、布局骨架，不要照搬占位文案
    · 除非用户明确要求查看整份模板，否则不要默认读取 `references/templates/*.xml` 全文
  - 读取 XML Schema 参考：
    · xml-schema-quick-ref.md — 元素和属性速查
    · xml-format-guide.md — 详细结构与示例
    · slides_demo.xml — 真实 XML 示例

Step 2: 生成大纲 → 用户确认 → 创建
  - 生成大纲前，先确认用户是否采用推荐模板；如果用户没有明确反对且候选中有明显最佳匹配，可默认按最匹配模板继续
  - 生成结构化大纲（每页标题 + 要点 + 布局描述），交给用户确认
  - 如果已选模板，大纲和页面布局要明确标注“基于哪个模板/哪些模板改写”
  - 如果用户明确不要模板，直接按自定义风格继续，不要重复推动模板选择
  - 先判断创建方式：
    · 简单 XML：可用 `slides +create --slides '[...]'` 一步创建
    · 多页复杂 XML：优先用 `slides +compose --slides-dir ./slides --asset-root ./assets`
      或 `--manifest @compose.json`，由 CLI 统一完成建空白 PPT、上传图片、逐页创建
    · 复杂 XML：优先先 `slides +create` 创建空白 PPT，再用 `xml_presentation.slide.create` 逐页添加
    · 超过 10 页：默认使用两步创建，避免单次输入过长
  - 含本地图片：
    · 新建带图 PPT —— 在 slide XML 里写 <img src="@./pic.png" .../>，
      +create 会自动上传并替换为 file_token（详见 lark-slides-create.md）
    · 批量把一组图片预上传到已有 PPT —— 用 `slides +bulk-media-upload --presentation $PID --dir ./assets`
      或重复 `--files`，直接拿到 `file_name -> file_token` map
    · 给已有 PPT 加带图新页 —— 先 `slides +media-upload --file ./pic.png --presentation $PID`
      拿到 file_token，再用它写进 slide XML 调 xml_presentation.slide.create
    · 给已有页加图 —— 两步：① `slides +media-upload` 拿 file_token
      ② `slides +replace-slide --parts '[{"action":"block_insert","insertion":"<img src=\"<file_token>\" .../>"}]'`
      不动其他元素，不要再整页重建（完整示例见 lark-slides-edit-workflows.md 的 block_insert 章节）
    · 路径必须是 CWD 内的相对路径（如 ./pic.png 或 ./assets/x.png）；
      `+compose` / `+replace-slide-xml` / `+bulk-media-upload` 可额外用 `--asset-root ./assets`
      降低素材目录切换成本，但本质上仍要求路径落在当前工作目录白名单内
  - 每页 slide 需要完整的 XML：背景、文本、图形、配色
  - 复杂元素（table、chart）需参考 XSD 原文
  - 创建前必须做 XML 自检：
    · 检查特殊字符是否按 XML 规则转义：`& -> &amp;`、`< -> &lt;`、`> -> &gt;`
    · 属性值里的双引号必须转义或改为外层安全包装，避免 shell 和 JSON 双重截断
    · 确认所有标签闭合，且 `<slide>` 直接子元素只包含 `<style>`、`<data>`、`<note>`
    · 如果内容里同时出现中文、大段文本、复杂布局、较多特殊字符，默认不要走 `--slides '[...]'`，直接改用两步创建法或 `+compose`
  - 如果使用模板生成页面，先复用模板骨架再填内容，不要直接复制模板中的长段占位文本
  - 默认不要推荐 heredoc Python 作为主路径：
    · 多页 XML 文件已落盘时，优先 `slides +compose --slides-dir ./slides`
    · 已有 manifest 时，优先 `slides +compose --manifest @compose.json`
    · 只有在 CLI 现有 shortcut 明显覆盖不了时，才退回临时脚本包装

Step 3: 审查 & 交付
  - 创建完成后，必须用 xml_presentations.get 读取全文 XML 做创建后验证，确认：
    · 页数是否正确？
    · 每页 `<data>` 是否包含预期的 `<shape>` / `<img>` / 其他元素？
    · 文本内容是否完整，是否有被截断、丢失、空白区域？
    · 关键布局坐标和尺寸是否合理，是否出现明显重叠？
    · 配色是否统一？字号层级是否合理？
  - 如果本地有 Python 3，建议额外运行
    `python3 skills/lark-slides/scripts/layout-lint.py --input presentation.xml`
    做重叠、越界、文本高度风险检查
  - 如果创建过程中失败：
    · 先保留并记录 `xml_presentation_id`，不要假设失败代表什么都没创建
    · 先判断是否已有部分页面写入，再决定是否在现有 PPT 上修复后继续追加
    · 优先排查当前失败页：先看该页 XML，再检查是否存在未转义 `&`、错误引号、标签未闭合、shell 传参截断
  - 局部问题 → 用 `+replace-slide` 块级修正；整页结构要改 → `slide.delete` 旧页 + `slide.create` 新页
  - 没问题 → 交付：告知用户演示文稿 ID 和访问方式
```

### 创建后验证

创建成功不等于内容正确。创建完 PPT 后，**必须**读取全文 XML 校验结果：

```bash
lark-cli slides xml_presentations get --as user \
  --params '{"xml_presentation_id":"YOUR_ID"}'
```

重点检查：

- [ ] 页数是否与预期一致
- [ ] 每页 `<data>` 中是否包含所有预期元素
- [ ] 文本内容是否完整，没有被 shell 截断或转义损坏
- [ ] 白底内容区、卡片区、图文区等关键布局是否实际生成
- [ ] 坐标、宽高是否合理，是否出现堆叠或越界

发现问题时：

1. 不要假设“创建成功就代表渲染正确”
2. 先读取问题页的 XML，确认是生成问题还是传参损坏
3. 删除问题页后重新添加；复杂页面优先改用两步创建法

### 最小验收清单

创建完成后，默认按下面顺序验收，不要省略：

1. 记录 `xml_presentation_id`
2. 确认返回的 `slides_added` 或实际页数是否符合预期
3. 立即执行 `xml_presentations get`
4. 检查标题、关键页面、关键文本是否存在
5. 检查是否有明显空白页、内容缺失、页序错误
6. 再决定是否向用户交付 URL 和后续编辑建议

推荐最小闭环：

```bash
# 创建
lark-cli slides +create --as user --title "Demo" --slides '[...]'

# 立即回读
lark-cli slides xml_presentations get --as user \
  --params '{"xml_presentation_id":"YOUR_ID"}'
```

## XML 自检与排障

在真正创建前，至少做下面 4 项检查：

- [ ] 特殊字符已转义：正文和标题里的 `&`、`<`、`>` 不能裸写
- [ ] 属性引号安全：XML 属性、shell 引号、JSON 字符串包装之间没有互相打断
- [ ] 结构合法：`<slide>` 下只放 `<style>`、`<data>`、`<note>`，文本都在 `<content>` 内
- [ ] 路径正确：`<img src="@...">` 只在 `+create --slides` / `+compose` / `+replace-slide-xml` 的支持链路中使用

高频失败信号和处理顺序：

1. `invalid param` / 某一页创建失败
2. 先检查失败页是否含未转义 `&`
3. 再检查标签闭合、属性引号、`<content>` 结构
4. 如果是 `--slides '[...]'`，怀疑 shell 截断时直接切两步创建法
5. 创建后无论成功失败，都优先记录 `xml_presentation_id` 并回读确认是否已有部分页面写入

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
    在这里放置 shape、line、table、chart 等元素
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

### 风格快速判断表

> **注意**：渐变色必须使用 `rgba()` 格式并带百分比停靠点，如 `linear-gradient(135deg,rgba(15,23,42,1) 0%,rgba(56,97,140,1) 100%)`。使用 `rgb()` 或省略停靠点会导致服务端回退为白色。

| 场景/主题 | 推荐风格 | 背景 | 主色 | 文字色 |
|----------|---------|------|------|-------|
| 科技/AI/产品 | 深色科技风 | 深蓝渐变 `linear-gradient(135deg,rgba(15,23,42,1) 0%,rgba(56,97,140,1) 100%)` | 蓝色系 `rgb(59,130,246)` | 白色 |
| 商务汇报/季度总结 | 浅色商务风 | 浅灰 `rgb(248,250,252)` | 深蓝 `rgb(30,60,114)` | 深灰 `rgb(30,41,59)` |
| 教育/培训 | 清新明亮风 | 白色 `rgb(255,255,255)` | 绿色系 `rgb(34,197,94)` | 深灰 `rgb(51,65,85)` |
| 创意/设计 | 渐变活力风 | 紫粉渐变 `linear-gradient(135deg,rgba(88,28,135,1) 0%,rgba(190,24,93,1) 100%)` | 粉紫色系 | 白色 |
| 周报/日常汇报 | 简约专业风 | 浅灰 `rgb(248,250,252)` + 顶部彩色渐变条 | 蓝色 `rgb(59,130,246)` | 深色 `rgb(15,23,42)` |
| 用户未指定 | 默认简约专业风 | 同上 | 同上 | 同上 |

### 页面布局建议

| 页面类型 | 布局要点 |
|---------|---------|
| 封面页 | 居中大标题 + 副标题 + 底部信息，背景用渐变或深色 |
| 数据概览页 | 指标卡片横排（rect 背景 + 大号数字 + 小号说明），下方列表或图表 |
| 内容页 | 左侧竖线装饰 + 标题，下方分栏或列表 |
| 对比/表格页 | table 元素或并列卡片，表头深色背景白字 |
| 图表页 | chart 元素（column/line/pie），配合文字说明 |
| 结尾页 | 居中感谢语 + 装饰线，风格与封面呼应 |

### 大纲模板

生成大纲时使用以下格式，交给用户确认：

```text
[PPT 标题] — [定位描述]，面向 [目标受众]

模板：[未使用模板 / <category>/<template>.xml（推荐原因）]

页面结构（N 页）：
1. 封面页：[标题文案]
2. [页面主题]：[要点1]、[要点2]、[要点3]
3. [页面主题]：[要点描述]
...
N. 结尾页：[结尾文案]

风格：[配色方案]，[排版风格]
```

### 模板引导话术模板

当主题明显可套用现成模板时，优先用下面的方式引导用户选择：

```text
这个主题可以直接基于现成模板来做，会比纯自定义更快。我先给你 3 个可选方向：

1. office/work_report.xml
   适合：工作汇报、项目进展
   风格：浅色正式、卡片布局
   推荐理由：信息密度中等时最稳

2. product/product_intro.xml
   适合：产品介绍、产品发布
   风格：多彩分节、视觉更强
   推荐理由：适合强调亮点和分章节表达

3. office/light_general.xml
   适合：内容较自由、但想沿用成熟浅色版式
   风格：浅色通用、页型丰富
   推荐理由：适合作为兜底模板按需挑版式

你可以直接选一个模板，我再按你的内容出大纲；如果不想套模板，我也可以按自定义风格来做。
```

### 场景 -> 模板快速路由

优先按下面的固定顺序推荐模板，减少 agent 临时猜测：

| 用户场景 | 默认推荐 | 备选 1 | 备选 2 |
|---------|---------|--------|--------|
| 读书分享 / 读书会 / 方法论分享 | `misc--book_sharing` | `personal--teaching_sharing` | `personal--experience_sharing` |
| 教学分享 / 培训课件 / 技术分享 | `personal--teaching_sharing` | `hr--employee_training` | `hr--employee_training_workshop` |
| 工作汇报 / 周报 / 月报 / 项目进展 | `office--work_report` | `office--work_summary_report` | `office--light_general` |
| 工作总结 / 年度总结 / 季度复盘 | `office--work_summary` | `office--quarterly_review` | `office--dept_annual_report` |
| 项目启动 / 宣讲 / kickoff | `office--project_kickoff` | `product--product_intro` | `office--dark_general` |
| 产品介绍 / 产品发布 / 产品方案 | `product--product_intro` | `product--product_promotion` | `product--product_promotion_2` |
| 产品分析 / 竞品分析 / 商业分析 | `product--product_analysis` | `product--market_analysis` | `product--business_case_analysis` |
| 商业计划书 / 路演 / 营销方案 | `marketing--business_plan` | `marketing--roadshow_business_plan` | `marketing--marketing_plan` |
| 晋升答辩 / 晋升汇报 / 个人述职 | `personal--promotion_report` | `personal--promotion_defense` | `personal--self_intro` |
| 公司介绍 / 文化宣导 / 全员会 | `administration--company_intro` | `administration--corporate_culture` | `administration--all_hands_meeting` |
| 没有明显场景，但想要成熟版式 | `office--light_general` | `office--dark_general` | 无 |

推荐顺序规则：

1. 先选场景最强相关模板，不先拿 `light_general` / `dark_general` 兜底
2. 用户没提风格时，优先给 1 个默认推荐 + 2 个备选
3. 用户只说“模板化一点”，但主题不明确时，才拿通用模板兜底
4. 用户明确拒绝模板后，直接走自定义风格，不要重复推动模板选择

### 常用 Slide XML 模板

可直接复制使用的模板（封面页、内容页、数据卡片页、结尾页）：[slide-templates.md](references/slide-templates.md)

### 场景模板目录（优先匹配）

当用户明确要求模板，或需求本身已经能映射到典型 PPT 主题时，优先走下面的模板匹配流程：

1. 先读 [template-index.json](references/template-index.json)，或运行 `python3 skills/lark-slides/scripts/template-tool.py search --query "<主题>" --tone <light|dark|colorful> --limit 3`，按场景、色调、正式度先整理出 2-3 个用户可选模板候选
2. 如果用户未明确拒绝模板，优先按最匹配候选继续；如果用户已选择模板，则锁定该模板
3. 需要页型摘要时，先运行 `python3 skills/lark-slides/scripts/template-tool.py summarize --template <template-id> --label <封面|目录|分节|内容|结尾>`
4. 需要具体布局骨架时，再运行 `python3 skills/lark-slides/scripts/template-tool.py extract --template <template-id> --label <页型>`，或用 Read 工具按 range 读取 `references/templates/<category>--<file>.xml` 的局部片段
5. 提取模板中的 `<theme>`、封面样式、目录页、分节页、内容页、结尾页结构
6. 用用户的真实内容重写页面，不要直接复用模板里的占位文字

如果用户只说“帮我做一个产品介绍/晋升汇报/商业计划书 PPT”，即使没明确说“模板”，只要主题与目录中的模板明显匹配，也应该主动给出模板候选，而不是只在内部默默参考模板。

---

## 核心概念

### URL 格式与 Token

| URL 格式 | 示例 | Token 类型 | 处理方式 |
|----------|------|-----------|----------|
| `/slides/` | `https://example.larkoffice.com/slides/xxxxxxxxxxxxx` | `xml_presentation_id` | URL 路径中的 token 直接作为 `xml_presentation_id` 使用 |
| `/wiki/` | `https://example.larkoffice.com/wiki/wikcnxxxxxxxxx` | `wiki_token` | ⚠️ **不能直接使用**，需要先查询获取真实的 `obj_token` |

> `+replace-slide` 和 `+media-upload` shortcut 会自动解析以上两种 URL；直接调用原生 API 时仍需手动解析 wiki 链接。

### Wiki 链接特殊处理（关键！）

知识库链接（`/wiki/TOKEN`）背后可能是云文档、电子表格、幻灯片等不同类型的文档。**不能直接假设 URL 中的 token 就是 `xml_presentation_id`**，必须先查询实际类型和真实 token。

#### 处理流程

1. **使用 `wiki.spaces.get_node` 查询节点信息**
   ```bash
   lark-cli wiki spaces get_node --as user --params '{"token":"wiki_token"}'
   ```

2. **从返回结果中提取关键信息**
   - `node.obj_type`：文档类型，幻灯片对应 `slides`
   - `node.obj_token`：**真实的演示文稿 token**（用于后续操作）
   - `node.title`：文档标题

3. **确认 `obj_type` 为 `slides` 后，使用 `obj_token` 作为 `xml_presentation_id`**

#### 查询示例

```bash
# 查询 wiki 节点
lark-cli wiki spaces get_node --as user --params '{"token":"wikcnxxxxxxxxx"}'
```

返回结果示例：
```json
{
   "node": {
      "obj_type": "slides",
      "obj_token": "xxxxxxxxxxxx",
      "title": "2026 产品年度总结",
      "node_type": "origin",
      "space_id": "1234567890"
   }
}
```

```bash
# 用 obj_token 读取幻灯片内容
lark-cli slides xml_presentations get --as user --params '{"xml_presentation_id":"xxxxxxxxxxxx"}'
```

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

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli slides +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+compose`](references/lark-slides-compose.md) | 从 slide XML 目录或 manifest 创建 PPT，自动上传 `@path` 图片并逐页创建 |
| [`+create`](references/lark-slides-create.md) | 创建 PPT（可选 `--slides` 一步添加页面，支持 `<img src="@./local.png">` 占位符自动上传），bot 模式自动授权 |
| [`+bulk-media-upload`](references/lark-slides-bulk-media-upload.md) | 批量上传本地图片到指定演示文稿，返回 `file_name -> file_token` map |
| [`+media-upload`](references/lark-slides-media-upload.md) | 上传本地图片到指定演示文稿，返回 `file_token`（用作 `<img src="...">`），最大 20 MB |
| [`+replace-slide`](references/lark-slides-replace-slide.md) | 对已有幻灯片页面进行块级替换/插入（`block_replace` / `block_insert`），自动注入 id 和 `<content/>`，不改变页序 |

## API Resources

```bash
lark-cli schema slides.<resource>.<method>   # 调用 API 前必须先查看参数结构
lark-cli slides <resource> <method> [flags] # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `--data` / `--params` 参数结构，不要猜测字段格式。

### xml_presentations

  - `get` — 读取演示文稿全文信息，XML 格式返回

### xml_presentation.slide

  - `create` — 在指定 XML 演示文稿下创建页面
  - `delete` — 在指定 XML 演示文稿下删除页面
  - `get` — 获取指定 XML 演示文稿的单个页面 XML 内容
  - `replace` — 对指定 XML 演示文稿页面进行元素级别的局部替换

## 核心规则

1. **先定模板/风格并出大纲再动手**：如果需求可匹配模板，先给用户 2-3 个模板候选；模板或自定义风格确定后，再生成大纲交给用户确认，避免返工
2. **创建流程按复杂度优先判断，不只看页数**：简单 XML 可用 `slides +create --slides '[...]'`；复杂 XML（含中文、大段文本、复杂布局、特殊字符）即使页数很少，也优先先 `slides +create` 创建空白 PPT，再用 `xml_presentation.slide.create` 逐页添加。超过 10 页默认使用两步创建
3. **`<slide>` 直接子元素只有 `<style>`、`<data>`、`<note>`**：文本和图形必须放在 `<data>` 内
4. **文本通过 `<content>` 表达**：必须用 `<content><p>...</p></content>`，不能把文字直接写在 shape 内
5. **保存关键 ID**：后续操作需要 `xml_presentation_id`、`slide_id`、`revision_id`
6. **删除谨慎**：删除操作不可逆，且至少保留一页幻灯片
7. **编辑已有页面优先块级替换**：修改单个 shape/img 用 `+replace-slide`（`block_replace` / `block_insert`），不要整页重建；只有需要替换整页结构时才用 `slide.delete` + `slide.create`
8. **`<img src>` 只能用上传到飞书 drive 的 `file_token`，禁止使用 http(s) 外链 URL**：飞书 slides 渲染端不会代理外链图片，外链 src 在 PPT 里通常不显示或显示破图。流程必须是「先把图存到本地 → 用 `slides +media-upload` 上传或 `+create --slides` 的 `@./path` 占位符自动上传 → 拿 `file_token` 写进 `<img src>`」。如果用户给了网图链接，先 `curl`/下载到 CWD 内再走上传流程，不要直接把外链 URL 塞进 `src`。**图片最大 20 MB**（slides upload API 不支持分片上传）。

## 权限表

| 方法 | 所需 scope |
|------|-----------|
| `slides +create` | `slides:presentation:create`, `slides:presentation:write_only`（含 `@` 占位符时还需 `docs:document.media:upload`） |
| `slides +media-upload` | `docs:document.media:upload`（wiki URL 解析还需 `wiki:node:read`） |
| `slides +replace-slide` | `slides:presentation:update`（wiki URL 解析还需 `wiki:node:read`） |
| `xml_presentations.get` | `slides:presentation:read` |
| `xml_presentation.slide.create` | `slides:presentation:update` 或 `slides:presentation:write_only` |
| `xml_presentation.slide.delete` | `slides:presentation:update` 或 `slides:presentation:write_only` |
| `xml_presentation.slide.get` | `slides:presentation:read` |
| `xml_presentation.slide.replace` | `slides:presentation:update` |

## 常见错误速查

| 错误码 | 含义 | 解决方案 |
|--------|------|----------|
| 400 | XML 格式错误 | 检查 XML 语法，确保标签闭合 |
| 400 | 请求包装错误 | 检查 `--data` 是否按 schema 传入 `xml_presentation.content` 或 `slide.content` |
| 创建成功但页面空白/内容缺失/布局错乱 | 常见于 `--slides '[...]'` 的 shell 转义或长参数传递问题 | 改用两步创建：先 `slides +create`，再用 `jq -n` 包装 `xml_presentation.slide.create` 逐页添加，并在创建后立即读取 XML 验证 |
| 404 | 演示文稿不存在 | 检查 `xml_presentation_id` 是否正确 |
| 404 | 幻灯片不存在 | 检查 `slide_id` 是否正确 |
| 403 | 权限不足 | 检查是否拥有对应的 scope |
| 400 | 无法删除唯一幻灯片 | 演示文稿至少保留一页幻灯片 |
| 1061002 | params error（媒体上传时） | 用 `slides +media-upload`，不要手拼原生 `medias/upload_all`；slides 唯一可用 `parent_type` 是 `slide_file` |
| 1061004 | forbidden：当前身份对演示文稿无编辑权限 | 确认 user/bot 对目标 PPT 有编辑权限；bot 常见于 PPT 非该 bot 创建，需先授权或用 `+create --as bot` 新建 |
| 3350001 | `xml_presentation.slide.replace` 失败（catch-all） | 检查 `block_replace` 替换根是否带 `id=<block_id>`；`<shape>` 是否含 `<content/>`；坐标是否在 960×540 内。详见 [lark-slides-replace-slide.md](references/lark-slides-replace-slide.md) |
| 3350002 | `revision_id` 大于当前版本 | 用 `-1` 取当前版本，或重新读 `xml_presentations.get` 取最新 `revision_id` |
| validation: unsafe file path | `--file` 给了绝对路径或上层路径 | `--file` 必须是 CWD 内相对路径；先 `cd` 到素材目录再执行 |

## 创建前自查

逐页生成 XML 前，快速检查：

- [ ] 每页背景色/渐变是否设置？风格是否与整体一致？
- [ ] 标题用大字号（28-48），正文用小字号（13-16），层级分明？
- [ ] 同类元素配色一致？（如所有指标卡片同色系、所有正文同色）
- [ ] 装饰元素（分割线、色块、竖线）颜色是否与主色协调？
- [ ] 文本框尺寸是否足够容纳内容？（宽度 × 高度）
- [ ] shape 的 `type` 是否正确？（文本框用 `text`，装饰用 `rect`）
- [ ] XML 标签是否全部正确闭合？特殊字符（`&`、`<`、`>`）是否转义？

## 症状 → 修复表

| 看到的问题 | 改什么 |
|-----------|--------|
| 文字被截断/看不全 | 增大 shape 的 `width` 或 `height` |
| 元素重叠 | 调整 `topLeftX`/`topLeftY`，拉开间距 |
| 页面大面积空白 | 缩小元素间距，或增加内容填充 |
| 文字和背景色太接近 | 深色背景用浅色文字，浅色背景用深色文字 |
| 表格列宽不合理 | 调整 `colgroup` 中 `col` 的 `width` 值 |
| 图表没有显示 | 检查 `chartPlotArea` 和 `chartData` 是否都包含，`dim1`/`dim2` 数据数量是否匹配 |
| 图片被裁掉一部分 | `<img>` 的 `width`/`height` 是裁剪后尺寸，比例和原图不一致时会自动裁剪；要整图显示就让 `width:height` 对齐原图比例 |
| 只想改某页的单个元素（文字/图片/形状） | 用 `+replace-slide` 块级替换，不要整页重建 |
| 想给已有页加一张图（不动原有元素） | ① `+media-upload` 拿 `file_token` ② `+replace-slide` 用 `block_insert` 插入 `<img src="<file_token>" .../>`；不要再用 "整页 create + delete" 的老流程 |
| 新插入的 `<img>` 挡住/重叠原有元素 | `slide.get` 读原页，对照已有块的 `topLeftX/Y/width/height` 挑空白位置；空间不够就在同一批 `--parts` 里先 `block_replace` 缩小/挪动现有块再 `block_insert` 图片 |
| 渐变背景变成白色 | 渐变必须用 `rgba()` 格式 + 百分比停靠点，如 `linear-gradient(135deg,rgba(30,60,114,1) 0%,rgba(59,130,246,1) 100%)`；用 `rgb()` 或省略停靠点会被回退为白色 |
| 渐变方向不对 | 调整 `linear-gradient` 的角度（`90deg` 水平、`180deg` 垂直、`135deg` 对角线） |
| 整体风格不统一 | 封面页和结尾页用同一背景，内容页保持一致的配色和字号体系 |
| API 返回 400 | 检查 XML 语法：标签闭合、属性引号、特殊字符转义 |
| API 返回 3350001 | `block_replace` 根元素缺 `id=<block_id>` 或 `<shape>` 缺 `<content/>`，详见 replace-slide 文档 |
| 图片不显示 / `<img src>` 仍是 `@path` | `@` 占位符**只在 `+create --slides` 中替换**；直接调 `xml_presentation.slide.create` 必须先用 `+media-upload` 拿 `file_token` 写进 src |
| 上传图片报 1061002 params error | `parent_type` 必须是 `slide_file`（slides 唯一接受值）；不要手拼，用 `slides +media-upload` |

## 参考文档

| 文档 | 说明 |
|------|------|
| [lark-slides-create.md](references/lark-slides-create.md) | **+create Shortcut：创建 PPT（支持 `--slides` 一步添加页面，含 `@` 占位符自动上传图片）** |
| [lark-slides-compose.md](references/lark-slides-compose.md) | **+compose Shortcut：从 slide XML 目录或 manifest 创建 PPT，并自动处理本地图片** |
| [lark-slides-bulk-media-upload.md](references/lark-slides-bulk-media-upload.md) | **+bulk-media-upload Shortcut：批量上传图片并返回 `file_name -> file_token`** |
| [lark-slides-media-upload.md](references/lark-slides-media-upload.md) | **+media-upload Shortcut：上传本地图片，返回 `file_token`** |
| [lark-slides-replace-slide.md](references/lark-slides-replace-slide.md) | **+replace-slide Shortcut：块级替换/插入，含合法根元素速查与 3350001 排错** |
| [lark-slides-edit-workflows.md](references/lark-slides-edit-workflows.md) | 编辑已有页面的读-改-写流程与 action 决策树 |
| [template-index.json](references/template-index.json) | **机器可读模板索引：优先用来做模板路由和候选筛选** |
| [template-catalog.md](references/template-catalog.md) | **按场景/色调匹配现成 PPT 模板，并定位到页型范围** |
| [`scripts/template-tool.py`](scripts/template-tool.py) | **可选 Python 辅助脚本：`search` / `summarize` / `extract`，支持 `--layout-tag` 与 `extract --with-summary`** |
| [`scripts/layout-lint.py`](scripts/layout-lint.py) | **本地布局检查脚本：检测重叠、越界、页脚碰撞、文本高度风险** |
| [xml-schema-quick-ref.md](references/xml-schema-quick-ref.md) | **XML Schema 精简速查（必读）** |
| [slide-templates.md](references/slide-templates.md) | 可复制的 Slide XML 模板 |
| [xml-format-guide.md](references/xml-format-guide.md) | XML 详细结构与示例 |
| [examples.md](references/examples.md) | CLI 调用示例 |
| [slides_demo.xml](references/slides_demo.xml) | 真实 PPT 的完整 XML |
| [slides_xml_schema_definition.xml](references/slides_xml_schema_definition.xml) | **完整 Schema 定义**（唯一协议依据） |
| [lark-slides-xml-presentations-get.md](references/lark-slides-xml-presentations-get.md) | 读取 PPT 命令详情 |
| [lark-slides-xml-presentation-slide-create.md](references/lark-slides-xml-presentation-slide-create.md) | 添加幻灯片命令详情 |
| [lark-slides-xml-presentation-slide-delete.md](references/lark-slides-xml-presentation-slide-delete.md) | 删除幻灯片命令详情 |
| [lark-slides-xml-presentation-slide-get.md](references/lark-slides-xml-presentation-slide-get.md) | 读取单个幻灯片命令详情 |
| [lark-slides-xml-presentation-slide-replace.md](references/lark-slides-xml-presentation-slide-replace.md) | 原生 slide.replace API 命令详情 |

> **注意**：如果 md 内容与 `slides_xml_schema_definition.xml` 或 `lark-cli schema slides.<resource>.<method>` 输出不一致，以后两者为准。
