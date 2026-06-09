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

**CRITICAL — 凡涉及预约日程/会议或查询/搜索会议室，第一步 MUST 读 [`references/lark-calendar-schedule-meeting.md`](references/lark-calendar-schedule-meeting.md)。禁止跳过此步直接调用 API 或 Shortcut！**

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
| [`+agenda`](references/lark-calendar-agenda.md) | 查看日程安排（默认今天） |
| [`+create`](references/lark-calendar-create.md) | 创建日程并邀请参会人（ISO 8601 时间） |
| [`+update`](references/lark-calendar-update.md) | 更新既有日程字段，或独立增量添加/移除参会人和会议室 |
| [`+freebusy`](references/lark-calendar-freebusy.md) | 查询用户主日历的忙闲信息和 RSVP 状态 |
| [`+room-find`](references/lark-calendar-room-find.md) | 针对一个或多个**明确的**时间块查找可用会议室（无明确时间时禁止直接调用，需先走 +suggestion） |
| [`+rsvp`](references/lark-calendar-rsvp.md) | 回复日程（接受/拒绝/待定） |
| [`+suggestion`](references/lark-calendar-suggestion.md) | 根据非明确时间或一段时间范围，推荐多个可用时间块方案 |

## 前置条件路由

| 场景 | 前置要求 |
|------|----------|
| 预约日程/会议、查会议室 | 先读 [lark-calendar-schedule-meeting.md](references/lark-calendar-schedule-meeting.md) |
| 编辑已有日程 | 先定位目标日程 `event_id`；若是重复性日程，必须定位到具体实例的 `event_id`（禁止使用原重复日程 ID） |
| 删除/修改后验证 | 等待 2 秒再查询（API 最终一致性），不要告知用户你等待了 |
| 调用任何 Shortcut | 先读其对应 reference 文档 |

## 核心概念

- **日程实例（Instance）**：重复性日程展开后的具体时间实例。操作重复日程的某次实例时，必须先定位该实例的 `event_id`，禁止使用原重复日程的 `event_id`。
- **全天日程（All-day Event）**：只按日期占用、没有具体起止时刻的日程，结束日期是包含在日程时间内的。
- **时间块 vs 时间范围**：时间块是具体确定的连续时间段（如 `14:00~15:00`），时间范围是泛指（如"今天下午"）。`+room-find` 必须基于确定时间块，不能基于模糊范围。
- **会议室（Room）**："room"不是"房间"，是"会议室"。会议室是日程的一种参与人（resource attendee），不能脱离日程单独预定。

## 术语映射

用户日常说的"帮我约个日历""查一下今天的日历"，实际意图是针对**日程（Event）**的创建或查询，而非操作日历（Calendar）容器本身。自动将口语化的"日历"意图映射为"日程"操作。

## 意图路由

| 用户意图 | 路由到 |
|----------|--------|
| 查询过去的会议（"昨天的会议""上周的会"） | [`../lark-vc/SKILL.md`](../lark-vc/SKILL.md)（会议数据含即时会议，仅查日程会遗漏） |
| 查询日历/日程或未来时间的会议 | 本 skill |
| 预约/改约日程、添加/移除参会人、添加/更换会议室、调整时间 | 先判断新建 vs 编辑，再进入 [schedule-meeting 工作流](references/lark-calendar-schedule-meeting.md) |

## 任务类型分流

处理"预约/改约日程、添加/移除参会人、添加/更换会议室、调整时间"时，必须先判断新建 vs 编辑：

- **编辑已有日程的强信号**：用户提到已存在的日程锚点（标题、时间段、`这个日程`、`这场会`）并表达修改动作（添加、移除、改到、换会议室、调整时间）。默认走编辑流，绝不能按新建处理。
- **新建日程**：用户表达新增意图（"新约一个会""创建一个日程""安排一次会议"），且没有指向既有日程的修改动作。

## 时间推断规范

- **星期的定义**：周一是一周的第一天，周日是最后一天。计算"下周一"等相对日期时，基于当前真实日期推算。
- **一天的范围**：用户提到"明天""今天"等泛指某天时，时间范围应覆盖整天，不要自行缩减。
- **历史时间约束**：不能预约已经完全过去的时间。唯一例外是"跨越当前时间"的日程（开始在过去、结束在未来）。

## 会议室规则

- 凡是"预定/查询/搜索可用会议室"，都必须进入 [schedule-meeting 工作流](references/lark-calendar-schedule-meeting.md)。
- `+room-find` 的时间输入必须是确定时间块，不能是时间区间搜索。
- 用户仅要求"查会议室"但未提供明确时间时，必须先调用 `+suggestion` 获取可用时间块，再将时间块交给 `+room-find`。严禁猜测时间盲目调用。
- 编辑已有日程时，"添加会议室"默认是增量语义，保留已有会议室；只有用户明确说"更换会议室""移除会议室"时才删除旧会议室。

## API Resources

```bash
lark-cli calendar <resource> <method> [flags]
```

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
  - `search_event` — 搜索日程（仅返回 日程ID/主题/时间，详情需走 `events get`）
  - `share_info` — 获取日程分享链接

### freebusys

  - `list` — 查询主日历日程忙闲信息

## 不在本 skill 范围

- 查询过去的视频会议记录 → [lark-vc](../lark-vc/SKILL.md)
- 待办任务管理 → [lark-task](../lark-task/SKILL.md)
- 会议室物理设施管理 → 管理员后台

**注意（强制性）：**
- 涉及日期（时间）字符串与时间戳的相互转换时，务必调用系统命令或脚本代码等外部工具进行处理，以确保转换的绝对准确。违者将导致严重的逻辑错误！
