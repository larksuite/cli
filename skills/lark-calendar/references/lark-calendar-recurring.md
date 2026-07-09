# 重复性日程操作规范

重复性日程的编辑/删除分为三种范围：「仅此次」「全部」「此次及后续」。用户未明确范围时，**必须询问确认**。

## 关键概念

- **event_id 结构**：`event_id` 的格式为 `{event_uid}_{originalTime}`。普通日程或重复性日程本体的 `originalTime` 为 `0`；例外的 `originalTime > 0`，代表该例外在原重复性序列中本来的时间位置。因此 `{event_uid}_0` 即为原重复性日程的 `event_id`。
- **原重复性日程**：携带 `rrule` 的日程本体，`event_id` 形如 `{event_uid}_0`。系列的所有属性（标题、时间、rrule、描述等）都挂在本体上。
- **例外（Exception）**：对某次实例做过「仅此次」编辑后产生的独立日程，`event_id` 形如 `{event_uid}_{originalTime}`（`originalTime > 0`）。通过 `event_uid` 部分即可关联回原重复性日程。
- 删除/更新原重复性日程 **不会** 级联处理例外——必须手动逐个处理。

## 前置步骤（所有范围通用）

1. 通过 `+agenda` 或 `+search-event` 定位重复性日程，获取原重复性日程的 `event_id`。
2. 通过 `events instance_view` 或 `+agenda` 列出实例，识别哪些是例外（`event_id` 中 `originalTime > 0` 的即为例外）。
3. 确认用户的操作范围。

## 编辑全部（更新时间）

| 步骤 | 命令 | 说明 |
|------|------|------|
| 1 | `lark-cli calendar +update --event-id <原重复日程ID> --start ... --end ...` | 更新原重复性日程的时间 |
| 2 | `lark-cli calendar events delete --params '{"calendar_id":"<CAL_ID>","event_id":"<例外ID>","need_notification":false}'` （逐个） | 时间变更后例外已无意义，必须删除 |

> 理由：更新时间会改变重复起止点，例外日程的原始占位已变，若保留会导致时间冲突或残留。

## 编辑全部（更新非时间字段）

| 步骤 | 命令 | 说明 |
|------|------|------|
| 1 | `lark-cli calendar +update --event-id <原重复日程ID> --summary ... --description ...` | 更新原重复性日程的标题/描述等 |
| 2 | `lark-cli calendar +update --event-id <例外ID> --summary ... --description ...` （逐个） | 同步更新例外日程的对应字段 |

> 理由：例外已脱离原重复性日程独立存在，不会自动继承原日程的更新。

## 删除全部

| 步骤 | 命令 | 说明 |
|------|------|------|
| 1 | `lark-cli calendar events delete --params '{"calendar_id":"<CAL_ID>","event_id":"<原重复日程ID>","need_notification":true}'` | 删除重复性日程本体 |
| 2 | `lark-cli calendar events delete --params '{"calendar_id":"<CAL_ID>","event_id":"<例外ID>","need_notification":false}'` （逐个） | 删除所有例外日程 |

> 理由：例外是独立实体，删除原重复性日程不会级联删除例外。

## 编辑此次及后续

| 步骤 | 命令 | 说明 |
|------|------|------|
| 1 | `lark-cli calendar +update --event-id <原重复日程ID> --rrule "FREQ=...;UNTIL=<截止日期>"` | 截短原重复性日程（UNTIL 设为指定时间前一次实例的日期） |
| 2 | `lark-cli calendar events delete ...` （逐个） | 删除指定时间之后（含）的例外日程 |
| 3 | `lark-cli calendar +create --summary ... --start <指定时间> --end ... --rrule "FREQ=..." --attendee-ids ...` | 从指定时间开始创建新的重复性日程（即「后续」部分，携带编辑后的内容） |

> UNTIL 计算规则：若用户选择「从第 N 次开始编辑」，UNTIL 应设置为第 N-1 次实例的日期（即保留到指定时间之前的最后一次）。
> 新日程应继承原日程的参会人、会议室等配置（除非用户明确要修改）。

## 删除此次及后续

| 步骤 | 命令 | 说明 |
|------|------|------|
| 1 | `lark-cli calendar +update --event-id <原重复日程ID> --rrule "FREQ=...;UNTIL=<截止日期>"` | 截短原重复性日程（UNTIL 设为指定时间前一次实例的日期） |
| 2 | `lark-cli calendar events delete ...` （逐个） | 删除指定时间之后（含）的例外日程 |

> 与「编辑此次及后续」的区别：不需要步骤 3（创建新的重复性日程），因为目标是删除后续而非替换。

## 仅此次

- **编辑仅此次**：通过 `+agenda` / `+search-event` 定位到具体实例的 `event_id`，然后正常调用 `+update`。
- **删除仅此次**：定位到具体实例的 `event_id`，调用 `events delete`。

## 用户意图映射

| 用户表达 | 操作范围 |
|----------|----------|
| 「改这个重复日程的标题」「全部改」「每次都改」 | 编辑全部 |
| 「删掉这个重复日程」「取消所有」 | 删除全部 |
| 「从下周开始改时间」「后面的都改」 | 编辑此次及后续 |
| 「从下周开始不要了」「后面的都删」 | 删除此次及后续 |
| 「就改这一次」「只删这一次」 | 仅此次 |
| 未明确范围 | **必须询问用户** |

## 注意事项

- 涉及时间戳计算（如推算 UNTIL 日期）时，必须调用系统命令或脚本，禁止心算。

## 参考

- [lark-calendar](../SKILL.md) — 日历全部命令
- [lark-calendar-update](lark-calendar-update.md) — 更新日程 Shortcut
- [lark-calendar-create](lark-calendar-create.md) — 创建日程 Shortcut
- [lark-shared](../../lark-shared/SKILL.md) — 认证和全局参数
