---
name: lark-calendar
version: 1.0.0
description: "飞书日历：管理日历日程和会议室。查看/搜索日程、创建/更新日程、管理参会人、查询忙闲和推荐时段、预定会议室。当用户需要查看日程安排、创建/修改会议、查询/预定会议室时使用。不负责：查询过去的视频会议记录（走 lark-vc）、待办任务（走 lark-task）。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli calendar --help"
---

# calendar (v4)

开始前先读 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)（认证、权限处理）。

**CRITICAL — 凡涉及预约日程/会议室、调整时间或查询/搜索会议室，第一步 MUST 读 [`references/lark-calendar-schedule-meeting.md`](references/lark-calendar-schedule-meeting.md)。仅编辑字段（改标题/描述）或增删参会人（不涉及时间和会议室）时可跳过，直接读 [`references/lark-calendar-update.md`](references/lark-calendar-update.md)。**

## 身份

日程操作默认使用 `--as user`（查看和管理当前用户的日程）。`--as bot` 只能访问 bot 自己的（空）日历，会拿到空结果——不要用 bot 身份查用户日程。

```bash
# BAD — bot 身份查用户日程，返回空列表
lark-cli calendar +agenda --as bot

# GOOD — user 身份查日程
lark-cli calendar +agenda --as user
```

## Shortcuts

| Shortcut | 说明 |
|----------|------|
| `+agenda` | 查看日程安排（默认今天） |
| [`+meeting`](references/lark-calendar-meeting.md) | 通过日程事件 ID 获取关联的视频会议信息（meeting_id、meeting_note），日程开过视频会议才会有meeting_id |
| [`+create`](references/lark-calendar-create.md) | 创建日程并邀请参会人（ISO 8601 时间） |
| [`+update`](references/lark-calendar-update.md) | 更新既有日程字段，或独立增量添加/移除参会人和会议室 |
| `+freebusy` | 查询用户主日历的忙闲信息和 RSVP 状态（纯查询场景；预约场景走 `+suggestion`） |
| [`+room-find`](references/lark-calendar-room-find.md) | 针对一个或多个**明确的**时间块查找可用会议室（无明确时间时禁止直接调用，需先走 +suggestion） |
| [`+rsvp`](references/lark-calendar-rsvp.md) | 回复日程（接受/拒绝/待定） |
| [`+suggestion`](references/lark-calendar-suggestion.md) | 根据非明确时间或一段时间范围，推荐多个可用时间块方案 |

### `+get` — 单日程详情

通过 `calendar_id` + `event_id` 获取**单个日程**详情。

```bash
# calendar_id不传，默认primary
lark-cli calendar +get --calendar-id <calendar_id> --event-id <event_id>
```

### `+search-event` — 按关键词、时间范围和参会人搜索日程

仅返回基础字段（`event_id`/`summary`/`start`/`end` 等），需要详情请走 `+get`。

```bash
# query 按关键词 可选
# start/end 按时间范围（ISO 8601 或 YYYY-MM-DD）可选
# attendee-ids 按参会人（自动识别 ou_ 用户 / oc_ 群聊 / omm_ 会议室前缀）可选
# page-token 分页游标，用于继续翻页 可选
# page-size 每页数量，默认 30 可选
lark-cli calendar +search-event --query "周会" --start 2026-04-20 --end 2026-04-27 --attendee-ids "ou_user1,oc_chat1,omm_room1" --page-token <page_token> --page-size 30
```

### `+agenda` — 查看近期日程安排

默认查询当天。结果应整理为按日期分组、按开始时间升序的易读时间线。

```bash
# start/end 时间范围（ISO 8601 / YYYY-MM-DD / Unix 秒），均可选；默认当天
# calendar-id 日历 ID（默认primary）可选
lark-cli calendar +agenda --start 2026-03-10 --end 2026-03-17 --calendar-id <calendar_id>
```

注意：
- 已取消的日程自动过滤；无日程时直接告知"日程清空"。
- 时间范围超过 40 天会自动拆分查询并合并结果。

### `+freebusy` — 查询主日历忙闲时段和 RSVP 状态

仅返回忙碌时段起止时间，不含日程标题等隐私信息；其他订阅日历不在范围内。

```bash
# start/end 时间范围（ISO 8601 / YYYY-MM-DD / Unix 秒），均可选；默认当天
# user-id 目标用户 open_id（ou_ 前缀）可选；默认当前登录用户，bot 身份必须显式指定
lark-cli calendar +freebusy --start 2026-03-11 --end 2026-03-12 --user-id ou_xxx
```

用法提示：
- **仅判断是否有空** → `+freebusy`；**需要日程详情** → `+agenda`。
- 检查多人可用性：分别调用并对比，找共同空闲。
- 预约/改约场景下，调用规则（参与人过多、含群组、来自 `+suggestion` 等）详见 [schedule-clear-time.md § 查询忙闲](references/lark-calendar-schedule-clear-time.md#2-查询忙闲)。

## 前置条件路由

| 场景 | 前置要求 |
|------|----------|
| 预约日程/会议、调整时间、查会议室 | 先读 [lark-calendar-schedule-meeting.md](references/lark-calendar-schedule-meeting.md) |
| 仅编辑字段（标题/描述）或增删参会人 | 先定位 `event_id`，再读 [lark-calendar-update.md](references/lark-calendar-update.md) |
| 编辑已有日程（涉及时间或会议室） | 先定位目标日程 `event_id`；若是重复性日程，必须定位到具体实例的 `event_id`（禁止使用原重复日程 ID） |
| 编辑/删除重复性日程 | 先读 [重复性日程操作规范](references/lark-calendar-recurring.md)，按操作范围（仅此次/全部/此次及后续）执行 |
| 调用任何 Shortcut | 先读其对应 reference 文档 |

## 写操作反馈

创建、更新、删除、RSVP 等写操作完成后，直接基于命令返回结果反馈用户；不要为了“确认是否生效”主动发起二次查询。只有用户明确要求复查，或命令返回信息不足以回答用户问题时，才需要再查询。

## 核心概念

- **日程实例（Instance）**：重复性日程展开后的具体时间实例。「仅此次」操作时使用具体实例的 `event_id`；「全部」或「此次及后续」操作时需对原重复性日程操作（使用原日程 `event_id`），并按需处理例外。
- **重复性日程例外（Exception）**：对重复性日程某次实例做过「仅此次」编辑后产生的独立日程（拥有独立 `event_id`）。删除/更新「全部」时必须同时处理例外，否则例外会残留。
- **全天日程（All-day Event）**：只按日期占用、没有具体起止时刻的日程，结束日期是包含在日程时间内的。
- **时间块 vs 时间范围**：时间块是具体确定的连续时间段（如 `14:00~15:00`），时间范围是泛指（如"今天下午"）。`+room-find` 必须基于确定时间块，不能基于模糊范围。
- **会议室（Room）**："room"不是"房间"，是"会议室"。会议室是日程的一种参与人（resource attendee），不能脱离日程单独预定。
- **日程会议 ID（Meeting ID）**：日程的历史视频会议 ID，在日程上开过视频会议才会有。

## 术语映射

用户日常说的"帮我约个日历""查一下今天的日历"，实际意图是针对**日程（Event）**的创建或查询，而非操作日历（Calendar）容器本身。自动将口语化的"日历"意图映射为"日程"操作。

## 意图路由

| 用户意图 | 路由到 |
|----------|--------|
| 查询过去的会议（"昨天的会议""上周的会"） | [`../lark-vc/SKILL.md`](../lark-vc/SKILL.md)（会议数据含即时会议，仅查日程会遗漏） |
| 查询日历/日程或未来时间的会议 | 本 skill |
| 按关键词搜索日程 | 本 skill（`+search-event`） |
| 从日程获取关联的视频会议 ID 或用户绑定的会议纪要文档 | 本 skill（`+meeting`） |
| 从日程进一步拿 AI 智能纪要 / 逐字稿 / 妙记产物 | 先 `+meeting` 取 `meeting_id`，再 [`vc +detail`](../lark-vc/references/lark-vc-detail.md) → [`note +detail`](../lark-note/references/lark-note-detail.md) / [`minutes +detail`](../lark-minutes/references/lark-minutes-detail.md) |
| 预约/改约日程、调整时间、添加/更换会议室、查会议室 | 先判断新建 vs 编辑，再进入 [schedule-meeting 工作流](references/lark-calendar-schedule-meeting.md) |
| 仅编辑日程字段（标题/描述）或增删参会人（不涉及时间和会议室） | 先定位 `event_id`，再读 [+update](references/lark-calendar-update.md) 执行变更 |
| 编辑/删除重复性日程（「改这个重复日程」「删掉后面的」「全部取消」等） | 先读 [重复性日程操作规范](references/lark-calendar-recurring.md)，确认操作范围后执行 |

## 任务类型分流

处理"预约/改约日程、添加/移除参会人、添加/更换会议室、调整时间"时，必须先判断新建 vs 编辑：

- **编辑已有日程的强信号**：用户提到已存在的日程锚点（标题、时间段、`这个日程`、`这场会`）并表达修改动作（添加、移除、改到、换会议室、调整时间）。默认走编辑流，绝不能按新建处理。
- **新建日程**：用户表达新增意图（"新约一个会""创建一个日程""安排一次会议"），且没有指向既有日程的修改动作。

## 时间推断规范

- **星期的定义**：周一是一周的第一天，周日是最后一天。计算"下周一"等相对日期时，基于当前真实日期推算。
- **一天的范围**：用户提到"明天""今天"等泛指某天时，时间范围应覆盖整天，不要自行缩减。
- **历史时间约束**：不能预约已经完全过去的时间。唯一例外是"跨越当前时间"的日程（开始在过去、结束在未来）。

## 会议室规则

- 凡是"预定/查询/搜索可用会议室"，都必须进入 [schedule-meeting 工作流](references/lark-calendar-schedule-meeting.md)，会议室参数规范详见 [+room-find](references/lark-calendar-room-find.md)。
- `+room-find` 的时间输入必须是确定时间块，不能是时间区间搜索。
- 用户仅要求"查会议室"但未提供明确时间时，必须先调用 `+suggestion` 获取可用时间块，再将时间块交给 `+room-find`。严禁猜测时间盲目调用。
- 编辑已有日程时，"添加会议室"默认是增量语义，保留已有会议室；只有用户明确说"更换会议室""移除会议室"时才删除旧会议室。

## API Resources

```bash
# 通用调用格式
lark-cli calendar <resource> <method> [flags]

# 查询用户主日历
lark-cli calendar calendars primary

# 获取日程分享链接
lark-cli calendar events share_info --calendar-id <calendar_id> --event-id <event_id>

# 删除日程
lark-cli calendar events delete --calendar-id <calendar_id> --event-id <event_id>
```

> `calendar_id` 可以直接传 `primary`，代表当前调用身份的主日历 ID。

### 查询资源的方法列表以及方法的使用方式

- 列出某资源下的方法：`lark-cli calendar <resource> -h`
- 查看方法的cli flag：`lark-cli calendar <resource> <method> -h`
- 查看方法API参数：`lark-cli schema calendar.<resource>.<method>`

`<resource>` 为 `calendars`（日历本身）/ `events`（日程）/ `event.attendees`（参与人）/ `freebusys`（忙闲）。例：`lark-cli schema calendar.events.delete`。

## 常用其他域命令

```bash
# 搜索用户，更多参数详见 lark-contact
lark-cli contact +search-user --query <query> --as user

# 搜索群聊，更多参数详见 lark-im
lark-cli im +chat-search --query <query> --as user
```

## 不在本 skill 范围

- 查询过去的视频会议记录 → [lark-vc](../lark-vc/SKILL.md)
- 待办任务管理 → [lark-task](../lark-task/SKILL.md)
- 通讯录 → [lark-contact](../lark-contact/SKILL.md)
- 即时通讯 → [lark-im](../lark-im/SKILL.md)
- 会议室物理设施管理 → 管理员后台

**注意（强制性）：**
- 涉及日期（时间）字符串与时间戳的相互转换时，务必调用系统命令或脚本代码等外部工具进行处理，以确保转换的绝对准确。违者将导致严重的逻辑错误！
