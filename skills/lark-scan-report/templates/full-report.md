# Full Report Template

**Purpose**: Comprehensive on-demand snapshot — answers "全面了解一下现在的状态"
**Archive**: `reports/full_YYYY-MM-DD_HH-MM.md`
**Weekly log**: Yes — append after generation

## Core Question

> "你现在的全部工作是什么状态？有哪些需要立刻行动的？"

## Execution

1. Full task fetch: `tasks list` → extract tasklist GUIDs → `tasklists tasks` per GUID (both open + today's completed)
2. All scan phases from config (discover-active, whitelist, P2P, groups, @me, threads)
3. Completion detection: task API + chat signals (dual source)
4. Intelligence extraction: message → action items, message → task mapping
5. Calendar: today's remaining events
6. Archive + append to weekly log

## Output Structure (Card Format)

```json
{
  "config": { "wide_screen_mode": true },
  "header": {
    "template": "blue",
    "title": { "tag": "plain_text", "content": "📊 全量报告 — YYYY-MM-DD HH:MM" }
  },
  "elements": [
    {
      "tag": "markdown",
      "content": "**✅ 已完成（今日）**\n• [任务] **Schema 更新** — 已合并\n• [消息] 发终稿给 @设计师 — 确认收到\n• [双] **支付 bugfix** — 已部署 + 任务已关"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**🔴 需要你立刻行动**\n• **回复 @PM** — 在 #核心团队 等你定优先级\n• **Review PR #42** — @张三 3小时前提交\n• **确认设计稿** — @设计师 私聊发了终版"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**📋 全部任务**\n\n*清单: TODO (12 open / 23 done)*\n\n**[负责] 我的任务:**\n• **限流模块** — 70% · 下一步: 跑测试 · 截止 3/30\n  ├ Redis 部署 ✅\n  └ 配置迁移 🔄\n• **API 迁移** — 进行中 · 等 PR review\n\n**[跟进] 关注的任务:**\n• **AI Reddit** — @某人 负责 · 2 子任务未完成\n• **视频翻译多国内容** — @某人 负责 · 无进展更新\n• **webapp builder 调研** — @某人 负责 · 待启动"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**🚫 阻塞项**\n• **沙盒凭证** — 等 @供应商 · 3天 · 明天再催\n• **CI 不稳定** — 等 @基础设施 · 2天 · 周二 pair-debug"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**💬 重要对话摘要**\n• [#核心团队](deeplink): 上线时间 → **延后一周** · *你需要更新排期*\n• 私聊 @PM: 入职流程 → **P0** · *你需要重排 sprint*\n• [#架构组](deeplink): 限流方案 → **令牌桶** · *与你任务一致*"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**⏳ 等待他人**\n• 设计评审 — 等 @设计师 · 预计明天\n• 沙盒权限 — 等 @供应商 · 预计今天"
    },
    { "tag": "hr" },
    {
      "tag": "markdown",
      "content": "**📡 信号**\n• CTO 提到组织架构调整可能\n• 竞品上了类似功能"
    },
    {
      "tag": "note",
      "elements": [{ "tag": "plain_text", "content": "HH:MM UTC+8 · 全量扫描" }]
    }
  ]
}
```

## Section Priority

Sections appear in this exact order (skip empty ones):
1. ✅ 已完成 — lead with wins
2. 🔴 需要立刻行动 — extracted from messages, highest urgency
3. 📋 全部任务 — split by role (负责 vs 跟进), grouped by tasklist
4. 🚫 阻塞项 — with age and escalation plan
5. 💬 重要对话 — topic → decision → your action (NOT raw messages)
6. ⏳ 等待他人 — who, what, when
7. 📡 信号 — only if notable, skip if nothing

## Task Display Rules

- Group by **section (guild)** from `config.yaml → tasks.section_names`:
  - **📋 TODO** — active work items, show first
  - **🏗️ Ongoing Project** — multi-day projects, show second
  - **👀 Tracking** — monitoring others' work, show third
  - **Content Topic** — personal memos, **skip entirely**
- To get section_guid: `tasks get` → `tasklists[].section_guid`, then map via config
- Subtask display:
  - Completed: `~~*subtask name*~~ ✅` (strikethrough + italic + checkmark)
  - In progress: `🔄 subtask name`
  - Indent subtasks with `\u2003\u2003` (em space x2) for card rendering
- Show `due` date if set, highlight if overdue
- Show custom_fields if available (priority, type, brief)

## Anti-Patterns

- ❌ "AnyGen-垂类群有 5 条消息" → ✅ extract what those messages mean for the user
- ❌ listing every message → ✅ one line per conversation thread: topic → conclusion → action
- ❌ "某人说了..." → ✅ "某人确认了X / 某人需要你做Y"
