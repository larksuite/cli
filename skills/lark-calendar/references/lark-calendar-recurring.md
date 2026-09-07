# 重复性日程操作规范

重复性日程/例外的编辑和删除必须显式指定操作范围。相关命令：

- `lark-cli calendar +delete` — 删除日程；重复性日程/例外必须传 `--apply-to`。
- `lark-cli calendar +update` — 更新日程；重复性日程/例外必须传 `--apply-to`。

> **强制规则：用户未明确操作范围时，必须先向用户确认，禁止 Agent 默认选取任何 `--apply-to`。** 用户表达含糊（如「删掉这个会」「改一下这个日程」）时也必须确认——`--apply-to=single` 只删/改一次，`--apply-to=all` 会影响整个序列及所有例外，代价截然不同。

## `--apply-to` 与日程类型的匹配矩阵

先记住四种日程类型：**普通日程（Normal）**、**重复性日程本体（Master）**、**重复性日程实例（Instance）**、**重复性日程例外（Exception）**。四种类型允许的 `--apply-to` 组合如下（❌ = 传入即报错）：

| 日程类型 | event_id 形状 | `single` | `all` | `this-and-following` |
|----------|--------------|:--------:|:-----:|:--------------------:|
| Normal（普通日程）          | `{uid}_0`（无 rrule） | 隐含默认 | ❌ | ❌ |
| Master（重复性日程本体）    | `{uid}_0`（有 rrule） | ❌ | ✅ | ❌ |
| Instance（重复性日程实例）  | `{uid}_{ts>0}`，`is_exception=false` | ✅ | ✅ | ✅ |
| Exception（重复性日程例外）| `{uid}_{ts>0}`，`is_exception=true`  | ✅ | ✅ | ❌（Exception 已占据该时间位，需改传另一个未被独立化的 Instance id 作为分割点） |

三个 `--apply-to` 的含义与影响面：

| 值 | 语义 | 影响面 |
|----|------|--------|
| `single` | 只操作当前这一次 | 只改/删传入的这一个 event_id 本身；例外不动其它例外，实例不动整个序列 |
| `all` | 操作整条重复性序列 | Master 本体 **和** 所有例外都会被处理（时间变更时例外先被删除，其它字段则会同步 PATCH 到每个例外） |
| `this-and-following` | 从「起始实例」起截断并新建后续序列 | 用 UNTIL 截断 Master、删除起始实例起的所有未来例外、以起始实例的时间为起点 创建 一条新序列继承 Master 的默认字段 |

## 关键概念

- **event_id 结构**：`event_id` 的格式为 `{event_uid}_{originalTime}`。`originalTime = 0` 表示 Master 或 Normal；`originalTime > 0` 表示某一次在原序列中本来的时间戳（Unix 秒）。因此 `{event_uid}_0` 即为重复性日程本体的 `event_id`。
- **Master（重复性日程本体）**：携带 `rrule` 的日程本体，`event_id` 形如 `{event_uid}_0`。序列的所有默认属性（标题、时间、rrule、描述、参会人等）都挂在本体上。
- **Normal（普通日程）**：不携带 `rrule`，`event_id` 也是 `{event_uid}_0`，但没有序列概念。只能用 `--apply-to=single`（可省略）。
- **Instance vs Exception —— 二者最容易混淆，务必区分**：
  - **Instance（实例）**：由 rrule 展开出来的「虚拟」发生点，本身不落库。`event_id` 形如 `{event_uid}_{originalTime}`（`originalTime > 0`），是从 `+agenda` / `+search-event` 返回的可寻址标识。**从未被单独编辑过**——所有属性都从 Master 继承而来。在 API 层可以对 Instance id 发 GET，但对它发写操作时（操作此次），会先把它「实体化」成一条 Exception。
  - **Exception（例外）**：某个 Instance 被显式修改（改时间、改标题、改参会人等）或被显式删除标记后落库产生的**独立日程**。`event_id` 形状与 Instance 完全一样（`{event_uid}_{originalTime}`），肉眼**无法**区分——唯一可靠的判据是 `calendar +get` 返回的 `is_exception=true`（Instance 为 false）。Exception 已经脱离 Master 的字段继承，是一份可独立编辑/删除的实体。
  - **一句话总结**：Instance 是 rrule 展开出来的「占位符」，Exception 是「已经被独立化的实例」。判断当前 event_id 是哪种，先跑 `+get` 看 `is_exception`。
- 删除/更新 Master **不会** 级联处理例外——命令内部会显式扫描并处理例外。

## 前置步骤（所有范围通用）

1. 通过 `+agenda` 或 `+search-event` 定位到目标日程 / 实例，拿到 `event_id`。
2. 判断日程类型：
   - `event_id` 后缀 `_0` 且无 `recurrence` → Normal；
   - `event_id` 后缀 `_0` 且有 `recurrence` → Master；
   - `event_id` 后缀 `_{数字>0}` 且 `is_exception=false` → Instance；
   - `event_id` 后缀 `_{数字>0}` 且 `is_exception=true` → Exception。
   - 需要精确判断时跑 `calendar +get` 看 `recurrence` 和 `is_exception` 字段。
3. **与用户确认 `--apply-to` 范围（未明确一律先问，禁止默认）**。

## 常见命令

### 删除

```bash
# 删除此次（例外或instance）
lark-cli calendar +delete --event-id <uid_originalTime> --apply-to single

# 删除全部（主日程 id 或任意 例外/instance id）
lark-cli calendar +delete --event-id <uid_originalTime> --apply-to all

# 删除此次及后续（必须传具体instance id）
lark-cli calendar +delete --event-id <uid_originalTime> --apply-to this-and-following
```

### 更新

```bash
# 编辑此次（单个实例 / 例外）
lark-cli calendar +update --event-id <uid_originalTime> --apply-to single --summary <summary>

# 编辑全部（主日程 id 或任意 例外/instance id）
lark-cli calendar +update --event-id <uid_originalTime> --apply-to all --summary <summary>

# 编辑全部：改时间
lark-cli calendar +update --event-id <uid_originalTime> --apply-to all --start <start> --end <end>

# 编辑此次及后续：截断主日程 + 删未来例外 + 创建新序列
lark-cli calendar +update --event-id <uid_originalTime> --apply-to this-and-following --summary <summary>
```

## 语义细则

- **`--apply-to=all` 时的字段传播**：只把用户本次显式传的 flag 应用到每个例外和主日程。例外原本自定义过的其他字段（例如自己的描述）保持不变。
- **`--apply-to=this-and-following` 的字段继承**：新创建的日程从原主日程继承 summary、description、rrule、start/end（用起始实例的时间）、vchat/reminders/location/visibility；用户任何显式传的 flag 优先。
- **`--start/--end` 变更**：`all` 场景下，例外会被删除（原始占位已无意义），主日程再 PATCH；`this-and-following` 场景下，`--start/--end` 传了会作为新序列的时间，否则用起始实例的时间。
- **参会人**：`this-and-following` 创建新序列时，若传了 `--add-attendee-ids`，会额外 添加attendees 到新序列；`--remove-attendee-ids` 会从新序列的参与人列表移除。
