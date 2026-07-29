# +search-bot

仅支持 user 身份,需要 `search:bot` 权限。

## 适用范围

- ✅ 已知机器人名字想找出它的 open_id
- ✅ 一次解析多个机器人名字(`--queries`,去重后最多 20 个词)
- ✅ 想知道某个群里有哪些机器人(`--chat-ids`,这是唯一能找到群内机器人的办法,见下)
- ✅ 想知道自己和哪些机器人聊过天 —— 但要配关键词
- ❌ 不给关键词、只想列出全部可见机器人 → 接口不支持,见下文
- ❌ 已知 open_id 想给机器人发消息 → 直接走 `lark-im`,不经过本命令

## 关键 flag

**必须给关键词**:`--query` 或 `--queries` 至少一个。两个 filter 都不能独立枚举 —— 服务端对纯 filter 请求返回**空列表而不是报错**,所以 CLI 提前拦住,避免"没有这种机器人"的假结论。

| Flag | 作用 |
|---|---|
| `--query <text>` | 关键词,≤ 50 字符(按字符计,不是字节) |
| `--queries <csv>` | 多个关键词并行搜,**去重后最多 20 个词**,每词 ≤ 50 字符;与 `--query` 互斥;输出 shape 不同(见下) |
| `--chat-ids <csv>` | **改变搜索范围**到这些群内,不是收窄(见下);**最多 100 个去重后的 chat_id** |
| `--has-chatted` | 收窄到和自己有单聊会话的;显式传 `=false` 会报错 —— 不传等于不过滤 |
| `--page-size <n>` | 每次返回条数,1–30(服务端上限就是 30) |

### `--chat-ids` 是换范围,不是过滤

**群内的机器人在租户级搜索里可能完全不可见,只有指定群才搜得到。** 所以 `--chat-ids` 的结果不是无过滤结果的子集,它能让你找到 `--query` 单独怎么都搜不出来的机器人。实测过的两种情形:

- 某个关键词在租户级搜索返回 **0 条**,把该机器人所在的群传给 `--chat-ids` 后就能搜到
- 同一个关键词,加 `--chat-ids` 前后返回的是**两组互不相交的结果**,而不是子集

对照 `--has-chatted` 才是真收窄:同一个关键词加上它之后结果条数下降,且剩下的 open_id 是原结果里的。

推论:**搜不到某个机器人时,先想想它是不是只存在于某个群里**,把群 ID 传进来再搜,而不是换关键词。

### 关键词匹配不是子串匹配

服务端的匹配规则未公开,黑盒实测能确定的是**它不是简单子串**:一个完整名字能匹配到中间多插了一个字的机器人,但反过来拿该名字的**后半段**去搜就匹配不到同一个机器人。所以搜不中时优先换更完整的名字,而不是截更短的片段。

### 输入归一化规则

- `--queries`:去首尾空白 → 丢弃空项 → 大小写敏感的精确去重 → 保留首次出现顺序。**20 个上限是按去重后的数量算的**,`'助手,助手,助手'` 只发一次请求,21 项里有 1 项重复也能通过。
- `--chat-ids`:接受裸 `oc_...`,也接受包含 `oc_...` 的飞书 / Lark 群链接 —— 链接会先归一化成裸 chat_id,**再**按 chat_id 去重,**最后**才检查 100 上限。所以同一个群写成链接和裸 ID 各传一遍只算一个,101 个相同 ID 也能通过。

## 常用例子

```bash
# 按名字找,拿 open_id
lark-cli contact +search-bot --query '会议助手' --as user

# 找某个群里的机器人(租户级搜不到它时的唯一办法)
lark-cli contact +search-bot --query '助' --chat-ids oc_xxx --as user

# 只要聊过天的
lark-cli contact +search-bot --query '助手' --has-chatted --as user

# 一次解析多个名字
lark-cli contact +search-bot --queries '会议助手,日报助手,审批助手' --as user
```

会被拒绝的写法(都是 typed validation error,`type: validation` / `subtype: invalid_argument`,退出码 2):

| 写法 | 错误信封点名 |
|---|---|
| 只给 filter 不给关键词,如 `--has-chatted` 单独用 | `params: ["--query", "--queries"]` |
| `--query` 与 `--queries` 同传 | `params: ["--query", "--queries"]`,message 说 mutually exclusive |
| 去重后超过 20 个词 | `param: "--queries"`,`must be at most 20 entries (got N)` |
| `--queries ',,,'` 解析不出词 | `param: "--queries"`,`no valid query parsed from ",,,"` |
| `--query` 超过 50 字符 | `param: "--query"` |
| `--has-chatted=false` 显式传 | `param: "--has-chatted"` |
| `--chat-ids` 里有非 `oc_` 开头的值 | `param: "--chat-ids"` |

## 批量并行查询 (fanout)

```bash
lark-cli contact +search-bot --queries '会议助手,日报,审批' --as user
```

- 每行 bot 带 `matched_query`,标识来自哪个词
- `queries[]` 每个去重后的词一条 `{query, error?, has_more, notice?}`
- 个别词失败不影响其他词,命令仍成功退出;**全部失败才报错**,且首个失败的分类(HTTP 状态 / API code)会透传
- `--chat-ids` / `--has-chatted` 会作用到每一个词上
- 并发上限 5;实测 20 个词约 4 秒

约束:去重后最多 20 个词、每词 ≤ 50 字符、全空 csv(`,,,`)报错。

## 多条命中怎么选

机器人重名比人更严重:一个业务领域的关键词往往命中十来个名字高度相似的机器人(同一个词根 + 不同修饰),光看名字分不出该用哪个。

后续操作若有副作用(拉群、发消息等),把候选列给用户挑,**不要擅自选第一条**。

筛选信号(可信度从高到低):

1. `description` —— 机器人的一句话简介,最能区分功能。名字相近时几乎只能靠它
2. `has_chatted` —— 你用过的那个,通常就是要找的
3. `is_agent` —— 区分智能体和普通推送机器人
4. `enable_join_group` —— 只在需要把它拉进群时才有筛选价值(注意它是空承诺,见下)

```bash
# 按简介精筛
lark-cli contact +search-bot --query '会议助手' \
  --jq '.data.bots[] | select(.description | contains("<功能关键词>"))' --as user
```

## 没有分页

和 `+search-user` 一致:没有 `--page-token`,输出也不给 `page_token`。`has_more=true` 时用 `--has-chatted` 或更具体的关键词收窄,而不是翻页 —— 注意 `--chat-ids` 起不到收窄作用。

## 注意事项

- **`enable_join_group=true` 不等于你能把它拉进群。** 加机器人进群需要应用的 `cli_` 开头 app_id,本命令不返回;拿这里的 `ou_` open_id 去调加群接口,服务端会把它放进 `invalid_id_list`,而且没有 open_id → app_id 的查询接口。看到这个字段为真时不要声称已加入成功。
- **`meta_data.chat_id` 的官方文档写错了。** 文档说是"机器人所属的群聊 ID",实际是**调用者与该机器人的单聊会话**。本命令按后者投影成 `p2p_chat_id`。
- **ID 类型只出 `open_id`。** 接口本身支持 `user_id_type`(实测 `union_id` 会返回 `on_...`),但本命令有固定输出结构,字段名会随之说谎,所以不暴露该参数;确实要 union_id / user_id 时走 `lark-cli api` 直调。
- **`notice` 一定会到达调用方,但位置随格式变。** 服务端用它说明本次搜索的额外情况(如结果不全、query 被截断)。`json` 放在信封的 `data.notice`;其余格式的 stdout 只有数据行,所以 notice 写到 **stderr**(`notice: ...`),保证 stdout 仍可直接进管道。扇出模式下逐词的 notice 和失败原因同样会逐行写 stderr(`notice: "词" — ...` / `failed: "词" — ...`)。

## 输出字段 contract

`data.bots[]` 每个机器人一条:

| 字段 | 类型 | 说明 | 空值行为 |
|---|---|---|---|
| `open_id` | string | `ou_` 开头的机器人 open_id,稳定标识 | 始终非空 |
| `name` | string | 从 `display_info` 第一行解析;解析不出时兜底为 `open_id` | 始终非空 |
| `description` | string (optional) | 从 `display_info` 第二行解析的一句话简介;**同名机器人主要靠它区分** | 空时**字段不出现** |
| `p2p_chat_id` | string | 与调用者的单聊会话(`oc_...`),可作为接受 `--chat-id` 的 IM 命令的输入 | 空时**字段仍在,值为空串** |
| `has_chatted` | bool | `p2p_chat_id != ""` 的派生字段 | — |
| `enable_join_group` | bool | 是否允许被拉进群聊(但你拉不动,见上) | — |
| `is_agent` | bool | 是否是智能体 | — |
| `tenant_id` | string (optional) | 租户标识 | 空时**字段不出现** |
| `match_segments` | string[] | 关键词命中的字符串片段,用于高亮展示 | 无命中时是 `[]`,不是 null |

顶层:`has_more`(bool)、`notice`(string, optional,空时不出现)。

**三种空值语义各不相同**,下游要分别处理:`description` / `tenant_id` 会整个消失,`p2p_chat_id` 保留空串,`match_segments` 保留空数组。

### `--queries` 模式额外字段

顶层 shape 变成 `{bots[], queries[], notice?}` —— **没有顶层 `has_more`**。

`data.bots[]` 每条多一个字段:

| 字段 | 类型 | 说明 |
|---|---|---|
| `matched_query` | string | 这一行是被哪个关键词命中的 |

`data.queries[]` 按去重后的输入顺序,每个词一条:

| 字段 | 类型 | 说明 |
|---|---|---|
| `query` | string | 关键词原文 |
| `error` | string (optional) | 该词失败的原因;成功时不出现 |
| `has_more` | bool | 该词是否还有更多命中 |
| `notice` | string (optional) | 该词的服务端补充说明 |

### 各 `--format` 的差异

| format | stdout | notice / 逐词失败 | 分页提示 | 扇出计数 |
|---|---|---|---|---|
| `json` | 完整信封,含 `notice` 与 `queries[]` | 在 stdout 信封里 | 无 | 无 |
| `ndjson` | 每行一条记录,无信封 | 写 stderr | 无 | 无 |
| `csv` | 完整结果字段 | 写 stderr | 无 | 写 stderr |
| `table` | 完整结果字段 | 写 stderr | 写 stderr | 写 stderr |
| `pretty` | 摘要表:单关键词 6 列;`--queries` 模式多一列 `matched_query`,共 7 列 | 写 stderr | 写 stderr | 写 stderr |

**stdout 在任何格式下都只有数据**,提示类信息一律走 stderr,所以管道安全。扇出计数那行形如 `2 queries, 3 total bots; 1 failed, 0 with has_more`。
