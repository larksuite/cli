# minutes +search

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

搜索妙记列表，支持关键词、所有者、参与者以及时间范围等多条件过滤。所有者与参与者都支持传入多个 open\_id，也支持传入 `me` 表示当前用户。只读操作，不修改任何妙记数据。

本 skill 对应 shortcut：`lark-cli minutes +search`（调用 `POST /open-apis/minutes/v1/minutes/search`）。

## 典型触发表达

以下说法通常应优先使用 `minutes +search`：

- 我的妙记
- 我拥有的妙记
- 我参与的妙记
- 最近的妙记
- 某个关键词的妙记
- 某段时间内的妙记

## 命令

```bash
# 关键词搜索
lark-cli minutes +search --query "预算复盘"

# 查询某一天内的妙记（单日查询时，建议将 start 和 end 都填写为同一天）
lark-cli minutes +search --start 2026-03-10 --end 2026-03-10

# 按时间范围搜索
lark-cli minutes +search --start "2026-03-10T00:00+08:00" --end "2026-03-17T00:00+08:00"
lark-cli minutes +search --start 2026-03-10 --end 2026-03-17

# 关键词 + 时间范围
lark-cli minutes +search --query "预算复盘" --start "2026-03-10T00:00+08:00" --end "2026-03-17T00:00+08:00"
lark-cli minutes +search --query "预算复盘" --start "2026-03-10T00:00+08:00"
lark-cli minutes +search --query "预算复盘" --end "2026-03-17T00:00+08:00"

# 按参与者过滤（open_id，逗号分隔）
lark-cli minutes +search --participant-ids "ou_x,ou_y"

# 按所有者过滤（open_id，逗号分隔）
lark-cli minutes +search --owner-ids "ou_owner,ou_owner_2"

# 严格只查我作为参与者的妙记（不含我拥有）
lark-cli minutes +search --participant-ids "me"

# 查询我拥有的妙记
lark-cli minutes +search --owner-ids "me"

# 广义查询我参与的妙记（自然语言默认：我拥有 ∪ 我参与）
lark-cli minutes +search --owner-ids "me" --start 2026-03-10 --end 2026-03-10
lark-cli minutes +search --participant-ids "me" --start 2026-03-10 --end 2026-03-10
# 然后按 token 去重合并两次结果

# 多条件组合查询
lark-cli minutes +search --owner-ids "ou_owner" --participant-ids "ou_x" --start "2026-03-10T00:00+08:00"

# 分页查询
lark-cli minutes +search --query "预算复盘" --page-size 20
lark-cli minutes +search --query "预算复盘" --page-size 20 --page-token '<PAGE_TOKEN>'

# 输出为结构化 JSON
lark-cli minutes +search --query "预算复盘" --format json
```

## 参数

| 参数                        | 必填 | 说明                                   |
| ------------------------- | -- | ------------------------------------ |
| `--query <text>`          | 否  | 搜索关键词                                |
| `--owner-ids <ids>`       | 否  | 所有者 open\_id 列表，逗号分隔；支持传 `me` 表示当前用户 |
| `--participant-ids <ids>` | 否  | 参与者 open\_id 列表，逗号分隔；支持传 `me` 表示当前用户 |
| `--start <time>`          | 否  | 开始时间（ISO 8601 或仅日期）                  |
| `--end <time>`            | 否  | 结束时间（ISO 8601 或仅日期）                  |
| `--page-size <n>`         | 否  | 每页数量，默认 `15`，最大 `30`                 |
| `--page-token <token>`    | 否  | 下一页分页 token                          |
| `--dry-run`               | 否  | 预览 API 调用，不执行                        |

## 核心约束

### 1. 至少提供一个过滤条件

所有参数均可选，但必须至少提供一个过滤条件：`--query`、`--owner-ids`、`--participant-ids`、`--start` 或 `--end`。

### 2. 仅支持 user 身份

该接口仅支持 `user` 身份，使用前需完成 `lark-cli auth login` 并具备 `minutes:minutes.search:read` 权限。

### 3. `me` 表示当前用户

在 `--owner-ids` 和 `--participant-ids` 中可使用 `me`，表示当前登录用户。该值会在本地解析为当前用户的 `open_id`，无需手动先查询自己的用户 ID。
若当前环境尚未完成用户登录，或 CLI 无法解析出当前用户的 `open_id`，则应先执行 `lark-cli auth login`，再重新执行搜索。

### 4. 自然语言中的“参与的妙记”默认按并集理解

当用户说"我参与的妙记""我参加过的妙记""参与过的妙记"时，默认理解为"我涉及的全部妙记"：

- 我拥有的妙记：`--owner-ids me`
- 我作为参与者的妙记：`--participant-ids me`

不要只跑一次 `--participant-ids me` 就直接下结论，也不要把 `--owner-ids me` 和 `--participant-ids me` 同时塞进一次查询里赌接口语义。应分别查询后，按 `token` 做并集去重。

只有在用户明确说"仅我参与但不是我拥有""别人拥有但我参与""只看参与者身份"时，才只使用 `--participant-ids`。

### 5. 支持分页

当返回 `has_more=true` 时，使用响应中的 `page_token` 配合 `--page-token` 获取下一页结果。

### 6. 日期型 `--end` 包含当天整天

当 `--end` 传入的是仅日期格式（如 `2026-03-10`）时，CLI 会将它解释为当天 `23:59:59`，而不是当天 `00:00:00`。
CLI 会先按输入的本地日历日语义解析，再标准化为 RFC3339 时间戳发给 API；在 dry-run 或排查请求体时，看到的 `Z` 结尾时间表示同一个绝对时间点的 UTC 表示，不改变“按当天整天查询”的语义。

这意味着：

- `--start 2026-03-10 --end 2026-03-10` 表示只查 `2026-03-10` 当天
- `--start 2026-03-10 --end 2026-03-11` 表示查询 `2026-03-10` 和 `2026-03-11` 两天

如果用户说“昨天的妙记”“今天的妙记”“某一天内的妙记”，应把 `--start` 和 `--end` 都设置为同一天，而不是把 `--end` 设成下一天。

### 7. 会议的妙记先定位会议

如果用户明确要找某场会议的妙记，或同时提到“会议 / 开会 / 会”和“妙记”，应优先使用 `vc +search` 先定位会议，再按需通过 `vc +recording` 获取 `minute_token`，不要直接按妙记时间范围或关键词搜索。
只有在无法通过会议搜索定位目标会议，或用户明确要求按妙记维度检索时，才回退到 `minutes +search`。

如果用户要的是"某场会议的妙记信息""某个日程对应的妙记详情""minute\_token""妙记链接""标题""时长""owner"，正确链路是：

1. `vc +search` 或 `calendar +agenda` 先定位会议 / 日程
2. `vc +recording` 获取 `minute_token`
3. `minutes minutes get` 查询妙记基础信息

不要为了查"妙记信息"直接走 `vc +notes --meeting-ids`。`vc +notes` 只适用于逐字稿、总结、待办、章节等纪要内容。

<br />

## 时间格式

`--start` 和 `--end` 支持以下时间格式：

| 格式             | 示例                          | 说明                                 |
| -------------- | --------------------------- | ---------------------------------- |
| ISO 8601（带时区）  | `2026-03-10T14:00:00+08:00` | 推荐                                 |
| ISO 8601（不带时区） | `2026-03-10T14:00:00`       | 按本地时区解析                            |
| 仅日期            | `2026-03-10`                | 按天粒度解析；若用于 `--end`，表示当天 `23:59:59` |

## 输出结果

- 默认输出包含 `items`、`total`、`has_more` 和 `page_token`。

## Pagination (`has_more` / `page_token`)

- 当结果中返回 `has_more=true` 时，说明还有更多页可继续获取。
- 继续翻页时，使用响应中的 `page_token` 搭配 `--page-token` 发起下一次查询。
- 不要假设调大 `--page-size` 就能拿全结果；分页遍历时应以 `has_more` 和 `page_token` 为准。
- `total` 数量小于 50 时，自动分页获取所有结果；`total` 数量大于 50 时，向用户确认是否获取全部结果。

```bash
# First page
lark-cli minutes +search --query "预算复盘" --page-size 20

# Next page
lark-cli minutes +search --query "预算复盘" --page-size 20 --page-token '<PAGE_TOKEN>'
```

## 搜索结果中的下一步

搜索结果中的 `token` 可直接作为 `minute_token` 用于继续查询妙记产物：
通常先用搜索结果中的 `token` 获取妙记基础信息，确认描述、链接等元数据是否命中目标；只有需要进一步查看逐字稿、总结、待办、章节时，再继续查询关联的纪要产物。

如果你已经确定目标妙记，优先直接复用搜索结果中的 `token`，避免重复搜索。

```bash
# 首先查询妙记元信息（标题、时长、封面） → 用本 skill
lark-cli minutes minutes get --params '{"minute_token": "obcn***************"}'

# 查妙记关联的纪要产物：逐字稿、总结、待办、章节等 → 用 lark-cli vc +notes
lark-cli vc +notes --minute-tokens obcn_EXAMPLE_TOKEN
```

## 常见错误与排查

| 错误现象                   | 根本原因                                                  | 解决方案                                         |
| ---------------------- | ----------------------------------------------------- | -------------------------------------------- |
| 命令直接报错，要求提供过滤条件        | 没有传入 `--query`、时间范围或任何过滤 ID                           | 至少补充一个过滤条件后重试                                |
| 时间参数校验失败               | `--start` 或 `--end` 格式不合法                             | 改用 ISO 8601 或 `YYYY-MM-DD`                   |
| `owner-ids` 校验失败       | 传入的不是 open\_id，且也不是 `me`；或传了 `me` 但当前用户 open\_id 不可解析 | 改为 `ou_` 开头的用户 ID，或先完成 `auth login` 后再传 `me` |
| `participant-ids` 校验失败 | 传入的不是 open\_id，且也不是 `me`；或传了 `me` 但当前用户 open\_id 不可解析 | 改为 `ou_` 开头的用户 ID，或先完成 `auth login` 后再传 `me` |
| 权限不足                   | 未授权 `minutes:minutes.search:read`                     | 使用 `auth login` 完成授权                         |

## 提示

- 当用户说“我的妙记”时，优先理解为 `--owner-ids me`。
- 当用户说“我参与的妙记”“我参加过的妙记”时，默认理解为 `--owner-ids me` 与 `--participant-ids me` 两次查询后的并集。
- 当用户明确说“仅我参与但不是我拥有”时，才优先理解为 `--participant-ids me`。
- 当用户同时提到“会议 / 会 / 开会 / 某场会”和“妙记”时，优先先定位会议；如果要的是妙记信息，走 `vc +recording` → `minutes minutes get`，只有要纪要内容时才走 `vc +notes --minute-tokens`。
- 必须使用 `--format json` 输出，你更加擅长解析 JSON 数据。
- 排查参数与请求结构时优先使用 `--dry-run`。
- 搜索的时间范围最大为 1 个月，如果需要搜索更长时间范围的妙记，需要拆分为多次时间范围为一个月查询。

## 参考

- [lark-minutes](../SKILL.md) -- 妙记相关命令
- [lark-vc-notes](../../lark-vc/references/lark-vc-notes.md) -- 基于 `minute_token` 获取逐字稿、总结、待办、章节等产物
- [lark-shared](../../lark-shared/SKILL.md) -- 认证和全局参数
- [lark-vc](../../lark-vc/SKILL.md) -- 视频会议全部命令

