# 人员列表 `person_list`

展示多个用户的头像/姓名。**Card 2.0**。

## 最小示例

```json
{
  "tag": "person_list",
  "persons": [{ "id": "ou_xxx" }, { "id": "ou_yyy" }]
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `person_list` |
| `persons` | 是 | Array | / | 每项 `{ id }`，id 支持 open_id / union_id / user_id |
| `show_name` | 否 | Boolean | true | 是否显示姓名；关掉且多人时为"葫芦串"叠头像样式 |
| `show_avatar` | 否 | Boolean | false | 是否显示头像 |
| `size` | 否 | String | medium | `extra_small` / `small` / `medium` / `large` |
| `lines` | 否 | Int | / | 最大行数，不可为 0 |
| `drop_invalid_user_id` | 否 | Boolean | false | true 忽略无效 ID；false 则有无效 ID 时报错 |
| `icon` / `ud_icon` | 否 | Object | / | 前缀图标（同 `div.icon`）；两者同设以 `icon` 为准 |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |
| `element_id` | 否 | String | / | 唯一标识，字母开头 ≤20 字符 |

## 易错点

- 发卡应用需有访问用户 ID 的权限，否则无法展示人员信息。
