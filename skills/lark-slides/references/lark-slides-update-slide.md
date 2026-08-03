# slides +update-slide（按整页 XML 更新一页）

交一份完整的 `<slide>` XML，CLI 读回这一页当前的样子、和你给的做 diff，然后**只对有差异的元素**发替换 / 新增 / 删除。`slide_id` 和页序不变。

`slides +update` 是等价别名（不出现在 `--help` 里）。

## 为什么是 diff 而不是整页覆盖

底层端点 `xml_presentation.slide.replace` 的 part 里，`block_id` 被校验成**短元素 id（必须 `b` 开头）**。所以：

- 页面自己的 id 是 `p` 开头 → **不能**用一个 part 覆盖整页
- 背景 fill 的 id 是 `f` 开头 → **不能**改背景

这两条都实测确认过（各自返回 3350001，而同一页上 `b` 开头的元素级 part 成功）。元素 id 是这个端点唯一的抓手，所以整页语义只能由 CLI 在客户端拆成元素级操作来表达。

**这带来一个硬限制：背景改不了。** 见下方「改不了的东西」。

## 什么时候用它，什么时候用 +replace-slide

| 场景 | 用哪个 |
|------|--------|
| 改一个标题、换一张图、动一个形状 | [`+replace-slide`](lark-slides-replace-slide.md)，你已经知道要改哪个块，不需要 diff |
| 一页里多个元素都要改（批量换字体 / 换配色 / 重排版式） | `+update-slide`，交整页 XML，不用逐块枚举 parts 和手写元素 XML |
| 多页整页重建 | [`+replace-pages`](lark-slides-replace-pages.md) |
| 新增一页 | `xml_presentation.slide create` |
| 只改页面背景 | **本命令做不到**，见下 |

## 命令

```bash
# 典型用法：读-改-写（--content 走文件，避免 shell 转义和长度问题）
PID=slidesXXXXXXXXXXXXXXXXXXXXXX
SID=piy

# 1) 读回这一页
lark-cli slides +xml-get --as user \
  --presentation "$PID" --slide-id "$SID" --output .lark-slides/page.xml

# 2) 本地改（这里：把整页字体统一成思源黑体；-i.bak 写法 macOS / Linux 通用）
sed -i.bak 's/fontFamily="[^"]*"/fontFamily="思源黑体"/g' .lark-slides/page.xml && rm .lark-slides/page.xml.bak

# 3) 版式准出检查（改字体会改变文本度量，必须过这一关）
python3 skills/lark-slides/scripts/xml_text_overlap_lint.py .lark-slides/page.xml

# 4) 写回：CLI 自己再读一次做 diff，只发有差异的元素
lark-cli slides +update-slide --as user \
  --presentation "$PID" --slide-id "$SID" --content @.lark-slides/page.xml

# stdin 也可以
cat .lark-slides/page.xml | lark-cli slides +update-slide --as user \
  --presentation "$PID" --slide-id "$SID" --content -

# 预览（只显示会发哪两个请求；parts 取决于页面当前状态，dry-run 不读页面所以显示不了）
lark-cli slides +update-slide --as user \
  --presentation "$PID" --slide-id "$SID" --content @.lark-slides/page.xml --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--presentation` | 是 | `xml_presentation_id`、`/slides/<token>` URL，或 `/wiki/<token>` URL（wiki 自动解析） |
| `--slide-id` | 是 | 要更新的页面 ID |
| `--content` | 是 | 整页 XML，**单个 `<slide>` 根元素**；支持 `@<file>` 和 `-`（stdin） |
| `--revision-id` | 否 | 读取和应用所基于的版本；默认 `-1` = 最新。**不是乐观锁**，见下方「revision 不是乐观锁」 |
| `--tid` | 否 | 并发事务 ID；多人协作长事务才用，单次单人调用留空 |

`--content` 也接受 `--xml` / `--slide-xml` / `--slide-content` / `--content-xml`，`--presentation` 也接受 `--token` / `--url` 等；不出现在 `--help` 里但传了能识别。**`--slide` 不是别名**——太容易和 `--slide-id` 混淆，故意没收录。

## 语义：`--content` 是这一页的目标状态

| 你写的 | 结果 |
|---|---|
| 元素带原 `id`、内容有变 | 替换该元素（`replaced`）|
| 元素带原 `id`、内容没变 | 不动它，不产生 part |
| 元素**不带 `id`** | 当新元素插入到你写的位置（`inserted`）|
| 原有元素**没出现**在 `--content` 里 | 删除（`deleted`）|
| `<note>` 有变 | 替换备注（`note_replaced`）|
| `<note>` 没出现 | **清空备注**（`note_cleared`）|
| 完全没有差异 | 不发写请求，返回 `unchanged: true` |

比较是**规范化**的：服务端返回的 XML 是 pretty-print、属性顺序被重排、还会注入你没写的样式默认值。CLI 比较时会把属性排序、忽略**结构元素之间**的排版空白，所以这些都不算变化——**原样读回、原样写回是幂等的**（已实测）。而发出去的替换内容是**你的原始字节**，你的格式和属性顺序会保留到页面里。

注意 **`<p>` 段落内的文本按原样比较**（包括空白）：SML 里 `&#32;` 是"保留空格"，解码后和普通空格是同一个字符，宁可把一个语义等价的空白变化多发一次替换，也不能把真实的 `&#32;` 编辑误判成"没变化"。

## 改不了的东西（会报错，不会静默）

| 你想做的 | 结果 | 为什么 |
|---|---|---|
| 改页面背景 / `<style>` | **报错** | `<style>` 自身没有 id，`<fill>` 的 id 是 `f` 开头，端点只收 `b` 开头 |
| 调换现有元素的顺序 | **报错** | 没有 move 操作，元素级 part 表达不出来 |
| `--content` 里写一个页面上不存在的 `id` | **报错** | CLI 不会替你造 id；想新建就**别写 id** |
| 同一个 `id` 出现两次 | **报错** | — |
| 根元素不是 `<slide>` | **报错** | `--content` 描述整页，传元素级片段会被理解成"这一页只剩这个"，其余全删 |
| 根元素 `id` 与 `--slide-id` 不一致 | **报错** | 大概率是拿错了页的 XML（读的 A 页、写的 B 页）。确实要跨页套用内容，就把根 `id` 去掉 |
| `<slide>` 下出现 `style` / `data` / `note` 之外的子元素、或它们重复出现、或夹带文本 | **报错** | diff 表达不了这类结构；如果放过去，这部分改动会被静默丢弃，甚至误报 `unchanged` |
| 给 `<slide>` 或 `<data>` 加属性 | **报错** | 容器属性没有可承载它的元素级 part。仅有的例外：`<slide>` 上可以带 SML namespace——接受与 `sxsd_validator.py` 相同的三种写法：`http://www.larkoffice.com/sml/2.0`、`https://www.larkoffice.com/sml/2.0`、`/sml/2.0`（或不带）；其它 xmlns、前缀 `xmlns:x`、`<data>` 上的任何属性都会被拒绝 |
| 编辑**任何**含 `<undefined>` 占位符的页面 | **报错** | 占位符是服务端对"导不出来的对象"（画板、未导出的音视频）的替身。整页重写是否会保留一个没被触碰的占位符，是服务端行为，**没有可编程复现的端点测试能证明它**（画板无法程序化创建），所以本命令直接拒绝编辑这类页面，而不是在无法验证的假设上动手。该页要改，用 [`+replace-slide`](lark-slides-replace-slide.md) 做元素级编辑 |
| 一次传多页 | **报错** | 一页一次调用 |

背景确实要改的话，目前只能重建这一页（`slide create` + `slide delete`），或在客户端手动改。

## revision 不是乐观锁

**实测确认**：传一个已经过期的 `--revision-id` 服务端**不会拒绝**。它的含义是"在这个版本的快照上应用改动"，然后把结果提交为新版本——所以钉住旧版本会把**该版本之后对这一页的所有编辑全部丢弃**。

所以：

- **默认 `-1`（最新）就是推荐值**，别去钉住你读到的那个 revision。`-1` 下你的 parts 应用在最新快照上，你没碰的元素保持别人的最新状态。
- 只有在明确想"回到某个快照 + 我的改动"时才传具体版本号，并且清楚这会丢掉之后的编辑。
- 想避免和别人抢同一页，靠的是 `--tid` 事务或流程约定，不是 `--revision-id`。

## 返回值

```json
{
  "xml_presentation_id": "slidesXXXXXXXXXXXXXXXXXXXXXX",
  "slide_id": "piy",
  "parts_count": 3,
  "replaced": 2,
  "inserted": 1,
  "deleted": 0,
  "revision_id": 103
}
```

| 字段 | 说明 |
|------|------|
| `parts_count` | 本次发出的元素级操作条数；`0` 表示没有差异 |
| `replaced` / `inserted` / `deleted` | 分别替换、新增、删除了几个元素 |
| `note_replaced` / `note_cleared` | 仅在备注被改 / 被清空时出现且为 `true` |
| `unchanged` | 仅在完全没有差异时出现且为 `true`，此时没有发生写入 |
| `revision_id` | 写入成功后的新版本号 |
| `failed_reason` | 不会出现在成功返回里——批次被拒时整条命令失败 |

单次最多 200 个 part（服务端上限）。差异超过 200 个元素会在本地报错，让你拆开调用。

## 整页更新会连带影响的东西

下面这些是**这个端点**的行为，不是本命令引入的——[`+replace-slide`](lark-slides-replace-slide.md) 改一个元素也一样会触发（两者最终都走同一个整页 rewrite）：

| 影响 | 说明 |
|---|---|
| **组合被打散** | 页面上所有 group 会被解除组合，且无法恢复——`<group>` 在读写两侧都不可表达 |
| **主题挂载被改** | 页面原本挂在非空 master / layout 上时，会被重新挂到空白 layout，从此与主题脱钩；占位符会被清理 |
| **动画丢失** | 被删除或被重建的元素上的动画会一并删掉（保住 `id` 的元素不受影响）；`<smartLayout>` 每次都重建所以动画必丢。翻页转场不受影响 |
| **静态图表换数据源** | 可编辑图表保留原有数据源；静态图表会拿到新 token，旧数据被弃用 |
| **评论锚点** | `id` 匹配的元素上的评论保留；元素被删则锚点随之消失 |
| **含 `<undefined>` 占位符的页面整页拒绝** | 画板、未导出的音视频读回来是 `<undefined>` 占位符；整页重写是否保留它无法用可复现的测试证明，所以本命令直接拒绝编辑这类页面（见上表）。[`+replace-slide`](lark-slides-replace-slide.md) 仍可对该页做元素级编辑 |

最后两条再次指向同一条建议：**以 `+xml-get` 的输出为基准做最小改动**，别手写整页。

## 常见错误

| 现象 | 原因 | 对策 |
|------|------|------|
| `--content changes <style> (the page background)` | 改了背景 | 把 `+xml-get` 读回的 `<style>` 原样保留；确实要改背景只能重建该页 |
| `--content reorders existing elements` | 调换了现有元素顺序 | 保持原顺序；要挪位置就删掉再以新元素插入 |
| `element id "bZZ" ... does not exist` | 写了页面上不存在的 id | 想新建元素就**不要写 id**；或重新 `+xml-get` 确认 id |
| `--content root element is <shape>` | 传了元素级片段 | 单个元素改动用 [`+replace-slide`](lark-slides-replace-slide.md)；整页更新要补全 `<slide>` 外层 |
| `--content root carries id "pold" but --slide-id is "pnew"` | 拿 A 页的 XML 写 B 页 | 重新对目标页 `+xml-get`；确实要跨页套用就去掉根 `id` |
| `--content contains an unknown <foo> element` / `a second <data>` | `<slide>` 下有 diff 表达不了的结构 | 一页只有一个 `<style>`、一个 `<data>`、一个 `<note>`；把多余结构去掉 |
| `slide piy contains an <undefined> placeholder` | 这一页上有画板或未导出的媒体对象 | 本命令拒绝编辑该页；用 [`+replace-slide`](lark-slides-replace-slide.md) 做元素级编辑 |
| `an unsupported xmlns "…" on <slide>` | xmlns 写错或用了前缀声明 | 根元素接受 `sxsd_validator.py` 认可的三种 SML namespace（可不带）；`<data>` 不收任何属性 |
| `slide piy contains ... which this command cannot represent` | **当前页**（不是你的输入）带有本命令无法处理的结构 | 这一页改用 [`+replace-slide`](lark-slides-replace-slide.md) 做元素级编辑 |
| `--content is not well-formed XML` | 括号没闭合、引号没配对、实体没转义 | 报错里带解析位置 |
| 返回 `unchanged: true` 但你以为改了 | 你的改动被规范化比较判定为无差异（例如只动了缩进或属性顺序） | 检查是不是真的改了内容 |
| 3350001 | 元素 XML 结构不合法，或嵌套 `<shape>` 缺 `<content/>` | 对照 [`xml-schema-quick-ref.md`](xml-schema-quick-ref.md)。注意 `+replace-slide` 会自动补 `<content/>`，本命令不会——元素 XML 原样发出 |
| 大页面偶发失败，报错与排队 / 超时相关 | 服务端处理背压，不是尺寸超限 | 先重试；持续出现说明该页确实偏大，拆成几次 `+replace-slide` |
| 403 | 权限不足 | 需要 `slides:presentation:update` 或 `write_only`，**以及 `slides:presentation:read`**（要先读页面）；wiki URL 还需要 `wiki:node:read` |

## 相关命令

- [+xml-get](lark-slides-xml-presentations-get.md) — 读回整页 XML，本命令的输入来源
- [+replace-slide](lark-slides-replace-slide.md) — 元素级替换 / 插入，已知目标块时用它
- [+replace-pages](lark-slides-replace-pages.md) — 多页整页重建
- [lark-slides-edit-workflows.md](lark-slides-edit-workflows.md) — 读-改-写闭环 + 决策树
