---
name: lark-calendar
version: 1.0.0
description: "飞书日历（calendar）：提供日历与日程（会议）的全面管理能力。核心场景包括：查看/搜索日程、创建/更新日程、管理参会人、查询忙闲状态及推荐空闲时段。高频操作请优先使用 Shortcuts：+agenda（快速概览今日/近期行程）、+create（创建日程并按需邀请参会人）、+freebusy（查询用户主日历的忙闲信息和rsvp的状态）、+rsvp（回复日程邀请）、+suggestion（针对时间未确定的预约日程需求，提供多个时间推荐方案）。"
metadata:
  requires:
    bins: ["lark-cli"]
  cliHelp: "lark-cli calendar --help"
---

# calendar (v4)

**CRITICAL — 开始前 MUST 先用 Read 工具读取 [`../lark-shared/SKILL.md`](../lark-shared/SKILL.md)，其中包含认证、权限处理**
**CRITICAL — 所有的 Shortcuts 在执行之前，务必先使用 Read 工具读取其对应的说明文档，禁止直接盲目调用命令。**

## 核心场景

日历技能包含以下核心场景：

### 1. 预约日程

这是日历技能最核心的场景，核心是让用户低成本地实现日程预约。

> **💡 核心原则：做智能助理，提供辅助决策，而不是表单填写机或替用户做主。**

**时间与日期推断规范：**
为确保准确性，在涉及时间推断时，请严格遵循以下规则：
- **星期的定义**：周一是一周的第一天，周日是一周的最后一天。计算`下周一`等相对日期时，务必基于当前真实日期和星期基准进行推算，避免算错日期。
- **一天的范围**：当用户提到`明天`、`今天`等泛指某一天时，时间范围应默认覆盖整天时间范围。**切勿**自行缩减查询范围，以免遗漏晚上的时间安排。
- **历史时间约束**：不能预约已经完全过去的时间。唯一的例外情况是“跨越当前时间”的日程，即日程的开始时间在过去，但结束时间在未来。

**预约日程的工作流：**

1. **智能推断默认值**
   - 标题，参与人，时长均存在默认值，无需频繁的和用户确认。
   - **参会人**：如未明确指定其他人，默认参会人仅为**用户自己**。当搜索特定参与人（人、群、会议室）出现多个结果无法唯一确定时，必须询问用户进行选择确认，并将该偏好记录为长期记忆，以便后续自动识别。
   - **会议室**：目前不支持主动预定会议室，除非当前上下文中已经存在对应的会议室ID(omm_ 前缀) 且需要添加到日程中。
   - **标题**：根据对话上下文自动生成（例如“沟通对齐”或“需求讨论”），如无法推断则默认为“会议”。
   - **时长**：基于会议类型和上下文动态推断（例如：“评审/汇报”推断为 60 分钟等），如无法推断，则默认为 30 分钟。

2. **时间建议与辅助决策（核心体验）**
   - **有明确时间点**（如`明早10点`）：调用相关工具（如 `lark-cli calendar +freebusy` [lark-calendar-freebusy](references/lark-calendar-freebusy.md)）先查询该时间段参会人的忙闲状态（注：若参会人已有日程的 RSVP 状态为拒绝，则认为该时段为空闲）。若均无冲突，直接进入下一步确认并创建；若有冲突，提示用户冲突情况并询问是否继续创建或重新选择时间。
   - **有时间区间**（如`明天`、`下午`、`本周`）：调用相关工具（如 `lark-cli calendar +suggestion` [lark-calendar-suggestion](references/lark-calendar-suggestion.md)）获取该区间内所有参会人的**多个时间推荐方案**供用户选择。**必须在用户确认方案后**，才能执行创建日程操作；且用户一旦选择了推荐的方案，**无需再次查询忙闲信息**。
   - **无任何时间信息**：默认推断一个合理区间（如“今天”或“近两天”），并同样获取**多个时间推荐方案**供用户快速选择。
   - **生活类需求**（如健身、游泳、遛弯、约饭、奶茶等，注意“约咖啡”算工作场景）：预期**不调用** `suggestion` 工具。应自行推断合适的非工作时间给到用户确认。如果无法推断，请尝试主动询问用户，并在用户给出反馈后形成记忆，以便后续直接应用。
   - **模糊语义消解与长期记忆构建 (Aha Moment)**：针对用户专属的时间表达习惯（如“上班后”、“下班前”）或存在歧义的时间场景（如未指明上下午的12小时制），严禁主观臆断。应通过主动澄清明确真实意图，并将此类个性化定义沉淀为长期偏好，推动系统认知能力的持续进化，最终实现“下次即懂”的智能化体验。

3. **非阻断式执行**
   - 待用户确认具体时间选项后，执行 `lark-cli calendar +create --summary "..." --start "..." --end "..." --attendee-ids ...`

4. **友好反馈**
   - 报告结果：返回创建成功的日程摘要信息

## 核心概念

- **日历（Calendar）**：日程的容器。每个用户有一个主日历（primary calendar），也可以创建或订阅共享日历。
- **日程（Event）**：日历中的单个事件条目，包含起止时间、地点、标题、参与人等属性。支持单次日程和重复日程，遵循RFC5545 iCalendar国际标准。
- ***全天日程（All-day Event）***: 只按日期占用、没有具体起止时刻的日程，结束日期是包含在日程时间内的。
- **日程实例（Instance）**：日程的具体时间实例，本质是对日程的展开。普通日程和例外日程对应1个Instance，重复性日程对应N个Instance。在按时间段查询时，可通过实例视图将重复日程展开为独立的实例返回，以便在时间线上准确展示和管理。
- **重复规则（Rrule/Recurrence Rule）**：定义重复性日程的重复规则，比如`FREQ=DAILY;UNTIL=20230307T155959Z;INTERVAL=14`表示每14天重复一次。
- **例外日程（Exception）**：重复性日程中与原重复性日程不一致的日程。
- **参会人（Attendee）**：日程的参与者，可以是用户、群、会议室资源、外部邮箱地址等。每个参与人有独立的RSVP状态。
- **响应状态（RSVP）**：参与人对日程邀请的回复状态（接受/拒绝/待定）。
- **忙闲时间（FreeBusy）**：查询用户在指定时间段的忙闲状态，用于会议时间协调。

## 资源关系

```
Calendar (日历)
└── Event (日程)
    ├── Attendee (参会人)
    └── Reminder (提醒)
```

## Shortcuts（推荐优先使用）

Shortcut 是对常用操作的高级封装（`lark-cli calendar +<verb> [flags]`）。有 Shortcut 的操作优先使用。

| Shortcut | 说明 |
|----------|------|
| [`+agenda`](references/lark-calendar-agenda.md) | 查看日程安排（默认今天） |
| [`+create`](references/lark-calendar-create.md) | 创建日程并邀请参会人（ISO 8601 时间） |
| [`+freebusy`](references/lark-calendar-freebusy.md) | 查询用户主日历的忙闲信息和rsvp的状态 |
| [`+rsvp`](references/lark-calendar-rsvp.md) | 回复日程（接受/拒绝/待定） |
| [`+suggestion`](references/lark-calendar-suggestion.md) | 针对时间未确定的预约日程需求，提供多个时间推荐方案 |

## +suggestion 使用
在调用 `+suggestion` 之前，务必读取 [lark-calendar-suggestion](references/lark-calendar-suggestion.md) 中的使用说明，禁止直接调用命令。
```bash
lark-cli calendar +suggestion --start "2026-03-10T00:00:00+08:00" --end "2026-03-10T11:00:00+08:00" --attendee-ids "ou_xxx,oc_yyy" --duration-minutes 30 # 为用户ou_xxx和群组oc_yyy里的成员推荐空闲时段
`````

## API Resources

```bash
lark-cli schema calendar.<resource>.<method>   # 调用 API 前必须先查看参数结构
lark-cli calendar <resource> <method> [flags] # 调用 API
```

> **重要**：使用原生 API 时，必须先运行 `schema` 查看 `--data` / `--params` 参数结构，不要猜测字段格式。

### calendars

  - `create` — 创建共享日历
  - `delete` — 删除共享日历
  - `get` — 查询日历信息
  - `list` — 查询日历列表
  - `patch` — 更新日历信息
  - `primary` — 查询用户主日历
  - `search` — 搜索日历

### event.attendees

  - `batch_delete` — 删除日程参与人
  - `create` — 添加日程参与人
  - `list` — 获取日程参与人列表

### events

  - `create` — 创建日程
  - `delete` — 删除日程
  - `get` — 获取日程
  - `instance_view` — 查询日程视图
  - `patch` — 更新日程
  - `search` — 搜索日程

### freebusys

  - `list` — 查询主日历日程忙闲信息

## 权限表

| 方法 | 所需 scope |
|------|-----------|
| `calendars.create` | `calendar:calendar:create` |
| `calendars.delete` | `calendar:calendar:delete` |
| `calendars.get` | `calendar:calendar:read` |
| `calendars.list` | `calendar:calendar:read` |
| `calendars.patch` | `calendar:calendar:update` |
| `calendars.primary` | `calendar:calendar:read` |
| `calendars.search` | `calendar:calendar:read` |
| `event.attendees.batch_delete` | `calendar:calendar.event:update` |
| `event.attendees.create` | `calendar:calendar.event:update` |
| `event.attendees.list` | `calendar:calendar.event:read` |
| `events.create` | `calendar:calendar.event:create` |
| `events.delete` | `calendar:calendar.event:delete` |
| `events.get` | `calendar:calendar.event:read` |
| `events.instance_view` | `calendar:calendar.event:read` |
| `events.patch` | `calendar:calendar.event:update` |
| `events.search` | `calendar:calendar.event:read` |
| `freebusys.list` | `calendar:calendar.free_busy:read` |

**注意（强制性）：**
- 涉及日期（时间）字符串与时间戳的相互转换时，务必调用系统命令或脚本代码等外部工具进行处理，以确保转换的绝对准确。违者将导致严重的逻辑错误！