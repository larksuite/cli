# +search-bot

按关键词搜索当前用户可见的机器人。仅支持 user 身份,需要 `search:bot` 权限。

- ✅ 已知机器人名字想找出它的 open_id
- ✅ 一次解析多个名字(`--queries`)
- ✅ 在指定群范围内搜索机器人(`--chat-ids`)

## 参数

必须传 `--query` 或 `--queries`。`--chat-ids` 和 `--has-chatted` 只能用于筛选,不能单独使用。

| Flag | 说明 |
|---|---|
| `--query <text>` | 搜索一个关键词,最多 50 个字符 |
| `--queries <csv>` | 并行搜索多个关键词,最多 20 个;每个最多 50 个字符。不能和 `--query` 一起使用 |
| `--chat-ids <csv>` | 只在指定群内搜索,最多 100 个群;支持群 ID 或群链接 |
| `--has-chatted` | 只返回聊过天的机器人;不需要时不要传此参数 |
| `--page-size <n>` | 返回条数,1–30,默认 20 |

```bash
lark-cli contact +search-bot --query '会议助手' --as user
lark-cli contact +search-bot --query '助手' --has-chatted --as user
lark-cli contact +search-bot --queries '会议助手,日报助手,审批助手' --as user
```

## 输出

| 字段 | 类型 | 说明 | 空值时 |
|---|---|---|---|
| `open_id` | string | 机器人 ID | 始终非空 |
| `name` | string | 机器人名称 | 无名称时使用 `open_id` |
| `description` | string | 机器人简介 | 字段省略 |
| `p2p_chat_id` | string | 与机器人的单聊 ID | 空字符串 |
| `has_chatted` | bool | 是否聊过天 | — |
| `enable_join_group` | bool | 是否允许加入群聊 | — |
| `is_agent` | bool | 是否是智能体 | — |
| `tenant_id` | string | 租户标识 | 字段省略 |
| `match_segments` | string[] | 命中的文本片段 | 无命中时为 `[]` |

### 没有分页

不支持分页。`has_more=true` 时应收窄关键词或搜索范围。

### 多条命中怎么选

命中多个机器人时,结合 `description`、`has_chatted` 和 `is_agent` 判断。后续要发消息或拉群时,让用户确认目标,不要直接选择第一条。

```bash
lark-cli contact +search-bot --query '会议助手' \
  --jq '.data.bots[] | select((.description // "") | contains("<功能关键词>"))' --as user
```

## fanout(`--queries`)

输出为 `{bots[], queries[], notice?}`。`has_more` 只出现在每个关键词的结果中。

- `bots[].matched_query`:该结果对应的关键词
- `queries[]`:每个关键词的执行结果,格式为 `{query, error?, has_more, notice?}`
- 部分关键词失败时保留其他结果;全部失败时命令报错
- `--chat-ids` 和 `--has-chatted` 对所有关键词生效
