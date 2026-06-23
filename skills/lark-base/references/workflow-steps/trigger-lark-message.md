# LarkMessageTrigger

```json
{
  "receive_scene": "group",
  "receiver": [{ "value_type": "group", "value": {"id": "oc_xxxx", "name": "测试群"} }],
  "scope": "all",
  "filter": {
    "conjunction": "and",
    "content_contains": ["关键词"],
    "sender_contains": [{ "value_type": "user", "value": {"id": "ou_xxxx", "name": ""} }],
    "is_new_message": true,
    "is_message_contain_attachment": false
  }
}
```

| 字段 | 必填 | 说明|
|------|------|---|
| `receive_scene` | 是 | 接收场景：`group`（群聊）/ `chat`（单聊）|
| `receiver` | 是 | 触发来源，支持 `user` / `group` / `ref`。在单聊场景下，该字段指“可以和机器人单聊的用户”；在群聊场景下，该字段指“接收信息的群组”|
| `scope` | 是 | 触发范围：`at`（@提及）/ `all`（所有消息）。该参数仅在群聊场景有效，单聊场景请勿指定该参数|
| `filter` | 是 | MessageFilter 消息过滤条件|

`MessageFilter`：

| 字段 | 类型 | 说明 |
|------|------|----|
| `conjunction` | string | `and` 满足所有条件 / `or` 任一条件|
| `content_contains` | string[] | 关键词列表|
| `sender_contains` | ValueInfo[] | 筛选发送人（仅群聊+群组来源时生效，单聊场景请勿指定该参数）|
| `is_new_message` | boolean | 仅新话题消息（仅群聊时有效，单聊场景请勿指定该参数）|
| `is_message_contain_attachment` | boolean | 是否仅附件消息触发|

---

## 相关

- 返回 [Workflow schema index](../lark-base-workflow-schema.md)
- ValueInfo、ref、Condition、RecordFilterInfo 等公共结构见 [common-types-and-refs.md](common-types-and-refs.md)
