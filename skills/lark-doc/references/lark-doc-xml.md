# 飞书 XML 语法

**语法采用类 HTML 标签，渲染采用纵向块级文档流：顶层 Block 按文档顺序纵向排列，块内支持富文本和子块嵌套。默认宽度约 820 px，宽版模式约 1020 px**

以下为 XML 语法示例，使用时需替换其中的示例值。属性必须写成 `name="value"`，禁止省略引号。

## 常用标签

- `p, h1-h9, blockquote, hr, img, b, em, u, del, br, span` 语义不变。普通文档建议只使用 `h1-h6`，`h7-h9` 仅在确需更深层级时使用。
- `<a type="url-preview" href="URL">链接标题</a>`
- `<latex>E = mc^2</latex>`：适用行内公式，也适用于上标、下标写法。
- `<ol><li>第一项<ul><li>子项</li></ul></li><li>第二项</li></ol>`：子列表放在 `<li>` 内；新增列表项必须放在 `<ul>` 或 `<ol>` 内。
- `<pre lang="go" caption="示例"><code>fmt.Println(&quot;hello&quot;)</code></pre>`：代码必须放在 `<code>` 内，禁止直接放在 `<pre>` 下；`caption` 可省略。
- `<img path="@./photo.png"/>`：上传当前工作目录内的本地图片。也可用 `<img href="URL"/>` 上传公开 HTTP(S) 网络图片，或用 `<img src="token"/>` 复制原始图片；三者任选一个，可选 `width`、`height`、`caption`、`name`。使用 `href` 时，CLI 会将远程图片转为本地资源并完成上传；响应须为 PNG、JPEG、GIF 或 WebP，单图不超过 20MiB。内部网络图片须先下载到本地再使用 `path`。
- `<source path="@./report.pdf" name="报告.pdf"/>`：上传本地附件；也可使用 `<source token="token" name="xx"/>` 复制已有附件。可独立使用、放入 `<p>` 作为行内附件，或写成 `<figure view-type="Card|Preview"><source/></figure>`；
- `<checkbox done="true|false">todo</checkbox>`
- `p, h1-h9, li, checkbox, title` 支持可选属性 `align`，可选值为 `left`、`center`、`right`，例如 `<p align="center">居中正文</p>`。

## 标题与列表编号

- 完整文档以唯一的 `<title>` 开头；正文标题使用 `<h1>` 至 `<h9>`，层级须连续，不跳级，例如 `<h1>` 后不能直接使用 `<h3>`，应先出现 `<h2>`。需要自动编号时设置 `seq="auto"`，系统会按标题层级生成并递增阿拉伯数字编号，例如一级标题为 `1`，二级标题为 `1.1`。
- 有序列表：默认属性 `seq="auto"`，需从指定数字开始时设置对应值，如 `seq="3"`。

## 表格

- `<table><thead><tr><th><p>表头</p></th></tr></thead><tbody><tr><td><p>内容</p></td></tr></tbody></table>`
- `<colgroup><col /></colgroup>` 紧跟 `<table>` 定义列宽；`width` 表示列宽，可选 `span` 表示连续作用的列数。
- `<th>` / `<td>` 支持 `background-color`、`vertical-align`、`colspan`、`rowspan`；`vertical-align`：`top | middle | bottom`；`background-color` 支持基础色相、`light-{色相}`、`medium-gray`，表头优先使用 `light-gray` 或 `medium-gray`，彩色单元格仅用于表达状态或分类。被合并的单元格不再写入。

## 扩展标签

- `<cite type="user" user-id="ou_xxx"/>`：@人，会渲染为用户头像；必须显式传入用户 `open_id`，不得用纯文本名字冒充 @人。
- `<cite type="doc" doc-id="DOC_TOKEN"/>`：@文档，会渲染为文档标题。
- `<cite type="citation"><a href="URL" url-type="N"></a></cite>`：参考文献容器，仅含多个 `<a>`。`url-type` 标识链接类型：`5`（WebURL）须在`<a></a>`中填写渲染标题；`1`（Docx）、`6`（Minutes）、`12`（Base）、`13`（Sheet）可留空。
- `<whiteboard></whiteboard>`：`type | src` 二选一。`type=blank` 为新建；`type=mermaid|plantuml|svg` 时，支持 `path=@./file` 导入，也支持在标签内直接写入内容；`src=token` 表示复制已有画板。复杂图需读取 [`lark-doc-whiteboard.md`](lark-doc-whiteboard.md)；
- `<grid><column width-ratio="0.5"><p>左栏</p></column><column width-ratio="0.5"><p>右栏</p></column></grid>`：各列 `width-ratio` 之和为 1。
- `<callout emoji="💡" background-color="light-*" border-color="*"><p>高亮块内容</p></callout>`：子块仅支持 `p`、`ol`、`ul`、`checkbox`、行内标签；禁止 `<table>`、`<img>`、`<pre>`、`<hr>`、`<grid>`、`<whiteboard>`、等其他块级标签或资源块。可选 `text-color`。
- 其他扩展标签 `html5-block`、`bookmark`、`button`、`time`、`sheet`、`task`、`chat_card`、`sub-page-list`、`okr` 见 [`lark-doc-xml-extended-blocks.md`](lark-doc-xml-extended-blocks.md)。

## 颜色

颜色用于表达语义，并在全文保持一致；默认保持中性色排版，避免仅为装饰而着色。

- **合法值**：色相为 `red, orange, yellow, green, blue, purple, gray`；`text-color`、`border-color` 使用基础色相；`<span>`、`<th>`、`<td>`、`<button>` 背景支持基础色相、`light-{色相}`、`medium-gray`；高亮块背景支持 `gray`、`light-{色相}`、`medium-{色相}`。
- **高亮块**：默认使用 `light-*` 背景和默认文字色；强提醒才使用 `medium-*`，彩色文字只强调短语。
- **表格**：表头优先使用 `light-gray` 或 `medium-gray`；彩色单元格只表达状态或分类，避免整表铺色。

## 转义规则

禁止转义标签本身；只转义标签内部的文本内容。

- 文本转义：`<` → `&lt;`，`>` → `&gt;`，`&` → `&amp;`，换行符 `\n` → `<br/>`。
- 错误：`&lt;p&gt;内容&lt;/p&gt;`
- 正确：`<p>A &amp; B 的对比：1 &lt; 2</p>`
