# 日程与视频会议的关系

用户口中的「会议」不区分「日程」和「视频会议」，实际是两类不同实体。本文定义两者关系，并给出「当前 / 未来 / 过去」三种查询意图的执行流程。

## 核心概念

- **日程（Calendar Event）**：对用户一段时间的预占，到点后可能开视频会议、也可能只是线下会议 / 私人时间块。
- **视频会议（VC Meeting）**：实际发生过的一次通话，`meeting_id` 只有真正发起后才存在。

| 场景 | `event_id` | `meeting_id` | 备注 |
|------|:---:|:---:|------|
| 日程发起了视频会议 | ✓ | ✓ | 一个日程可发起多次通话，产出多个 `meeting_id` |
| 日程未开视频会议 | ✓ | ✗ | 线下会议 / 私人时间块 |
| 即时视频会议 | ✗ | ✓ | 无日程绑定 |

**关键不变量**：视频会议只发生在**当下和过去**，不存在「未来的视频会议」。

## 意图 1：查询当前正在开的会议

**目标覆盖**：当下时间点用户可能关心的所有活动——正在开的视频会议 + 当前时间的日程（无论有没有开视频）。

**执行步骤**：

```bash
# 1. 当前时间的日程
lark-cli calendar +agenda --start <now> --end <now>

# 2. 用户已加入的视频会议
lark-cli vc +meeting-list-active --as user

# 3. 步骤 1 每个日程回查关联 meeting_id
#    输出是 event_id → meeting_id 映射；后续所有交叉都按 meeting_id 匹配
#    （vc +meeting-list-active / vc +detail 结果的 id 字段即 meeting_id，无 event_id）
lark-cli calendar +meeting --event-ids <event_id1>,<event_id2>

# 4. 判定视频会议是否仍在进行
#    仅对「步骤 3 非空 meeting_id 且不在步骤 2 里」的调用
#    end_time 为空或 <= start_time → 仍在进行；否则已结束
lark-cli vc +detail --meeting-ids <meeting_id1>,<meeting_id2>
```

**结果分组呈现**：按下列**四组顺序**归类，每组独立成节，空组可省略。

1. **当前用户正在参与的会议**（`meeting_id` 命中步骤 2）
   - **即时会议**（无关联 `event_id`）：仅展示视频会议信息。
   - **日程会议**（能与步骤 3 的 `event_id` 关联）：展示日程信息 + 视频会议信息。
2. **当前正在视频会议的日程（用户未加入）**：日程有 `meeting_id`、步骤 4 判定仍在进行、但不在步骤 2 里。展示日程信息 + 视频会议信息。
3. **当前正在进行中的日程（视频会议已结束）**：日程仍在时间窗内、有 `meeting_id`，但步骤 4 判定已结束。展示日程信息 + 视频会议信息（标注「已结束」）。
4. **当前正在进行中的日程（未开启视频会议）**：日程仍在时间窗内，步骤 3 回查无 `meeting_id`。仅展示日程信息。

## 意图 2：查询未来的会议

**只有日程视角**：视频会议只发生在当下和过去，用户说的「未来的会议」等价于「未来的日程」。

```bash
# 二选一：无关键词 → +agenda；有关键词 → +search-event
lark-cli calendar +agenda --start <future_start> --end <future_end>
lark-cli calendar +search-event --query <keyword> --start <future_start> --end <future_end>

# 禁用：vc +search 对未来返回空，容易被误判「没有会议」
# lark-cli vc +search --start <future> --end <future>   ← 不要这样做
```

若用户明确要求「未来的视频会议」，仍返回日程列表并**主动说明**：视频会议是否真正开要等到时间到达才能确定。

## 意图 3：查询过去的会议

**目标覆盖**：过去时间段内发生过的视频会议（含即时会议）+ 过去时间段的日程（含未开视频会议的）。

**执行步骤**：

```bash
# 1. 过去的视频会议（含即时会议——仅查日程会漏掉）
lark-cli vc +search --start <past_start> --end <past_end>

# 2. 过去的日程
lark-cli calendar +agenda --start <past_start> --end <past_end>

# 3. 步骤 2 每个日程回查 meeting_id，构建 meeting_id → event_id 映射
#    遍历步骤 1 每条结果，用其 id 字段（即 meeting_id）查此映射：
#    命中 → 日程视频会议；未命中 → 无日程的即时会议
lark-cli calendar +meeting --event-ids <event_id1>,<event_id2>
```

**结果分组**：按三组顺序呈现。

1. **无日程的即时视频会议**：步骤 1 里找不到关联 `event_id`。仅展示视频会议信息。
2. **日程视频会议**：日程 + 关联 `meeting_id`。同时展示日程信息和视频会议信息。
3. **未开视频会议的日程**：日程存在但步骤 3 回查无 `meeting_id`。仅展示日程信息。

## 常见判断路径

| 用户输入 | 动作 |
|----------|------|
| 只给了「会议标题」 | 不确定是日程标题还是即时会议标题，**同时**查 `calendar +search-event --query <标题>` 与 [`lark-meeting`](../../lark-meeting/SKILL.md) 的 `vc +search --query <标题>`，交叉后按上述意图分流 |
| 直接给了 `meeting_id` | 直接进入 [`lark-meeting`](../../lark-meeting/SKILL.md)，跳过日程 |
| 相对锚点（「今天下午 3 点那个会」） | 先 `+agenda` 定位日程，再按意图 1 或 3 判断 |
| 过去锚点（「昨天开的会」） | **禁止只查 `+agenda`**——必须同时查 `vc +search`，否则漏掉即时会议 |
| 未来锚点（「明天下午的会」） | 只查日程，不查 `vc +search`（未来永远返回空） |
