# XML 扩展块补充说明

本文件用于补充说明 block XML 扩展能力。常用标签和通用规则见 [`lark-doc-xml.md`](lark-doc-xml.md)；后续新增其他 block 说明时可继续追加到本文件。

## HTML5 block

1. 写入 HTML 内容块时，把完整单文件 HTML 存为本地 `.html` 文件，XML 写 `<html5-block path="@widget.html"></html5-block>`；已有 `data-ref` 时配合 `--reference-map @reference-map.json`。读取时 `<html5-block data-ref="html5_1"></html5-block>` 只是占位，必须从 `document.reference_map["html5-block"]["html5_1"].data` 读取 HTML；若 entry 是 `path`，读取对应 `@doc-fetch-resources/...html` 文件。
2. 格式如下：

```html
<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <meta name="use-iframe" content="true">
  <meta name="html-box-height-mode" content="auto">
  <meta name="description" content="内容摘要，会导出为 html5-block 的 alt 属性，帮助模型理解该 HTML 块的用途">
  <title></title>
</head>
<body>
  ...
</body>
</html>
```

### 布局与高度

- `lark-cli` 会读取 `.html` 文件并原样写入 `reference_map`，不会解析或校验 `html-box-height-mode`；创建或更新文档前在 `<head>` 中显式声明 `auto` 或 `viewport`。
- 生成时只使用 `auto` 或 `viewport`，不要臆造 `fixed`、`initial` 或像素值等其他 mode。
- 文档常见可用宽度约 `820px`；根容器使用 `width: 100%`、`max-width: 100%`、`box-sizing: border-box`。

四种策略：

1. 内容自然撑开：`auto` + 普通文档流；根容器不设固定高度或 `overflow: hidden`。
2. 仅按初始内容定高：`auto` + 首次渲染后不再追加或展开内容。
3. 固定像素操作区：`auto` + 业务容器按场景设置固定的 CSS `height` 和 `overflow: auto`；高度数值不写进 meta。
4. 单屏应用：`viewport` + `100vh` + 内部滚动、切页或缩放；适用于游戏、幻灯片、Dashboard、canvas 编辑器。

正文需要在飞书文档中完整展开时选 `auto`；内容应在 HTML Block 内滚动时选 `viewport`。`lark-cli` 不参与页面加载后的高度刷新，不要臆造相关 CLI flag。

### 内容限制

- HTML 总长度上限为 500KB。不要内联大图片、Base64、字体、长 JSON/CSV 或大量 mock 数据。

## OKR block

OKR block 可用 XML 格式完整表达。创建前先参考 [`lark-okr`](../../lark-okr/SKILL.md) 确认可用周期；创建时只写 root-only `<okr cycle-id="..."/>` 挂载已有 OKR，不构造 Objective/KR/Progress 子树。

获取时，XML 结构示例如下：

```xml
<okr cycle-id="" cycle-name="CYCLE_NAME" user-name="USER_NAME">
  <okr-objective objective-id="OBJECTIVE_ID" status="normal" percent="80" score="75">
    <p>O 描述</p>
    <okr-progress>
      <p>O 进展</p>
      <checkbox done="true">事项</checkbox>
      <ul><li>列表项内可包含 <a href="https://example.com">链接</a></li></ul>
    </okr-progress>
    <okr-key-result key-result-id="KEY_RESULT_ID" status="risk" percent="60" score="80">
      <p>KR 描述</p>
      <okr-progress>
        <p>KR 进展</p>
      </okr-progress>
    </okr-key-result>
  </okr-objective>
</okr>
```

- `cycle-id` 仅用于创建时挂载已有当前周期 OKR；`cycle-name`、`user-name` 只读。
- `objective-id`、`key-result-id` 为只读业务 ID，更新已有 OKR 时保持不变。
- `okr-objective` / `okr-key-result`
  - 可更新 `status`、`percent`、`score`；`percent` / `score` 取值 0-100，`status` 取值 `unset`/`normal`/`risk`/`extended`。
  - 不可更新 objective 和 key-result 内容描述。
- `okr-progress` 承载进展内容，支持更新。直接子节点支持 `<p>`、`<checkbox>`、`<grid>`、`<img>`、`<source>`、`<ol>`、`<ul>`、`<h1>` 到 `<h9>`。
