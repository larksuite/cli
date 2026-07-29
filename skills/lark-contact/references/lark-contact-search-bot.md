# +search-bot

按关键词搜索当前用户可见的机器人。仅支持 user 身份,需要 `search:bot` 权限。

- ✅ 已知机器人名字想找出它的 open_id
- ✅ 一次解析多个名字(`--queries`)
- ✅ 某个机器人在租户级搜不到、但知道它在哪个群(`--chat-ids`)

## flag

**关键词必填**:`--query` 或 `--queries` 至少给一个。另外两个 flag 只能配合关键词,不能独立枚举。传错会得到 typed validation error,`param` / `params` 会点名该改哪个 flag。

| Flag | 说明 |
|---|---|
| `--query <text>` | 关键词,≤ 50 字符(按字符计,不是字节) |
| `--queries <csv>` | 多个关键词并行搜。先去空白、丢空项、精确去重(大小写敏感)、保留首次出现顺序,**再**按去重结果算上限:≤ 20 个词,每词 ≤ 50 字符。与 `--query` 互斥,输出结构也不同(见 fanout) |
| `--chat-ids <csv>` | 改到**这些群内**去搜,而不是在租户范围里搜(见下)。接受裸 `oc_...` 或含 `oc_...` 的群链接,链接先归一化再去重,**≤ 100 个去重后的 chat_id** |
| `--has-chatted` | 只要和自己有单聊会话的。显式传 `=false` 会报错 —— 不传就等于不过滤 |
| `--page-size <n>` | 每次返回条数,1–30(服务端上限就是 30) |

```bash
lark-cli contact +search-bot --query '会议助手' --as user
lark-cli contact +search-bot --query '助手' --has-chatted --as user
lark-cli contact +search-bot --queries '会议助手,日报助手,审批助手' --as user
```

### 搜不到某个机器人?把它所在的群 ID 传进来

不带 `--chat-ids` 时,搜的范围是"当前用户在**整个租户**里可见的机器人"。有些机器人不在这个范围里,**只在某个群内可见**:

```bash
lark-cli contact +search-bot --query '<机器人名>' --chat-ids oc_xxx --as user
```

## 输出

`data.bots[]` 每个机器人一条,顶层还有 `has_more`(bool) 和 `notice`(string,可选)。

| 字段 | 类型 | 说明 | 空值时 |
|---|---|---|---|
| `open_id` | string | `ou_` 开头,机器人的稳定标识 | 始终非空 |
| `name` | string | 从 `display_info` 第一行解析;解析不出兜底为 `open_id` | 始终非空 |
| `description` | string | 一句话简介(`display_info` 第二行)。**同名机器人主要靠它区分** | **整个字段消失** |
| `p2p_chat_id` | string | 与调用者的单聊会话,可喂给接受 `--chat-id` 的 IM 命令 | **字段仍在,值为空串** |
| `has_chatted` | bool | `p2p_chat_id != ""` 的派生字段 | — |
| `enable_join_group` | bool | 是否允许被拉进群聊 | — |
| `is_agent` | bool | 是否是智能体 | — |
| `tenant_id` | string | 租户标识 | **整个字段消失** |
| `match_segments` | string[] | 命中的关键词片段,供高亮 | 无命中是 `[]`,不是 null |

三种空值语义不同,下游要分开处理:字段消失、空串、空数组。

### 没有分页

和 `+search-user` 一致:没有 `--page-token`,也不返回 `page_token`。`has_more=true` 就收窄条件重搜,不是翻页。

### 多条命中怎么选

一个业务领域的关键词往往命中十来个名字高度相似的机器人(同词根 + 不同修饰),光看名字分不出来。后续操作有副作用(拉群、发消息)时,把候选列给用户挑,**不要擅自选第一条**。

按可信度排序的筛选信号:`description`(最能区分功能,名字相近时几乎只能靠它)> `has_chatted`(你用过的那个)> `is_agent` > `enable_join_group`。

```bash
lark-cli contact +search-bot --query '会议助手' \
  --jq '.data.bots[] | select(.description | contains("<功能关键词>"))' --as user
```

## fanout(`--queries`)

顶层变成 `{bots[], queries[], notice?}` —— **没有顶层 `has_more`**,它挪到逐词里。

- `bots[]` 每条多一个 `matched_query`,标明来自哪个词
- `queries[]` 按去重后的输入顺序,每词一条 `{query, error?, has_more, notice?}`
- 个别词失败**不影响其他词**,命令仍成功退出,原因在该词的 `error` 里;**全部失败才报错**,且首个失败的分类(HTTP 状态 / API code)会透传
- `--chat-ids` / `--has-chatted` 作用到每一个词上
- 并发上限 5

