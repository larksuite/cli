# contact +search-user

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

按关键词与多维 filter 搜索员工，返回带多语种姓名、联系方式、聊天关系上下文与命中片段的用户列表，结果按亲密度排序。只读操作，不修改通讯录数据。

本 skill 对应 shortcut：`lark-cli contact +search-user`（调用 `POST /open-apis/contact/v3/users/search`）。

## 典型触发表达

以下说法通常应优先使用 `contact +search-user`：

- 搜一下叫「张三」的同事
- 找一个我聊过天的「李海」
- 列出和我聊过天的离职同事
- 找有企业邮箱的「王」姓同事
- 把这一串 open_id 批量取一下姓名 / 邮箱 / 是否激活
- 查一下「张三」的英文名 / 日文名

## 命令

```bash
# 关键词搜索
lark-cli contact +search-user --query "张三"

# 关键词 + 只搜聊过天的
lark-cli contact +search-user --query "张三" --has-chatted

# 关键词 + 只搜同租户（排除外部联系人）
lark-cli contact +search-user --query "张三" --exclude-outer-contact

# 关键词 + 只搜已离职且聊过天的
lark-cli contact +search-user --query "张三" --is-resigned

# 关键词 + 只搜有企业邮箱的
lark-cli contact +search-user --query "张三" --has-enterprise-email

# 多 filter 组合
lark-cli contact +search-user --query "张三" --has-chatted --exclude-outer-contact

# 取自己的资料
lark-cli contact +search-user --user-ids me

# 按 open_id 批量回填资料
lark-cli contact +search-user --user-ids "ou_a,ou_b,ou_c"

# 关键词 + 限定在指定候选 open_id 集合内验证
lark-cli contact +search-user --query "张三" --user-ids "ou_a,ou_b,me"

# 指定姓名 locale（默认按 tenant brand：feishu→zh_cn，lark→en_us）
lark-cli contact +search-user --query "张三" --lang en_us

# 自动翻页（最多 --page-limit 页，默认 20，上限 40）
lark-cli contact +search-user --query "张三" --page-all

# 自动翻页并限定最多 5 页
lark-cli contact +search-user --query "张三" --page-limit 5

# 自定义单页大小（最大 30）
lark-cli contact +search-user --query "张三" --page-size 30 --page-all

# 排查请求体
lark-cli contact +search-user --query "张三" --has-chatted --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|---|---|---|
| `--query <text>` | 否¹ | 搜索关键词，最大 64 rune（与客户端大搜框对齐） |
| `--user-ids <ids>` | 否¹ | open_id 列表，逗号分隔；支持 `me` 表示当前用户；最多 100 个 |
| `--is-resigned` | 否¹² | 仅搜「离职且聊过天」的用户 |
| `--has-chatted` | 否¹² | 仅搜聊过天的用户（flag 名跟输出字段 `has_chatted` 对齐） |
| `--exclude-outer-contact` | 否¹² | 仅搜同租户用户（排除外部联系人） |
| `--has-enterprise-email` | 否¹² | 仅搜有企业邮箱的用户 |
| `--lang <locale>` | 否 | 覆盖输出 `name` 的语种（如 `zh_cn` / `en_us` / `ja_jp`），默认按 tenant brand |
| `--page-size <n>` | 否 | 每页数量，默认 `20`，最大 `30` |
| `--page-all` | 否³ | 自动翻页直到 `has_more=false` 或到达 `--page-limit` |
| `--page-limit <n>` | 否³ | 自动翻页最多翻几页，默认 `20`，最大 `40`；**单独传此 flag 隐式启用 `--page-all`** |
| `--dry-run` | 否 | 预览 API 请求，不真正调用 |

¹ 至少需要提供 `--query` / `--user-ids` / 4 个 bool filter 中的**任意一项**。
² Bool filter 是 **opt-in**：传 `--has-chatted` 等价 `=true` 触发该过滤；不传或 `=false` 一律视为不施加该 filter（API 不支持反向 filter）。
³ 不传 `--page-all` 与 `--page-limit` 时 CLI 只请求一页；需要更多结果请收敛 filter 或启用自动翻页。

## 核心约束

### 1. 至少一个搜索输入

`--query` / `--user-ids` / 4 个 bool filter 至少一项非空，否则 CLI 返回 validation error `specify at least one of ...`。

### 2. 仅支持 user 身份

需要 `lark-cli auth login --as user` 完成授权，并具备 `contact:user:search` 权限。

### 3. `me` 表示当前用户

`--user-ids` 中可使用 `me`，CLI 本地解析为当前登录用户的 `open_id`，无需手动先查询自己的用户 ID。

### 4. Bool filter 是 opt-in

4 个 bool filter（`--is-resigned` / `--has-chatted` / `--exclude-outer-contact` / `--has-enterprise-email`）只有 `=true` 触发实际 filter；`=false` 与不传等价（API 限制，不支持反向过滤）。

### 5. 跨租户用户字段可能为空

当结果中 `is_cross_tenant=true` 时，`email` / `enterprise_email` / `display_info` 中的部门路径段可能为空（跟随飞书 profile 页可见性规则）。消费端不应假设"搜到结果就一定有联系方式"，应做空值兜底。

## 输出结果

`data.users[]` 每条对象 12 个字段：

| 字段 | 类型 | 用途 |
|---|---|---|
| `name` | string | 按 locale 挑选的单一显示名（默认按 tenant brand，可被 `--lang` 覆盖） |
| `i18n_names` | map<string,string> | 完整多语种姓名 map（`zh_cn` / `en_us` / `ja_jp` 等），双语场景直接读取 |
| `open_id` | string | 用户 open_id，可作为 `chat-id` 之外的标识传给其他命令 |
| `email` | string | 联系邮箱；跨租户用户可能为空 |
| `enterprise_email` | string | 企业邮箱；跨租户用户可能为空 |
| `is_registered` | bool | 是否已激活飞书账号；`false` 表示无法接收消息 |
| `chat_id` | string | 与该用户的单聊 chat_id；**仅在曾经聊过才有值**，可用于 chat-scoped 操作（如 `im +messages-list --chat-id` 查历史）；发新消息用 `im +messages-send --user-id <open_id>` 即可，不依赖此字段 |
| `has_chatted` | bool | `chat_id != ""` 的派生 bool，prompt 里直接条件判断 |
| `is_cross_tenant` | bool | 是否跨租户（外部联系人） |
| `tenant_id` | string | 用户所属租户 ID |
| `description` | string | 用户签名（自填，常含职位 / 团队信息） |
| `display_info` | string | 后端聚合的展示串：`<h>命中关键词</h>` + 部门路径 + `[Contacted N days ago]`（仅曾联系过才有），用于 disambiguation |

顶层还有 `has_more` 指示是否有下一页。

## 翻页模式

默认只请求一页。返回 `has_more=true` 时,推荐先**收敛 filter**;确实要拉多页再启用自动翻页。

| 传的 flag | 模式 | 行为 |
|---|---|---|
| (都不传) | 单页 | 返 1 页;`has_more=true` 时 stderr 提示收敛 filter 或用 `--page-all` |
| `--page-all` | 自动(上限 40) | 自动循环直到 `has_more=false` 或达到 40 页 |
| `--page-limit 5` | 自动(上限 5) | 隐式启用 auto,最多 5 页 |
| `--page-all --page-limit 5` | 自动(上限 5) | 跟只传 `--page-limit 5` 等价 |

到达 `--page-limit` 仍有 `has_more` 时,stderr 会出:
```text
warning: stopped after fetching N page(s); refine the query with more filters, or raise --page-limit (max 40)
```

`--page-size` 控制单页大小(1-30,默认 20),跟自动 / 手动模式正交。

## 搜索结果中的下一步

- **发消息**：直接用 `open_id` 调 `im +messages-send --user-id <open_id>` —— p2p 聊天 API 自动解析到单聊会话，不需要先查 chat。
- **查历史聊天**：当 `has_chatted=true` 时 `chat_id` 是已存在的单聊 ID，可用于 `im +messages-list --chat-id <chat_id>` 等 chat-scoped 操作（**新发消息不需要这个字段**，`--user-id` 就够）。
- **查更多个人资料**：拿 `open_id` 调 `contact +get-user --user-id <open_id>` 获取完整 profile（含部门 ID、union_id 等）。
- **多语种叫法**：直接读 `i18n_names.<locale>`，不必再发请求。

```bash
# 一步到位：搜「张三」中第一个聊过天的对象，给 ta 发消息
OPEN_ID=$(lark-cli contact +search-user --query "张三" --has-chatted --jq '.data.users[0].open_id')
lark-cli im +messages-send --user-id "$OPEN_ID" --text "Hi!"
```

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---|---|---|
| `specify at least one of --query, --user-ids, ...` | 6 个搜索输入全空 | 至少补一个：`--query` 或某个 filter 或 `--user-ids` |
| `--query: length must be between 1 and 64 characters` | 关键词超 64 rune | 缩短到 64 rune 以内（CJK 按字符数计） |
| `--user-ids: must be at most 100 entries` | 列表超 100 个 | 拆批查询，每批 ≤ 100 |
| `invalid user ID format, should start with 'ou_'` | `--user-ids` 里有非 `ou_` 前缀的项 | 改为 `ou_xxx` 或 `me` |
| `"me" requires a logged-in user with a resolvable open_id` | 没登录 | 先 `lark-cli auth login --as user` |
| `--page-size N: must be between 1 and 30` | 分页大小越界 | 1 ≤ N ≤ 30 |
| `--has-chatted=false` 没有过滤效果 | API 不支持反向 filter，`=false` 等同未传 | 用 `--has-chatted`（=true）后客户端按 `has_chatted` 过滤 |
| `--page-limit N` 传了 `N > 40` | 超过 CLI 上限 | CLI 拒掉；1 ≤ N ≤ 40 |
| 跨租户结果 `email` 是空字符串 | 飞书 profile 页可见性规则 | 不是 bug，消费端做空值兜底 |

## 提示

- 默认 `--format json`,便于 jq / 解析;排查请求结构时优先 `--dry-run`。
- "我聊过天的同事"：用 `--has-chatted` filter 而非客户端过滤(少一次大数据传输 + 排序更优)。
- 多语种姓名：`name` 已按 locale 挑好,需要其他语种直接读 `i18n_names.<locale>`。
- 同名 disambiguation：参考 `display_info`(含部门路径 + 最近联系提示)选择正确目标。

## 参考

- [lark-contact-get-user](lark-contact-get-user.md) — 用 `open_id` 查完整 profile
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
- [lark-im](../../lark-im/SKILL.md) — 拿到 `open_id` 或 `chat_id` 后用 im
