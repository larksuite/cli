# +search-user

## 关键 flag

`--query` / `--queries` / `--user-ids` / bool filter 至少传一个。bool filter 显式传 `=false` 会报错——不传等于不过滤。

| Flag | 作用 |
|---|---|
| `--query <text>` | 关键词(姓名 / 邮箱 / 手机号),≤ 50 rune |
| `--queries <csv>` | 多个关键词并行搜,**最多 20 条**;与 `--query` / `--user-ids` 互斥 |
| `--user-ids <csv>` | open_id 列表,≤ 100;支持 `me` 表示自己;与 `--query` 同传时把搜索范围限定在该集合 |
| `--lang <locale>` | 覆盖 `localized_name` 的语种(如 `zh_cn` / `en_us` / `ja_jp`) |
| `--has-chatted` / `--has-enterprise-email` / `--exclude-external-users` / `--left-organization` | 各自维度的筛选。**都不是默认写法**——加上就会把不满足该维度的人整批筛掉 |
| `--page-size <n>` | 单页大小 1-30,默认 20 |

## 批量并行查询 (fanout)

一次查多个名字,比逐个 `--query` 少很多往返:

```bash
lark-cli contact +search-user --queries "Alice,Bob,张三"
```

- 每行 user 带 `matched_query`,标识来自哪个 query;`queries[]` 按输入顺序每个一条 `{query, error?, has_more}`
- 部分失败不影响其它 query;全部失败才 exit 非 0
- bool filter 对每个 query 都生效;重复条目静默去重,全空 csv (`,,,`) 报错

## 命令输出里看不出来的语义

- **同名 disambiguation 的筛选信号**(可信度从高到低):`chat_recency_hint`(近期联系过) > `enterprise_email` 前缀 > `department` 关键词。`localized_name` 同名时无区分作用。
- **`--lang` 只影响输出展示名**(`localized_name` 就是按 `--lang` / brand 选出的展示名,兜底为 open_id),不影响匹配字段。
- **`is_activated`**:是否已激活飞书账号。未激活也可投递消息,但用户可能看不到。
- **`is_cross_tenant=true`**(外部联系人)的业务字段可能是空字符串,需做空值兜底。
- **`department`**:部门路径,服务端可能用 `-` 拼层级,层级数不固定,**按可子串匹配的字符串处理**。
- **`p2p_chat_id`**:与当前用户的 P2P 会话 ID(`oc_...`),空表示从未私聊过;可作为接受 `--chat-id` 的 IM 命令的输入。`has_chatted` 是它的派生字段。
- **`signature`**(个性签名)为空时字段不出现。
- **不会自动翻页**。`has_more=true` 表示需要 refine query。
- **`--query` 与 `--user-ids` 同时设**:`--user-ids` 限定搜索范围,`--query` 在该集合内匹配。
