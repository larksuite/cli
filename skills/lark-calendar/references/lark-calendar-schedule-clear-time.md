# 明确时间分支：room-find + freebusy + 冲突处理

> 本文档处理**时间已明确**的场景。"明确时间"来源：用户直接表达（如"明天下午3点"）、编辑流中已定位日程的原始 start/end、或经用户确认的 suggestion 时间块。

## 前置条件

进入此分支前，调度器（[schedule-meeting.md](./lark-calendar-schedule-meeting.md)）已完成：
- 任务类型判定（新建 / 编辑）
- 编辑流：目标 event_id 已定位
- 新建流：默认值已补全
- 时间已判定为**明确**

## 流程

### 1. 查询会议室（如需）

若用户需要会议室，先调用 `+room-find`。详见 [`lark-calendar-room-find.md`](./lark-calendar-room-find.md)。

```bash
lark-cli calendar +room-find \
  --slot "<start>~<end>" \
  --attendee-ids "<ids>" \
  --city "<city>" \
  --building "<building>" \
  --floor "<F2>" \
  --room-name "<room_name>"
```

时间块确定规则：
- **编辑流且不改时间，只新增会议室**：`--slot` 必须来自已定位日程的当前 `start/end`
- **编辑流且既改时间又加会议室**：`--slot` 必须来自候选新时间，而不是旧时间

详见 [`lark-calendar-room-find.md`](./lark-calendar-room-find.md)。

### 2. 查询忙闲

```bash
lark-cli calendar +freebusy --start "<start>" --end "<end>"
```

规则：
- 参与人过多（超过 5 人）：仅查询**当前用户**及少数核心人员忙闲即可
- 参与人含**群组**：无需展开群组成员查询忙闲
- 如果用户是从 `+suggestion` 确认了时间块后进入本分支的，**无需再调用 `+freebusy`**

### 3. 冲突处理

- **无冲突**：直接让用户选择会议室（如需），进入落地操作
- **有冲突**：必须先说明冲突情况，询问用户：
  - **继续当前时间** → 让用户选择会议室（如需），进入落地操作
  - **换时间** → 转入 [模糊时间分支](./lark-calendar-schedule-fuzzy-time.md)

## 落地

根据任务类型：
- 新建 → [`+create`](./lark-calendar-create.md)
- 编辑 → [`+update`](./lark-calendar-update.md)

落地规则详见 [schedule-meeting.md § 落地日程变更](./lark-calendar-schedule-meeting.md#落地日程变更)。
