# Morning Report Template

**Purpose**: Daily planning + overnight catch-up — answers "你今天要做什么？"
**Schedule**: Mon-Fri 10:30 UTC+8
**Archive**: `reports/morning_YYYY-MM-DD.md`

## Core Question

> "今天最重要的 3 件事是什么？有什么需要立刻回复或推动的？"

## Scan Window

- Overnight messages: previous evening report time (22:00) → now (10:30)
- Tasks: all open tasks from tasklists (assignee + follower)
- Previous evening report: read for continuity
- Calendar: today's agenda via `lark-cli calendar +agenda`

## Intelligence Rules

- **TOP 3** = highest priority TASKS only: today's deadline tasks first > P0 tasks > overdue tasks. These are things you must ship/close today.
- **今日关注** = action items extracted from messages (需要回复、需要定方案、需要确认), NOT tasks. These are conversation-driven actions.
- Overnight messages: extract action items, don't just list "谁说了什么"
- If a message says "明天做X", check if "明天" is today — if so, report as "今天需要做X"
- Distinguish "需要你回复" vs "FYI" — only surface FYI if truly notable

## Output Structure (Card Format)

Card JSON template — use `--msg-type interactive`:

```json
{
  "config": { "wide_screen_mode": true },
  "header": {
    "template": "blue",
    "title": { "tag": "plain_text", "content": "☀️ 早报 — YYYY-MM-DD 周X" }
  },
  "elements": [
    {
      "tag": "markdown",
      "content": "**🎯 今日 TOP 3**\n1. **[最重要]** — 原因/截止\n2. **[第二]** — 背景\n3. **[第三]** — 背景"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**📬 需要你回复/处理**\n• @某人 在 [群名](deeplink): 问了XXX → *你需要回复XXX*\n• @某人 私聊: 发了文件 → *你需要确认*\n\n— 或：无需立即回复的事项"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**📋 待办任务** *(清单: TODO)*\n• **任务名** — 状态 · 下一步\n  ├ 子任务1 ✅\n  └ 子任务2 🔄 进行中\n• **任务名** [跟进] — 等 @某人 回复"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**⏳ 等待他人**\n• 事项 — 等 @谁 · 已等N天 · 预计XX"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**📅 今日日程**\n• 14:00 Sprint 规划 — 团队\n• 16:00 1:1 with PM"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**💬 隔夜群动态**\n• [#核心团队](deeplink): 讨论了X → 结论Y → *与你无关 / 你需要跟进Z*\n• [#架构组](deeplink): @张三 提了新方案 → *FYI*"
    },
    {
      "tag": "note",
      "elements": [{ "tag": "plain_text", "content": "10:30 UTC+8 · 扫描窗口 22:00→10:30" }]
    }
  ]
}
```

## Section Rules

| Section | Show when | Skip when |
|---------|-----------|-----------|
| 🎯 TOP 3 | Always | Never |
| 📬 需要回复 | Has action items | No messages or all FYI |
| 📋 待办任务 | Has open tasks | — |
| ⏳ 等待他人 | Waiting on someone | Nothing pending |
| 📅 今日日程 | Has calendar events | No events |
| 💬 群动态 | Groups had activity | No activity |

## Task Display

- Group by **section (guild)** — TODO first, Ongoing second, Tracking third, Content Topic hidden
- Subtask formatting:
  - Completed: `~~*subtask name*~~ ✅` (strikethrough + italic + checkmark)
  - In progress: `🔄 subtask name`
  - Indent subtasks with `\u2003\u2003` (em space x2)
- If task has `custom_fields` with priority, show it

## Text Fallback

If `output.default_format == "text"`, use the card content as markdown-like plain text (no JSON wrapper).
