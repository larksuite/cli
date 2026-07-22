---
name: interactive-prototype
metadata:
  display-names:
    zh-CN: 交互原型
    en-US: Interactive Prototype
description: 可交互原型：像真实应用一样直接运行的高保真交互 demo（working app with real interactions）。触发词：可交互原型, 交互原型, 点击原型, interactive prototype, working app, 产品 demo, 工单系统, 管理后台, 看板工具, 多页面应用
available-agents:
  - CreativeDesign
---

Create a fully interactive prototype with realistic state management and transitions. Use React useState/useEffect for dynamic behavior. Include hover states, click interactions, form validation, animated transitions, and multi-step navigation flows. It should feel like a real working app, not a static mockup.

Do not wrap interactive prototypes in `design-canvas.jsx`, `<DCArtboard>`, or any pan/zoom artboard shell. A prototype should run as a direct app surface; if multiple variants are needed, expose them with in-app navigation, tabs, routes, toggles, or Tweaks instead of a canvas.

## 多页面与路由

多页面原型按普通 MPA 做：一个页面一个 HTML 文件，入口固定为项目根目录的 `index.html`，页面间用相对路径的普通链接跳转（`<a href="detail.html">`）。不要引入任何 router 库——锁定版本的 CDN 清单里没有 router，也不要用 `type="module"` 模拟 SPA 路由。共享组件和样式拆成独立的 `.jsx` / `.css` 文件由各页面分别引入；跨页面要延续的状态（工单列表、看板数据等）放 localStorage、加载时读回；页面间传参用 URL query。

## 像真实应用，而不是摆拍

- 准备一份贴近业务的 mock 数据（名称、状态、时间戳都要像真的），页面从数据渲染，不要把内容写死在标记里。
- 每个可见的按钮、输入、切换都要有反应：提交有校验和反馈、列表可增删改、状态会流转、空状态有设计。点了没反应的控件比没有这个控件更伤可信度。
- 按 [`../creative-design.md`](../creative-design.md)「Tweaks」把关键选项（主题色、密度、布局变体等）用 `tweaks-panel.jsx` 暴露出来，不要自己实现控件面板。
