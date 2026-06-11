# +search-user

仅 user 身份。需要 scope `contact:user:search`。

## 适用范围

- ✅ 已知姓名 / 邮箱 / 「聊过的人」想找出 open_id
- ✅ 已知一组 open_id 想批量校验或回填字段(`--user-ids`,最多 100,支持 `me`)
- ✅ 按聊天关系 / 在职状态 / 租户边界 / 企业邮箱等维度筛选员工
- ❌ 已知 open_id 想拿完整 profile → 用 `+get-user --as bot`
- ❌ 已知 open_id 想发消息 → 直接走 `lark-im`,不经过本命令

## 关键 flag

`--query` / `--queries` / `--user-ids` / bool filter 至少传一个。bool filter 显式传 `=false` 会报错——不传等于不过滤。

| Flag | 作用 |
|---|---|
| `--query <text>` | 关键词(姓名 / 邮箱 / 手机号),≤ 50 rune |
| `--queries <csv>` | 多个关键词并行搜,**最多 20 条**;与 `--query` / `--user-ids` 互斥;输出新 shape(见下) |
| `--user-ids <csv>` | open_id 列表,≤ 100;支持 `me` 表示自己;与 `--query` 同传时把搜索范围限定在该集合 |
| `--has-chatted` | 仅搜聊过天的 |
| `--has-enterprise-email` | 仅搜有企业邮箱的 |
| `--exclude-external-users` | 仅搜同租户(排除外部联系人) |
| `--left-organization` | 仅搜已离职的 |
| `--lang <locale>` | 覆盖 `localized_name` 的语种(如 `zh_cn` / `en_us` / `ja_jp`) |
| `--page-size <n>` | 单页大小 1-30,默认 20 |

## 常用例子

```bash
# 按姓名搜,看候选确认是哪个张三
lark-cli contact +search-user --query "张三" --has-chatted

# 按完整邮箱搜(命中通常唯一,适合作后续命令的输入)
lark-cli contact +search-user --query "alice@example.com"

# 查看自己
lark-cli contact +search-user --user-ids me

# 批量回填:已知一组 open_id,取姓名 / 邮箱 / 部门
lark-cli contact +search-user --user-ids "ou_a,ou_b,ou_c" --format json

# 多 filter 组合:同租户的、有企业邮箱的「王」姓员工
lark-cli contact +search-user --query "王" --exclude-external-users --has-enterprise-email

# filter-only 枚举:列出所有"聊过天的离职同事"(无关键词)
lark-cli contact +search-user --has-chatted --left-organization
```

## 批量并行查询 (fanout)

一次查多个名字:

```bash
lark-cli contact +search-user --queries "Alice,Bob,张三"
```

- 每行 user 带 `matched_query`,标识来自哪个 query
- `queries[]` 每个输入一条 `{query, error?, has_more}`,失败的有 `error`
- 部分失败不影响其它 query;全部失败才 exit 非 0

```bash
# bool filter 对每个 query 都生效
lark-cli contact +search-user --queries "Alice,Bob" --has-chatted

# 与 --query / --user-ids 互斥
lark-cli contact +search-user --queries "a" --query "b"   # ❌ exit 2
```

约束:
- 最多 20 条; 每条 ≤ 50 字符
- 重复条目静默去重;全空 csv (`,,,`) 报错

## 同名 disambiguation

搜常见姓名常返回多条同名结果。后续操作若有副作用(发消息、邀请会议等),把候选列给用户挑;**不要擅自选**。

筛选信号(可信度从高到低):`chat_recency_hint`(近期联系过) > `enterprise_email` 前缀 > `department` 关键词。`localized_name` 同名时无区分作用。

```bash
# 用 jq 按部门精筛
lark-cli contact +search-user --query "张三" \
  --jq '.data.users[] | select(.department | contains("<部门关键词>"))'
```

## 注意事项

- **不会自动翻页**。`has_more=true` 表示需要 refine query。
- **`--lang` 只影响输出展示名**,不影响匹配字段。
- **`--query` 与 `--user-ids` 同时设**:`--user-ids` 限定搜索范围,`--query` 在该集合内匹配。

## 输出字段 contract

跨租户用户(`is_cross_tenant=true`)的业务字段可能为空字符串,需做空值兜底。

| 字段 | 类型 | 说明 | 跨租户 |
|---|---|---|---|
| `open_id` | string | 稳定标识,后续命令的输入 | 始终非空 |
| `localized_name` | string | 按 `--lang` / brand 选出的展示名 | 始终非空(兜底为 open_id) |
| `email` | string | 个人邮箱 | 可能为空 |
| `enterprise_email` | string | 企业邮箱 | 可能为空 |
| `is_activated` | bool | 是否已激活飞书账号(未激活也可投递消息,但用户可能看不到) | 可能 false |
| `is_cross_tenant` | bool | 是否跨租户用户(同公司=false,外部联系人=true) | — |
| `p2p_chat_id` | string | 与当前用户的 P2P 会话 ID(`oc_...`);空表示从未私聊过。可作为接受 `--chat-id` 的 IM 命令的输入 | 可能为空 |
| `has_chatted` | bool | `p2p_chat_id != ""` 的派生字段 | — |
| `department` | string | 部门路径,服务端可能用 `-` 拼层级,层级数不固定。**按可子串匹配的字符串处理** | 可能为空 |
| `signature` | string (optional) | 用户个性签名;空时字段不出现 | 可能不出现 |
| `chat_recency_hint` | string | 最近联系的提示文案,仅供展示 | 可能为空 |
| `match_segments` | string[] | 关键词命中的字符串片段,用于高亮展示;无命中则为空数组 | — |

### `--queries` 模式额外字段

`data.users[]` 每条多 `matched_query` (string),指明本行来自哪个 query。

`data.queries[]` 按输入顺序、dedup 后每个 query 一条:

| 字段 | 类型 | 说明 |
|---|---|---|
| `query` | string | 该输入 |
| `error` | string (optional) | 失败原因;成功时不出现 |
| `has_more` | bool | 该 query 还有更多结果 |

fanout 模式无顶层 `data.has_more`。
