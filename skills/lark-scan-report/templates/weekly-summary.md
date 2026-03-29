# Weekly Summary Template

**Purpose**: Generate weekly report draft — answers "这周做了什么？下周做什么？"
**Schedule**: Sunday 15:00 UTC+8
**Archive**: `reports/weekly_YYYY-WNN.md`

## Core Scenario

User needs to write a weekly report (周报). The output should be a **draft they can directly paste**, structured by business module, with concrete deliverables and data points. NOT a message recap.

## Data Sources

1. **Weekly log** (`reports/weekly-logs/week-YYYY-WNN.md`) — accumulated daily entries
2. **Weekend delta** — Fri 22:00 → Sun 15:00
3. **Fresh task pull** — current open tasks for next-week plan
4. **Chat messages** — mine for data points, decisions, deliverables mentioned in conversations

## Output Structure (Card Format)

```json
{
  "config": { "wide_screen_mode": true },
  "header": {
    "template": "violet",
    "title": { "tag": "plain_text", "content": "📅 周报草稿 — W13 (3/24→3/28)" }
  },
  "elements": [
    {
      "tag": "markdown",
      "content": "**本周进展**\n\n**[模块名]** @负责人\n• 具体完成项，带数据/链接\n• 具体完成项\n  - 细节补充\n\n**[模块名]** @负责人\n• ..."
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**下周计划**\n\n**[模块名]**\n1. 计划项\n2. 计划项\n\n**[模块名]**\n1. 计划项"
    }
  ]
}
```

## Intelligence Rules

### 本周进展 — Extraction Logic

Group by **business module** (not by day/group/task). Derive modules from:
- Task section names (TODO, Ongoing Project items)
- Chat group topics (增长&留存 → 增长策略, 裂变/积分 → 裂变与积分体系, etc.)
- Cluster related items into the same module

For each module:
- **Lead with deliverables**: what shipped, what launched, what decision was made
- **Include data points**: any numbers mentioned in chat (DAU, 转化率, 积分消耗, etc.)
- **Include experiment status**: if an A/B test or experiment was discussed, state: launched/观测中/结论
- **Include blockers resolved / remaining**
- **Attribute to people**: `@某人` for accountability

### 下周计划 — Derivation Logic

Derive from:
1. **Open TODO tasks** — things that must ship next week
2. **Unresolved discussion threads** — decisions pending from this week's conversations
3. **Experiment observation** — things in "再观察一周" state
4. **Tracking tasks with upcoming milestones**

Keep it brief: 2-4 items per module, one line each.

### Format Rules

- Mirror the user's actual 周报 style: module headers with @owner, bullet + sub-bullet, data inline
- Chinese language, concise professional tone
- No emoji in the report body (this is for pasting into formal docs)
- Data first, narrative second: "付费率 US/SG 正向，BR 持平" not "我们观察到一些积极的信号"
- Link to relevant docs/dashboards if URLs appeared in messages

## After Generation

Freeze weekly log. Update `.last-scan-ts`.
