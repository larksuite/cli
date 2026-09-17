# 人员 `person`

展示单个用户的头像/姓名，点击可看名片。**Card 2.0**。

## 最小示例

```json
{
  "tag": "person",
  "user_id": "ou_xxx",
  "show_name": true
}
```

## 字段

| 字段 | 必填 | 类型 | 默认 | 说明 |
|---|---|---|---|---|
| `tag` | 是 | String | / | 固定 `person` |
| `user_id` | 是 | String | / | 人员 ID，支持 open_id / union_id / user_id |
| `size` | 否 | String | medium | `extra_small` / `small` / `medium` / `large` |
| `show_avatar` | 否 | Boolean | true | 是否显示头像 |
| `show_name` | 否 | Boolean | false | 是否显示姓名 |
| `style` | 否 | String | normal | `normal` / `capsule`（胶囊） |
| `margin` | 否 | String | 0 | 外边距 [-99,99]px |
| `element_id` | 否 | String | / | 唯一标识，字母开头 ≤20 字符 |

## 易错点

- 发卡应用需有访问用户 ID 的权限，否则人员信息无法展示。
