# Quick Scan Template

**Purpose**: Attention recap after a break — answers "我关注的事有进展吗？"
**Archive**: No
**Weekly log**: No

## Core Logic

Quick scan refreshes the user's **active attention** — not a generic message dump. Three layers:

1. **Task focus refresh**: Show TODO (today DDL + P0) and Tracking tasks — are there chat updates that move these forward?
2. **@你 + 需回复**: Anything requiring action since last scan
3. **Overall digest**: If messages exist but none need action, one-line summary per active group

## Execution

1. Read `.last-scan-ts` for delta window
2. Pull latest 10 messages from each whitelist group (2-4 API calls)
3. Filter to messages AFTER last scan
4. Match new messages against TODO + Tracking tasks (keyword/people match)
5. No full task re-fetch — use cached task list from morning/last report

## Output Structure (Card Format)

**Has actionable updates:**
```json
{
  "config": { "wide_screen_mode": true },
  "header": {
    "template": "turquoise",
    "title": { "tag": "plain_text", "content": "⚡ N 条新 · Xh 前" }
  },
  "elements": [
    {
      "tag": "markdown",
      "content": "**@你:**\n• [群名](link) 某人：内容 → *需回复*\n\n**任务进展:**\n• **订阅弹窗** — [群名](link) 某人确认了视觉稿 → 可进下一步\n• **任务机制改版** — [群名](link) AA实验结果出了 → 查看\n\n→ **需处理 N / FYI N**"
    }
  ]
}
```

**Has messages but nothing actionable:**
```json
{
  "config": { "wide_screen_mode": true },
  "header": {
    "template": "turquoise",
    "title": { "tag": "plain_text", "content": "⚡ N 条新 · 无需处理 · Xh 前" }
  },
  "elements": [
    {
      "tag": "markdown",
      "content": "[群名](link) 在聊X · [群名](link) 在聊Y"
    }
  ]
}
```

**No updates:**
```json
{
  "config": { "wide_screen_mode": true },
  "header": {
    "template": "turquoise",
    "title": { "tag": "plain_text", "content": "⚡ 无新消息 · 距上次 Xh" }
  },
  "elements": []
}
```

## Rules

- **@你 first** — if someone @'d you, that's #1
- **Task match second** — scan new messages for keywords matching TODO/Tracking task names. If a message updates a task's status, show it as `**任务名** — [群](link) 进展摘要`
- **One-line overall last** — if messages exist but don't match any task or @you, compress to `[群](link) 在聊X` per group, all on one line
- **Never repeat task list** — user already knows their tasks from morning report. Only show tasks that had NEW activity.
- Target: ≤8 lines. Header carries the verdict (需处理/无需处理/无新消息)
- Do NOT include signals/analysis — just delta facts
