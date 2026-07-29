---
name: wireframe
metadata:
  display-names:
    zh-CN: 线框图
    en-US: Wireframe
description: 用线框图和故事板探索多种想法。触发词：wireframe, storyboard, 线框图, 故事板, 分镜, 草图, 低保真, 方案探索, 设计探索
available-agents:
  - CreativeDesign
---

# 线框图

帮助用户快速探索设计想法。提问遵循 [`../creative-design.md`](../creative-design.md)「提问」一节：关键信息缺失且承重时先做一轮聚焦提问，否则基于合理假设直接铺方案。生成多个粗略的线框图，在锁定方向之前把设计空间勾勒出来。优先追求广度而非精细打磨：默认每个想法给出 2-3 种明显不同的方案，用户明确要求广度探索时再加。用简单的形状、占位文字和极少的颜色，把焦点留在结构和流程上。整体保持手绘草图的感觉——手写风格但清晰可读的字体；以黑白为主、点缀少量颜色；低保真、简洁。多方案默认铺进 `design-canvas.jsx` 画布（见 [`../creative-design.md`](../creative-design.md)「如何开展设计工作」）；单个 artboard 内部的局部变体用 Tweaks 承载（用 `tweaks-panel.jsx`，见 [`../creative-design.md`](../creative-design.md)「Tweaks」，不要手写控件面板）。
