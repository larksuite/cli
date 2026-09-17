# 按钮 `button`

交互按钮，支持跳转 / 回调 / 表单提交三类行为。**Card 2.0**。

## 最小示例

```json
{
  "tag": "button",
  "text": { "tag": "plain_text", "content": "确定" },
  "type": "primary",
  "behaviors": [{ "type": "callback", "value": { "action": "ok" } }]
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `button` |
| `text` | 否 | Object | / | `{tag:"plain_text", content}`，≤100 字符 |
| `type` | 否 | String | default | 见下方 type 枚举 |
| `size` | 否 | String | medium | `tiny` / `small` / `medium` / `large` |
| `width` | 否 | String | default | `default` / `fill` / `[100,∞)px` |
| `behaviors` | 是* | Array | / | 交互行为，见下；表单内按钮不用 behaviors 而用 `form_action_type` |
| `icon` | 否 | Object | / | 前缀图标（同 `div.icon`） |
| `hover_tips` | 否 | Object | / | PC 端悬浮提示，plain_text |
| `disabled` | 否 | Boolean | false | 是否禁用 |
| `disabled_tips` | 否 | Object | / | 禁用后悬浮提示，plain_text |
| `confirm` | 否 | Object | / | 二次确认弹窗 `{title, text}`（均 plain_text，title 必填） |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |
| `element_id` | 否 | String | / | 唯一标识，字母开头 ≤20 字符 |

**type 枚举**：`default`(黑字描边) / `primary`(蓝字描边) / `danger`(红字描边) / `text` / `primary_text` / `danger_text`(无边框) / `primary_filled`(蓝底白字) / `danger_filled`(红底白字) / `laser`(镭射)。

## 按钮主次（强制）

- 全卡仅 1 个按钮 → `type: "primary_filled"`，并 `width: "fill"` 撑满成强焦点。
- 多个并列按钮 → 第一个（主操作）`primary_filled`，其余一律 `default`，形成「一主多次」层级。
- 删除 / 拒绝等危险操作用 `danger` 系（`danger` 或 `danger_filled`）。

## behaviors（交互行为）

```json
// 1. 服务端回调
{ "type": "callback", "value": { "key": "v" } }
// 2. 跳转链接（可与 callback 同数组共存）
{ "type": "open_url", "default_url": "https://x", "pc_url": "", "ios_url": "", "android_url": "" }
```

表单容器内的按钮 **不用 behaviors**，改用根字段：

| 字段 | 必填 | 说明 |
|---|---|---|
| `name` | 是 | 表单内唯一标识 |
| `form_action_type` | 是 | `submit`（提交表单）/ `reset`（重置） |

## 嵌套 / 易错点

- 可嵌套在 column_set / form / collapsible_panel / 循环容器 / interactive_container 内。
- 2.0 已废弃 `action` 交互模块，按钮直接放 `elements`，用间距控制排列。
- 旧式 `url`/`value` 顶层字段是 1.0 写法；2.0 一律用 `behaviors`。
- 点击触发 `card.action.trigger`，回传 `action.tag="button"` + `action.value`（即 callback 的 value）。
