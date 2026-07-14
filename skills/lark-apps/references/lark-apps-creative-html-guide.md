# 创意模式 HTML 编写指南

> **前置条件：** 先阅读 [`../../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解通用安全规则。本文档定义妙搭 HTML 应用场景下创意模式的 HTML / CSS / JS 写法规范、设计原则、可复制片段和场景模板。

**CRITICAL 创意模式 HTML 不同于普通网页开发——它运行在妙搭托管环境中，必须遵守本文档的约束**
**CRITICAL 所有产出必须是单文件 `index.html`（内联 CSS + JS），不依赖外部构建工具**
**CRITICAL 发布通过 `lark-cli apps +html-publish` 完成，发布前必读 [`lark-apps-html-publish.md`](lark-apps-html-publish.md) 了解体积限制和发布流程**

## 适用场景

创意模式适用于以下场景的 HTML 应用开发：

| 场景 | 示例 |
|------|------|
| 活动页 / 落地页 | 报名页、宣传页、邀请函 |
| 数据可视化 / 仪表盘 | 图表展示、实时数据看板 |
| 互动工具 | 投票、抽奖、问卷、小游戏 |
| 展示型页面 | 作品集、产品介绍、PPT 式演示 |

## 文件结构

创意模式产出为**单个 `index.html` 文件**，所有资源内联：

```html
<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>页面标题</title>
  <style>
    /* CSS 写在这里 */
  </style>
</head>
<body>
  <!-- HTML 结构 -->

  <script>
    // JS 写在这里
  </script>
</body>
</html>
```

## 设计原则

### 视觉风格

- **现代感优先**：圆角、柔和阴影、渐变色、适量留白
- **移动端适配**：使用 `viewport` meta + 响应式布局（flexbox / grid），确保手机端可用
- **字体栈**：优先系统字体，避免外部字体加载延迟

```css
body {
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif;
}
```

### 调色盘（推荐）

```css
/* 主色 */
--primary: #3370FF;       /* 飞书蓝 */
--primary-hover: #2860E1;
--primary-light: #E1EAFF;

/* 中性色 */
--text-primary: #1F2329;   /* 主文本 */
--text-secondary: #646A73; /* 副文本 */
--text-placeholder: #8F959E;
--bg-body: #F5F6F7;        /* 页面背景 */
--bg-card: #FFFFFF;         /* 卡片背景 */
--border: #DEE0E3;          /* 边框 */

/* 语义色 */
--success: #34C724;
--warning: #FF7D00;
--danger: #F54A45;
--info: #3370FF;
```

### 间距系统

使用 4px 基准的间距系统：

| token | 值 | 用途 |
|-------|------|------|
| xs | 4px | 紧凑元素间距 |
| sm | 8px | 相关元素间距 |
| md | 16px | 段落/卡片内间距 |
| lg | 24px | 区块间距 |
| xl | 32px | 大区域分隔 |
| xxl | 48px | 页面级分隔 |

## 常用组件片段

### 卡片

```html
<div style="background:#fff;border-radius:12px;padding:24px;box-shadow:0 2px 8px rgba(0,0,0,0.08);margin-bottom:16px">
  <h3 style="margin:0 0 12px;font-size:18px;color:#1F2329">卡片标题</h3>
  <p style="margin:0;color:#646A73;font-size:14px;line-height:1.6">卡片内容描述</p>
</div>
```

### 按钮

```html
<!-- 主按钮 -->
<button style="background:#3370FF;color:#fff;border:none;border-radius:8px;padding:10px 24px;font-size:14px;cursor:pointer;transition:background 0.2s" onmouseover="this.style.background='#2860E1'" onmouseout="this.style.background='#3370FF'">
  主按钮
</button>

<!-- 次按钮 -->
<button style="background:#fff;color:#3370FF;border:1px solid #3370FF;border-radius:8px;padding:10px 24px;font-size:14px;cursor:pointer">
  次按钮
</button>
```

### 表格

```html
<table style="width:100%;border-collapse:collapse;font-size:14px">
  <thead>
    <tr style="background:#F5F6F7">
      <th style="padding:12px 16px;text-align:left;color:#646A73;font-weight:500;border-bottom:1px solid #DEE0E3">列A</th>
      <th style="padding:12px 16px;text-align:left;color:#646A73;font-weight:500;border-bottom:1px solid #DEE0E3">列B</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td style="padding:12px 16px;border-bottom:1px solid #F0F1F2;color:#1F2329">数据</td>
      <td style="padding:12px 16px;border-bottom:1px solid #F0F1F2;color:#1F2329">数据</td>
    </tr>
  </tbody>
</table>
```

### 标签/徽章

```html
<span style="display:inline-block;padding:2px 8px;border-radius:4px;font-size:12px;background:#E1EAFF;color:#3370FF">标签</span>
<span style="display:inline-block;padding:2px 8px;border-radius:4px;font-size:12px;background:#FFF0E0;color:#FF7D00">警告</span>
<span style="display:inline-block;padding:2px 8px;border-radius:4px;font-size:12px;background:#E8F8E5;color:#34C724">成功</span>
```

## 外部资源引用规则

<!-- TODO: 补充妙搭托管环境对外部资源的限制（CDN 白名单、CSP 策略等） -->

### 允许的 CDN 库

```html
<!-- 图表库 -->
<script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
<script src="https://cdn.jsdelivr.net/npm/echarts/dist/echarts.min.js"></script>

<!-- 动画库 -->
<script src="https://cdn.jsdelivr.net/npm/animejs/lib/anime.min.js"></script>

<!-- 图标 -->
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@icon-park/svg/styles/index.css">
```

### 图片

- 优先使用 SVG 内联或 base64 编码小图
- 大图通过妙搭文件存储上传后引用签名 URL（见 [`lark-apps-file.md`](lark-apps-file.md)）

## 禁止事项

- **禁止** 使用 `<iframe>` 嵌入外部页面
- **禁止** 发起跨域 API 请求（除非目标在妙搭允许的域名内）
- **禁止** 使用 `document.cookie` 读写 cookie
- **禁止** 使用 `localStorage` / `sessionStorage` 存储敏感信息
- **禁止** 内联 `<script>` 中引入挖矿、追踪等恶意代码

## 发布流程

编写完成后通过 `+html-publish` 发布，详见 [`lark-apps-html-publish.md`](lark-apps-html-publish.md)。

```bash
lark-cli apps +html-publish --app-id <app_id> --path ./index.html --as user
```

## 相关文档

- [`+html-publish` 发布指南](lark-apps-html-publish.md)
- [`+create` 创建应用](lark-apps-create.md)
- [应用文件存储](lark-apps-file.md)
- [可见范围设置](lark-apps-access-scope-set.md)
