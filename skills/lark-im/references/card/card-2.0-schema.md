# 卡片 2.0 组件大纲

Card 2.0 组件按**容器 / 展示 / 交互**三类，均通过 `tag` 字段声明。先在下表按用途选组件，再点明细看字段：有明细文件的点 `components/<tag>.md`（完整字段+示例+易错点），低频组件点链接看官方文档。

## 根结构

顶层固定四字段，先搭骨架再往 `body.elements` 填组件。以下为**推荐完整骨架**（含 type scale、light/dark color token、header 三件套）：

```json
{
  "schema": "2.0",
  "config": {
    "update_multi": true,
    "width_mode": "default",
    "style": {
      "text_size": {
        "title":   { "default": "heading-2", "pc": "heading-2", "mobile": "heading-3" },
        "body":    { "default": "normal",    "pc": "normal",    "mobile": "normal"    },
        "caption": { "default": "notation",  "pc": "notation",  "mobile": "notation"  }
      },
      "color": {
        "cus-primary":    { "light_mode": "rgba(30,120,255,1)",    "dark_mode": "rgba(80,150,255,1)"   },
        "cus-primary-bg": { "light_mode": "rgba(30,120,255,0.08)", "dark_mode": "rgba(80,150,255,0.12)" },
        "cus-muted":      { "light_mode": "rgba(100,106,115,1)",   "dark_mode": "rgba(150,155,163,1)"  }
      }
    }
  },
  "header": {
    "title":    { "tag": "plain_text", "content": "卡片标题" },
    "subtitle": { "tag": "plain_text", "content": "副标题：一句上下文（时间/来源/状态）" },
    "template": "blue",
    "icon": { "tag": "standard_icon", "token": "notice_colorful" },
    "text_tag_list": [
      { "tag": "text_tag", "text": { "tag": "plain_text", "content": "状态标签" }, "color": "blue" }
    ]
  },
  "body": { "direction": "vertical", "padding": "12px 12px 20px 12px", "elements": [] }
}
```

> **按需裁剪**：`subtitle` / `text_tag_list` / color token 按实际诉求取舍，不强制全用。组件里用 `"text_size": "title"` / `"caption"` 引用 token，用 `"font_color": "cus-muted"` 引用颜色 token；主色系变化时只需改 config 里的 RGBA，全卡自动跟随。

- `schema` 必须显式为 `"2.0"`，否则按 1.0 渲染。`header` 详见 `components/header.md`。
- **元素通用字段**（所有 `elements[]` 组件）：`tag`(必填) · `element_id`(卡内唯一，字母开头、≤20 字符) · `margin`(外边距 [-99,99]px)。
- `card_link`（整卡跳转）：`{url, pc_url, ios_url, android_url}`，至少填 `url`；某端禁跳设 `lark://msgcard/unsupported_action`。
- 硬限制：单卡 ≤ **200** 元素；需客户端 **≥ 7.20**（旧版仅显示 header）。
- 颜色 / 图标枚举见 `resource/colors.md` · `resource/icons.md`。

**config**（全局行为，可整体省略）：

| 字段 | 默认 | 说明 |
|---|---|---|
| `update_multi` | true | 共享卡片，v2 仅支持 true |
| `width_mode` | default | `default`(≤600px) / `compact`(400px) / `fill`(撑满) |
| `enable_forward` | true | 是否允许转发 |
| `summary` | — | 会话列表预览：`{content, i18n_content:{zh_cn,en_us,…}}` |
| `streaming_mode` | false | 流式更新模式（配 `streaming_config`） |
| `style.text_size` | — | 自定义字号 token，格式 `{"<名称>":{default,pc,mobile}}`；名称可自定义（如 `title`/`caption`），组件 `text_size` 引用该名称 |
| `style.color` | — | 自定义颜色 token，格式 `{"<名称>":{light_mode,dark_mode}}`（RGBA）；名称可自定义（如 `cus-primary`），组件 `font_color`/`background_style` 等字段引用 |

> 多语言：`config.locales` 限定生效语种、`use_custom_translation` 优先用自带 i18n。

**body 布局字段**（均 v2 新增）：`direction`(vertical/horizontal) · `padding`([0,99]px) · `horizontal_spacing`/`vertical_spacing`(`small`4/`medium`8/`large`12/`extra_large`16 或 px) · `horizontal_align`/`vertical_align`。

---

## 容器类（布局 / 组织交互）

| 组件 | 用途 |
|---|---|
| [column_set](components/column_set.md) | 横向分栏，多列图文对齐（数据表、字段对、列表） |
| [collapsible_panel](components/collapsible_panel.md) | 折叠面板，收纳备注/长文本等次要信息 |
| [form](components/form.md) | 表单容器，批量录入表单项后一次提交 |
| [interactive_container](components/interactive_container.md) | 整块可点击区域，可统一定义样式与交互 |
| [循环容器](components/recycling_container.md) | 批量渲染同版式不同数据（仅搭建工具） |

## 展示类（无交互）

| 组件 | 用途 |
|---|---|
| [header](components/header.md) | 卡片标题区：主/副标题、后缀标签、主题色 |
| [div](components/div.md) | 普通文本，带前缀图标、字段对 |
| [markdown](components/markdown.md) | 富文本，最常用；@人、彩色、链接、列表、表格等 |
| [img](components/img.md) | 单图 |
| [img_combination](components/img_combination.md) | 多图拼排（双图/三图/宫格） |
| [person](components/person.md) | 单个人员头像/姓名 |
| [person_list](components/person_list.md) | 多个人员头像/姓名 |
| [chart](components/chart.md) | VChart 图表（折线/柱/饼/词云等） |
| [table](components/table.md) | 多列数据表（只能放根节点） |
| [hr](components/hr.md) | 分割线 |

## 交互类

| 组件 | 用途 |
|---|---|
| [button](components/button.md) | 按钮：回调 / 跳转 / 表单提交 |
| [input](components/input.md) | 文本输入框（多嵌在 form 内） |
| [overflow](components/overflow.md) | 折叠按钮组，收纳多个操作 |
| [select_static](components/select_static.md) | 下拉单选 |
| [multi_select_static](components/multi_select_static.md) | 下拉多选 |
| [select_person](components/select_person.md) | 人员单选 |
| [multi_select_person](components/multi_select_person.md) | 人员多选 |
| [date_picker](components/date_picker.md) | 日期选择器 |
| [picker_time](components/picker_time.md) | 时间选择器 |
| [picker_datetime](components/picker_datetime.md) | 日期时间选择器 |
| [select_img](components/select_img.md) | 图片选择（单/多选） |
| [checker](components/checker.md) | 勾选器，任务勾选回调 |
