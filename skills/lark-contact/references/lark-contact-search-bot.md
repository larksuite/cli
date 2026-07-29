# +search-bot

仅支持 user 身份,需要 `search:bot` 权限。

## 适用范围

- ✅ 已知机器人名字(或名字片段)想找出它的 open_id
- ✅ 一次解析多个机器人名字(`--queries`,最多 20 个词)
- ✅ 想知道某个群里有哪些机器人、或自己和哪些机器人聊过天 —— 但都要配关键词
- ❌ 不给关键词、只想列出全部可见机器人 → 接口不支持,见下文
- ❌ 已知 open_id 想给机器人发消息 → 直接走 `lark-im`,不经过本命令

## 关键 flag

**必须给关键词**:`--query` 或 `--queries` 至少一个。`--chat-ids` 和 `--has-chatted` 只能收窄关键词搜索,不能独立枚举 —— 服务端对纯 filter 请求返回**空列表而不是报错**,所以 CLI 提前拦住,避免"没有这种机器人"的假结论。

| Flag | 作用 |
|---|---|
| `--query <text>` | 关键词,≤ 50 字符(按字符计,不是字节) |
| `--queries <csv>` | 多个关键词并行搜,**最多 20 个唯一词**,每词 ≤ 50 字符;与 `--query` 互斥;输出 shape 不同(见下) |
| `--chat-ids <csv>` | 只在这些群里找,**最多 100 个去重后的 chat_id** |
| `--has-chatted` | 只要和自己有单聊会话的;显式传 `=false` 会报错 —— 不传等于不过滤 |
| `--page-size <n>` | 每次返回条数,1–30(服务端上限就是 30) |

### 输入归一化规则

- `--queries`:去首尾空白 → 丢弃空项 → 大小写敏感的精确去重 → 保留首次出现顺序。**20 个上限是按去重后的数量算的**,`'助手,助手,助手'` 只发一次请求。
- `--chat-ids`:接受裸 `oc_...`,也接受包含 `oc_...` 的飞书 / Lark 群链接 —— 链接会先归一化成裸 chat_id,**再**按 chat_id 去重,**最后**才检查 100 上限。所以同一个群写成链接和裸 ID 各传一遍,只算一个。

## 常用例子

```bash
# 按名字找,拿 open_id
lark-cli contact +search-bot --query '会议助手' --as user

# 只在某个群里找
lark-cli contact +search-bot --query '助手' --chat-ids oc_xxx --as user

# 只要聊过天的
lark-cli contact +search-bot --query '助手' --has-chatted --as user

# 一次解析多个名字
lark-cli contact +search-bot --queries '会议助手,日报助手,审批助手' --as user
```

## 没有分页

和 `+search-user` 一致:没有 `--page-token`,输出也不给 `page_token`。`has_more=true` 时**收窄搜索**(补 `--chat-ids` / `--has-chatted`,或换更具体的关键词),而不是翻页。

## 注意事项

- **`enable_join_group=true` 不等于你能把它拉进群。** 加机器人进群需要应用的 `cli_` 开头 app_id,本命令不返回;拿这里的 `ou_` open_id 去调加群接口,服务端会把它放进 `invalid_id_list`,而且没有 open_id → app_id 的查询接口。看到这个字段为真时不要声称已加入成功。
- **`meta_data.chat_id` 的官方文档写错了。** 文档说是"机器人所属的群聊 ID",实际是**调用者与该机器人的单聊会话**。本命令按后者投影成 `p2p_chat_id`。
- **ID 类型只出 `open_id`。** 接口本身支持 `user_id_type`(实测 `union_id` 会返回 `on_...`),但本命令有固定输出结构,字段名会随之说谎,所以不暴露该参数;确实要 union_id / user_id 时走 `lark-cli api` 直调。
- **`notice` 原样透出。** 服务端用它说明本次搜索的额外情况(如结果不全),不要忽略。

## 输出字段 contract

单关键词模式:

```
bots[]        每个机器人一条
  open_id           ou_ 开头,机器人的 open_id
  name              从 display_info 第一行解析
  description        从 display_info 第二行解析(可能为空,此时省略该字段)
  p2p_chat_id       与调用者的单聊会话;没有则为空字符串(字段仍存在)
  has_chatted       p2p_chat_id 是否非空
  enable_join_group 是否允许被拉进群聊
  is_agent          是否是智能体
  tenant_id         租户标识(可能省略)
  match_segments[]  命中的关键词片段;无命中时为 []
has_more      还有更多命中
notice        服务端补充说明(可能省略)
```

### `--queries` 模式额外字段

顶层 shape 变成 `{bots[], queries[], notice?}` —— **没有顶层 `has_more`**,`has_more` 只在 `queries[]` 逐词给出。

```
bots[]        比单关键词模式多一个字段:
  matched_query     这一行是被哪个关键词命中的
queries[]     按去重后的输入顺序,每个词一条:
  query             关键词原文
  error             该词失败的原因(成功时省略)
  has_more          该词是否还有更多命中
  notice            该词的服务端补充说明(可能省略)
```

个别词失败不影响其他词,命令仍然成功退出,失败原因在对应的 `queries[].error` 里;**只有全部词都失败才报错**,且首个失败的分类(HTTP 状态 / API code)会透传出来。

### 各 `--format` 的差异

| format | stdout | 扇出计数写 stderr |
|---|---|---|
| `pretty` | 摘要表:单关键词 6 列;`--queries` 模式多一列 `matched_query`,共 7 列 | 是 |
| `table` | 完整结果字段 | 是 |
| `csv` | 完整结果字段 | 是 |
| `json` | 完整信封 | 否 |
| `ndjson` | 每行一条完整记录 | 否 |

扇出计数那行形如 `2 queries, 3 total bots; 1 failed, 0 with has_more`,只在 `pretty` / `table` / `csv` 下输出。用 stdout 做管道时选 `json` 或 `ndjson`,不会混入这行。
