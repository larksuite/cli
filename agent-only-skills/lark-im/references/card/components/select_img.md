# 多图选择 `select_img`

以图片为选项的交互组件，支持单选/多选（如商品图、模板图、AI 生成图）。仅支持手写 JSON，搭建工具不支持。**Card 2.0**。

## 最小示例

```json
{
  "tag": "select_img",
  "name": "select_img_1",
  "layout": "bisect",
  "aspect_ratio": "16:9",
  "options": [
    { "img_key": "img_v2_xxx", "value": "picture1" },
    { "img_key": "img_v2_yyy", "value": "picture2" }
  ]
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `select_img` |
| `options` | 是 | Array | / | 选项，每项 `{img_key, value, disabled?, disabled_tips?, hover_tips?}` |
| `multi_select` | 否 | Boolean | false | 多选仅支持异步提交，**必须**内嵌在 form 中，否则报错 |
| `layout` | 否 | String | bisect | 图片布局：`stretch`(撑满)/`bisect`(二等分)/`trisect`(三等分) |
| `aspect_ratio` | 否 | String | 16:9 | `1:1`/`16:9`/`4:3` |
| `name` | 否* | String | / | 唯一标识；**form 内必填且全局唯一** |
| `required` | 否 | Boolean | false | 是否必选（form 内生效） |
| `can_preview` | 否 | Boolean | true | 点击图片是否弹窗放大（仅 form 内生效） |
| `disabled` | 否 | Boolean | false | 是否禁用整组件 |
| `value` | 否 | String/Object | / | 自定义回传参数 |
| `behaviors` | 是 | Array | / | `[{type:"callback", value:{...}}]` |
| `confirm` | 否 | Object | / | 二次确认弹窗 `{title, text}` |

## 嵌套 / 易错点

- 可嵌套在根节点 / column_set / form / 交互容器（搭建工具暂不支持嵌套交互容器）。
- **不在 form 内**：仅支持单选，点击立即提交触发回调，不支持多选/异步提交。
- **在 form 内**：支持单选/多选 + 异步提交（随表单一起提交）。
- 回调（非 form）：`action.tag="select_img"` + `action.options`（单选时仍是该字段）；form 内则读 `form_value[name]`。
