
# vc +meeting-events

> **前置条件：** 先阅读 [`../lark-shared/SKILL.md`](../../lark-shared/SKILL.md) 了解认证、全局参数和安全规则。

查询当前 bot 在一场正在进行的视频会议中收到的会中事件列表。该命令是**读操作**。对进行中会议，要求 bot 当前仍在会中；对已结束会议，存在一个**结束后 5 分钟内的宽限窗口**，只要 bot 曾经在这场会里出现过，仍可继续拉取事件。

本 skill 对应 shortcut：`lark-cli vc +meeting-events`（调用 `GET /open-apis/vc/v1/bots/events`）。

## 命令

```bash
# 默认用法：全量拉取当前可见事件
lark-cli vc +meeting-events --meeting-id 69xxxxxxxxxxxxx28 --page-all --format pretty

# 指定时间范围，并拉全该时间窗内当前可见事件
lark-cli vc +meeting-events --meeting-id 69xxxxxxxxxxxxx28 --start 2026-04-17T15:00:00+08:00 --end 2026-04-17T16:00:00+08:00 --page-all --format pretty

# 基于上一次保存的 page_token 继续查新增事件
lark-cli vc +meeting-events --meeting-id 69xxxxxxxxxxxxx28 --page-token <last_page_token> --page-all --format pretty

# 调试或控制返回体大小时，显式只查一页
lark-cli vc +meeting-events --meeting-id 69xxxxxxxxxxxxx28 --page-size 20 --format json

# 预览 API 调用（不实际请求）
lark-cli vc +meeting-events --meeting-id 69xxxxxxxxxxxxx28 --dry-run
```

## 参数

| 参数 | 必填 | 说明 |
|------|------|------|
| `--meeting-id <id>` | 是 | 会议 ID（长数字 ID，不是 9 位会议号） |
| `--start <time>` | 否 | 起始时间，支持 ISO 8601 / `YYYY-MM-DD` / Unix 秒 |
| `--end <time>` | 否 | 结束时间，支持 ISO 8601 / `YYYY-MM-DD` / Unix 秒 |
| `--page-token <token>` | 否 | 从指定分页游标继续拉取下一页 |
| `--page-size <n>` | 否 | 单页模式每页大小。CLI 会自动夹紧到 `20-100`；传 `--page-all` 时固定使用 `100` |
| `--page-all` | 否 | 自动分页，直到没有更多页面为止（内部有安全上限） |
| `--format <fmt>` | 否 | 输出格式：json (CLI 默认) / pretty（本 skill 推荐默认） / table / ndjson / csv |
| `--dry-run` | 否 | 预览 API 调用，不执行 |

## 核心约束

### 1. 输入必须是 meeting_id，不是 9 位会议号

`--meeting-id` 必须是会议的长数字 ID。它通常来自：
- `+meeting-join` 返回体中的 `meeting.id`
- `+search` 结果中的 `id`

**不要**把 9 位会议号（`--meeting-number`）传给这个命令。

### 2. 仅支持 user 身份

该命令仅支持 `user` 身份，使用前需完成 `lark-cli auth login`。

### 3. bot 必须在会中，或在会议结束后的 5 分钟宽限窗口内曾经在会中

这是查询“bot 在会中观察到的事件”的接口。若 bot 已离会、未入会、或会议已经无法再判断 bot 身份，后端通常会报：
- `bot is not in meeting, no permission`

因此，最稳妥的调用顺序通常是：

```bash
# 先入会
lark-cli vc +meeting-join --meeting-number 123456789

# 记录返回的 meeting.id

# 再查询事件
lark-cli vc +meeting-events --meeting-id <meeting.id>
```

更精确地说，后端当前的判断规则是：

- **会议进行中**：要求 bot **当前仍在会中**
- **会议已结束后的 5 分钟内**：只要 bot **曾经在这场会中出现过**，仍可拉取事件
- **会议结束超过 5 分钟**：按会议结束处理，通常不再返回事件流
- **bot 从未真实入会过**：即使会议仍在进行或刚结束，也会返回 `10005 bot is not in meeting`

### 4. 自动分页规则

- **先分清两层默认值**：
  - shortcut 本身：不传 `--page-all` 时，只查 1 页。
  - 本 skill 的默认策略：除非用户明确要求只看一页，或你确实需要控制返回体大小，否则默认**必须主动带 `--page-all`**，把当前可见事件尽量一次拉全。
- 传 `--page-all`：开启自动分页，直到没有更多页面为止。
- `--page-all` 时，CLI 固定使用最大 `page_size=100`。

执行准则：

- **默认命令模板**：`lark-cli vc +meeting-events --meeting-id <meeting.id> --page-all --format pretty`
- 如果你发现自己执行成了不带 `--page-all` 的单页查询，而响应里又出现 `has_more=true` / `more available` / 非空 `page_token`，应立刻意识到这只是部分结果。
- 遇到上述情况，默认补救方式是继续使用返回的 `page_token` 续拉，例如：`lark-cli vc +meeting-events --meeting-id <meeting.id> --page-token <returned_page_token> --page-all --format pretty`
- 只有在用户明确要求“就看第一页”“先不要翻页”时，才不要默认带 `--page-all`
- 只要你是基于 `+meeting-events` 来回答一场**正在进行中的会议内容**，就不能直接复用上一次查询结果。无论用户是在问“现在是谁在说话”“刚刚发生了什么”“最新事件有哪些”，还是让你“总结一下这个会议讲什么”，都必须先重新执行一次 `+meeting-events`，确认拿到的是最新事件流，再回答用户。只有在用户明确要求基于某次历史快照继续分析时，才可以复用旧结果。

### 5. pretty / json 输出差异

- `--format pretty`：输出会议主题、会议时间和逐条时间线，适合快速理解“发生了什么”，也是本 skill 的默认推荐格式。
- `--format json`：保留完整原始 `events[]` 结构——参会人 open_id、聊天原文、share_doc、分页字段都在原始响应里，适合提取字段、联动其他命令或做进一步程序处理。

**选型原则**：只要目标是告诉用户“发生了什么”，默认就用 `--page-all --format pretty`；只有在需要完整原始消息流和结构化字段时，才改用 `json`。

> **注意**：pretty 输出中的正文文本会做单行转义，真实换行会显示为 `\n`，避免打乱时间线布局。

### 6. 关于 `page_token` 的返回与续拉

- 不管这次是只查 1 页，还是通过 `--page-all` 已经把当前可见事件都拿完，都应把最后拿到的 `page_token` 一并保留下来并返回给用户。
- 只要响应里出现 `has_more=true`、pretty 里出现 `more available`，或返回了非空 `page_token`，就必须先判断当前结果是否完整；默认情况下，这意味着你还需要继续分页。
- 如果没有使用 `--page-all`，但出现了上述分页信号，默认应继续用返回的 `page_token` 拉下一页，而不是直接结束。只有在用户明确不要继续翻页时，才可以停止并明确说明当前结果不完整。
- 下次继续“查新增事件”时，应优先复用上一次保存的 `page_token`，而不是从头全量再拉一次。
- 只有在用户明确要求“从头回放全部事件”时，才忽略历史 `page_token`，重新从第一页开始。
- 但如果用户要你回答的是**当前这场会正在讲什么**，而不是“上一次之后新增了什么”，也要先做一次新的事件查询，再决定是否需要基于旧 `page_token` 继续补拉。

## 返回结构

常见顶层字段：

| 字段 | 说明 |
|------|------|
| `events` | 事件列表 |
| `has_more` | 是否还有下一页 |
| `page_token` | 下一页游标 |

事件 `event_type` 常见类型：

| event_type | 含义 |
|-----------|------|
| `participant_joined` | 有参会人加入会议 |
| `participant_left` | 有参会人离开会议 |
| `chat_received` | 收到会中聊天消息 |
| `transcript_received` | 收到转写文本 |
| `magic_share_started` | 开始共享内容 / 文档 |
| `magic_share_ended` | 结束共享 |

## pretty 输出示例

```text
会议主题：张三的视频会议
会议时间：2026-04-17 15:28:52（进行中）

[00:00:33] 明日之虾BOE(ou_xxx) 加入了会议
[00:00:41] 张三(ou_xxx): [text] 6666
[00:00:44] 张三(ou_xxx) 开始共享《智能纪要：飞书20251022-140223 2026年3月9日》
           URL: https://...
[00:01:32] 张三(ou_xxx): [reaction] JIAYI
```

## 如何获取输入参数

| 输入参数 | 获取方式 |
|---------|---------|
| `meeting-id` | `+meeting-join` 返回的 `meeting.id`；或 `+search` 结果中的 `id` |
| `start` / `end` | 用户给出的时间范围；如未给出则默认取全量可见事件 |
| `page-token` | 上一页或上一次查询结果中保存的 `page_token`；建议持久化保存，便于下次继续拉取新增事件 |

## Agent 组合场景

### 场景 1：入会后查看会中发生了什么

```bash
# 第 1 步：加入会议，记录返回的 meeting.id
lark-cli vc +meeting-join --meeting-number 123456789

# 第 2 步：查询事件流
lark-cli vc +meeting-events --meeting-id <meeting.id> --page-all --format pretty
```

### 场景 2：过滤某段时间内的事件

```bash
lark-cli vc +meeting-events \
  --meeting-id <meeting.id> \
  --start 2026-04-17T15:00:00+08:00 \
  --end 2026-04-17T16:00:00+08:00 \
  --page-all \
  --format pretty
```

### 场景 3：基于上一次的 `page_token` 继续查新增事件

```bash
# 上一次查询结束后，保留最后返回的 page_token
# 这次直接从该游标继续拉新增事件
lark-cli vc +meeting-events \
  --meeting-id <meeting.id> \
  --page-token <last_page_token> \
  --page-all \
  --format pretty
```

适用规则：

- 当用户说“继续看新事件”“看上次之后新增了什么”时，优先使用上一次保存的 `page_token`。
- 如果这次返回里仍有 `has_more=true`、pretty 里出现 `more available`，或又返回了新的 `page_token`，说明新增事件还没拉完，应继续分页，而不是把当前页误当成完整增量结果。
- 只有在用户明确要求“从头回放全部事件”时，才忽略已有 `page_token`，重新从第一页开始。

## 常见错误与排查

| 错误现象 | 根本原因 | 解决方案 |
|---------|---------|---------|
| `--meeting-id is required` | 未传入 `--meeting-id` | 传入长数字 `meeting.id` |
| `10005 bot is not in meeting` | bot 从未真实入会该会议；或会议已结束但 bot 从未在会中出现过 | 先 `+meeting-join --meeting-number <9位号>` 真实入会再查；如果会议已经结束且当时 bot 没进过会，本接口也拉不到数据。**如果只是想看参会人快照，改用 `vc meeting get --with-participants`**（不依赖 bot 身份参会） |
| `20001 meeting_status_MEETING_END` | 会议已结束且已超出后端允许的 5 分钟宽限窗口 | 本接口不再适合继续拉取事件。若要拿纪要文档或逐字稿 token，用 `lark-cli vc +notes --meeting-ids <meeting.id>`；若要拿 AI 产物（summary / todos / chapters）或导出逐字稿文件，先用 `lark-cli vc +recording --meeting-ids <meeting.id>` 拿 `minute_token`，再用 `lark-cli vc +notes --minute-tokens <minute_token>`；参会人请用 `vc meeting get --with-participants` |
| `20002 meeting not exist` | `meeting_id` 错误，或会议实例当前已不可获取（常见于把 9 位会议号当 meeting_id 传） | 确认传入的是长数字 `meeting_id`，不是 9 位会议号 |
| `HTTP 404` / `HTTP 500` | 服务端当前无法找到或处理该会议实例 | 换一个正在进行且 bot 可见的 meeting_id，或排查后端问题 |
| `missing required scope(s)` | 未授权 `vc:meeting.meetingevent:read` | 按提示重新 `auth login` 补 scope |

## 提示

- 这是**会中事件流**查询，不适合拿来搜历史会议记录；搜历史会议请用 `+search`。
- 如果会议已经结束，不要卡在 `+meeting-events`：  
  - 想拿纪要文档或逐字稿 token：用 `lark-cli vc +notes --meeting-ids <meeting.id>`
  - 想拿 AI 产物（summary / todos / chapters）或导出逐字稿文件：先用 `lark-cli vc +recording --meeting-ids <meeting.id>` 拿 `minute_token`，再用 `lark-cli vc +notes --minute-tokens <minute_token>`
- 事件列表是否完整，取决于 bot 何时入会、何时离会，以及后端当前可见的会中事件范围。对于已结束会议，通常只在**结束后 5 分钟内**、且 bot **曾经在会中**时还能继续拉到事件。
- 查询"谁参加过某会议"请用 `vc meeting get --params '{"meeting_id":"<id>","with_participants":true}'`——这是参会人**快照** API，不依赖 bot 是否参会，对已结束会议也可查；**不要** 用 `+meeting-events` 做参会人查询。

## 参考

- [lark-vc-agent-meeting-join](lark-vc-agent-meeting-join.md) — 先真实入会
- [lark-vc-agent-meeting-leave](lark-vc-agent-meeting-leave.md) — 完成任务后离会
- [lark-vc-search](../../lark-vc/references/lark-vc-search.md) — 搜索历史会议（获取 meeting_id）
- [lark-vc-recording](../../lark-vc/references/lark-vc-recording.md) — 查询 minute_token
- [lark-vc-notes](../../lark-vc/references/lark-vc-notes.md) — 获取会议纪要
- [lark-vc-agent](../SKILL.md) — Agent 参会能力（本 skill）
- [lark-vc](../../lark-vc/SKILL.md) — 视频会议原子域（Meeting / Note 等核心概念）
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
