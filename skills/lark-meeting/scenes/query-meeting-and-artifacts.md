# 查询会议及其产物

围绕目标会议执行查询：先取得唯一 `meeting_id`，再按用户目标查询参会人、智能纪要、妙记或录制。已有 `note_id` 或 `minute_token` 时从对应产物直接开始，不要绕回会议搜索。

## 定位会议

在决定批量命令和批次大小之前，必须先规范化全部输入标识：

- 恰好 9 位纯数字是 `meeting_no`，即使用户称其为“会议 ID”。
- `meeting_no` 必须先逐个通过 `vc +search --query` 转换为搜索结果中的 `id`。
- `vc +search` 不支持批量会议号；多个 `meeting_no` 按输入顺序逐个解析，也可使用脚本批量转换。

优先复用已有标识，不重复搜索：

| 已有信息 | 操作 |
|---|---|
| `meeting_id` | 直接查询会议或关联产物 |
| `meeting_no` / 9 位会议号 | 用 `vc +search --query "<meeting_no>" --format json --as user` 搜索会议，从结果的 `id` 取得 `meeting_id` |
| Calendar `event_id` | 用 `calendar +meeting` 获取 `meeting_id` 和用户绑定的 `meeting_note` |
| `note_id` | 直接进入 [智能纪要场景](query-note-and-artifacts.md) |
| `minute_token` / 妙记 URL | 直接进入 [妙记场景](query-minutes-and-artifacts.md)；URL 取路径最后一段并去掉 query 参数 |

没有标识时，用 `vc +search` 搜索已经结束的会议：

```bash
lark-cli vc +search --query <query> --start <start> --end <end> --format json
```

- 至少提供关键词、时间范围、组织者、参与者或会议室中的一个条件；不要把“总结”“回顾”“所有会议”等动作词当作 `--query`。
- “今天有哪些会议”需要合并两部分：`vc +search` 查询今天已结束的会议， lark-calendar 查询进行中或未开始的日程。
- 只有自然语言纪要标题、没有会议 ID、时间、参会人等会议线索时，改用 Drive/Doc 搜索纪要文档，不要把纪要标题当作会议关键词。
- 根据 `has_more` 和 `page_token` 翻页。未明确要求全量时，累计结果超过 50 条后先确认是否继续；用户明确要求“全部、统计、排序”时直接获取全部结果。
- 多个候选时展示主题、时间、组织者和 `meeting_id`，让用户选择；不要擅自选择最近的一场。

只需要找到会议时，返回唯一 `meeting_id` 后停止。

搜索参数、日期语义和分页细节见 [`lark-vc-search`](../references/lark-vc-search.md)。

## 选择查询身份

- `vc +search` 仅支持用户身份。`vc +detail`、`vc +recording`、`vc meeting get` 和 `note +detail` 支持用户或应用身份。
- 已有 `meeting_id`、`note_id` 或 `minute_token` 时，沿用其来源身份；后续 Minutes、Note、Doc 和 Drive 命令都显式传入同一个 `--as`。不要为查询参会人或绕过权限错误擅自切换身份。
- `note +transcript` 仅支持用户身份。应用身份查到 unified Note 时，先说明限制，只有用户明确同意后才切换身份。

## 获取参会人

查询“谁参加过、何时加入或离开、某人是否参会”时，读取会议的参会人快照：

```bash
lark-cli vc meeting get --params '{"meeting_id":"<meeting_id>","with_participants":true}' --as <source_identity>
```

这是服务端快照，不要求应用机器人入会，会议结束后也可以查询。不要用会中事件代替完整参会人快照。

## 获取会议产物标识

使用 `vc +detail` 获取会议关联的 `note_id` 和 `minute_token`：

```bash
lark-cli vc +detail --meeting-ids <meeting_id> --as <source_identity>
```

Note 与 Minutes 来自相互独立的 AI 总结和录制链路，可能同时存在、只存在一个或都不存在：

- 用户明确指定“智能纪要”或“妙记”时，沿指定链路处理，不要改道。
- 只存在一类产物时，使用存在的那一类，不要因默认优先级而把缺失的 Note 或 Minutes 当作错误。
- 两者都存在且用户未指定时，优先使用 Note。Note 及其逐字稿会后通常直接对参会人可读；Minutes 包含原始音视频并受独立资源 ACL 控制，往往需要所有者授权或由用户明确申请权限。
- Note 和 Minutes 的总结、待办等 AI 产物可能内容重叠。按上述规则选定一条主链路；除非用户明确要求对照，不要自动拼接、合并或去重两份 AI 产物。
- 用户只要产物标识时，返回取得的 `note_id` / `minute_token` 后停止；需要产物链接时，进入对应下游场景解析，不继续读取正文。
- `meeting_note` 是 Calendar 日程上由用户绑定的 Doc，只能从 `event_id` 经 `calendar +meeting` 获取；它与 AI 智能纪要独立，不要从 `meeting_id` 或 `note_id` 推断。
- 用户询问“有哪些纪要”或“纪要链接”且上下文包含 `event_id` 时，保留 `calendar +meeting` 返回的 `meeting_note`；如存在 `note_id`，进入智能纪要场景取得 `note_doc_token`，再同时返回两者供用户区分选择。

会议详情字段见 [`lark-vc-detail`](../references/lark-vc-detail.md)。

## 转交智能纪要场景

取得 `note_id` 后，进入 [基于 note_id 查询智能纪要及关联产物](query-note-and-artifacts.md)，并传递 `note_id` 与取得该 ID 时使用的 `source_identity`。`note +detail`、正文与封面读取、`note_display_type` 逐字稿路由、共享文档和 Doc 元信息查询全部以该场景为准，不在本场景重复定义。

## 转交妙记场景

取得 `minute_token` 后，进入 [查询妙记及其产物](query-minutes-and-artifacts.md)，并传递 `minute_token` 与取得该 Token 时使用的 `source_identity`。妙记基础信息、AI 产物、Transcript、媒体下载、关联 Note 和资源权限处理全部以该场景为准，不在本场景重复定义。

如果需要从 `meeting_id` 或 Calendar `event_id` 补查录制，先按 [`vc +recording`](../references/lark-vc-recording.md) 取得 `minute_token`，再转入妙记场景。

## 基于会议内容回答或分析

- 用户只要现成的 AI 总结、待办或章节时，直接返回选定链路的对应 AI 产物，不为此额外读取逐字稿。待办通常包含提出人或负责人，章节按话题组织，因此查看待办或会议结构时应优先使用这些结构化 AI 产物。
- 用户要求提炼、重新总结、复盘、争议分析或“谁说了什么”时，读取 Note 逐字稿或 Minutes Transcript 的原始对话并独立分析；禁止把现成 AI 总结重新排版后冒充独立结论。
- Note 和 Minutes 都有原始记录且用户未指定时，优先使用 Note 逐字稿；用户明确说“基于妙记”时使用 Minutes Transcript。
- 如果产物不存在或无权限，如实说明，并保留已经取得的 `meeting_id`、`note_id`、`minute_token` 或文档 token，方便用户继续处理。
