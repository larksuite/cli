# +search-bot

按关键词搜索当前用户可见的机器人,返回的机器人 open_id 同样是 `ou_` 开头。

## 关键 flag

必须传 `--query` 或 `--queries`;`--chat-ids` / `--has-chatted` 都不能单独使用。

| Flag | 说明 |
|---|---|
| `--query <text>` | 搜索一个关键词,最多 50 个字符 |
| `--queries <csv>` | 并行搜索多个关键词,最多 20 个;不能和 `--query` 一起使用 |
| `--chat-ids <csv>` | 只在指定群内搜索,最多 100 个群;支持群 ID 或群链接 |
| `--has-chatted` | 只返回聊过天的机器人。**不是默认写法**,不需要时不要传 |
| `--page-size <n>` | 返回条数,1–30,默认 20 |

## 输出与选择

- 关键字段:`is_agent`(是否是智能体)、`enable_join_group`(是否允许加入群聊)、`description`(简介,为空时字段省略)、`chat_id`(与机器人的单聊 ID,为空表示没有单聊)。
- **多条命中怎么选**:结合 `description` 和 `is_agent` 判断。后续要发消息或拉群时,让用户确认目标,不要直接选择第一条。
- **不支持分页**。`has_more=true` 时改用更具体的关键词,或用 `--chat-ids` 收窄范围。
- `--queries` 模式:每条结果带 `matched_query`;`queries[]` 给每个关键词的执行结果;部分关键词失败时保留其他结果,全部失败时命令报错;`--chat-ids` / `--has-chatted` 对所有关键词生效。
