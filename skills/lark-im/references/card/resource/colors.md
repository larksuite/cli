# 颜色枚举

卡片所有颜色字段（`font_color` / `text_color` / `background_style` / `border_color` / icon `color` 等）共用同一套枚举，按属性名区分用途，无单独的文字/背景色表。

## 基础色名（14 色系）

`blue` `carmine` `green` `indigo` `lime` `orange` `purple` `red` `sunflower` `turquoise` `violet` `wathet` `yellow` `grey`

> **标签例外**：`text_tag` / `<text_tag>` 的灰色用 `neutral`（不是 `grey`）；标签枚举无 `grey`。

## 深浅后缀

- 彩色系（13 个非 grey）：`-50 -100 -200 -300 -350 -400 -500 -600 -700 -800 -900`，数字越大越深。
- **无后缀基础名（如 `blue`）= `-600`**（同色值）。
- grey 范围更细：`-00 -50 -100 … -650 … -950 -1000`。
- 用法语义：`-50` 区块背景 · `-100` 标签背景 · `-500` 正文 · `-600/-700` 强调文字。

## 特殊值

`white`（白）· `bg-white`（背景白：浅色 #ffffff / 深色 #1A1A1A）。无 `transparent` 枚举。

## 自定义 RGBA

在 `config.style.color` 定义 token 再引用：

```json
"config": { "style": { "color": {
  "cus-0": { "light_mode": "rgba(5,157,178,0.52)", "dark_mode": "rgba(...)" }
} } }
```

组件里写 `"font_color": "cus-0"`。RGBA 支持的属性同枚举（font/text_color、background_style、border_color、icon color 等）。

> `column` 的 `background_style` 需客户端 v7.9+。配色搭配规则见 `../lark-im-card-style.md` 视觉规范。
