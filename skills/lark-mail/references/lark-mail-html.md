# 邮件 HTML 写法指南

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解通用安全规则。本文档定义 lark-cli mail 写信场景下的 HTML / CSS / URL 写法、LarkSuite mail-editor 原生格式、可复制片段、3 套场景模板。

**CRITICAL 邮件是重要的对外交流渠道，请你保证书写语言凝练扼要**
**CRITICAL 电子邮件的 HTML 不是 Web 开发的 HTML，请你务必遵守本文档中提及的常用邮件格式书写规范**
**CRITICAL 请务必使用 shortcut 来进行邮件内容编辑 （`+send` / `+draft-create` / `+reply` / `+reply-all` / `+forward`）或 `+draft-edit` 的 body op，严禁自行拼接 EML**

你可以参考 **官方模板库** [`../assets/templates/`](../assets/templates) — 提供部分场景模板，可供参考

> 请注意，邮件内容编辑相关的 shortcut 内置 HTML lint 工具，处于安全考虑和格式适配，你输入的 HTML 可能会被自动调整

## 风格底线

- **邮件标题小于50字**： 邮件主题行 `--subject` 应控制在 50 字内，避免超长标题带来理解困难
- **多用列表、表格**：不要堆叠过长的文本段落，请擅长使用列表`<ul>` / `<ol>`或分段 `<p>` 
- **列表书写规则**：**不要**用 `<p>一、...</p><p>二、...</p>` 这种「中文编号 + 段落」的列表样式，"①②③"、"1) 2) 3)的机械写法也请摒弃；请擅长使用列表格式 `<ul>` / `<ol>`。
- **正文长度自适应**：不限制正文长度，但要求**首屏要见到关键信息**。

## 格式书写规范

电子邮件的 HTML 受客户端兼容性与安全沙箱约束，跟 Web 浏览器 HTML 不是同一规范体系。下面是飞书邮箱已验证的最纯净、最美观写法，请直接复制使用。

### 段落

```html
<p>文字</p>
```

### 标题

```html
<h1>一级标题（26px，自动加粗）</h1>
<h2>二级标题（22px）</h2>
<h3>三级标题（20px）</h3>
<h4>四级标题（18px）</h4>
```

### 加粗

```html
<b>加粗文字</b>
```

### 斜体

```html
<i>斜体文字</i>
```

### 下划线

```html
<u>下划线文字</u>
```

### 删除线

```html
<s>删除文字</s>
```

### 字号

```html
<span style="font-size:18px">放大到 18px</span>
```

### 字体

```html
<span style="font-family:'Courier New',monospace">等宽字体</span>
```

### 文字颜色

```html
<span style="color:rgb(245,74,69)">红色文字</span>
```

### 换行

```html
第一行<br>第二行
```

### 分隔

```html
<hr>
```

### 列表

```html
<!-- 无序列表 -->
<ul><li>项</li></ul>

<!-- 有序列表 -->
<ol><li>条</li></ol>

<!-- 多级列表通用规则（适用于下面两个示例）：
     - <ul>/<ol> 的直接子节点必须是 <li>，HTML 规范不允许 <ul> 直接套 <ul>
     - 子列表必须嵌套在父 <li> 内，不要拆成多个独立 ol/ul 兄弟
     - 每级 list-style-type 用不同符号区分层级（disc/circle/square 或 decimal/lower-alpha/lower-roman）
     - 子级用 margin-left:24px 视觉缩进 -->

<!-- 多级有序列表（全 ol 三级嵌套：decimal → lower-alpha → lower-roman） -->
<ol data-list-number="true" style="margin:0px;padding-left:0px;list-style-position:inside">
   <li class="temp-li number1" data-li-line="true" data-list="number1" data-ol-id="demo-ol" style="line-height:1.6;margin:4px 0;padding-left:0px;display:list-item;list-style-type:decimal;font-family:inherit;font-size:14px;list-style-position:inside" dir="auto">
      <b><span style="font-family:inherit"><span style="color:rgb(31,35,41)">第一级（decimal）</span></span></b>
      <ol data-list-number="true" style="margin:0px 0px 0px 24px;padding-left:0px;list-style-position:inside">
         <li class="temp-li number2" data-li-line="true" data-list="number2" data-ol-id="demo-ol" style="line-height:1.6;margin:4px 0;padding-left:0px;display:list-item;list-style-type:lower-alpha;font-family:inherit;font-size:14px;list-style-position:inside" dir="auto">
            <span style="font-family:inherit"><span style="color:rgb(31,35,41)">第二级（lower-alpha，缩进 24px）</span></span>
            <ol data-list-number="true" style="margin:0px 0px 0px 24px;padding-left:0px;list-style-position:inside">
               <li class="temp-li number3" data-li-line="true" data-list="number3" data-ol-id="demo-ol" style="line-height:1.6;margin:4px 0;padding-left:0px;display:list-item;list-style-type:lower-roman;font-family:inherit;font-size:14px;list-style-position:inside" dir="auto">
                  <span style="font-family:inherit"><span style="color:rgb(31,35,41)">第三级（lower-roman，再缩进 24px）</span></span>
               </li>
            </ol>
         </li>
         <li class="temp-li number2" data-li-line="true" data-list="number2" data-ol-id="demo-ol" style="line-height:1.6;margin:4px 0;padding-left:0px;display:list-item;list-style-type:lower-alpha;font-family:inherit;font-size:14px;list-style-position:inside" dir="auto">
            <span style="font-family:inherit"><span style="color:rgb(31,35,41)">第二级（同层）</span></span>
         </li>
      </ol>
   </li>
   <li class="temp-li number1" data-li-line="true" data-list="number1" data-ol-id="demo-ol" style="line-height:1.6;margin:4px 0;padding-left:0px;display:list-item;list-style-type:decimal;font-family:inherit;font-size:14px;list-style-position:inside" dir="auto">
      <b><span style="font-family:inherit"><span style="color:rgb(31,35,41)">第一级（接续编号）</span></span></b>
   </li>
</ol>

<!-- 多级无序列表（全 ul 三级嵌套：disc → circle → square） -->
<ul data-list-bullet="true" style="margin:0px;padding-left:0px;list-style-position:inside">
   <li class="temp-li bullet1" data-li-line="true" data-list="bullet1" style="line-height:1.6;margin:4px 0;padding-left:0px;display:list-item;list-style-type:disc;font-family:inherit;font-size:14px;list-style-position:inside" dir="auto">
      <span style="font-family:inherit"><span style="color:rgb(31,35,41)">第一级（disc）</span></span>
      <ul data-list-bullet="true" style="margin:0px 0px 0px 24px;padding-left:0px;list-style-position:inside">
         <li class="temp-li bullet2" data-li-line="true" data-list="bullet2" style="line-height:1.6;margin:4px 0;padding-left:0px;display:list-item;list-style-type:circle;font-family:inherit;font-size:14px;list-style-position:inside" dir="auto">
            <span style="font-family:inherit"><span style="color:rgb(31,35,41)">第二级（circle，缩进 24px）</span></span>
            <ul data-list-bullet="true" style="margin:0px 0px 0px 24px;padding-left:0px;list-style-position:inside">
               <li class="temp-li bullet3" data-li-line="true" data-list="bullet3" style="line-height:1.6;margin:4px 0;padding-left:0px;display:list-item;list-style-type:square;font-family:inherit;font-size:14px;list-style-position:inside" dir="auto">
                  <span style="font-family:inherit"><span style="color:rgb(31,35,41)">第三级（square，再缩进 24px）</span></span>
               </li>
            </ul>
         </li>
         <li class="temp-li bullet2" data-li-line="true" data-list="bullet2" style="line-height:1.6;margin:4px 0;padding-left:0px;display:list-item;list-style-type:circle;font-family:inherit;font-size:14px;list-style-position:inside" dir="auto">
            <span style="font-family:inherit"><span style="color:rgb(31,35,41)">第二级（同层）</span></span>
         </li>
      </ul>
   </li>
   <li class="temp-li bullet1" data-li-line="true" data-list="bullet1" style="line-height:1.6;margin:4px 0;padding-left:0px;display:list-item;list-style-type:disc;font-family:inherit;font-size:14px;list-style-position:inside" dir="auto">
      <span style="font-family:inherit"><span style="color:rgb(31,35,41)">第一级（同层）</span></span>
   </li>
</ul>
```

### 表格

```html
  <table style="border-collapse:collapse">
    <thead>
      <tr style="background-color:rgb(242,243,245)">
        <th rowspan="2" style="border:1px solid rgb(222,224,227);padding:8px;vertical-align:middle">A</th>
        <th colspan="2" style="border:1px solid rgb(222,224,227);padding:8px;text-align:center">B</th>
        <th rowspan="2" style="border:1px solid rgb(222,224,227);padding:8px;vertical-align:middle">C</th>
      </tr>
      <tr style="background-color:rgb(242,243,245)">
        <th style="border:1px solid rgb(222,224,227);padding:8px">B1</th>
        <th style="border:1px solid rgb(222,224,227);padding:8px">B2</th>
      </tr>
    </thead>
    <tbody>
      <tr>
        <td style="border:1px solid rgb(222,224,227);padding:8px">a1</td>
        <td style="border:1px solid rgb(222,224,227);padding:8px">b1-1</td>
        <td style="border:1px solid rgb(222,224,227);padding:8px">b2-1</td>
        <td style="border:1px solid rgb(222,224,227);padding:8px">c1</td>
      </tr>
      <tr>
        <td style="border:1px solid rgb(222,224,227);padding:8px">a2</td>
        <td style="border:1px solid rgb(222,224,227);padding:8px">b1-2</td>
        <td style="border:1px solid rgb(222,224,227);padding:8px">b2-2</td>
        <td style="border:1px solid rgb(222,224,227);padding:8px">c2</td>
      </tr>
    </tbody>
  </table>
```

### 链接

```html
<a href="https://www.larkoffice.com" style="color:rgb(20,86,240);text-decoration:none">链接文字</a>
```

### AT 用户

```html
<a id="at-user-1" href="mailto:user@example.com" style="cursor:pointer;color:rgb(20,86,240);padding:2px;text-decoration:none;border-radius:999em;margin:0px 2px">@姓名</a>
```

**必填字段** `id="at-user-N"`、`mailto:` 和姓名文本

### 引用

```html
<blockquote style="padding-left:12px;color:rgb(100,106,115);border-left:2px solid rgb(187,191,196);margin:0px">引用文字</blockquote>
```

### 文字高亮（荧光笔风格）

```html
<span style="background-color:rgb(255,200,220);color:rgb(31,35,41)">关键里程碑</span>
<span style="background-color:rgb(255,225,140);color:rgb(31,35,41)">待跟进</span>
<span style="background-color:rgb(190,230,200);color:rgb(31,35,41)">已完成</span>
```

### 文字强调

```html
<b><span style="font-family:inherit"><span style="color:rgb(245,74,69)">红色加粗</span></span></b>
<i><span style="font-family:inherit"><span style="color:rgb(0,0,0)">斜体</span></span></i>
<u><span style="font-family:inherit"><span style="color:rgb(0,0,0)">下划线</span></span></u>
<s><span style="font-family:inherit"><span style="color:rgb(0,0,0)">删除线</span></span></s>
```

### 居中 / 左对齐 / 右对齐

```html
<div style="text-align:center">居中</div>
<div style="text-align:left">左对齐（默认）</div>
<div style="text-align:right">右对齐</div>
```

### 盒模型

```html
<div style="margin:8px;padding:12px;width:300px">外边距 8px / 内边距 12px / 宽度 300px</div>
```

### 边框

```html
<div style="border:1px solid rgb(222,224,227);border-radius:8px;padding:8px">圆角描边</div>
```

### 透明

```html
<span style="opacity:0.5">半透明文字</span>
```

### 颜色（推荐调色盘）

```html
<!-- 主黑（正文） -->
<span style="color:rgb(31,35,41)">主文本</span>
<!-- 副灰（次要说明 / 时间 / 备注） -->
<span style="color:rgb(100,106,115)">副文本</span>
<!-- 浅灰（三级文本 / 占位） -->
<span style="color:rgb(143,149,158)">浅灰文本</span>
<!-- LarkSuite 蓝（链接 / mention） -->
<span style="color:rgb(20,86,240)">蓝色文字</span>
<!-- LarkSuite 深蓝（重点标题） -->
<span style="color:rgb(36,91,219)">深蓝标题</span>
<!-- 警示红（错误 / 失败 / 红色加粗） -->
<span style="color:rgb(245,74,69)">警示红</span>
<!-- 紧急橙（紧急 / 阻塞 / 环比上升） -->
<span style="color:rgb(255,140,40)">紧急橙</span>
```

### URL scheme

```html
<a href="https://example.com">外链</a>
<a href="mailto:user@example.com">邮件链接</a>
<img src="cid:abc"> <!-- 内嵌图片，配合 --inline 参数 -->
<img src="data:image/png;base64,iVBOR..."> <!-- base64 内嵌图片 -->
```

## 官方 HTML 模板

仓库 [`../assets/templates/`](../assets/templates/) 内预制了若干场景模板，按 LarkSuite mail-editor 原生格式写好。**注意：模板是静态 HTML，没有变量替换能力，AI 需要手工把模板里的样例文本替换成本次邮件的真实内容。**

| 文件                              | 说明       |
|---------------------------------|----------|
| `newsletter--weekly-brief.html` | 资讯周报     |
| `weekly--personal-report.html`  | 工作周报（个人） |
| `weekly--team-report.html`      | 工作周报（团队） |
| `research--market-report.html`  | 调研报告     |
| `job-application--resume.html`  | 简历邮件     |

跟飞书 OAPI 个人邮件模板（`mail.user_mailbox.templates`）不同——OAPI 模板是用户邮箱里的"我的模板"，跨客户端可见；这里是仓库里的静态 HTML 文件，AI 单次套用即可。

### AI 套用流程

1. **判断是否能用模板** — 看用户当前要写的邮件类型（周报 / 调研 / 简历 / 资讯 / ...）能否对上 [`../assets/templates/`](../assets/templates/) 里的某个文件；不匹配就跳过模板，直接按写法规范从零写。
2. **Read 整个 HTML** — 用 Read 工具完整读取选定的模板文件，理解骨架（章节标题 / 列表层级 / 占位文本 / mention chip / 段落顺序）。
3. **替换文本内容** — 把模板里的样例文字换成用户当前邮件的真实内容；保留所有 inline style / class / data-* 等结构性属性不动；列表条目 / 表格行可按需增删；不需要的整段（如「风险」「下周计划」）整段删除即可，不要留空骨架。
4. **调写信 shortcut 生成草稿** — 把替换后的 HTML 通过 `--body` 参数交给写信链路（推荐 `+draft-create` 先存草稿、用户复核后再 `+send`）：

   ```bash
   lark-cli mail +draft-create --as user \
     --to alice@example.com --subject 'Q3 团队周报' \
     --body "$(cat skills/lark-mail/assets/templates/weekly--team-report.html)"
   ```

   实际使用时 `$(cat ...)` 可换成 AI 替换文本后写入的本地副本，或直接把替换后的 HTML 字符串作为 `--body` 的值。

5. **拿到草稿链接给用户复核** — 写信 shortcut 返回 `reference` 字段（草稿打开链接），把它给用户在飞书邮箱 UI 里打开核对，再决定下一步发送 / 编辑。

## 写信 shortcut 的 lint 返回值

写信链路（`+send` / `+draft-create` / `+reply` / `+reply-all` / `+forward` / `+draft-edit` body op）调用 `emlbuilder` 之前会强制 lint 净化 HTML，但 **默认 envelope 不携带任何 lint 字段**（既无 `*_count` 也无 finding 数组），envelope 保持小巧供 AI 消费。每个写信 shortcut 默认 envelope 的字段集合：

| 字段 | 出现条件 | 说明 |
|------|---------|------|
| `compose_hint` | 6 个 shortcut 默认都附 | 固定英文文案，提示 AI / 用户在组合 HTML 前阅读本文 |
| `draft_edit_hint` | **仅** `+draft-create` 默认附（其他 5 个 shortcut 不附） | 固定英文文案，提示拿到 `draft_id` 后改稿走 `+draft-edit --draft-id <id>` 而不是重跑 `+draft-create` 产生重复草稿 |
| `draft_id` / `message_id` | OAPI 写入成功后写回 | `+draft-create` / `+draft-edit` 返回 `draft_id`；`+send` / `+reply` / `+reply-all` / `+forward` 返回 `message_id` |

需要看 lint 详情时加 `--show-lint-details`：

```bash
lark-cli mail +draft-create --show-lint-details \
  --to alice@example.com --subject 'Hi' --body '<p>正文</p>'
```

加了 `--show-lint-details` 后 envelope 同时返回 `lint_applied[]` / `original_blocked[]` 两个完整 Finding 数组（每条含 `rule_id` / `severity` / `tag_or_attr` / `excerpt` / `hint`），**不再返回任何 `*_count` 字段** —— 调用方需要 count 时直接 `len(lint_applied)` / `len(original_blocked)`。**默认场景不要加这个 flag**，徒增 token 消耗。

如果只是想预览 lint 会怎么改 HTML，建议直接用 [`+lint-html`](./lark-mail-lint-html.md) 命令——它本来就返回完整 `warnings[]` / `errors[]` + `cleaned_html`，比写信链路 `--show-lint-details` 更清晰。

## 相关文档

- [`+lint-html` 用法](./lark-mail-lint-html.md)
- 写信 shortcut: [`+send`](./lark-mail-send.md) / [`+draft-create`](./lark-mail-draft-create.md) / [`+reply`](./lark-mail-reply.md) / [`+reply-all`](./lark-mail-reply-all.md) / [`+forward`](./lark-mail-forward.md) / [`+draft-edit`](./lark-mail-draft-edit.md)
