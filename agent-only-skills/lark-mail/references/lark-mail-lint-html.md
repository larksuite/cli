# mail +lint-html

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解通用安全规则。

## 作用

`+lint-html` 是邮件 HTML 正文的本地预检工具（read-only，无网络 IO）。

- 校验 HTML 是否符合飞书邮箱的兼容性 / 安全 / 原生写法要求；
- 自动修复非法或不规范写法（autofix 始终启用），输出 `cleaned_html`；
- 不写入任何邮箱状态，不调用任何 OAPI。

写信链路（`+send` / `+draft-create` / `+reply` / `+reply-all` / `+forward` / `+draft-edit` body op）已**强制内置**同一份 lint，提交前会自动净化 HTML。默认 envelope 不携带任何 lint 字段以保持响应小巧；加 `--show-lint-details` 可拿到完整 `lint_applied[]` / `original_blocked[]` 两个 Finding 数组（不再返回任何 `*_count` 字段，调用方需要 count 时 `len(arr)` 即可，详见 [邮件 HTML 写法指南](./lark-mail-html.md#写信-shortcut-的-lint-返回值)）。本命令是写信链路 lint 的预览版，行为一致，调用更轻量，适合：

- AI / 用户在创建草稿前自检 HTML 会被怎么改写；
- CI 流水线把 HTML 模板当作产物校验。

## 命令

```bash
# 直接传 HTML
lark-cli mail +lint-html --body '<p>正文</p>'

# 从文件读 HTML（路径必须在 cwd 子树内）
lark-cli mail +lint-html --body-file ./template.html

# 查看完整 lint 详情
lark-cli mail +lint-html --body-file ./template.html --show-lint-details
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--body <html>` | 二选一 | 待检查的 HTML 内容 |
| `--body-file <path>` | 二选一 | 从文件读取 HTML，仅支持 cwd 子树（绝对路径 / `..` 越出 cwd 会被拒） |
| `--show-lint-details` | 否 | 默认 `false`。`true` 时 envelope 同时返回 `warnings[]` / `errors[]` 完整 Finding 数组；默认仅返回 `cleaned_html`，避免复杂模板触发数十条装饰性 warning 把响应撑大几千 token |
| `--format <fmt>` | 否 | `json`（默认）/ `pretty` / `table` / `csv` / `ndjson` |
| `--jq <expr>` | 否 | 对返回 JSON 用 jq 表达式过滤 |
| `--dry-run` | 否 | 不执行 lint，仅返回 dry-run 描述 |

## 返回值

**默认 envelope**（仅 `cleaned_html`，token-frugal）：

```json
{
  "ok": true,
  "data": {
    "cleaned_html": "<p>...</p>"
  }
}
```

**加 `--show-lint-details` 后**：

```json
{
  "ok": true,
  "data": {
    "cleaned_html": "<p>...</p>",
    "warnings": [
      { "rule_id": "...", "severity": "warning", "tag_or_attr": "...", "excerpt": "...", "hint": "..." }
    ],
    "errors": [
      { "rule_id": "...", "severity": "error", "tag_or_attr": "...", "excerpt": "...", "hint": "..." }
    ]
  }
}
```

| 字段 | 说明 |
|------|------|
| `cleaned_html` | 修复后的 HTML（autofix 始终启用）；warning 已自动改写，error 已删除 |
| `warnings[]` | 警告级 finding 数组（**仅 `--show-lint-details` 时返回**）。无违规时输出 `[]` |
| `errors[]` | 错误级 finding 数组（**仅 `--show-lint-details` 时返回**）。无违规时输出 `[]` |

每条 finding 含：

| 字段 | 说明 |
|------|------|
| `rule_id` | 规则编号（UPPER_SNAKE_CASE） |
| `severity` | `"warning"` 或 `"error"` |
| `tag_or_attr` | 触发规则的 tag / attribute / `style.<property>` |
| `excerpt` | HTML 片段（最多 200 字节，超出截断） |
| `hint` | 可读的修复说明 |

## 调用示例

下面是用 `lark-cli mail +lint-html --body '<INPUT>' --show-lint-details` 实跑得到的典型 case（加 `--show-lint-details` 才能看到 finding；默认只返回 `cleaned_html`），覆盖 error 类（强制删）和 warning 类（自动修复）。

### Error 类（强制删除，写信链路也会拒）

#### 1. `<script>` 整段删除

输入：

```html
<script>alert(1)</script>正文
```

输出：

```html
正文
```

原因：`<script>` 有 XSS 风险，整段丢弃。

#### 2. `javascript:` URL 删除

输入：

```html
<a href="javascript:void(0)">click</a>
```

输出：

```html
<a class="not-doclink" style="cursor:pointer;text-decoration:none;color:rgb(20,86,240)">click</a>
```

原因：`javascript:` scheme 是 XSS 入口，`href` 属性被剥。

#### 3. `on*` 事件 handler 删除

输入：

```html
<p onclick="alert(1)">hi</p>
```

输出：

```html
<div style="margin-top:4px;margin-bottom:4px;line-height:1.6"><div dir="auto" style="font-size:14px">hi</div></div>
```

原因：inline event handler（`onclick` / `onerror` 等）是脚本注入入口，属性被剥。

### Warning 类（自动修复，视觉无差异）

#### 4. `<font>` → `<span style>`

输入：

```html
<font color="red" size="3">字</font>
```

输出：

```html
<span style="color:red; font-size:16px">字</span>
```

原因：`<font>` 是 HTML4 过时标签，飞书 mail-editor 用 inline style 表达字号 / 颜色。

#### 5. `<p>` 段落容器原生化

输入：

```html
<p>正文</p>
```

输出：

```html
<div style="margin-top:4px;margin-bottom:4px;line-height:1.6"><div dir="auto" style="font-size:14px">正文</div></div>
```

原因：飞书 mail-editor 段落实际是双层 div（外层定 margin / line-height，内层定 font-size）。

#### 6. `<ul>/<li>` 列表原生化

输入：

```html
<ul><li>第一项</li></ul>
```

输出：

```html
<ul style="margin-top:0px;margin-bottom:0px;margin-left:0px;padding-left:0px;list-style-position:inside" data-list-bullet="true"><li class="temp-li bullet1" data-li-line="true" data-list="bullet1" style="line-height:1.6;margin-top:0px;margin-bottom:0px;padding-left:0px;display:list-item;list-style-type:disc;font-family:inherit;font-size:14px;margin-left:0px;list-style-position:inside" dir="auto"><span style="font-family:inherit"><span style="color:rgb(0,0,0)">第一项</span></span></li></ul>
```

原因：飞书 native list-block 要求 `<ul>` / `<li>` 补全 class + data marker + 双层 span 包裹，否则 li 之间会出现可见空行。

#### 7. `<blockquote>` 加灰边 + 灰文字

输入：

```html
<blockquote>引用</blockquote>
```

输出：

```html
<blockquote style="padding-left:0px;color:rgb(100,106,115);border-left:2px solid rgb(187,191,196);margin:0px">引用</blockquote>
```

原因：补飞书原生引用样式（左侧 2px 灰边 + 灰色文字）。

#### 8. `<a>` 链接补 not-doclink + LarkSuite 蓝

输入：

```html
<a href="https://example.com">link</a>
```

输出：

```html
<a href="https://example.com" class="not-doclink" style="cursor:pointer;text-decoration:none;color:rgb(20,86,240)">link</a>
```

原因：补 `not-doclink` class（防误识为内部 doc share）+ LarkSuite 品牌蓝 + 无下划线。

#### 9. 非白名单 CSS property 删除

输入：

```html
<p style="position:absolute;color:red">x</p>
```

输出：

```html
<div style="color:red;margin-top:4px;margin-bottom:4px;line-height:1.6"><div dir="auto" style="font-size:14px">x</div></div>
```

原因：`position` 不在 inline style 白名单内被剔除，`color` 保留。

## 相关命令

- 写信 shortcut（已内置同一份 lint）：[`+send`](./lark-mail-send.md) / [`+draft-create`](./lark-mail-draft-create.md) / [`+reply`](./lark-mail-reply.md) / [`+reply-all`](./lark-mail-reply-all.md) / [`+forward`](./lark-mail-forward.md) / [`+draft-edit`](./lark-mail-draft-edit.md)
- 知识文档：[邮件 HTML 写法指南](./lark-mail-html.md)
