
# vc +search

搜索已结束的历史会议记录，支持关键词、时间范围、组织者、参与者、会议室多条件过滤。只读，仅 `--as user`。

## 关键词使用边界

`--query` 只用于真实会议关键词，例如会议主题、项目名、评审名、客户名。用户只是说"我这月参加的所有视频会议"、"最近两周我组织的所有视频会议"、"总结主要议题 / 看看参会情况"时，本质是历史会议列表和后续总结，不要把"回顾"、"所有视频会议"、"总结主要议题"等动作词放进 `--query`。这类请求应先用时间范围 + `--participant-ids` / `--organizer-ids` 搜全量候选，再按结果继续取纪要或录制信息。

列表阶段只负责找会议记录；总结阶段必须继续取证。若用户要求"主要议题"、"主要决策"、"参会情况"，先确认搜索结果的 `meeting_id`、时间、组织者/参与者符合过滤条件，然后用 `vc +detail` 或 `minutes` 读取纪要、妙记或录制信息。没有纪要或妙记时，如实说明只能基于会议标题/参会数据汇总，不要编造议题。

## 典型触发表达

以下说法通常应优先使用 `vc +search`：

- 今天开过的会
- 今天开了哪些会
- 最近参加过哪些会
- 我这周开过的会
- 已结束的会议
- 历史会议记录

## 命令

```bash
# 关键词搜索
lark-cli vc +search --query "周会"

# 查询某一天开过的会（单日查询时，start 和 end 必须填写同一天）
lark-cli vc +search --start 2026-03-10 --end 2026-03-10

# 按时间范围搜索
lark-cli vc +search --start "2026-03-10T00:00+08:00" --end "2026-03-17T00:00+08:00"

# 按组织者 / 参与者 / 会议室（逗号分隔）
lark-cli vc +search --organizer-ids "ou_user1,ou_user2"
lark-cli vc +search --participant-ids "ou_user1,ou_user2"
lark-cli vc +search --room-ids "123,456"

# 多条件组合
lark-cli vc +search --organizer-ids "ou_user1" --room-ids "123" --start "2026-03-10T00:00+08:00"

# 翻页
lark-cli vc +search --query "周会" --page-token "<PAGE_TOKEN>"
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--query <text>` | 否 | 搜索关键词 |
| `--start <time>` | 否 | 开始时间（ISO 8601 或仅日期） |
| `--end <time>` | 否 | 结束时间（ISO 8601 或仅日期） |
| `--organizer-ids <ids>` | 否 | 组织者 open_id 列表，逗号分隔 |
| `--participant-ids <ids>` | 否 | 参与者 open_id 列表，逗号分隔 |
| `--room-ids <ids>` | 否 | 会议室 ID 列表，逗号分隔 |
| `--page-size <n>` | 否 | 每页数量，默认 `15`，最大 `30` |
| `--page-token <token>` | 否 | 翻页标记，用于获取下一页 |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 核心约束

### 1. 至少提供一个过滤条件

所有参数均可选，但必须至少提供一个过滤条件：`--query`、`--start`、`--end`、`--organizer-ids`、`--participant-ids` 或 `--room-ids`。

没有真实关键词时，时间范围或人员过滤已经满足这个约束，`--query` 可以省略。

涉及"本月"、"最近两周"这类相对时间时，先基于执行当天计算 `"<YYYY-MM-DD>"` 占位符，再运行命令；不要沿用文档示例生成时的具体日期。

### 2. 仅搜索历史会议

`vc +search` 只能搜索已结束的历史会议记录，不用于查询未来日程。查询未来会议安排请使用 [lark-calendar](../../lark-calendar/SKILL.md)。

### 3. 仅支持 user 身份

该接口仅支持 `user` 身份，使用前需完成 `lark-cli auth login` 并具备 `vc:meeting.search:read` 权限。

### 4. 支持分页

当返回 `has_more=true` 时，使用响应中的 `page_token` 配合 `--page-token` 获取下一页结果。

### 5. 机器人可同时加入多个会议

机器人支持同时加入多个正在进行中的会议；加入新会议前，不需要先退出已经在会中的其他会议。

这意味着：

- 不要假设 bot 一次只能在一个会议中
- 如果用户要求 bot 再加入另一场会，可以直接继续执行对应的入会命令
- 只有在用户明确要求结束某一场会中的 bot 参会时，才调用对应的离会命令

### 6. 日期型 `--end` 包含当天整天

当 `--end` 传入的是仅日期格式（如 `2026-03-10`）时，CLI 会将它解释为当天 `23:59:59`，而不是当天 `00:00:00`。

这意味着：

- `--start 2026-03-10 --end 2026-03-10` 表示只查 `2026-03-10` 当天
- `--start 2026-03-10 --end 2026-03-11` 表示查询 `2026-03-10` 和 `2026-03-11` 两天

如果用户说“昨天开过的会”“今天开过的会”“某一天开过的会”，应把 `--start` 和 `--end` 都设置为同一天，而不是把 `--end` 设成下一天。

## 时间格式

`--start` 和 `--end` 支持以下时间格式：

| 格式 | 示例 | 说明 |
|------|------|------|
| ISO 8601（带时区） | `2026-03-10T14:00:00+08:00` | 推荐 |
| ISO 8601（不带时区） | `2026-03-10T14:00:00` | 按本地时区解析 |
| 仅日期 | `2026-03-10` | 按天粒度解析；若用于 `--end`，表示当天 `23:59:59` |

## 输出结果

- 默认输出 JSON，包含 `items`、`has_more` 和 `page_token`。

## Pagination (`has_more` / `page_token`)

- 当结果中返回 `has_more=true` 时，说明还有更多页可继续获取。
- 继续翻页时，使用响应中的 `page_token` 搭配 `--page-token` 发起下一次查询。
- 不要假设调大 `--page-size` 就能拿全结果；分页遍历时应以 `has_more` 和 `page_token` 为准。
- 未明确要求全量时，逐页累计已读取的 `items` 数：累计不到 50 条之前可自动继续翻页（`has_more=true` 即继续）；超过 50 条且仍 `has_more=true` 时，先向用户确认是否继续获取全部结果。
- 用户明确说"所有 / 全部 / 统计 / 按时间排序"时，该全量意图优先于 50 条的确认门槛；直接按 `has_more` 翻完所有页并去重，再排序或统计，不要只用第一页回答。

```bash
# First page
lark-cli vc +search --query "周会" --page-size 15

# Next page
lark-cli vc +search --query "周会" --page-size 15 --page-token "<PAGE_TOKEN>"
```

## 搜索结果中的下一步

搜索结果中的 `meeting_id` 可直接用于继续查询会议纪要或妙记：

```bash
# 如果要会议纪要 / 逐字稿 / AI 总结 / 待办 / 章节
lark-cli vc +detail --meeting-ids <MEETING_ID>

# 如果要会议对应的妙记信息 / minute_token / 妙记链接
lark-cli vc +recording --meeting-ids <MEETING_ID>
# 然后再用返回的 minute_token 调用：
lark-cli minutes minutes get --params '{"minute_token":"<MINUTE_TOKEN>"}'
```

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| 命令直接报错，要求提供过滤条件 | 没有传入 `--query`、时间范围或任何过滤 ID | 至少补充一个过滤条件后重试 |
| 时间参数校验失败 | `--start` 或 `--end` 格式不合法 | 改用 ISO 8601 或 `YYYY-MM-DD` |
| 搜不到未来会议 | `vc +search` 只查历史会议 | 改用 [lark-calendar](../../lark-calendar/SKILL.md) 查询未来日程 |
| 权限不足 | 未授权 `vc:meeting.search:read` | 使用 `auth login` 完成授权 |

## 提示
- 必须使用 `--format json` 输出，你更佳擅长解析 JSON 数据。
- 排查参数与请求结构时优先使用 `--dry-run`。
- 搜索的时间范围最大为 1 个月，如果需要搜索更长时间范围的会议，需要拆分为多次时间范围为一个月查询。
- 不要使用 `yesterday`、`today` 这类相对时间字面量；请先转换成明确日期，例如 `2026-03-10`。
- 用户如果明确问的是“妙记信息”而不是“纪要内容”，不要默认走 `vc +detail`；应先用 `vc +recording`。

