---
name: lark-slides
version: 1.0.0
description: "飞书幻灯片：以 XML 格式读取和管理 PPT 页面。创建演示文稿优先用 `+create`；XML API 主要用于读取 PPT 全文信息、创建和删除幻灯片页面。当用户需要创建 PPT、读取 PPT 内容、管理幻灯片页面时使用。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli slides --help"
---

# slides (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

**CRITICAL — 生成任何 XML 之前，MUST 先用 Read 工具读取 [xml-schema-quick-ref.md](references/xml-schema-quick-ref.md)，禁止凭记忆猜测 XML 结构。**

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
3. 只有在用户明确要求“用应用身份 / bot 身份操作”，或当前工作流就是 bot 创建资源后再做协作授权时，才切换到 `--as bot`。

## 快速开始

一条命令创建包含页面内容的 PPT（推荐）：

```bash
lark-cli slides +create --title "演示文稿标题" --slides '[
  "<slide xmlns=\"http://www.larkoffice.com/sml/2.0\"><style><fill><fillColor color=\"rgb(245,245,245)\"/></fill></style><data><shape type=\"text\" topLeftX=\"80\" topLeftY=\"80\" width=\"800\" height=\"100\"><content textType=\"title\"><p>页面标题</p></content></shape><shape type=\"text\" topLeftX=\"80\" topLeftY=\"200\" width=\"800\" height=\"200\"><content textType=\"body\"><p>正文内容</p><ul><li><p>要点一</p></li><li><p>要点二</p></li></ul></content></shape></data></slide>"
]'
```

也可以分两步（先创建空白 PPT，再逐页添加），详见 [+create 参考文档](references/lark-slides-create.md)。

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
| 需要 CLI 调用示例 | [examples.md](references/examples.md) |
| 需要参考真实 PPT 的 XML | [slides_demo.xml](references/slides_demo.xml) |
| 需要用 table/chart 等复杂元素 | [slides_xml_schema_definition.xml](references/slides_xml_schema_definition.xml)（完整 Schema） |
| 需要了解某个命令的详细参数 | 对应命令的 reference 文档（见下方参考文档章节） |

## Workflow

> **这是演示文稿，不是文档。** 每页 slide 是独立的视觉画面，信息密度要低，排版要留白。

```text
Step 1: 需求澄清 & 读取知识
  - 澄清用户需求：主题、受众、页数、风格偏好
  - 如果用户没有明确风格，根据主题推荐（见下方风格判断表）
  - 读取 XML Schema 参考：
    · xml-schema-quick-ref.md — 元素和属性速查
    · xml-format-guide.md — 详细结构与示例
    · slides_demo.xml — 真实 XML 示例

Step 2: 生成大纲 → 用户确认 → 创建
  - 生成结构化大纲（每页标题 + 要点 + 布局描述），交给用户确认
  - 10 页以内：用 slides +create --slides '[...]' 一步创建 PPT 并添加所有页面
  - 超过 10 页：先 `slides +create` 创建空白 PPT，再用 `xml_presentation.slide.create` 逐页添加
  - 含本地图片：
    · 新建带图 PPT —— 在 slide XML 里写 <img src="@./pic.png" .../>，
      +create 会自动上传并替换为 file_token（详见 lark-slides-create.md）
    · 给已有 PPT 加带图新页 —— 先 `slides +media-upload --file ./pic.png --presentation $PID`
      拿到 file_token，再用它写进 slide XML 调 xml_presentation.slide.create
    · 给已有页加图 —— XML API 无元素级编辑，需要整页替换；必守规则和流程见下方「给已有 PPT 的已有页加图」章节
    · 路径必须是 CWD 内的相对路径（如 ./pic.png 或 ./assets/x.png）；
      绝对路径会被 CLI 拒绝，先 cd 到素材所在目录再执行
  - 每页 slide 需要完整的 XML：背景、文本、图形、配色
  - 复杂元素（table、chart）需参考 XSD 原文

Step 3: 审查 & 交付
  - 创建完成后，用 xml_presentations.get 读取全文 XML，确认：
    · 页数是否正确？每页内容是否完整？
    · 配色是否统一？字号层级是否合理？
  - 有问题 → 用 xml_presentation.slide.delete 删除问题页，重新创建
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

### 给已有 PPT 的已有页加图（整页替换）

XML API 没有元素级编辑接口（见核心规则 7）。想给某一页加图，只能**整页替换**：读原 slide → 加 `<img>` → 原位 create 新页 → 删除旧页。

**必守 4 条**：

1. **先 create 后 delete** —— 顺序反了且 create 失败会丢页
2. **原 slide 的所有元素必须原样搬到新 XML**（标题、正文、形状、原有 img）—— 只写新 `<img>` 会把原页其他内容全删掉
3. **`before_slide_id=<旧 slide_id>` 必传，且必须放在 `--data` body 里**（与 `slide` 同级），**不能放在 `--params`** —— `--params` 只接 path/query 参数，body 字段塞进去会被 CLI 当未知 query 参数静默下发，服务端忽略，结果是新页追加到末尾、打乱页序。正确形状：`--data '{"slide":{"content":"..."},"before_slide_id":"<旧 slide_id>"}'`
4. **新 `<img>` 坐标避开现有元素** —— 读原 `<data>` 里元素的 `topLeftX/Y/width/height` 挑空白区；空间不够就先缩小/挪动现有元素再放图

完整 bash 模板和 `+media-upload` 参数见 [+media-upload 文档](references/lark-slides-media-upload.md)。

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

页面结构（N 页）：
1. 封面页：[标题文案]
2. [页面主题]：[要点1]、[要点2]、[要点3]
3. [页面主题]：[要点描述]
...
N. 结尾页：[结尾文案]

风格：[配色方案]，[排版风格]
```

### 常用 Slide XML 模板

可直接复制使用的模板（封面页、内容页、数据卡片页、结尾页）：[slide-templates.md](references/slide-templates.md)

---

## 核心概念

### URL 格式与 Token

| URL 格式 | 示例 | Token 类型 | 处理方式 |
|----------|------|-----------|----------|
| `/slides/` | `https://example.larkoffice.com/slides/xxxxxxxxxxxxx` | `xml_presentation_id` | URL 路径中的 token 直接作为 `xml_presentation_id` 使用 |
| `/wiki/` | `https://example.larkoffice.com/wiki/wikcnxxxxxxxxx` | `wiki_token` | ⚠️ **不能直接使用**，需要先查询获取真实的 `obj_token` |

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
| [`+create`](references/lark-slides-create.md) | 创建 PPT（可选 `--slides` 一步添加页面，支持 `<img src="@./local.png">` 占位符自动上传），bot 模式自动授权 |
| [`+media-upload`](references/lark-slides-media-upload.md) | 上传本地图片到指定演示文稿，返回 `file_token`（用作 `<img src="...">`），最大 20 MB |

## API Resources

```bash
lark-cli schema slides.<resource>.<method>    # 调用 API 前必须先查看参数结构
lark-cli slides <resource> <method> [flags]  # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `--data` / `--params` 参数结构，不要猜测字段格式。

### xml_presentations

  - `get` — 读取ppt全文信息，xml格式返回

### xml_presentation.slide

  - `create` — 在指定 xml 演示文稿下创建页面
  - `delete` — 删除指定 xml 演示文稿下的页面

## 核心规则

1. **先出大纲再动手**：创建 PPT 前先生成大纲交给用户确认，避免返工
2. **创建流程**：10 页以内推荐 `slides +create --slides '[...]'` 一步创建；超过 10 页先 `slides +create` 创建空白 PPT，再用 `xml_presentation.slide.create` 逐页添加
3. **`<slide>` 直接子元素只有 `<style>`、`<data>`、`<note>`**：文本和图形必须放在 `<data>` 内
4. **文本通过 `<content>` 表达**：必须用 `<content><p>...</p></content>`，不能把文字直接写在 shape 内
5. **保存关键 ID**：后续操作需要 `xml_presentation_id`、`slide_id`、`revision_id`
6. **删除谨慎**：删除操作不可逆，且至少保留一页幻灯片
7. **没有元素级编辑能力**：飞书 slides XML API 只有 slide 级 `create` / `delete`，**没有更新单个 shape/img 坐标或尺寸的接口**。不要向用户承诺"微调坐标/尺寸"、"挪一下这个图"。要改只能整页重建（`xml_presentations.get` 读回 → 改 XML → `slide.delete` 旧页 + `slide.create` 新页），且 `slide_id` 会变、默认追加到末尾（要回原位需 `before_slide_id`）。整页重建前必须先告知用户代价并确认
8. **`<img src>` 只能用上传到飞书 drive 的 `file_token`，禁止使用 http(s) 外链 URL**：飞书 slides 渲染端不会代理外链图片，外链 src 在 PPT 里通常不显示或显示破图。流程必须是「先把图存到本地 → 用 `slides +media-upload` 上传或 `+create --slides` 的 `@./path` 占位符自动上传 → 拿 `file_token` 写进 `<img src>`」。如果用户给了网图链接，先 `curl`/下载到 CWD 内再走上传流程，不要直接把外链 URL 塞进 `src`。**图片最大 20 MB**（slides upload API 不支持分片上传）。

## 权限表

| 方法 | 所需 scope |
|------|-----------|
| `slides +create` | `slides:presentation:create`, `slides:presentation:write_only`（含 `@` 占位符时还需 `docs:document.media:upload`） |
| `slides +media-upload` | `docs:document.media:upload`（wiki URL 解析还需 `wiki:node:read`） |
| `xml_presentations.get` | `slides:presentation:read` |
| `xml_presentation.slide.create` | `slides:presentation:update` 或 `slides:presentation:write_only` |
| `xml_presentation.slide.delete` | `slides:presentation:update` 或 `slides:presentation:write_only` |

## 常见错误速查

| 错误码 | 含义 | 解决方案 |
|--------|------|----------|
| 400 | XML 格式错误 | 检查 XML 语法，确保标签闭合 |
| 400 | create 内容超出支持范围 | `xml_presentations.create` 仅用于创建空白 PPT，不要在这里传完整 slide 内容 |
| 400 | 请求包装错误 | 检查 `--data` 是否按 schema 传入 `xml_presentation.content` 或 `slide.content` |
| 404 | 演示文稿不存在 | 检查 `xml_presentation_id` 是否正确 |
| 404 | 幻灯片不存在 | 检查 `slide_id` 是否正确 |
| 403 | 权限不足 | 检查是否拥有对应的 scope |
| 400 | 无法删除唯一幻灯片 | 演示文稿至少保留一页幻灯片 |
| 1061002 | params error（媒体上传时） | 用 `slides +media-upload`，不要手拼原生 `medias/upload_all`；slides 唯一可用 `parent_type` 是 `slide_file` |
| 1061004 | forbidden：当前身份对演示文稿无编辑权限 | 确认 user/bot 对目标 PPT 有编辑权限；bot 常见于 PPT 非该 bot 创建，需先授权或用 `+create --as bot` 新建 |
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
| 给已有页加图后，原页文字/形状消失了 | 替换整页时必须把原 slide 的 `<data>` 所有元素原样搬过来，不能只写新 `<img>` |
| 渐变背景变成白色 | 渐变必须用 `rgba()` 格式 + 百分比停靠点，如 `linear-gradient(135deg,rgba(30,60,114,1) 0%,rgba(59,130,246,1) 100%)`；用 `rgb()` 或省略停靠点会被回退为白色 |
| 渐变方向不对 | 调整 `linear-gradient` 的角度（`90deg` 水平、`180deg` 垂直、`135deg` 对角线） |
| 整体风格不统一 | 封面页和结尾页用同一背景，内容页保持一致的配色和字号体系 |
| API 返回 400 | 检查 XML 语法：标签闭合、属性引号、特殊字符转义 |
| API 返回 3350001 | `xml_presentation_id` 不是通过 `xml_presentations.create` 创建的，或 token 不正确 |
| 图片不显示 / `<img src>` 仍是 `@path` | `@` 占位符**只在 `+create --slides` 中替换**；直接调 `xml_presentation.slide.create` 必须先用 `+media-upload` 拿 `file_token` 写进 src |
| 上传图片报 1061002 params error | `parent_type` 必须是 `slide_file`（slides 唯一接受值）；不要手拼，用 `slides +media-upload` |

## 参考文档

| 文档 | 说明 |
|------|------|
| [lark-slides-create.md](references/lark-slides-create.md) | **+create Shortcut：创建 PPT（支持 `--slides` 一步添加页面，含 `@` 占位符自动上传图片）** |
| [lark-slides-media-upload.md](references/lark-slides-media-upload.md) | **+media-upload Shortcut：上传本地图片，返回 `file_token`** |
| [xml-schema-quick-ref.md](references/xml-schema-quick-ref.md) | **XML Schema 精简速查（必读）** |
| [slide-templates.md](references/slide-templates.md) | 可复制的 Slide XML 模板 |
| [xml-format-guide.md](references/xml-format-guide.md) | XML 详细结构与示例 |
| [examples.md](references/examples.md) | CLI 调用示例 |
| [slides_demo.xml](references/slides_demo.xml) | 真实 PPT 的完整 XML |
| [slides_xml_schema_definition.xml](references/slides_xml_schema_definition.xml) | **完整 Schema 定义**（唯一协议依据） |
| [lark-slides-xml-presentations-create.md](references/lark-slides-xml-presentations-create.md) | 创建空白 PPT 命令详情 |
| [lark-slides-xml-presentations-get.md](references/lark-slides-xml-presentations-get.md) | 读取 PPT 命令详情 |
| [lark-slides-xml-presentation-slide-create.md](references/lark-slides-xml-presentation-slide-create.md) | 添加幻灯片命令详情 |
| [lark-slides-xml-presentation-slide-delete.md](references/lark-slides-xml-presentation-slide-delete.md) | 删除幻灯片命令详情 |

> **注意**：如果 md 内容与 `slides_xml_schema_definition.xml` 或 `lark-cli schema slides.<resource>.<method>` 输出不一致，以后两者为准。
