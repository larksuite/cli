# +search-user

仅 user 身份。需要 scope `contact:user:search`。

## 适用范围

- ✅ 已知姓名 / 邮箱 / 「聊过的人」想找出 open_id
- ✅ 已知一组 open_id 想批量校验或回填字段(`--user-ids`,最多 100,支持 `me`)
- ✅ 按聊天关系 / 在职状态 / 租户边界 / 企业邮箱等维度筛选员工
- ❌ 已知 open_id 想拿完整 profile → 用 `+get-user --as bot`
- ❌ 已知 open_id 想发消息 → 直接走 `lark-im`,不经过本命令

## 关键 flag

`--query` / `--user-ids` / 4 个 bool filter 至少传一个,否则报错。完整 flag 看 `lark-cli contact +search-user --help`。

| Flag | 作用 |
|---|---|
| `--query <text>` | 关键词(姓名 / 邮箱 / 手机号),≤ 64 rune |
| `--user-ids <csv>` | open_id 列表,逗号分隔,≤ 100;支持 `me` 表示自己;与 `--query` 同传时把搜索范围限定在该集合 |
| `--has-chatted` | 仅搜聊过天的(opt-in;**显式 `=false` 会被拒**,下同) |
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

## 同名 disambiguation

搜常见姓名常返回多条同名结果。后续操作若有副作用(发消息、邀请会议等),把候选列给用户挑;**不要擅自选**。

筛选信号(可信度从高到低):`chat_recency_hint`(近期联系过) > `enterprise_email` 前缀 > `department` 关键词。`localized_name` 同名时无区分作用。

```bash
# 用 jq 按部门精筛
lark-cli contact +search-user --query "张三" \
  --jq '.data.users[] | select(.department | contains("<部门关键词>"))'
```

## 注意事项

- **不会自动翻页**。`has_more=true` 表示要 refine query,不是叫你翻页。
- **bool filter 显式传 `=false` 会报错**:不传等于不过滤;启用就传 flag(不带值)。
- **`--lang` 只影响输出展示名**,不影响匹配字段。
- **`--query` 与 `--user-ids` 同时设**:`--user-ids` 进 `filter.user_ids`(限定搜索范围),`--query` 进顶层(关键字),按服务端 filter 语义在该 ID 集合内匹配;请求结构可 `--dry-run` 确认。

## 输出字段 contract

`data.users[]` 的字段集合稳定,可直接 jq / 反序列化。跨租户用户(`is_cross_tenant=true`)按飞书可见性规则,业务字段可能为空字符串 —— 下游做空值兜底,不要当成"字段缺失"。

| 字段 | 类型 | 说明 | 跨租户 |
|---|---|---|---|
| `open_id` | string | 稳定标识,后续命令以此为准 | 始终非空 |
| `localized_name` | string | 按 `--lang` / brand 选出的展示名;想换语言重查时传 `--lang en_us` 等 | 始终非空(兜底为 open_id) |
| `email` | string | 个人邮箱 | 可能为空 |
| `enterprise_email` | string | 企业邮箱 | 可能为空 |
| `is_activated` | bool | 是否已激活飞书账号(未激活也可投递消息,但用户可能看不到) | 可能 false |
| `is_cross_tenant` | bool | 是否跨租户用户(同公司=false,外部联系人=true) | — |
| `p2p_chat_id` | string | 与当前用户的现有 P2P 会话 ID(`oc_...`);空表示从未私聊过。可作为任何接受 `--chat-id` 的 IM 命令的输入 | 可能为空 |
| `has_chatted` | bool | `p2p_chat_id != ""` 的派生字段 | — |
| `department` | string | 部门路径,服务端可能用 `-` 拼层级,层级数不固定。**按可子串匹配的字符串处理** | 可能为空 |
| `signature` | string | 用户个性签名(API 原名 `description`,本 CLI 重命名以反映真实语义)。同名 disambiguation 时可作为辅助信号 | 可能为空 |
| `chat_recency_hint` | string | 最近联系提示文案,如 `"Contacted 2 days ago"`;空表示无近期联系 | 可能为空 |
| `match_segments` | string[] | 关键词命中的字符串片段,用于高亮展示;无命中则为空数组 | — |

表中字段即本 shortcut 的输出契约,移除或改名按 breaking change 处理。
