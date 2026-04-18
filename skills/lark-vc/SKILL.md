---
name: lark-vc
version: 1.0.0
description: "飞书视频会议：查询会议记录、获取会议纪要产物（总结、待办、章节、逐字稿），查询 bot 在会中的事件列表，以及通过会议号加入/离开正在进行的会议。1. 查询已经结束的会议数量或详情时使用本技能(如历史日期｜ 昨天 | 上周 | 今天已经开过的会议等场景)，查询未开始的会议日程使用 lark-calendar 技能。2. 支持通过关键词、时间范围、组织者、参与者、会议室等筛选条件搜索会议记录。3. 获取或整理会议纪要时使用本技能。4. Agent 需要真实入会/离会（如会中助手、参会机器人）时使用本技能的 +meeting-join / +meeting-leave。5. Agent 需要查看会中发生了什么（入会、离会、聊天、共享、转写等事件）时，使用 +meeting-events。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli vc --help"
---

# vc (v1)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**

## 核心概念

- **视频会议（Meeting）**：飞书视频会议实例，通过 meeting\_id 标识。已结束的会议支持通过关键词、时间段、参会人、组织者、会议室等条件搜索（见 `+search`）。
- **会议纪要（Note）**：视频会议结束后生成的结构化文档，包含纪要文档（包含总结、待办、章节）和逐字稿文档。
- **妙记（Minutes）**：来源于飞书视频会议的录制产物或用户上传的音视频文件，支持视频/音频的转写和会议纪要，通过 minute\_token 标识。
- **纪要文档（MainDoc）**：AI 智能纪要的主文档，包含 AI 生成的总结和待办，对应 `note_doc_token`。
- **用户会议纪要（MeetingNotes）**：用户主动绑定到会议的纪要文档，对应 `meeting_notes`。仅通过 `--calendar-event-ids` 路径返回。
- **逐字稿（VerbatimDoc）**：会议的逐句文字记录，包含说话人和时间戳。

## 核心场景

### 1. 搜索会议记录
1. 仅支持搜索已结束的会议，对于还未开始的未来会议，需要使用 lark-calendar 技能。
2. 仅支持使用关键词、时间段、参会人、组织者、会议室等筛选条件搜索会议记录，对于不支持的筛选条件，需要提示用户。
3. 搜索结果存在多条数据时，务必注意分页数据获取，不要遗漏任何会议记录。

### 2. 整理会议纪要
1. 整理纪要文档时默认给出纪要文档和逐字稿链接即可，无需读取纪要文档或逐字稿内容。
2. 用户明确需要获取纪要文档中的总结、待办、章节产物时，再读取文档获取具体内容。
3. 读取智能纪要（`note_doc_token`）内容时，纪要文档的**第一个 `<whiteboard>`** 标签是封面图（AI 生成的总结可视化），应同时下载展示给用户：
```bash
# 1. 读取纪要内容
lark-cli docs +fetch --doc <note_doc_token>
# 2. 从返回的 markdown 中提取第一个 <whiteboard token="xxx"/> 的 token
# 3. 下载封面图到 artifact 目录（和逐字稿同目录，保持产物归拢）
#    并非所有纪要都有封面画板，没有 <whiteboard> 标签时跳过即可
lark-cli docs +media-download --type whiteboard --token <whiteboard_token> --output ./artifact-<title>/cover
```
> **产物目录规范**：同一会议的所有下载产物（封面图、逐字稿等）统一放到 `artifact-<title>/` 目录下，不要散落在当前工作目录。

> **纪要相关文档 — 根据用户意图选择：**
> - `note_doc_token` → **AI 智能纪要**（AI 总结 + 待办 + 章节）
> - `meeting_notes` → **用户绑定的会议纪要**（用户主动关联到会议的文档，仅 `--calendar-event-ids` 路径返回）
> - `verbatim_doc_token` → **逐字稿**（完整的逐句文字记录，含说话人和时间戳）— 用户说"逐字稿""完整记录""谁说了什么"时用这个
> - 用户说"纪要""总结""纪要内容"时，应同时返回 `note_doc_token` 和 `meeting_notes`（如有）
> - 用户意图不明确时，应展示所有文档链接让用户选择，而不是替用户决定

### 3. 加入 / 离开正在进行的会议（写操作）
1. 用户要求 Agent **真实入会**（如参会机器人、会中助手、代为旁听）时，使用 `+meeting-join`；只拉数据不需要入会。
2. `+meeting-join` 仅接受 **9 位纯数字**的会议号（`--meeting-number`），不要把会议链接整条或 `meeting_id` 当会议号传入。
3. `+meeting-join` 返回体中的 `meeting.id` **必须立刻记录**，离会时 `+meeting-leave --meeting-id` 用的就是它——不是会议号。
4. 两个命令都是**写操作**，不支持回放；执行前优先 `--dry-run` 核对请求体。
5. 仅支持 `user` 身份，需提前完成 `lark-cli auth login` 并拥有 `vc:meeting.bot.join:write` scope。

### 4. 纪要文档与逐字稿链接
1. 纪要文档、逐字稿文档与关联的共享文档默认使用文档 Token 返回。
2. 仅需要获取文档名称和 URL 等基本信息时，使用 `lark-cli drive metas batch_query` 查询
```bash
# 学习命令使用方式
lark-cli schema drive.metas.batch_query

# 批量获取文档基本信息: 一次最多查询 10 个文档
lark-cli drive metas batch_query --data '{"request_docs": [{"doc_type": "docx", "doc_token": "<doc_token>"}], "with_url": true}'
```
3. 需要获取文档内容时，使用 `lark-cli docs +fetch`。
```bash
# 获取文档内容
lark-cli docs +fetch --doc <doc_token>
```

### 5. 查询会中事件列表（读操作）
1. 用户要求查看“会议里发生了什么”“谁加入/离开了会议”“聊天记录事件”“共享开始/结束”“转写事件”等会中时间线时，优先使用 `+meeting-events`。
2. `+meeting-events` 的输入是 **meeting_id**（长数字 ID），不是 9 位会议号。
3. 该命令是**读操作**，但后端要求当前 bot 仍在会中；若 bot 已离会，接口会报 `bot is not in meeting`（`10005`）；会议已结束会报 `meeting_status_MEETING_END`（`20001`）。这个接口**不能做会后复盘**。
4. `+meeting-events` 默认查 1 页；需要自动翻页时可使用 `--page-limit` 或 `--page-all`。
5. 默认优先使用 `--format pretty`，因为 `json` 返回体通常比 pretty 大很多；只有在需要完整原始事件结构时再使用 `--format json`。
6. 即使这次已经拿全，也应一并返回最后拿到的 `page_token`（若有），方便下次继续增量拉取，而不是每次都从头开始。

### 6. 查询参会人快照（读操作）

用户问“谁参加过这场会议”“这个会议有哪些参会人”“某某参会了吗”等**参会人快照**类问题时，**不要**默认用 `+meeting-events`——它要求 bot 参会过该会议且会议进行中，对已结束会议或 bot 没入会的会议直接不可用。

正确抓手是 **`vc meeting get --with-participants`**：这是参会人服务端快照 API，不依赖 bot 身份参会，**已结束会议也可查**：

```bash
lark-cli vc meeting get --params '{"meeting_id":"<meeting_id>","with_participants":true}'
```

选型判断表：

| 用户意图 | 推荐命令 | 为什么 |
|---------|---------|--------|
| 参会人快照（谁参加过、何时入/离会）| `vc meeting get --with-participants` | 任意时点可查，含已结束会议 |
| 会中实时事件流（转写、聊天、共享）| `vc +meeting-events` | 仅 bot 参会且进行中有效 |
| 已结束会议的发言内容 | `vc +notes` 取 `verbatim_doc_token` 再 `docs +fetch` | 逐字稿是会后产物 |

## 资源关系

```
Meeting (视频会议)
├── Note (会议纪要)
│   ├── MainDoc (AI 智能纪要文档, note_doc_token)
│   ├── MeetingNotes (用户绑定的会议纪要文档, meeting_notes)
│   ├── VerbatimDoc (逐字稿, verbatim_doc_token)
│   └── SharedDoc (会中共享文档)
└── Minutes (妙记) ← minute_token 标识，+recording 从 meeting_id 获取
    ├── Transcript (文字记录)
    ├── Summary (总结)
    ├── Todos (待办)
    └── Chapters (章节)
```

> **注意**：`+search` 只能查询已结束的历史会议。查询未来的日程安排请使用 [lark-calendar](../lark-calendar/SKILL.md)。
>
> **优先级**：当用户搜索历史会议时，应优先使用 `vc +search` 而非 `calendar events search`。calendar 的搜索面向日程，vc 的搜索面向已结束的会议记录，支持按参会人、组织者、会议室等维度过滤。
>
> **路由规则**：如果用户在问“开过的会”“今天开了哪些会”“最近参加过什么会”“已结束的会议”“历史会议记录”，优先使用 `vc +search`。只有在查询未来日程、待开的会、agenda 时才优先使用 [lark-calendar](../lark-calendar/SKILL.md)。
> 
> **特殊情况**: 当用户查询“今天有哪些会议”时，通过 `vc +search` 查询今天开过的会议记录，同时使用 lark-calendar 技能查询今天还未开始的会议，统一整理后展示给用户。

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli vc +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+search`](references/lark-vc-search.md) | Search meeting records (requires at least one filter) |
| [`+notes`](references/lark-vc-notes.md) | Query meeting notes (via meeting-ids, minute-tokens, or calendar-event-ids) |
| [`+recording`](references/lark-vc-recording.md) | Query minute_token from meeting-ids or calendar-event-ids |
| [`+meeting-events`](references/lark-vc-meeting-events.md) | List bot meeting events by meeting_id (read) |
| [`+meeting-join`](references/lark-vc-meeting-join.md) | Join an in-progress meeting by 9-digit meeting number (write) |
| [`+meeting-leave`](references/lark-vc-meeting-leave.md) | Leave a meeting by meeting_id (write) |

- 使用 `+search` 命令时，必须阅读 [references/lark-vc-search.md](references/lark-vc-search.md)，了解搜索参数和返回值结构。
- 使用 `+notes` 命令时，必须阅读 [references/lark-vc-notes.md](references/lark-vc-notes.md)，了解查询参数、产物类型和返回值结构。
- 使用 `+recording` 命令时，必须阅读 [references/lark-vc-recording.md](references/lark-vc-recording.md)，了解查询参数和返回值结构。
- 使用 `+meeting-events` 命令时，必须阅读 [references/lark-vc-meeting-events.md](references/lark-vc-meeting-events.md)，了解 `meeting_id` 的来源、分页方式，以及“bot 仍在会中”的限制。
- 使用 `+meeting-join` 命令时，必须阅读 [references/lark-vc-meeting-join.md](references/lark-vc-meeting-join.md)，了解入参格式与写操作风险。
- 使用 `+meeting-leave` 命令时，必须阅读 [references/lark-vc-meeting-leave.md](references/lark-vc-meeting-leave.md)，了解 `meeting_id` 的来源与写操作风险。

> **写操作提示**：`+meeting-join` / `+meeting-leave` 是**写操作**，会真实入会 / 离会。执行前优先 `--dry-run` 核对请求体；`+meeting-join` 返回的 `meeting.id` 必须保留，用于后续 `+meeting-leave`。

## API Resources

```bash
lark-cli schema vc.<resource>.<method>   # 调用 API 前必须先查看参数结构
lark-cli vc <resource> <method> [flags] # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `--data` / `--params` 参数结构，不要猜测字段格式。

### meeting

  - `get` — 获取会议详情（主题、时间、参会人、note_id）

```bash
# 获取会议基础信息：不包含参会人列表
lark-cli vc meeting get --params '{"meeting_id": "<meeting_id>"}'


# 获取会议基础信息：包含参会人列表
lark-cli vc meeting get --params '{"meeting_id": "<meeting_id>", "with_participants": true}'
```

### minutes（跨域，详见 [lark-minutes](../lark-minutes/SKILL.md)）

  - `get` — 获取妙记基础信息（标题、时长、封面）；查询纪要**内容**请用 `+notes --minute-tokens <minute-token>`

## 权限表

| 方法 | 所需 scope |
|------|-----------|
| `+notes --meeting-ids` | `vc:meeting.meetingevent:read`、`vc:note:read` |
| `+notes --minute-tokens` | `vc:note:read`、`minutes:minutes:readonly`、`minutes:minutes.artifacts:read`、`minutes:minutes.transcript:export` |
| `+notes --calendar-event-ids` | `calendar:calendar:read`、`calendar:calendar.event:read`、`vc:meeting.meetingevent:read`、`vc:note:read` |
| `+recording --meeting-ids` | `vc:record:readonly` |
| `+recording --calendar-event-ids` | `vc:record:readonly`、`calendar:calendar:read`、`calendar:calendar.event:read` |
| `+search` | `vc:meeting.search:read` |
| `+meeting-events` | `vc:meeting.meetingevent:read` |
| `+meeting-join` | `vc:meeting.bot.join:write` |
| `+meeting-leave` | `vc:meeting.bot.join:write` |
| `meeting.get` | `vc:meeting.meetingevent:read` |
